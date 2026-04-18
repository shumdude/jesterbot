package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-telegram/bot"

	tgamlengine "gobot/tgaml/pkg/engine"
	tgamlsession "gobot/tgaml/pkg/session"

	"jesterbot/cmd/jesterbot/botconfig"
	"jesterbot/internal/domain"
	"jesterbot/internal/service"
	"jesterbot/internal/telegram/constants"
	"jesterbot/internal/telegram/session_backend"
)

type Transport struct {
	logger *slog.Logger
	bot    *bot.Bot
	engine *tgamlengine.Engine
	legacy *Controller
}

func NewTransport(logger *slog.Logger, token string, pollTimeout time.Duration, workers int, svc *service.Service) (*Transport, error) {
	cfg, err := botconfig.Load()
	if err != nil {
		return nil, err
	}

	backend := session_backend.New(constants.SceneMenu)
	eng := tgamlengine.New(cfg, backend)
	registerShowIf(eng, svc)

	legacy := NewLegacyRouter(logger, svc, eng)
	RegisterTgamlHandlers(eng, svc, legacy)

	httpTimeout := telegramHTTPClientTimeout(pollTimeout)
	opts := append([]bot.Option{}, eng.BotOptions()...)
	opts = append(opts,
		bot.WithWorkers(workers),
		bot.WithHTTPClient(pollTimeout, &http.Client{Timeout: httpTimeout}),
		bot.WithErrorsHandler(func(err error) {
			logger.Error("telegram bot poll error", "error", err)
		}),
	)

	b, err := bot.New(token, opts...)
	if err != nil {
		return nil, fmt.Errorf("create telegram bot: %w", err)
	}

	legacy.AttachBot(b)
	legacy.RegisterLegacyHandlers()

	return &Transport{
		logger: logger,
		bot:    b,
		engine: eng,
		legacy: legacy,
	}, nil
}

func (t *Transport) Start(ctx context.Context) error {
	t.logger.Info("telegram polling started via tgaml transport")
	t.bot.Start(ctx)
	t.logger.Info("telegram polling stopped")
	return nil
}

func (t *Transport) Notifier() Notifier {
	return t.legacy
}

func registerShowIf(eng *tgamlengine.Engine, svc *service.Service) {
	registered := func(sess *tgamlsession.Session) bool {
		if sess == nil {
			return false
		}
		_, err := svc.FindUserByTelegramID(context.Background(), sess.UserID)
		return err == nil
	}

	eng.RegisterShowIf(constants.ShowIfRegistered, registered)
	eng.RegisterShowIf(constants.ShowIfUnregistered, func(sess *tgamlsession.Session) bool {
		if sess == nil {
			return true
		}
		_, err := svc.FindUserByTelegramID(context.Background(), sess.UserID)
		return errors.Is(err, domain.ErrNotFound)
	})
}
