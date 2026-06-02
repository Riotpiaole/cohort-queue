package coordinator

import (
	"context"
	"sync"
	"testing"
	"time"

	cohortv1 "github.com/elective/cohort-backend/gen/cohort/v1"
	"github.com/elective/cohort-backend/internal/checkpoint"
	"github.com/elective/cohort-backend/internal/queue"
)

func newTestCoordinator(t *testing.T) *Coordinator {
	t.Helper()
	store, err := checkpoint.New(t.TempDir())
	if err != nil {
		t.Fatalf("checkpoint store: %v", err)
	}
	c := &Coordinator{q: queue.New(10), store: store, ttl: time.Minute, maxBuffer: defaultMaxBuffer}
	c.cond = sync.NewCond(&c.mu)
	return c
}

var pullReq = &cohortv1.PullTaskRequest{WorkerId: "w1"}

// Without a buffered pull, PullTask blocks and only returns (has_task=false) when
// the context is cancelled — i.e. the worker waits instead of draining.
func TestPullTaskBlocksUntilRequested(t *testing.T) {
	c := newTestCoordinator(t)
	c.q.Add(10) // a cohort exists, but no pull was requested

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	resp, err := c.PullTask(ctx, pullReq)
	if err != nil {
		t.Fatalf("PullTask err: %v", err)
	}
	if resp.GetHasTask() {
		t.Fatal("PullTask handed out a cohort with no pull requested")
	}
	if time.Since(start) < 100*time.Millisecond {
		t.Fatal("PullTask returned early — it should have blocked until ctx expired")
	}
}

// One buffered pull yields exactly one (oldest) cohort, and the buffer drains.
func TestPullConsumesOneCohort(t *testing.T) {
	c := newTestCoordinator(t)
	c.q.Add(22) // [2,10,10]; oldest is a full 10

	if depth, ok := c.enqueuePull(); !ok || depth != 1 {
		t.Fatalf("enqueuePull: depth=%d ok=%v", depth, ok)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	resp, err := c.PullTask(ctx, pullReq)
	if err != nil {
		t.Fatalf("PullTask err: %v", err)
	}
	if !resp.GetHasTask() || resp.GetCount() != 10 {
		t.Fatalf("PullTask: hasTask=%v count=%d want true/10", resp.GetHasTask(), resp.GetCount())
	}
	if c.pendingPulls != 0 {
		t.Fatalf("pendingPulls=%d want 0 after consume", c.pendingPulls)
	}
}

// A pull requested while the queue is empty stays buffered; a later Add unblocks
// the waiting worker.
func TestPullBufferedBeforeCohort(t *testing.T) {
	c := newTestCoordinator(t)
	c.enqueuePull() // requested while empty

	done := make(chan *cohortv1.PullTaskResponse, 1)
	go func() {
		resp, _ := c.PullTask(context.Background(), pullReq)
		done <- resp
	}()

	// Let the worker reach its wait, then add a cohort.
	time.Sleep(50 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("PullTask returned before any cohort was added")
	default:
	}

	c.q.Add(5)
	c.wakeAll()

	select {
	case resp := <-done:
		if !resp.GetHasTask() || resp.GetCount() != 5 {
			t.Fatalf("after Add: hasTask=%v count=%d want true/5", resp.GetHasTask(), resp.GetCount())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("PullTask did not unblock after Add")
	}
}

// The pull buffer is persisted in the checkpoint and restored on reload.
func TestPendingPullsCheckpointed(t *testing.T) {
	c := newTestCoordinator(t)
	c.enqueuePull()
	c.enqueuePull()
	c.enqueuePull() // pendingPulls = 3
	c.persist("test")

	state, ok, err := c.store.Load()
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if state.PendingPulls != 3 {
		t.Fatalf("checkpoint PendingPulls=%d want 3", state.PendingPulls)
	}
}
