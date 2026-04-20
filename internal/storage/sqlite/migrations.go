package sqlite

var migrations = []string{
	`PRAGMA journal_mode = WAL;`,
	`PRAGMA foreign_keys = ON;`,
	`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY
	);`,
	`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		telegram_user_id INTEGER NOT NULL UNIQUE,
		chat_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		utc_offset_minutes INTEGER NOT NULL,
		morning_time TEXT NOT NULL,
		reminder_interval_minutes INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS user_scheduler_settings (
		user_id INTEGER PRIMARY KEY,
		tick_interval_minutes INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS activities (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		title TEXT NOT NULL,
		sort_order INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS daily_plans (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		day_local TEXT NOT NULL,
		status TEXT NOT NULL,
		cycle INTEGER NOT NULL DEFAULT 1,
		next_reminder_at TEXT,
		morning_sent_at TEXT,
		selection_finalized_at TEXT,
		completed_at TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		UNIQUE(user_id, day_local),
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS daily_plan_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		plan_id INTEGER NOT NULL,
		activity_id INTEGER NOT NULL,
		title_snapshot TEXT NOT NULL,
		selected INTEGER NOT NULL,
		completed INTEGER NOT NULL,
		reminder_cycle INTEGER NOT NULL DEFAULT 0,
		completed_at TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		UNIQUE(plan_id, activity_id),
		FOREIGN KEY(plan_id) REFERENCES daily_plans(id) ON DELETE CASCADE,
		FOREIGN KEY(activity_id) REFERENCES activities(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS one_off_task_reminder_settings (
		user_id INTEGER PRIMARY KEY,
		low_priority_minutes INTEGER NOT NULL,
		medium_priority_minutes INTEGER NOT NULL,
		high_priority_minutes INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS one_off_tasks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		title TEXT NOT NULL,
		priority TEXT NOT NULL,
		status TEXT NOT NULL,
		next_reminder_at TEXT,
		completed_at TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
	);`,
	`CREATE INDEX IF NOT EXISTS idx_one_off_tasks_user_status_next_reminder
		ON one_off_tasks(user_id, status, next_reminder_at);`,
	`ALTER TABLE activities ADD COLUMN times_per_day INTEGER NOT NULL DEFAULT 1;`,
	`ALTER TABLE daily_plan_items ADD COLUMN times_per_day INTEGER NOT NULL DEFAULT 1;`,
	`ALTER TABLE daily_plan_items ADD COLUMN completed_count INTEGER NOT NULL DEFAULT 0;`,
	`ALTER TABLE activities ADD COLUMN reminder_window_start TEXT NOT NULL DEFAULT '';`,
	`ALTER TABLE activities ADD COLUMN reminder_window_end TEXT NOT NULL DEFAULT '';`,
	`CREATE TABLE IF NOT EXISTS one_off_task_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id INTEGER NOT NULL,
		title TEXT NOT NULL,
		sort_order INTEGER NOT NULL,
		completed INTEGER NOT NULL,
		completed_at TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		UNIQUE(task_id, sort_order),
		FOREIGN KEY(task_id) REFERENCES one_off_tasks(id) ON DELETE CASCADE
	);`,
	`ALTER TABLE users ADD COLUMN day_end_time TEXT NOT NULL DEFAULT '00:00';`,
	`ALTER TABLE users ADD COLUMN notifications_paused_until TEXT;`,
	`CREATE TABLE IF NOT EXISTS activity_reminder_windows (
		activity_id INTEGER NOT NULL,
		sort_order INTEGER NOT NULL,
		window_start TEXT NOT NULL,
		window_end TEXT NOT NULL,
		PRIMARY KEY(activity_id, sort_order),
		FOREIGN KEY(activity_id) REFERENCES activities(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS reminder_messages (
		user_id INTEGER NOT NULL,
		chat_id INTEGER NOT NULL,
		message_id INTEGER NOT NULL,
		logical_day TEXT NOT NULL,
		kind TEXT NOT NULL,
		sent_at TEXT NOT NULL,
		PRIMARY KEY(user_id, message_id),
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
	);`,
	`CREATE INDEX IF NOT EXISTS idx_reminder_messages_user_day
		ON reminder_messages(user_id, logical_day);`,
}
