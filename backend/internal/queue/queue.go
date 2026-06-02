// Package queue implements the cohort waiting list: an in-memory, mutex-protected
// FIFO of cohorts plus a set of in-flight leases (SQS-style visibility timeout).
//
// Ordering: cohorts[0] is the NEWEST (left); cohorts[len-1] is the OLDEST (right).
// Creators are added on the left and served/consumed from the right (FIFO).
//
// A "task" is the oldest available cohort. Lease moves it out of the waiting slice
// into `leases`; Complete removes it for good; ReapExpired returns timed-out leases
// to the waiting slice. Disk IO must NEVER be done while holding the mutex — callers
// take a Snapshot (a deep copy) and persist it outside the lock.
package queue

import (
	"fmt"
	"sync"
	"time"
)

// Cohort is a bucket of creators. Invariant: 1 <= Count <= capacity. A cohort is
// removed once fully drained — it never lingers at 0.
type Cohort struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

// Lease is an in-flight cohort held by a worker until Deadline.
type Lease struct {
	Cohort   Cohort    `json:"cohort"`
	WorkerID string    `json:"workerId"`
	Deadline time.Time `json:"deadline"`
}

// State is the serializable snapshot used for checkpointing.
type State struct {
	Capacity   int      `json:"capacity"`
	Seq        uint64   `json:"seq"`
	Cohorts    []Cohort `json:"cohorts"`
	Leases     []Lease  `json:"leases"`
	TotalAdded uint64   `json:"totalAdded"` // lifetime creators ever added
	// PendingPulls is the coordinator's pull-request buffer depth. The queue does
	// not manage it, but it lives here because State is the on-disk checkpoint
	// format and the buffer must survive a coordinator restart.
	PendingPulls int `json:"pendingPulls"`
}

// Queue is the cohort waiting list. All methods are safe for concurrent use.
type Queue struct {
	mu         sync.Mutex
	capacity   int
	cohorts    []Cohort
	leases     map[string]Lease
	seq        uint64
	totalAdded uint64 // lifetime counter of creators ever added (survives Create)
}

// New returns an empty queue with the given per-cohort capacity (clamped to >= 1).
func New(capacity int) *Queue {
	return &Queue{capacity: max(capacity, 1), leases: map[string]Lease{}}
}

func (q *Queue) nextID() string {
	q.seq++
	return fmt.Sprintf("c%d", q.seq)
}

// Capacity returns the current per-cohort capacity.
func (q *Queue) Capacity() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.capacity
}

// Create resets the waiting list to empty with a new capacity (clamped to >= 1).
// Matches the spec's "always refresh a new queue when New is called". In-flight
// leases are also cleared.
func (q *Queue) Create(capacity int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.capacity = max(capacity, 1)
	q.cohorts = nil
	q.leases = map[string]Lease{}
}

// Add inserts n creators: top off the newest (left) cohort to capacity, then open
// new cohorts on the left for the remainder. No cohort ever exceeds capacity.
// n <= 0 is a no-op.
func (q *Queue) Add(n int) {
	if n <= 0 {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()

	// Every creator in this call is placed (cohort count is capped, cohort count
	// is not), so the whole n contributes to the lifetime counter.
	q.totalAdded += uint64(n)

	// Top off the current newest cohort first.
	if len(q.cohorts) > 0 && q.cohorts[0].Count < q.capacity {
		fill := min(q.capacity-q.cohorts[0].Count, n)
		q.cohorts[0].Count += fill
		n -= fill
	}

	// Open new cohorts for the remainder. Build oldest-of-new first (full chunks),
	// so the partial leftover lands as the newest (leftmost) cohort.
	if n > 0 {
		var group []Cohort
		for n > 0 {
			chunk := min(q.capacity, n)
			group = append(group, Cohort{ID: q.nextID(), Count: chunk})
			n -= chunk
		}
		// group is [full, full, ..., partial(newest)] — reverse so newest is first.
		for i, j := 0, len(group)-1; i < j; i, j = i+1, j-1 {
			group[i], group[j] = group[j], group[i]
		}
		q.cohorts = append(group, q.cohorts...)
	}
}

// Take serves up to n creators from the oldest (right) cohorts, removing emptied
// cohorts. Returns the number actually served. n <= 0 serves 0; n greater than the
// total serves everything available. Leased cohorts are untouched (not in the slice).
func (q *Queue) Take(n int) int {
	if n <= 0 {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()

	served := 0
	for n > 0 && len(q.cohorts) > 0 {
		last := len(q.cohorts) - 1
		if q.cohorts[last].Count <= n {
			served += q.cohorts[last].Count
			n -= q.cohorts[last].Count
			q.cohorts = q.cohorts[:last]
		} else {
			q.cohorts[last].Count -= n
			served += n
			n = 0
		}
	}
	return served
}

// Total returns the number of creators currently waiting (excludes in-flight leases).
func (q *Queue) Total() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.totalLocked()
}

func (q *Queue) totalLocked() int {
	sum := 0
	for _, c := range q.cohorts {
		sum += c.Count
	}
	return sum
}

// TotalAdded returns the lifetime number of creators ever added via Add. It is
// cumulative and intentionally survives Create (it counts everything ever enqueued,
// not the current waiting count).
func (q *Queue) TotalAdded() uint64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.totalAdded
}

// InFlight returns the number of creators currently leased to workers.
func (q *Queue) InFlight() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	sum := 0
	for _, l := range q.leases {
		sum += l.Cohort.Count
	}
	return sum
}

// Lease moves the oldest available cohort to the worker as an in-flight lease that
// expires at now+ttl. Returns ok=false when the waiting list is empty.
func (q *Queue) Lease(workerID string, now time.Time, ttl time.Duration) (Cohort, time.Time, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.cohorts) == 0 {
		return Cohort{}, time.Time{}, false
	}
	last := len(q.cohorts) - 1
	c := q.cohorts[last]
	q.cohorts = q.cohorts[:last]
	deadline := now.Add(ttl)
	q.leases[c.ID] = Lease{Cohort: c, WorkerID: workerID, Deadline: deadline}
	return c, deadline, true
}

// Complete confirms a lease and removes the cohort permanently. Idempotent: returns
// false if the lease is unknown/expired or owned by a different worker.
func (q *Queue) Complete(workerID, cohortID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	l, ok := q.leases[cohortID]
	if !ok || l.WorkerID != workerID {
		return false
	}
	delete(q.leases, cohortID)
	return true
}

// ReapExpired returns leases whose deadline has passed to the waiting list (at the
// oldest end, preserving rough FIFO). Returns the number of cohorts reclaimed.
func (q *Queue) ReapExpired(now time.Time) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	reclaimed := 0
	for id, l := range q.leases {
		if now.After(l.Deadline) {
			delete(q.leases, id)
			q.cohorts = append(q.cohorts, l.Cohort) // append = oldest end
			reclaimed++
		}
	}
	return reclaimed
}

// Depth returns the number of waiting cohorts (not creators).
func (q *Queue) Depth() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.cohorts)
}

// Counts returns just the waiting cohort creator-counts, newest first — the shape
// the frontend renders.
func (q *Queue) Counts() []int {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]int, len(q.cohorts))
	for i, c := range q.cohorts {
		out[i] = c.Count
	}
	return out
}

// Snapshot returns a deep copy of the full state for checkpointing.
func (q *Queue) Snapshot() State {
	q.mu.Lock()
	defer q.mu.Unlock()
	cohorts := append([]Cohort(nil), q.cohorts...)
	leases := make([]Lease, 0, len(q.leases))
	for _, l := range q.leases {
		leases = append(leases, l)
	}
	return State{Capacity: q.capacity, Seq: q.seq, Cohorts: cohorts, Leases: leases, TotalAdded: q.totalAdded}
}

// Restore replaces the queue's contents from a checkpointed State.
func (q *Queue) Restore(s State) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.capacity = max(s.Capacity, 1)
	q.seq = s.Seq
	q.totalAdded = s.TotalAdded
	q.cohorts = append([]Cohort(nil), s.Cohorts...)
	q.leases = make(map[string]Lease, len(s.Leases))
	for _, l := range s.Leases {
		q.leases[l.Cohort.ID] = l
	}
}
