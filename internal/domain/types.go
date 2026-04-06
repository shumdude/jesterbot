package domain

import "time"

type User struct {
	ID                      int64
	TelegramUserID          int64
	ChatID                  int64
	Name                    string
	UTCOffsetMinutes        int
	MorningTime             string
	ReminderIntervalMinutes int
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type Activity struct {
	ID        int64
	UserID    int64
	Title     string
	SortOrder int
	CreatedAt time.Time
	UpdatedAt time.Time
}

type PlanStatus string

const (
	PlanStatusAwaitingSelection PlanStatus = "awaiting_selection"
	PlanStatusActive            PlanStatus = "active"
	PlanStatusCompleted         PlanStatus = "completed"
)

type DayPlan struct {
	ID                   int64
	UserID               int64
	DayLocal             string
	Status               PlanStatus
	Cycle                int
	NextReminderAt       *time.Time
	MorningSentAt        *time.Time
	SelectionFinalizedAt *time.Time
	CompletedAt          *time.Time
	Items                []DayPlanItem
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type DayPlanItem struct {
	ID            int64
	PlanID        int64
	ActivityID    int64
	TitleSnapshot string
	Selected      bool
	Completed     bool
	ReminderCycle int
	CompletedAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type DailyStats struct {
	DaysWithPlan           int
	CompletedDays          int
	SelectedActivities     int
	CompletedActivities    int
	SkippedActivities      int
	CompletionRate         float64
	CurrentCompletedStreak int
}
