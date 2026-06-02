// Package worker is the consume side: a gRPC client that runs N concurrent loops,
// each pulling the oldest cohort, "processing" it, and reporting completion. This
// models the README's ops workers — scale N to trade latency for throughput.
package worker

import (
	"context"
	"time"

	cohortv1 "github.com/elective/cohort-backend/gen/cohort/v1"
	"github.com/elective/cohort-backend/internal/metrics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Config holds worker runtime settings.
type Config struct {
	CoordinatorAddr string
	Workers         int           // number of concurrent pull loops
	ProcessTime     time.Duration // simulated per-cohort processing time
	ErrorBackoff    time.Duration // wait before retrying after a connection error
}

// Run dials the coordinator and runs Config.Workers pull loops until ctx is done.
func Run(ctx context.Context, cfg Config) error {
	conn, err := grpc.NewClient(cfg.CoordinatorAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	client := cohortv1.NewCoordinatorClient(conn)

	metrics.App(metrics.INFO, "worker connected to "+cfg.CoordinatorAddr, nil)

	done := make(chan struct{})
	for i := 0; i < cfg.Workers; i++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			loop(ctx, client, cfg, "w"+itoa(id))
		}(i)
	}

	// Wait for every loop to exit on ctx cancellation.
	for i := 0; i < cfg.Workers; i++ {
		<-done
	}
	return nil
}

// loop is one worker's pull -> process -> complete cycle. PullTask blocks
// server-side until a user requests a pull AND a cohort exists, so the worker
// waits idly instead of busy-polling (no idle backoff). A short backoff is used
// only to recover from connection errors.
func loop(ctx context.Context, client cohortv1.CoordinatorClient, cfg Config, workerID string) {
	for {
		if ctx.Err() != nil {
			return
		}

		// Blocking long-poll: returns only when there's work, or on shutdown/disconnect.
		resp, err := client.PullTask(ctx, &cohortv1.PullTaskRequest{WorkerId: workerID})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			metrics.App(metrics.ERROR, workerID+" PullTask failed", err)
			if !sleep(ctx, cfg.ErrorBackoff) { // reconnect backoff only
				return
			}
			continue
		}

		if !resp.GetHasTask() {
			// has_task=false only happens on shutdown/disconnect — exit if so.
			if ctx.Err() != nil {
				return
			}
			continue
		}

		// Simulate consuming the cohort.
		processStart := time.Now()
		if !sleep(ctx, cfg.ProcessTime) {
			// Shutting down mid-process: don't ack, let the lease expire and requeue.
			return
		}
		metrics.Latency("worker.task.process_ms", processStart)
		metrics.App(metrics.INFO, workerID+" consumed cohort "+resp.GetCohortId()+" count="+itoa(int(resp.GetCount())), nil)

		ack, err := client.TaskComplete(ctx, &cohortv1.TaskCompleteRequest{
			WorkerId: workerID,
			CohortId: resp.GetCohortId(),
		})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			// Lease will expire and requeue — at-least-once.
			metrics.App(metrics.ERROR, workerID+" TaskComplete failed for "+resp.GetCohortId(), err)
			continue
		}
		if !ack.GetOk() {
			metrics.App(metrics.WARN, workerID+" lease lost before complete (expired?) cohort="+resp.GetCohortId(), nil)
		}
	}
}

// sleep waits for d or until ctx is cancelled. Returns false if cancelled.
func sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

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
