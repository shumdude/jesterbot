package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"jesterbot/internal/domain"
)

func (r *Repository) GetOneOffReminderSettings(ctx context.Context, userID int64) (*domain.OneOffReminderSettings, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT user_id, low_priority_minutes, medium_priority_minutes, high_priority_minutes, created_at, updated_at
		FROM one_off_task_reminder_settings
		WHERE user_id = ?`,
		userID,
	)
	return scanOneOffReminderSettings(row)
}

func (r *Repository) SaveOneOffReminderSettings(ctx context.Context, settings *domain.OneOffReminderSettings) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO one_off_task_reminder_settings (
			user_id, low_priority_minutes, medium_priority_minutes, high_priority_minutes, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			low_priority_minutes = excluded.low_priority_minutes,
			medium_priority_minutes = excluded.medium_priority_minutes,
			high_priority_minutes = excluded.high_priority_minutes,
			updated_at = excluded.updated_at`,
		settings.UserID,
		settings.LowPriorityMinutes,
		settings.MediumPriorityMinutes,
		settings.HighPriorityMinutes,
		formatTime(settings.CreatedAt),
		formatTime(settings.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("save one-off reminder settings: %w", err)
	}

	return nil
}

func (r *Repository) GetOneOffTask(ctx context.Context, userID, taskID int64) (*domain.OneOffTask, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, title, priority, status, next_reminder_at, completed_at, created_at, updated_at
		FROM one_off_tasks
		WHERE user_id = ? AND id = ?`,
		userID,
		taskID,
	)

	task, err := scanOneOffTask(row)
	if err != nil {
		return nil, err
	}

	items, err := r.loadOneOffTaskItems(ctx, task.ID)
	if err != nil {
		return nil, err
	}
	task.Items = items

	return task, nil
}

func (r *Repository) ListOneOffTasks(ctx context.Context, userID int64) ([]domain.OneOffTask, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, title, priority, status, next_reminder_at, completed_at, created_at, updated_at
		FROM one_off_tasks
		WHERE user_id = ?
		ORDER BY
			CASE status WHEN 'active' THEN 0 ELSE 1 END,
			CASE priority WHEN 'high' THEN 0 WHEN 'medium' THEN 1 ELSE 2 END,
			id DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list one-off tasks: %w", err)
	}
	defer rows.Close()

	tasks := make([]domain.OneOffTask, 0)
	for rows.Next() {
		task, err := scanOneOffTaskRows(rows)
		if err != nil {
			return nil, err
		}
		items, err := r.loadOneOffTaskItems(ctx, task.ID)
		if err != nil {
			return nil, err
		}
		task.Items = items
		tasks = append(tasks, *task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list one-off tasks rows: %w", err)
	}

	return tasks, nil
}

func (r *Repository) SaveOneOffTask(ctx context.Context, task *domain.OneOffTask) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin save one-off task: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if task.ID == 0 {
		result, execErr := tx.ExecContext(ctx, `
			INSERT INTO one_off_tasks (
				user_id, title, priority, status, next_reminder_at, completed_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			task.UserID,
			task.Title,
			string(task.Priority),
			string(task.Status),
			formatNullableTime(task.NextReminderAt),
			formatNullableTime(task.CompletedAt),
			formatTime(task.CreatedAt),
			formatTime(task.UpdatedAt),
		)
		if execErr != nil {
			err = fmt.Errorf("insert one-off task: %w", execErr)
			return err
		}

		task.ID, err = result.LastInsertId()
		if err != nil {
			err = fmt.Errorf("one-off task last insert id: %w", err)
			return err
		}
	} else {
		_, err = tx.ExecContext(ctx, `
			UPDATE one_off_tasks
			SET title = ?, priority = ?, status = ?, next_reminder_at = ?, completed_at = ?, updated_at = ?
			WHERE id = ? AND user_id = ?`,
			task.Title,
			string(task.Priority),
			string(task.Status),
			formatNullableTime(task.NextReminderAt),
			formatNullableTime(task.CompletedAt),
			formatTime(task.UpdatedAt),
			task.ID,
			task.UserID,
		)
		if err != nil {
			err = fmt.Errorf("update one-off task: %w", err)
			return err
		}
	}

	for i := range task.Items {
		item := &task.Items[i]
		_, err = tx.ExecContext(ctx, `
			INSERT INTO one_off_task_items (
				task_id, title, sort_order, completed, completed_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(task_id, sort_order) DO UPDATE SET
				title = excluded.title,
				completed = excluded.completed,
				completed_at = excluded.completed_at,
				updated_at = excluded.updated_at`,
			task.ID,
			item.Title,
			item.SortOrder,
			boolToInt(item.Completed),
			formatNullableTime(item.CompletedAt),
			formatTime(item.CreatedAt),
			formatTime(item.UpdatedAt),
		)
		if err != nil {
			err = fmt.Errorf("upsert one-off task item: %w", err)
			return err
		}

		if item.ID == 0 {
			item.ID, err = queryOneOffTaskItemID(ctx, tx, task.ID, item.SortOrder)
			if err != nil {
				return err
			}
		}
		item.TaskID = task.ID
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit one-off task: %w", err)
	}

	return nil
}

func (r *Repository) DeleteOneOffTask(ctx context.Context, userID, taskID int64) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM one_off_tasks
		WHERE id = ? AND user_id = ?`,
		taskID,
		userID,
	)
	if err != nil {
		return fmt.Errorf("delete one-off task: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("one-off task rows affected: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}

	return nil
}

func (r *Repository) loadOneOffTaskItems(ctx context.Context, taskID int64) ([]domain.OneOffTaskItem, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_id, title, sort_order, completed, completed_at, created_at, updated_at
		FROM one_off_task_items
		WHERE task_id = ?
		ORDER BY sort_order, id`,
		taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("load one-off task items: %w", err)
	}
	defer rows.Close()

	items := make([]domain.OneOffTaskItem, 0)
	for rows.Next() {
		item, err := scanOneOffTaskItemRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("one-off task item rows: %w", err)
	}

	return items, nil
}

func scanOneOffReminderSettings(row *sql.Row) (*domain.OneOffReminderSettings, error) {
	var (
		settings             domain.OneOffReminderSettings
		createdAt, updatedAt string
	)
	if err := row.Scan(
		&settings.UserID,
		&settings.LowPriorityMinutes,
		&settings.MediumPriorityMinutes,
		&settings.HighPriorityMinutes,
		&createdAt,
		&updatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("scan one-off reminder settings: %w", err)
	}

	var err error
	settings.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	settings.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}

	return &settings, nil
}

func scanOneOffTask(row *sql.Row) (*domain.OneOffTask, error) {
	task, err := scanOneOffTaskScanner(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return task, nil
}

func scanOneOffTaskRows(rows *sql.Rows) (*domain.OneOffTask, error) {
	return scanOneOffTaskScanner(rows)
}

func scanOneOffTaskScanner(scanner interface{ Scan(dest ...any) error }) (*domain.OneOffTask, error) {
	var (
		task                 domain.OneOffTask
		priority, status     string
		nextReminderAt       sql.NullString
		completedAt          sql.NullString
		createdAt, updatedAt string
	)
	if err := scanner.Scan(
		&task.ID,
		&task.UserID,
		&task.Title,
		&priority,
		&status,
		&nextReminderAt,
		&completedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, fmt.Errorf("scan one-off task: %w", err)
	}

	var err error
	task.Priority = domain.OneOffTaskPriority(priority)
	task.Status = domain.OneOffTaskStatus(status)
	task.NextReminderAt, err = parseNullableTime(nextReminderAt)
	if err != nil {
		return nil, err
	}
	task.CompletedAt, err = parseNullableTime(completedAt)
	if err != nil {
		return nil, err
	}
	task.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	task.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}

	return &task, nil
}

func scanOneOffTaskItemRows(rows *sql.Rows) (*domain.OneOffTaskItem, error) {
	var (
		item                 domain.OneOffTaskItem
		completed            int
		completedAt          sql.NullString
		createdAt, updatedAt string
	)
	if err := rows.Scan(
		&item.ID,
		&item.TaskID,
		&item.Title,
		&item.SortOrder,
		&completed,
		&completedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, fmt.Errorf("scan one-off task item: %w", err)
	}

	item.Completed = completed == 1
	var err error
	item.CompletedAt, err = parseNullableTime(completedAt)
	if err != nil {
		return nil, err
	}
	item.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	item.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}

	return &item, nil
}

func queryOneOffTaskItemID(ctx context.Context, tx *sql.Tx, taskID int64, sortOrder int) (int64, error) {
	var id int64
	if err := tx.QueryRowContext(ctx, `
		SELECT id
		FROM one_off_task_items
		WHERE task_id = ? AND sort_order = ?`,
		taskID,
		sortOrder,
	).Scan(&id); err != nil {
		return 0, fmt.Errorf("query one-off task item id: %w", err)
	}
	return id, nil
}
