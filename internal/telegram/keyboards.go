package telegram

import (
	"fmt"

	"github.com/go-telegram/bot/models"

	"jesterbot/internal/domain"
)

func buildActivitiesKeyboard(activities []domain.Activity) models.ReplyMarkup {
	return buildActivitiesKeyboardPage(activities, 0, defaultInlinePageSize)
}

func buildActivitiesKeyboardPage(activities []domain.Activity, page, pageSize int) models.ReplyMarkup {
	view := paginate(activities, page, pageSize)
	rows := make([][]models.InlineKeyboardButton, 0, len(view.Items)+3)
	for _, activity := range view.Items {
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: activity.Title, CallbackData: fmt.Sprintf("activity:open:%d:%d", activity.ID, view.Page)},
		})
	}
	if paginationRow := buildPaginationRow("activity:page", view.Page, view.TotalPages); len(paginationRow) > 0 {
		rows = append(rows, paginationRow)
	}

	rows = append(rows, []models.InlineKeyboardButton{{Text: tr("button_add_activity"), CallbackData: "activity:add"}})
	rows = append(rows, mainMenuBackRow())
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func buildActivityDetailKeyboard(activity domain.Activity, page int) models.ReplyMarkup {
	timesPerDay := activity.TimesPerDay
	if timesPerDay < 1 {
		timesPerDay = 1
	}

	rows := [][]models.InlineKeyboardButton{
		{{Text: tr("button_activity_times", timesPerDay), CallbackData: fmt.Sprintf("activity:times:%d:%d", activity.ID, page)}},
		{{Text: activityWindowButton(activity), CallbackData: fmt.Sprintf("activity:window:%d:%d", activity.ID, page)}},
		{{Text: tr("button_delete"), CallbackData: fmt.Sprintf("activity:delete:%d:%d", activity.ID, page)}},
		{{Text: tr("button_back_to_list"), CallbackData: fmt.Sprintf("activity:list:%d", page)}},
		mainMenuBackRow(),
	}

	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func buildPlanSelectionKeyboard(plan *domain.DayPlan) models.ReplyMarkup {
	return buildPlanSelectionKeyboardPage(plan, 0, defaultInlinePageSize)
}

func buildPlanSelectionKeyboardPage(plan *domain.DayPlan, page, pageSize int) models.ReplyMarkup {
	view := paginate(plan.Items, page, pageSize)
	rows := make([][]models.InlineKeyboardButton, 0, len(view.Items)+2)
	for _, item := range view.Items {
		icon := "✅"
		if !item.Selected {
			icon = "⬜"
		}
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: fmt.Sprintf("%s %s", icon, item.TitleSnapshot), CallbackData: fmt.Sprintf("plan:toggle:%d:%d", item.ActivityID, view.Page)},
		})
	}
	if paginationRow := buildPaginationRow("plan:page", view.Page, view.TotalPages); len(paginationRow) > 0 {
		rows = append(rows, paginationRow)
	}

	rows = append(rows, []models.InlineKeyboardButton{
		{Text: tr("button_plan_all"), CallbackData: "plan:all"},
		{Text: tr("button_plan_start"), CallbackData: "plan:finalize"},
	})
	rows = append(rows, mainMenuBackRow())

	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func buildProgressKeyboard(plan *domain.DayPlan) models.ReplyMarkup {
	return buildProgressKeyboardPage(plan, 0, defaultInlinePageSize)
}

func buildProgressKeyboardPage(plan *domain.DayPlan, page, pageSize int) models.ReplyMarkup {
	selectedItems := make([]domain.DayPlanItem, 0, len(plan.Items))
	for _, item := range plan.Items {
		if item.Selected {
			selectedItems = append(selectedItems, item)
		}
	}

	view := paginate(selectedItems, page, pageSize)
	rows := make([][]models.InlineKeyboardButton, 0, len(view.Items)+2)
	for _, item := range view.Items {
		if item.Selected && !item.Completed {
			timesPerDay := item.TimesPerDay
			if timesPerDay < 1 {
				timesPerDay = 1
			}
			var btnText string
			if timesPerDay > 1 {
				btnText = tr("button_done_partial", item.CompletedCount, timesPerDay, item.TitleSnapshot)
			} else {
				btnText = tr("button_done_prefix", item.TitleSnapshot)
			}
			rows = append(rows, []models.InlineKeyboardButton{
				{Text: btnText, CallbackData: fmt.Sprintf("done:%d:%d", item.ActivityID, view.Page)},
			})
		}
	}
	if paginationRow := buildPaginationRow("plan:page", view.Page, view.TotalPages); len(paginationRow) > 0 {
		rows = append(rows, paginationRow)
	}
	rows = append(rows, mainMenuBackRow())
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func buildSettingsKeyboard() models.ReplyMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: tr("button_settings_morning"), CallbackData: "settings:morning"}},
			{{Text: tr("button_settings_interval"), CallbackData: "settings:interval"}},
			{{Text: tr("button_settings_tick"), CallbackData: "settings:tick"}},
			{{Text: tr("button_settings_oneoff"), CallbackData: "settings:oneoff"}},
			mainMenuBackRow(),
		},
	}
}

func buildOneOffTasksKeyboard(tasks []domain.OneOffTask) models.ReplyMarkup {
	return buildOneOffTasksKeyboardPage(tasks, 0, defaultInlinePageSize)
}

func buildOneOffTasksKeyboardPage(tasks []domain.OneOffTask, page, pageSize int) models.ReplyMarkup {
	activeTasks, _ := splitOneOffTasks(tasks)
	view := paginate(activeTasks, page, pageSize)
	rows := make([][]models.InlineKeyboardButton, 0, len(view.Items)+3)
	for _, task := range view.Items {
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: fmt.Sprintf("🟢 %s %s", oneOffPriorityIcon(task.Priority), task.Title), CallbackData: fmt.Sprintf("oneoff:open:%d:%d", task.ID, view.Page)},
			{Text: tr("button_delete_icon"), CallbackData: fmt.Sprintf("oneoff:delete:%d:%d", task.ID, view.Page)},
		})
	}
	if paginationRow := buildPaginationRow("oneoff:page", view.Page, view.TotalPages); len(paginationRow) > 0 {
		rows = append(rows, paginationRow)
	}

	rows = append(rows, []models.InlineKeyboardButton{
		{Text: tr("button_add_oneoff"), CallbackData: "oneoff:add"},
	})
	rows = append(rows, mainMenuBackRow())

	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func buildOneOffPriorityKeyboard() models.ReplyMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: tr("button_priority_low"), CallbackData: "oneoff:create:priority:low"},
				{Text: tr("button_priority_medium"), CallbackData: "oneoff:create:priority:medium"},
				{Text: tr("button_priority_high"), CallbackData: "oneoff:create:priority:high"},
			},
			mainMenuBackRow(),
		},
	}
}

func buildOneOffTaskDetailKeyboard(task *domain.OneOffTask) models.ReplyMarkup {
	return buildOneOffTaskDetailKeyboardPage(task, 0)
}

func buildOneOffTaskDetailKeyboardPage(task *domain.OneOffTask, page int) models.ReplyMarkup {
	rows := make([][]models.InlineKeyboardButton, 0, len(task.Items)+2)
	if task.Status != domain.OneOffTaskStatusCompleted {
		for _, item := range task.Items {
			icon := "⬜"
			if item.Completed {
				icon = "☑️"
			}
			rows = append(rows, []models.InlineKeyboardButton{
				{Text: fmt.Sprintf("%s %s", icon, item.Title), CallbackData: fmt.Sprintf("oneoff:item:%d:%d:%d", task.ID, item.ID, page)},
			})
		}
	}

	if task.Status != domain.OneOffTaskStatusCompleted {
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: tr("button_complete_oneoff"), CallbackData: fmt.Sprintf("oneoff:complete:%d:%d", task.ID, page)},
		})
	}
	rows = append(rows, []models.InlineKeyboardButton{
		{Text: tr("button_back_to_list"), CallbackData: fmt.Sprintf("oneoff:back:%d", page)},
		{Text: tr("button_delete"), CallbackData: fmt.Sprintf("oneoff:delete:%d:%d", task.ID, page)},
	})
	rows = append(rows, mainMenuBackRow())

	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func buildOneOffReminderKeyboard(task *domain.OneOffTask) models.ReplyMarkup {
	return buildOneOffReminderKeyboardPage(task, 0)
}

func buildOneOffReminderKeyboardPage(task *domain.OneOffTask, page int) models.ReplyMarkup {
	rows := make([][]models.InlineKeyboardButton, 0, len(task.Items)+2)
	for _, item := range task.Items {
		if item.Completed {
			continue
		}
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: "⬜ " + item.Title, CallbackData: fmt.Sprintf("oneoff:item:%d:%d:%d", task.ID, item.ID, page)},
		})
	}

	rows = append(rows, []models.InlineKeyboardButton{
		{Text: tr("button_complete_oneoff"), CallbackData: fmt.Sprintf("oneoff:complete:%d:%d", task.ID, page)},
	})
	rows = append(rows, []models.InlineKeyboardButton{
		{Text: tr("button_open_oneoff"), CallbackData: fmt.Sprintf("oneoff:open:%d:%d", task.ID, page)},
	})
	rows = append(rows, mainMenuBackRow())

	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func mainMenuBackRow() []models.InlineKeyboardButton {
	return []models.InlineKeyboardButton{{Text: tr("button_back_to_menu"), CallbackData: "menu:back"}}
}

func activityWindowButton(a domain.Activity) string {
	if a.ReminderWindowStart == "" || a.ReminderWindowEnd == "" {
		return tr("button_activity_window_none")
	}
	return tr("button_activity_window", a.ReminderWindowStart, a.ReminderWindowEnd)
}
