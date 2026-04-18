package telegram

import (
	"context"

	"jesterbot/internal/domain"
)

type Notifier interface {
	ShowMorningPlan(ctx context.Context, chatID int64, plan *domain.DayPlan)
	ShowPlanCompletion(ctx context.Context, chatID, userID int64, plan *domain.DayPlan)
	SendReminder(ctx context.Context, chatID int64, item *domain.DayPlanItem, plan *domain.DayPlan)
	SendOneOffReminder(ctx context.Context, chatID int64, task *domain.OneOffTask)
}

func (r *Controller) ShowMorningPlan(ctx context.Context, chatID int64, plan *domain.DayPlan) {
	r.showScreen(ctx, chatID, selectionText(plan), buildPlanSelectionKeyboard(plan))
}

func (r *Controller) ShowPlanCompletion(ctx context.Context, chatID, userID int64, plan *domain.DayPlan) {
	r.showScreen(ctx, chatID, completionMessage(plan), r.menuMarkup(userID, chatID))
}

func (r *Controller) SendReminder(ctx context.Context, chatID int64, item *domain.DayPlanItem, plan *domain.DayPlan) {
	r.sendMessage(ctx, chatID, reminderText(item, plan), nil)
}

func (r *Controller) SendOneOffReminder(ctx context.Context, chatID int64, task *domain.OneOffTask) {
	r.sendMessage(ctx, chatID, oneOffReminderText(task), buildOneOffReminderKeyboard(task))
}
