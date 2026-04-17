package telegram

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	tgamlengine "gobot/tgaml/pkg/engine"
	tgamlsession "gobot/tgaml/pkg/session"

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
	eng.Register(constants.HandlerOpenOneOff, openLegacyScreenHandler(eng, svc, ui.OpenOneOffTasks))
	eng.Register(constants.HandlerOpenSettings, openLegacyScreenHandler(eng, svc, ui.OpenSettings))
	eng.Register(constants.HandlerOpenStats, openLegacyScreenHandler(eng, svc, ui.OpenStats))
	eng.Register(constants.HandlerAddActivity, addActivityHandler(svc, ui))
	eng.Register(constants.HandlerEditActivity, editActivityHandler(svc, ui))
	eng.Register(constants.HandlerSetActivityTimes, activityTimesHandler(svc, ui))
	eng.Register(constants.HandlerSetActivityWindow, activityWindowHandler(svc, ui))
	eng.Register(constants.HandlerUpdateMorning, updateMorningHandler(svc, ui))
	eng.Register(constants.HandlerUpdateReminder, updateReminderHandler(svc, ui))
	eng.Register(constants.HandlerUpdateTick, updateTickHandler(svc, ui))
	eng.Register(constants.HandlerUpdateOneOffReminder, updateOneOffReminderHandler(svc, ui))
	eng.Register(constants.HandlerOneOffTitle, oneOffTitleHandler(ui))
	eng.Register(constants.HandlerOneOffItems, oneOffItemsHandler(svc, ui))
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
	return func(ctx context.Context, b *bot.Bot, _ *models.Update, s *tgamlsession.Session) (string, error) {
		_, err := svc.FindUserByTelegramID(ctx, s.UserID)
		switch {
		case err == nil:
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
		var start, end string
		if strings.TrimSpace(u.Message.Text) != "-" {
			start, end, err = parseWindowInput(u.Message.Text)
			if err != nil {
				ui.showScreen(ctx, s.ChatID, tr("activity_error_invalid_window"), nil)
				return "", nil
			}
		}
		if err := svc.SetActivityReminderWindow(ctx, user.ID, activityID, start, end); err != nil {
			ui.showScreen(ctx, s.ChatID, tr("activity_error_window", err.Error()), nil)
			return "", nil
		}
		_ = s.ClearNamespace(constants.NSActivity)
		_ = s.Transition(ctx, constants.SceneMenu)
		ui.showActivityDetail(ctx, s.ChatID, user.ID, activityID, page, tr("activity_success_window"))
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
		if err := svc.UpdateSettings(ctx, user.ID, strings.TrimSpace(u.Message.Text), user.ReminderIntervalMinutes); err != nil {
			ui.showScreen(ctx, s.ChatID, tr("settings_error_update_morning", err.Error()), nil)
			return "", nil
		}
		_ = s.Transition(ctx, constants.SceneMenu)
		ui.showSettings(ctx, s.ChatID, s.UserID, tr("settings_success_morning"))
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
		if err := svc.UpdateSettings(ctx, user.ID, user.MorningTime, minutes); err != nil {
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

func oneOffItemsHandler(svc *service.Service, ui *Controller) tgamlengine.HandlerFunc {
	return func(ctx context.Context, _ *bot.Bot, u *models.Update, s *tgamlsession.Session) (string, error) {
		user, err := svc.FindUserByTelegramID(ctx, s.UserID)
		if err != nil {
			ui.handleRegistrationRequired(ctx, s.ChatID, s.UserID)
			return "", nil
		}
		priority := domain.OneOffTaskPriority(s.GetStr(constants.NSOneOff, constants.KeyPriority))
		task, err := svc.CreateOneOffTask(ctx, user.ID, s.GetStr(constants.NSOneOff, constants.KeyTaskTitle), priority, parseOneOffChecklistInput(u.Message.Text))
		if err != nil {
			ui.showScreen(ctx, s.ChatID, tr("oneoff_error_create", err.Error()), nil)
			return "", nil
		}
		_ = s.ClearNamespace(constants.NSOneOff)
		_ = s.Transition(ctx, constants.SceneMenu)
		ui.showOneOffTasks(ctx, s.ChatID, user.ID, tr("oneoff_success_create", task.Title))
		return "", nil
	}
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
