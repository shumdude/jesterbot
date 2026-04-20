package sqlite

import (
	"context"
	"fmt"

	"jesterbot/internal/domain"
)

func (r *Repository) SaveReminderMessage(ctx context.Context, message *domain.ReminderMessage) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO reminder_messages (
			user_id, chat_id, message_id, logical_day, kind, sent_at
		) VALUES (?, ?, ?, ?, ?, ?)`,
		message.UserID,
		message.ChatID,
		message.MessageID,
		message.LogicalDay,
		string(message.Kind),
		formatTime(message.SentAt),
	)
	if err != nil {
		return fmt.Errorf("save reminder message: %w", err)
	}
	return nil
}

func (r *Repository) ListReminderMessagesBeforeDay(ctx context.Context, userID int64, dayLocal string) ([]domain.ReminderMessage, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT user_id, chat_id, message_id, logical_day, kind, sent_at
		FROM reminder_messages
		WHERE user_id = ? AND logical_day < ?
		ORDER BY logical_day, sent_at, message_id`,
		userID,
		dayLocal,
	)
	if err != nil {
		return nil, fmt.Errorf("list reminder messages before day: %w", err)
	}
	defer rows.Close()

	messages := make([]domain.ReminderMessage, 0)
	for rows.Next() {
		var (
			message domain.ReminderMessage
			kind    string
			sentAt  string
		)
		if err := rows.Scan(
			&message.UserID,
			&message.ChatID,
			&message.MessageID,
			&message.LogicalDay,
			&kind,
			&sentAt,
		); err != nil {
			return nil, fmt.Errorf("scan reminder message: %w", err)
		}
		parsedSentAt, err := parseTime(sentAt)
		if err != nil {
			return nil, err
		}
		message.Kind = domain.ReminderMessageKind(kind)
		message.SentAt = parsedSentAt
		messages = append(messages, message)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reminder message rows: %w", err)
	}

	return messages, nil
}

func (r *Repository) DeleteReminderMessage(ctx context.Context, userID int64, messageID int) error {
	if _, err := r.db.ExecContext(ctx, `
		DELETE FROM reminder_messages
		WHERE user_id = ? AND message_id = ?`,
		userID,
		messageID,
	); err != nil {
		return fmt.Errorf("delete reminder message: %w", err)
	}
	return nil
}
