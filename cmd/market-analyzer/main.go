package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/imbpp123/market-analyzer/internal/service"
)

func main() {
	healthcheck, err := healthcheckRequested(os.Args[1:])
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if healthcheck {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		client := &http.Client{Transport: &http.Transport{Proxy: nil}}
		err := checkHealth(ctx, os.Getenv, client)
		cancel()
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

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
