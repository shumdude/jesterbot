# internal/storage/sqlite

SQLite-backed repository implementation.

Contains DB initialization, migrations, and query logic.

Production rule: the bot is already in use in production; DB structure must be changed only by adding new migrations (no direct schema edits).
