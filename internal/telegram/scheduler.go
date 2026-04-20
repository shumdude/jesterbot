package telegram

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"jesterbot/internal/domain"
	"jesterbot/internal/service"
)

const schedulerTick = time.Minute
const reminderCleanupDeleteDelay = 120 * time.Millisecond

type Scheduler struct {
	logger      *slog.Logger
	service     *service.Service
	notifier    Notifier
	lastUserRun map[int64]time.Time
}

func NewScheduler(logger *slog.Logger, svc *service.Service, notifier Notifier) *Scheduler {
	return &Scheduler{
		logger:      logger,
		service:     svc,
		notifier:    notifier,
		lastUserRun: make(map[int64]time.Time),
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	// The scheduler runs on a fixed internal minute cadence because all
	// user-visible timing knobs are already minute-based user settings.
	s.logger.Info("scheduler started", "tick_interval", schedulerTick.String())
	ticker := time.NewTicker(schedulerTick)
	defer func() {
		ticker.Stop()
		s.logger.Info("scheduler stopped")
	}()

	s.runTick(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runTick(ctx)
		}
	}
}

func (s *Scheduler) runTick(ctx context.Context) {
	users, err := s.service.ListUsers(ctx)
	if err != nil {
		s.logger.Error("scheduler list users failed", "error", err)
		return
	}

	now := time.Now().UTC()
	for _, user := range users {
		tickIntervalMinutes, err := s.service.GetUserTickInterval(ctx, user.ID)
		if err != nil {
			s.logger.Error("scheduler get user tick interval failed", "error", err, "user_id", user.ID)
			continue
		}
		if !s.shouldProcessUser(user.ID, now, tickIntervalMinutes) {
			continue
		}

		currentDayLocal := logicalDayForUser(now, user.UTCOffsetMinutes, user.DayEndTime)
		s.cleanupPreviousReminderMessages(ctx, user, currentDayLocal)
		if notificationsPaused(user, now) {
			continue
		}

		s.handleMorning(ctx, user, now)
		s.handleReminder(ctx, user, now)
		s.handleOneOffReminder(ctx, user, now, currentDayLocal)
	}
}

func (s *Scheduler) cleanupPreviousReminderMessages(ctx context.Context, user domain.User, currentDayLocal string) {
	messages, err := s.service.ListReminderMessagesBeforeDay(ctx, user.ID, currentDayLocal)
	if err != nil {
		s.logger.Error("scheduler list stale reminder messages failed", "error", err, "user_id", user.ID, "day", currentDayLocal)
		return
	}
	if len(messages) == 0 {
		return
	}

	result := s.notifier.CleanupReminderMessages(ctx, user.ID, messages, reminderCleanupDeleteDelay)
	s.logger.Info(
		"scheduler cleanup stale reminder messages",
		"user_id", user.ID,
		"chat_id", user.ChatID,
		"current_day", currentDayLocal,
		"attempted", result.Attempted,
		"deleted", result.Deleted,
		"failed", result.Failed,
	)
}

func (s *Scheduler) handleMorning(ctx context.Context, user domain.User, now time.Time) {
	if localClock(now, user.UTCOffsetMinutes) < user.MorningTime {
		return
	}

	plan, err := s.service.StartMorningPlan(ctx, user.ID, now)
	if errors.Is(err, domain.ErrNoActivities) || errors.Is(err, domain.ErrPlanNotReady) {
		return
	}
	if errors.Is(err, domain.ErrNotFound) {
		return
	}
	if err != nil {
		if !errors.Is(err, domain.ErrAlreadyRegistered) {
			s.logger.Error("scheduler start morning plan failed", "error", err, "user_id", user.ID)
		}
		return
	}

	// Only send the morning prompt on the exact minute when the plan was created
	// to avoid duplicate sends during the same minute across repeated ticks.
	if plan.MorningSentAt == nil || !sameMinute(*plan.MorningSentAt, now) {
		return
	}

	s.logger.Info("scheduler sending morning plan", "user_id", user.ID, "chat_id", user.ChatID, "day", plan.DayLocal, "items", len(plan.Items))
	s.notifier.ShowMorningPlan(ctx, user.ChatID, plan)
}

func (s *Scheduler) handleReminder(ctx context.Context, user domain.User, now time.Time) {
	plan, err := s.service.GetTodayPlan(ctx, user.ID, now)
	if errors.Is(err, domain.ErrNotFound) {
		return
	}
	if err != nil {
		s.logger.Error("scheduler get plan failed", "error", err, "user_id", user.ID)
		return
	}
	if plan.Status != domain.PlanStatusActive || plan.NextReminderAt == nil || plan.NextReminderAt.After(now) {
		return
	}

	item, updatedPlan, err := s.service.PickReminder(ctx, user.ID, now)
	if errors.Is(err, domain.ErrPlanNotReady) {
		return
	}
	if errors.Is(err, domain.ErrPlanClosed) {
		s.logger.Info("scheduler detected completed plan", "user_id", user.ID, "chat_id", user.ChatID, "day", plan.DayLocal)
		s.notifier.ShowPlanCompletion(ctx, user.ChatID, user.TelegramUserID, updatedPlan)
		return
	}
	if err != nil {
		s.logger.Error("scheduler pick reminder failed", "error", err, "user_id", user.ID)
		return
	}

	s.logger.Info(
		"scheduler sending reminder",
		"user_id", user.ID,
		"chat_id", user.ChatID,
		"day", updatedPlan.DayLocal,
		"activity_id", item.ActivityID,
		"activity_title", item.TitleSnapshot,
		"cycle", updatedPlan.Cycle,
	)
	s.notifier.SendReminder(ctx, user.ID, user.ChatID, item, updatedPlan)
}

func (s *Scheduler) handleOneOffReminder(ctx context.Context, user domain.User, now time.Time, currentDayLocal string) {
	task, err := s.service.PickOneOffReminder(ctx, user.ID, now)
	if errors.Is(err, domain.ErrNotFound) {
		return
	}
	if err != nil {
		s.logger.Error("scheduler pick one-off reminder failed", "error", err, "user_id", user.ID)
		return
	}

	s.logger.Info(
		"scheduler sending one-off reminder",
		"user_id", user.ID,
		"chat_id", user.ChatID,
		"task_id", task.ID,
		"task_title", task.Title,
		"priority", task.Priority,
	)
	s.notifier.SendOneOffReminder(ctx, user.ID, user.ChatID, currentDayLocal, task)
}

func localClock(now time.Time, offsetMinutes int) string {
	return now.UTC().Add(time.Duration(offsetMinutes) * time.Minute).Format("15:04")
}

func sameMinute(left, right time.Time) bool {
	return left.UTC().Format("2006-01-02T15:04") == right.UTC().Format("2006-01-02T15:04")
}

func notificationsPaused(user domain.User, now time.Time) bool {
	if user.NotificationsPausedUntil == nil {
		return false
	}
	return user.NotificationsPausedUntil.After(now.UTC())
}

func logicalDayForUser(now time.Time, utcOffsetMinutes int, dayEndTime string) string {
	local := now.UTC().Add(time.Duration(utcOffsetMinutes) * time.Minute)
	normalizedDayEndTime, err := service.NormalizeClock(dayEndTime)
	if err != nil {
		normalizedDayEndTime = "00:00"
	}
	if local.Format("15:04") < normalizedDayEndTime {
		local = local.AddDate(0, 0, -1)
	}
	return local.Format("2006-01-02")
}

func (s *Scheduler) shouldProcessUser(userID int64, now time.Time, tickIntervalMinutes int) bool {
	lastRun, ok := s.lastUserRun[userID]
	if ok && now.Sub(lastRun) < time.Duration(tickIntervalMinutes)*time.Minute {
		return false
	}

	s.lastUserRun[userID] = now
	return true
}

func reminderText(item *domain.DayPlanItem, _ *domain.DayPlan) string {
	lines := []string{
		tr("reminder_text_title"),
		tr("reminder_text_now", item.TitleSnapshot),
	}

	return strings.Join(lines, "\n")
}

func completionMessage(plan *domain.DayPlan) string {
	return tr("completion_message")
}
