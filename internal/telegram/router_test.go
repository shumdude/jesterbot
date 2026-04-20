package telegram

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	tgamlengine "github.com/shumdude/tgaml/pkg/engine"

	"jesterbot/cmd/jesterbot/botconfig"
	"jesterbot/internal/domain"
	"jesterbot/internal/service"
	"jesterbot/internal/telegram/constants"
	"jesterbot/internal/telegram/session_backend"
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

func TestBuildActivityTimesKeyboardUsesDraftCallbacks(t *testing.T) {
	markup := buildActivityTimesKeyboard(42, 2, 1)
	inline, ok := markup.(*models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("expected inline keyboard, got %T", markup)
	}

	if inline.InlineKeyboard[0][0].CallbackData != "activity:times:set:42:2:1" {
		t.Fatalf("expected clamped decrease callback, got %+v", inline.InlineKeyboard[0][0])
	}
	if inline.InlineKeyboard[0][1].CallbackData != "activity:times:set:42:2:2" {
		t.Fatalf("expected increase callback, got %+v", inline.InlineKeyboard[0][1])
	}
	if inline.InlineKeyboard[1][0].CallbackData != "activity:times:confirm:42:2:1" {
		t.Fatalf("expected confirm callback, got %+v", inline.InlineKeyboard[1][0])
	}
	if inline.InlineKeyboard[2][0].CallbackData != "activity:open:42:2" {
		t.Fatalf("expected back to activity callback, got %+v", inline.InlineKeyboard[2][0])
	}
	if inline.InlineKeyboard[3][0].CallbackData != "menu:back" {
		t.Fatalf("expected main menu callback, got %+v", inline.InlineKeyboard[3][0])
	}
}

func TestActivitiesTextPageShowsTimesPerDayInList(t *testing.T) {
	text := activitiesTextPage([]domain.Activity{
		{ID: 1, Title: "Read", TimesPerDay: 3},
		{ID: 2, Title: "Walk", TimesPerDay: 0},
	}, 0, defaultInlinePageSize)

	if !strings.Contains(text, "1. Read (3x)") {
		t.Fatalf("expected first activity count in list text, got %q", text)
	}
	if !strings.Contains(text, "2. Walk (1x)") {
		t.Fatalf("expected clamped activity count in list text, got %q", text)
	}
}

func TestActivityDetailTextShowsTimesInTextBlock(t *testing.T) {
	text := activityDetailText("Prefix", domain.Activity{
		ID:          1,
		Title:       "Read",
		TimesPerDay: 0,
	})

	if !strings.Contains(text, "Количество в день: 1x") {
		t.Fatalf("expected times-per-day in detail text, got %q", text)
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

func TestSendOneOffReminderSendsMessageWithoutReplyMarkup(t *testing.T) {
	const (
		token  = "test-token"
		chatID = int64(456)
	)

	var (
		receivedPayload     string
		receivedReplyMarkup bool
	)
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/bot" + token + "/getMe":
			_, _ = rw.Write([]byte(`{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"test","username":"test_bot"}}`))
		case "/bot" + token + "/sendMessage":
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("read sendMessage request body: %v", err)
			}
			if err := req.Body.Close(); err != nil {
				t.Fatalf("close sendMessage request body: %v", err)
			}
			trimmed := strings.TrimSpace(string(body))
			decoded, err := url.QueryUnescape(trimmed)
			if err != nil {
				t.Fatalf("decode sendMessage payload: %v", err)
			}
			receivedPayload = decoded
			receivedReplyMarkup = strings.Contains(trimmed, "reply_markup") || strings.Contains(decoded, "reply_markup")
			_, _ = rw.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":456,"type":"private"}}}`))
		default:
			t.Fatalf("unexpected request path: %s", req.URL.Path)
		}
	}))
	defer server.Close()

	tgBot, err := bot.New(token, bot.WithServerURL(server.URL))
	if err != nil {
		t.Fatalf("create test telegram bot: %v", err)
	}

	controller := &Controller{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		bot:    tgBot,
	}

	task := &domain.OneOffTask{
		ID:    11,
		Title: "Pay bill",
		Items: []domain.OneOffTaskItem{
			{ID: 101, Title: "Open bank app"},
		},
	}
	controller.SendOneOffReminder(context.Background(), 1, chatID, "2026-04-10", task)

	if receivedReplyMarkup {
		t.Fatal("expected one-off reminder message without reply_markup")
	}
	if !strings.Contains(receivedPayload, task.Title) {
		t.Fatalf("expected reminder payload to mention task title, got %q", receivedPayload)
	}
}

func TestOneOffNoItemsHandlerCreatesTaskWithoutChecklist(t *testing.T) {
	const (
		token          = "test-token"
		telegramUserID = int64(777001)
		chatID         = int64(9001)
	)

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/bot" + token + "/getMe":
			_, _ = rw.Write([]byte(`{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"test","username":"test_bot"}}`))
		case "/bot" + token + "/sendMessage":
			_, _ = rw.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":9001,"type":"private"}}}`))
		default:
			t.Fatalf("unexpected request path: %s", req.URL.Path)
		}
	}))
	defer server.Close()

	tgBot, err := bot.New(token, bot.WithServerURL(server.URL))
	if err != nil {
		t.Fatalf("create test telegram bot: %v", err)
	}

	cfg, err := botconfig.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	repo := &oneOffHandlerRepo{
		menuMarkupRepo: menuMarkupRepo{
			userByTelegramID: map[int64]*domain.User{
				telegramUserID: {
					ID:             42,
					TelegramUserID: telegramUserID,
					ChatID:         chatID,
					Name:           "Alexey",
				},
			},
		},
	}
	svc := service.New(repo, 30)
	eng := tgamlengine.New(cfg, session_backend.New(constants.SceneMenu))

	controller := &Controller{
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		service: svc,
		eng:     eng,
		bot:     tgBot,
	}

	session := eng.NewSession(telegramUserID, chatID)
	if err := session.SetStr(constants.NSOneOff, constants.KeyTaskTitle, "Pay bill"); err != nil {
		t.Fatalf("set one-off title: %v", err)
	}
	if err := session.SetStr(constants.NSOneOff, constants.KeyPriority, string(domain.OneOffTaskPriorityHigh)); err != nil {
		t.Fatalf("set one-off priority: %v", err)
	}

	if _, err := oneOffNoItemsHandler(svc, controller)(context.Background(), tgBot, nil, session); err != nil {
		t.Fatalf("run no-items handler: %v", err)
	}

	if len(repo.tasks) != 1 {
		t.Fatalf("expected exactly one created task, got %+v", repo.tasks)
	}
	if repo.tasks[0].Title != "Pay bill" {
		t.Fatalf("expected task title to be preserved, got %+v", repo.tasks[0])
	}
	if len(repo.tasks[0].Items) != 0 {
		t.Fatalf("expected task without checklist items, got %+v", repo.tasks[0].Items)
	}
}

type oneOffHandlerRepo struct {
	menuMarkupRepo
	tasks                  []domain.OneOffTask
	oneOffReminderSettings *domain.OneOffReminderSettings
}

func (r *oneOffHandlerRepo) GetOneOffReminderSettings(context.Context, int64) (*domain.OneOffReminderSettings, error) {
	if r.oneOffReminderSettings == nil {
		return nil, domain.ErrNotFound
	}
	return r.oneOffReminderSettings, nil
}

func (r *oneOffHandlerRepo) SaveOneOffReminderSettings(_ context.Context, settings *domain.OneOffReminderSettings) error {
	copied := *settings
	r.oneOffReminderSettings = &copied
	return nil
}

func (r *oneOffHandlerRepo) ListOneOffTasks(context.Context, int64) ([]domain.OneOffTask, error) {
	tasks := make([]domain.OneOffTask, len(r.tasks))
	copy(tasks, r.tasks)
	return tasks, nil
}

func (r *oneOffHandlerRepo) SaveOneOffTask(_ context.Context, task *domain.OneOffTask) error {
	copied := *task
	if copied.ID == 0 {
		copied.ID = int64(len(r.tasks) + 1)
	}
	if len(copied.Items) > 0 {
		items := make([]domain.OneOffTaskItem, len(copied.Items))
		copy(items, copied.Items)
		copied.Items = items
	}
	task.ID = copied.ID
	r.tasks = append(r.tasks, copied)
	return nil
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

func TestParseIDPageCallbackSupportsDoneButtons(t *testing.T) {
	activityID, page, err := parseIDPageCallback("done:42:3")
	if err != nil {
		t.Fatalf("expected done callback to parse, got %v", err)
	}
	if activityID != 42 {
		t.Fatalf("expected activity id 42, got %d", activityID)
	}
	if page != 3 {
		t.Fatalf("expected page 3, got %d", page)
	}
}

func TestParseActivityTimesCallbackParsesAndClampsTimes(t *testing.T) {
	activityID, page, timesPerDay, err := parseActivityTimesCallback("activity:times:confirm:42:3:0", "confirm")
	if err != nil {
		t.Fatalf("expected activity times callback to parse, got %v", err)
	}
	if activityID != 42 {
		t.Fatalf("expected activity id 42, got %d", activityID)
	}
	if page != 3 {
		t.Fatalf("expected page 3, got %d", page)
	}
	if timesPerDay != 1 {
		t.Fatalf("expected times per day to be clamped to 1, got %d", timesPerDay)
	}
}

func TestParseWindowInputSupportsMultipleRanges(t *testing.T) {
	windows, err := parseWindowInput("08:00-10:00, 14:00-16:00")
	if err != nil {
		t.Fatalf("expected comma-separated windows to parse, got %v", err)
	}
	if len(windows) != 2 {
		t.Fatalf("expected 2 windows, got %+v", windows)
	}
	if windows[0].Start != "08:00" || windows[0].End != "10:00" {
		t.Fatalf("unexpected first window: %+v", windows[0])
	}
	if windows[1].Start != "14:00" || windows[1].End != "16:00" {
		t.Fatalf("unexpected second window: %+v", windows[1])
	}

	windows, err = parseWindowInput("22:00-01:00;\n04:00-06:00")
	if err != nil {
		t.Fatalf("expected semicolon/newline-separated windows to parse, got %v", err)
	}
	if len(windows) != 2 {
		t.Fatalf("expected 2 windows with semicolon/newline, got %+v", windows)
	}
	if windows[0].Start != "22:00" || windows[0].End != "01:00" {
		t.Fatalf("unexpected midnight-crossing window: %+v", windows[0])
	}
	if windows[1].Start != "04:00" || windows[1].End != "06:00" {
		t.Fatalf("unexpected second window: %+v", windows[1])
	}
}

func TestParseWindowInputRejectsInvalidRanges(t *testing.T) {
	if _, err := parseWindowInput("09:00-09:00"); err == nil {
		t.Fatal("expected equal start/end to fail")
	}
	if _, err := parseWindowInput("09:00-12:00, bad"); err == nil {
		t.Fatal("expected invalid segment to fail")
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

	hasDayEnd := false
	hasClearChat := false
	for _, row := range inline.InlineKeyboard {
		for _, button := range row {
			if button.CallbackData == "settings:day_end" {
				hasDayEnd = true
			}
			if button.CallbackData == "settings:clear_chat" {
				hasClearChat = true
			}
		}
	}
	if !hasDayEnd {
		t.Fatalf("expected day end settings button, got %+v", inline.InlineKeyboard)
	}
	if !hasClearChat {
		t.Fatalf("expected clear chat settings button, got %+v", inline.InlineKeyboard)
	}

	lastRow := inline.InlineKeyboard[len(inline.InlineKeyboard)-1]
	if lastRow[0].CallbackData != "menu:back" {
		t.Fatalf("expected main menu callback, got %+v", lastRow[0])
	}
}

func TestBuildStatsKeyboardIncludesMainMenuBack(t *testing.T) {
	markup := buildStatsKeyboard()
	inline, ok := markup.(*models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("expected inline keyboard, got %T", markup)
	}

	if len(inline.InlineKeyboard) != 1 {
		t.Fatalf("expected single row, got %+v", inline.InlineKeyboard)
	}
	if inline.InlineKeyboard[0][0].CallbackData != "menu:back" {
		t.Fatalf("expected main menu callback, got %+v", inline.InlineKeyboard[0][0])
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
		DayEndTime:              "02:00",
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
	if !strings.Contains(text, "02:00") {
		t.Fatalf("expected day end time in settings text, got %q", text)
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

func TestParseOneOffReminderSettingsInputSupportsUnits(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		low    int
		medium int
		high   int
	}{
		{
			name:   "plain minutes",
			input:  "60,30,10",
			low:    60,
			medium: 30,
			high:   10,
		},
		{
			name:   "mixed day and hour units",
			input:  "3d,1d,12h",
			low:    3 * 24 * 60,
			medium: 24 * 60,
			high:   12 * 60,
		},
		{
			name:   "mixed day and minute units with separators",
			input:  " 3D ; 1d ; 30m ",
			low:    3 * 24 * 60,
			medium: 24 * 60,
			high:   30,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			low, medium, high, err := parseOneOffReminderSettingsInput(tc.input)
			if err != nil {
				t.Fatalf("parse one-off reminder settings: %v", err)
			}
			if low != tc.low || medium != tc.medium || high != tc.high {
				t.Fatalf("unexpected parsed values: got (%d,%d,%d), want (%d,%d,%d)", low, medium, high, tc.low, tc.medium, tc.high)
			}
		})
	}
}

func TestParseOneOffReminderSettingsInputRejectsInvalidValues(t *testing.T) {
	inputs := []string{
		"3d,1d",
		"3w,1d,30m",
		"d,1d,30m",
		"3d,0,10m",
	}

	for _, input := range inputs {
		if _, _, _, err := parseOneOffReminderSettingsInput(input); err == nil {
			t.Fatalf("expected parsing to fail for %q", input)
		}
	}
}
