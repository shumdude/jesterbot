# deploy/systemd

Systemd units and deployment artifacts for Linux production installs.

## Files

- `jesterbot.service` - main bot service.
- `jesterbot-backup.service` - one-shot SQLite backup job.
- `jesterbot-backup.timer` - runs backup job every day.
- `jesterbot-backup.sh` - backup script with 3-day retention cleanup.

## Enable Daily Backups

Backup units require installed `sqlite3` CLI on the host.

```bash
sudo cp deploy/systemd/jesterbot-backup.service /etc/systemd/system/jesterbot-backup.service
sudo cp deploy/systemd/jesterbot-backup.timer /etc/systemd/system/jesterbot-backup.timer
sudo chmod +x /opt/jesterbot/deploy/systemd/jesterbot-backup.sh
sudo systemctl daemon-reload
sudo systemctl enable --now jesterbot-backup.timer
```

Manual run and checks:

```bash
sudo systemctl start jesterbot-backup.service
sudo systemctl status jesterbot-backup.service --no-pager
sudo systemctl list-timers jesterbot-backup.timer --all
```

Defaults used by backup script:

- DB path: `/opt/jesterbot/data/jesterbot.db`
- Backup directory: `/opt/jesterbot/backups`
- Retention: delete backups older than 3 days
