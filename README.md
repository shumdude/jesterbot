# Jesterbot

Telegram bot for daily activity planning, reminders, and progress tracking.

## What Is Implemented

- registration flow with FSM-like in-memory session states
- activity CRUD
- morning daily plan creation with opt-out selection
- progress tracking and done buttons
- random reminder cycle without repeats until the cycle resets
- settings for morning time and reminder interval
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
- `JESTERBOT_TICK_INTERVAL` - scheduler tick, default `1m`
- `JESTERBOT_POLL_TIMEOUT` - long polling timeout, default `10s`
- `JESTERBOT_WORKERS` - Telegram update workers, default `4`
- `JESTERBOT_DEFAULT_REMINDER_MINUTES` - default reminder interval, default `30`

## Validation

Primary verification command:

```powershell
$env:GOCACHE='C:\Users\thefi\.codex\memories\jesterbot-gocache'; go test ./...
```

## Deployment

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
