package telegram

import (
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
	if len(lastRow) != 1 || lastRow[0].CallbackData != "activity:back" {
		t.Fatalf("expected back button row, got %+v", lastRow)
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
	if !strings.Contains(text, "процент выполнения") {
		t.Fatalf("expected russian completion label, got %q", text)
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
}

func TestOneOffTasksTextAndKeyboardHideCompletedTasksBehindHistory(t *testing.T) {
	now := time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC)
	tasks := []domain.OneOffTask{
		{ID: 1, Title: "Pay bill", Priority: domain.OneOffTaskPriorityHigh, Status: domain.OneOffTaskStatusActive},
		{ID: 2, Title: "Archive notes", Priority: domain.OneOffTaskPriorityLow, Status: domain.OneOffTaskStatusCompleted, CompletedAt: &now},
	}

	text := oneOffTasksText(tasks)
	if strings.Contains(text, "Archive notes") {
		t.Fatalf("expected completed task to be removed from active list, got %q", text)
	}
	if !strings.Contains(text, "История дел") {
		t.Fatalf("expected history summary, got %q", text)
	}

	markup := buildOneOffTasksKeyboard(tasks)
	inline, ok := markup.(*models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("expected inline keyboard, got %T", markup)
	}

	var hasHistoryButton bool
	for _, row := range inline.InlineKeyboard {
		for _, button := range row {
			if button.CallbackData == "oneoff:history" {
				hasHistoryButton = true
			}
			if strings.Contains(button.Text, "Archive notes") {
				t.Fatalf("expected completed task to be absent from main keyboard, got %+v", button)
			}
		}
	}
	if !hasHistoryButton {
		t.Fatal("expected history button for completed tasks")
	}
}

func TestOneOffHistoryTextListsCompletedTasks(t *testing.T) {
	text := oneOffHistoryText([]domain.OneOffTask{
		{Title: "Archive notes", Priority: domain.OneOffTaskPriorityLow, Status: domain.OneOffTaskStatusCompleted},
	})
	if !strings.Contains(text, "Archive notes") {
		t.Fatalf("expected completed task in history text, got %q", text)
	}
}

func TestTelegramHTTPClientTimeoutAddsSafetyBuffer(t *testing.T) {
	got := telegramHTTPClientTimeout(10 * time.Second)
	if got < time.Minute {
		t.Fatalf("expected at least one minute timeout, got %s", got)
	}
}
