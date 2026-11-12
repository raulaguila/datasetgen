package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/raulaguila/dataset/config"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest"
	"github.com/raulaguila/dataset/internal/composition"
	"github.com/raulaguila/dataset/internal/infra"
	"github.com/raulaguila/dataset/pkg/logger"
)

func initLogger(cfg *config.Environment) *logger.Logger {
	return logger.New(logger.Config{
		Level:      cfg.Log.Level,
		Format:     cfg.Log.Format,
		TimeFormat: time.DateTime,
		Fields:     map[string]any{"version": cfg.Version, "service": "backend"},
	})
}

func main() {
	cfg := config.MustLoad()
	log := initLogger(cfg)
	log.Info("Configuration loaded", slog.Int("port", cfg.API.Port), slog.String("version", cfg.Version), slog.String("environment", cfg.Environment))

	res, err := infra.New(cfg, log)
	if err != nil {
		log.Error("Infrastructure bootstrap failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer res.Shutdown(log)
	_, stopDiagnostics, err := res.StartDiagnostics(cfg.DiagnosticsAddress, false)
	if err != nil {
		log.Error("Diagnostics failed", slog.String("error", err.Error()))
		res.Shutdown(log)
		os.Exit(1)
	}
	defer stopDiagnostics()

	fiberApp := rest.New(cfg, composition.NewApplication(cfg, res, log), composition.NewRateLimit(res), log)

	if cfg.Auth.AllowManualTokenDelivery {
		log.Warn("AUTH_MANUAL_TOKEN_DELIVERY is on: activation and reset tokens are returned in API responses. Development only.")
	}

	jobsCtx, stopJobs := context.WithCancel(context.Background())
	jobsDone := make(chan struct{})
	go func() {
		defer close(jobsDone)
		_ = composition.NewJobs(res, log).Run(jobsCtx)
	}()
	// Runs before the deferred shutdowns: a job in flight finishes its mark before the pool closes.
	defer func() { stopJobs(); <-jobsDone }()

	addr := fmt.Sprintf(":%d", cfg.API.Port)
	failed := make(chan error, 1)
	go func() {
		log.Info("HTTP server listening", slog.String("addr", addr))
		failed <- fiberApp.Listen(addr)
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-failed: // back in main, so the deferred shutdowns run before the exit code
		if err != nil {
			log.Error("HTTP server stopped", slog.String("error", err.Error()))
			stopJobs()
			<-jobsDone
			stopDiagnostics()
			res.Shutdown(log)
			os.Exit(1)
		}
		return
	case sig := <-stop:
		log.Info("Shutdown signal received", slog.String("signal", sig.String()))
	}

	if err := fiberApp.ShutdownWithTimeout(cfg.API.ShutdownTimeout); err != nil {
		log.Warn("HTTP shutdown error", slog.String("error", err.Error()))
	}
	log.Info("Backend stopped")
}
