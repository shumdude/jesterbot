package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"jesterbot/internal/domain"
	"jesterbot/internal/service"
)

type Scheduler struct {
	logger  *slog.Logger
	service *service.Service
	router  *Router
	tick    time.Duration
}

func NewScheduler(logger *slog.Logger, tick time.Duration, svc *service.Service, router *Router) *Scheduler {
	return &Scheduler{
		logger:  logger,
		service: svc,
		router:  router,
		tick:    tick,
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	s.logger.Info("scheduler started", "tick_interval", s.tick.String())
	ticker := time.NewTicker(s.tick)
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
		s.handleMorning(ctx, user, now)
		s.handleReminder(ctx, user, now)
	}
}

func (s *Scheduler) handleMorning(ctx context.Context, user domain.User, now time.Time) {
	if localClock(now, user.UTCOffsetMinutes) != user.MorningTime {
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

	if plan.MorningSentAt == nil || !sameMinute(*plan.MorningSentAt, now) {
		return
	}

	s.logger.Info("scheduler sending morning plan", "user_id", user.ID, "chat_id", user.ChatID, "day", plan.DayLocal, "items", len(plan.Items))
	s.router.sendMessage(ctx, user.ChatID, selectionText(plan), buildPlanSelectionKeyboard(plan))
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
		s.router.sendMessage(ctx, user.ChatID, completionMessage(updatedPlan), s.router.mainMenu)
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
	s.router.sendMessage(ctx, user.ChatID, reminderText(item, updatedPlan), buildReminderKeyboard(item))
}

func localClock(now time.Time, offsetMinutes int) string {
	return now.UTC().Add(time.Duration(offsetMinutes) * time.Minute).Format("15:04")
}

func sameMinute(left, right time.Time) bool {
	return left.UTC().Format("2006-01-02T15:04") == right.UTC().Format("2006-01-02T15:04")
}

func reminderText(item *domain.DayPlanItem, plan *domain.DayPlan) string {
	pending := 0
	for _, candidate := range plan.Items {
		if candidate.Selected && !candidate.Completed {
			pending++
		}
	}

	lines := []string{
		"🔔 Напоминание.",
		fmt.Sprintf("👉 Сейчас лучше сделать: %s", item.TitleSnapshot),
		fmt.Sprintf("📌 Осталось активностей: %d", pending),
	}

	return strings.Join(lines, "\n")
}

func completionMessage(plan *domain.DayPlan) string {
	return "🎉 Все активности на сегодня закрыты. Отличная работа."
}
