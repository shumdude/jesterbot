package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"jesterbot/internal/domain"
)

func TestRepositoryUserActivityAndPlanRoundTrip(t *testing.T) {
	db, repo := newTestRepository(t)
	defer db.Close()

	ctx := context.Background()
	now := time.Date(2026, 4, 6, 6, 0, 0, 0, time.UTC)

	user := &domain.User{
		TelegramUserID:          101,
		ChatID:                  202,
		Name:                    "Alice",
		UTCOffsetMinutes:        180,
		MorningTime:             "08:00",
		DayEndTime:              "00:00",
		ReminderIntervalMinutes: 30,
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	if err := repo.CreateUser(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	activity := &domain.Activity{
		UserID:    user.ID,
		Title:     "Stretch",
		SortOrder: 1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateActivity(ctx, activity); err != nil {
		t.Fatalf("create activity: %v", err)
	}

	plan := &domain.DayPlan{
		UserID:         user.ID,
		DayLocal:       "2026-04-06",
		Status:         domain.PlanStatusActive,
		Cycle:          1,
		NextReminderAt: ptrTime(now.Add(30 * time.Minute)),
		MorningSentAt:  ptrTime(now),
		CreatedAt:      now,
		UpdatedAt:      now,
		Items: []domain.DayPlanItem{
			{
				ActivityID:    activity.ID,
				TitleSnapshot: activity.Title,
				Selected:      true,
				Completed:     false,
				CreatedAt:     now,
				UpdatedAt:     now,
			},
		},
	}
	if err := repo.SaveDayPlan(ctx, plan); err != nil {
		t.Fatalf("save day plan: %v", err)
	}

	loadedUser, err := repo.GetUserByTelegramID(ctx, user.TelegramUserID)
	if err != nil {
		t.Fatalf("get user by telegram id: %v", err)
	}
	if loadedUser.Name != user.Name {
		t.Fatalf("unexpected loaded user: %+v", loadedUser)
	}

	loadedPlan, err := repo.GetDayPlan(ctx, user.ID, "2026-04-06")
	if err != nil {
		t.Fatalf("get day plan: %v", err)
	}
	if loadedPlan.Status != domain.PlanStatusActive || len(loadedPlan.Items) != 1 {
		t.Fatalf("unexpected loaded plan: %+v", loadedPlan)
	}
	if loadedPlan.Items[0].ActivityID != activity.ID {
		t.Fatalf("unexpected loaded item: %+v", loadedPlan.Items[0])
	}
}

func TestRepositoryListPlansSortedAndUpdateSettings(t *testing.T) {
	db, repo := newTestRepository(t)
	defer db.Close()

	ctx := context.Background()
	now := time.Date(2026, 4, 6, 6, 0, 0, 0, time.UTC)

	user := &domain.User{
		TelegramUserID:          303,
		ChatID:                  404,
		Name:                    "Bob",
		UTCOffsetMinutes:        0,
		MorningTime:             "07:00",
		ReminderIntervalMinutes: 15,
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	if err := repo.CreateUser(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	if err := repo.UpdateUserSettings(ctx, user.ID, "09:45", "02:00", 20); err != nil {
		t.Fatalf("update user settings: %v", err)
	}

	plans := []*domain.DayPlan{
		{UserID: user.ID, DayLocal: "2026-04-06", Status: domain.PlanStatusActive, Cycle: 1, CreatedAt: now, UpdatedAt: now},
		{UserID: user.ID, DayLocal: "2026-04-05", Status: domain.PlanStatusCompleted, Cycle: 1, CreatedAt: now, UpdatedAt: now},
	}
	for _, plan := range plans {
		if err := repo.SaveDayPlan(ctx, plan); err != nil {
			t.Fatalf("save day plan %s: %v", plan.DayLocal, err)
		}
	}

	loadedUser, err := repo.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get user by id: %v", err)
	}
	if loadedUser.MorningTime != "09:45" || loadedUser.DayEndTime != "02:00" || loadedUser.ReminderIntervalMinutes != 20 {
		t.Fatalf("user settings not updated: %+v", loadedUser)
	}
	if loadedUser.NotificationsPausedUntil != nil {
		t.Fatalf("unexpected notifications pause in default state: %v", loadedUser.NotificationsPausedUntil)
	}

	pausedUntil := now.Add(20 * time.Hour)
	if err := repo.UpdateUserNotificationsPausedUntil(ctx, user.ID, &pausedUntil); err != nil {
		t.Fatalf("update user notifications pause: %v", err)
	}
	loadedUser, err = repo.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("reload user by id: %v", err)
	}
	if loadedUser.NotificationsPausedUntil == nil || !loadedUser.NotificationsPausedUntil.Equal(pausedUntil) {
		t.Fatalf("notifications pause was not stored: %+v", loadedUser)
	}

	loadedPlans, err := repo.ListPlans(ctx, user.ID)
	if err != nil {
		t.Fatalf("list plans: %v", err)
	}
	if len(loadedPlans) != 2 {
		t.Fatalf("unexpected plans len: %d", len(loadedPlans))
	}
	if loadedPlans[0].DayLocal != "2026-04-05" || loadedPlans[1].DayLocal != "2026-04-06" {
		t.Fatalf("plans are not sorted by day: %+v", loadedPlans)
	}
}

func TestRepositoryActivityReminderWindowsRoundTrip(t *testing.T) {
	db, repo := newTestRepository(t)
	defer db.Close()

	ctx := context.Background()
	now := time.Date(2026, 4, 6, 6, 0, 0, 0, time.UTC)

	user := &domain.User{
		TelegramUserID:          505,
		ChatID:                  606,
		Name:                    "Clara",
		UTCOffsetMinutes:        0,
		MorningTime:             "07:00",
		DayEndTime:              "00:00",
		ReminderIntervalMinutes: 30,
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	if err := repo.CreateUser(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	activity := &domain.Activity{
		UserID:      user.ID,
		Title:       "Read",
		SortOrder:   1,
		TimesPerDay: 1,
		ReminderWindows: []domain.ReminderWindow{
			{Start: "08:00", End: "10:00"},
			{Start: "22:00", End: "01:00"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateActivity(ctx, activity); err != nil {
		t.Fatalf("create activity: %v", err)
	}

	activities, err := repo.ListActivities(ctx, user.ID)
	if err != nil {
		t.Fatalf("list activities: %v", err)
	}
	if len(activities) != 1 {
		t.Fatalf("unexpected activities count: %d", len(activities))
	}
	if len(activities[0].ReminderWindows) != 2 {
		t.Fatalf("expected 2 reminder windows, got %+v", activities[0].ReminderWindows)
	}
	if activities[0].ReminderWindowStart != "08:00" || activities[0].ReminderWindowEnd != "10:00" {
		t.Fatalf("unexpected legacy window columns: %s-%s", activities[0].ReminderWindowStart, activities[0].ReminderWindowEnd)
	}

	if err := repo.UpdateActivityReminderWindows(ctx, user.ID, activity.ID, []domain.ReminderWindow{
		{Start: "14:00", End: "16:00"},
	}); err != nil {
		t.Fatalf("update activity reminder windows: %v", err)
	}

	activities, err = repo.ListActivities(ctx, user.ID)
	if err != nil {
		t.Fatalf("list activities after update: %v", err)
	}
	if len(activities[0].ReminderWindows) != 1 {
		t.Fatalf("expected 1 reminder window after update, got %+v", activities[0].ReminderWindows)
	}
	if activities[0].ReminderWindows[0].Start != "14:00" || activities[0].ReminderWindows[0].End != "16:00" {
		t.Fatalf("unexpected reminder window after update: %+v", activities[0].ReminderWindows[0])
	}

	if err := repo.UpdateActivityReminderWindows(ctx, user.ID, activity.ID, nil); err != nil {
		t.Fatalf("clear activity reminder windows: %v", err)
	}
	activities, err = repo.ListActivities(ctx, user.ID)
	if err != nil {
		t.Fatalf("list activities after clear: %v", err)
	}
	if len(activities[0].ReminderWindows) != 0 {
		t.Fatalf("expected no reminder windows after clear, got %+v", activities[0].ReminderWindows)
	}
	if activities[0].ReminderWindowStart != "" || activities[0].ReminderWindowEnd != "" {
		t.Fatalf("expected empty legacy window columns after clear, got %s-%s", activities[0].ReminderWindowStart, activities[0].ReminderWindowEnd)
	}
}

func TestRepositoryReminderMessagesRoundTrip(t *testing.T) {
	db, repo := newTestRepository(t)
	defer db.Close()

	ctx := context.Background()
	now := time.Date(2026, 4, 7, 6, 0, 0, 0, time.UTC)

	user := &domain.User{
		TelegramUserID:          909,
		ChatID:                  1001,
		Name:                    "Dina",
		UTCOffsetMinutes:        0,
		MorningTime:             "07:00",
		DayEndTime:              "00:00",
		ReminderIntervalMinutes: 30,
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	if err := repo.CreateUser(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	if err := repo.SaveReminderMessage(ctx, &domain.ReminderMessage{
		UserID:     user.ID,
		ChatID:     user.ChatID,
		MessageID:  11,
		LogicalDay: "2026-04-06",
		Kind:       domain.ReminderMessageKindDaily,
		SentAt:     now.Add(-2 * time.Hour),
	}); err != nil {
		t.Fatalf("save first reminder message: %v", err)
	}
	if err := repo.SaveReminderMessage(ctx, &domain.ReminderMessage{
		UserID:     user.ID,
		ChatID:     user.ChatID,
		MessageID:  12,
		LogicalDay: "2026-04-07",
		Kind:       domain.ReminderMessageKindOneOff,
		SentAt:     now.Add(-1 * time.Hour),
	}); err != nil {
		t.Fatalf("save second reminder message: %v", err)
	}

	stale, err := repo.ListReminderMessagesBeforeDay(ctx, user.ID, "2026-04-07")
	if err != nil {
		t.Fatalf("list stale reminder messages: %v", err)
	}
	if len(stale) != 1 {
		t.Fatalf("expected one stale reminder, got %+v", stale)
	}
	if stale[0].MessageID != 11 || stale[0].Kind != domain.ReminderMessageKindDaily {
		t.Fatalf("unexpected stale reminder: %+v", stale[0])
	}

	if err := repo.DeleteReminderMessage(ctx, user.ID, 11); err != nil {
		t.Fatalf("delete reminder message: %v", err)
	}

	stale, err = repo.ListReminderMessagesBeforeDay(ctx, user.ID, "2026-04-08")
	if err != nil {
		t.Fatalf("list stale reminders after delete: %v", err)
	}
	if len(stale) != 1 || stale[0].MessageID != 12 {
		t.Fatalf("unexpected reminders after delete: %+v", stale)
	}
}

func newTestRepository(t *testing.T) (*sql.DB, *Repository) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}

	return db, NewRepository(db)
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
