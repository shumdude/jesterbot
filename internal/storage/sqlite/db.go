package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create sqlite directory: %w", err)
	}

	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_txlock=immediate", filepath.ToSlash(path))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

func migrate(db *sql.DB) error {
	for i, query := range migrations {
		version := i + 1

		var applied int
		err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&applied)
		if err == nil && applied > 0 {
			continue
		}

		if _, execErr := db.Exec(query); execErr != nil {
			// ALTER TABLE ADD COLUMN fails with "duplicate column name" when the column
			// already exists (e.g. re-running on a DB that predates version tracking).
			// Treat that as already applied.
			if strings.Contains(execErr.Error(), "duplicate column name") {
				_, _ = db.Exec(`INSERT OR IGNORE INTO schema_migrations (version) VALUES (?)`, version)
				continue
			}
			return fmt.Errorf("apply migration v%d: %w", version, execErr)
		}

		_, _ = db.Exec(`INSERT OR IGNORE INTO schema_migrations (version) VALUES (?)`, version)
	}
	return nil
}
