package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
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
	configureLogging()
	logEffectiveProxy()

	dataPath := envOr("DATA_PATH", defaultDataPath)
	addr := envOr("ADDR", defaultAddr)

	// The admin password gates all write APIs; without it the service would
	// run unlocked, so refuse to start. It never touches disk or logs.
	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminPassword == "" {
		return errors.New("ADMIN_PASSWORD environment variable is required (admin login password)")
	}

	db, err := store.Open(dataPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	// TRUST_PROXY=true tells the server to honor X-Forwarded-For when
	// resolving client IPs (rate limiting, audit). Enable only behind a
	// forwarding proxy that REPLACES (not appends to) any client-supplied
	// X-Forwarded-For header; otherwise the leftmost hop is spoofable.
	srv := server.New(db, adminPassword, server.WithTrustProxy(envOr("TRUST_PROXY", "") == "true"))
	if dist, err := fs.Sub(web.DistFS, "dist"); err == nil {
		srv.SetStaticFS(dist)
	}

	// Start the probe scheduler on the wall clock; it shares the process
	// lifecycle and is stopped during graceful shutdown below. Its prober
	// reports rounds to the server's shared alert evaluator so scheduled and
	// manual probes produce one coherent alert stream.
	schedProber := prober.New(db, hubclient.New())
	schedProber.AfterRound = srv.Alerter().HandleRound
	sched := scheduler.New(db, schedProber, scheduler.RealClock{})
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
					slog.Error("discovery sync failed", "error", err)
				}
				timer.Reset(discoveryInterval)
			}
		}
	}()

	// The rollup worker aggregates old probes into hourly rollups and prunes
	// raw probes past the retention window. Like discovery it runs on its own
	// loop: its hourly/daily cadence is unrelated to probe dispatch. It shares
	// schedCtx and stops during graceful shutdown.
	rollupWorker := scheduler.NewRollupWorker(db, scheduler.RealClock{})
	go rollupWorker.Run(schedCtx)

	// The eval worker runs the full evaluation batch (all suites x all active
	// chat models) every Sunday early morning. It executes through the
	// server's shared evaluator so judge-model resolution and score-drop
	// alerting behave identically to manual runs.
	evalWorker := scheduler.NewEvalWorker(db, srv.Evaluator(), scheduler.RealClock{})
	go evalWorker.Run(schedCtx)

	httpServer := &http.Server{
		Addr:    addr,
		Handler: srv,
	}

	// Start listening in the background.
	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", addr)
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
		slog.Info("shutting down")
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

// configureLogging installs the process-wide slog default handler. LOG_LEVEL
// (debug|info|warn|error, default info) controls verbosity.
func configureLogging() {
	level := slog.LevelInfo
	switch strings.ToLower(envOr("LOG_LEVEL", "info")) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
}

// logEffectiveProxy reports which proxy (if any) outbound hub traffic uses.
// On machines running fake-ip local proxies, direct DNS answers are
// unroutable — seeing the effective proxy at startup makes that class of
// dial failures trivial to diagnose. Credentials in the URL are masked.
func logEffectiveProxy() {
	raw := ""
	for _, key := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		if v := os.Getenv(key); v != "" {
			raw = v
			break
		}
	}
	if raw == "" {
		slog.Info("outbound proxy: none (direct)")
		return
	}
	u, err := url.Parse(raw)
	if err != nil {
		// Never log the raw value here: it may carry credentials.
		slog.Info("outbound proxy: set (unparseable URL, not logged)")
		return
	}
	u.User = nil
	slog.Info("outbound proxy", "url", u.String())
}
