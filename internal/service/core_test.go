package service

import (
	"context"
	"testing"
	"time"

	"jesterbot/internal/domain"
)

func TestRegisterUserValidatesAndNormalizes(t *testing.T) {
	repo := newMemoryRepo()
	svc := New(repo, 30)
	svc.now = func() time.Time { return time.Date(2026, 4, 6, 6, 0, 0, 0, time.UTC) }

	user, err := svc.RegisterUser(context.Background(), RegistrationInput{
		TelegramUserID: 7,
		ChatID:         9,
		Name:           " Alice ",
		UTCOffset:      "UTC+03:30",
		MorningTime:    "08:15",
	})
	if err != nil {
		t.Fatalf("register user: %v", err)
	}

	if user.Name != "Alice" {
		t.Fatalf("unexpected name: %q", user.Name)
	}
	if user.UTCOffsetMinutes != 210 {
		t.Fatalf("unexpected offset: %d", user.UTCOffsetMinutes)
	}
	if user.MorningTime != "08:15" {
		t.Fatalf("unexpected morning time: %s", user.MorningTime)
	}
}

func TestReminderCycleAvoidsRepeatsBeforeReset(t *testing.T) {
	repo := newMemoryRepo()
	svc := New(repo, 30)
	svc.rng.Seed(1)

	user := seedUser(repo)
	seedActivities(repo, user.ID, "A", "B", "C")

	now := time.Date(2026, 4, 6, 5, 0, 0, 0, time.UTC)
	plan, err := svc.StartMorningPlan(context.Background(), user.ID, now)
	if err != nil {
		t.Fatalf("start morning plan: %v", err)
	}
	if _, err := svc.FinalizePlan(context.Background(), user.ID, now); err != nil {
		t.Fatalf("finalize plan: %v", err)
	}

	seen := map[int64]bool{}
	for i := range plan.Items {
		reminderTime := now.Add(time.Duration((i+1)*31) * time.Minute)
		item, _, err := svc.PickReminder(context.Background(), user.ID, reminderTime)
		if err != nil {
			t.Fatalf("pick reminder: %v", err)
		}
		if seen[item.ActivityID] {
			t.Fatalf("activity %d repeated in same cycle", item.ActivityID)
		}
		seen[item.ActivityID] = true
	}

	item, updatedPlan, err := svc.PickReminder(context.Background(), user.ID, now.Add(124*time.Minute))
	if err != nil {
		t.Fatalf("pick reminder after cycle reset: %v", err)
	}
	if updatedPlan.Cycle != 2 {
		t.Fatalf("expected cycle 2, got %d", updatedPlan.Cycle)
	}
	if item == nil {
		t.Fatal("expected reminder item")
	}
}

func TestMarkDoneCompletesPlan(t *testing.T) {
	repo := newMemoryRepo()
	svc := New(repo, 30)

	user := seedUser(repo)
	activities := seedActivities(repo, user.ID, "Stretch")

	now := time.Date(2026, 4, 6, 5, 0, 0, 0, time.UTC)
	if _, err := svc.StartMorningPlan(context.Background(), user.ID, now); err != nil {
		t.Fatalf("start morning plan: %v", err)
	}
	if _, err := svc.FinalizePlan(context.Background(), user.ID, now); err != nil {
		t.Fatalf("finalize plan: %v", err)
	}

	plan, err := svc.MarkActivityDone(context.Background(), user.ID, activities[0].ID, now.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("mark done: %v", err)
	}
	if plan.Status != domain.PlanStatusCompleted {
		t.Fatalf("expected completed plan, got %s", plan.Status)
	}
	if plan.CompletedAt == nil {
		t.Fatal("expected completion timestamp")
	}
}

func TestBuildStats(t *testing.T) {
	repo := newMemoryRepo()
	svc := New(repo, 30)

	user := seedUser(repo)
	now := time.Date(2026, 4, 6, 5, 0, 0, 0, time.UTC)
	repo.savePlan(&domain.DayPlan{
		UserID:   user.ID,
		DayLocal: "2026-04-05",
		Status:   domain.PlanStatusCompleted,
		Items: []domain.DayPlanItem{
			{ActivityID: 1, Selected: true, Completed: true},
			{ActivityID: 2, Selected: false, Completed: false},
		},
		CreatedAt: now,
		UpdatedAt: now,
	})
	repo.savePlan(&domain.DayPlan{
		UserID:   user.ID,
		DayLocal: "2026-04-06",
		Status:   domain.PlanStatusActive,
		Items: []domain.DayPlanItem{
			{ActivityID: 3, Selected: true, Completed: false},
		},
		CreatedAt: now,
		UpdatedAt: now,
	})

	stats, err := svc.BuildStats(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("build stats: %v", err)
	}

	if stats.DaysWithPlan != 2 || stats.CompletedDays != 1 {
		t.Fatalf("unexpected day stats: %+v", stats)
	}
	if stats.SelectedActivities != 2 || stats.CompletedActivities != 1 || stats.SkippedActivities != 1 {
		t.Fatalf("unexpected item stats: %+v", stats)
	}
}

func TestAddActivitiesBatch(t *testing.T) {
	repo := newMemoryRepo()
	svc := New(repo, 30)

	user := seedUser(repo)
	created, err := svc.AddActivities(context.Background(), user.ID, " Stretch , Read\nWalk ,, ")
	if err != nil {
		t.Fatalf("add activities batch: %v", err)
	}

	if len(created) != 3 {
		t.Fatalf("unexpected created len: %d", len(created))
	}

	activities, err := repo.ListActivities(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("list activities: %v", err)
	}
	if len(activities) != 3 {
		t.Fatalf("unexpected stored activities len: %d", len(activities))
	}

	expectedTitles := []string{"Stretch", "Read", "Walk"}
	for i, title := range expectedTitles {
		if activities[i].Title != title {
			t.Fatalf("unexpected title at %d: %q", i, activities[i].Title)
		}
		if activities[i].SortOrder != i+1 {
			t.Fatalf("unexpected sort order at %d: %d", i, activities[i].SortOrder)
		}
	}
}

func TestUpdateSettingsReschedulesActivePlanReminder(t *testing.T) {
	repo := newMemoryRepo()
	svc := New(repo, 30)

	user := seedUser(repo)
	activities := seedActivities(repo, user.ID, "Stretch")
	_ = activities

	startedAt := time.Date(2026, 4, 6, 5, 0, 0, 0, time.UTC)
	if _, err := svc.StartMorningPlan(context.Background(), user.ID, startedAt); err != nil {
		t.Fatalf("start morning plan: %v", err)
	}
	if _, err := svc.FinalizePlan(context.Background(), user.ID, startedAt); err != nil {
		t.Fatalf("finalize plan: %v", err)
	}

	updatedAt := startedAt.Add(5 * time.Minute)
	svc.now = func() time.Time { return updatedAt }
	if err := svc.UpdateSettings(context.Background(), user.ID, user.MorningTime, 10); err != nil {
		t.Fatalf("update settings: %v", err)
	}

	plan, err := svc.GetTodayPlan(context.Background(), user.ID, updatedAt)
	if err != nil {
		t.Fatalf("get today plan: %v", err)
	}
	if plan.NextReminderAt == nil {
		t.Fatal("expected next reminder to be rescheduled")
	}

	expectedReminder := updatedAt.Add(10 * time.Minute)
	if !plan.NextReminderAt.Equal(expectedReminder) {
		t.Fatalf("unexpected next reminder: got %v want %v", plan.NextReminderAt, expectedReminder)
	}
}

type memoryRepo struct {
	nextUserID             int64
	nextActivityID         int64
	nextPlanID             int64
	nextOneOffTaskID       int64
	nextOneOffTaskItemID   int64
	users                  map[int64]domain.User
	usersByTG              map[int64]int64
	activities             map[int64][]domain.Activity
	plans                  map[int64]map[string]domain.DayPlan
	oneOffTasks            map[int64]map[int64]domain.OneOffTask
	oneOffReminderSettings map[int64]domain.OneOffReminderSettings
	userTickIntervals      map[int64]int
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		users:                  make(map[int64]domain.User),
		usersByTG:              make(map[int64]int64),
		activities:             make(map[int64][]domain.Activity),
		plans:                  make(map[int64]map[string]domain.DayPlan),
		oneOffTasks:            make(map[int64]map[int64]domain.OneOffTask),
		oneOffReminderSettings: make(map[int64]domain.OneOffReminderSettings),
		userTickIntervals:      make(map[int64]int),
	}
}

func (m *memoryRepo) GetUserByTelegramID(_ context.Context, telegramUserID int64) (*domain.User, error) {
	userID, ok := m.usersByTG[telegramUserID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	user := m.users[userID]
	return cloneUser(user), nil
}

func (m *memoryRepo) GetUserByID(_ context.Context, userID int64) (*domain.User, error) {
	user, ok := m.users[userID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return cloneUser(user), nil
}

func (m *memoryRepo) ListUsers(_ context.Context) ([]domain.User, error) {
	users := make([]domain.User, 0, len(m.users))
	for _, user := range m.users {
		users = append(users, user)
	}
	return users, nil
}

func (m *memoryRepo) CreateUser(_ context.Context, user *domain.User) error {
	m.nextUserID++
	user.ID = m.nextUserID
	m.users[user.ID] = *user
	m.usersByTG[user.TelegramUserID] = user.ID
	return nil
}

func (m *memoryRepo) UpdateUserSettings(_ context.Context, userID int64, morningTime string, reminderIntervalMinutes int) error {
	user, ok := m.users[userID]
	if !ok {
		return domain.ErrNotFound
	}
	user.MorningTime = morningTime
	user.ReminderIntervalMinutes = reminderIntervalMinutes
	m.users[userID] = user
	return nil
}

func (m *memoryRepo) CreateActivity(_ context.Context, activity *domain.Activity) error {
	m.nextActivityID++
	activity.ID = m.nextActivityID
	m.activities[activity.UserID] = append(m.activities[activity.UserID], *activity)
	return nil
}

func (m *memoryRepo) UpdateActivity(_ context.Context, userID, activityID int64, title string) error {
	activities := m.activities[userID]
	for i := range activities {
		if activities[i].ID == activityID {
			activities[i].Title = title
			m.activities[userID] = activities
			return nil
		}
	}
	return domain.ErrNotFound
}

func (m *memoryRepo) UpdateActivityTimesPerDay(_ context.Context, userID, activityID int64, timesPerDay int) error {
	activities := m.activities[userID]
	for i := range activities {
		if activities[i].ID == activityID {
			activities[i].TimesPerDay = timesPerDay
			m.activities[userID] = activities
			return nil
		}
	}
	return domain.ErrNotFound
}

func (m *memoryRepo) DeleteActivity(_ context.Context, userID, activityID int64) error {
	activities := m.activities[userID]
	for i := range activities {
		if activities[i].ID == activityID {
			m.activities[userID] = append(activities[:i], activities[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (m *memoryRepo) ListActivities(_ context.Context, userID int64) ([]domain.Activity, error) {
	activities := m.activities[userID]
	cloned := make([]domain.Activity, len(activities))
	copy(cloned, activities)
	return cloned, nil
}

func (m *memoryRepo) GetDayPlan(_ context.Context, userID int64, dayLocal string) (*domain.DayPlan, error) {
	plansByDay, ok := m.plans[userID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	plan, ok := plansByDay[dayLocal]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return clonePlan(plan), nil
}

func (m *memoryRepo) SaveDayPlan(_ context.Context, plan *domain.DayPlan) error {
	return m.savePlan(plan)
}

func (m *memoryRepo) ListPlans(_ context.Context, userID int64) ([]domain.DayPlan, error) {
	plansByDay := m.plans[userID]
	plans := make([]domain.DayPlan, 0, len(plansByDay))
	for _, plan := range plansByDay {
		plans = append(plans, plan)
	}
	return plans, nil
}

func (m *memoryRepo) savePlan(plan *domain.DayPlan) error {
	if plan.ID == 0 {
		m.nextPlanID++
		plan.ID = m.nextPlanID
	}
	if m.plans[plan.UserID] == nil {
		m.plans[plan.UserID] = make(map[string]domain.DayPlan)
	}
	m.plans[plan.UserID][plan.DayLocal] = *clonePlan(*plan)
	return nil
}

func seedUser(repo *memoryRepo) domain.User {
	user := domain.User{
		TelegramUserID:          1,
		ChatID:                  10,
		Name:                    "Alice",
		UTCOffsetMinutes:        0,
		MorningTime:             "08:00",
		ReminderIntervalMinutes: 30,
		CreatedAt:               time.Now().UTC(),
		UpdatedAt:               time.Now().UTC(),
	}
	repo.nextUserID++
	user.ID = repo.nextUserID
	repo.users[user.ID] = user
	repo.usersByTG[user.TelegramUserID] = user.ID
	return user
}

func seedActivities(repo *memoryRepo, userID int64, titles ...string) []domain.Activity {
	activities := make([]domain.Activity, 0, len(titles))
	for i, title := range titles {
		repo.nextActivityID++
		activity := domain.Activity{
			ID:        repo.nextActivityID,
			UserID:    userID,
			Title:     title,
			SortOrder: i + 1,
		}
		repo.activities[userID] = append(repo.activities[userID], activity)
		activities = append(activities, activity)
	}
	return activities
}

func cloneUser(user domain.User) *domain.User {
	cloned := user
	return &cloned
}

func clonePlan(plan domain.DayPlan) *domain.DayPlan {
	cloned := plan
	cloned.Items = append([]domain.DayPlanItem(nil), plan.Items...)
	return &cloned
}
