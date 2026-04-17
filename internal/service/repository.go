package service

import (
	"context"

	"jesterbot/internal/domain"
)

type Repository interface {
	GetUserByTelegramID(ctx context.Context, telegramUserID int64) (*domain.User, error)
	GetUserByID(ctx context.Context, userID int64) (*domain.User, error)
	ListUsers(ctx context.Context) ([]domain.User, error)
	CreateUser(ctx context.Context, user *domain.User) error
	UpdateUserSettings(ctx context.Context, userID int64, morningTime string, reminderIntervalMinutes int) error
	GetUserTickInterval(ctx context.Context, userID int64) (int, error)
	SaveUserTickInterval(ctx context.Context, userID int64, minutes int) error

	CreateActivity(ctx context.Context, activity *domain.Activity) error
	UpdateActivity(ctx context.Context, userID, activityID int64, title string) error
	UpdateActivityTimesPerDay(ctx context.Context, userID, activityID int64, timesPerDay int) error
	DeleteActivity(ctx context.Context, userID, activityID int64) error
	ListActivities(ctx context.Context, userID int64) ([]domain.Activity, error)

	GetDayPlan(ctx context.Context, userID int64, dayLocal string) (*domain.DayPlan, error)
	SaveDayPlan(ctx context.Context, plan *domain.DayPlan) error
	ListPlans(ctx context.Context, userID int64) ([]domain.DayPlan, error)

	GetOneOffReminderSettings(ctx context.Context, userID int64) (*domain.OneOffReminderSettings, error)
	SaveOneOffReminderSettings(ctx context.Context, settings *domain.OneOffReminderSettings) error
	GetOneOffTask(ctx context.Context, userID, taskID int64) (*domain.OneOffTask, error)
	ListOneOffTasks(ctx context.Context, userID int64) ([]domain.OneOffTask, error)
	SaveOneOffTask(ctx context.Context, task *domain.OneOffTask) error
	DeleteOneOffTask(ctx context.Context, userID, taskID int64) error
}
