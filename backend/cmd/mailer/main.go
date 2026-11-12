package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/raulaguila/dataset/config"
	"github.com/raulaguila/dataset/internal/composition"
	"github.com/raulaguila/dataset/internal/infra"
	"github.com/raulaguila/dataset/pkg/logger"
)

func initLogger(cfg *config.Environment) *logger.Logger {
	return logger.New(logger.Config{
		Level:      cfg.Log.Level,
		Format:     cfg.Log.Format,
		TimeFormat: time.DateTime,
		Fields:     map[string]any{"version": cfg.Version, "service": "mailer"},
	})
}

func main() {
	cfg := config.MustLoad()
	log := initLogger(cfg)
	log.Info("Configuration loaded", slog.String("version", cfg.Version), slog.String("environment", cfg.Environment))

	res, err := infra.New(cfg, log)
	if err != nil {
		log.Error("Infrastructure bootstrap failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer res.Shutdown(log)
	diagnostics, stopDiagnostics, err := res.StartDiagnostics(cfg.DiagnosticsAddress, true)
	if err != nil {
		log.Error("Diagnostics failed", slog.String("error", err.Error()))
		res.Shutdown(log)
		os.Exit(1)
	}
	defer stopDiagnostics()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	worker := composition.NewMailer(cfg, res, log)
	worker.Heartbeat = diagnostics.Heartbeat
	if err := worker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("Mailer stopped with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
