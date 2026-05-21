package telegram

import (
	"context"
	"errors"
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

func RegisterTgamlHandlers(eng *tgamlengine.Engine, svc *service.Service, ui *Controller) {
	eng.Register(constants.HandlerRegistrationName, registrationNameHandler(eng))
	eng.Register(constants.HandlerRegistrationOffset, registrationOffsetHandler(eng))
	eng.Register(constants.HandlerRegistrationMorning, registrationMorningHandler(eng, svc, ui))
	eng.Register(constants.HandlerOpenToday, openLegacyScreenHandler(eng, svc, ui.OpenToday))
	eng.Register(constants.HandlerOpenActivities, openLegacyScreenHandler(eng, svc, ui.OpenActivities))
	eng.Register(constants.HandlerOpenShop, openLegacyScreenHandler(eng, svc, ui.OpenShop))
	eng.Register(constants.HandlerOpenOneOff, openLegacyScreenHandler(eng, svc, ui.OpenOneOffTasks))
	eng.Register(constants.HandlerOpenSettings, openLegacyScreenHandler(eng, svc, ui.OpenSettings))
	eng.Register(constants.HandlerOpenStats, openLegacyScreenHandler(eng, svc, ui.OpenStats))
	eng.Register(constants.HandlerFinishDay, finishDayHandler(eng, svc, ui))
	eng.Register(constants.HandlerBackActivityDetail, backActivityDetailHandler(svc, ui))
	eng.Register(constants.HandlerBackOneOffPriority, backOneOffPriorityHandler(ui))
	eng.Register(constants.HandlerBackOneOffReward, backOneOffRewardHandler(ui))
	eng.Register(constants.HandlerAddActivity, addActivityHandler(svc, ui))
	eng.Register(constants.HandlerEditActivity, editActivityHandler(svc, ui))
	eng.Register(constants.HandlerSetActivityTimes, activityTimesHandler(svc, ui))
	eng.Register(constants.HandlerSetActivityWindow, activityWindowHandler(svc, ui))
	eng.Register(constants.HandlerAddShopItem, addShopItemHandler(svc, ui))
	eng.Register(constants.HandlerEditShopItem, editShopItemHandler(svc, ui))
	eng.Register(constants.HandlerUpdateMorning, updateMorningHandler(svc, ui))
	eng.Register(constants.HandlerUpdateDayEnd, updateDayEndHandler(svc, ui))
	eng.Register(constants.HandlerUpdateReminder, updateReminderHandler(svc, ui))
	eng.Register(constants.HandlerUpdateTick, updateTickHandler(svc, ui))
	eng.Register(constants.HandlerUpdateOneOffReminder, updateOneOffReminderHandler(svc, ui))
	eng.Register(constants.HandlerOneOffTitle, oneOffTitleHandler(ui))
	eng.Register(constants.HandlerOneOffReward, oneOffRewardHandler(ui))
	eng.Register(constants.HandlerOneOffItems, oneOffItemsHandler(svc, ui))
	eng.Register(constants.HandlerOneOffNoItems, oneOffNoItemsHandler(svc, ui))
}

func finishDayHandler(eng *tgamlengine.Engine, svc *service.Service, ui *Controller) tgamlengine.HandlerFunc {
	return func(ctx context.Context, _ *bot.Bot, _ *models.Update, s *tgamlsession.Session) (string, error) {
		user, err := svc.FindUserByTelegramID(ctx, s.UserID)
		if err != nil {
			ui.handleRegistrationRequired(ctx, s.ChatID, s.UserID)
			return "", nil
		}
		if _, err := svc.FinishDay(ctx, user.ID, time.Now().UTC()); err != nil {
			ui.showScreen(ctx, s.ChatID, tr("finish_day_error", err.Error()), ui.menuMarkup(s.UserID, s.ChatID))
			return "", nil
		}

		_ = s.Transition(ctx, constants.SceneMenu)
		ui.showScreen(ctx, s.ChatID, eng.T("messages.menu.day_finished"), ui.menuMarkup(s.UserID, s.ChatID))
		return "", nil
	}
}

func backActivityDetailHandler(svc *service.Service, ui *Controller) tgamlengine.HandlerFunc {
	return func(ctx context.Context, _ *bot.Bot, _ *models.Update, s *tgamlsession.Session) (string, error) {
		user, err := svc.FindUserByTelegramID(ctx, s.UserID)
		if err != nil {
			ui.handleRegistrationRequired(ctx, s.ChatID, s.UserID)
			return "", nil
		}
		activityID, err := sessionInt64(s, constants.NSActivity, constants.KeyActivityID)
		if err != nil {
			ui.showActivities(ctx, s.ChatID, user.ID, tr("activity_title"))
			return "", nil
		}
		page := sessionInt(s, constants.NSActivity, constants.KeyActivityPage)
		_ = s.Transition(ctx, constants.SceneMenu)
		ui.showActivityDetail(ctx, s.ChatID, user.ID, activityID, page, tr("activity_detail_title"))
		return "", nil
	}
}

func backOneOffPriorityHandler(ui *Controller) tgamlengine.HandlerFunc {
	return func(ctx context.Context, _ *bot.Bot, _ *models.Update, s *tgamlsession.Session) (string, error) {
		title := strings.TrimSpace(s.GetStr(constants.NSOneOff, constants.KeyTaskTitle))
		if title == "" {
			_ = s.Transition(ctx, constants.SceneMenu)
			ui.OpenOneOffTasks(ctx, s.ChatID, s.UserID)
			return "", nil
		}
		if err := s.Transition(ctx, constants.SceneAddOneOffPriority); err != nil {
			return "", err
		}
		ui.showScreen(ctx, s.ChatID, tr("oneoff_prompt_priority"), buildOneOffPriorityKeyboard())
		return "", nil
	}
}

func backOneOffRewardHandler(ui *Controller) tgamlengine.HandlerFunc {
	return func(ctx context.Context, _ *bot.Bot, _ *models.Update, s *tgamlsession.Session) (string, error) {
		title := strings.TrimSpace(s.GetStr(constants.NSOneOff, constants.KeyTaskTitle))
		priority := strings.TrimSpace(s.GetStr(constants.NSOneOff, constants.KeyPriority))
		if title == "" || priority == "" {
			if err := s.Transition(ctx, constants.SceneAddOneOffPriority); err != nil {
				return "", err
			}
			ui.showScreen(ctx, s.ChatID, tr("oneoff_prompt_priority"), buildOneOffPriorityKeyboard())
			return "", nil
		}
		if err := s.Transition(ctx, constants.SceneAddOneOffReward); err != nil {
			return "", err
		}
		ui.showScreen(ctx, s.ChatID, tr("oneoff_prompt_reward"), ui.sceneKeyboardMarkup("oneoff_reward_back_menu", s.UserID, s.ChatID))
		return "", nil
	}
}

func registrationNameHandler(eng *tgamlengine.Engine) tgamlengine.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, u *models.Update, s *tgamlsession.Session) (string, error) {
		text := strings.TrimSpace(u.Message.Text)
		if text == "" {
			_, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: s.ChatID, Text: eng.T("messages.registration.error_name")})
			return "", err
		}
		if err := s.SetStr(constants.NSRegistration, constants.KeyName, text); err != nil {
			return "", err
		}
		return transitionAndRenderScene(ctx, b, eng, s, constants.SceneRegOffset)
	}
}

func registrationOffsetHandler(eng *tgamlengine.Engine) tgamlengine.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, u *models.Update, s *tgamlsession.Session) (string, error) {
		text := strings.TrimSpace(u.Message.Text)
		if _, err := service.ParseUTCOffset(text); err != nil {
			_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: s.ChatID, Text: eng.T("messages.registration.error_offset")})
			return "", sendErr
		}
		if err := s.SetStr(constants.NSRegistration, constants.KeyUTCOffset, text); err != nil {
			return "", err
		}
		return transitionAndRenderScene(ctx, b, eng, s, constants.SceneRegMorning)
	}
}

func registrationMorningHandler(eng *tgamlengine.Engine, svc *service.Service, ui *Controller) tgamlengine.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, u *models.Update, s *tgamlsession.Session) (string, error) {
		user, err := svc.RegisterUser(ctx, service.RegistrationInput{
			TelegramUserID: u.Message.From.ID,
			ChatID:         s.ChatID,
			Name:           s.GetStr(constants.NSRegistration, constants.KeyName),
			UTCOffset:      s.GetStr(constants.NSRegistration, constants.KeyUTCOffset),
			MorningTime:    strings.TrimSpace(u.Message.Text),
		})
		if err != nil {
			_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: s.ChatID,
				Text:   eng.Render("messages.registration.error_finish", map[string]string{"error": err.Error()}),
			})
			return "", sendErr
		}
		if err := s.ClearNamespace(constants.NSRegistration); err != nil {
			return "", err
		}
		if err := s.Transition(ctx, constants.SceneMenu); err != nil {
			return "", err
		}
		ui.ShowWelcome(ctx, s.ChatID, user)
		return "", nil
	}
}

func openLegacyScreenHandler(eng *tgamlengine.Engine, svc *service.Service, open func(context.Context, int64, int64)) tgamlengine.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, u *models.Update, s *tgamlsession.Session) (string, error) {
		_, err := svc.FindUserByTelegramID(ctx, s.UserID)
		switch {
		case err == nil:
			if u != nil && u.CallbackQuery != nil && u.CallbackQuery.Message.Message != nil {
				_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
					ChatID:    s.ChatID,
					MessageID: u.CallbackQuery.Message.Message.ID,
				})
			}
			open(ctx, s.ChatID, s.UserID)
			return "", nil
		case errors.Is(err, domain.ErrNotFound):
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: s.ChatID, Text: eng.T("messages.common.registration_required")})
			return transitionAndRenderScene(ctx, b, eng, s, constants.SceneRegName)
		default:
			_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: s.ChatID, Text: eng.T("messages.common.registration_check_failed")})
			return "", sendErr
		}
	}
}

func addActivityHandler(svc *service.Service, ui *Controller) tgamlengine.HandlerFunc {
	return func(ctx context.Context, _ *bot.Bot, u *models.Update, s *tgamlsession.Session) (string, error) {
		user, err := svc.FindUserByTelegramID(ctx, s.UserID)
		if err != nil {
			ui.handleRegistrationRequired(ctx, s.ChatID, s.UserID)
			return "", nil
		}
		activities, err := svc.AddActivities(ctx, user.ID, strings.TrimSpace(u.Message.Text))
		if err != nil {
			ui.showScreen(ctx, s.ChatID, tr("activity_error_add", err.Error()), nil)
			return "", nil
		}
		_ = s.ClearNamespace(constants.NSActivity)
		_ = s.Transition(ctx, constants.SceneMenu)
		prefix := tr("activity_success_add_one")
		if len(activities) > 1 {
			prefix = tr("activity_success_add_many", len(activities))
		}
		ui.showActivities(ctx, s.ChatID, user.ID, prefix)
		return "", nil
	}
}

func editActivityHandler(svc *service.Service, ui *Controller) tgamlengine.HandlerFunc {
	return func(ctx context.Context, _ *bot.Bot, u *models.Update, s *tgamlsession.Session) (string, error) {
		user, err := svc.FindUserByTelegramID(ctx, s.UserID)
		if err != nil {
			ui.handleRegistrationRequired(ctx, s.ChatID, s.UserID)
			return "", nil
		}
		activityID, err := sessionInt64(s, constants.NSActivity, constants.KeyActivityID)
		if err != nil {
			ui.showScreen(ctx, s.ChatID, tr("activity_error_update", err.Error()), nil)
			return "", nil
		}
		page := sessionInt(s, constants.NSActivity, constants.KeyActivityPage)
		if err := svc.UpdateActivity(ctx, user.ID, activityID, strings.TrimSpace(u.Message.Text)); err != nil {
			ui.showScreen(ctx, s.ChatID, tr("activity_error_update", err.Error()), nil)
			return "", nil
		}
		_ = s.ClearNamespace(constants.NSActivity)
		_ = s.Transition(ctx, constants.SceneMenu)
		ui.showActivityDetail(ctx, s.ChatID, user.ID, activityID, page, tr("activity_success_update"))
		return "", nil
	}
}

func activityTimesHandler(svc *service.Service, ui *Controller) tgamlengine.HandlerFunc {
	return func(ctx context.Context, _ *bot.Bot, u *models.Update, s *tgamlsession.Session) (string, error) {
		user, err := svc.FindUserByTelegramID(ctx, s.UserID)
		if err != nil {
			ui.handleRegistrationRequired(ctx, s.ChatID, s.UserID)
			return "", nil
		}
		activityID, err := sessionInt64(s, constants.NSActivity, constants.KeyActivityID)
		if err != nil {
			ui.showScreen(ctx, s.ChatID, tr("activity_error_times", err.Error()), nil)
			return "", nil
		}
		page := sessionInt(s, constants.NSActivity, constants.KeyActivityPage)
		times, parseErr := strconv.Atoi(strings.TrimSpace(u.Message.Text))
		if parseErr != nil || times < 1 {
			ui.showScreen(ctx, s.ChatID, tr("activity_error_invalid_times"), nil)
			return "", nil
		}
		if err := svc.SetActivityTimesPerDay(ctx, user.ID, activityID, times); err != nil {
			ui.showScreen(ctx, s.ChatID, tr("activity_error_times", err.Error()), nil)
			return "", nil
		}
		_ = s.ClearNamespace(constants.NSActivity)
		_ = s.Transition(ctx, constants.SceneMenu)
		ui.showActivityDetail(ctx, s.ChatID, user.ID, activityID, page, tr("activity_success_times"))
		return "", nil
	}
}

func activityWindowHandler(svc *service.Service, ui *Controller) tgamlengine.HandlerFunc {
	return func(ctx context.Context, _ *bot.Bot, u *models.Update, s *tgamlsession.Session) (string, error) {
		user, err := svc.FindUserByTelegramID(ctx, s.UserID)
		if err != nil {
			ui.handleRegistrationRequired(ctx, s.ChatID, s.UserID)
			return "", nil
		}
		activityID, err := sessionInt64(s, constants.NSActivity, constants.KeyActivityID)
		if err != nil {
			ui.showScreen(ctx, s.ChatID, tr("activity_error_window", err.Error()), nil)
			return "", nil
		}
		page := sessionInt(s, constants.NSActivity, constants.KeyActivityPage)
		var windows []domain.ReminderWindow
		if strings.TrimSpace(u.Message.Text) != "-" {
			windows, err = parseWindowInput(u.Message.Text)
			if err != nil {
				ui.showScreen(ctx, s.ChatID, tr("activity_error_invalid_window"), nil)
				return "", nil
			}
		}
		if err := svc.SetActivityReminderWindows(ctx, user.ID, activityID, windows); err != nil {
			ui.showScreen(ctx, s.ChatID, tr("activity_error_window", err.Error()), nil)
			return "", nil
		}
		_ = s.ClearNamespace(constants.NSActivity)
		_ = s.Transition(ctx, constants.SceneMenu)
		ui.showActivityDetail(ctx, s.ChatID, user.ID, activityID, page, tr("activity_success_window"))
		return "", nil
	}
}

func addShopItemHandler(svc *service.Service, ui *Controller) tgamlengine.HandlerFunc {
	return func(ctx context.Context, _ *bot.Bot, u *models.Update, s *tgamlsession.Session) (string, error) {
		user, err := svc.FindUserByTelegramID(ctx, s.UserID)
		if err != nil {
			ui.handleRegistrationRequired(ctx, s.ChatID, s.UserID)
			return "", nil
		}
		title, cost, err := parseShopItemInput(u.Message.Text)
		if err != nil {
			ui.showScreen(ctx, s.ChatID, tr("shop_error_invalid_item"), nil)
			return "", nil
		}
		item, err := svc.SaveShopItem(ctx, user.ID, 0, title, cost)
		if err != nil {
			ui.showScreen(ctx, s.ChatID, tr("shop_error_save", err.Error()), nil)
			return "", nil
		}
		_ = s.ClearNamespace(constants.NSShop)
		_ = s.Transition(ctx, constants.SceneMenu)
		ui.showShopEditPage(ctx, s.ChatID, user.ID, tr("shop_success_add", item.Title), 0)
		return "", nil
	}
}

func editShopItemHandler(svc *service.Service, ui *Controller) tgamlengine.HandlerFunc {
	return func(ctx context.Context, _ *bot.Bot, u *models.Update, s *tgamlsession.Session) (string, error) {
		user, err := svc.FindUserByTelegramID(ctx, s.UserID)
		if err != nil {
			ui.handleRegistrationRequired(ctx, s.ChatID, s.UserID)
			return "", nil
		}
		itemID, err := sessionInt64(s, constants.NSShop, constants.KeyShopItemID)
		if err != nil {
			ui.showScreen(ctx, s.ChatID, tr("shop_error_save", err.Error()), nil)
			return "", nil
		}
		page := sessionInt(s, constants.NSShop, constants.KeyShopItemPage)
		title, cost, err := parseShopItemInput(u.Message.Text)
		if err != nil {
			ui.showScreen(ctx, s.ChatID, tr("shop_error_invalid_item"), nil)
			return "", nil
		}
		item, err := svc.SaveShopItem(ctx, user.ID, itemID, title, cost)
		if err != nil {
			ui.showScreen(ctx, s.ChatID, tr("shop_error_save", err.Error()), nil)
			return "", nil
		}
		_ = s.ClearNamespace(constants.NSShop)
		_ = s.Transition(ctx, constants.SceneMenu)
		ui.showShopEditPage(ctx, s.ChatID, user.ID, tr("shop_success_update", item.Title), page)
		return "", nil
	}
}

func updateMorningHandler(svc *service.Service, ui *Controller) tgamlengine.HandlerFunc {
	return func(ctx context.Context, _ *bot.Bot, u *models.Update, s *tgamlsession.Session) (string, error) {
		user, err := svc.FindUserByTelegramID(ctx, s.UserID)
		if err != nil {
			ui.handleRegistrationRequired(ctx, s.ChatID, s.UserID)
			return "", nil
		}
		if err := svc.UpdateSettings(ctx, user.ID, strings.TrimSpace(u.Message.Text), user.DayEndTime, user.ReminderIntervalMinutes); err != nil {
			ui.showScreen(ctx, s.ChatID, tr("settings_error_update_morning", err.Error()), nil)
			return "", nil
		}
		_ = s.Transition(ctx, constants.SceneMenu)
		ui.showSettings(ctx, s.ChatID, s.UserID, tr("settings_success_morning"))
		return "", nil
	}
}

func updateDayEndHandler(svc *service.Service, ui *Controller) tgamlengine.HandlerFunc {
	return func(ctx context.Context, _ *bot.Bot, u *models.Update, s *tgamlsession.Session) (string, error) {
		user, err := svc.FindUserByTelegramID(ctx, s.UserID)
		if err != nil {
			ui.handleRegistrationRequired(ctx, s.ChatID, s.UserID)
			return "", nil
		}
		if err := svc.UpdateSettings(ctx, user.ID, user.MorningTime, strings.TrimSpace(u.Message.Text), user.ReminderIntervalMinutes); err != nil {
			ui.showScreen(ctx, s.ChatID, tr("settings_error_update_day_end", err.Error()), nil)
			return "", nil
		}
		_ = s.Transition(ctx, constants.SceneMenu)
		ui.showSettings(ctx, s.ChatID, s.UserID, tr("settings_success_day_end"))
		return "", nil
	}
}

func updateReminderHandler(svc *service.Service, ui *Controller) tgamlengine.HandlerFunc {
	return func(ctx context.Context, _ *bot.Bot, u *models.Update, s *tgamlsession.Session) (string, error) {
		user, err := svc.FindUserByTelegramID(ctx, s.UserID)
		if err != nil {
			ui.handleRegistrationRequired(ctx, s.ChatID, s.UserID)
			return "", nil
		}
		minutes, err := strconv.Atoi(strings.TrimSpace(u.Message.Text))
		if err != nil || minutes <= 0 {
			ui.showScreen(ctx, s.ChatID, tr("settings_error_invalid_minutes"), nil)
			return "", nil
		}
		if err := svc.UpdateSettings(ctx, user.ID, user.MorningTime, user.DayEndTime, minutes); err != nil {
			ui.showScreen(ctx, s.ChatID, tr("settings_error_update_interval", err.Error()), nil)
			return "", nil
		}
		_ = s.Transition(ctx, constants.SceneMenu)
		ui.showSettings(ctx, s.ChatID, s.UserID, tr("settings_success_interval"))
		return "", nil
	}
}

func updateTickHandler(svc *service.Service, ui *Controller) tgamlengine.HandlerFunc {
	return func(ctx context.Context, _ *bot.Bot, u *models.Update, s *tgamlsession.Session) (string, error) {
		user, err := svc.FindUserByTelegramID(ctx, s.UserID)
		if err != nil {
			ui.handleRegistrationRequired(ctx, s.ChatID, s.UserID)
			return "", nil
		}
		minutes, err := strconv.Atoi(strings.TrimSpace(u.Message.Text))
		if err != nil || minutes <= 0 {
			ui.showScreen(ctx, s.ChatID, tr("settings_error_invalid_tick"), nil)
			return "", nil
		}
		if err := svc.UpdateUserTickInterval(ctx, user.ID, minutes); err != nil {
			ui.showScreen(ctx, s.ChatID, tr("settings_error_update_tick", err.Error()), nil)
			return "", nil
		}
		_ = s.Transition(ctx, constants.SceneMenu)
		ui.showSettings(ctx, s.ChatID, s.UserID, tr("settings_success_tick"))
		return "", nil
	}
}

func updateOneOffReminderHandler(svc *service.Service, ui *Controller) tgamlengine.HandlerFunc {
	return func(ctx context.Context, _ *bot.Bot, u *models.Update, s *tgamlsession.Session) (string, error) {
		user, err := svc.FindUserByTelegramID(ctx, s.UserID)
		if err != nil {
			ui.handleRegistrationRequired(ctx, s.ChatID, s.UserID)
			return "", nil
		}
		low, medium, high, err := parseOneOffReminderSettingsInput(u.Message.Text)
		if err != nil {
			ui.showScreen(ctx, s.ChatID, tr("settings_error_invalid_oneoff"), nil)
			return "", nil
		}
		if err := svc.UpdateOneOffReminderSettings(ctx, user.ID, low, medium, high); err != nil {
			ui.showScreen(ctx, s.ChatID, tr("settings_error_update_oneoff", err.Error()), nil)
			return "", nil
		}
		_ = s.Transition(ctx, constants.SceneMenu)
		ui.showSettings(ctx, s.ChatID, s.UserID, tr("settings_success_oneoff"))
		return "", nil
	}
}

func oneOffTitleHandler(ui *Controller) tgamlengine.HandlerFunc {
	return func(ctx context.Context, _ *bot.Bot, u *models.Update, s *tgamlsession.Session) (string, error) {
		title := strings.TrimSpace(u.Message.Text)
		if title == "" {
			ui.showScreen(ctx, s.ChatID, tr("oneoff_error_empty_title"), nil)
			return "", nil
		}
		if err := s.SetStr(constants.NSOneOff, constants.KeyTaskTitle, title); err != nil {
			return "", err
		}
		if err := s.Transition(ctx, constants.SceneAddOneOffPriority); err != nil {
			return "", err
		}
		ui.showScreen(ctx, s.ChatID, tr("oneoff_prompt_priority"), buildOneOffPriorityKeyboard())
		return "", nil
	}
}

func oneOffRewardHandler(ui *Controller) tgamlengine.HandlerFunc {
	return func(ctx context.Context, _ *bot.Bot, u *models.Update, s *tgamlsession.Session) (string, error) {
		reward, err := parseOneOffRewardInput(u.Message.Text)
		if err != nil {
			ui.showScreen(ctx, s.ChatID, tr("oneoff_error_invalid_reward"), nil)
			return "", nil
		}
		if err := s.SetStr(constants.NSOneOff, constants.KeyReward, strconv.Itoa(reward)); err != nil {
			return "", err
		}
		if err := s.Transition(ctx, constants.SceneAddOneOffItems); err != nil {
			return "", err
		}
		ui.showScreen(ctx, s.ChatID, tr("oneoff_prompt_items"), ui.sceneKeyboardMarkup("oneoff_items_back_menu", s.UserID, s.ChatID))
		return "", nil
	}
}

func oneOffItemsHandler(svc *service.Service, ui *Controller) tgamlengine.HandlerFunc {
	return func(ctx context.Context, _ *bot.Bot, u *models.Update, s *tgamlsession.Session) (string, error) {
		return createOneOffTaskWithChecklist(ctx, svc, ui, s, parseOneOffChecklistInput(u.Message.Text))
	}
}

func oneOffNoItemsHandler(svc *service.Service, ui *Controller) tgamlengine.HandlerFunc {
	return func(ctx context.Context, _ *bot.Bot, _ *models.Update, s *tgamlsession.Session) (string, error) {
		return createOneOffTaskWithChecklist(ctx, svc, ui, s, nil)
	}
}

func createOneOffTaskWithChecklist(ctx context.Context, svc *service.Service, ui *Controller, s *tgamlsession.Session, checklist []string) (string, error) {
	user, err := svc.FindUserByTelegramID(ctx, s.UserID)
	if err != nil {
		ui.handleRegistrationRequired(ctx, s.ChatID, s.UserID)
		return "", nil
	}
	priority := domain.OneOffTaskPriority(s.GetStr(constants.NSOneOff, constants.KeyPriority))
	reward := sessionInt(s, constants.NSOneOff, constants.KeyReward)
	task, err := svc.CreateOneOffTaskWithReward(ctx, user.ID, s.GetStr(constants.NSOneOff, constants.KeyTaskTitle), priority, reward, checklist)
	if err != nil {
		ui.showScreen(ctx, s.ChatID, tr("oneoff_error_create", err.Error()), nil)
		return "", nil
	}
	_ = s.ClearNamespace(constants.NSOneOff)
	_ = s.Transition(ctx, constants.SceneMenu)
	ui.showOneOffTasks(ctx, s.ChatID, user.ID, tr("oneoff_success_create", task.Title))
	return "", nil
}

func transitionAndRenderScene(ctx context.Context, b *bot.Bot, eng *tgamlengine.Engine, s *tgamlsession.Session, sceneID string) (string, error) {
	if err := s.Transition(ctx, sceneID); err != nil {
		return "", err
	}
	eng.RenderScene(ctx, b, sceneID, s.ChatID, s)
	return "", nil
}

func sessionInt64(s *tgamlsession.Session, ns, key string) (int64, error) {
	return strconv.ParseInt(s.GetStr(ns, key), 10, 64)
}

func sessionInt(s *tgamlsession.Session, ns, key string) int {
	value, err := strconv.Atoi(s.GetStr(ns, key))
	if err != nil {
		return 0
	}
	return value
}
