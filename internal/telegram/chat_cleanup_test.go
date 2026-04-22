package telegram

import (
	"context"
	"reflect"
	"testing"
	"time"

	"jesterbot/internal/domain"
)

func TestMessageIDsForChatCleanupSkipsCurrentMessage(t *testing.T) {
	got := messageIDsForChatCleanup(5)
	want := []int{4, 3, 2, 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected cleanup ids: got %v, want %v", got, want)
	}
}

func TestClearChatMessagesWithDelaySleepsBetweenDeleteAttempts(t *testing.T) {
	var (
		deletedIDs []int
		sleeps     []time.Duration
	)

	result := clearChatMessagesWithDelay(
		context.Background(),
		777,
		5,
		func(_ context.Context, _ int64, messageID int) bool {
			deletedIDs = append(deletedIDs, messageID)
			return messageID%2 == 0
		},
		func(delay time.Duration) {
			sleeps = append(sleeps, delay)
		},
		75*time.Millisecond,
	)

	if want := []int{4, 3, 2, 1}; !reflect.DeepEqual(deletedIDs, want) {
		t.Fatalf("unexpected deleted ids order: got %v, want %v", deletedIDs, want)
	}
	if len(sleeps) != 3 {
		t.Fatalf("expected 3 sleeps between 4 delete attempts, got %d", len(sleeps))
	}
	for _, sleep := range sleeps {
		if sleep != 75*time.Millisecond {
			t.Fatalf("unexpected sleep duration: %s", sleep)
		}
	}
	if result.Attempted != 4 || result.Deleted != 2 || result.Failed != 2 {
		t.Fatalf("unexpected cleanup result: %+v", result)
	}
}

func TestClearMessageIDsWithDelayReturnsDeletedMessages(t *testing.T) {
	messages := []domain.ReminderMessage{
		{UserID: 7, ChatID: 777, MessageID: 11},
		{UserID: 7, ChatID: 777, MessageID: 12},
		{UserID: 7, ChatID: 777, MessageID: 13},
	}
	var (
		deletedIDs []int
		sleeps     []time.Duration
	)

	result := clearMessageIDsWithDelay(
		context.Background(),
		messages,
		func(_ context.Context, _ int64, messageID int) bool {
			deletedIDs = append(deletedIDs, messageID)
			return messageID != 12
		},
		func(delay time.Duration) {
			sleeps = append(sleeps, delay)
		},
		50*time.Millisecond,
	)

	if want := []int{11, 12, 13}; !reflect.DeepEqual(deletedIDs, want) {
		t.Fatalf("unexpected delete order: got %v, want %v", deletedIDs, want)
	}
	if len(sleeps) != 2 {
		t.Fatalf("expected two sleeps between three attempts, got %d", len(sleeps))
	}
	if result.Attempted != 3 || result.Deleted != 2 || result.Failed != 1 {
		t.Fatalf("unexpected cleanup result: %+v", result)
	}
	if got := []int{result.DeletedMessages[0].MessageID, result.DeletedMessages[1].MessageID}; !reflect.DeepEqual(got, []int{11, 13}) {
		t.Fatalf("unexpected deleted message ids: %v", got)
	}
}
