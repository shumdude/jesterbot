package telegram

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"

	"jesterbot/internal/domain"
)

func TestBuildActivitiesKeyboardAddsBackButton(t *testing.T) {
	markup := buildActivitiesKeyboard([]domain.Activity{{ID: 7, Title: "Walk"}})
	inline, ok := markup.(*models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("expected inline keyboard, got %T", markup)
	}

	lastRow := inline.InlineKeyboard[len(inline.InlineKeyboard)-1]
	if len(lastRow) != 1 || lastRow[0].CallbackData != "menu:back" {
		t.Fatalf("expected back button row, got %+v", lastRow)
	}
}

func TestBuildActivitiesKeyboardPageUsesSingleOpenButtonPerActivity(t *testing.T) {
	activities := make([]domain.Activity, 0, 13)
	for i := 1; i <= 13; i++ {
		activities = append(activities, domain.Activity{ID: int64(i), Title: "Task"})
	}

	markup := buildActivitiesKeyboardPage(activities, 1, 12)
	inline, ok := markup.(*models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("expected inline keyboard, got %T", markup)
	}

	firstRow := inline.InlineKeyboard[0]
	if len(firstRow) != 1 {
		t.Fatalf("expected single button row, got %+v", firstRow)
	}
	if firstRow[0].CallbackData != "activity:open:13:1" {
		t.Fatalf("expected open callback to keep page, got %+v", firstRow[0])
	}

	pagerRow := inline.InlineKeyboard[1]
	if pagerRow[len(pagerRow)-1].CallbackData != noopCallbackData {
		t.Fatalf("expected page indicator row, got %+v", pagerRow)
	}
}

func TestBuildActivityDetailKeyboardUsesPageAwareCallbacks(t *testing.T) {
	markup := buildActivityDetailKeyboard(domain.Activity{
		ID:                  42,
		Title:               "Read",
		TimesPerDay:         3,
		ReminderWindowStart: "08:00",
		ReminderWindowEnd:   "22:00",
	}, 2)
	inline, ok := markup.(*models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("expected inline keyboard, got %T", markup)
	}

	if inline.InlineKeyboard[0][0].CallbackData != "activity:times:42:2" {
		t.Fatalf("expected times callback, got %+v", inline.InlineKeyboard[0][0])
	}
	if inline.InlineKeyboard[1][0].CallbackData != "activity:window:42:2" {
		t.Fatalf("expected window callback, got %+v", inline.InlineKeyboard[1][0])
	}
	if inline.InlineKeyboard[2][0].CallbackData != "activity:delete:42:2" {
		t.Fatalf("expected delete callback, got %+v", inline.InlineKeyboard[2][0])
	}
	if inline.InlineKeyboard[3][0].CallbackData != "activity:list:2" {
		t.Fatalf("expected list callback, got %+v", inline.InlineKeyboard[3][0])
	}
	if inline.InlineKeyboard[4][0].CallbackData != "menu:back" {
		t.Fatalf("expected main menu callback, got %+v", inline.InlineKeyboard[4][0])
	}
}

func TestTodayPlanErrorTextForNoActivities(t *testing.T) {
	text := todayPlanErrorText(domain.ErrNoActivities)
	if strings.Contains(text, domain.ErrNoActivities.Error()) {
		t.Fatalf("expected friendly text without raw domain error, got %q", text)
	}
	if !strings.Contains(text, "Список активностей пока пуст") {
		t.Fatalf("expected empty activities hint, got %q", text)
	}
}

func TestStatsTextUsesRussianLabels(t *testing.T) {
	text := statsText(domain.DailyStats{CompletionRate: 0.5, OneOffCompletionRate: 0.25})
	if strings.Contains(text, "completion rate") {
		t.Fatalf("expected russian labels, got %q", text)
	}
	if !strings.Contains(text, "Jester: статистика") {
		t.Fatalf("expected updated header, got %q", text)
	}
	if !strings.Contains(text, "0/0 (0%)") {
		t.Fatalf("expected ratio-based summary, got %q", text)
	}
}

func TestProgressTextTranslatesStatusAndHidesSkipped(t *testing.T) {
	text := progressText(&domain.DayPlan{
		Status: domain.PlanStatusActive,
		Items: []domain.DayPlanItem{
			{TitleSnapshot: "Stretch", Selected: true, Completed: true},
			{TitleSnapshot: "Read", Selected: true, Completed: false},
			{TitleSnapshot: "Walk", Selected: false, Completed: false},
		},
	})

	if strings.Contains(text, "active") {
		t.Fatalf("expected translated status, got %q", text)
	}
	if !strings.Contains(text, "в процессе") {
		t.Fatalf("expected translated status label, got %q", text)
	}
	if strings.Contains(text, "Пропуск") {
		t.Fatalf("expected skipped block to be hidden, got %q", text)
	}
	if !strings.Contains(text, "1/2 (50%)") {
		t.Fatalf("expected combined progress summary, got %q", text)
	}
	if !strings.Contains(text, "<b>Осталось:</b>\n▪️ Read") {
		t.Fatalf("expected remaining items as multiline decorated list, got %q", text)
	}
	if !strings.Contains(text, "<b>Прогресс: 1/2 (50%)</b>") {
		t.Fatalf("expected bold progress line, got %q", text)
	}
}

func TestReminderTextShowsOnlyCurrentActivity(t *testing.T) {
	text := reminderText(&domain.DayPlanItem{TitleSnapshot: "РџРѕС‡РёСЃС‚РёС‚СЊ Р·СѓР±С‹"}, &domain.DayPlan{
		Items: []domain.DayPlanItem{
			{TitleSnapshot: "РџРѕС‡РёСЃС‚РёС‚СЊ Р·СѓР±С‹", Selected: true},
			{TitleSnapshot: "Р—Р°СЂСЏРґРєР°", Selected: true},
		},
	})

	expected := "🔔 Напоминание.\n👉 Сейчас лучше сделать: РџРѕС‡РёСЃС‚РёС‚СЊ Р·СѓР±С‹"
	if text != expected {
		t.Fatalf("expected compact reminder text %q, got %q", expected, text)
	}
}

func TestOneOffTasksTextAndKeyboardHideHistoryFromMenu(t *testing.T) {
	now := time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC)
	tasks := []domain.OneOffTask{
		{ID: 1, Title: "Pay bill", Priority: domain.OneOffTaskPriorityHigh, Status: domain.OneOffTaskStatusActive},
		{ID: 2, Title: "Archive notes", Priority: domain.OneOffTaskPriorityLow, Status: domain.OneOffTaskStatusCompleted, CompletedAt: &now},
	}

	text := oneOffTasksText(tasks)
	if strings.Contains(text, "Archive notes") {
		t.Fatalf("expected completed task to be removed from active list, got %q", text)
	}
	if strings.Contains(text, "История дел") {
		t.Fatalf("expected history to be absent from one-off menu, got %q", text)
	}

	markup := buildOneOffTasksKeyboard(tasks)
	inline, ok := markup.(*models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("expected inline keyboard, got %T", markup)
	}

	for _, row := range inline.InlineKeyboard {
		for _, button := range row {
			if strings.Contains(button.Text, "Archive notes") {
				t.Fatalf("expected completed task to be absent from main keyboard, got %+v", button)
			}
			if button.CallbackData == "oneoff:history" {
				t.Fatalf("expected history button to be removed, got %+v", button)
			}
		}
	}
}

func TestOneOffTasksTextPageShowsOnlyCurrentSlice(t *testing.T) {
	tasks := make([]domain.OneOffTask, 0, 13)
	for i := 1; i <= 13; i++ {
		tasks = append(tasks, domain.OneOffTask{
			ID:       int64(i),
			Title:    fmt.Sprintf("Task %d", i),
			Priority: domain.OneOffTaskPriorityMedium,
			Status:   domain.OneOffTaskStatusActive,
		})
	}

	text := oneOffTasksTextPage(tasks, 1, 12)
	if !strings.Contains(text, "Страница 2/2") {
		t.Fatalf("expected page summary, got %q", text)
	}
	if strings.Contains(text, "\n1. рџџЁ Task 1 ") {
		t.Fatalf("expected first page task to be hidden, got %q", text)
	}
	if !strings.Contains(text, "13. 🟨 Task 13") {
		t.Fatalf("expected second page task numbering, got %q", text)
	}
}

func TestBuildPlanSelectionKeyboardPageUsesPageAwareCallbacks(t *testing.T) {
	plan := &domain.DayPlan{
		Items: make([]domain.DayPlanItem, 0, 13),
	}
	for i := 1; i <= 13; i++ {
		plan.Items = append(plan.Items, domain.DayPlanItem{
			ActivityID:    int64(i),
			TitleSnapshot: fmt.Sprintf("Task %d", i),
			Selected:      true,
		})
	}

	markup := buildPlanSelectionKeyboardPage(plan, 1, 12)
	inline, ok := markup.(*models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("expected inline keyboard, got %T", markup)
	}

	firstRow := inline.InlineKeyboard[0]
	if firstRow[0].CallbackData != "plan:toggle:13:1" {
		t.Fatalf("expected paged toggle callback, got %+v", firstRow[0])
	}

	lastRow := inline.InlineKeyboard[len(inline.InlineKeyboard)-1]
	if lastRow[0].CallbackData != "menu:back" {
		t.Fatalf("expected main menu callback, got %+v", lastRow[0])
	}
}

func TestBuildProgressKeyboardAlwaysIncludesMainMenuBack(t *testing.T) {
	markup := buildProgressKeyboard(&domain.DayPlan{
		Items: []domain.DayPlanItem{
			{ActivityID: 1, TitleSnapshot: "Stretch", Selected: true, Completed: true},
		},
	})
	inline, ok := markup.(*models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("expected inline keyboard, got %T", markup)
	}

	if len(inline.InlineKeyboard) != 1 {
		t.Fatalf("expected only main menu row, got %+v", inline.InlineKeyboard)
	}
	if inline.InlineKeyboard[0][0].CallbackData != "menu:back" {
		t.Fatalf("expected main menu callback, got %+v", inline.InlineKeyboard[0][0])
	}
}

func TestBuildSettingsKeyboardIncludesMainMenuBack(t *testing.T) {
	markup := buildSettingsKeyboard()
	inline, ok := markup.(*models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("expected inline keyboard, got %T", markup)
	}

	lastRow := inline.InlineKeyboard[len(inline.InlineKeyboard)-1]
	if lastRow[0].CallbackData != "menu:back" {
		t.Fatalf("expected main menu callback, got %+v", lastRow[0])
	}
}

func TestBuildOneOffKeyboardsIncludeMainMenuBack(t *testing.T) {
	listMarkup := buildOneOffTasksKeyboard([]domain.OneOffTask{{ID: 1, Title: "Pay bill", Priority: domain.OneOffTaskPriorityMedium, Status: domain.OneOffTaskStatusActive}})
	listInline, ok := listMarkup.(*models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("expected inline keyboard, got %T", listMarkup)
	}
	if listInline.InlineKeyboard[len(listInline.InlineKeyboard)-1][0].CallbackData != "menu:back" {
		t.Fatalf("expected one-off list main menu callback, got %+v", listInline.InlineKeyboard[len(listInline.InlineKeyboard)-1][0])
	}

	priorityMarkup := buildOneOffPriorityKeyboard()
	priorityInline, ok := priorityMarkup.(*models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("expected inline keyboard, got %T", priorityMarkup)
	}
	if priorityInline.InlineKeyboard[len(priorityInline.InlineKeyboard)-1][0].CallbackData != "menu:back" {
		t.Fatalf("expected one-off priority main menu callback, got %+v", priorityInline.InlineKeyboard[len(priorityInline.InlineKeyboard)-1][0])
	}

	detailMarkup := buildOneOffTaskDetailKeyboard(&domain.OneOffTask{
		ID:       1,
		Title:    "Pay bill",
		Priority: domain.OneOffTaskPriorityMedium,
		Status:   domain.OneOffTaskStatusActive,
	})
	detailInline, ok := detailMarkup.(*models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("expected inline keyboard, got %T", detailMarkup)
	}
	if detailInline.InlineKeyboard[len(detailInline.InlineKeyboard)-1][0].CallbackData != "menu:back" {
		t.Fatalf("expected one-off detail main menu callback, got %+v", detailInline.InlineKeyboard[len(detailInline.InlineKeyboard)-1][0])
	}
}

func TestSettingsTextShowsCurrentUserSettings(t *testing.T) {
	text := settingsText(&domain.User{
		MorningTime:             "08:30",
		ReminderIntervalMinutes: 45,
		UTCOffsetMinutes:        180,
	}, 2, &domain.OneOffReminderSettings{
		LowPriorityMinutes:    120,
		MediumPriorityMinutes: 60,
		HighPriorityMinutes:   15,
	})

	if !strings.Contains(text, "Русский") {
		t.Fatalf("expected language in settings text, got %q", text)
	}
	if !strings.Contains(text, "UTC+03:00") {
		t.Fatalf("expected timezone in settings text, got %q", text)
	}
	if !strings.Contains(text, "Время утра: 08:30") {
		t.Fatalf("expected morning time in settings text, got %q", text)
	}
	if !strings.Contains(text, "Частота проверки: 2 мин") {
		t.Fatalf("expected tick interval in settings text, got %q", text)
	}
	if !strings.Contains(text, "низкий 120 мин, средний 60 мин, высокий 15 мин") {
		t.Fatalf("expected one-off reminder settings in settings text, got %q", text)
	}
}

func TestTelegramHTTPClientTimeoutAddsSafetyBuffer(t *testing.T) {
	got := telegramHTTPClientTimeout(10 * time.Second)
	if got < time.Minute {
		t.Fatalf("expected at least one minute timeout, got %s", got)
	}
}
