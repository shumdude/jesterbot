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

	if err := repo.UpdateUserSettings(ctx, user.ID, "09:45", 20); err != nil {
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
	if loadedUser.MorningTime != "09:45" || loadedUser.ReminderIntervalMinutes != 20 {
		t.Fatalf("user settings not updated: %+v", loadedUser)
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
