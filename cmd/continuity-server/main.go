package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/neonet/codex-continuity/internal/server"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := server.LoadConfig()
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}
	store, err := server.OpenStore(cfg)
	if err != nil {
		logger.Error("open store", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go store.CleanupExpiredSessions(ctx)

	httpServer := &http.Server{
		Addr:              cfg.Address,
		Handler:           server.NewHTTPServer(cfg, store, logger),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	go func() {
		logger.Info("codex continuity server started", "address", cfg.Address, "dataDir", cfg.DataDir)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server stopped", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown", "error", err)
	}
}
