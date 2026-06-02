package queue

import (
	"fmt"
	"testing"
	"time"
)

func counts(q *Queue) string { return fmt.Sprint(q.Counts()) }

// TestCanonicalFlow replays the exact example table from PROBLEMS.md.
func TestCanonicalFlow(t *testing.T) {
	q := New(10)

	q.Add(3)
	if got := counts(q); got != "[3]" {
		t.Fatalf("add 3: got %s want [3]", got)
	}
	q.Add(13)
	if got := counts(q); got != "[6 10]" {
		t.Fatalf("add 13: got %s want [6 10]", got)
	}
	q.Add(22)
	if got := counts(q); got != "[8 10 10 10]" {
		t.Fatalf("add 22: got %s want [8 10 10 10]", got)
	}
	if served := q.Take(4); served != 4 {
		t.Fatalf("take 4: served %d want 4", served)
	}
	if got := counts(q); got != "[8 10 10 6]" {
		t.Fatalf("take 4: got %s want [8 10 10 6]", got)
	}
	if served := q.Take(7); served != 7 {
		t.Fatalf("take 7: served %d want 7", served)
	}
	if got := counts(q); got != "[8 10 9]" {
		t.Fatalf("take 7: got %s want [8 10 9]", got)
	}
	if q.Total() != 27 {
		t.Fatalf("total after take 7: got %d want 27", q.Total())
	}
	if served := q.Take(20); served != 20 {
		t.Fatalf("take 20: served %d want 20", served)
	}
	if got := counts(q); got != "[7]" {
		t.Fatalf("take 20: got %s want [7]", got)
	}
	if q.Total() != 7 {
		t.Fatalf("total after take 20: got %d want 7", q.Total())
	}
}

func TestEdgeCases(t *testing.T) {
	t.Run("add 0 is a no-op", func(t *testing.T) {
		q := New(10)
		q.Add(0)
		if q.Total() != 0 || q.Depth() != 0 {
			t.Fatalf("add 0 changed state: %s", counts(q))
		}
	})

	t.Run("take 0 serves nothing", func(t *testing.T) {
		q := New(10)
		q.Add(5)
		if served := q.Take(0); served != 0 {
			t.Fatalf("take 0 served %d want 0", served)
		}
		if counts(q) != "[5]" {
			t.Fatalf("take 0 changed state: %s", counts(q))
		}
	})

	t.Run("take more than total serves all and empties", func(t *testing.T) {
		q := New(10)
		q.Add(15) // [5 10]
		if served := q.Take(999); served != 15 {
			t.Fatalf("take 999 served %d want 15", served)
		}
		if q.Total() != 0 || q.Depth() != 0 {
			t.Fatalf("queue not empty after over-take: %s", counts(q))
		}
	})

	t.Run("capacity 1 opens one cohort per creator", func(t *testing.T) {
		q := New(1)
		q.Add(3)
		if counts(q) != "[1 1 1]" {
			t.Fatalf("cap 1 add 3: got %s want [1 1 1]", counts(q))
		}
	})

	t.Run("emptied cohort is removed, not left at 0", func(t *testing.T) {
		q := New(10)
		q.Add(13) // [3 10]
		q.Take(10)
		if counts(q) != "[3]" {
			t.Fatalf("after draining oldest: got %s want [3]", counts(q))
		}
	})

	t.Run("negative add/take are no-ops", func(t *testing.T) {
		q := New(10)
		q.Add(-5)
		if q.Total() != 0 {
			t.Fatalf("negative add mutated state")
		}
		q.Add(5)
		if q.Take(-3) != 0 || counts(q) != "[5]" {
			t.Fatalf("negative take mutated state: %s", counts(q))
		}
	})

	t.Run("create resets everything", func(t *testing.T) {
		q := New(10)
		q.Add(50)
		q.Create(5)
		if q.Total() != 0 || q.Capacity() != 5 || q.Depth() != 0 {
			t.Fatalf("create did not reset: total=%d cap=%d depth=%d", q.Total(), q.Capacity(), q.Depth())
		}
	})
}

func TestTotalAddedIsCumulative(t *testing.T) {
	q := New(10)
	q.Add(3)
	q.Add(13)
	if q.TotalAdded() != 16 {
		t.Fatalf("after adds: got %d want 16", q.TotalAdded())
	}
	// Taking does not reduce the lifetime counter.
	q.Take(100)
	if q.TotalAdded() != 16 {
		t.Fatalf("after take: got %d want 16", q.TotalAdded())
	}
	// Create refreshes the queue but the lifetime counter persists.
	q.Create(5)
	if q.TotalAdded() != 16 {
		t.Fatalf("after create: got %d want 16 (lifetime survives)", q.TotalAdded())
	}
	q.Add(2)
	if q.TotalAdded() != 18 {
		t.Fatalf("after post-create add: got %d want 18", q.TotalAdded())
	}
	// Survives a checkpoint round-trip.
	r := New(1)
	r.Restore(q.Snapshot())
	if r.TotalAdded() != 18 {
		t.Fatalf("after restore: got %d want 18", r.TotalAdded())
	}
}

func TestLeaseLifecycle(t *testing.T) {
	now := time.Unix(1_000, 0)
	ttl := 30 * time.Second

	q := New(10)
	q.Add(22) // fresh queue: [2 10 10], oldest is a full 10

	c, deadline, ok := q.Lease("w1", now, ttl)
	if !ok || c.Count != 10 {
		t.Fatalf("lease: ok=%v count=%d want true/10", ok, c.Count)
	}
	if !deadline.Equal(now.Add(ttl)) {
		t.Fatalf("lease deadline = %v want %v", deadline, now.Add(ttl))
	}
	// Leased cohort leaves the waiting set; total drops, in-flight rises.
	if q.Total() != 12 {
		t.Fatalf("total after lease: got %d want 12", q.Total())
	}
	if q.InFlight() != 10 {
		t.Fatalf("in-flight after lease: got %d want 10", q.InFlight())
	}
	if counts(q) != "[2 10]" {
		t.Fatalf("waiting after lease: got %s want [2 10]", counts(q))
	}

	// Wrong worker cannot complete; correct worker can; second completion is a no-op.
	if q.Complete("intruder", c.ID) {
		t.Fatalf("complete by wrong worker should fail")
	}
	if !q.Complete("w1", c.ID) {
		t.Fatalf("complete by owner should succeed")
	}
	if q.Complete("w1", c.ID) {
		t.Fatalf("double complete should be a no-op false")
	}
	if q.InFlight() != 0 {
		t.Fatalf("in-flight after complete: got %d want 0", q.InFlight())
	}
}

func TestReapExpired(t *testing.T) {
	now := time.Unix(1_000, 0)
	q := New(10)
	q.Add(10) // [10]

	_, _, ok := q.Lease("w1", now, 10*time.Second)
	if !ok {
		t.Fatal("lease failed")
	}
	// Before expiry: nothing reclaimed.
	if n := q.ReapExpired(now.Add(5 * time.Second)); n != 0 {
		t.Fatalf("premature reap: got %d want 0", n)
	}
	if q.Total() != 0 {
		t.Fatalf("cohort should still be in-flight, total=%d", q.Total())
	}
	// After expiry: cohort returns to the waiting list.
	if n := q.ReapExpired(now.Add(20 * time.Second)); n != 1 {
		t.Fatalf("reap: got %d want 1", n)
	}
	if q.Total() != 10 || counts(q) != "[10]" {
		t.Fatalf("cohort not reclaimed: total=%d counts=%s", q.Total(), counts(q))
	}
}

func TestSnapshotRestoreRoundTrip(t *testing.T) {
	now := time.Unix(1_000, 0)
	q := New(10)
	q.Add(22)
	q.Lease("w1", now, time.Minute)

	snap := q.Snapshot()

	restored := New(1)
	restored.Restore(snap)

	if restored.Capacity() != 10 {
		t.Fatalf("restored capacity %d want 10", restored.Capacity())
	}
	if restored.Total() != q.Total() {
		t.Fatalf("restored total %d want %d", restored.Total(), q.Total())
	}
	if restored.InFlight() != q.InFlight() {
		t.Fatalf("restored in-flight %d want %d", restored.InFlight(), q.InFlight())
	}
	if counts(restored) != counts(q) {
		t.Fatalf("restored counts %s want %s", counts(restored), counts(q))
	}
}
