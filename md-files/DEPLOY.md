# Deployment Guide

This document describes how to deploy `jesterbot` on a Linux host.

## Scope

The repository supports three practical runtime paths:

1. Manual run with `go run ./cmd/jesterbot`
2. Native binary + `systemd`
3. Docker container

The recommended production path is native binary + `systemd`.

## Production DB Policy

- The bot is already used in production.
- Do not modify the SQLite schema directly on a live instance.
- Do not rewrite existing migration files that may already be applied.
- Any schema change must be delivered only as a new forward migration.

## Prerequisites

- Linux host with outbound internet access
- Telegram bot token
- Go toolchain for building on the server, or a prebuilt binary copied to the server
- `git`, `systemd`, and `sqlite` filesystem access
- `sqlite3` CLI installed on the host (required for `jesterbot-backup.service`)

## Runtime Configuration

The env surface is intentionally small:

- `JESTERBOT_BOT_TOKEN` - required
- `JESTERBOT_DB_PATH` - optional, defaults to `data/jesterbot.db`
- `JESTERBOT_POLL_TIMEOUT` - optional, defaults to `10s`

These values are not configured through env anymore:

- worker count
- default reminder minutes
- scheduler check frequency

User-facing behavior is configured inside the bot after startup:

- morning time
- reminder interval
- one-off reminder intervals by priority
- per-user scheduler check frequency

## Option 1: Manual Run with `go run`

This is the simplest way to start the bot and matches the current local workflow.
It is useful for development, manual verification, and small non-production setups.

### 1. Create the env file

Create `.env` in the repo root:

```dotenv
JESTERBOT_BOT_TOKEN=replace-me
JESTERBOT_DB_PATH=data/jesterbot.db
JESTERBOT_POLL_TIMEOUT=10s
```

### 2. Start the bot

```bash
go run ./cmd/jesterbot
```

### 3. Verify

You should see startup logs in the terminal. Stop the bot with `Ctrl+C`.

This mode does not provide automatic restart after crashes or reboot.

## Option 2: Native Binary + systemd

### 1. Create a runtime user

```bash
sudo useradd --system --create-home --home-dir /opt/jesterbot --shell /usr/sbin/nologin jesterbot
```

### 2. Copy the source code

```bash
sudo mkdir -p /opt/jesterbot
sudo chown -R jesterbot:jesterbot /opt/jesterbot
sudo -u jesterbot git clone <your-repo-url> /opt/jesterbot
```

If the code already exists, update it with:

```bash
cd /opt/jesterbot
git fetch --all --prune
git checkout <branch-or-tag>
git pull --ff-only
```

### 3. Build the binary

```bash
cd /opt/jesterbot
go build -o jesterbot ./cmd/jesterbot
```

### 4. Create the env file

Create `/opt/jesterbot/.env`:

```dotenv
JESTERBOT_BOT_TOKEN=replace-me
JESTERBOT_DB_PATH=/opt/jesterbot/data/jesterbot.db
JESTERBOT_POLL_TIMEOUT=10s
```

Lock down permissions:

```bash
chmod 600 /opt/jesterbot/.env
chown jesterbot:jesterbot /opt/jesterbot/.env
mkdir -p /opt/jesterbot/data
chown -R jesterbot:jesterbot /opt/jesterbot/data
```

### 5. Install the systemd unit

Copy and adjust the unit:

```bash
sudo cp deploy/systemd/jesterbot.service /etc/systemd/system/jesterbot.service
```

Update these values if needed:

- `WorkingDirectory`
- env values in the `Environment=` lines
- `ExecStart`

### 6. Enable and start the service

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now jesterbot
```

### 7. Verify the deployment

```bash
sudo systemctl status jesterbot
sudo journalctl -u jesterbot -n 200 --no-pager
```

### 8. Enable daily SQLite backups (3-day retention)

Backup units require installed `sqlite3` CLI on the host.

Install backup artifacts:

```bash
sudo cp deploy/systemd/jesterbot-backup.service /etc/systemd/system/jesterbot-backup.service
sudo cp deploy/systemd/jesterbot-backup.timer /etc/systemd/system/jesterbot-backup.timer
sudo chmod +x /opt/jesterbot/deploy/systemd/jesterbot-backup.sh
```

Enable the timer:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now jesterbot-backup.timer
```

Verify backup scheduling and first run:

```bash
sudo systemctl list-timers jesterbot-backup.timer --all
sudo systemctl start jesterbot-backup.service
sudo systemctl status jesterbot-backup.service --no-pager
```

Defaults (can be overridden via environment):

- DB path: `JESTERBOT_DB_PATH` (default `/opt/jesterbot/data/jesterbot.db`)
- Backup directory: `JESTERBOT_BACKUP_DIR` (default `/opt/jesterbot/backups`)
- Retention days: `JESTERBOT_BACKUP_RETENTION_DAYS` (default `3`)

## Option 3: Docker

### 1. Build the image

```bash
docker build -t jesterbot .
```

### 2. Prepare the env file

Create an env file such as `/opt/jesterbot/jesterbot.env`:

```dotenv
JESTERBOT_BOT_TOKEN=replace-me
JESTERBOT_DB_PATH=/app/data/jesterbot.db
JESTERBOT_POLL_TIMEOUT=10s
```

### 3. Start the container

```bash
mkdir -p /opt/jesterbot/data
docker run -d \
  --name jesterbot \
  --restart unless-stopped \
  --env-file /opt/jesterbot/jesterbot.env \
  -v /opt/jesterbot/data:/app/data \
  jesterbot
```

### 4. Verify

```bash
docker ps
docker logs --tail 200 jesterbot
```

## First-Start Checklist

After the bot is online:

1. Open the bot in Telegram.
2. Run `/start`.
3. Complete registration.
4. Configure morning time and reminder interval.
5. Configure one-off reminder intervals and scheduler check frequency in settings.
6. Add recurring activities and, if needed, one-off tasks.

## Updating the Deployment

For native deployment:

```bash
cd /opt/jesterbot
git fetch --all --prune
git checkout <branch-or-tag>
git pull --ff-only
go build -o jesterbot ./cmd/jesterbot
sudo systemctl restart jesterbot
```

For Docker deployment:

```bash
cd /opt/jesterbot
git fetch --all --prune
git checkout <branch-or-tag>
git pull --ff-only
docker build -t jesterbot .
docker rm -f jesterbot
docker run -d \
  --name jesterbot \
  --restart unless-stopped \
  --env-file /opt/jesterbot/jesterbot.env \
  -v /opt/jesterbot/data:/app/data \
  jesterbot
```

## Backup and Recovery

SQLite data lives at the DB path configured through `JESTERBOT_DB_PATH`.

For native `systemd` deployment, use `jesterbot-backup.timer` +
`jesterbot-backup.service` + `deploy/systemd/jesterbot-backup.sh` to run daily backups and remove backup files older than 3 days.

At minimum, keep backups of:

- SQLite database snapshots from `/opt/jesterbot/backups`
- the `.env` or Docker env file
- your custom `systemd` unit files if you changed them locally

## Validation

Before deploying a new build, run:

```bash
go test ./...
```
