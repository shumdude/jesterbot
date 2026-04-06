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
	Telegram  *telegram.Router
	Scheduler *telegram.Scheduler
}

func New(logger *slog.Logger) (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	db, err := sqlite.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}

	repo := sqlite.NewRepository(db)
	svc := service.New(repo, cfg.DefaultReminderMinutes)
	router, err := telegram.NewRouter(logger, cfg.BotToken, cfg.PollTimeout, cfg.WorkerCount, svc)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	return &App{
		Config:    cfg,
		Logger:    logger,
		DB:        db,
		Service:   svc,
		Telegram:  router,
		Scheduler: telegram.NewScheduler(logger, cfg.TickInterval, svc, router),
	}, nil
}

func (a *App) Start(ctx context.Context) error {
	a.Logger.Info("application bootstrap completed")
	go a.Scheduler.Start(ctx)
	return a.Telegram.Start(ctx)
}

func (a *App) Close() error {
	if a.DB == nil {
		return nil
	}
	if err := a.DB.Close(); err != nil {
		return fmt.Errorf("close database: %w", err)
	}
	return nil
}
