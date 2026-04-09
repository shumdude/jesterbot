package service

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"jesterbot/internal/domain"
)

type RegistrationInput struct {
	TelegramUserID int64
	ChatID         int64
	Name           string
	UTCOffset      string
	MorningTime    string
}

type Service struct {
	repo                   Repository
	rng                    *rand.Rand
	defaultReminderMinutes int
	now                    func() time.Time
}

func New(repo Repository, defaultReminderMinutes int) *Service {
	return &Service{
		repo:                   repo,
		rng:                    rand.New(rand.NewSource(time.Now().UnixNano())),
		defaultReminderMinutes: defaultReminderMinutes,
		now:                    time.Now,
	}
}

func (s *Service) RegisterUser(ctx context.Context, input RegistrationInput) (*domain.User, error) {
	if _, err := s.repo.GetUserByTelegramID(ctx, input.TelegramUserID); err == nil {
		return nil, domain.ErrAlreadyRegistered
	} else if err != nil && err != domain.ErrNotFound {
		return nil, err
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, domain.ErrEmptyTitle
	}

	offsetMinutes, err := ParseUTCOffset(input.UTCOffset)
	if err != nil {
		return nil, err
	}

	morningTime, err := NormalizeClock(input.MorningTime)
	if err != nil {
		return nil, err
	}

	now := s.now().UTC()
	user := &domain.User{
		TelegramUserID:          input.TelegramUserID,
		ChatID:                  input.ChatID,
		Name:                    name,
		UTCOffsetMinutes:        offsetMinutes,
		MorningTime:             morningTime,
		ReminderIntervalMinutes: s.defaultReminderMinutes,
		CreatedAt:               now,
		UpdatedAt:               now,
	}

	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *Service) FindUserByTelegramID(ctx context.Context, telegramUserID int64) (*domain.User, error) {
	return s.repo.GetUserByTelegramID(ctx, telegramUserID)
}

func (s *Service) ListUsers(ctx context.Context) ([]domain.User, error) {
	return s.repo.ListUsers(ctx)
}

func (s *Service) UpdateSettings(ctx context.Context, userID int64, morningTime string, reminderIntervalMinutes int) error {
	normalizedMorningTime, err := NormalizeClock(morningTime)
	if err != nil {
		return err
	}

	if reminderIntervalMinutes <= 0 {
		return fmt.Errorf("reminder interval must be positive")
	}

	return s.repo.UpdateUserSettings(ctx, userID, normalizedMorningTime, reminderIntervalMinutes)
}

func (s *Service) AddActivity(ctx context.Context, userID int64, title string) (*domain.Activity, error) {
	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		return nil, domain.ErrEmptyTitle
	}

	activities, err := s.repo.ListActivities(ctx, userID)
	if err != nil {
		return nil, err
	}

	now := s.now().UTC()
	activity := &domain.Activity{
		UserID:    userID,
		Title:     cleanTitle,
		SortOrder: len(activities) + 1,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.repo.CreateActivity(ctx, activity); err != nil {
		return nil, err
	}

	return activity, nil
}

func (s *Service) UpdateActivity(ctx context.Context, userID, activityID int64, title string) error {
	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		return domain.ErrEmptyTitle
	}

	return s.repo.UpdateActivity(ctx, userID, activityID, cleanTitle)
}

func (s *Service) DeleteActivity(ctx context.Context, userID, activityID int64) error {
	return s.repo.DeleteActivity(ctx, userID, activityID)
}

func (s *Service) ListActivities(ctx context.Context, userID int64) ([]domain.Activity, error) {
	return s.repo.ListActivities(ctx, userID)
}

func (s *Service) StartMorningPlan(ctx context.Context, userID int64, now time.Time) (*domain.DayPlan, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// A plan is unique per user and local calendar day (based on user's UTC offset).
	// Repeated morning ticks should reuse the same plan instead of creating duplicates.
	dayLocal := localDay(now, user.UTCOffsetMinutes)
	if existing, err := s.repo.GetDayPlan(ctx, userID, dayLocal); err == nil {
		return existing, nil
	} else if err != domain.ErrNotFound {
		return nil, err
	}

	activities, err := s.repo.ListActivities(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(activities) == 0 {
		return nil, domain.ErrNoActivities
	}

	stamp := now.UTC()
	plan := &domain.DayPlan{
		UserID:        userID,
		DayLocal:      dayLocal,
		Status:        domain.PlanStatusAwaitingSelection,
		Cycle:         1,
		MorningSentAt: &stamp,
		CreatedAt:     stamp,
		UpdatedAt:     stamp,
	}

	plan.Items = make([]domain.DayPlanItem, 0, len(activities))
	for _, activity := range activities {
		plan.Items = append(plan.Items, domain.DayPlanItem{
			ActivityID:    activity.ID,
			TitleSnapshot: activity.Title,
			Selected:      true,
			Completed:     false,
			CreatedAt:     stamp,
			UpdatedAt:     stamp,
		})
	}

	if err := s.repo.SaveDayPlan(ctx, plan); err != nil {
		return nil, err
	}

	return plan, nil
}

func (s *Service) GetTodayPlan(ctx context.Context, userID int64, now time.Time) (*domain.DayPlan, error) {
	plan, _, err := s.loadTodayPlan(ctx, userID, now)
	return plan, err
}

func (s *Service) TogglePlanItem(ctx context.Context, userID, activityID int64, now time.Time) (*domain.DayPlan, error) {
	plan, _, err := s.loadTodayPlan(ctx, userID, now)
	if err != nil {
		return nil, err
	}
	if plan.Status != domain.PlanStatusAwaitingSelection {
		return nil, domain.ErrPlanNotReady
	}

	for i := range plan.Items {
		if plan.Items[i].ActivityID == activityID {
			plan.Items[i].Selected = !plan.Items[i].Selected
			plan.Items[i].UpdatedAt = now.UTC()
			plan.UpdatedAt = now.UTC()
			if err := s.repo.SaveDayPlan(ctx, plan); err != nil {
				return nil, err
			}
			return plan, nil
		}
	}

	return nil, domain.ErrNotFound
}

func (s *Service) FinalizePlan(ctx context.Context, userID int64, now time.Time) (*domain.DayPlan, error) {
	plan, user, err := s.loadTodayPlan(ctx, userID, now)
	if err != nil {
		return nil, err
	}
	if plan.Status != domain.PlanStatusAwaitingSelection {
		return nil, domain.ErrPlanNotReady
	}

	stamp := now.UTC()
	plan.SelectionFinalizedAt = &stamp
	plan.UpdatedAt = stamp

	if pendingSelectedCount(plan) == 0 {
		plan.Status = domain.PlanStatusCompleted
		plan.CompletedAt = &stamp
		plan.NextReminderAt = nil
	} else {
		plan.Status = domain.PlanStatusActive
		nextReminder := now.Add(time.Duration(user.ReminderIntervalMinutes) * time.Minute).UTC()
		plan.NextReminderAt = &nextReminder
	}

	if err := s.repo.SaveDayPlan(ctx, plan); err != nil {
		return nil, err
	}

	return plan, nil
}

func (s *Service) SelectAllAndFinalize(ctx context.Context, userID int64, now time.Time) (*domain.DayPlan, error) {
	plan, _, err := s.loadTodayPlan(ctx, userID, now)
	if err != nil {
		return nil, err
	}
	if plan.Status != domain.PlanStatusAwaitingSelection {
		return nil, domain.ErrPlanNotReady
	}

	stamp := now.UTC()
	for i := range plan.Items {
		plan.Items[i].Selected = true
		plan.Items[i].UpdatedAt = stamp
	}
	plan.UpdatedAt = stamp

	if err := s.repo.SaveDayPlan(ctx, plan); err != nil {
		return nil, err
	}

	return s.FinalizePlan(ctx, userID, now)
}

func (s *Service) PickReminder(ctx context.Context, userID int64, now time.Time) (*domain.DayPlanItem, *domain.DayPlan, error) {
	plan, user, err := s.loadTodayPlan(ctx, userID, now)
	if err != nil {
		return nil, nil, err
	}
	if plan.Status != domain.PlanStatusActive {
		return nil, nil, domain.ErrPlanNotReady
	}

	if pendingSelectedCount(plan) == 0 {
		s.completePlan(plan, now.UTC())
		if err := s.repo.SaveDayPlan(ctx, plan); err != nil {
			return nil, nil, err
		}
		return nil, plan, domain.ErrPlanClosed
	}

	if plan.NextReminderAt != nil && plan.NextReminderAt.After(now.UTC()) {
		return nil, nil, domain.ErrPlanNotReady
	}

	// Within one cycle, each selected item can be reminded at most once.
	// When every selected item has been touched in current cycle, increment cycle
	// and start a new pass over remaining not-completed items.
	candidates := reminderCandidates(plan)
	if len(candidates) == 0 {
		plan.Cycle++
		candidates = reminderCandidates(plan)
	}
	if len(candidates) == 0 {
		return nil, nil, domain.ErrPlanClosed
	}

	index := candidates[s.rng.Intn(len(candidates))]
	plan.Items[index].ReminderCycle = plan.Cycle
	plan.Items[index].UpdatedAt = now.UTC()
	nextReminder := now.Add(time.Duration(user.ReminderIntervalMinutes) * time.Minute).UTC()
	plan.NextReminderAt = &nextReminder
	plan.UpdatedAt = now.UTC()

	if err := s.repo.SaveDayPlan(ctx, plan); err != nil {
		return nil, nil, err
	}

	item := plan.Items[index]
	return &item, plan, nil
}

func (s *Service) MarkActivityDone(ctx context.Context, userID, activityID int64, now time.Time) (*domain.DayPlan, error) {
	plan, user, err := s.loadTodayPlan(ctx, userID, now)
	if err != nil {
		return nil, err
	}
	if plan.Status == domain.PlanStatusCompleted {
		return nil, domain.ErrPlanClosed
	}

	stamp := now.UTC()
	for i := range plan.Items {
		if plan.Items[i].ActivityID != activityID {
			continue
		}
		plan.Items[i].Completed = true
		plan.Items[i].CompletedAt = &stamp
		plan.Items[i].UpdatedAt = stamp
		plan.UpdatedAt = stamp

		if pendingSelectedCount(plan) == 0 {
			s.completePlan(plan, stamp)
		} else {
			nextReminder := now.Add(time.Duration(user.ReminderIntervalMinutes) * time.Minute).UTC()
			plan.NextReminderAt = &nextReminder
		}

		if err := s.repo.SaveDayPlan(ctx, plan); err != nil {
			return nil, err
		}

		return plan, nil
	}

	return nil, domain.ErrNotFound
}

func (s *Service) BuildStats(ctx context.Context, userID int64) (domain.DailyStats, error) {
	plans, err := s.repo.ListPlans(ctx, userID)
	if err != nil {
		return domain.DailyStats{}, err
	}

	stats := domain.DailyStats{}
	currentStreak := 0
	for _, plan := range plans {
		stats.DaysWithPlan++
		if plan.Status == domain.PlanStatusCompleted {
			stats.CompletedDays++
			currentStreak++
			if currentStreak > stats.CurrentCompletedStreak {
				stats.CurrentCompletedStreak = currentStreak
			}
		} else {
			currentStreak = 0
		}

		for _, item := range plan.Items {
			if item.Selected {
				stats.SelectedActivities++
			} else {
				stats.SkippedActivities++
			}
			if item.Completed {
				stats.CompletedActivities++
			}
		}
	}

	if stats.SelectedActivities > 0 {
		stats.CompletionRate = float64(stats.CompletedActivities) / float64(stats.SelectedActivities)
	}

	return stats, nil
}

func ParseUTCOffset(input string) (int, error) {
	value := strings.TrimSpace(strings.ToUpper(input))
	if value == "UTC" || value == "Z" {
		return 0, nil
	}
	if strings.HasPrefix(value, "UTC") {
		value = strings.TrimPrefix(value, "UTC")
	}
	if value == "" {
		return 0, domain.ErrInvalidUTCOffset
	}

	sign := 1
	switch value[0] {
	case '+':
		value = value[1:]
	case '-':
		sign = -1
		value = value[1:]
	default:
		return 0, domain.ErrInvalidUTCOffset
	}

	hoursPart := value
	minutesPart := "00"
	if strings.Contains(value, ":") {
		parts := strings.Split(value, ":")
		if len(parts) != 2 {
			return 0, domain.ErrInvalidUTCOffset
		}
		hoursPart = parts[0]
		minutesPart = parts[1]
	}

	hours, err := strconv.Atoi(hoursPart)
	if err != nil || hours > 14 {
		return 0, domain.ErrInvalidUTCOffset
	}
	minutes, err := strconv.Atoi(minutesPart)
	if err != nil || minutes < 0 || minutes >= 60 {
		return 0, domain.ErrInvalidUTCOffset
	}

	total := sign * ((hours * 60) + minutes)
	if total < -12*60 || total > 14*60 {
		return 0, domain.ErrInvalidUTCOffset
	}

	return total, nil
}

func NormalizeClock(input string) (string, error) {
	value := strings.TrimSpace(input)
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return "", domain.ErrInvalidClock
	}
	return parsed.Format("15:04"), nil
}

func localDay(now time.Time, utcOffsetMinutes int) string {
	// Persist day boundaries using user's local date, not server timezone.
	return now.UTC().Add(time.Duration(utcOffsetMinutes) * time.Minute).Format("2006-01-02")
}

func pendingSelectedCount(plan *domain.DayPlan) int {
	count := 0
	for _, item := range plan.Items {
		if item.Selected && !item.Completed {
			count++
		}
	}
	return count
}

func reminderCandidates(plan *domain.DayPlan) []int {
	candidates := make([]int, 0, len(plan.Items))
	for i, item := range plan.Items {
		if !item.Selected || item.Completed {
			continue
		}
		// reminder_cycle stores the last cycle in which this item was pinged.
		if item.ReminderCycle < plan.Cycle {
			candidates = append(candidates, i)
		}
	}
	return candidates
}

func (s *Service) loadTodayPlan(ctx context.Context, userID int64, now time.Time) (*domain.DayPlan, *domain.User, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, nil, err
	}

	plan, err := s.repo.GetDayPlan(ctx, userID, localDay(now, user.UTCOffsetMinutes))
	if err != nil {
		return nil, nil, err
	}

	return plan, user, nil
}

func (s *Service) completePlan(plan *domain.DayPlan, stamp time.Time) {
	plan.Status = domain.PlanStatusCompleted
	plan.CompletedAt = &stamp
	plan.NextReminderAt = nil
	plan.UpdatedAt = stamp
}
