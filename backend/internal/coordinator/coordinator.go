// Package coordinator hosts the cohort waiting list. It exposes the 4 operations
// over HTTP (for the frontend) and a gRPC consume API (for workers), backed by one
// mutex-protected queue. Every mutation is checkpointed for crash recovery, and a
// background reaper returns expired leases to the queue.
package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

	cohortv1 "github.com/elective/cohort-backend/gen/cohort/v1"
	"github.com/elective/cohort-backend/internal/checkpoint"
	"github.com/elective/cohort-backend/internal/metrics"
	"github.com/elective/cohort-backend/internal/queue"
	"google.golang.org/grpc"
)

// Config holds coordinator runtime settings.
type Config struct {
	HTTPAddr  string
	GRPCAddr  string
	LeaseTTL  time.Duration
	ReapEvery time.Duration
	Capacity  int
	Workspace string
}

// Coordinator wires the queue, checkpoint store, and gRPC service together.
type Coordinator struct {
	cohortv1.UnimplementedCoordinatorServer
	q     *queue.Queue
	store *checkpoint.Store
	ttl   time.Duration
}

type stateResponse struct {
	Capacity   int    `json:"capacity"`
	Cohorts    []int  `json:"cohorts"`
	InFlight   int    `json:"inFlight"`
	Total      int    `json:"total"`
	TotalAdded uint64 `json:"totalAdded"` // lifetime creators ever added
}

type apiError struct {
	Error string `json:"error"`
}

// Run starts the coordinator and blocks until ctx is cancelled, then shuts down
// the HTTP and gRPC servers gracefully.
func Run(ctx context.Context, cfg Config) error {
	store, err := checkpoint.New(cfg.Workspace)
	if err != nil {
		return err
	}

	q := queue.New(cfg.Capacity)
	if state, ok, lerr := store.Load(); lerr != nil {
		// Corrupt/unreadable checkpoint: start fresh rather than crash-loop.
		metrics.App(metrics.ERROR, "checkpoint load failed; starting fresh", lerr)
	} else if ok {
		q.Restore(state)
		metrics.App(metrics.INFO, "recovered from checkpoint "+store.Path(), nil)
		metrics.Service("queue.depth", q.Depth(), "cohorts")
		metrics.Service("queue.inflight", q.InFlight(), "creators")
	}

	c := &Coordinator{q: q, store: store, ttl: cfg.LeaseTTL}

	// gRPC server.
	grpcServer := grpc.NewServer()
	cohortv1.RegisterCoordinatorServer(grpcServer, c)
	grpcLis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return err
	}
	go func() {
		metrics.App(metrics.INFO, "gRPC listening on "+cfg.GRPCAddr, nil)
		if serr := grpcServer.Serve(grpcLis); serr != nil && !errors.Is(serr, grpc.ErrServerStopped) {
			metrics.App(metrics.ERROR, "gRPC server stopped", serr)
		}
	}()

	// HTTP server.
	httpServer := &http.Server{Addr: cfg.HTTPAddr, Handler: c.routes()}
	go func() {
		metrics.App(metrics.INFO, "HTTP listening on "+cfg.HTTPAddr, nil)
		if serr := httpServer.ListenAndServe(); serr != nil && !errors.Is(serr, http.ErrServerClosed) {
			metrics.App(metrics.ERROR, "HTTP server stopped", serr)
		}
	}()

	// Lease reaper.
	go c.reapLoop(ctx, cfg.ReapEvery)

	<-ctx.Done()
	metrics.App(metrics.INFO, "shutting down coordinator", nil)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	grpcServer.GracefulStop()
	return nil
}

func (c *Coordinator) reapLoop(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n := c.q.ReapExpired(time.Now()); n > 0 {
				metrics.App(metrics.WARN, "reclaimed expired lease(s) count="+itoa(n), nil)
				c.persist("reap")
			}
		}
	}
}

// persist snapshots the queue (under the queue's own lock) and writes the
// checkpoint outside any lock. Also emits queue gauges.
func (c *Coordinator) persist(op string) {
	metrics.Service("queue.depth", c.q.Depth(), "cohorts")
	metrics.Service("queue.inflight", c.q.InFlight(), "creators")
	start := time.Now()
	if err := c.store.Save(c.q.Snapshot()); err != nil {
		// Graceful degradation: keep serving from memory, surface the failure.
		metrics.App(metrics.ERROR, "checkpoint save failed after "+op, err)
		return
	}
	metrics.Latency("checkpoint.write_ms", start)
}

// ---- gRPC consume API ----

// PullTask leases the oldest available cohort to the worker.
func (c *Coordinator) PullTask(_ context.Context, req *cohortv1.PullTaskRequest) (*cohortv1.PullTaskResponse, error) {
	defer metrics.Latency("grpc.pulltask.latency_ms", time.Now())
	cohort, deadline, ok := c.q.Lease(req.GetWorkerId(), time.Now(), c.ttl)
	if !ok {
		return &cohortv1.PullTaskResponse{HasTask: false}, nil
	}
	c.persist("pull")
	return &cohortv1.PullTaskResponse{
		HasTask:           true,
		CohortId:          cohort.ID,
		Count:             int32(cohort.Count),
		LeaseDeadlineUnix: deadline.Unix(),
	}, nil
}

// TaskComplete confirms a leased cohort was consumed and removes it.
func (c *Coordinator) TaskComplete(_ context.Context, req *cohortv1.TaskCompleteRequest) (*cohortv1.TaskCompleteResponse, error) {
	defer metrics.Latency("grpc.taskcomplete.latency_ms", time.Now())
	ok := c.q.Complete(req.GetWorkerId(), req.GetCohortId())
	if ok {
		c.persist("complete")
	} else {
		metrics.App(metrics.WARN, "task complete for unknown/expired lease cohort="+req.GetCohortId(), nil)
	}
	return &cohortv1.TaskCompleteResponse{Ok: ok}, nil
}

// ---- HTTP API ----

func (c *Coordinator) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/create", c.handleCreate)
	mux.HandleFunc("/api/add", c.handleAdd)
	mux.HandleFunc("/api/take", c.handleTake)
	mux.HandleFunc("/api/total", c.handleTotal)
	mux.HandleFunc("/api/state", c.handleState)
	return withCORS(mux)
}

func (c *Coordinator) handleCreate(w http.ResponseWriter, r *http.Request) {
	defer metrics.Latency("api.create.latency_ms", time.Now())
	var body struct {
		Capacity *int `json:"capacity"`
	}
	if !decode(w, r, &body) {
		return
	}
	capacity := c.q.Capacity()
	if body.Capacity != nil {
		if *body.Capacity < 1 {
			fail(w, http.StatusBadRequest, `"capacity" must be an integer >= 1`)
			return
		}
		capacity = *body.Capacity
	}
	c.q.Create(capacity)
	c.persist("create")
	c.writeState(w)
}

func (c *Coordinator) handleAdd(w http.ResponseWriter, r *http.Request) {
	defer metrics.Latency("api.add.latency_ms", time.Now())
	n, ok := decodeCount(w, r, "n")
	if !ok {
		return
	}
	c.q.Add(n)
	// The cumulative added-count is a first-class KPI: emit how many came in this
	// call and the new lifetime total.
	metrics.Service("api.add.count", n, "creators")
	metrics.Service("queue.added_total", c.q.TotalAdded(), "creators")
	c.persist("add")
	c.writeState(w)
}

func (c *Coordinator) handleTake(w http.ResponseWriter, r *http.Request) {
	defer metrics.Latency("api.take.latency_ms", time.Now())
	n, ok := decodeCount(w, r, "n")
	if !ok {
		return
	}
	served := c.q.Take(n)
	metrics.Service("api.take.served", served, "creators")
	c.persist("take")
	c.writeState(w)
}

func (c *Coordinator) handleTotal(w http.ResponseWriter, _ *http.Request) {
	defer metrics.Latency("api.total.latency_ms", time.Now())
	writeJSON(w, http.StatusOK, map[string]int{"total": c.q.Total()})
}

// handleState is a read-only snapshot for the frontend's initial paint/refresh —
// it does NOT mutate the queue (unlike create).
func (c *Coordinator) handleState(w http.ResponseWriter, _ *http.Request) {
	defer metrics.Latency("api.state.latency_ms", time.Now())
	c.writeState(w)
}

func (c *Coordinator) writeState(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, stateResponse{
		Capacity:   c.q.Capacity(),
		Cohorts:    c.q.Counts(),
		InFlight:   c.q.InFlight(),
		Total:      c.q.Total(),
		TotalAdded: c.q.TotalAdded(),
	})
}

// ---- small HTTP helpers ----

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// decode reads an optional JSON body. An empty body is treated as {}.
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if r.Body == nil || r.ContentLength == 0 {
		return true
	}
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		metrics.App(metrics.WARN, "bad JSON body", err)
		fail(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

// decodeCount reads a non-negative integer field; missing => 0.
func decodeCount(w http.ResponseWriter, r *http.Request, field string) (int, bool) {
	body := map[string]json.Number{}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			metrics.App(metrics.WARN, "bad JSON body", err)
			fail(w, http.StatusBadRequest, "invalid JSON body")
			return 0, false
		}
	}
	raw, present := body[field]
	if !present {
		return 0, true
	}
	v, err := raw.Int64()
	if err != nil || v < 0 {
		metrics.App(metrics.WARN, "bad "+field+"="+raw.String()+" (want non-negative integer)", err)
		fail(w, http.StatusBadRequest, `"`+field+`" must be a non-negative integer`)
		return 0, false
	}
	return int(v), true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{Error: msg})
}

// itoa converts a small non-negative int to a string for log messages without
// pulling strconv into the reaper path.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
