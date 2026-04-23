// AI-AGENT: Core daily-planning service logic for users, activities, day plans, and reminder selection.
// Entry points are Service methods such as StartMorningPlan, PickReminder, FinishDay, and UpdateSettings.
// Tightly coupled to internal/domain types, Repository, one_off.go, and telegram scheduler behavior.
// Change logical-day or quiet-hour helpers with extra care because stored plan keys and reminders depend on them.
//
package service

import (
	"context"
	"errors"
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

const defaultDayEndTime = "00:00"

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
		DayEndTime:              defaultDayEndTime,
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

func (s *Service) FinishDay(ctx context.Context, userID int64, now time.Time) (time.Time, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return time.Time{}, err
	}

	pausedUntil, err := nextMorningStartUTC(now, user.UTCOffsetMinutes, user.MorningTime)
	if err != nil {
		return time.Time{}, err
	}
	if err := s.repo.UpdateUserNotificationsPausedUntil(ctx, userID, &pausedUntil); err != nil {
		return time.Time{}, err
	}

	return pausedUntil, nil
}

func (s *Service) ListUsers(ctx context.Context) ([]domain.User, error) {
	return s.repo.ListUsers(ctx)
}

func (s *Service) UpdateSettings(ctx context.Context, userID int64, morningTime, dayEndTime string, reminderIntervalMinutes int) error {
	normalizedMorningTime, err := NormalizeClock(morningTime)
	if err != nil {
		return err
	}
	normalizedDayEndTime, err := NormalizeClock(dayEndTime)
	if err != nil {
		return err
	}

	if reminderIntervalMinutes <= 0 {
		return fmt.Errorf("reminder interval must be positive")
	}

	if err := s.repo.UpdateUserSettings(ctx, userID, normalizedMorningTime, normalizedDayEndTime, reminderIntervalMinutes); err != nil {
		return err
	}

	return s.rescheduleActivePlanReminder(ctx, userID, reminderIntervalMinutes)
}

func (s *Service) AddActivity(ctx context.Context, userID int64, title string) (*domain.Activity, error) {
	activities, err := s.AddActivities(ctx, userID, title)
	if err != nil {
		return nil, err
	}

	return &activities[0], nil
}

func (s *Service) AddActivities(ctx context.Context, userID int64, input string) ([]domain.Activity, error) {
	titles := splitBatchTitles(input)
	if len(titles) == 0 {
		return nil, domain.ErrEmptyTitle
	}

	existingActivities, err := s.repo.ListActivities(ctx, userID)
	if err != nil {
		return nil, err
	}

	now := s.now().UTC()
	created := make([]domain.Activity, 0, len(titles))
	for i, title := range titles {
		activity := domain.Activity{
			UserID:      userID,
			Title:       title,
			SortOrder:   len(existingActivities) + i + 1,
			TimesPerDay: 1,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		if err := s.repo.CreateActivity(ctx, &activity); err != nil {
			return nil, err
		}

		created = append(created, activity)
	}

	return created, nil
}

func (s *Service) UpdateActivity(ctx context.Context, userID, activityID int64, title string) error {
	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		return domain.ErrEmptyTitle
	}

	return s.repo.UpdateActivity(ctx, userID, activityID, cleanTitle)
}

func (s *Service) SetActivityTimesPerDay(ctx context.Context, userID, activityID int64, times int) error {
	if times < 1 {
		return fmt.Errorf("times per day must be at least 1")
	}
	if err := s.repo.UpdateActivityTimesPerDay(ctx, userID, activityID, times); err != nil {
		return err
	}

	return s.syncTodayPlanTimesPerDay(ctx, userID, activityID, times)
}

func (s *Service) SetActivityReminderWindows(ctx context.Context, userID, activityID int64, windows []domain.ReminderWindow) error {
	return s.repo.UpdateActivityReminderWindows(ctx, userID, activityID, windows)
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

	// A plan is unique per user and logical day, which starts at the user's morning time.
	// Repeated morning ticks should reuse the same plan instead of creating duplicates.
	dayLocal := LogicalDay(now, user.UTCOffsetMinutes, user.MorningTime)
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
		timesPerDay := activity.TimesPerDay
		if timesPerDay < 1 {
			timesPerDay = 1
		}
		plan.Items = append(plan.Items, domain.DayPlanItem{
			ActivityID:    activity.ID,
			TitleSnapshot: activity.Title,
			Selected:      true,
			Completed:     false,
			TimesPerDay:   timesPerDay,
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
	if InNotificationQuietHours(now, user.UTCOffsetMinutes, user.MorningTime, user.DayEndTime) {
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

	// Load activities to check per-activity reminder windows.
	activities, err := s.repo.ListActivities(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	actMap := make(map[int64]domain.Activity, len(activities))
	for _, a := range activities {
		actMap[a.ID] = a
	}
	clock := localClockHHMM(now, user.UTCOffsetMinutes)

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

	windowedOpenCandidates := make([]int, 0, len(candidates))
	regularCandidates := make([]int, 0, len(candidates))
	for _, idx := range candidates {
		act, ok := actMap[plan.Items[idx].ActivityID]
		if !ok {
			regularCandidates = append(regularCandidates, idx)
			continue
		}
		if hasReminderWindows(act) {
			if isInReminderWindow(act, clock) {
				windowedOpenCandidates = append(windowedOpenCandidates, idx)
			}
			continue
		}
		regularCandidates = append(regularCandidates, idx)
	}
	var eligibleCandidates []int
	if len(windowedOpenCandidates) > 0 {
		eligibleCandidates = windowedOpenCandidates
	} else if len(regularCandidates) > 0 {
		eligibleCandidates = regularCandidates
	}
	if len(eligibleCandidates) == 0 {
		// There are pending windowed activities but none are currently open, and no
		// unrestricted candidates are available to fall back to.
		return nil, nil, domain.ErrPlanNotReady
	}

	index := eligibleCandidates[s.rng.Intn(len(eligibleCandidates))]
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

func localClockHHMM(now time.Time, utcOffsetMinutes int) string {
	return now.UTC().Add(time.Duration(utcOffsetMinutes) * time.Minute).Format("15:04")
}

func hasReminderWindows(act domain.Activity) bool {
	return len(reminderWindows(act)) > 0
}

func reminderWindows(act domain.Activity) []domain.ReminderWindow {
	if len(act.ReminderWindows) > 0 {
		return act.ReminderWindows
	}
	if act.ReminderWindowStart == "" || act.ReminderWindowEnd == "" {
		return nil
	}
	return []domain.ReminderWindow{
		{Start: act.ReminderWindowStart, End: act.ReminderWindowEnd},
	}
}

// isInReminderWindow returns true if the activity has no window restriction or if
// localClock (format "HH:MM") falls inside at least one configured window.
// Supports midnight-crossing windows (e.g. Start="22:00", End="06:00").
func isInReminderWindow(act domain.Activity, localClock string) bool {
	windows := reminderWindows(act)
	if len(windows) == 0 {
		return true
	}
	for _, w := range windows {
		s, e := w.Start, w.End
		if s <= e {
			if localClock >= s && localClock < e {
				return true
			}
			continue
		}
		// Window crosses midnight.
		if localClock >= s || localClock < e {
			return true
		}
	}
	return false
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
		timesPerDay := plan.Items[i].TimesPerDay
		if timesPerDay < 1 {
			timesPerDay = 1
		}
		plan.Items[i].CompletedCount++
		plan.Items[i].UpdatedAt = stamp
		plan.UpdatedAt = stamp
		if plan.Items[i].CompletedCount >= timesPerDay {
			plan.Items[i].Completed = true
			plan.Items[i].CompletedAt = &stamp
		}

		if pendingSelectedCount(plan) == 0 {
			s.completePlan(plan, stamp)
		} else {
			if plan.NextReminderAt == nil || !plan.NextReminderAt.After(stamp) {
				nextReminder := stamp.Add(time.Duration(user.ReminderIntervalMinutes) * time.Minute)
				plan.NextReminderAt = &nextReminder
			}
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
	oneOffTasks, err := s.repo.ListOneOffTasks(ctx, userID)
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
	for _, task := range oneOffTasks {
		stats.OneOffTasks++
		if task.Status == domain.OneOffTaskStatusCompleted {
			stats.CompletedOneOffTasks++
		} else {
			stats.PendingOneOffTasks++
		}
		for _, item := range task.Items {
			stats.OneOffChecklistItems++
			if item.Completed {
				stats.CompletedOneOffChecklistItems++
			}
		}
	}
	if stats.OneOffTasks > 0 {
		stats.OneOffCompletionRate = float64(stats.CompletedOneOffTasks) / float64(stats.OneOffTasks)
	}

	return stats, nil
}

func (s *Service) TrackReminderMessage(
	ctx context.Context,
	userID, chatID int64,
	messageID int,
	dayLocal string,
	kind domain.ReminderMessageKind,
) error {
	if messageID <= 0 {
		return fmt.Errorf("message id must be positive")
	}
	if strings.TrimSpace(dayLocal) == "" {
		return fmt.Errorf("logical day must not be empty")
	}
	if kind == "" {
		return fmt.Errorf("reminder kind must not be empty")
	}

	now := s.now().UTC()
	return s.repo.SaveReminderMessage(ctx, &domain.ReminderMessage{
		UserID:     userID,
		ChatID:     chatID,
		MessageID:  messageID,
		LogicalDay: dayLocal,
		Kind:       kind,
		SentAt:     now,
	})
}

func (s *Service) ListReminderMessagesBeforeDay(ctx context.Context, userID int64, dayLocal string) ([]domain.ReminderMessage, error) {
	if strings.TrimSpace(dayLocal) == "" {
		return nil, fmt.Errorf("logical day must not be empty")
	}
	return s.repo.ListReminderMessagesBeforeDay(ctx, userID, dayLocal)
}

func (s *Service) RemoveTrackedReminderMessage(ctx context.Context, userID int64, messageID int) error {
	return s.repo.DeleteReminderMessage(ctx, userID, messageID)
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

func LogicalDay(now time.Time, utcOffsetMinutes int, morningTime string) string {
	local := now.UTC().Add(time.Duration(utcOffsetMinutes) * time.Minute)
	normalizedMorningTime, err := NormalizeClock(morningTime)
	if err != nil {
		normalizedMorningTime = "00:00"
	}
	if local.Format("15:04") < normalizedMorningTime {
		local = local.AddDate(0, 0, -1)
	}
	return local.Format("2006-01-02")
}

func InNotificationQuietHours(now time.Time, utcOffsetMinutes int, morningTime, dayEndTime string) bool {
	normalizedMorningTime, err := NormalizeClock(morningTime)
	if err != nil {
		normalizedMorningTime = "00:00"
	}
	normalizedDayEndTime, err := NormalizeClock(dayEndTime)
	if err != nil {
		normalizedDayEndTime = defaultDayEndTime
	}
	if normalizedMorningTime == normalizedDayEndTime {
		return false
	}

	clock := localClockHHMM(now, utcOffsetMinutes)
	if normalizedDayEndTime < normalizedMorningTime {
		return clock >= normalizedDayEndTime && clock < normalizedMorningTime
	}
	return clock >= normalizedDayEndTime || clock < normalizedMorningTime
}

func nextMorningStartUTC(now time.Time, utcOffsetMinutes int, morningTime string) (time.Time, error) {
	normalizedMorningTime, err := NormalizeClock(morningTime)
	if err != nil {
		normalizedMorningTime = "00:00"
	}

	morningClock, err := time.Parse("15:04", normalizedMorningTime)
	if err != nil {
		return time.Time{}, err
	}

	localNow := now.UTC().Add(time.Duration(utcOffsetMinutes) * time.Minute)
	localBoundary := time.Date(
		localNow.Year(),
		localNow.Month(),
		localNow.Day(),
		morningClock.Hour(),
		morningClock.Minute(),
		0,
		0,
		time.UTC,
	)
	if !localNow.Before(localBoundary) {
		localBoundary = localBoundary.Add(24 * time.Hour)
	}

	return localBoundary.Add(-time.Duration(utcOffsetMinutes) * time.Minute).UTC(), nil
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

	plan, err := s.repo.GetDayPlan(ctx, userID, LogicalDay(now, user.UTCOffsetMinutes, user.MorningTime))
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

func (s *Service) rescheduleActivePlanReminder(ctx context.Context, userID int64, reminderIntervalMinutes int) error {
	now := s.now().UTC()
	plan, _, err := s.loadTodayPlan(ctx, userID, now)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if plan.Status != domain.PlanStatusActive || plan.NextReminderAt == nil {
		return nil
	}

	nextReminder := now.Add(time.Duration(reminderIntervalMinutes) * time.Minute)
	plan.NextReminderAt = &nextReminder
	plan.UpdatedAt = now

	return s.repo.SaveDayPlan(ctx, plan)
}

func (s *Service) syncTodayPlanTimesPerDay(ctx context.Context, userID, activityID int64, timesPerDay int) error {
	now := s.now().UTC()
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}

	plan, err := s.repo.GetDayPlan(ctx, userID, LogicalDay(now, user.UTCOffsetMinutes, user.MorningTime))
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	updated := false
	for i := range plan.Items {
		if plan.Items[i].ActivityID != activityID {
			continue
		}

		plan.Items[i].TimesPerDay = timesPerDay
		if plan.Items[i].CompletedCount >= timesPerDay {
			plan.Items[i].Completed = true
			if plan.Items[i].CompletedAt == nil {
				completedAt := now
				plan.Items[i].CompletedAt = &completedAt
			}
		} else {
			plan.Items[i].Completed = false
			plan.Items[i].CompletedAt = nil
		}
		plan.Items[i].UpdatedAt = now
		updated = true
		break
	}
	if !updated {
		return nil
	}

	pendingSelected := pendingSelectedCount(plan)
	switch plan.Status {
	case domain.PlanStatusCompleted:
		if pendingSelected > 0 {
			plan.Status = domain.PlanStatusActive
			plan.CompletedAt = nil
			nextReminder := now.Add(time.Duration(user.ReminderIntervalMinutes) * time.Minute)
			plan.NextReminderAt = &nextReminder
		}
	case domain.PlanStatusActive:
		if pendingSelected == 0 {
			s.completePlan(plan, now)
		} else if plan.NextReminderAt == nil {
			nextReminder := now.Add(time.Duration(user.ReminderIntervalMinutes) * time.Minute)
			plan.NextReminderAt = &nextReminder
		}
	}

	plan.UpdatedAt = now
	return s.repo.SaveDayPlan(ctx, plan)
}

func splitBatchTitles(input string) []string {
	parts := strings.FieldsFunc(input, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	})

	titles := make([]string, 0, len(parts))
	for _, part := range parts {
		cleanTitle := strings.TrimSpace(part)
		if cleanTitle == "" {
			continue
		}
		titles = append(titles, cleanTitle)
	}

	return titles
}
