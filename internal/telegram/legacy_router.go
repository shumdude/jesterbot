package telegram

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"

	tgamlengine "gobot/tgaml/pkg/engine"

	"jesterbot/internal/domain"
	"jesterbot/internal/service"
)

func NewLegacyRouter(logger *slog.Logger, svc *service.Service, eng *tgamlengine.Engine) *Controller {
	return &Controller{
		logger:  logger,
		service: svc,
		eng:     eng,
	}
}

func (r *Controller) AttachBot(b *bot.Bot) {
	r.bot = b
}

func (r *Controller) RegisterLegacyHandlers() {
	r.bot.RegisterHandler(bot.HandlerTypeCallbackQueryData, "activity:", bot.MatchTypePrefix, r.handleActivityCallback)
	r.bot.RegisterHandler(bot.HandlerTypeCallbackQueryData, "plan:", bot.MatchTypePrefix, r.handlePlanCallback)
	r.bot.RegisterHandler(bot.HandlerTypeCallbackQueryData, "done:", bot.MatchTypePrefix, r.handleDoneCallback)
	r.bot.RegisterHandler(bot.HandlerTypeCallbackQueryData, "oneoff:", bot.MatchTypePrefix, r.handleOneOffCallback)
	r.bot.RegisterHandler(bot.HandlerTypeCallbackQueryData, "settings:", bot.MatchTypePrefix, r.handleSettingsCallback)
	r.bot.RegisterHandler(bot.HandlerTypeCallbackQueryData, noopCallbackData, bot.MatchTypeExact, r.handleNoopCallback)
}

func (r *Controller) ShowWelcome(ctx context.Context, chatID int64, user *domain.User) {
	r.showScreen(ctx, chatID, welcomeText(user), r.menuMarkup(user.ID, chatID))
}

func (r *Controller) OpenToday(ctx context.Context, chatID, telegramUserID int64) {
	user, err := r.registeredUser(ctx, telegramUserID)
	if err != nil {
		r.handleRegistrationRequired(ctx, chatID, telegramUserID)
		return
	}

	now := time.Now().UTC()
	plan, err := r.service.GetTodayPlan(ctx, user.ID, now)
	if errors.Is(err, domain.ErrNotFound) {
		plan, err = r.service.StartMorningPlan(ctx, user.ID, now)
	}
	if err != nil {
		r.showScreen(ctx, chatID, todayPlanErrorText(err), r.menuMarkup(user.ID, chatID))
		return
	}

	if plan.Status == domain.PlanStatusAwaitingSelection {
		r.showScreen(ctx, chatID, selectionTextPage(plan, 0, defaultInlinePageSize), buildPlanSelectionKeyboardPage(plan, 0, defaultInlinePageSize))
		return
	}

	r.showScreen(ctx, chatID, progressTextPage(plan, 0, defaultInlinePageSize), buildProgressKeyboardPage(plan, 0, defaultInlinePageSize))
}

func (r *Controller) OpenActivities(ctx context.Context, chatID, telegramUserID int64) {
	user, err := r.registeredUser(ctx, telegramUserID)
	if err != nil {
		r.handleRegistrationRequired(ctx, chatID, telegramUserID)
		return
	}
	r.showActivitiesPage(ctx, chatID, user.ID, tr("activity_title"), 0)
}

func (r *Controller) OpenOneOffTasks(ctx context.Context, chatID, telegramUserID int64) {
	user, err := r.registeredUser(ctx, telegramUserID)
	if err != nil {
		r.handleRegistrationRequired(ctx, chatID, telegramUserID)
		return
	}
	r.showOneOffTasksPage(ctx, chatID, user.ID, tr("oneoff_title"), 0)
}

func (r *Controller) OpenSettings(ctx context.Context, chatID, telegramUserID int64) {
	if _, err := r.registeredUser(ctx, telegramUserID); err != nil {
		r.handleRegistrationRequired(ctx, chatID, telegramUserID)
		return
	}
	r.showSettings(ctx, chatID, telegramUserID, tr("settings_title"))
}

func (r *Controller) OpenStats(ctx context.Context, chatID, telegramUserID int64) {
	user, err := r.registeredUser(ctx, telegramUserID)
	if err != nil {
		r.handleRegistrationRequired(ctx, chatID, telegramUserID)
		return
	}
	stats, err := r.service.BuildStats(ctx, user.ID)
	if err != nil {
		r.showScreen(ctx, chatID, tr("stats_error_build"), r.menuMarkup(user.ID, chatID))
		return
	}
	r.showScreen(ctx, chatID, statsText(stats), r.menuMarkup(user.ID, chatID))
}
