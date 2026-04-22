package telegram

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
	tgamlengine "github.com/shumdude/tgaml/pkg/engine"

	"jesterbot/cmd/jesterbot/botconfig"
	"jesterbot/internal/domain"
	"jesterbot/internal/service"
	"jesterbot/internal/telegram/constants"
	"jesterbot/internal/telegram/session_backend"
)

func TestMenuMarkupUsesTelegramUserIDForRegisteredUser(t *testing.T) {
	cfg, err := botconfig.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	repo := &menuMarkupRepo{
		userByTelegramID: map[int64]*domain.User{
			777001: {
				ID:             42,
				TelegramUserID: 777001,
				ChatID:         9001,
				Name:           "Alexey",
			},
		},
	}
	svc := service.New(repo, 30)
	eng := tgamlengine.New(cfg, session_backend.New(constants.SceneMenu))
	registerShowIf(eng, svc)

	controller := &Controller{service: svc, eng: eng}
	markup := controller.menuMarkup(777001, 9001)
	buttons := collectButtonTexts(t, markup)

	if contains(buttons, "Регистрация") {
		t.Fatalf("expected registered menu without registration button, got %v", buttons)
	}
	if !containsSubstring(buttons, "Сегодня") {
		t.Fatalf("expected registered menu buttons, got %v", buttons)
	}
	if !containsSubstring(buttons, "Закончить день") {
		t.Fatalf("expected finish day button, got %v", buttons)
	}
}

func TestOneOffItemsSceneKeyboardIncludesNoSubitemsButton(t *testing.T) {
	cfg, err := botconfig.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	repo := &menuMarkupRepo{
		userByTelegramID: map[int64]*domain.User{
			777001: {
				ID:             42,
				TelegramUserID: 777001,
				ChatID:         9001,
				Name:           "Alexey",
			},
		},
	}
	svc := service.New(repo, 30)
	eng := tgamlengine.New(cfg, session_backend.New(constants.SceneMenu))
	registerShowIf(eng, svc)

	controller := &Controller{service: svc, eng: eng}
	markup := controller.sceneKeyboardMarkup("oneoff_items_back_menu", 777001, 9001)
	buttons := collectButtonTexts(t, markup)

	if !containsSubstring(buttons, "Без подпунктов") {
		t.Fatalf("expected no-subitems button in one-off items keyboard, got %v", buttons)
	}
}

type menuMarkupRepo struct {
	userByTelegramID map[int64]*domain.User
}

func (r *menuMarkupRepo) GetUserByTelegramID(_ context.Context, telegramUserID int64) (*domain.User, error) {
	user, ok := r.userByTelegramID[telegramUserID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return user, nil
}

func (r *menuMarkupRepo) GetUserByID(context.Context, int64) (*domain.User, error) {
	return nil, domain.ErrNotFound
}

func (r *menuMarkupRepo) ListUsers(context.Context) ([]domain.User, error) {
	return nil, nil
}

func (r *menuMarkupRepo) CreateUser(context.Context, *domain.User) error {
	return nil
}

func (r *menuMarkupRepo) UpdateUserSettings(context.Context, int64, string, string, int) error {
	return nil
}

func (r *menuMarkupRepo) UpdateUserNotificationsPausedUntil(context.Context, int64, *time.Time) error {
	return nil
}

func (r *menuMarkupRepo) GetUserTickInterval(context.Context, int64) (int, error) {
	return 1, nil
}

func (r *menuMarkupRepo) SaveUserTickInterval(context.Context, int64, int) error {
	return nil
}

func (r *menuMarkupRepo) CreateActivity(context.Context, *domain.Activity) error {
	return nil
}

func (r *menuMarkupRepo) UpdateActivity(context.Context, int64, int64, string) error {
	return nil
}

func (r *menuMarkupRepo) UpdateActivityTimesPerDay(context.Context, int64, int64, int) error {
	return nil
}

func (r *menuMarkupRepo) UpdateActivityReminderWindows(context.Context, int64, int64, []domain.ReminderWindow) error {
	return nil
}

func (r *menuMarkupRepo) DeleteActivity(context.Context, int64, int64) error {
	return nil
}

func (r *menuMarkupRepo) ListActivities(context.Context, int64) ([]domain.Activity, error) {
	return nil, nil
}

func (r *menuMarkupRepo) GetDayPlan(context.Context, int64, string) (*domain.DayPlan, error) {
	return nil, domain.ErrNotFound
}

func (r *menuMarkupRepo) SaveDayPlan(context.Context, *domain.DayPlan) error {
	return nil
}

func (r *menuMarkupRepo) ListPlans(context.Context, int64) ([]domain.DayPlan, error) {
	return nil, nil
}

func (r *menuMarkupRepo) GetOneOffReminderSettings(context.Context, int64) (*domain.OneOffReminderSettings, error) {
	return nil, domain.ErrNotFound
}

func (r *menuMarkupRepo) SaveOneOffReminderSettings(context.Context, *domain.OneOffReminderSettings) error {
	return nil
}

func (r *menuMarkupRepo) GetOneOffTask(context.Context, int64, int64) (*domain.OneOffTask, error) {
	return nil, domain.ErrNotFound
}

func (r *menuMarkupRepo) ListOneOffTasks(context.Context, int64) ([]domain.OneOffTask, error) {
	return nil, nil
}

func (r *menuMarkupRepo) SaveOneOffTask(context.Context, *domain.OneOffTask) error {
	return nil
}

func (r *menuMarkupRepo) DeleteOneOffTask(context.Context, int64, int64) error {
	return nil
}

func (r *menuMarkupRepo) SaveReminderMessage(context.Context, *domain.ReminderMessage) error {
	return nil
}

func (r *menuMarkupRepo) ListReminderMessagesBeforeDay(context.Context, int64, string) ([]domain.ReminderMessage, error) {
	return nil, nil
}

func (r *menuMarkupRepo) DeleteReminderMessage(context.Context, int64, int) error {
	return nil
}

func collectButtonTexts(t *testing.T, markup models.ReplyMarkup) []string {
	t.Helper()

	switch typed := markup.(type) {
	case *models.ReplyKeyboardMarkup:
		var texts []string
		for _, row := range typed.Keyboard {
			for _, button := range row {
				texts = append(texts, button.Text)
			}
		}
		return texts
	case *models.InlineKeyboardMarkup:
		var texts []string
		for _, row := range typed.InlineKeyboard {
			for _, button := range row {
				texts = append(texts, button.Text)
			}
		}
		return texts
	default:
		t.Fatalf("unexpected markup type %T", markup)
		return nil
	}
}

func contains(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}

func containsSubstring(items []string, needle string) bool {
	for _, item := range items {
		if strings.Contains(item, needle) {
			return true
		}
	}
	return false
}
