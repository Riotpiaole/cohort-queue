// Command cohort is a single binary that runs in one of two modes:
//
//	cohort --mode=coordinator   # owns the queue; serves HTTP + gRPC
//	cohort --mode=worker        # pulls and consumes cohorts over gRPC
//
// One binary, two Docker images: the k8s manifests pick the mode via args.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/elective/cohort-backend/internal/coordinator"
	"github.com/elective/cohort-backend/internal/metrics"
	"github.com/elective/cohort-backend/internal/worker"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	var (
		mode = flag.String("mode", "coordinator", "coordinator | worker")

		// coordinator flags
		httpAddr  = flag.String("http-addr", ":8080", "coordinator HTTP listen address")
		grpcAddr  = flag.String("grpc-addr", ":9090", "coordinator gRPC listen address")
		capacity  = flag.Int("capacity", 10, "default per-cohort capacity")
		leaseTTL  = flag.Duration("lease-ttl", 30*time.Second, "worker lease visibility timeout")
		reapEvery = flag.Duration("reap-every", 5*time.Second, "how often to reclaim expired leases")
		workspace = flag.String("workspace", defaultWorkspace(), "checkpoint root ({workspace}/checkpt)")

		// worker flags
		coordAddr   = flag.String("coordinator-addr", "localhost:9090", "worker: coordinator gRPC address")
		workers     = flag.Int("workers", 1, "worker: number of concurrent pull loops")
		processTime  = flag.Duration("process-time", 500*time.Millisecond, "worker: simulated per-cohort processing time")
		errorBackoff = flag.Duration("error-backoff", 1*time.Second, "worker: wait before retrying after a connection error")
	)
	flag.Parse()

	// Cancel on SIGINT/SIGTERM for graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch *mode {
	case "coordinator":
		err = coordinator.Run(ctx, coordinator.Config{
			HTTPAddr:  *httpAddr,
			GRPCAddr:  *grpcAddr,
			LeaseTTL:  *leaseTTL,
			ReapEvery: *reapEvery,
			Capacity:  *capacity,
			Workspace: *workspace,
		})
	case "worker":
		err = worker.Run(ctx, worker.Config{
			CoordinatorAddr: *coordAddr,
			Workers:         *workers,
			ProcessTime:     *processTime,
			ErrorBackoff:    *errorBackoff,
		})
	default:
		metrics.App(metrics.ERROR, "unknown --mode="+*mode+" (want coordinator|worker)", nil)
		os.Exit(2)
	}

	if err != nil {
		metrics.App(metrics.ERROR, "fatal: "+*mode+" exited with error", err)
		os.Exit(1)
	}
	metrics.App(metrics.INFO, *mode+" stopped cleanly", nil)
}

// defaultWorkspace honors WORKSPACE_FOLDER, falling back to the current directory.
func defaultWorkspace() string {
	if ws := os.Getenv("WORKSPACE_FOLDER"); ws != "" {
		return ws
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return "."
}
