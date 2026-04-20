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
	if user.DayEndTime != "00:00" {
		t.Fatalf("unexpected day end time: %s", user.DayEndTime)
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

func TestMarkDoneKeepsScheduledReminderWhenStillInFuture(t *testing.T) {
	repo := newMemoryRepo()
	svc := New(repo, 30)
	svc.rng.Seed(1)

	user := seedUser(repo)
	activities := seedActivities(repo, user.ID, "Stretch", "Read")

	startedAt := time.Date(2026, 4, 6, 5, 0, 0, 0, time.UTC)
	if _, err := svc.StartMorningPlan(context.Background(), user.ID, startedAt); err != nil {
		t.Fatalf("start morning plan: %v", err)
	}
	if _, err := svc.FinalizePlan(context.Background(), user.ID, startedAt); err != nil {
		t.Fatalf("finalize plan: %v", err)
	}

	doneAt := startedAt.Add(10 * time.Minute)
	plan, err := svc.MarkActivityDone(context.Background(), user.ID, activities[0].ID, doneAt)
	if err != nil {
		t.Fatalf("mark done: %v", err)
	}

	expectedFirstTick := startedAt.Add(30 * time.Minute)
	if plan.NextReminderAt == nil || !plan.NextReminderAt.Equal(expectedFirstTick) {
		t.Fatalf("next reminder moved unexpectedly: got %v want %v", plan.NextReminderAt, expectedFirstTick)
	}

	item, updatedPlan, err := svc.PickReminder(context.Background(), user.ID, expectedFirstTick)
	if err != nil {
		t.Fatalf("pick reminder on scheduled tick: %v", err)
	}
	if item == nil || item.ActivityID != activities[1].ID {
		t.Fatalf("expected reminder for remaining activity %d, got %+v", activities[1].ID, item)
	}

	expectedNextTick := expectedFirstTick.Add(30 * time.Minute)
	if updatedPlan.NextReminderAt == nil || !updatedPlan.NextReminderAt.Equal(expectedNextTick) {
		t.Fatalf("unexpected next reminder after tick: got %v want %v", updatedPlan.NextReminderAt, expectedNextTick)
	}
}

func TestMarkDoneReschedulesReminderWhenOverdue(t *testing.T) {
	repo := newMemoryRepo()
	svc := New(repo, 30)

	user := seedUser(repo)
	activities := seedActivities(repo, user.ID, "Stretch", "Read")

	startedAt := time.Date(2026, 4, 6, 5, 0, 0, 0, time.UTC)
	if _, err := svc.StartMorningPlan(context.Background(), user.ID, startedAt); err != nil {
		t.Fatalf("start morning plan: %v", err)
	}
	if _, err := svc.FinalizePlan(context.Background(), user.ID, startedAt); err != nil {
		t.Fatalf("finalize plan: %v", err)
	}

	doneAt := startedAt.Add(40 * time.Minute)
	plan, err := svc.MarkActivityDone(context.Background(), user.ID, activities[0].ID, doneAt)
	if err != nil {
		t.Fatalf("mark done overdue: %v", err)
	}

	expectedRescheduled := doneAt.Add(30 * time.Minute)
	if plan.NextReminderAt == nil || !plan.NextReminderAt.Equal(expectedRescheduled) {
		t.Fatalf("unexpected overdue reminder reschedule: got %v want %v", plan.NextReminderAt, expectedRescheduled)
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
	if err := svc.UpdateSettings(context.Background(), user.ID, user.MorningTime, user.DayEndTime, 10); err != nil {
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

func TestSetActivityTimesPerDayReopensCompletedPlanForCurrentLogicalDay(t *testing.T) {
	repo := newMemoryRepo()
	svc := New(repo, 30)

	user := seedUser(repo)
	activities := seedActivities(repo, user.ID, "Stretch")

	startedAt := time.Date(2026, 4, 6, 5, 0, 0, 0, time.UTC)
	if _, err := svc.StartMorningPlan(context.Background(), user.ID, startedAt); err != nil {
		t.Fatalf("start morning plan: %v", err)
	}
	if _, err := svc.FinalizePlan(context.Background(), user.ID, startedAt); err != nil {
		t.Fatalf("finalize plan: %v", err)
	}
	if _, err := svc.MarkActivityDone(context.Background(), user.ID, activities[0].ID, startedAt.Add(5*time.Minute)); err != nil {
		t.Fatalf("mark done: %v", err)
	}

	updatedAt := startedAt.Add(10 * time.Minute)
	svc.now = func() time.Time { return updatedAt }
	if err := svc.SetActivityTimesPerDay(context.Background(), user.ID, activities[0].ID, 2); err != nil {
		t.Fatalf("set times per day: %v", err)
	}

	plan, err := svc.GetTodayPlan(context.Background(), user.ID, updatedAt)
	if err != nil {
		t.Fatalf("get today plan: %v", err)
	}

	if plan.Status != domain.PlanStatusActive {
		t.Fatalf("expected active plan after target increase, got %s", plan.Status)
	}
	if plan.CompletedAt != nil {
		t.Fatalf("expected completed_at to be cleared, got %v", plan.CompletedAt)
	}
	expectedNextReminder := updatedAt.Add(30 * time.Minute)
	if plan.NextReminderAt == nil || !plan.NextReminderAt.Equal(expectedNextReminder) {
		t.Fatalf("unexpected next reminder: got %v want %v", plan.NextReminderAt, expectedNextReminder)
	}

	item := plan.Items[0]
	if item.TimesPerDay != 2 {
		t.Fatalf("unexpected item times per day: %d", item.TimesPerDay)
	}
	if item.Completed {
		t.Fatal("expected item to become not completed after target increase")
	}
	if item.CompletedAt != nil {
		t.Fatalf("expected item completed_at to be cleared, got %v", item.CompletedAt)
	}
	if item.CompletedCount != 1 {
		t.Fatalf("unexpected completed count: %d", item.CompletedCount)
	}
}

func TestSetActivityTimesPerDayCompletesActivePlanForCurrentLogicalDay(t *testing.T) {
	repo := newMemoryRepo()
	svc := New(repo, 30)

	user := seedUser(repo)
	activities := seedActivities(repo, user.ID, "Stretch")
	if err := svc.SetActivityTimesPerDay(context.Background(), user.ID, activities[0].ID, 2); err != nil {
		t.Fatalf("set initial times per day: %v", err)
	}

	startedAt := time.Date(2026, 4, 6, 5, 0, 0, 0, time.UTC)
	if _, err := svc.StartMorningPlan(context.Background(), user.ID, startedAt); err != nil {
		t.Fatalf("start morning plan: %v", err)
	}
	if _, err := svc.FinalizePlan(context.Background(), user.ID, startedAt); err != nil {
		t.Fatalf("finalize plan: %v", err)
	}
	if _, err := svc.MarkActivityDone(context.Background(), user.ID, activities[0].ID, startedAt.Add(5*time.Minute)); err != nil {
		t.Fatalf("mark done: %v", err)
	}

	updatedAt := startedAt.Add(10 * time.Minute)
	svc.now = func() time.Time { return updatedAt }
	if err := svc.SetActivityTimesPerDay(context.Background(), user.ID, activities[0].ID, 1); err != nil {
		t.Fatalf("set times per day: %v", err)
	}

	plan, err := svc.GetTodayPlan(context.Background(), user.ID, updatedAt)
	if err != nil {
		t.Fatalf("get today plan: %v", err)
	}

	if plan.Status != domain.PlanStatusCompleted {
		t.Fatalf("expected completed plan after target decrease, got %s", plan.Status)
	}
	if plan.CompletedAt == nil || !plan.CompletedAt.Equal(updatedAt) {
		t.Fatalf("expected completed_at=%v, got %v", updatedAt, plan.CompletedAt)
	}
	if plan.NextReminderAt != nil {
		t.Fatalf("expected no next reminder for completed plan, got %v", plan.NextReminderAt)
	}

	item := plan.Items[0]
	if item.TimesPerDay != 1 {
		t.Fatalf("unexpected item times per day: %d", item.TimesPerDay)
	}
	if !item.Completed {
		t.Fatal("expected item to become completed after target decrease")
	}
	if item.CompletedAt == nil || !item.CompletedAt.Equal(updatedAt) {
		t.Fatalf("expected item completed_at=%v, got %v", updatedAt, item.CompletedAt)
	}
	if item.CompletedCount != 1 {
		t.Fatalf("unexpected completed count: %d", item.CompletedCount)
	}
}

func TestSetActivityTimesPerDayUsesLogicalDayBoundary(t *testing.T) {
	repo := newMemoryRepo()
	svc := New(repo, 30)

	user := seedUser(repo)
	user.DayEndTime = "03:00"
	repo.users[user.ID] = user
	activities := seedActivities(repo, user.ID, "Stretch")

	planTime := time.Date(2026, 4, 6, 1, 0, 0, 0, time.UTC) // logical day is 2026-04-05
	if _, err := svc.StartMorningPlan(context.Background(), user.ID, planTime); err != nil {
		t.Fatalf("start morning plan: %v", err)
	}
	if _, err := svc.FinalizePlan(context.Background(), user.ID, planTime); err != nil {
		t.Fatalf("finalize plan: %v", err)
	}
	if _, err := svc.MarkActivityDone(context.Background(), user.ID, activities[0].ID, planTime.Add(5*time.Minute)); err != nil {
		t.Fatalf("mark done: %v", err)
	}

	updatedAt := time.Date(2026, 4, 6, 1, 30, 0, 0, time.UTC) // still logical day 2026-04-05
	svc.now = func() time.Time { return updatedAt }
	if err := svc.SetActivityTimesPerDay(context.Background(), user.ID, activities[0].ID, 2); err != nil {
		t.Fatalf("set times per day: %v", err)
	}

	plan, err := svc.GetTodayPlan(context.Background(), user.ID, updatedAt)
	if err != nil {
		t.Fatalf("get today plan: %v", err)
	}
	if plan.DayLocal != "2026-04-05" {
		t.Fatalf("unexpected logical day: %s", plan.DayLocal)
	}
	if plan.Status != domain.PlanStatusActive {
		t.Fatalf("expected active plan after target increase within same logical day, got %s", plan.Status)
	}
	if plan.Items[0].TimesPerDay != 2 {
		t.Fatalf("unexpected item times per day: %d", plan.Items[0].TimesPerDay)
	}
	if plan.Items[0].Completed {
		t.Fatal("expected item to be not completed after target increase")
	}
}

func TestPickReminderRespectsTimeWindow(t *testing.T) {
	repo := newMemoryRepo()
	svc := New(repo, 30)

	user := seedUser(repo) // UTCOffsetMinutes = 0, so local clock == UTC
	activities := seedActivities(repo, user.ID, "Morning activity", "Unrestricted")

	// Restrict first activity to morning window 08:00-10:00.
	if err := repo.UpdateActivityReminderWindows(context.Background(), user.ID, activities[0].ID, []domain.ReminderWindow{
		{Start: "08:00", End: "10:00"},
	}); err != nil {
		t.Fatalf("set window: %v", err)
	}

	// 05:00 UTC, outside the 08:00-10:00 window.
	now := time.Date(2026, 4, 6, 5, 0, 0, 0, time.UTC)
	if _, err := svc.StartMorningPlan(context.Background(), user.ID, now); err != nil {
		t.Fatalf("start morning plan: %v", err)
	}
	if _, err := svc.FinalizePlan(context.Background(), user.ID, now); err != nil {
		t.Fatalf("finalize plan: %v", err)
	}

	// First reminder tick (31 min after plan start, past the 30-min NextReminderAt).
	item, _, err := svc.PickReminder(context.Background(), user.ID, now.Add(31*time.Minute))
	if err != nil {
		t.Fatalf("pick reminder at 05:31: %v", err)
	}
	// Only "Unrestricted" should be eligible at 05:31.
	if item.ActivityID != activities[1].ID {
		t.Fatalf("expected unrestricted activity, got activity_id=%d", item.ActivityID)
	}

	// Second tick: both "Morning activity" and "Unrestricted" have been reminded (or one was
	// skipped due to window). Re-pick after interval; now at 08:31 — inside the window.
	now2 := time.Date(2026, 4, 6, 8, 31, 0, 0, time.UTC)
	item2, _, err := svc.PickReminder(context.Background(), user.ID, now2)
	if err != nil {
		t.Fatalf("pick reminder at 08:31: %v", err)
	}
	// "Morning activity" window is now open, so windowed activities have priority over unrestricted ones.
	if item2.ActivityID != activities[0].ID {
		t.Fatalf("expected windowed activity, got activity_id=%d", item2.ActivityID)
	}
}

func TestIsInReminderWindow(t *testing.T) {
	tests := []struct {
		windows []domain.ReminderWindow
		clock   string
		want    bool
	}{
		{nil, "10:00", true}, // no restriction
		{[]domain.ReminderWindow{{Start: "09:00", End: "21:00"}}, "09:00", true},
		{[]domain.ReminderWindow{{Start: "09:00", End: "21:00"}}, "20:59", true},
		{[]domain.ReminderWindow{{Start: "09:00", End: "21:00"}}, "21:00", false},
		{[]domain.ReminderWindow{{Start: "09:00", End: "21:00"}}, "08:59", false},
		{[]domain.ReminderWindow{{Start: "22:00", End: "06:00"}}, "23:00", true},
		{[]domain.ReminderWindow{{Start: "22:00", End: "06:00"}}, "05:59", true},
		{[]domain.ReminderWindow{{Start: "22:00", End: "06:00"}}, "06:00", false},
		{[]domain.ReminderWindow{{Start: "22:00", End: "06:00"}}, "10:00", false},
		{
			windows: []domain.ReminderWindow{
				{Start: "08:00", End: "10:00"},
				{Start: "14:00", End: "16:00"},
			},
			clock: "14:30",
			want:  true,
		},
		{
			windows: []domain.ReminderWindow{
				{Start: "22:00", End: "01:00"},
				{Start: "04:00", End: "06:00"},
			},
			clock: "00:30",
			want:  true,
		},
		{
			windows: []domain.ReminderWindow{
				{Start: "22:00", End: "01:00"},
				{Start: "04:00", End: "06:00"},
			},
			clock: "03:00",
			want:  false,
		},
	}
	for _, tt := range tests {
		act := domain.Activity{ReminderWindows: tt.windows}
		got := isInReminderWindow(act, tt.clock)
		if got != tt.want {
			t.Errorf("isInReminderWindow(%+v, %q) = %v, want %v", tt.windows, tt.clock, got, tt.want)
		}
	}
}

func TestLocalDayUsesConfiguredDayEnd(t *testing.T) {
	tests := []struct {
		name      string
		now       time.Time
		offsetMin int
		dayEnd    string
		wantDay   string
	}{
		{
			name:      "default day end at midnight keeps current day",
			now:       time.Date(2026, 4, 6, 0, 30, 0, 0, time.UTC),
			offsetMin: 0,
			dayEnd:    "00:00",
			wantDay:   "2026-04-06",
		},
		{
			name:      "before day end belongs to previous day",
			now:       time.Date(2026, 4, 6, 1, 59, 0, 0, time.UTC),
			offsetMin: 0,
			dayEnd:    "02:00",
			wantDay:   "2026-04-05",
		},
		{
			name:      "at day end switches to current day",
			now:       time.Date(2026, 4, 6, 2, 0, 0, 0, time.UTC),
			offsetMin: 0,
			dayEnd:    "02:00",
			wantDay:   "2026-04-06",
		},
		{
			name:      "applies user utc offset before day end check",
			now:       time.Date(2026, 4, 5, 23, 30, 0, 0, time.UTC), // local 02:30 at UTC+03
			offsetMin: 180,
			dayEnd:    "03:00",
			wantDay:   "2026-04-05",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := localDay(tt.now, tt.offsetMin, tt.dayEnd)
			if got != tt.wantDay {
				t.Fatalf("localDay() = %s, want %s", got, tt.wantDay)
			}
		})
	}
}

func TestFinishDayPausesNotificationsUntilNextLogicalDay(t *testing.T) {
	repo := newMemoryRepo()
	svc := New(repo, 30)
	user := seedUser(repo)

	user.DayEndTime = "03:00"
	user.UTCOffsetMinutes = 180
	repo.users[user.ID] = user

	now := time.Date(2026, 4, 6, 21, 30, 0, 0, time.UTC) // local 2026-04-07 00:30
	pausedUntil, err := svc.FinishDay(context.Background(), user.ID, now)
	if err != nil {
		t.Fatalf("finish day: %v", err)
	}

	want := time.Date(2026, 4, 7, 0, 0, 0, 0, time.UTC) // local 03:00
	if !pausedUntil.Equal(want) {
		t.Fatalf("unexpected pause until: got %v want %v", pausedUntil, want)
	}

	updatedUser, err := repo.GetUserByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("get user by id: %v", err)
	}
	if updatedUser.NotificationsPausedUntil == nil || !updatedUser.NotificationsPausedUntil.Equal(want) {
		t.Fatalf("unexpected stored pause until: %v", updatedUser.NotificationsPausedUntil)
	}
}

func TestNextLogicalDayStartUTC(t *testing.T) {
	tests := []struct {
		name      string
		now       time.Time
		offsetMin int
		dayEnd    string
		want      time.Time
	}{
		{
			name:      "before day end points to same local date boundary",
			now:       time.Date(2026, 4, 6, 1, 59, 0, 0, time.UTC),
			offsetMin: 0,
			dayEnd:    "02:00",
			want:      time.Date(2026, 4, 6, 2, 0, 0, 0, time.UTC),
		},
		{
			name:      "at day end points to next day boundary",
			now:       time.Date(2026, 4, 6, 2, 0, 0, 0, time.UTC),
			offsetMin: 0,
			dayEnd:    "02:00",
			want:      time.Date(2026, 4, 7, 2, 0, 0, 0, time.UTC),
		},
		{
			name:      "respects positive utc offset",
			now:       time.Date(2026, 4, 6, 21, 30, 0, 0, time.UTC), // local 00:30
			offsetMin: 180,
			dayEnd:    "03:00",
			want:      time.Date(2026, 4, 7, 0, 0, 0, 0, time.UTC), // local 03:00
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := nextLogicalDayStartUTC(tt.now, tt.offsetMin, tt.dayEnd)
			if err != nil {
				t.Fatalf("nextLogicalDayStartUTC() error: %v", err)
			}
			if !got.Equal(tt.want) {
				t.Fatalf("nextLogicalDayStartUTC() = %v, want %v", got, tt.want)
			}
		})
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
	reminderMessages       map[int64]map[int]domain.ReminderMessage
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
		reminderMessages:       make(map[int64]map[int]domain.ReminderMessage),
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

func (m *memoryRepo) UpdateUserSettings(_ context.Context, userID int64, morningTime, dayEndTime string, reminderIntervalMinutes int) error {
	user, ok := m.users[userID]
	if !ok {
		return domain.ErrNotFound
	}
	user.MorningTime = morningTime
	user.DayEndTime = dayEndTime
	user.ReminderIntervalMinutes = reminderIntervalMinutes
	m.users[userID] = user
	return nil
}

func (m *memoryRepo) UpdateUserNotificationsPausedUntil(_ context.Context, userID int64, pausedUntil *time.Time) error {
	user, ok := m.users[userID]
	if !ok {
		return domain.ErrNotFound
	}
	user.NotificationsPausedUntil = pausedUntil
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

func (m *memoryRepo) UpdateActivityReminderWindows(_ context.Context, userID, activityID int64, windows []domain.ReminderWindow) error {
	activities := m.activities[userID]
	for i := range activities {
		if activities[i].ID == activityID {
			if len(windows) == 0 {
				activities[i].ReminderWindowStart = ""
				activities[i].ReminderWindowEnd = ""
				activities[i].ReminderWindows = nil
			} else {
				activities[i].ReminderWindowStart = windows[0].Start
				activities[i].ReminderWindowEnd = windows[0].End
				activities[i].ReminderWindows = append([]domain.ReminderWindow(nil), windows...)
			}
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

func (m *memoryRepo) SaveReminderMessage(_ context.Context, message *domain.ReminderMessage) error {
	if m.reminderMessages[message.UserID] == nil {
		m.reminderMessages[message.UserID] = make(map[int]domain.ReminderMessage)
	}
	m.reminderMessages[message.UserID][message.MessageID] = *message
	return nil
}

func (m *memoryRepo) ListReminderMessagesBeforeDay(_ context.Context, userID int64, dayLocal string) ([]domain.ReminderMessage, error) {
	userMessages := m.reminderMessages[userID]
	messages := make([]domain.ReminderMessage, 0, len(userMessages))
	for _, message := range userMessages {
		if message.LogicalDay < dayLocal {
			messages = append(messages, message)
		}
	}
	return messages, nil
}

func (m *memoryRepo) DeleteReminderMessage(_ context.Context, userID int64, messageID int) error {
	if m.reminderMessages[userID] != nil {
		delete(m.reminderMessages[userID], messageID)
	}
	return nil
}

func seedUser(repo *memoryRepo) domain.User {
	user := domain.User{
		TelegramUserID:          1,
		ChatID:                  10,
		Name:                    "Alice",
		UTCOffsetMinutes:        0,
		MorningTime:             "08:00",
		DayEndTime:              "00:00",
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
