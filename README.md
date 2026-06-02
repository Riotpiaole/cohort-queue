# Elective Take Home (Cohort)

A small **cohort waiting-list** system, modelled as an SQS-style queue: creators
(each represented by the number `1`) wait in fixed-capacity **cohorts** (FIFO,
newest on the left, oldest served first), and **ops workers** consume the oldest
cohort on demand.

### Stack
- **Frontend** — Express + TypeScript. A web UI that visualizes the queue as an
  array of cohort counts (e.g. `[8, 10, 10, 6]`), collapsing the middle past 20
  cohorts.
- **Backend** (`backend/`) — Go, one binary with two modes:
  - **coordinator** — owns the mutex-protected FIFO queue; serves the HTTP API
    (`create` / `add` / `take` / `pull` / `total` / `state`) for the frontend and a
    gRPC API (`PullTask` / `TaskComplete`) for workers; checkpoints state to
    `{WORKSPACE_FOLDER}/checkpt` for crash recovery.
  - **worker** — blocks on gRPC until a user **Pull** is requested, then consumes
    one (oldest) cohort. Consumption is **demand-driven**, not auto-draining.
- **Deploy** — three `v1.0.0` images on **minikube** (coordinator, worker,
  frontend), wired together in-cluster; the frontend reverse-proxies `/api/*` to
  the coordinator.

See [`PROBLEMS.md`](PROBLEMS.md) for the original spec and [`LOG.md`](LOG.md) for
the per-session work log and exact verification commands.

## Getting Started

### Prerequisites
Node 20+, Go 1.23+, Docker, `make`, `minikube`, and `kubectl`.

> **Docker must be running before you start.** minikube uses the docker driver, so
> start Docker Desktop (or your Docker daemon) first — `docker info` should succeed.
> `npm run up` will start minikube for you, but it cannot start Docker.

### Run the whole stack on minikube — one command
```bash
npm install
npm run up        # build all 3 images, start/use minikube, deploy, then open the UI
```
(If minikube isn't running yet, `npm run up` starts it with `--driver=docker`.)
`npm run up` opens the frontend in your browser and holds a tunnel (Ctrl-C to
stop). Other helpers (all `npm run` wrappers around the root `Makefile`):

| Command | Does |
|---|---|
| `npm run up`     | cluster → build images → deploy → open the UI |
| `npm run open`   | re-open the frontend in the browser |
| `npm run url`    | print the frontend URL |
| `npm run status` | show deployments / pods / services |
| `npm run logs`   | tail coordinator + worker logs |
| `npm run down`   | remove the workloads |

### Using the UI
- **Add** — fills cohorts (newest on the left).
- **Pull (worker)** — asks a worker to consume the oldest cohort (one per click).
- **Take (admin)** — serves creators directly from the coordinator.
- **Refresh / Total** — re-reads the live state (in-flight + lifetime-added shown too).

### Frontend-only dev (no k8s)
Serve the Express frontend on `:3000`, proxying `/api/*` to a coordinator at
`$COORDINATOR_URL` (default `http://localhost:8080`):
```bash
npm run dev
# in another shell, run a coordinator locally:
cd backend && WORKSPACE_FOLDER=$PWD go run ./cmd/cohort --mode=coordinator
```
Backend tests: `cd backend && go test ./...`.

---

## Build journal

> The sections below capture how the project was built, prompt by prompt.

- First, i will interact with the AI to understand the problem
    - This include understand the core concept of the cohort and what does the number is representing. 
- And i will load up my own SoftwareDev prompt to work withe the AI. 
    - view in this [gist](https://gist.github.com/Riotpiaole/d81bda8adcc866ab22cd44dfeade3973)
    - And ask `claude code` to load it up. 
    - And read the `PROBLEMS.md` in the memory and compact it.

> NOTE, you can read LOGS.md for the working session of claude code and testable approach for the state of the project.


## Frontend visualization 

- From the problem, i can summarized the problem is designing SQS. Where each entry is represent a queue with collection of creator (product) waitting to be consume. 

- From there i need to figured out the way we can complete the problem. I will ask AI to implement me an Express JS app where leverage static value for representation in plan mode.

> Prompt:
```
Can u build me an ExpressJS App where staticly interact and represent it ? 
 - It should contains 4 API buttons 
 - The visualization should be an array of number similar to PROBLEMS. The size of the array should be kept up to 20 of them and addition tail item should convert to .... 
  - The state of the system should make it into a static version just for demo purpose. 
```

## Backend setup 

- We need to figure out How do we setup the backend in terms of scaling. 
    - We can scale the numbers of ops work to pull down the cohort group to consume. (Scalability)
    - We can concurrently scale the numbers of ops worker pulling. (Latency)
    - We an scale the size of the cohort to hold more creator in one of the entry 

- So each time a frontend call `Pull` should be processed by a pool of process and handle by a mutex to avoid dirty write.

- For the cohort seems like a storage representation of bucket for holding creator. The bucket can considered as a permanent storage with `N` size. And changing it can rethink scale the storage space or volume to bigger size.
  - Of course there exists migration of the bucket can be  (old bucket down or up size) a new topic to discuss. But in this we assume we always refresh a new queue when New is called.
  - Due to the limit of time, I assume we use a simple fifo queue sitting at the backend for coordination of the work.

- Now I can refine a new prompt for claude code to work on 

```
Following the same context of the problem, Make a folder backend which have 4 APIs specified using golang.

- The service can support two mode, one is coordinator (backend intergate with frontend) and worker (where communicate with backend for pull and consumes the message)

- The worker and coordinator are communicate with gRPC with two call `PullTask` and `TaskComplete`.

- The coordinator should have in memory static queue which protected with mutex for modification. 

- The failure recovery should be handled as a checkpointing with local storage in `{WORKSPACE_FOLDER}/checkpt` 

- There should exists two images to be built in version v1.0.0 and dockerize for k8s demo purpose

```

## Integration (Put everything together)

- Now you can simply write a prompt to generate frontend calls

```
Now integrate the frontend with the coordinator service using localhost.
```


## Test and Iteration 

- When i am doing a manual test, i found the Add would randomly delete all the entry within the queue. I notice the direction of the trigger should changed from worker pulling from the queue into user click submit a job and worker pull from the job queue which consume an entry within coordinator fifo queue.

- I basically write the issue to AI, and ask him to write me a plan. 

```
I think i got the wrong idea, worker should pull from the queue upon the user submitted a request. We should add a addition buffer between the worker and coordinator. So the user click pull or add, it should be a queue parse up the message and distribute to the coordinator for consumption.
```