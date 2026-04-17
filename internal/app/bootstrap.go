package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"jesterbot/internal/config"
	"jesterbot/internal/service"
	"jesterbot/internal/storage/sqlite"
	"jesterbot/internal/telegram"
)

type App struct {
	Config    config.Config
	Logger    *slog.Logger
	DB        *sql.DB
	Service   *service.Service
	Telegram  *telegram.Transport
	Scheduler *telegram.Scheduler
}

func New(logger *slog.Logger) (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	logger.Info(
		"configuration loaded",
		"db_path", cfg.DBPath,
		"poll_timeout", cfg.PollTimeout.String(),
		"worker_count", cfg.WorkerCount,
		"default_reminder_minutes", cfg.DefaultReminderMinutes,
	)

	db, err := sqlite.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	logger.Info("database opened", "db_path", cfg.DBPath)

	repo := sqlite.NewRepository(db)
	svc := service.New(repo, cfg.DefaultReminderMinutes)
	transport, err := telegram.NewTransport(logger, cfg.BotToken, cfg.PollTimeout, cfg.WorkerCount, svc)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	logger.Info("telegram transport initialized", "worker_count", cfg.WorkerCount, "poll_timeout", cfg.PollTimeout.String())

	return &App{
		Config:    cfg,
		Logger:    logger,
		DB:        db,
		Service:   svc,
		Telegram:  transport,
		Scheduler: telegram.NewScheduler(logger, svc, transport.Notifier()),
	}, nil
}

func (a *App) Start(ctx context.Context) error {
	a.Logger.Info(
		"application bootstrap completed",
		"db_path", a.Config.DBPath,
		"poll_timeout", a.Config.PollTimeout.String(),
		"worker_count", a.Config.WorkerCount,
	)
	go a.Scheduler.Start(ctx)
	return a.Telegram.Start(ctx)
}

func (a *App) Close() error {
	if a.DB == nil {
		return nil
	}
	a.Logger.Info("closing application resources")
	if err := a.DB.Close(); err != nil {
		return fmt.Errorf("close database: %w", err)
	}
	a.Logger.Info("database closed")
	return nil
}
