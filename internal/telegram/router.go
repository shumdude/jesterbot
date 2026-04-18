package telegram

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	tgamlengine "github.com/shumdude/tgaml/pkg/engine"
	tgamlsession "github.com/shumdude/tgaml/pkg/session"

	"jesterbot/internal/domain"
	"jesterbot/internal/service"
	"jesterbot/internal/telegram/constants"
)

type Controller struct {
	logger  *slog.Logger
	service *service.Service
	bot     *bot.Bot
	eng     *tgamlengine.Engine
}

func (r *Controller) handleActivityCallback(ctx context.Context, _ *bot.Bot, update *models.Update) {
	r.logCallbackEvent("handling activity callback", update.CallbackQuery)
	r.answerCallback(ctx, update.CallbackQuery.ID)
	chatID, userID, messageID := callbackIdentity(update)
	user, err := r.registeredUser(ctx, userID)
	if err != nil {
		r.handleRegistrationRequired(ctx, chatID, userID)
		return
	}

	data := update.CallbackQuery.Data
	switch {
	case data == "activity:back":
		r.showMainMenuFromCallback(ctx, chatID, userID, messageID)
	case strings.HasPrefix(data, "activity:list:"):
		page, err := parsePageCallback(data)
		if err != nil {
			return
		}
		r.showActivitiesPageAsEdit(ctx, chatID, messageID, user.ID, tr("activity_title"), page)
	case strings.HasPrefix(data, "activity:page:"):
		page, err := parsePageCallback(data)
		if err != nil {
			return
		}
		r.showActivitiesPageAsEdit(ctx, chatID, messageID, user.ID, tr("activity_title"), page)
	case strings.HasPrefix(data, "activity:open:"):
		activityID, page, err := parseIDPageCallback(data)
		if err != nil {
			return
		}
		r.showActivityDetailAsEdit(ctx, chatID, messageID, user.ID, activityID, page, tr("activity_detail_title"))
	case data == "activity:add":
		sess := r.session(userID, chatID)
		_ = sess.ClearNamespace(constants.NSActivity)
		_ = sess.Transition(ctx, constants.SceneAddActivity)
		r.showScreenFromCallback(ctx, chatID, messageID, tr("activity_prompt_add"), r.sceneKeyboardMarkup("activities_back_menu", userID, chatID))
	case strings.HasPrefix(data, "activity:edit:"):
		activityID, err := parseID(data)
		if err != nil {
			return
		}
		sess := r.session(userID, chatID)
		_ = sess.SetStrings(constants.NSActivity, map[string]string{
			constants.KeyActivityID:   strconv.FormatInt(activityID, 10),
			constants.KeyActivityPage: "0",
		})
		_ = sess.Transition(ctx, constants.SceneEditActivity)
		r.showScreenFromCallback(ctx, chatID, messageID, tr("activity_prompt_edit"), r.sceneKeyboardMarkup("activity_detail_back_menu", userID, chatID))
	case strings.HasPrefix(data, "activity:times:"):
		activityID, page, err := parseIDPageCallback(data)
		if err != nil {
			return
		}
		activities, err := r.service.ListActivities(ctx, user.ID)
		if err != nil {
			r.showScreenFromCallback(ctx, chatID, messageID, tr("activity_error_list"), nil)
			return
		}
		var activityTitle string
		for _, a := range activities {
			if a.ID == activityID {
				activityTitle = a.Title
				break
			}
		}
		sess := r.session(userID, chatID)
		_ = sess.SetStrings(constants.NSActivity, map[string]string{
			constants.KeyActivityID:   strconv.FormatInt(activityID, 10),
			constants.KeyActivityPage: strconv.Itoa(page),
		})
		_ = sess.Transition(ctx, constants.SceneSetActivityTimes)
		r.showScreenFromCallback(ctx, chatID, messageID, tr("activity_prompt_times", activityTitle), r.sceneKeyboardMarkup("activity_detail_back_menu", userID, chatID))
	case strings.HasPrefix(data, "activity:window:"):
		activityID, page, err := parseIDPageCallback(data)
		if err != nil {
			return
		}
		activities, err := r.service.ListActivities(ctx, user.ID)
		if err != nil {
			r.showScreenFromCallback(ctx, chatID, messageID, tr("activity_error_list"), nil)
			return
		}
		var windowDesc string
		for _, a := range activities {
			if a.ID == activityID {
				if a.ReminderWindowStart != "" {
					windowDesc = a.ReminderWindowStart + "–" + a.ReminderWindowEnd
				} else {
					windowDesc = tr("activity_window_none")
				}
				break
			}
		}
		sess := r.session(userID, chatID)
		_ = sess.SetStrings(constants.NSActivity, map[string]string{
			constants.KeyActivityID:   strconv.FormatInt(activityID, 10),
			constants.KeyActivityPage: strconv.Itoa(page),
		})
		_ = sess.Transition(ctx, constants.SceneSetActivityWindow)
		r.showScreenFromCallback(ctx, chatID, messageID, tr("activity_prompt_window", windowDesc), r.sceneKeyboardMarkup("activity_detail_back_menu", userID, chatID))
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

func (r *Controller) handlePlanCallback(ctx context.Context, _ *bot.Bot, update *models.Update) {
	r.logCallbackEvent("handling plan callback", update.CallbackQuery)
	r.answerCallback(ctx, update.CallbackQuery.ID)
	chatID, userID, messageID := callbackIdentity(update)
	user, err := r.registeredUser(ctx, userID)
	if err != nil {
		r.handleRegistrationRequired(ctx, chatID, userID)
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
			r.showScreenFromCallback(ctx, chatID, messageID, todayPlanErrorText(err), r.menuMarkup(user.TelegramUserID, chatID))
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

func (r *Controller) handleDoneCallback(ctx context.Context, _ *bot.Bot, update *models.Update) {
	r.logCallbackEvent("handling done callback", update.CallbackQuery)
	r.answerCallback(ctx, update.CallbackQuery.ID)
	chatID, userID, messageID := callbackIdentity(update)
	user, err := r.registeredUser(ctx, userID)
	if err != nil {
		r.handleRegistrationRequired(ctx, chatID, userID)
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
		r.showScreenFromCallback(ctx, chatID, messageID, completionMessage(plan), r.menuMarkup(user.TelegramUserID, chatID))
	}
}

func (r *Controller) handleSettingsCallback(ctx context.Context, _ *bot.Bot, update *models.Update) {
	r.logCallbackEvent("handling settings callback", update.CallbackQuery)
	r.answerCallback(ctx, update.CallbackQuery.ID)
	chatID, userID, messageID := callbackIdentity(update)

	switch update.CallbackQuery.Data {
	case "settings:morning":
		sess := r.session(userID, chatID)
		_ = sess.Transition(ctx, constants.SceneUpdateMorning)
		r.showScreenFromCallback(ctx, chatID, messageID, tr("settings_prompt_morning"), r.sceneKeyboardMarkup("settings_back_menu", userID, chatID))
	case "settings:interval":
		sess := r.session(userID, chatID)
		_ = sess.Transition(ctx, constants.SceneUpdateReminder)
		r.showScreenFromCallback(ctx, chatID, messageID, tr("settings_prompt_interval"), r.sceneKeyboardMarkup("settings_back_menu", userID, chatID))
	case "settings:tick":
		user, err := r.registeredUser(ctx, update.CallbackQuery.From.ID)
		if err != nil {
			r.handleRegistrationRequired(ctx, chatID, userID)
			return
		}
		minutes, err := r.service.GetUserTickInterval(ctx, user.ID)
		if err != nil {
			r.showScreenFromCallback(ctx, chatID, messageID, tr("settings_error_tick_get"), nil)
			return
		}
		sess := r.session(userID, chatID)
		_ = sess.Transition(ctx, constants.SceneUpdateTick)
		r.showScreenFromCallback(ctx, chatID, messageID, tr("settings_prompt_tick", minutes), r.sceneKeyboardMarkup("settings_back_menu", userID, chatID))
	case "settings:oneoff":
		user, err := r.registeredUser(ctx, update.CallbackQuery.From.ID)
		if err != nil {
			r.handleRegistrationRequired(ctx, chatID, userID)
			return
		}
		settings, err := r.service.GetOneOffReminderSettings(ctx, user.ID)
		if err != nil {
			r.showScreenFromCallback(ctx, chatID, messageID, tr("settings_error_oneoff_get"), nil)
			return
		}
		sess := r.session(userID, chatID)
		_ = sess.Transition(ctx, constants.SceneUpdateOneOffReminder)
		r.showScreenFromCallback(ctx, chatID, messageID, oneOffReminderSettingsPrompt(settings), r.sceneKeyboardMarkup("settings_back_menu", userID, chatID))
	}
}

func (r *Controller) handleMenuCallback(ctx context.Context, _ *bot.Bot, update *models.Update) {
	r.logCallbackEvent("handling menu callback", update.CallbackQuery)
	r.answerCallback(ctx, update.CallbackQuery.ID)
	chatID, userID, messageID := callbackIdentity(update)

	if _, err := r.registeredUser(ctx, userID); err != nil {
		r.handleRegistrationRequired(ctx, chatID, userID)
		return
	}

	switch update.CallbackQuery.Data {
	case "menu:back":
		r.showMainMenuFromCallback(ctx, chatID, userID, messageID)
	}
}

func (r *Controller) registeredUser(ctx context.Context, telegramUserID int64) (*domain.User, error) {
	return r.service.FindUserByTelegramID(ctx, telegramUserID)
}

func (r *Controller) session(userID, chatID int64) *tgamlsession.Session {
	return r.eng.NewSession(userID, chatID)
}

func (r *Controller) menuMarkup(userID, chatID int64) models.ReplyMarkup {
	markup := r.eng.KeyboardMarkupForUser("main_menu", r.session(userID, chatID))
	return &markup
}

func (r *Controller) sceneKeyboardMarkup(name string, userID, chatID int64) models.ReplyMarkup {
	markup := r.eng.KeyboardMarkupForUser(name, r.session(userID, chatID))
	return &markup
}

func (r *Controller) showActivities(ctx context.Context, chatID, userID int64, prefix string) {
	r.showActivitiesPage(ctx, chatID, userID, prefix, 0)
}

func (r *Controller) showActivitiesPage(ctx context.Context, chatID, userID int64, prefix string, page int) {
	activities, err := r.service.ListActivities(ctx, userID)
	if err != nil {
		r.showScreen(ctx, chatID, tr("activity_error_list"), r.menuMarkup(userID, chatID))
		return
	}
	r.showScreen(ctx, chatID, prefix+"\n\n"+activitiesTextPage(activities, page, defaultInlinePageSize), buildActivitiesKeyboardPage(activities, page, defaultInlinePageSize))
}

func (r *Controller) showActivitiesPageAsEdit(ctx context.Context, chatID int64, messageID int, userID int64, prefix string, page int) {
	activities, err := r.service.ListActivities(ctx, userID)
	if err != nil {
		r.showScreenFromCallback(ctx, chatID, messageID, tr("activity_error_list"), r.menuMarkup(userID, chatID))
		return
	}
	r.showScreenFromCallback(ctx, chatID, messageID, prefix+"\n\n"+activitiesTextPage(activities, page, defaultInlinePageSize), buildActivitiesKeyboardPage(activities, page, defaultInlinePageSize))
}

func (r *Controller) showActivityDetail(ctx context.Context, chatID, userID, activityID int64, page int, prefix string) {
	activities, err := r.service.ListActivities(ctx, userID)
	if err != nil {
		r.showScreen(ctx, chatID, tr("activity_error_list"), r.menuMarkup(userID, chatID))
		return
	}

	activity, ok := findActivityByID(activities, activityID)
	if !ok {
		r.showActivitiesPage(ctx, chatID, userID, tr("activity_error_list"), page)
		return
	}

	r.showScreen(ctx, chatID, activityDetailText(prefix, activity), buildActivityDetailKeyboard(activity, page))
}

func (r *Controller) showActivityDetailAsEdit(ctx context.Context, chatID int64, messageID int, userID, activityID int64, page int, prefix string) {
	activities, err := r.service.ListActivities(ctx, userID)
	if err != nil {
		r.showScreenFromCallback(ctx, chatID, messageID, tr("activity_error_list"), r.menuMarkup(userID, chatID))
		return
	}

	activity, ok := findActivityByID(activities, activityID)
	if !ok {
		r.showActivitiesPageAsEdit(ctx, chatID, messageID, userID, tr("activity_error_list"), page)
		return
	}

	r.showScreenFromCallback(ctx, chatID, messageID, activityDetailText(prefix, activity), buildActivityDetailKeyboard(activity, page))
}

func (r *Controller) showSettings(ctx context.Context, chatID, telegramUserID int64, prefix string) {
	user, err := r.registeredUser(ctx, telegramUserID)
	if err != nil {
		r.handleRegistrationRequired(ctx, chatID, telegramUserID)
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

func (r *Controller) mustActivities(ctx context.Context, userID int64) []domain.Activity {
	activities, err := r.service.ListActivities(ctx, userID)
	if err != nil {
		r.logger.Error("list activities failed", "error", err, "user_id", userID)
		return nil
	}
	return activities
}

func (r *Controller) handleRegistrationRequired(ctx context.Context, chatID, userID int64) {
	sess := r.session(userID, chatID)
	if err := sess.Transition(ctx, constants.SceneRegName); err != nil {
		r.logger.Error("transition to registration scene failed", "error", err, "chat_id", chatID, "telegram_user_id", userID)
		r.showScreen(ctx, chatID, tr("register_required"), nil)
		return
	}
	r.eng.RenderScene(ctx, r.bot, constants.SceneRegName, chatID, sess)
}

func (r *Controller) sendMessage(ctx context.Context, chatID int64, text string, markup models.ReplyMarkup) int {
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

func (r *Controller) editMessage(ctx context.Context, chatID int64, messageID int, text string, markup models.ReplyMarkup) bool {
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

func (r *Controller) deleteMessage(ctx context.Context, chatID int64, messageID int) bool {
	if messageID == 0 {
		return false
	}
	if _, err := r.bot.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: chatID, MessageID: messageID}); err != nil {
		r.logger.Error("delete message failed", "error", err, "chat_id", chatID, "message_id", messageID)
		return false
	}
	return true
}

func (r *Controller) showScreen(ctx context.Context, chatID int64, text string, markup models.ReplyMarkup) {
	r.sendMessage(ctx, chatID, text, markup)
}

func (r *Controller) showScreenFromCallback(ctx context.Context, chatID int64, currentMessageID int, text string, markup models.ReplyMarkup) {
	if currentMessageID != 0 && supportsMessageEdit(markup) && r.editMessage(ctx, chatID, currentMessageID, text, markup) {
		return
	}
	if currentMessageID != 0 && !supportsMessageEdit(markup) {
		r.deleteMessage(ctx, chatID, currentMessageID)
	}
	r.sendMessage(ctx, chatID, text, markup)
}

func (r *Controller) showMainMenuFromCallback(ctx context.Context, chatID, userID int64, currentMessageID int) {
	sess := r.session(userID, chatID)
	_ = sess.ClearNamespace(constants.NSActivity)
	_ = sess.Transition(ctx, constants.SceneMenu)
	r.deleteMessage(ctx, chatID, currentMessageID)
	r.showScreen(ctx, chatID, r.eng.T("messages.menu.registered"), r.menuMarkup(userID, chatID))
}

func usesHTMLParseMode(text string) bool {
	return strings.Contains(text, "<b>") || strings.Contains(text, "</b>")
}

func supportsMessageEdit(markup models.ReplyMarkup) bool {
	if markup == nil {
		return true
	}
	_, ok := markup.(*models.InlineKeyboardMarkup)
	return ok
}

func (r *Controller) answerCallback(ctx context.Context, callbackID string) {
	if _, err := r.bot.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: callbackID}); err != nil {
		r.logger.Error("answer callback failed", "error", err)
	}
}

func (r *Controller) handleNoopCallback(ctx context.Context, _ *bot.Bot, update *models.Update) {
	r.answerCallback(ctx, update.CallbackQuery.ID)
}

func callbackIdentity(update *models.Update) (chatID int64, userID int64, messageID int) {
	// For callback queries, ownership checks and message edits rely on the tuple
	// (chat id, telegram user id, message id).
	return update.CallbackQuery.Message.Message.Chat.ID, update.CallbackQuery.From.ID, update.CallbackQuery.Message.Message.ID
}

func (r *Controller) logMessageEvent(event string, message *models.Message, attrs ...any) {
	args := []any{"chat_id", message.Chat.ID, "telegram_user_id", message.From.ID}
	r.logger.Info(event, append(args, attrs...)...)
}

func (r *Controller) logCallbackEvent(event string, callback *models.CallbackQuery, attrs ...any) {
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

// parseWindowInput parses "HH:MM-HH:MM" into (start, end). Returns an error if
// either part is not a valid 24-hour clock time or start equals end.
func parseWindowInput(input string) (start, end string, err error) {
	parts := strings.Split(strings.TrimSpace(input), "-")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("expected HH:MM-HH:MM")
	}
	parsed, parseErr := time.Parse("15:04", strings.TrimSpace(parts[0]))
	if parseErr != nil {
		return "", "", fmt.Errorf("invalid start time")
	}
	start = parsed.Format("15:04")
	parsed, parseErr = time.Parse("15:04", strings.TrimSpace(parts[1]))
	if parseErr != nil {
		return "", "", fmt.Errorf("invalid end time")
	}
	end = parsed.Format("15:04")
	if start == end {
		return "", "", fmt.Errorf("start and end must differ")
	}
	return start, end, nil
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

func activityDetailText(prefix string, activity domain.Activity) string {
	timesPerDay := activity.TimesPerDay
	if timesPerDay < 1 {
		timesPerDay = 1
	}

	window := tr("activity_window_none")
	if activity.ReminderWindowStart != "" && activity.ReminderWindowEnd != "" {
		window = tr("activity_window_value", activity.ReminderWindowStart, activity.ReminderWindowEnd)
	}

	lines := []string{
		prefix,
		"",
		tr("activity_detail_name", activity.Title),
		tr("activity_detail_times", timesPerDay),
		tr("activity_detail_window", window),
	}

	return strings.Join(lines, "\n")
}

func findActivityByID(activities []domain.Activity, activityID int64) (domain.Activity, bool) {
	for _, activity := range activities {
		if activity.ID == activityID {
			return activity, true
		}
	}
	return domain.Activity{}, false
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
		timesPerDay := item.TimesPerDay
		if timesPerDay < 1 {
			timesPerDay = 1
		}
		switch {
		case item.Completed:
			completed = append(completed, html.EscapeString(item.TitleSnapshot))
		case timesPerDay > 1 && item.CompletedCount > 0:
			remaining = append(remaining, fmt.Sprintf("%s (%d/%d)", html.EscapeString(item.TitleSnapshot), item.CompletedCount, timesPerDay))
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
