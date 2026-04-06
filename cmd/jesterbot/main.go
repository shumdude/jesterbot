package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"jesterbot/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger.Info("starting jesterbot")

	application, err := app.New(logger)
	if err != nil {
		logger.Error("startup failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		if closeErr := application.Close(); closeErr != nil {
			logger.Error("shutdown failed", "error", closeErr)
		}
	}()

	if err := application.Start(ctx); err != nil {
		logger.Error("application stopped with error", "error", err)
		os.Exit(1)
	}

	logger.Info("application stopped gracefully")
}
