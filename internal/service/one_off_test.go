package service

import (
	"context"
	"testing"
	"time"

	"jesterbot/internal/domain"
)

func TestCreateOneOffTaskUsesPriorityReminderSettings(t *testing.T) {
	repo := newMemoryRepo()
	svc := New(repo, 30)
	now := time.Date(2026, 4, 9, 9, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	user := seedUser(repo)
	if err := svc.UpdateOneOffReminderSettings(context.Background(), user.ID, 60, 30, 10); err != nil {
		t.Fatalf("update one-off reminder settings: %v", err)
	}

	task, err := svc.CreateOneOffTask(context.Background(), user.ID, "Prepare docs", domain.OneOffTaskPriorityHigh, []string{"Draft", "Review"})
	if err != nil {
		t.Fatalf("create one-off task: %v", err)
	}

	if task.Priority != domain.OneOffTaskPriorityHigh {
		t.Fatalf("unexpected priority: %s", task.Priority)
	}
	if task.NextReminderAt == nil || !task.NextReminderAt.Equal(now.Add(10*time.Minute)) {
		t.Fatalf("unexpected next reminder: %v", task.NextReminderAt)
	}
	if len(task.Items) != 2 {
		t.Fatalf("unexpected checklist items len: %d", len(task.Items))
	}
}

func TestPickOneOffReminderPrefersHigherPriority(t *testing.T) {
	repo := newMemoryRepo()
	svc := New(repo, 30)
	now := time.Date(2026, 4, 9, 9, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	user := seedUser(repo)
	if err := svc.UpdateOneOffReminderSettings(context.Background(), user.ID, 90, 30, 10); err != nil {
		t.Fatalf("update one-off reminder settings: %v", err)
	}

	lowTask, err := svc.CreateOneOffTask(context.Background(), user.ID, "Buy batteries", domain.OneOffTaskPriorityLow, nil)
	if err != nil {
		t.Fatalf("create low-priority task: %v", err)
	}
	highTask, err := svc.CreateOneOffTask(context.Background(), user.ID, "Pay bill", domain.OneOffTaskPriorityHigh, nil)
	if err != nil {
		t.Fatalf("create high-priority task: %v", err)
	}

	lowReminder := now.Add(-time.Minute)
	lowTask.NextReminderAt = &lowReminder
	if err := repo.SaveOneOffTask(context.Background(), lowTask); err != nil {
		t.Fatalf("save low-priority task: %v", err)
	}
	highReminder := now.Add(-2 * time.Minute)
	highTask.NextReminderAt = &highReminder
	if err := repo.SaveOneOffTask(context.Background(), highTask); err != nil {
		t.Fatalf("save high-priority task: %v", err)
	}

	reminder, err := svc.PickOneOffReminder(context.Background(), user.ID, now)
	if err != nil {
		t.Fatalf("pick one-off reminder: %v", err)
	}

	if reminder.ID != highTask.ID {
		t.Fatalf("expected high-priority task, got %d", reminder.ID)
	}
	if reminder.NextReminderAt == nil || !reminder.NextReminderAt.Equal(now.Add(10*time.Minute)) {
		t.Fatalf("unexpected next reminder: %v", reminder.NextReminderAt)
	}
}

func TestToggleOneOffTaskItemsCompletesTaskAndBuildsStats(t *testing.T) {
	repo := newMemoryRepo()
	svc := New(repo, 30)
	now := time.Date(2026, 4, 9, 9, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	user := seedUser(repo)
	task, err := svc.CreateOneOffTask(context.Background(), user.ID, "Launch feature", domain.OneOffTaskPriorityMedium, []string{"Backend", "Bot UI"})
	if err != nil {
		t.Fatalf("create one-off task: %v", err)
	}

	task, err = svc.ToggleOneOffTaskItem(context.Background(), user.ID, task.ID, task.Items[0].ID, now.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("toggle first checklist item: %v", err)
	}
	if task.Status != domain.OneOffTaskStatusActive {
		t.Fatalf("expected active task after first toggle, got %s", task.Status)
	}

	task, err = svc.ToggleOneOffTaskItem(context.Background(), user.ID, task.ID, task.Items[1].ID, now.Add(10*time.Minute))
	if err != nil {
		t.Fatalf("toggle second checklist item: %v", err)
	}
	if task.Status != domain.OneOffTaskStatusCompleted {
		t.Fatalf("expected completed task, got %s", task.Status)
	}
	if task.NextReminderAt != nil {
		t.Fatalf("expected completed task to clear reminder, got %v", task.NextReminderAt)
	}

	stats, err := svc.BuildStats(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("build stats: %v", err)
	}
	if stats.OneOffTasks != 1 || stats.CompletedOneOffTasks != 1 || stats.PendingOneOffTasks != 0 {
		t.Fatalf("unexpected one-off task stats: %+v", stats)
	}
	if stats.OneOffChecklistItems != 2 || stats.CompletedOneOffChecklistItems != 2 {
		t.Fatalf("unexpected one-off checklist stats: %+v", stats)
	}
}

func TestGetAndUpdateUserTickInterval(t *testing.T) {
	repo := newMemoryRepo()
	svc := New(repo, 30)

	user := seedUser(repo)
	minutes, err := svc.GetUserTickInterval(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("get default user tick interval: %v", err)
	}
	if minutes != defaultUserTickIntervalMinutes {
		t.Fatalf("unexpected default tick interval: %d", minutes)
	}

	if err := svc.UpdateUserTickInterval(context.Background(), user.ID, 3); err != nil {
		t.Fatalf("update user tick interval: %v", err)
	}
	minutes, err = svc.GetUserTickInterval(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("get updated user tick interval: %v", err)
	}
	if minutes != 3 {
		t.Fatalf("unexpected updated tick interval: %d", minutes)
	}
}

func TestUpdateOneOffReminderSettingsReschedulesActiveTasks(t *testing.T) {
	repo := newMemoryRepo()
	svc := New(repo, 30)
	createdAt := time.Date(2026, 4, 9, 9, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return createdAt }

	user := seedUser(repo)
	highTask, err := svc.CreateOneOffTask(context.Background(), user.ID, "Pay bill", domain.OneOffTaskPriorityHigh, nil)
	if err != nil {
		t.Fatalf("create high task: %v", err)
	}
	lowTask, err := svc.CreateOneOffTask(context.Background(), user.ID, "Buy batteries", domain.OneOffTaskPriorityLow, nil)
	if err != nil {
		t.Fatalf("create low task: %v", err)
	}

	updatedAt := createdAt.Add(5 * time.Minute)
	svc.now = func() time.Time { return updatedAt }
	if err := svc.UpdateOneOffReminderSettings(context.Background(), user.ID, 60, 30, 10); err != nil {
		t.Fatalf("update one-off reminder settings: %v", err)
	}

	reloadedHighTask, err := svc.GetOneOffTask(context.Background(), user.ID, highTask.ID)
	if err != nil {
		t.Fatalf("reload high task: %v", err)
	}
	reloadedLowTask, err := svc.GetOneOffTask(context.Background(), user.ID, lowTask.ID)
	if err != nil {
		t.Fatalf("reload low task: %v", err)
	}

	expectedHighReminder := updatedAt.Add(10 * time.Minute)
	if reloadedHighTask.NextReminderAt == nil || !reloadedHighTask.NextReminderAt.Equal(expectedHighReminder) {
		t.Fatalf("unexpected high-priority reminder: %v", reloadedHighTask.NextReminderAt)
	}
	expectedLowReminder := updatedAt.Add(60 * time.Minute)
	if reloadedLowTask.NextReminderAt == nil || !reloadedLowTask.NextReminderAt.Equal(expectedLowReminder) {
		t.Fatalf("unexpected low-priority reminder: %v", reloadedLowTask.NextReminderAt)
	}
}

func (m *memoryRepo) GetOneOffReminderSettings(_ context.Context, userID int64) (*domain.OneOffReminderSettings, error) {
	settings, ok := m.oneOffReminderSettings[userID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return cloneOneOffReminderSettings(settings), nil
}

func (m *memoryRepo) SaveOneOffReminderSettings(_ context.Context, settings *domain.OneOffReminderSettings) error {
	m.oneOffReminderSettings[settings.UserID] = *cloneOneOffReminderSettings(*settings)
	return nil
}

func (m *memoryRepo) GetUserTickInterval(_ context.Context, userID int64) (int, error) {
	minutes, ok := m.userTickIntervals[userID]
	if !ok {
		return 0, domain.ErrNotFound
	}
	return minutes, nil
}

func (m *memoryRepo) SaveUserTickInterval(_ context.Context, userID int64, minutes int) error {
	m.userTickIntervals[userID] = minutes
	return nil
}

func (m *memoryRepo) GetOneOffTask(_ context.Context, userID, taskID int64) (*domain.OneOffTask, error) {
	userTasks := m.oneOffTasks[userID]
	task, ok := userTasks[taskID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return cloneOneOffTask(task), nil
}

func (m *memoryRepo) ListOneOffTasks(_ context.Context, userID int64) ([]domain.OneOffTask, error) {
	userTasks := m.oneOffTasks[userID]
	tasks := make([]domain.OneOffTask, 0, len(userTasks))
	for _, task := range userTasks {
		tasks = append(tasks, *cloneOneOffTask(task))
	}
	return tasks, nil
}

func (m *memoryRepo) SaveOneOffTask(_ context.Context, task *domain.OneOffTask) error {
	if task.ID == 0 {
		m.nextOneOffTaskID++
		task.ID = m.nextOneOffTaskID
	}
	for i := range task.Items {
		if task.Items[i].ID == 0 {
			m.nextOneOffTaskItemID++
			task.Items[i].ID = m.nextOneOffTaskItemID
		}
		task.Items[i].TaskID = task.ID
	}

	if m.oneOffTasks[task.UserID] == nil {
		m.oneOffTasks[task.UserID] = make(map[int64]domain.OneOffTask)
	}
	m.oneOffTasks[task.UserID][task.ID] = *cloneOneOffTask(*task)
	return nil
}

func (m *memoryRepo) DeleteOneOffTask(_ context.Context, userID, taskID int64) error {
	userTasks := m.oneOffTasks[userID]
	if _, ok := userTasks[taskID]; !ok {
		return domain.ErrNotFound
	}
	delete(userTasks, taskID)
	return nil
}

func cloneOneOffTask(task domain.OneOffTask) *domain.OneOffTask {
	cloned := task
	cloned.Items = append([]domain.OneOffTaskItem(nil), task.Items...)
	return &cloned
}

func cloneOneOffReminderSettings(settings domain.OneOffReminderSettings) *domain.OneOffReminderSettings {
	cloned := settings
	return &cloned
}
