# Jesterbot

Telegram bot for daily activity planning, reminders, and progress tracking.

Planned work lives in [md-files/FUTURE.md](md-files/FUTURE.md).

## What Is Implemented

- registration flow with FSM-like in-memory session states
- activity CRUD
- batch activity creation from a single message with comma/newline separators
- morning daily plan creation with opt-out selection
- progress tracking and done buttons
- random reminder cycle without repeats until the cycle resets
- settings for morning time, reminder interval, per-user scheduler check frequency, and one-off reminder intervals
- one-off tasks with checklist items, priority-based reminders, and dedicated reminder settings
- SQLite persistence
- automated tests for core logic and SQLite repository

## Architecture

- `cmd/jesterbot`
  - process entrypoint
- `internal/app`
  - bootstrap and wiring
- `internal/config`
  - env-based configuration
- `internal/domain`
  - entities and domain errors
- `internal/service`
  - business logic and use cases
- `internal/storage/sqlite`
  - migrations and repository implementation
- `internal/telegram`
  - Telegram transport, session FSM, keyboards, scheduler
- `configs`
  - config examples
- `deploy/systemd`
  - service unit for auto-restart

## Run

1. Create a `.env` file in the project root or in `cmd/jesterbot`.
2. Set `JESTERBOT_BOT_TOKEN`.
3. Run:

```powershell
go run ./cmd/jesterbot
```

This is the single-command local run path.

The app auto-loads `.env` from the current directory and parent directories. Process environment variables still take precedence over file values.

## Configuration

Environment variables:

- `JESTERBOT_BOT_TOKEN` - required Telegram bot token
- `JESTERBOT_DB_PATH` - SQLite DB path, default `data/jesterbot.db`
- `JESTERBOT_POLL_TIMEOUT` - long polling timeout, default `10s`

Worker count and the initial reminder default are internal runtime defaults, not env knobs.

Scheduler checks due work on a fixed internal one-minute cadence, while user-facing reminder behavior and per-user scheduler check frequency are controlled via bot settings.

## Validation

Primary verification command:

```powershell
go test ./...
```

## Deployment

Detailed deployment instructions live in [md-files/DEPLOY.md](md-files/DEPLOY.md).

Production note: the bot is already used in production. Do not change DB schema directly; any schema change must go through a new forward migration.

### Docker

Build and run:

```powershell
docker build -t jesterbot .
docker run --restart unless-stopped --env-file .\configs\example.env -v ${PWD}\data:/app/data jesterbot
```

### systemd

Use `deploy/systemd/jesterbot.service`, adjust `WorkingDirectory`, `Environment`, and binary path, then:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now jesterbot
```

## Future

See [md-files/FUTURE.md](md-files/FUTURE.md) for the current implementation backlog.
