package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"git.github.net/taliove2009/ai-hub-checker/internal/hubclient"
	"git.github.net/taliove2009/ai-hub-checker/internal/prober"
	"git.github.net/taliove2009/ai-hub-checker/internal/scheduler"
	"git.github.net/taliove2009/ai-hub-checker/internal/server"
	"git.github.net/taliove2009/ai-hub-checker/internal/store"
	"git.github.net/taliove2009/ai-hub-checker/web"
)

// defaultAddr is the fallback listen address.
const defaultAddr = ":8080"

// defaultDataPath is the fallback SQLite database path.
const defaultDataPath = "./data/app.db"

// shutdownTimeout bounds graceful shutdown.
const shutdownTimeout = 10 * time.Second

// discoveryInterval is how often model auto-discovery syncs with the hubs.
const discoveryInterval = time.Hour

// discoveryInitialDelay postpones the first discovery run so it does not
// stack on the probe storm that fires at startup.
const discoveryInitialDelay = 30 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

// run assembles dependencies, starts the HTTP server, and blocks until a
// termination signal triggers graceful shutdown.
func run() error {
	dataPath := envOr("DATA_PATH", defaultDataPath)
	addr := envOr("ADDR", defaultAddr)

	db, err := store.Open(dataPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	srv := server.New(db)
	if dist, err := fs.Sub(web.DistFS, "dist"); err == nil {
		srv.SetStaticFS(dist)
	}

	// Start the probe scheduler on the wall clock; it shares the process
	// lifecycle and is stopped during graceful shutdown below.
	sched := scheduler.New(db, prober.New(db, hubclient.New()), scheduler.RealClock{})
	schedCtx, schedCancel := context.WithCancel(context.Background())
	schedDone := make(chan struct{})
	go func() {
		defer close(schedDone)
		sched.Run(schedCtx)
	}()

	// Run model discovery on its own hourly loop instead of folding it into
	// the probe scheduler: discovery cadence (hourly, wall-clock) is unrelated
	// to per-endpoint probe intervals, and a separate goroutine keeps the
	// scheduler's tight dispatch loop free of slow multi-request syncs. The
	// first run is delayed so it does not pile onto the startup probe storm.
	// The loop stops with the shared schedCtx during graceful shutdown.
	go func() {
		timer := time.NewTimer(discoveryInitialDelay)
		defer timer.Stop()
		for {
			select {
			case <-schedCtx.Done():
				return
			case <-timer.C:
				if _, err := srv.Discovery().Sync(schedCtx); err != nil {
					log.Printf("discovery sync: %v", err)
				}
				timer.Reset(discoveryInterval)
			}
		}
	}()

	httpServer := &http.Server{
		Addr:    addr,
		Handler: srv,
	}

	// Start listening in the background.
	errCh := make(chan error, 1)
	go func() {
		fmt.Println("ai-hub-checker listening on", addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// Wait for a shutdown signal or a fatal server error.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		schedCancel()
		return fmt.Errorf("listen: %w", err)
	case <-stop:
		fmt.Println("shutting down...")
	}

	// Stop dispatching new probe rounds and wait for in-flight workers.
	schedCancel()
	select {
	case <-schedDone:
	case <-time.After(shutdownTimeout):
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return httpServer.Shutdown(ctx)
}

// envOr returns the environment variable value or a fallback.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
