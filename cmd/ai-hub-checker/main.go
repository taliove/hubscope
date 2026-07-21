package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
		return fmt.Errorf("listen: %w", err)
	case <-stop:
		fmt.Println("shutting down...")
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
