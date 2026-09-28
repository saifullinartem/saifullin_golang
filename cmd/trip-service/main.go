package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/saifullinartem/saifullin_golang/internal/config"
	"github.com/saifullinartem/saifullin_golang/internal/database"
	"github.com/saifullinartem/saifullin_golang/internal/httpserver"
	"github.com/saifullinartem/saifullin_golang/internal/trip"
)

func main() {
	if err := run(); err != nil {
		slog.Error("trip service stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	var level slog.Level
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	pool, err := database.Open(context.Background(), cfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	repository := trip.NewRepository(pool, cfg.DatabaseQueryTimeout)
	history := trip.NewHistoryRepository(pool, cfg.DatabaseQueryTimeout)
	tx := database.NewTxManager(pool, cfg.DatabaseQueryTimeout)
	service := trip.NewService(repository, history, tx)
	server := httpserver.New(cfg, pool, cfg.DatabaseQueryTimeout, service)
	serverErrors := make(chan error, 1)
	go func() {
		slog.Info("HTTP server listening", "address", cfg.HTTPAddr)
		serverErrors <- server.ListenAndServe()
	}()

	signals, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-signals.Done():
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		closeErr := server.Close()
		return errors.Join(fmt.Errorf("HTTP shutdown did not complete within SHUTDOWN_TIMEOUT: %w", err), closeErr)
	}
	return nil
}
