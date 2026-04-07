package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"jesterbot/internal/domain"
	"jesterbot/internal/service"
)

type Router struct {
	logger   *slog.Logger
	service  *service.Service
	sessions *SessionStore
	mainMenu models.ReplyMarkup
	bot      *bot.Bot
}

func NewRouter(logger *slog.Logger, token string, pollTimeout time.Duration, workers int, svc *service.Service) (*Router, error) {
	router := &Router{
		logger:   logger,
		service:  svc,
		sessions: NewSessionStore(),
	}

	options := []bot.Option{
		bot.WithDefaultHandler(router.handleDefault),
		bot.WithWorkers(workers),
		bot.WithHTTPClient(pollTimeout, &http.Client{Timeout: pollTimeout + 5*time.Second}),
	}

	b, err := bot.New(token, options...)
	if err != nil {
		return nil, fmt.Errorf("create telegram bot: %w", err)
	}

	router.bot = b
	router.mainMenu = buildMainMenu(b, router)
	router.registerHandlers()
	return router, nil
}

func (r *Router) Start(ctx context.Context) error {
	r.logger.Info("telegram polling started")
	r.bot.Start(ctx)
	r.logger.Info("telegram polling stopped")
	return nil
}

func (r *Router) handleDefault(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.Text == "" {
		return
	}

	chatID := update.Message.Chat.ID
	userText := strings.TrimSpace(update.Message.Text)
	session := r.sessions.Get(chatID)
	if session.State != stateIdle {
		r.logMessageEvent("handling session text input", update.Message, "session_state", session.State, "text_length", len(userText))
	}

	switch session.State {
	case stateRegisterName:
		r.sessions.Update(chatID, func(s *Session) {
			s.State = stateRegisterOffset
			s.Name = userText
		})
		r.sendMessage(ctx, chatID, "🌍 Напиши смещение UTC в формате `+03:00`, `-05:00` или `UTC`.", nil)
	case stateRegisterOffset:
		if _, err := service.ParseUTCOffset(userText); err != nil {
			r.sendMessage(ctx, chatID, "❌ Не смог распознать UTC offset. Пример: `+03:00`.", nil)
			return
		}
		r.sessions.Update(chatID, func(s *Session) {
			s.State = stateRegisterMorning
			s.UTCOffset = userText
		})
		r.sendMessage(ctx, chatID, "🌅 Во сколько начинать утро? Формат `HH:MM`, например `08:30`.", nil)
	case stateRegisterMorning:
		draft := r.sessions.Get(chatID)
		user, err := r.service.RegisterUser(ctx, service.RegistrationInput{
			TelegramUserID: update.Message.From.ID,
			ChatID:         chatID,
			Name:           draft.Name,
			UTCOffset:      draft.UTCOffset,
			MorningTime:    userText,
		})
		if err != nil {
			r.sendMessage(ctx, chatID, "❌ Не получилось завершить регистрацию: "+err.Error(), nil)
			return
		}
		r.sessions.Clear(chatID)
		r.logger.Info("user registered", "user_id", user.ID, "chat_id", chatID, "telegram_user_id", update.Message.From.ID, "utc_offset_minutes", user.UTCOffsetMinutes, "morning_time", user.MorningTime)
		r.sendMessage(ctx, chatID, welcomeText(user), r.mainMenu)
	case stateAddActivity:
		user, err := r.registeredUser(ctx, update.Message.From.ID)
		if err != nil {
			r.handleRegistrationRequired(ctx, chatID)
			return
		}
		if _, err := r.service.AddActivity(ctx, user.ID, userText); err != nil {
			r.sendMessage(ctx, chatID, "❌ Не получилось добавить активность: "+err.Error(), nil)
			return
		}
		r.sessions.Clear(chatID)
		r.showActivities(ctx, chatID, user.ID, "✅ Активность добавлена.")
	case stateEditActivity:
		user, err := r.registeredUser(ctx, update.Message.From.ID)
		if err != nil {
			r.handleRegistrationRequired(ctx, chatID)
			return
		}
		if err := r.service.UpdateActivity(ctx, user.ID, session.EditActivityID, userText); err != nil {
			r.sendMessage(ctx, chatID, "❌ Не получилось обновить активность: "+err.Error(), nil)
			return
		}
		r.sessions.Clear(chatID)
		r.showActivities(ctx, chatID, user.ID, "✅ Название обновлено.")
	case stateUpdateMorning:
		user, err := r.registeredUser(ctx, update.Message.From.ID)
		if err != nil {
			r.handleRegistrationRequired(ctx, chatID)
			return
		}
		if err := r.service.UpdateSettings(ctx, user.ID, userText, user.ReminderIntervalMinutes); err != nil {
			r.sendMessage(ctx, chatID, "❌ Не получилось обновить время утра: "+err.Error(), nil)
			return
		}
		r.sessions.Clear(chatID)
		r.sendMessage(ctx, chatID, "✅ Время утра обновлено.", r.mainMenu)
	case stateUpdateReminder:
		user, err := r.registeredUser(ctx, update.Message.From.ID)
		if err != nil {
			r.handleRegistrationRequired(ctx, chatID)
			return
		}
		minutes, err := strconv.Atoi(userText)
		if err != nil || minutes <= 0 {
			r.sendMessage(ctx, chatID, "❗ Нужны целые минуты, например `30`.", nil)
			return
		}
		if err := r.service.UpdateSettings(ctx, user.ID, user.MorningTime, minutes); err != nil {
			r.sendMessage(ctx, chatID, "❌ Не получилось обновить интервал: "+err.Error(), nil)
			return
		}
		r.sessions.Clear(chatID)
		r.sendMessage(ctx, chatID, "✅ Интервал напоминаний обновлён.", r.mainMenu)
	default:
		r.sendMessage(ctx, chatID, helpText(), r.mainMenu)
	}
}

func (r *Router) handleStart(ctx context.Context, _ *bot.Bot, update *models.Update) {
	r.logMessageEvent("handling start command", update.Message)
	chatID := update.Message.Chat.ID
	user, err := r.service.FindUserByTelegramID(ctx, update.Message.From.ID)
	if err == nil {
		r.sessions.Clear(chatID)
		r.logger.Info("start command for registered user", "user_id", user.ID, "chat_id", chatID)
		r.sendMessage(ctx, chatID, welcomeText(user), r.mainMenu)
		return
	}
	if !errors.Is(err, domain.ErrNotFound) {
		r.sendMessage(ctx, chatID, "❌ Не получилось проверить регистрацию.", nil)
		return
	}

	r.sessions.Update(chatID, func(s *Session) {
		*s = Session{State: stateRegisterName}
	})
	r.sendMessage(ctx, chatID, "👋 Давай зарегистрируемся. Как тебя зовут?", nil)
}

func (r *Router) handleTodayCommand(ctx context.Context, _ *bot.Bot, update *models.Update) {
	r.logMessageEvent("handling today command", update.Message)
	chatID := update.Message.Chat.ID
	user, err := r.registeredUser(ctx, update.Message.From.ID)
	if err != nil {
		r.handleRegistrationRequired(ctx, chatID)
		return
	}

	now := time.Now().UTC()
	plan, err := r.service.GetTodayPlan(ctx, user.ID, now)
	if errors.Is(err, domain.ErrNotFound) {
		plan, err = r.service.StartMorningPlan(ctx, user.ID, now)
	}
	if err != nil {
		r.sendMessage(ctx, chatID, "❌ Не получилось открыть план дня: "+err.Error(), r.mainMenu)
		return
	}

	if plan.Status == domain.PlanStatusAwaitingSelection {
		r.sendMessage(ctx, chatID, selectionText(plan), buildPlanSelectionKeyboard(plan))
		return
	}

	r.sendMessage(ctx, chatID, progressText(plan), buildProgressKeyboard(plan))
}

func (r *Router) handleActivitiesCommand(ctx context.Context, _ *bot.Bot, update *models.Update) {
	r.logMessageEvent("handling activities command", update.Message)
	chatID := update.Message.Chat.ID
	user, err := r.registeredUser(ctx, update.Message.From.ID)
	if err != nil {
		r.handleRegistrationRequired(ctx, chatID)
		return
	}
	r.showActivities(ctx, chatID, user.ID, "🧩 Твои активности.")
}

func (r *Router) handleSettingsCommand(ctx context.Context, _ *bot.Bot, update *models.Update) {
	r.logMessageEvent("handling settings command", update.Message)
	chatID := update.Message.Chat.ID
	_, err := r.registeredUser(ctx, update.Message.From.ID)
	if err != nil {
		r.handleRegistrationRequired(ctx, chatID)
		return
	}
	r.sendMessage(ctx, chatID, "⚙️ Что обновить?", buildSettingsKeyboard())
}

func (r *Router) handleStatsCommand(ctx context.Context, _ *bot.Bot, update *models.Update) {
	r.logMessageEvent("handling stats command", update.Message)
	chatID := update.Message.Chat.ID
	user, err := r.registeredUser(ctx, update.Message.From.ID)
	if err != nil {
		r.handleRegistrationRequired(ctx, chatID)
		return
	}

	stats, err := r.service.BuildStats(ctx, user.ID)
	if err != nil {
		r.sendMessage(ctx, chatID, "❌ Не получилось собрать статистику.", r.mainMenu)
		return
	}

	text := fmt.Sprintf(
		"📊 Статистика:\n- дней с планом: %d\n- завершённых дней: %d\n- выбранных активностей: %d\n- завершённых активностей: %d\n- пропущенных активностей: %d\n- completion rate: %.0f%%",
		stats.DaysWithPlan,
		stats.CompletedDays,
		stats.SelectedActivities,
		stats.CompletedActivities,
		stats.SkippedActivities,
		stats.CompletionRate*100,
	)
	r.sendMessage(ctx, chatID, text, r.mainMenu)
}

func (r *Router) handleActivityCallback(ctx context.Context, _ *bot.Bot, update *models.Update) {
	r.logCallbackEvent("handling activity callback", update.CallbackQuery)
	r.answerCallback(ctx, update.CallbackQuery.ID)
	chatID, userID, messageID := callbackIdentity(update)
	user, err := r.registeredUser(ctx, userID)
	if err != nil {
		r.handleRegistrationRequired(ctx, chatID)
		return
	}

	data := update.CallbackQuery.Data
	switch {
	case data == "activity:add":
		r.sessions.Update(chatID, func(s *Session) {
			*s = Session{State: stateAddActivity}
		})
		r.sendMessage(ctx, chatID, "✍️ Напиши название новой активности.", nil)
	case strings.HasPrefix(data, "activity:edit:"):
		activityID, err := parseID(data)
		if err != nil {
			return
		}
		r.sessions.Update(chatID, func(s *Session) {
			*s = Session{State: stateEditActivity, EditActivityID: activityID}
		})
		r.sendMessage(ctx, chatID, "✍️ Пришли новое название активности.", nil)
	case strings.HasPrefix(data, "activity:delete:"):
		activityID, err := parseID(data)
		if err != nil {
			return
		}
		if err := r.service.DeleteActivity(ctx, user.ID, activityID); err != nil {
			r.sendMessage(ctx, chatID, "❌ Не получилось удалить активность: "+err.Error(), nil)
			return
		}
		r.editMessage(ctx, chatID, messageID, "🗑 Активность удалена.\n\n"+activitiesText(r.mustActivities(ctx, user.ID)), buildActivitiesKeyboard(r.mustActivities(ctx, user.ID)))
	}
}

func (r *Router) handlePlanCallback(ctx context.Context, _ *bot.Bot, update *models.Update) {
	r.logCallbackEvent("handling plan callback", update.CallbackQuery)
	r.answerCallback(ctx, update.CallbackQuery.ID)
	chatID, userID, messageID := callbackIdentity(update)
	user, err := r.registeredUser(ctx, userID)
	if err != nil {
		r.handleRegistrationRequired(ctx, chatID)
		return
	}

	now := time.Now().UTC()
	data := update.CallbackQuery.Data
	switch {
	case strings.HasPrefix(data, "plan:toggle:"):
		activityID, err := parseID(data)
		if err != nil {
			return
		}
		plan, err := r.service.TogglePlanItem(ctx, user.ID, activityID, now)
		if err != nil {
			r.sendMessage(ctx, chatID, "❌ Не получилось обновить выбор: "+err.Error(), nil)
			return
		}
		r.logger.Info("plan item toggled", "user_id", user.ID, "chat_id", chatID, "activity_id", activityID, "selected_count", countSelectedItems(plan))
		r.editMessage(ctx, chatID, messageID, selectionText(plan), buildPlanSelectionKeyboard(plan))
	case data == "plan:finalize":
		plan, err := r.service.FinalizePlan(ctx, user.ID, now)
		if err != nil {
			r.sendMessage(ctx, chatID, "❌ Не получилось зафиксировать план: "+err.Error(), nil)
			return
		}
		r.logger.Info("plan finalized", "user_id", user.ID, "chat_id", chatID, "selected_count", countSelectedItems(plan), "completed_count", countCompletedItems(plan))
		r.editMessage(ctx, chatID, messageID, progressText(plan), buildProgressKeyboard(plan))
	case data == "plan:all":
		plan, err := r.service.SelectAllAndFinalize(ctx, user.ID, now)
		if err != nil {
			r.sendMessage(ctx, chatID, "❌ Не получилось выбрать все активности: "+err.Error(), nil)
			return
		}
		r.logger.Info("plan finalized with all activities", "user_id", user.ID, "chat_id", chatID, "selected_count", countSelectedItems(plan))
		r.editMessage(ctx, chatID, messageID, progressText(plan), buildProgressKeyboard(plan))
	}
}

func (r *Router) handleDoneCallback(ctx context.Context, _ *bot.Bot, update *models.Update) {
	r.logCallbackEvent("handling done callback", update.CallbackQuery)
	r.answerCallback(ctx, update.CallbackQuery.ID)
	chatID, userID, messageID := callbackIdentity(update)
	user, err := r.registeredUser(ctx, userID)
	if err != nil {
		r.handleRegistrationRequired(ctx, chatID)
		return
	}

	activityID, err := parseID(update.CallbackQuery.Data)
	if err != nil {
		return
	}

	plan, err := r.service.MarkActivityDone(ctx, user.ID, activityID, time.Now().UTC())
	if err != nil {
		r.sendMessage(ctx, chatID, "❌ Не получилось отметить активность: "+err.Error(), nil)
		return
	}

	r.logger.Info("activity marked done", "user_id", user.ID, "chat_id", chatID, "activity_id", activityID, "completed_count", countCompletedItems(plan))
	r.editMessage(ctx, chatID, messageID, progressText(plan), buildProgressKeyboard(plan))
	if plan.Status == domain.PlanStatusCompleted {
		r.logger.Info("day plan completed", "user_id", user.ID, "chat_id", chatID, "day", plan.DayLocal)
		r.sendMessage(ctx, chatID, completionMessage(plan), r.mainMenu)
	}
}

func (r *Router) handleSettingsCallback(ctx context.Context, _ *bot.Bot, update *models.Update) {
	r.logCallbackEvent("handling settings callback", update.CallbackQuery)
	r.answerCallback(ctx, update.CallbackQuery.ID)
	chatID, _, _ := callbackIdentity(update)

	switch update.CallbackQuery.Data {
	case "settings:morning":
		r.sessions.Update(chatID, func(s *Session) {
			*s = Session{State: stateUpdateMorning}
		})
		r.sendMessage(ctx, chatID, "⏰ Пришли новое время утра в формате `HH:MM`.", nil)
	case "settings:interval":
		r.sessions.Update(chatID, func(s *Session) {
			*s = Session{State: stateUpdateReminder}
		})
		r.sendMessage(ctx, chatID, "🔁 Пришли новый интервал напоминаний в минутах, например `30`.", nil)
	}
}

func (r *Router) registerHandlers() {
	r.bot.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypeExact, r.handleStart)
	r.bot.RegisterHandler(bot.HandlerTypeMessageText, "/help", bot.MatchTypeExact, r.handleStart)
	r.bot.RegisterHandler(bot.HandlerTypeCallbackQueryData, "activity:", bot.MatchTypePrefix, r.handleActivityCallback)
	r.bot.RegisterHandler(bot.HandlerTypeCallbackQueryData, "plan:", bot.MatchTypePrefix, r.handlePlanCallback)
	r.bot.RegisterHandler(bot.HandlerTypeCallbackQueryData, "done:", bot.MatchTypePrefix, r.handleDoneCallback)
	r.bot.RegisterHandler(bot.HandlerTypeCallbackQueryData, "settings:", bot.MatchTypePrefix, r.handleSettingsCallback)
}

func (r *Router) registeredUser(ctx context.Context, telegramUserID int64) (*domain.User, error) {
	return r.service.FindUserByTelegramID(ctx, telegramUserID)
}

func (r *Router) showActivities(ctx context.Context, chatID, userID int64, prefix string) {
	activities, err := r.service.ListActivities(ctx, userID)
	if err != nil {
		r.sendMessage(ctx, chatID, "❌ Не получилось получить список активностей.", r.mainMenu)
		return
	}
	r.sendMessage(ctx, chatID, prefix+"\n\n"+activitiesText(activities), buildActivitiesKeyboard(activities))
}

func (r *Router) mustActivities(ctx context.Context, userID int64) []domain.Activity {
	activities, err := r.service.ListActivities(ctx, userID)
	if err != nil {
		r.logger.Error("list activities failed", "error", err, "user_id", userID)
		return nil
	}
	return activities
}

func (r *Router) handleRegistrationRequired(ctx context.Context, chatID int64) {
	r.sessions.Update(chatID, func(s *Session) {
		*s = Session{State: stateRegisterName}
	})
	r.sendMessage(ctx, chatID, "📝 Сначала нужна регистрация. Как тебя зовут?", nil)
}

func (r *Router) sendMessage(ctx context.Context, chatID int64, text string, markup models.ReplyMarkup) {
	params := &bot.SendMessageParams{ChatID: chatID, Text: text}
	if markup != nil {
		params.ReplyMarkup = markup
	}
	if _, err := r.bot.SendMessage(ctx, params); err != nil {
		r.logger.Error("send message failed", "error", err, "chat_id", chatID)
	}
}

func (r *Router) editMessage(ctx context.Context, chatID int64, messageID int, text string, markup models.ReplyMarkup) {
	params := &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: messageID,
		Text:      text,
	}
	if markup != nil {
		params.ReplyMarkup = markup
	}
	if _, err := r.bot.EditMessageText(ctx, params); err != nil {
		r.logger.Error("edit message failed", "error", err, "chat_id", chatID, "message_id", messageID)
	}
}

func (r *Router) answerCallback(ctx context.Context, callbackID string) {
	if _, err := r.bot.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: callbackID}); err != nil {
		r.logger.Error("answer callback failed", "error", err)
	}
}

func callbackIdentity(update *models.Update) (chatID int64, userID int64, messageID int) {
	return update.CallbackQuery.Message.Message.Chat.ID, update.CallbackQuery.From.ID, update.CallbackQuery.Message.Message.ID
}

func (r *Router) logMessageEvent(event string, message *models.Message, attrs ...any) {
	args := []any{"chat_id", message.Chat.ID, "telegram_user_id", message.From.ID}
	r.logger.Info(event, append(args, attrs...)...)
}

func (r *Router) logCallbackEvent(event string, callback *models.CallbackQuery, attrs ...any) {
	args := []any{
		"chat_id", callback.Message.Message.Chat.ID,
		"telegram_user_id", callback.From.ID,
		"message_id", callback.Message.Message.ID,
		"callback_data", callback.Data,
	}
	r.logger.Info(event, append(args, attrs...)...)
}

func countSelectedItems(plan *domain.DayPlan) int {
	count := 0
	for _, item := range plan.Items {
		if item.Selected {
			count++
		}
	}
	return count
}

func countCompletedItems(plan *domain.DayPlan) int {
	count := 0
	for _, item := range plan.Items {
		if item.Completed {
			count++
		}
	}
	return count
}

func parseID(data string) (int64, error) {
	parts := strings.Split(data, ":")
	if len(parts) == 0 {
		return 0, fmt.Errorf("invalid callback data: %s", data)
	}
	return strconv.ParseInt(parts[len(parts)-1], 10, 64)
}

func welcomeText(user *domain.User) string {
	return fmt.Sprintf(
		"👋 Привет, %s.\nЯ помогу вести утренние активности, план дня и напоминания.\nИспользуй меню ниже или команду /today.",
		user.Name,
	)
}

func helpText() string {
	return "Доступно меню: 📅 Сегодня, 🧩 Активности, ⚙️ Настройки, 📊 Статистика. Для первого запуска используй /start."
}

func activitiesText(activities []domain.Activity) string {
	if len(activities) == 0 {
		return "🗒 Список пуст. Добавь первую активность."
	}

	lines := make([]string, 0, len(activities)+1)
	lines = append(lines, "🧩 Текущий список:")
	for i, activity := range activities {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, activity.Title))
	}
	return strings.Join(lines, "\n")
}

func selectionText(plan *domain.DayPlan) string {
	lines := []string{
		"✅ Отметь, что сегодня делать не будешь.",
		"📌 По умолчанию выбрано всё.",
	}
	for _, item := range plan.Items {
		status := "делаю"
		if !item.Selected {
			status = "пропускаю"
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", item.TitleSnapshot, status))
	}
	return strings.Join(lines, "\n")
}

func progressText(plan *domain.DayPlan) string {
	completed := make([]string, 0)
	remaining := make([]string, 0)
	skipped := make([]string, 0)
	for _, item := range plan.Items {
		switch {
		case !item.Selected:
			skipped = append(skipped, item.TitleSnapshot)
		case item.Completed:
			completed = append(completed, item.TitleSnapshot)
		default:
			remaining = append(remaining, item.TitleSnapshot)
		}
	}

	lines := []string{
		fmt.Sprintf("📍 Статус дня: %s", string(plan.Status)),
		fmt.Sprintf("✅ Готово: %d", len(completed)),
		fmt.Sprintf("⏳ Осталось: %d", len(remaining)),
	}
	if len(completed) > 0 {
		lines = append(lines, "🎯 Сделано: "+strings.Join(completed, ", "))
	}
	if len(remaining) > 0 {
		lines = append(lines, "📌 Осталось: "+strings.Join(remaining, ", "))
	}
	if len(skipped) > 0 {
		lines = append(lines, "⏭ Пропуск: "+strings.Join(skipped, ", "))
	}
	return strings.Join(lines, "\n")
}
