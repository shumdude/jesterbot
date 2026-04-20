package telegram

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-telegram/bot/models"
)

const (
	defaultInlinePageSize = 12
	noopCallbackData      = "nav:noop"
)

type pageView[T any] struct {
	Items      []T
	Page       int
	PageSize   int
	TotalItems int
	TotalPages int
	Start      int
	End        int
}

func paginate[T any](items []T, page, pageSize int) pageView[T] {
	if pageSize <= 0 {
		pageSize = defaultInlinePageSize
	}

	totalItems := len(items)
	totalPages := 1
	if totalItems > 0 {
		totalPages = (totalItems + pageSize - 1) / pageSize
	}
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}

	start := page * pageSize
	end := start + pageSize
	if end > totalItems {
		end = totalItems
	}

	view := pageView[T]{
		Page:       page,
		PageSize:   pageSize,
		TotalItems: totalItems,
		TotalPages: totalPages,
		Start:      start,
		End:        end,
	}
	if totalItems == 0 {
		view.Items = []T{}
		return view
	}

	view.Items = items[start:end]
	return view
}

func buildPaginationRow(prefix string, page, totalPages int) []models.InlineKeyboardButton {
	if totalPages <= 1 {
		return nil
	}

	row := make([]models.InlineKeyboardButton, 0, 3)
	if page > 0 {
		row = append(row, models.InlineKeyboardButton{
			Text:         "⬅️",
			CallbackData: fmt.Sprintf("%s:%d", prefix, page-1),
		})
	}

	row = append(row, models.InlineKeyboardButton{
		Text:         fmt.Sprintf("%d/%d", page+1, totalPages),
		CallbackData: noopCallbackData,
	})

	if page < totalPages-1 {
		row = append(row, models.InlineKeyboardButton{
			Text:         "➡️",
			CallbackData: fmt.Sprintf("%s:%d", prefix, page+1),
		})
	}

	return row
}

func pageSummary(page, totalPages, start, end, total int) string {
	return tr("page_summary", page+1, totalPages, start+1, end, total)
}

func parsePageCallback(data string) (int, error) {
	parts := strings.Split(data, ":")
	if len(parts) == 0 {
		return 0, fmt.Errorf("invalid page callback: %s", data)
	}

	page, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 0, fmt.Errorf("parse page callback: %w", err)
	}
	if page < 0 {
		return 0, fmt.Errorf("negative page callback: %s", data)
	}

	return page, nil
}

func parseIDPageCallback(data string) (int64, int, error) {
	parts := strings.Split(data, ":")
	if len(parts) < 3 {
		return 0, 0, fmt.Errorf("invalid id-page callback: %s", data)
	}

	id, err := strconv.ParseInt(parts[len(parts)-2], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse callback id: %w", err)
	}
	page, err := parsePageCallback(data)
	if err != nil {
		return 0, 0, err
	}

	return id, page, nil
}

func parseTwoIDsPageCallback(data string) (int64, int64, int, error) {
	parts := strings.Split(data, ":")
	if len(parts) < 5 {
		return 0, 0, 0, fmt.Errorf("invalid two-id-page callback: %s", data)
	}

	firstID, err := strconv.ParseInt(parts[len(parts)-3], 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse first callback id: %w", err)
	}
	secondID, err := strconv.ParseInt(parts[len(parts)-2], 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse second callback id: %w", err)
	}
	page, err := parsePageCallback(data)
	if err != nil {
		return 0, 0, 0, err
	}

	return firstID, secondID, page, nil
}

func parseActivityTimesCallback(data, action string) (int64, int, int, error) {
	parts := strings.Split(data, ":")
	if len(parts) != 6 || parts[0] != "activity" || parts[1] != "times" || parts[2] != action {
		return 0, 0, 0, fmt.Errorf("invalid activity times callback: %s", data)
	}

	activityID, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse activity id: %w", err)
	}

	page, err := strconv.Atoi(parts[4])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse activity page: %w", err)
	}
	if page < 0 {
		return 0, 0, 0, fmt.Errorf("negative activity page: %s", data)
	}

	timesPerDay, err := strconv.Atoi(parts[5])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse activity times: %w", err)
	}
	if timesPerDay < 1 {
		timesPerDay = 1
	}

	return activityID, page, timesPerDay, nil
}
