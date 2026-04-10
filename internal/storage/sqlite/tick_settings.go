package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"jesterbot/internal/domain"
)

func (r *Repository) GetUserTickInterval(ctx context.Context, userID int64) (int, error) {
	var minutes int
	if err := r.db.QueryRowContext(ctx, `
		SELECT tick_interval_minutes
		FROM user_scheduler_settings
		WHERE user_id = ?`,
		userID,
	).Scan(&minutes); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, domain.ErrNotFound
		}
		return 0, fmt.Errorf("get user tick interval: %w", err)
	}

	return minutes, nil
}

func (r *Repository) SaveUserTickInterval(ctx context.Context, userID int64, minutes int) error {
	now := formatTime(time.Now().UTC())
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO user_scheduler_settings (user_id, tick_interval_minutes, created_at, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			tick_interval_minutes = excluded.tick_interval_minutes,
			updated_at = excluded.updated_at`,
		userID,
		minutes,
		now,
		now,
	)
	if err != nil {
		return fmt.Errorf("save user tick interval: %w", err)
	}

	return nil
}
