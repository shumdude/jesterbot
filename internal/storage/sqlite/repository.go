package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"jesterbot/internal/domain"
	"jesterbot/internal/service"
)

var _ service.Repository = (*Repository)(nil)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetUserByTelegramID(ctx context.Context, telegramUserID int64) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, telegram_user_id, chat_id, name, utc_offset_minutes, morning_time, reminder_interval_minutes, created_at, updated_at
		FROM users
		WHERE telegram_user_id = ?`,
		telegramUserID,
	)
	return scanUser(row)
}

func (r *Repository) GetUserByID(ctx context.Context, userID int64) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, telegram_user_id, chat_id, name, utc_offset_minutes, morning_time, reminder_interval_minutes, created_at, updated_at
		FROM users
		WHERE id = ?`,
		userID,
	)
	return scanUser(row)
}

func (r *Repository) ListUsers(ctx context.Context) ([]domain.User, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, telegram_user_id, chat_id, name, utc_offset_minutes, morning_time, reminder_interval_minutes, created_at, updated_at
		FROM users
		ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	users := make([]domain.User, 0)
	for rows.Next() {
		user, err := scanUserRows(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list users rows: %w", err)
	}

	return users, nil
}

func (r *Repository) CreateUser(ctx context.Context, user *domain.User) error {
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO users (
			telegram_user_id, chat_id, name, utc_offset_minutes, morning_time, reminder_interval_minutes, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		user.TelegramUserID,
		user.ChatID,
		user.Name,
		user.UTCOffsetMinutes,
		user.MorningTime,
		user.ReminderIntervalMinutes,
		formatTime(user.CreatedAt),
		formatTime(user.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	user.ID, err = result.LastInsertId()
	if err != nil {
		return fmt.Errorf("user last insert id: %w", err)
	}

	return nil
}

func (r *Repository) UpdateUserSettings(ctx context.Context, userID int64, morningTime string, reminderIntervalMinutes int) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE users
		SET morning_time = ?, reminder_interval_minutes = ?, updated_at = ?
		WHERE id = ?`,
		morningTime,
		reminderIntervalMinutes,
		formatTime(time.Now().UTC()),
		userID,
	)
	if err != nil {
		return fmt.Errorf("update user settings: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("user settings rows affected: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}

	return nil
}

func (r *Repository) CreateActivity(ctx context.Context, activity *domain.Activity) error {
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO activities (user_id, title, sort_order, times_per_day, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		activity.UserID,
		activity.Title,
		activity.SortOrder,
		activity.TimesPerDay,
		formatTime(activity.CreatedAt),
		formatTime(activity.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("create activity: %w", err)
	}

	activity.ID, err = result.LastInsertId()
	if err != nil {
		return fmt.Errorf("activity last insert id: %w", err)
	}

	return nil
}

func (r *Repository) UpdateActivityTimesPerDay(ctx context.Context, userID, activityID int64, timesPerDay int) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE activities
		SET times_per_day = ?, updated_at = ?
		WHERE id = ? AND user_id = ?`,
		timesPerDay,
		formatTime(time.Now().UTC()),
		activityID,
		userID,
	)
	if err != nil {
		return fmt.Errorf("update activity times per day: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("activity times per day rows affected: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}

	return nil
}

func (r *Repository) UpdateActivity(ctx context.Context, userID, activityID int64, title string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE activities
		SET title = ?, updated_at = ?
		WHERE id = ? AND user_id = ?`,
		title,
		formatTime(time.Now().UTC()),
		activityID,
		userID,
	)
	if err != nil {
		return fmt.Errorf("update activity: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("activity rows affected: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}

	return nil
}

func (r *Repository) DeleteActivity(ctx context.Context, userID, activityID int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM activities WHERE id = ? AND user_id = ?`, activityID, userID)
	if err != nil {
		return fmt.Errorf("delete activity: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete activity rows affected: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}

	return nil
}

func (r *Repository) ListActivities(ctx context.Context, userID int64) ([]domain.Activity, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, title, sort_order, times_per_day, created_at, updated_at
		FROM activities
		WHERE user_id = ?
		ORDER BY sort_order, id`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list activities: %w", err)
	}
	defer rows.Close()

	activities := make([]domain.Activity, 0)
	for rows.Next() {
		activity, err := scanActivityRows(rows)
		if err != nil {
			return nil, err
		}
		activities = append(activities, *activity)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("activity rows: %w", err)
	}

	return activities, nil
}

func (r *Repository) GetDayPlan(ctx context.Context, userID int64, dayLocal string) (*domain.DayPlan, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, day_local, status, cycle, next_reminder_at, morning_sent_at, selection_finalized_at, completed_at, created_at, updated_at
		FROM daily_plans
		WHERE user_id = ? AND day_local = ?`,
		userID,
		dayLocal,
	)

	plan, err := scanPlan(row)
	if err != nil {
		return nil, err
	}

	items, err := r.loadPlanItems(ctx, plan.ID)
	if err != nil {
		return nil, err
	}
	plan.Items = items
	return plan, nil
}

func (r *Repository) SaveDayPlan(ctx context.Context, plan *domain.DayPlan) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin save day plan: %w", err)
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Plan header and all items are persisted atomically so scheduler and handlers
	// never observe a mixed state (updated plan without updated items or vice versa).
	if plan.ID == 0 {
		result, execErr := tx.ExecContext(ctx, `
			INSERT INTO daily_plans (
				user_id, day_local, status, cycle, next_reminder_at, morning_sent_at, selection_finalized_at, completed_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			plan.UserID,
			plan.DayLocal,
			string(plan.Status),
			plan.Cycle,
			formatNullableTime(plan.NextReminderAt),
			formatNullableTime(plan.MorningSentAt),
			formatNullableTime(plan.SelectionFinalizedAt),
			formatNullableTime(plan.CompletedAt),
			formatTime(plan.CreatedAt),
			formatTime(plan.UpdatedAt),
		)
		if execErr != nil {
			err = fmt.Errorf("insert day plan: %w", execErr)
			return err
		}

		plan.ID, err = result.LastInsertId()
		if err != nil {
			err = fmt.Errorf("day plan last insert id: %w", err)
			return err
		}
	} else {
		_, err = tx.ExecContext(ctx, `
			UPDATE daily_plans
			SET status = ?, cycle = ?, next_reminder_at = ?, morning_sent_at = ?, selection_finalized_at = ?, completed_at = ?, updated_at = ?
			WHERE id = ?`,
			string(plan.Status),
			plan.Cycle,
			formatNullableTime(plan.NextReminderAt),
			formatNullableTime(plan.MorningSentAt),
			formatNullableTime(plan.SelectionFinalizedAt),
			formatNullableTime(plan.CompletedAt),
			formatTime(plan.UpdatedAt),
			plan.ID,
		)
		if err != nil {
			err = fmt.Errorf("update day plan: %w", err)
			return err
		}
	}

	for i := range plan.Items {
		item := &plan.Items[i]
		// Rewrites existing (plan_id, activity_id) rows and inserts new ones.
		// This keeps SaveDayPlan idempotent for repeated calls with same snapshot.
		_, err = tx.ExecContext(ctx, `
			INSERT INTO daily_plan_items (
				plan_id, activity_id, title_snapshot, selected, completed, reminder_cycle, times_per_day, completed_count, completed_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(plan_id, activity_id) DO UPDATE SET
				title_snapshot = excluded.title_snapshot,
				selected = excluded.selected,
				completed = excluded.completed,
				reminder_cycle = excluded.reminder_cycle,
				times_per_day = excluded.times_per_day,
				completed_count = excluded.completed_count,
				completed_at = excluded.completed_at,
				updated_at = excluded.updated_at`,
			plan.ID,
			item.ActivityID,
			item.TitleSnapshot,
			boolToInt(item.Selected),
			boolToInt(item.Completed),
			item.ReminderCycle,
			item.TimesPerDay,
			item.CompletedCount,
			formatNullableTime(item.CompletedAt),
			formatTime(item.CreatedAt),
			formatTime(item.UpdatedAt),
		)
		if err != nil {
			err = fmt.Errorf("upsert day plan item: %w", err)
			return err
		}

		if item.ID == 0 {
			item.ID, err = queryPlanItemID(ctx, tx, plan.ID, item.ActivityID)
			if err != nil {
				return err
			}
		}
		item.PlanID = plan.ID
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit day plan: %w", err)
	}

	return nil
}

func (r *Repository) ListPlans(ctx context.Context, userID int64) ([]domain.DayPlan, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, day_local, status, cycle, next_reminder_at, morning_sent_at, selection_finalized_at, completed_at, created_at, updated_at
		FROM daily_plans
		WHERE user_id = ?
		ORDER BY day_local`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	defer rows.Close()

	plans := make([]domain.DayPlan, 0)
	for rows.Next() {
		plan, err := scanPlanRows(rows)
		if err != nil {
			return nil, err
		}
		items, err := r.loadPlanItems(ctx, plan.ID)
		if err != nil {
			return nil, err
		}
		plan.Items = items
		plans = append(plans, *plan)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list plans rows: %w", err)
	}

	return plans, nil
}

func (r *Repository) loadPlanItems(ctx context.Context, planID int64) ([]domain.DayPlanItem, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, plan_id, activity_id, title_snapshot, selected, completed, reminder_cycle, times_per_day, completed_count, completed_at, created_at, updated_at
		FROM daily_plan_items
		WHERE plan_id = ?
		ORDER BY id`,
		planID,
	)
	if err != nil {
		return nil, fmt.Errorf("load plan items: %w", err)
	}
	defer rows.Close()

	items := make([]domain.DayPlanItem, 0)
	for rows.Next() {
		item, err := scanPlanItemRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("plan item rows: %w", err)
	}

	return items, nil
}

func scanUser(row *sql.Row) (*domain.User, error) {
	user, err := scanUserScanner(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return user, nil
}

func scanUserRows(rows *sql.Rows) (*domain.User, error) {
	return scanUserScanner(rows)
}

func scanUserScanner(scanner interface{ Scan(dest ...any) error }) (*domain.User, error) {
	var (
		user                 domain.User
		createdAt, updatedAt string
	)
	if err := scanner.Scan(
		&user.ID,
		&user.TelegramUserID,
		&user.ChatID,
		&user.Name,
		&user.UTCOffsetMinutes,
		&user.MorningTime,
		&user.ReminderIntervalMinutes,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}

	var err error
	user.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	user.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

func scanActivityRows(rows *sql.Rows) (*domain.Activity, error) {
	var (
		activity             domain.Activity
		createdAt, updatedAt string
	)
	if err := rows.Scan(&activity.ID, &activity.UserID, &activity.Title, &activity.SortOrder, &activity.TimesPerDay, &createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("scan activity: %w", err)
	}

	var err error
	activity.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	activity.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}

	return &activity, nil
}

func scanPlan(row *sql.Row) (*domain.DayPlan, error) {
	plan, err := scanPlanScanner(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return plan, nil
}

func scanPlanRows(rows *sql.Rows) (*domain.DayPlan, error) {
	return scanPlanScanner(rows)
}

func scanPlanScanner(scanner interface{ Scan(dest ...any) error }) (*domain.DayPlan, error) {
	var (
		plan                              domain.DayPlan
		status                            string
		nextReminderAt, morningSentAt     sql.NullString
		selectionFinalizedAt, completedAt sql.NullString
		createdAt, updatedAt              string
	)
	if err := scanner.Scan(
		&plan.ID,
		&plan.UserID,
		&plan.DayLocal,
		&status,
		&plan.Cycle,
		&nextReminderAt,
		&morningSentAt,
		&selectionFinalizedAt,
		&completedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, fmt.Errorf("scan plan: %w", err)
	}

	plan.Status = domain.PlanStatus(status)
	var err error
	plan.NextReminderAt, err = parseNullableTime(nextReminderAt)
	if err != nil {
		return nil, err
	}
	plan.MorningSentAt, err = parseNullableTime(morningSentAt)
	if err != nil {
		return nil, err
	}
	plan.SelectionFinalizedAt, err = parseNullableTime(selectionFinalizedAt)
	if err != nil {
		return nil, err
	}
	plan.CompletedAt, err = parseNullableTime(completedAt)
	if err != nil {
		return nil, err
	}
	plan.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	plan.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}

	return &plan, nil
}

func scanPlanItemRows(rows *sql.Rows) (*domain.DayPlanItem, error) {
	var (
		item                 domain.DayPlanItem
		selected, completed  int
		completedAt          sql.NullString
		createdAt, updatedAt string
	)
	if err := rows.Scan(
		&item.ID,
		&item.PlanID,
		&item.ActivityID,
		&item.TitleSnapshot,
		&selected,
		&completed,
		&item.ReminderCycle,
		&item.TimesPerDay,
		&item.CompletedCount,
		&completedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, fmt.Errorf("scan plan item: %w", err)
	}

	item.Selected = selected == 1
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

func queryPlanItemID(ctx context.Context, tx *sql.Tx, planID, activityID int64) (int64, error) {
	var id int64
	if err := tx.QueryRowContext(ctx, `
		SELECT id
		FROM daily_plan_items
		WHERE plan_id = ? AND activity_id = ?`,
		planID,
		activityID,
	).Scan(&id); err != nil {
		return 0, fmt.Errorf("query day plan item id: %w", err)
	}
	return id, nil
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func formatNullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time %q: %w", value, err)
	}
	return parsed.UTC(), nil
}

func parseNullableTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid || value.String == "" {
		return nil, nil
	}
	parsed, err := parseTime(value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
