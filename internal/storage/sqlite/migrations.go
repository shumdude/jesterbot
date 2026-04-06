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
}
