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

	r.showOneOffTasks(ctx, chatID, user.ID, "📝 Разовые дела.")
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
		r.sendMessage(ctx, chatID, "📝 Пришли название разового дела.", nil)
	case data == "oneoff:back":
		r.showOneOffTasksAsEdit(ctx, chatID, messageID, user.ID, "📝 Разовые дела.")
	case data == "oneoff:history":
		r.showOneOffHistoryAsEdit(ctx, chatID, messageID, user.ID)
	case strings.HasPrefix(data, "oneoff:open:"):
		taskID, err := parseID(data)
		if err != nil {
			return
		}
		r.showOneOffTaskDetailAsEdit(ctx, chatID, messageID, user.ID, taskID)
	case strings.HasPrefix(data, "oneoff:delete:"):
		taskID, err := parseID(data)
		if err != nil {
			return
		}
		if err := r.service.DeleteOneOffTask(ctx, user.ID, taskID); err != nil {
			r.sendMessage(ctx, chatID, "❌ Не получилось удалить разовое дело: "+err.Error(), nil)
			return
		}
		r.showOneOffTasksAsEdit(ctx, chatID, messageID, user.ID, "🗑 Разовое дело удалено.")
	case strings.HasPrefix(data, "oneoff:complete:"):
		taskID, err := parseID(data)
		if err != nil {
			return
		}
		task, err := r.service.CompleteOneOffTask(ctx, user.ID, taskID, time.Now().UTC())
		if err != nil {
			r.sendMessage(ctx, chatID, "❌ Не получилось завершить разовое дело: "+err.Error(), nil)
			return
		}
		r.editMessage(ctx, chatID, messageID, oneOffTaskDetailText(task), buildOneOffTaskDetailKeyboard(task))
	case strings.HasPrefix(data, "oneoff:item:"):
		taskID, itemID, err := parseOneOffItemIDs(data)
		if err != nil {
			return
		}
		task, err := r.service.ToggleOneOffTaskItem(ctx, user.ID, taskID, itemID, time.Now().UTC())
		if err != nil {
			r.sendMessage(ctx, chatID, "❌ Не получилось обновить чекбокс: "+err.Error(), nil)
			return
		}
		r.editMessage(ctx, chatID, messageID, oneOffTaskDetailText(task), buildOneOffTaskDetailKeyboard(task))
	case strings.HasPrefix(data, "oneoff:create:priority:"):
		priorityValue := strings.TrimPrefix(data, "oneoff:create:priority:")
		priority := domain.OneOffTaskPriority(priorityValue)
		draft := r.sessions.Get(chatID)
		if draft.State != stateAddOneOffTitle || strings.TrimSpace(draft.OneOffTaskTitle) == "" {
			r.sendMessage(ctx, chatID, "❗ Сначала пришли название разового дела.", nil)
			return
		}
		r.sessions.Update(chatID, func(s *Session) {
			s.State = stateAddOneOffItems
			s.OneOffTaskPriority = priority
		})
		r.sendMessage(ctx, chatID, "☑️ Пришли подпункты через запятую или новой строкой. Если подпунктов нет, отправь `-`.", nil)
	}
}

func (r *Router) showOneOffTasks(ctx context.Context, chatID, userID int64, prefix string) {
	tasks, err := r.service.ListOneOffTasks(ctx, userID)
	if err != nil {
		r.sendMessage(ctx, chatID, "❌ Не получилось получить разовые дела.", r.mainMenu)
		return
	}
	r.sendMessage(ctx, chatID, prefix+"\n\n"+oneOffTasksText(tasks), buildOneOffTasksKeyboard(tasks))
}

func (r *Router) showOneOffTasksAsEdit(ctx context.Context, chatID int64, messageID int, userID int64, prefix string) {
	tasks, err := r.service.ListOneOffTasks(ctx, userID)
	if err != nil {
		r.sendMessage(ctx, chatID, "❌ Не получилось получить разовые дела.", r.mainMenu)
		return
	}
	r.editMessage(ctx, chatID, messageID, prefix+"\n\n"+oneOffTasksText(tasks), buildOneOffTasksKeyboard(tasks))
}

func (r *Router) showOneOffTaskDetailAsEdit(ctx context.Context, chatID int64, messageID int, userID, taskID int64) {
	task, err := r.service.GetOneOffTask(ctx, userID, taskID)
	if err != nil {
		r.sendMessage(ctx, chatID, "❌ Не получилось открыть разовое дело.", nil)
		return
	}
	r.editMessage(ctx, chatID, messageID, oneOffTaskDetailText(task), buildOneOffTaskDetailKeyboard(task))
}

func (r *Router) showOneOffHistoryAsEdit(ctx context.Context, chatID int64, messageID int, userID int64) {
	tasks, err := r.service.ListOneOffTasks(ctx, userID)
	if err != nil {
		r.sendMessage(ctx, chatID, "❌ Не получилось получить историю разовых дел.", r.mainMenu)
		return
	}
	r.editMessage(ctx, chatID, messageID, oneOffHistoryText(tasks), buildOneOffHistoryKeyboard(tasks))
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

func parseOneOffItemIDs(data string) (int64, int64, error) {
	parts := strings.Split(data, ":")
	if len(parts) != 4 {
		return 0, 0, fmt.Errorf("invalid one-off item callback: %s", data)
	}

	taskID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return 0, 0, err
	}
	itemID, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return 0, 0, err
	}

	return taskID, itemID, nil
}

func oneOffTasksText(tasks []domain.OneOffTask) string {
	activeTasks, completedTasks := splitOneOffTasks(tasks)
	if len(activeTasks) == 0 && len(completedTasks) == 0 {
		return "🗒 Разовых дел пока нет. Добавь первое."
	}

	lines := make([]string, 0, len(activeTasks)+4)
	lines = append(lines, "📝 Активные разовые дела:")
	if len(activeTasks) == 0 {
		lines = append(lines, "- Сейчас активных разовых дел нет.")
	}
	for i, task := range activeTasks {
		lines = append(lines, fmt.Sprintf(
			"%d. %s %s %s (%s)",
			i+1,
			oneOffPriorityIcon(task.Priority),
			task.Title,
			oneOffChecklistSummary(task),
			oneOffStatusLabel(task.Status),
		))
	}
	if len(completedTasks) > 0 {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("🕘 История дел: %d.", len(completedTasks)))
		lines = append(lines, "Открой историю кнопкой ниже.")
	}
	return strings.Join(lines, "\n")
}

func oneOffHistoryText(tasks []domain.OneOffTask) string {
	_, completedTasks := splitOneOffTasks(tasks)
	if len(completedTasks) == 0 {
		return "🕘 История дел пока пуста."
	}

	lines := make([]string, 0, len(completedTasks)+1)
	lines = append(lines, "🕘 История дел:")
	for i, task := range completedTasks {
		lines = append(lines, fmt.Sprintf(
			"%d. %s %s %s",
			i+1,
			oneOffPriorityIcon(task.Priority),
			task.Title,
			oneOffChecklistSummary(task),
		))
	}
	return strings.Join(lines, "\n")
}

func oneOffTaskDetailText(task *domain.OneOffTask) string {
	lines := []string{
		fmt.Sprintf("%s Разовое дело: %s", oneOffPriorityIcon(task.Priority), task.Title),
		fmt.Sprintf("📍 Статус: %s", oneOffStatusLabel(task.Status)),
		fmt.Sprintf("☑️ Подпункты: %s", oneOffChecklistSummary(*task)),
	}
	if task.NextReminderAt != nil && task.Status == domain.OneOffTaskStatusActive {
		lines = append(lines, fmt.Sprintf("⏰ Следующее напоминание: %s UTC", task.NextReminderAt.UTC().Format("2006-01-02 15:04")))
	}
	if len(task.Items) == 0 {
		lines = append(lines, "• Подпунктов нет.")
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
	prefix := "🟨 Напоминание о разовом деле."
	switch task.Priority {
	case domain.OneOffTaskPriorityHigh:
		prefix = "🟥 Срочное напоминание о разовом деле."
	case domain.OneOffTaskPriorityLow:
		prefix = "🟩 Спокойное напоминание о разовом деле."
	}

	lines := []string{
		prefix,
		fmt.Sprintf("👉 %s", task.Title),
		fmt.Sprintf("☑️ Прогресс: %s", oneOffChecklistSummary(*task)),
	}
	pendingItems := pendingOneOffItemTitles(*task)
	if len(pendingItems) > 0 {
		lines = append(lines, "📌 Осталось: "+strings.Join(pendingItems, ", "))
	}

	return strings.Join(lines, "\n")
}

func oneOffReminderSettingsPrompt(settings *domain.OneOffReminderSettings) string {
	return fmt.Sprintf(
		"📝 Пришли интервалы напоминаний для разовых дел в минутах в порядке `низкий,средний,высокий`.\nТекущие значения: `%d,%d,%d`.",
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
		return "завершено"
	}
	return "активно"
}

func oneOffChecklistSummary(task domain.OneOffTask) string {
	if len(task.Items) == 0 {
		if task.Status == domain.OneOffTaskStatusCompleted {
			return "выполнено"
		}
		return "без подпунктов"
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
