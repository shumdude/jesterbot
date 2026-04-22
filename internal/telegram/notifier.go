package telegram

import (
	"context"
	"time"

	"jesterbot/internal/domain"
)

type Notifier interface {
	ShowMorningPlan(ctx context.Context, chatID int64, plan *domain.DayPlan)
	ShowPlanCompletion(ctx context.Context, chatID, telegramUserID int64, plan *domain.DayPlan)
	SendReminder(ctx context.Context, userID, chatID int64, item *domain.DayPlanItem, plan *domain.DayPlan)
	SendOneOffReminder(ctx context.Context, userID, chatID int64, dayLocal string, task *domain.OneOffTask)
	CleanupReminderMessages(ctx context.Context, userID int64, messages []domain.ReminderMessage, delay time.Duration) chatCleanupResult
}

func (r *Controller) ShowMorningPlan(ctx context.Context, chatID int64, plan *domain.DayPlan) {
	r.showScreen(ctx, chatID, selectionText(plan), buildPlanSelectionKeyboard(plan))
}

func (r *Controller) ShowPlanCompletion(ctx context.Context, chatID, telegramUserID int64, plan *domain.DayPlan) {
	r.showScreen(ctx, chatID, completionMessage(plan), r.menuMarkup(telegramUserID, chatID))
}

func (r *Controller) SendReminder(ctx context.Context, userID, chatID int64, item *domain.DayPlanItem, plan *domain.DayPlan) {
	messageID := r.sendMessage(ctx, chatID, reminderText(item, plan), nil)
	if messageID == 0 {
		return
	}
	if r.service == nil {
		return
	}
	if err := r.service.TrackReminderMessage(ctx, userID, chatID, messageID, plan.DayLocal, domain.ReminderMessageKindDaily); err != nil {
		r.logger.Error("track daily reminder message failed", "error", err, "user_id", userID, "chat_id", chatID, "message_id", messageID)
	}
}

func (r *Controller) SendOneOffReminder(ctx context.Context, userID, chatID int64, dayLocal string, task *domain.OneOffTask) {
	messageID := r.sendMessage(ctx, chatID, oneOffReminderText(task), nil)
	if messageID == 0 {
		return
	}
	if r.service == nil {
		return
	}
	if err := r.service.TrackReminderMessage(ctx, userID, chatID, messageID, dayLocal, domain.ReminderMessageKindOneOff); err != nil {
		r.logger.Error("track one-off reminder message failed", "error", err, "user_id", userID, "chat_id", chatID, "message_id", messageID)
	}
}

func (r *Controller) CleanupReminderMessages(ctx context.Context, userID int64, messages []domain.ReminderMessage, delay time.Duration) chatCleanupResult {
	result := clearMessageIDsWithDelay(ctx, messages, r.deleteMessage, time.Sleep, delay)
	if r.service == nil {
		return result
	}
	for _, message := range result.DeletedMessages {
		if err := r.service.RemoveTrackedReminderMessage(ctx, userID, message.MessageID); err != nil {
			r.logger.Error("remove tracked reminder message failed", "error", err, "user_id", userID, "chat_id", message.ChatID, "message_id", message.MessageID)
		}
	}
	return result
}
