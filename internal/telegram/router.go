package telegram

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"math"
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

	httpTimeout := telegramHTTPClientTimeout(pollTimeout)
	options := []bot.Option{
		bot.WithDefaultHandler(router.handleDefault),
		bot.WithWorkers(workers),
		bot.WithHTTPClient(pollTimeout, &http.Client{Timeout: httpTimeout}),
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
	r.cleanupUserMessage(ctx, update.Message)
	userText := strings.TrimSpace(update.Message.Text)
	// Text input is interpreted through per-chat state machine stored in SessionStore.
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
		r.showScreen(ctx, chatID, tr("register_prompt_offset"), nil)
	case stateRegisterOffset:
		if _, err := service.ParseUTCOffset(userText); err != nil {
			r.showScreen(ctx, chatID, tr("register_error_offset"), nil)
			return
		}
		r.sessions.Update(chatID, func(s *Session) {
			s.State = stateRegisterMorning
			s.UTCOffset = userText
		})
		r.showScreen(ctx, chatID, tr("register_prompt_morning"), nil)
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
			r.showScreen(ctx, chatID, tr("register_error_finish", err.Error()), nil)
			return
		}
		r.sessions.Clear(chatID)
		r.logger.Info("user registered", "user_id", user.ID, "chat_id", chatID, "telegram_user_id", update.Message.From.ID, "utc_offset_minutes", user.UTCOffsetMinutes, "morning_time", user.MorningTime)
		r.showScreen(ctx, chatID, welcomeText(user), r.mainMenu)
	case stateAddActivity:
		user, err := r.registeredUser(ctx, update.Message.From.ID)
		if err != nil {
			r.handleRegistrationRequired(ctx, chatID)
			return
		}
		activities, err := r.service.AddActivities(ctx, user.ID, userText)
		if err != nil {
			r.showScreen(ctx, chatID, tr("activity_error_add", err.Error()), nil)
			return
		}
		r.sessions.Clear(chatID)
		prefix := tr("activity_success_add_one")
		if len(activities) > 1 {
			prefix = tr("activity_success_add_many", len(activities))
		}
		r.showActivities(ctx, chatID, user.ID, prefix)
	case stateAddOneOffTitle:
		if strings.TrimSpace(userText) == "" {
			r.showScreen(ctx, chatID, tr("oneoff_error_empty_title"), nil)
			return
		}
		r.sessions.Update(chatID, func(s *Session) {
			s.State = stateAddOneOffTitle
			s.OneOffTaskTitle = userText
		})
		r.showScreen(ctx, chatID, tr("oneoff_prompt_priority"), buildOneOffPriorityKeyboard())
	case stateAddOneOffItems:
		user, err := r.registeredUser(ctx, update.Message.From.ID)
		if err != nil {
			r.handleRegistrationRequired(ctx, chatID)
			return
		}
		draft := r.sessions.Get(chatID)
		task, err := r.service.CreateOneOffTask(ctx, user.ID, draft.OneOffTaskTitle, draft.OneOffTaskPriority, parseOneOffChecklistInput(userText))
		if err != nil {
			r.showScreen(ctx, chatID, tr("oneoff_error_create", err.Error()), nil)
			return
		}
		r.sessions.Clear(chatID)
		r.showOneOffTasks(ctx, chatID, user.ID, tr("oneoff_success_create", task.Title))
	case stateEditActivity:
		user, err := r.registeredUser(ctx, update.Message.From.ID)
		if err != nil {
			r.handleRegistrationRequired(ctx, chatID)
			return
		}
		if err := r.service.UpdateActivity(ctx, user.ID, session.EditActivityID, userText); err != nil {
			r.showScreen(ctx, chatID, tr("activity_error_update", err.Error()), nil)
			return
		}
		r.sessions.Clear(chatID)
		r.showActivities(ctx, chatID, user.ID, tr("activity_success_update"))
	case stateUpdateMorning:
		user, err := r.registeredUser(ctx, update.Message.From.ID)
		if err != nil {
			r.handleRegistrationRequired(ctx, chatID)
			return
		}
		if err := r.service.UpdateSettings(ctx, user.ID, userText, user.ReminderIntervalMinutes); err != nil {
			r.showScreen(ctx, chatID, tr("settings_error_update_morning", err.Error()), nil)
			return
		}
		r.sessions.Clear(chatID)
		r.showSettings(ctx, chatID, update.Message.From.ID, tr("settings_success_morning"))
	case stateUpdateReminder:
		user, err := r.registeredUser(ctx, update.Message.From.ID)
		if err != nil {
			r.handleRegistrationRequired(ctx, chatID)
			return
		}
		minutes, err := strconv.Atoi(userText)
		if err != nil || minutes <= 0 {
			r.showScreen(ctx, chatID, tr("settings_error_invalid_minutes"), nil)
			return
		}
		if err := r.service.UpdateSettings(ctx, user.ID, user.MorningTime, minutes); err != nil {
			r.showScreen(ctx, chatID, tr("settings_error_update_interval", err.Error()), nil)
			return
		}
		r.sessions.Clear(chatID)
		r.showSettings(ctx, chatID, update.Message.From.ID, tr("settings_success_interval"))
	case stateUpdateOneOffReminder:
		user, err := r.registeredUser(ctx, update.Message.From.ID)
		if err != nil {
			r.handleRegistrationRequired(ctx, chatID)
			return
		}
		low, medium, high, err := parseOneOffReminderSettingsInput(userText)
		if err != nil {
			r.showScreen(ctx, chatID, tr("settings_error_invalid_oneoff"), nil)
			return
		}
		if err := r.service.UpdateOneOffReminderSettings(ctx, user.ID, low, medium, high); err != nil {
			r.showScreen(ctx, chatID, tr("settings_error_update_oneoff", err.Error()), nil)
			return
		}
		r.sessions.Clear(chatID)
		r.showSettings(ctx, chatID, update.Message.From.ID, tr("settings_success_oneoff"))
	case stateUpdateTickInterval:
		user, err := r.registeredUser(ctx, update.Message.From.ID)
		if err != nil {
			r.handleRegistrationRequired(ctx, chatID)
			return
		}
		minutes, err := strconv.Atoi(userText)
		if err != nil || minutes <= 0 {
			r.showScreen(ctx, chatID, tr("settings_error_invalid_tick"), nil)
			return
		}
		if err := r.service.UpdateUserTickInterval(ctx, user.ID, minutes); err != nil {
			r.showScreen(ctx, chatID, tr("settings_error_update_tick", err.Error()), nil)
			return
		}
		r.sessions.Clear(chatID)
		r.showSettings(ctx, chatID, update.Message.From.ID, tr("settings_success_tick"))
	default:
		r.showScreen(ctx, chatID, helpText(), r.mainMenu)
	}
}

func (r *Router) handleStart(ctx context.Context, _ *bot.Bot, update *models.Update) {
	r.logMessageEvent("handling start command", update.Message)
	chatID := update.Message.Chat.ID
	r.cleanupUserMessage(ctx, update.Message)
	user, err := r.service.FindUserByTelegramID(ctx, update.Message.From.ID)
	if err == nil {
		r.sessions.Clear(chatID)
		r.logger.Info("start command for registered user", "user_id", user.ID, "chat_id", chatID)
		r.showScreen(ctx, chatID, welcomeText(user), r.mainMenu)
		return
	}
	if !errors.Is(err, domain.ErrNotFound) {
		r.showScreen(ctx, chatID, tr("register_error_check"), nil)
		return
	}

	r.sessions.Update(chatID, func(s *Session) {
		s.resetForState(stateRegisterName)
	})
	r.showScreen(ctx, chatID, tr("register_prompt_name"), nil)
}

func (r *Router) handleTodayCommand(ctx context.Context, _ *bot.Bot, update *models.Update) {
	r.logMessageEvent("handling today command", update.Message)
	chatID := update.Message.Chat.ID
	r.cleanupUserMessage(ctx, update.Message)
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
		r.showScreen(ctx, chatID, todayPlanErrorText(err), r.mainMenu)
		return
	}

	if plan.Status == domain.PlanStatusAwaitingSelection {
		r.showScreen(ctx, chatID, selectionTextPage(plan, 0, defaultInlinePageSize), buildPlanSelectionKeyboardPage(plan, 0, defaultInlinePageSize))
		return
	}

	r.showScreen(ctx, chatID, progressTextPage(plan, 0, defaultInlinePageSize), buildProgressKeyboardPage(plan, 0, defaultInlinePageSize))
}

func (r *Router) handleActivitiesCommand(ctx context.Context, _ *bot.Bot, update *models.Update) {
	r.logMessageEvent("handling activities command", update.Message)
	chatID := update.Message.Chat.ID
	r.cleanupUserMessage(ctx, update.Message)
	user, err := r.registeredUser(ctx, update.Message.From.ID)
	if err != nil {
		r.handleRegistrationRequired(ctx, chatID)
		return
	}
	r.showActivitiesPage(ctx, chatID, user.ID, tr("activity_title"), 0)
}

func (r *Router) handleSettingsCommand(ctx context.Context, _ *bot.Bot, update *models.Update) {
	r.logMessageEvent("handling settings command", update.Message)
	chatID := update.Message.Chat.ID
	r.cleanupUserMessage(ctx, update.Message)
	_, err := r.registeredUser(ctx, update.Message.From.ID)
	if err != nil {
		r.handleRegistrationRequired(ctx, chatID)
		return
	}
	r.showSettings(ctx, chatID, update.Message.From.ID, tr("settings_title"))
}

func (r *Router) handleStatsCommand(ctx context.Context, _ *bot.Bot, update *models.Update) {
	r.logMessageEvent("handling stats command", update.Message)
	chatID := update.Message.Chat.ID
	r.cleanupUserMessage(ctx, update.Message)
	user, err := r.registeredUser(ctx, update.Message.From.ID)
	if err != nil {
		r.handleRegistrationRequired(ctx, chatID)
		return
	}

	stats, err := r.service.BuildStats(ctx, user.ID)
	if err != nil {
		r.showScreen(ctx, chatID, tr("stats_error_build"), r.mainMenu)
		return
	}

	r.showScreen(ctx, chatID, statsText(stats), r.mainMenu)
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
	case data == "activity:back":
		r.showScreenFromCallback(ctx, chatID, messageID, tr("activity_back_to_menu"), emptyInlineKeyboard())
	case strings.HasPrefix(data, "activity:page:"):
		page, err := parsePageCallback(data)
		if err != nil {
			return
		}
		r.showActivitiesPageAsEdit(ctx, chatID, messageID, user.ID, tr("activity_title"), page)
	case data == "activity:add":
		r.sessions.Update(chatID, func(s *Session) {
			s.resetForState(stateAddActivity)
		})
		r.showScreenFromCallback(ctx, chatID, messageID, tr("activity_prompt_add"), nil)
	case strings.HasPrefix(data, "activity:edit:"):
		activityID, err := parseID(data)
		if err != nil {
			return
		}
		r.sessions.Update(chatID, func(s *Session) {
			s.resetForState(stateEditActivity)
			s.EditActivityID = activityID
		})
		r.showScreenFromCallback(ctx, chatID, messageID, tr("activity_prompt_edit"), nil)
	case strings.HasPrefix(data, "activity:delete:"):
		activityID, page, err := parseIDPageCallback(data)
		if err != nil {
			return
		}
		if err := r.service.DeleteActivity(ctx, user.ID, activityID); err != nil {
			r.showScreenFromCallback(ctx, chatID, messageID, tr("activity_error_delete", err.Error()), nil)
			return
		}
		r.showActivitiesPageAsEdit(ctx, chatID, messageID, user.ID, tr("activity_success_delete"), page)
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
	case strings.HasPrefix(data, "plan:page:"):
		page, err := parsePageCallback(data)
		if err != nil {
			return
		}
		plan, err := r.service.GetTodayPlan(ctx, user.ID, now)
		if err != nil {
			r.showScreenFromCallback(ctx, chatID, messageID, todayPlanErrorText(err), r.mainMenu)
			return
		}
		if plan.Status == domain.PlanStatusAwaitingSelection {
			r.showScreenFromCallback(ctx, chatID, messageID, selectionTextPage(plan, page, defaultInlinePageSize), buildPlanSelectionKeyboardPage(plan, page, defaultInlinePageSize))
			return
		}
		r.showScreenFromCallback(ctx, chatID, messageID, progressTextPage(plan, page, defaultInlinePageSize), buildProgressKeyboardPage(plan, page, defaultInlinePageSize))
	case strings.HasPrefix(data, "plan:toggle:"):
		activityID, page, err := parseIDPageCallback(data)
		if err != nil {
			return
		}
		plan, err := r.service.TogglePlanItem(ctx, user.ID, activityID, now)
		if err != nil {
			r.showScreenFromCallback(ctx, chatID, messageID, tr("today_error_toggle", err.Error()), nil)
			return
		}
		r.logger.Info("plan item toggled", "user_id", user.ID, "chat_id", chatID, "activity_id", activityID, "selected_count", countSelectedItems(plan))
		r.showScreenFromCallback(ctx, chatID, messageID, selectionTextPage(plan, page, defaultInlinePageSize), buildPlanSelectionKeyboardPage(plan, page, defaultInlinePageSize))
	case data == "plan:finalize":
		plan, err := r.service.FinalizePlan(ctx, user.ID, now)
		if err != nil {
			r.showScreenFromCallback(ctx, chatID, messageID, tr("today_error_finalize", err.Error()), nil)
			return
		}
		r.logger.Info("plan finalized", "user_id", user.ID, "chat_id", chatID, "selected_count", countSelectedItems(plan), "completed_count", countCompletedItems(plan))
		r.showScreenFromCallback(ctx, chatID, messageID, progressTextPage(plan, 0, defaultInlinePageSize), buildProgressKeyboardPage(plan, 0, defaultInlinePageSize))
	case data == "plan:all":
		plan, err := r.service.SelectAllAndFinalize(ctx, user.ID, now)
		if err != nil {
			r.showScreenFromCallback(ctx, chatID, messageID, tr("today_error_all", err.Error()), nil)
			return
		}
		r.logger.Info("plan finalized with all activities", "user_id", user.ID, "chat_id", chatID, "selected_count", countSelectedItems(plan))
		r.showScreenFromCallback(ctx, chatID, messageID, progressTextPage(plan, 0, defaultInlinePageSize), buildProgressKeyboardPage(plan, 0, defaultInlinePageSize))
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

	activityID, page, err := parseIDPageCallback(update.CallbackQuery.Data)
	if err != nil {
		return
	}

	plan, err := r.service.MarkActivityDone(ctx, user.ID, activityID, time.Now().UTC())
	if err != nil {
		r.showScreenFromCallback(ctx, chatID, messageID, tr("today_error_done", err.Error()), nil)
		return
	}

	r.logger.Info("activity marked done", "user_id", user.ID, "chat_id", chatID, "activity_id", activityID, "completed_count", countCompletedItems(plan))
	r.showScreenFromCallback(ctx, chatID, messageID, progressTextPage(plan, page, defaultInlinePageSize), buildProgressKeyboardPage(plan, page, defaultInlinePageSize))
	if plan.Status == domain.PlanStatusCompleted {
		r.logger.Info("day plan completed", "user_id", user.ID, "chat_id", chatID, "day", plan.DayLocal)
		r.showScreenFromCallback(ctx, chatID, messageID, completionMessage(plan), r.mainMenu)
	}
}

func (r *Router) handleSettingsCallback(ctx context.Context, _ *bot.Bot, update *models.Update) {
	r.logCallbackEvent("handling settings callback", update.CallbackQuery)
	r.answerCallback(ctx, update.CallbackQuery.ID)
	chatID, _, _ := callbackIdentity(update)

	switch update.CallbackQuery.Data {
	case "settings:morning":
		r.sessions.Update(chatID, func(s *Session) {
			s.resetForState(stateUpdateMorning)
		})
		r.showScreenFromCallback(ctx, chatID, update.CallbackQuery.Message.Message.ID, tr("settings_prompt_morning"), nil)
	case "settings:interval":
		r.sessions.Update(chatID, func(s *Session) {
			s.resetForState(stateUpdateReminder)
		})
		r.showScreenFromCallback(ctx, chatID, update.CallbackQuery.Message.Message.ID, tr("settings_prompt_interval"), nil)
	case "settings:tick":
		user, err := r.registeredUser(ctx, update.CallbackQuery.From.ID)
		if err != nil {
			r.handleRegistrationRequired(ctx, chatID)
			return
		}
		minutes, err := r.service.GetUserTickInterval(ctx, user.ID)
		if err != nil {
			r.showScreenFromCallback(ctx, chatID, update.CallbackQuery.Message.Message.ID, tr("settings_error_tick_get"), nil)
			return
		}
		r.sessions.Update(chatID, func(s *Session) {
			s.resetForState(stateUpdateTickInterval)
		})
		r.showScreenFromCallback(ctx, chatID, update.CallbackQuery.Message.Message.ID, tr("settings_prompt_tick", minutes), nil)
	case "settings:oneoff":
		user, err := r.registeredUser(ctx, update.CallbackQuery.From.ID)
		if err != nil {
			r.handleRegistrationRequired(ctx, chatID)
			return
		}
		settings, err := r.service.GetOneOffReminderSettings(ctx, user.ID)
		if err != nil {
			r.showScreenFromCallback(ctx, chatID, update.CallbackQuery.Message.Message.ID, tr("settings_error_oneoff_get"), nil)
			return
		}
		r.sessions.Update(chatID, func(s *Session) {
			s.resetForState(stateUpdateOneOffReminder)
		})
		r.showScreenFromCallback(ctx, chatID, update.CallbackQuery.Message.Message.ID, oneOffReminderSettingsPrompt(settings), nil)
	}
}

func (r *Router) registerHandlers() {
	r.bot.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypeExact, r.handleStart)
	r.bot.RegisterHandler(bot.HandlerTypeMessageText, "/help", bot.MatchTypeExact, r.handleStart)
	r.bot.RegisterHandler(bot.HandlerTypeMessageText, tr("main_menu_oneoff"), bot.MatchTypeExact, r.handleOneOffTasksCommand)
	r.bot.RegisterHandler(bot.HandlerTypeCallbackQueryData, "activity:", bot.MatchTypePrefix, r.handleActivityCallback)
	r.bot.RegisterHandler(bot.HandlerTypeCallbackQueryData, "plan:", bot.MatchTypePrefix, r.handlePlanCallback)
	r.bot.RegisterHandler(bot.HandlerTypeCallbackQueryData, "done:", bot.MatchTypePrefix, r.handleDoneCallback)
	r.bot.RegisterHandler(bot.HandlerTypeCallbackQueryData, "oneoff:", bot.MatchTypePrefix, r.handleOneOffCallback)
	r.bot.RegisterHandler(bot.HandlerTypeCallbackQueryData, "settings:", bot.MatchTypePrefix, r.handleSettingsCallback)
	r.bot.RegisterHandler(bot.HandlerTypeCallbackQueryData, noopCallbackData, bot.MatchTypeExact, r.handleNoopCallback)
}

func (r *Router) registeredUser(ctx context.Context, telegramUserID int64) (*domain.User, error) {
	return r.service.FindUserByTelegramID(ctx, telegramUserID)
}

func (r *Router) showActivities(ctx context.Context, chatID, userID int64, prefix string) {
	r.showActivitiesPage(ctx, chatID, userID, prefix, 0)
}

func (r *Router) showActivitiesPage(ctx context.Context, chatID, userID int64, prefix string, page int) {
	activities, err := r.service.ListActivities(ctx, userID)
	if err != nil {
		r.showScreen(ctx, chatID, tr("activity_error_list"), r.mainMenu)
		return
	}
	r.showScreen(ctx, chatID, prefix+"\n\n"+activitiesTextPage(activities, page, defaultInlinePageSize), buildActivitiesKeyboardPage(activities, page, defaultInlinePageSize))
}

func (r *Router) showActivitiesPageAsEdit(ctx context.Context, chatID int64, messageID int, userID int64, prefix string, page int) {
	activities, err := r.service.ListActivities(ctx, userID)
	if err != nil {
		r.showScreenFromCallback(ctx, chatID, messageID, tr("activity_error_list"), r.mainMenu)
		return
	}
	r.showScreenFromCallback(ctx, chatID, messageID, prefix+"\n\n"+activitiesTextPage(activities, page, defaultInlinePageSize), buildActivitiesKeyboardPage(activities, page, defaultInlinePageSize))
}

func (r *Router) showSettings(ctx context.Context, chatID, telegramUserID int64, prefix string) {
	user, err := r.registeredUser(ctx, telegramUserID)
	if err != nil {
		r.handleRegistrationRequired(ctx, chatID)
		return
	}

	tickMinutes, err := r.service.GetUserTickInterval(ctx, user.ID)
	if err != nil {
		r.showScreen(ctx, chatID, tr("settings_error_tick_get"), nil)
		return
	}
	oneOffSettings, err := r.service.GetOneOffReminderSettings(ctx, user.ID)
	if err != nil {
		r.showScreen(ctx, chatID, tr("settings_error_oneoff_get"), nil)
		return
	}

	r.showScreen(ctx, chatID, prefix+"\n\n"+settingsText(user, tickMinutes, oneOffSettings), buildSettingsKeyboard())
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
		s.resetForState(stateRegisterName)
	})
	r.showScreen(ctx, chatID, tr("register_required"), nil)
}

func (r *Router) sendMessage(ctx context.Context, chatID int64, text string, markup models.ReplyMarkup) int {
	params := &bot.SendMessageParams{ChatID: chatID, Text: text}
	if usesHTMLParseMode(text) {
		params.ParseMode = models.ParseModeHTML
	}
	if markup != nil {
		params.ReplyMarkup = markup
	}
	message, err := r.bot.SendMessage(ctx, params)
	if err != nil {
		r.logger.Error("send message failed", "error", err, "chat_id", chatID)
		return 0
	}
	if message == nil {
		return 0
	}
	return message.ID
}

func (r *Router) editMessage(ctx context.Context, chatID int64, messageID int, text string, markup models.ReplyMarkup) bool {
	params := &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: messageID,
		Text:      text,
	}
	if usesHTMLParseMode(text) {
		params.ParseMode = models.ParseModeHTML
	}
	if markup != nil {
		params.ReplyMarkup = markup
	}
	if _, err := r.bot.EditMessageText(ctx, params); err != nil {
		r.logger.Error("edit message failed", "error", err, "chat_id", chatID, "message_id", messageID)
		return false
	}
	return true
}

func (r *Router) deleteMessage(ctx context.Context, chatID int64, messageID int) bool {
	if messageID == 0 {
		return false
	}
	if _, err := r.bot.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: chatID, MessageID: messageID}); err != nil {
		r.logger.Error("delete message failed", "error", err, "chat_id", chatID, "message_id", messageID)
		return false
	}
	return true
}

func (r *Router) showScreen(ctx context.Context, chatID int64, text string, markup models.ReplyMarkup) {
	r.renderScreen(ctx, chatID, 0, text, markup)
}

func (r *Router) showScreenFromCallback(ctx context.Context, chatID int64, currentMessageID int, text string, markup models.ReplyMarkup) {
	r.renderScreen(ctx, chatID, currentMessageID, text, markup)
}

func (r *Router) renderScreen(ctx context.Context, chatID int64, currentMessageID int, text string, markup models.ReplyMarkup) {
	session := r.sessions.Get(chatID)
	targetMessageID := session.ActiveMessageID
	if targetMessageID == 0 {
		targetMessageID = currentMessageID
	}

	switch session.messageMode() {
	case uiMessageModeEdit:
		if targetMessageID != 0 && r.editMessage(ctx, chatID, targetMessageID, text, markup) {
			r.setActiveMessage(chatID, targetMessageID)
			return
		}
	case uiMessageModeDelete:
		if targetMessageID != 0 {
			r.deleteMessage(ctx, chatID, targetMessageID)
			r.clearActiveMessage(chatID, targetMessageID)
		}
	}

	newMessageID := r.sendMessage(ctx, chatID, text, markup)
	r.setActiveMessage(chatID, newMessageID)
}

func (r *Router) cleanupUserMessage(ctx context.Context, message *models.Message) {
	if message == nil || r.sessions.Get(message.Chat.ID).messageMode() == uiMessageModeNormal {
		return
	}
	r.deleteMessage(ctx, message.Chat.ID, message.ID)
}

func (r *Router) setActiveMessage(chatID int64, messageID int) {
	if messageID == 0 {
		return
	}
	r.sessions.Update(chatID, func(s *Session) {
		s.ActiveMessageID = messageID
	})
}

func (r *Router) clearActiveMessage(chatID int64, messageID int) {
	r.sessions.Update(chatID, func(s *Session) {
		if messageID == 0 || s.ActiveMessageID == messageID {
			s.ActiveMessageID = 0
		}
	})
}

func usesHTMLParseMode(text string) bool {
	return strings.Contains(text, "<b>") || strings.Contains(text, "</b>")
}

func (r *Router) answerCallback(ctx context.Context, callbackID string) {
	if _, err := r.bot.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: callbackID}); err != nil {
		r.logger.Error("answer callback failed", "error", err)
	}
}

func (r *Router) handleNoopCallback(ctx context.Context, _ *bot.Bot, update *models.Update) {
	r.answerCallback(ctx, update.CallbackQuery.ID)
}

func callbackIdentity(update *models.Update) (chatID int64, userID int64, messageID int) {
	// For callback queries, ownership checks and message edits rely on the tuple
	// (chat id, telegram user id, message id).
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

func telegramHTTPClientTimeout(pollTimeout time.Duration) time.Duration {
	httpTimeout := pollTimeout + 30*time.Second
	if httpTimeout < time.Minute {
		return time.Minute
	}
	return httpTimeout
}

func emptyInlineKeyboard() models.ReplyMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{}}
}

func parseID(data string) (int64, error) {
	parts := strings.Split(data, ":")
	if len(parts) == 0 {
		return 0, fmt.Errorf("invalid callback data: %s", data)
	}
	return strconv.ParseInt(parts[len(parts)-1], 10, 64)
}

func welcomeText(user *domain.User) string {
	return tr("welcome_text", user.Name)
}

func helpText() string {
	return tr("help_text")
}

func activitiesText(activities []domain.Activity) string {
	return activitiesTextPage(activities, 0, defaultInlinePageSize)
}

func activitiesTextPage(activities []domain.Activity, page, pageSize int) string {
	if len(activities) == 0 {
		return tr("activity_list_empty")
	}

	view := paginate(activities, page, pageSize)
	lines := make([]string, 0, len(view.Items)+2)
	lines = append(lines, tr("activity_list_title"))
	if view.TotalPages > 1 {
		lines = append(lines, pageSummary(view.Page, view.TotalPages, view.Start, view.End, view.TotalItems))
	}
	for i, activity := range view.Items {
		lines = append(lines, fmt.Sprintf("%d. %s", view.Start+i+1, activity.Title))
	}
	return strings.Join(lines, "\n")
}

func selectionText(plan *domain.DayPlan) string {
	return selectionTextPage(plan, 0, defaultInlinePageSize)
}

func selectionTextPage(plan *domain.DayPlan, page, pageSize int) string {
	view := paginate(plan.Items, page, pageSize)
	lines := []string{
		tr("today_selection_intro"),
		tr("today_selection_default"),
	}
	if view.TotalPages > 1 {
		lines = append(lines, pageSummary(view.Page, view.TotalPages, view.Start, view.End, view.TotalItems))
	}
	for _, item := range view.Items {
		status := tr("today_selection_status_do")
		if !item.Selected {
			status = tr("today_selection_status_skip")
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", item.TitleSnapshot, status))
	}
	return strings.Join(lines, "\n")
}

func todayPlanErrorText(err error) string {
	if errors.Is(err, domain.ErrNoActivities) {
		return tr("today_error_no_activities")
	}
	return tr("today_error_open", err.Error())
}

func statsText(stats domain.DailyStats) string {
	lines := []string{
		tr("stats_header"),
		tr("stats_days", progressRatio(stats.CompletedDays, stats.DaysWithPlan)),
		tr("stats_days_with_plan", stats.DaysWithPlan),
		tr("stats_streak", stats.CurrentCompletedStreak),
		"",
		tr("stats_activities", progressRatio(stats.CompletedActivities, stats.SelectedActivities)),
		tr("stats_selected", stats.SelectedActivities),
		tr("stats_completed", stats.CompletedActivities),
		tr("stats_skipped", stats.SkippedActivities),
		"",
		tr("stats_oneoff", progressRatio(stats.CompletedOneOffTasks, stats.OneOffTasks)),
		tr("stats_total", stats.OneOffTasks),
		tr("stats_pending", stats.PendingOneOffTasks),
		tr("stats_oneoff_completed", stats.CompletedOneOffTasks),
		"",
		tr("stats_oneoff_checklist", progressRatio(stats.CompletedOneOffChecklistItems, stats.OneOffChecklistItems)),
		tr("stats_oneoff_checklist_total", stats.OneOffChecklistItems),
		tr("stats_oneoff_checklist_completed", stats.CompletedOneOffChecklistItems),
	}

	return strings.Join(lines, "\n")
}

func planStatusLabel(status domain.PlanStatus) string {
	switch status {
	case domain.PlanStatusAwaitingSelection:
		return tr("today_status_awaiting_selection")
	case domain.PlanStatusActive:
		return tr("today_status_active")
	case domain.PlanStatusCompleted:
		return tr("today_status_completed")
	default:
		return string(status)
	}
}

func progressText(plan *domain.DayPlan) string {
	return progressTextPage(plan, 0, defaultInlinePageSize)
}

func progressTextPage(plan *domain.DayPlan, page, pageSize int) string {
	allSelected := make([]domain.DayPlanItem, 0, len(plan.Items))
	completed := make([]string, 0)
	remaining := make([]string, 0)
	for _, item := range plan.Items {
		if item.Selected {
			allSelected = append(allSelected, item)
		}
	}

	view := paginate(allSelected, page, pageSize)
	for _, item := range view.Items {
		switch {
		case item.Completed:
			completed = append(completed, html.EscapeString(item.TitleSnapshot))
		default:
			remaining = append(remaining, html.EscapeString(item.TitleSnapshot))
		}
	}

	lines := []string{
		tr("today_status_line", html.EscapeString(planStatusLabel(plan.Status))),
		tr("today_progress_line", progressRatio(countCompletedItems(plan), countSelectedItems(plan))),
	}
	if view.TotalPages > 1 {
		lines = append(lines, pageSummary(view.Page, view.TotalPages, view.Start, view.End, view.TotalItems))
	}
	if len(completed) > 0 {
		lines = append(lines, decoratedLines(tr("today_done_title"), completed)...)
	}
	if len(remaining) > 0 {
		lines = append(lines, decoratedLines(tr("today_remaining_title"), remaining)...)
	}
	return strings.Join(lines, "\n")
}

func settingsText(user *domain.User, tickMinutes int, oneOffSettings *domain.OneOffReminderSettings) string {
	lines := []string{
		tr("settings_summary_language", tr("language_ru")),
		tr("settings_summary_timezone", formatUTCOffset(user.UTCOffsetMinutes)),
		tr("settings_summary_morning", user.MorningTime),
		tr("settings_summary_interval", user.ReminderIntervalMinutes),
		tr("settings_summary_tick", tickMinutes),
		tr(
			"settings_summary_oneoff",
			oneOffSettings.LowPriorityMinutes,
			oneOffSettings.MediumPriorityMinutes,
			oneOffSettings.HighPriorityMinutes,
		),
	}

	return strings.Join(lines, "\n")
}

func formatUTCOffset(offsetMinutes int) string {
	sign := "+"
	if offsetMinutes < 0 {
		sign = "-"
		offsetMinutes = -offsetMinutes
	}

	hours := offsetMinutes / 60
	minutes := offsetMinutes % 60
	return fmt.Sprintf("UTC%s%02d:%02d", sign, hours, minutes)
}

func progressRatio(done, total int) string {
	return fmt.Sprintf("%d/%d (%d%%)", done, total, roundedPercent(done, total))
}

func roundedPercent(done, total int) int {
	if total <= 0 {
		return 0
	}
	return int(math.Round(float64(done) * 100 / float64(total)))
}

func decoratedLines(title string, items []string) []string {
	lines := []string{title}
	for i, item := range items {
		lines = append(lines, fmt.Sprintf("%s %s", decorativeBullet(i), item))
	}
	return lines
}

func decorativeBullet(index int) string {
	bullets := []string{"▪️", "🔺", "♦️", "🔹", "🔸"}
	return bullets[index%len(bullets)]
}
