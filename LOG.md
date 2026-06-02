# LOG

Newest entries on top. Format per ~/.claude/prompts/SOFTWARE_DEV.md.

---

[2026-06-02 14:15] | Built Go `backend/`: coordinator (HTTP 4 APIs + gRPC) + worker (gRPC consume), mutex queue, lease/visibility-timeout, checkpoint recovery, cumulative added-counter, two v1.0.0 images.
VERIFIED:
  cd backend
  make tools          # one-time: go install protoc-gen-go, protoc-gen-go-grpc, buf
  make proto          # regenerate gen/ (committed)
  go test -race ./... # queue replays PROBLEMS.md table + lease/expiry/edge cases + cumulative counter
  # E2E (two terminals):
  WORKSPACE_FOLDER=$PWD go run ./cmd/cohort --mode=coordinator           # HTTP :8080, gRPC :9090
  go run ./cmd/cohort --mode=worker --coordinator-addr=localhost:9090 --workers=3
  # HTTP (matches PROBLEMS.md): create cap10 -> add 3/13/22 -> [8,10,10,10]; take 4 -> [8,10,10,6]
  curl -s -XPOST localhost:8080/api/add  -H 'content-type: application/json' -d '{"n":22}'
  curl -s         localhost:8080/api/total
  # Recovery: SIGKILL coordinator, restart -> total + leases + totalAdded restored from checkpt/coordinator.json
  make images         # builds cohort-coordinator:v1.0.0 AND cohort-worker:v1.0.0 (distroless, 12.4MB)
  docker run -p 8080:8080 -p 9090:9090 cohort-coordinator:v1.0.0
  kubectl apply -f deploy/k8s/   # optional (kind/minikube); kubectl scale deploy/cohort-worker --replicas=5
RESULTS:
  - go build/vet/test all pass (race clean). HTTP flow reproduced PROBLEMS.md exactly.
  - 3 workers drained [8,10,10,6] (34) -> 0 via PullTask/TaskComplete; gRPC latency metrics emitted.
  - Crash recovery: after SIGKILL, restart restored total=22, cohorts=[2,10,10], totalAdded preserved.
  - Both v1.0.0 images built; coordinator container smoke-tested; k8s YAML well-formed.
K8S DEMO (minikube, 1 coordinator + 1 worker) — verified:
  minikube start --driver=docker
  minikube image load cohort-coordinator:v1.0.0 && minikube image load cohort-worker:v1.0.0
  kubectl apply -f deploy/k8s/            # worker.yaml is replicas: 1 (scale up = throughput lever)
  kubectl rollout status deploy/cohort-coordinator && kubectl rollout status deploy/cohort-worker
  # HTTP access is a NodePort now (no kubectl port-forward):
  URL=$(minikube service cohort-coordinator-http --url)
  curl -s -XPOST $URL/api/add -H 'content-type: application/json' -d '{"n":22}'   # -> [2,10,10]
  curl -s $URL/api/total                                                          # drains to 0
  RESULTS: PVC cohort-checkpt Bound (128Mi RWO); both pods Running, 0 restarts; worker drained
    22 -> 0 over gRPC PullTask/TaskComplete (~11s: idle-backoff caps at base*16=16s before a sleeping
    worker wakes — tune --idle-backoff for snappier pickup).
  SERVICE SPLIT: cohort-coordinator (ClusterIP :9090, gRPC, workers dial in-cluster) +
    cohort-coordinator-http (NodePort 30080, HTTP for frontend/host) — replaces the earlier
    ClusterIP-only svc that required `kubectl port-forward`. NOTE: the 22->0 drain above was
    proven via port-forward on the old ClusterIP svc; the NodePort split is in the manifest but
    NOT yet re-applied/verified on the cluster (apply pending).
  GOTCHA: a stale/reused minikube profile was unhealthy (provisioner + control-plane flapping) ->
    PVC stuck Pending, worker SandboxChanged/CrashLoop, and `Forbidden ... nodes/proxy` on pod logs.
    Fix = `minikube delete` + fresh `minikube start` (0-restart control plane, PVC binds instantly).
METRICS:
  serviceMetric: api.{create,add,take,total}.latency_ms, api.add.count, queue.added_total, api.take.served,
    grpc.{pulltask,taskcomplete}.latency_ms, queue.depth, queue.inflight, checkpoint.write_ms,
    worker.{pull.latency_ms,task.process_ms}.
  applicationMetric: ERROR on checkpoint IO failure (degrades, keeps serving) + gRPC/HTTP server errors;
    WARN on bad HTTP input, expired-lease reclaim, and complete-of-unknown-lease. No silent catches.
DESIGN NOTES:
  - Task = oldest cohort; PullTask leases it (visibility timeout), TaskComplete removes it; reaper
    requeues expired leases (at-least-once). Leased cohorts leave the `waiting` array.
  - cumulative `totalAdded` is lifetime (survives Create), persisted in the checkpoint, emitted as a KPI.
  - Disk IO never under the queue mutex (snapshot-then-write). One binary, --mode flag, two image tags.
  - Backend is API-only (CORS on); Express frontend stays separate (can point app.js at :8080).
NEXT:
  - Optional: kind cluster demo, gRPC mTLS, multi-coordinator HA, real DB persistence, bucket resize/migration.

---

[2026-06-02 12:40] | Scaffolded Express + TS static-demo app: 4 API endpoints + cohort-array visualization with >20 middle-collapse.
VERIFIED:
  npm install
  npm run dev            # http://localhost:3000 — click Create / Add x5 / Take / Total
  # CLI:
  curl -s -X POST localhost:3000/api/create -H 'content-type: application/json' -d '{"capacity":10}'
  curl -s -X POST localhost:3000/api/add    -H 'content-type: application/json' -d '{"n":13}'
  curl -s -X POST localhost:3000/api/take   -H 'content-type: application/json' -d '{"n":4}'
  curl -s localhost:3000/api/total
  npm run typecheck      # tsc --noEmit, exit 0
METRICS:
  serviceMetric: api.{create,add,take,total}.latency_ms emitted per request (withMetrics wrapper, src/server.ts).
  applicationMetric: WARN on bad n/capacity (400 path); ERROR + stack on any handler throw (500 path) — no silent catches.
NOTES:
  - This is the STATIC demo only: src/demo-state.ts serves a hand-authored scripted "tour"
    (grow adds-on-left to a 25-cohort peak, then drain takes-from-right). Request body `n` is
    validated but ignored — snapshots are canned. NOT the real engine.
  - Known limitation: module-level cursor is shared across requests (fine for single-user demo).
NEXT:
  - Implement the real WaitingList engine (add = top-off-then-open-left, take = drain-from-right,
    capacity enforcement) + edge cases: take 0, take > total, add 0, capacity 1, empty-cohort cleanup.
  - Wire endpoints to the engine using the real `n`; add unit tests replaying the PROBLEMS.md table.
