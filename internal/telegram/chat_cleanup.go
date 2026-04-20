package telegram

import (
	"context"
	"time"

	"jesterbot/internal/domain"
)

const settingsChatCleanupDeleteDelay = 100 * time.Millisecond

type messageDeleteFunc func(context.Context, int64, int) bool
type sleepFunc func(time.Duration)

type chatCleanupResult struct {
	Attempted       int
	Deleted         int
	Failed          int
	DeletedMessages []domain.ReminderMessage
}

func messageIDsForChatCleanup(currentMessageID int) []int {
	if currentMessageID <= 1 {
		return nil
	}

	ids := make([]int, 0, currentMessageID-1)
	for messageID := currentMessageID - 1; messageID >= 1; messageID-- {
		ids = append(ids, messageID)
	}
	return ids
}

func clearChatMessagesWithDelay(
	ctx context.Context,
	chatID int64,
	currentMessageID int,
	deleteFn messageDeleteFunc,
	sleep sleepFunc,
	delay time.Duration,
) chatCleanupResult {
	messageIDs := messageIDsForChatCleanup(currentMessageID)
	messages := make([]domain.ReminderMessage, 0, len(messageIDs))
	for _, messageID := range messageIDs {
		messages = append(messages, domain.ReminderMessage{
			ChatID:    chatID,
			MessageID: messageID,
		})
	}
	return clearMessageIDsWithDelay(ctx, messages, deleteFn, sleep, delay)
}

func clearMessageIDsWithDelay(
	ctx context.Context,
	messages []domain.ReminderMessage,
	deleteFn messageDeleteFunc,
	sleep sleepFunc,
	delay time.Duration,
) chatCleanupResult {
	result := chatCleanupResult{
		DeletedMessages: make([]domain.ReminderMessage, 0),
	}
	for i, message := range messages {
		if ctx.Err() != nil {
			break
		}

		result.Attempted++
		if deleteFn(ctx, message.ChatID, message.MessageID) {
			result.Deleted++
			result.DeletedMessages = append(result.DeletedMessages, message)
		} else {
			result.Failed++
		}

		if i < len(messages)-1 && delay > 0 && sleep != nil {
			sleep(delay)
		}
	}

	return result
}
