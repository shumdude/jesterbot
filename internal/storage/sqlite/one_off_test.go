package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"jesterbot/internal/domain"
)

func TestRepositoryOneOffTaskAndReminderSettingsRoundTrip(t *testing.T) {
	db, repo := newTestRepository(t)
	defer db.Close()

	ctx := context.Background()
	now := time.Date(2026, 4, 9, 9, 0, 0, 0, time.UTC)

	user := &domain.User{
		TelegramUserID:          505,
		ChatID:                  606,
		Name:                    "Carol",
		UTCOffsetMinutes:        180,
		MorningTime:             "08:30",
		ReminderIntervalMinutes: 20,
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	if err := repo.CreateUser(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := repo.SaveUserTickInterval(ctx, user.ID, 3); err != nil {
		t.Fatalf("save user tick interval: %v", err)
	}
	tickInterval, err := repo.GetUserTickInterval(ctx, user.ID)
	if err != nil {
		t.Fatalf("get user tick interval: %v", err)
	}
	if tickInterval != 3 {
		t.Fatalf("unexpected user tick interval: %d", tickInterval)
	}

	settings := &domain.OneOffReminderSettings{
		UserID:                user.ID,
		LowPriorityMinutes:    120,
		MediumPriorityMinutes: 45,
		HighPriorityMinutes:   15,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if err := repo.SaveOneOffReminderSettings(ctx, settings); err != nil {
		t.Fatalf("save one-off reminder settings: %v", err)
	}

	loadedSettings, err := repo.GetOneOffReminderSettings(ctx, user.ID)
	if err != nil {
		t.Fatalf("get one-off reminder settings: %v", err)
	}
	if loadedSettings.HighPriorityMinutes != 15 || loadedSettings.LowPriorityMinutes != 120 {
		t.Fatalf("unexpected one-off reminder settings: %+v", loadedSettings)
	}

	task := &domain.OneOffTask{
		UserID:         user.ID,
		Title:          "Prepare release",
		Priority:       domain.OneOffTaskPriorityHigh,
		Status:         domain.OneOffTaskStatusActive,
		NextReminderAt: ptrTime(now.Add(15 * time.Minute)),
		CreatedAt:      now,
		UpdatedAt:      now,
		Items: []domain.OneOffTaskItem{
			{Title: "Write changelog", SortOrder: 1, CreatedAt: now, UpdatedAt: now},
			{Title: "Publish build", SortOrder: 2, CreatedAt: now, UpdatedAt: now},
		},
	}
	if err := repo.SaveOneOffTask(ctx, task); err != nil {
		t.Fatalf("save one-off task: %v", err)
	}

	loadedTask, err := repo.GetOneOffTask(ctx, user.ID, task.ID)
	if err != nil {
		t.Fatalf("get one-off task: %v", err)
	}
	if loadedTask.Title != task.Title || loadedTask.Priority != domain.OneOffTaskPriorityHigh {
		t.Fatalf("unexpected loaded one-off task: %+v", loadedTask)
	}
	if len(loadedTask.Items) != 2 {
		t.Fatalf("unexpected loaded one-off items len: %d", len(loadedTask.Items))
	}

	loadedTask.Items[0].Completed = true
	loadedTask.Items[0].CompletedAt = ptrTime(now.Add(5 * time.Minute))
	loadedTask.Items[0].UpdatedAt = now.Add(5 * time.Minute)
	if err := repo.SaveOneOffTask(ctx, loadedTask); err != nil {
		t.Fatalf("update one-off task: %v", err)
	}

	loadedTasks, err := repo.ListOneOffTasks(ctx, user.ID)
	if err != nil {
		t.Fatalf("list one-off tasks: %v", err)
	}
	if len(loadedTasks) != 1 || !loadedTasks[0].Items[0].Completed {
		t.Fatalf("unexpected listed one-off tasks: %+v", loadedTasks)
	}

	if err := repo.DeleteOneOffTask(ctx, user.ID, task.ID); err != nil {
		t.Fatalf("delete one-off task: %v", err)
	}

	_, err = repo.GetOneOffTask(ctx, user.ID, task.ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found after delete, got %v", err)
	}
}
