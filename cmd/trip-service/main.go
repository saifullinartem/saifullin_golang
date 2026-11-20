package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/saifullinartem/saifullin_golang/internal/config"
	"github.com/saifullinartem/saifullin_golang/internal/container"
)

func main() {
	if err := run(); err != nil {
		container.LogError(context.Background(), err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	app, err := container.New(context.Background(), cfg)
	if err != nil {
		return err
	}
	server := app.Server
	serverErrors := make(chan error, 1)
	go func() {
		app.Logger.Info("HTTP server listening", "address", cfg.HTTPAddr)
		serverErrors <- server.ListenAndServe()
	}()

	signals, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var serveErr error
	select {
	case serveErr = <-serverErrors:
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
	case <-signals.Done():
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	shutdownErr := server.Shutdown(ctx)
	if shutdownErr != nil {
		shutdownErr = errors.Join(fmt.Errorf("HTTP shutdown did not complete within SHUTDOWN_TIMEOUT: %w", shutdownErr), server.Close())
	}
	closeErr := closeWithinContext(ctx, app)
	if err := errors.Join(serveErr, shutdownErr, closeErr); err != nil {
		return err
	}
	app.Logger.Info("service stopped")
	return nil
}
