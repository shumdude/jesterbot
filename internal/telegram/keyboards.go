package telegram

import (
	"fmt"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	replykbd "github.com/go-telegram/ui/keyboard/reply"

	"jesterbot/internal/domain"
)

func buildMainMenu(b *bot.Bot, r *Router) *replykbd.ReplyKeyboard {
	return replykbd.New(replykbd.ResizableKeyboard()).
		Button("Сегодня", b, bot.MatchTypeExact, r.handleTodayCommand).
		Button("Активности", b, bot.MatchTypeExact, r.handleActivitiesCommand).
		Row().
		Button("Настройки", b, bot.MatchTypeExact, r.handleSettingsCommand).
		Button("Статистика", b, bot.MatchTypeExact, r.handleStatsCommand)
}

func buildActivitiesKeyboard(activities []domain.Activity) models.ReplyMarkup {
	rows := make([][]models.InlineKeyboardButton, 0, len(activities)+1)
	for _, activity := range activities {
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: "✏️ " + activity.Title, CallbackData: fmt.Sprintf("activity:edit:%d", activity.ID)},
			{Text: "🗑 Удалить", CallbackData: fmt.Sprintf("activity:delete:%d", activity.ID)},
		})
	}

	rows = append(rows, []models.InlineKeyboardButton{{Text: "➕ Добавить активность", CallbackData: "activity:add"}})
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func buildPlanSelectionKeyboard(plan *domain.DayPlan) models.ReplyMarkup {
	rows := make([][]models.InlineKeyboardButton, 0, len(plan.Items)+1)
	for _, item := range plan.Items {
		icon := "✅"
		if !item.Selected {
			icon = "⬜"
		}
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: fmt.Sprintf("%s %s", icon, item.TitleSnapshot), CallbackData: fmt.Sprintf("plan:toggle:%d", item.ActivityID)},
		})
	}

	rows = append(rows, []models.InlineKeyboardButton{
		{Text: "Сделаю всё", CallbackData: "plan:all"},
		{Text: "Начать день", CallbackData: "plan:finalize"},
	})

	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func buildProgressKeyboard(plan *domain.DayPlan) models.ReplyMarkup {
	rows := make([][]models.InlineKeyboardButton, 0)
	for _, item := range plan.Items {
		if item.Selected && !item.Completed {
			rows = append(rows, []models.InlineKeyboardButton{
				{Text: "✅ Готово: " + item.TitleSnapshot, CallbackData: fmt.Sprintf("done:%d", item.ActivityID)},
			})
		}
	}
	if len(rows) == 0 {
		return nil
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func buildReminderKeyboard(item *domain.DayPlanItem) models.ReplyMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "✅ Done", CallbackData: fmt.Sprintf("done:%d", item.ActivityID)}},
		},
	}
}

func buildSettingsKeyboard() models.ReplyMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "⏰ Время утра", CallbackData: "settings:morning"}},
			{{Text: "🔁 Интервал напоминаний", CallbackData: "settings:interval"}},
		},
	}
}
