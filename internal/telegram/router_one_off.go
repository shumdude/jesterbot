package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"jesterbot/internal/domain"
)

func (r *Router) handleOneOffTasksCommand(ctx context.Context, _ *bot.Bot, update *models.Update) {
	r.logMessageEvent("handling one-off tasks command", update.Message)
	chatID := update.Message.Chat.ID
	user, err := r.registeredUser(ctx, update.Message.From.ID)
	if err != nil {
		r.handleRegistrationRequired(ctx, chatID)
		return
	}

	r.showOneOffTasksPage(ctx, chatID, user.ID, tr("oneoff_title"), 0)
}

func (r *Router) handleOneOffCallback(ctx context.Context, _ *bot.Bot, update *models.Update) {
	r.logCallbackEvent("handling one-off callback", update.CallbackQuery)
	r.answerCallback(ctx, update.CallbackQuery.ID)
	chatID, userID, messageID := callbackIdentity(update)
	user, err := r.registeredUser(ctx, userID)
	if err != nil {
		r.handleRegistrationRequired(ctx, chatID)
		return
	}

	data := update.CallbackQuery.Data
	switch {
	case data == "oneoff:add":
		r.sessions.Update(chatID, func(s *Session) {
			*s = Session{State: stateAddOneOffTitle}
		})
		r.sendMessage(ctx, chatID, tr("oneoff_prompt_title"), nil)
	case strings.HasPrefix(data, "oneoff:back:"):
		page, err := parsePageCallback(data)
		if err != nil {
			return
		}
		r.showOneOffTasksPageAsEdit(ctx, chatID, messageID, user.ID, tr("oneoff_title"), page)
	case strings.HasPrefix(data, "oneoff:page:"):
		page, err := parsePageCallback(data)
		if err != nil {
			return
		}
		r.showOneOffTasksPageAsEdit(ctx, chatID, messageID, user.ID, tr("oneoff_title"), page)
	case strings.HasPrefix(data, "oneoff:open:"):
		taskID, page, err := parseIDPageCallback(data)
		if err != nil {
			return
		}
		r.showOneOffTaskDetailAsEdit(ctx, chatID, messageID, user.ID, taskID, page)
	case strings.HasPrefix(data, "oneoff:delete:"):
		taskID, page, err := parseIDPageCallback(data)
		if err != nil {
			return
		}
		if err := r.service.DeleteOneOffTask(ctx, user.ID, taskID); err != nil {
			r.sendMessage(ctx, chatID, tr("oneoff_error_delete", err.Error()), nil)
			return
		}
		r.showOneOffTasksPageAsEdit(ctx, chatID, messageID, user.ID, tr("oneoff_success_delete"), page)
	case strings.HasPrefix(data, "oneoff:complete:"):
		taskID, page, err := parseIDPageCallback(data)
		if err != nil {
			return
		}
		task, err := r.service.CompleteOneOffTask(ctx, user.ID, taskID, time.Now().UTC())
		if err != nil {
			r.sendMessage(ctx, chatID, tr("oneoff_error_complete", err.Error()), nil)
			return
		}
		r.editMessage(ctx, chatID, messageID, oneOffTaskDetailText(task), buildOneOffTaskDetailKeyboardPage(task, page))
	case strings.HasPrefix(data, "oneoff:item:"):
		taskID, itemID, page, err := parseOneOffItemIDs(data)
		if err != nil {
			return
		}
		task, err := r.service.ToggleOneOffTaskItem(ctx, user.ID, taskID, itemID, time.Now().UTC())
		if err != nil {
			r.sendMessage(ctx, chatID, tr("oneoff_error_toggle_item", err.Error()), nil)
			return
		}
		r.editMessage(ctx, chatID, messageID, oneOffTaskDetailText(task), buildOneOffTaskDetailKeyboardPage(task, page))
	case strings.HasPrefix(data, "oneoff:create:priority:"):
		priorityValue := strings.TrimPrefix(data, "oneoff:create:priority:")
		priority := domain.OneOffTaskPriority(priorityValue)
		draft := r.sessions.Get(chatID)
		if draft.State != stateAddOneOffTitle || strings.TrimSpace(draft.OneOffTaskTitle) == "" {
			r.sendMessage(ctx, chatID, tr("oneoff_error_title_required"), nil)
			return
		}
		r.sessions.Update(chatID, func(s *Session) {
			s.State = stateAddOneOffItems
			s.OneOffTaskPriority = priority
		})
		r.sendMessage(ctx, chatID, tr("oneoff_prompt_items"), nil)
	}
}

func (r *Router) showOneOffTasks(ctx context.Context, chatID, userID int64, prefix string) {
	r.showOneOffTasksPage(ctx, chatID, userID, prefix, 0)
}

func (r *Router) showOneOffTasksPage(ctx context.Context, chatID, userID int64, prefix string, page int) {
	tasks, err := r.service.ListOneOffTasks(ctx, userID)
	if err != nil {
		r.sendMessage(ctx, chatID, tr("oneoff_error_list"), r.mainMenu)
		return
	}
	r.sendMessage(ctx, chatID, prefix+"\n\n"+oneOffTasksTextPage(tasks, page, defaultInlinePageSize), buildOneOffTasksKeyboardPage(tasks, page, defaultInlinePageSize))
}

func (r *Router) showOneOffTasksAsEdit(ctx context.Context, chatID int64, messageID int, userID int64, prefix string) {
	r.showOneOffTasksPageAsEdit(ctx, chatID, messageID, userID, prefix, 0)
}

func (r *Router) showOneOffTasksPageAsEdit(ctx context.Context, chatID int64, messageID int, userID int64, prefix string, page int) {
	tasks, err := r.service.ListOneOffTasks(ctx, userID)
	if err != nil {
		r.sendMessage(ctx, chatID, tr("oneoff_error_list"), r.mainMenu)
		return
	}
	r.editMessage(ctx, chatID, messageID, prefix+"\n\n"+oneOffTasksTextPage(tasks, page, defaultInlinePageSize), buildOneOffTasksKeyboardPage(tasks, page, defaultInlinePageSize))
}

func (r *Router) showOneOffTaskDetailAsEdit(ctx context.Context, chatID int64, messageID int, userID, taskID int64, page int) {
	task, err := r.service.GetOneOffTask(ctx, userID, taskID)
	if err != nil {
		r.sendMessage(ctx, chatID, tr("oneoff_error_open"), nil)
		return
	}
	r.editMessage(ctx, chatID, messageID, oneOffTaskDetailText(task), buildOneOffTaskDetailKeyboardPage(task, page))
}

func parseOneOffChecklistInput(input string) []string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" || trimmed == "-" || strings.EqualFold(trimmed, "нет") {
		return nil
	}

	parts := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	})
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		items = append(items, item)
	}
	return items
}

func parseOneOffReminderSettingsInput(input string) (int, int, int, error) {
	parts := strings.FieldsFunc(strings.TrimSpace(input), func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r'
	})
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("expected three values")
	}

	values := make([]int, 0, 3)
	for _, part := range parts {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || value <= 0 {
			return 0, 0, 0, fmt.Errorf("invalid interval: %s", part)
		}
		values = append(values, value)
	}

	return values[0], values[1], values[2], nil
}

func parseOneOffItemIDs(data string) (int64, int64, int, error) {
	parts := strings.Split(data, ":")
	if len(parts) != 5 {
		return 0, 0, 0, fmt.Errorf("invalid one-off item callback: %s", data)
	}

	taskID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	itemID, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	page, err := strconv.Atoi(parts[4])
	if err != nil {
		return 0, 0, 0, err
	}

	return taskID, itemID, page, nil
}

func oneOffTasksText(tasks []domain.OneOffTask) string {
	return oneOffTasksTextPage(tasks, 0, defaultInlinePageSize)
}

func oneOffTasksTextPage(tasks []domain.OneOffTask, page, pageSize int) string {
	activeTasks, completedTasks := splitOneOffTasks(tasks)
	if len(activeTasks) == 0 && len(completedTasks) == 0 {
		return tr("oneoff_list_empty")
	}

	view := paginate(activeTasks, page, pageSize)
	lines := make([]string, 0, len(view.Items)+3)
	lines = append(lines, tr("oneoff_list_title"))
	if len(view.Items) == 0 {
		lines = append(lines, tr("oneoff_list_empty_active"))
	}
	if view.TotalPages > 1 {
		lines = append(lines, pageSummary(view.Page, view.TotalPages, view.Start, view.End, view.TotalItems))
	}
	for i, task := range view.Items {
		lines = append(lines, fmt.Sprintf(
			"%d. %s %s %s (%s)",
			view.Start+i+1,
			oneOffPriorityIcon(task.Priority),
			task.Title,
			oneOffChecklistSummary(task),
			oneOffStatusLabel(task.Status),
		))
	}
	return strings.Join(lines, "\n")
}

func oneOffTaskDetailText(task *domain.OneOffTask) string {
	lines := []string{
		tr("oneoff_detail_title", oneOffPriorityIcon(task.Priority), task.Title),
		tr("oneoff_detail_status", oneOffStatusLabel(task.Status)),
		tr("oneoff_detail_items", oneOffChecklistSummary(*task)),
	}
	if task.NextReminderAt != nil && task.Status == domain.OneOffTaskStatusActive {
		lines = append(lines, tr("oneoff_detail_next", task.NextReminderAt.UTC().Format("2006-01-02 15:04")))
	}
	if len(task.Items) == 0 {
		lines = append(lines, tr("oneoff_detail_no_items"))
	} else {
		for _, item := range task.Items {
			icon := "⬜"
			if item.Completed {
				icon = "☑️"
			}
			lines = append(lines, fmt.Sprintf("%s %s", icon, item.Title))
		}
	}
	return strings.Join(lines, "\n")
}

func oneOffReminderText(task *domain.OneOffTask) string {
	prefix := tr("oneoff_reminder_default")
	switch task.Priority {
	case domain.OneOffTaskPriorityHigh:
		prefix = tr("oneoff_reminder_high")
	case domain.OneOffTaskPriorityLow:
		prefix = tr("oneoff_reminder_low")
	}

	lines := []string{
		prefix,
		tr("oneoff_reminder_title", task.Title),
		tr("oneoff_reminder_progress", oneOffChecklistSummary(*task)),
	}
	pendingItems := pendingOneOffItemTitles(*task)
	if len(pendingItems) > 0 {
		lines = append(lines, decoratedLines(tr("oneoff_reminder_remaining"), pendingItems)...)
	}

	return strings.Join(lines, "\n")
}

func oneOffReminderSettingsPrompt(settings *domain.OneOffReminderSettings) string {
	return tr(
		"settings_prompt_oneoff",
		settings.LowPriorityMinutes,
		settings.MediumPriorityMinutes,
		settings.HighPriorityMinutes,
	)
}

func oneOffPriorityIcon(priority domain.OneOffTaskPriority) string {
	switch priority {
	case domain.OneOffTaskPriorityHigh:
		return "🟥"
	case domain.OneOffTaskPriorityLow:
		return "🟩"
	default:
		return "🟨"
	}
}

func oneOffStatusLabel(status domain.OneOffTaskStatus) string {
	if status == domain.OneOffTaskStatusCompleted {
		return tr("oneoff_status_completed")
	}
	return tr("oneoff_status_active")
}

func oneOffChecklistSummary(task domain.OneOffTask) string {
	if len(task.Items) == 0 {
		if task.Status == domain.OneOffTaskStatusCompleted {
			return tr("oneoff_checklist_completed")
		}
		return tr("oneoff_checklist_none")
	}

	completed := 0
	for _, item := range task.Items {
		if item.Completed {
			completed++
		}
	}

	return fmt.Sprintf("%d/%d", completed, len(task.Items))
}

func pendingOneOffItemTitles(task domain.OneOffTask) []string {
	items := make([]string, 0, len(task.Items))
	for _, item := range task.Items {
		if item.Completed {
			continue
		}
		items = append(items, item.Title)
	}
	return items
}

func splitOneOffTasks(tasks []domain.OneOffTask) ([]domain.OneOffTask, []domain.OneOffTask) {
	activeTasks := make([]domain.OneOffTask, 0, len(tasks))
	completedTasks := make([]domain.OneOffTask, 0, len(tasks))
	for _, task := range tasks {
		if task.Status == domain.OneOffTaskStatusCompleted {
			completedTasks = append(completedTasks, task)
			continue
		}
		activeTasks = append(activeTasks, task)
	}
	return activeTasks, completedTasks
}
