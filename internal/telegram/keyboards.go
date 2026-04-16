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
		Button("📅 Сегодня", b, bot.MatchTypeExact, r.handleTodayCommand).
		Button("🧩 Активности", b, bot.MatchTypeExact, r.handleActivitiesCommand).
		Row().
		Button("📝 Разовые дела", b, bot.MatchTypeExact, r.handleOneOffTasksCommand).
		Button("⚙️ Настройки", b, bot.MatchTypeExact, r.handleSettingsCommand).
		Row().
		Button("📊 Статистика", b, bot.MatchTypeExact, r.handleStatsCommand)
}

func buildActivitiesKeyboard(activities []domain.Activity) models.ReplyMarkup {
	rows := make([][]models.InlineKeyboardButton, 0, len(activities)+2)
	for _, activity := range activities {
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: "✏️ " + activity.Title, CallbackData: fmt.Sprintf("activity:edit:%d", activity.ID)},
			{Text: "🗑 Удалить", CallbackData: fmt.Sprintf("activity:delete:%d", activity.ID)},
		})
	}

	rows = append(rows, []models.InlineKeyboardButton{{Text: "➕ Добавить активность", CallbackData: "activity:add"}})
	rows = append(rows, []models.InlineKeyboardButton{{Text: "⬅️ Назад", CallbackData: "activity:back"}})
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
		{Text: "💪 Сделаю всё", CallbackData: "plan:all"},
		{Text: "🚀 Начать день", CallbackData: "plan:finalize"},
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

func buildSettingsKeyboard() models.ReplyMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "⏰ Время утра", CallbackData: "settings:morning"}},
			{{Text: "🔁 Интервал напоминаний", CallbackData: "settings:interval"}},
			{{Text: "🕒 Частота проверки", CallbackData: "settings:tick"}},
			{{Text: "📝 Интервалы разовых дел", CallbackData: "settings:oneoff"}},
		},
	}
}

func buildOneOffTasksKeyboard(tasks []domain.OneOffTask) models.ReplyMarkup {
	activeTasks, _ := splitOneOffTasks(tasks)
	rows := make([][]models.InlineKeyboardButton, 0, len(activeTasks)+1)
	for _, task := range activeTasks {
		statusIcon := "🟢"
		if task.Status == domain.OneOffTaskStatusCompleted {
			statusIcon = "✅"
		}
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: fmt.Sprintf("%s %s %s", statusIcon, oneOffPriorityIcon(task.Priority), task.Title), CallbackData: fmt.Sprintf("oneoff:open:%d", task.ID)},
			{Text: "🗑", CallbackData: fmt.Sprintf("oneoff:delete:%d", task.ID)},
		})
	}

	rows = append(rows, []models.InlineKeyboardButton{
		{Text: "➕ Добавить разовое дело", CallbackData: "oneoff:add"},
	})

	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func buildOneOffPriorityKeyboard() models.ReplyMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "🟩 Низкий", CallbackData: "oneoff:create:priority:low"},
				{Text: "🟨 Средний", CallbackData: "oneoff:create:priority:medium"},
				{Text: "🟥 Высокий", CallbackData: "oneoff:create:priority:high"},
			},
		},
	}
}

func buildOneOffTaskDetailKeyboard(task *domain.OneOffTask) models.ReplyMarkup {
	rows := make([][]models.InlineKeyboardButton, 0, len(task.Items)+2)
	if task.Status != domain.OneOffTaskStatusCompleted {
		for _, item := range task.Items {
			icon := "⬜"
			if item.Completed {
				icon = "☑️"
			}
			rows = append(rows, []models.InlineKeyboardButton{
				{Text: fmt.Sprintf("%s %s", icon, item.Title), CallbackData: fmt.Sprintf("oneoff:item:%d:%d", task.ID, item.ID)},
			})
		}
	}

	if task.Status != domain.OneOffTaskStatusCompleted {
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: "✅ Завершить дело", CallbackData: fmt.Sprintf("oneoff:complete:%d", task.ID)},
		})
	}
	rows = append(rows, []models.InlineKeyboardButton{
		{Text: "⬅️ К списку", CallbackData: "oneoff:back"},
		{Text: "🗑 Удалить", CallbackData: fmt.Sprintf("oneoff:delete:%d", task.ID)},
	})

	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func buildOneOffReminderKeyboard(task *domain.OneOffTask) models.ReplyMarkup {
	rows := make([][]models.InlineKeyboardButton, 0, len(task.Items)+2)
	for _, item := range task.Items {
		if item.Completed {
			continue
		}
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: "⬜ " + item.Title, CallbackData: fmt.Sprintf("oneoff:item:%d:%d", task.ID, item.ID)},
		})
	}

	rows = append(rows, []models.InlineKeyboardButton{
		{Text: "✅ Завершить дело", CallbackData: fmt.Sprintf("oneoff:complete:%d", task.ID)},
	})
	rows = append(rows, []models.InlineKeyboardButton{
		{Text: "📋 Открыть дело", CallbackData: fmt.Sprintf("oneoff:open:%d", task.ID)},
	})

	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}
