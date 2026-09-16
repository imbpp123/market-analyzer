package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/imbpp123/market-analyzer/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	config, err := service.ConfigFromEnv(os.Getenv)
	if err != nil {
		logger.Error("configuration failed", "error", err)
		os.Exit(1)
	}

	app, err := service.New(config, logger)
	if err != nil {
		logger.Error("startup failed", "error", err)
		os.Exit(1)
	}
	app.Start()
	logger.Info("service started", "grpc_address", app.GRPCAddress(), "http_address", app.HTTPAddress())

	signalContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	failed := false
	select {
	case <-signalContext.Done():
	case serveErr := <-app.Errors():
		logger.Error("server failed", "error", serveErr)
		failed = true
	}
	stop()

	shutdownContext, cancel := context.WithTimeout(context.Background(), config.ShutdownTimeout)
	if err := app.Shutdown(shutdownContext); err != nil {
		logger.Error("shutdown failed", "error", err)
		failed = true
	}
	cancel()
	logger.Info("service stopped")
	if failed {
		os.Exit(1)
	}
}
