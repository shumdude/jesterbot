package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"jesterbot/internal/domain"
)

const (
	defaultOneOffLowPriorityMinutes    = 720
	defaultOneOffMediumPriorityMinutes = 180
	defaultOneOffHighPriorityMinutes   = 60
)

func (s *Service) GetOneOffReminderSettings(ctx context.Context, userID int64) (*domain.OneOffReminderSettings, error) {
	return s.ensureOneOffReminderSettings(ctx, userID)
}

func (s *Service) UpdateOneOffReminderSettings(ctx context.Context, userID int64, lowPriorityMinutes, mediumPriorityMinutes, highPriorityMinutes int) error {
	if lowPriorityMinutes <= 0 || mediumPriorityMinutes <= 0 || highPriorityMinutes <= 0 {
		return fmt.Errorf("one-off reminder intervals must be positive")
	}

	now := s.now().UTC()
	settings := &domain.OneOffReminderSettings{
		UserID:                userID,
		LowPriorityMinutes:    lowPriorityMinutes,
		MediumPriorityMinutes: mediumPriorityMinutes,
		HighPriorityMinutes:   highPriorityMinutes,
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	if err := s.repo.SaveOneOffReminderSettings(ctx, settings); err != nil {
		return err
	}

	return s.rescheduleActiveOneOffReminders(ctx, userID, settings, now)
}

func (s *Service) CreateOneOffTask(ctx context.Context, userID int64, title string, priority domain.OneOffTaskPriority, checklistTitles []string) (*domain.OneOffTask, error) {
	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		return nil, domain.ErrEmptyTitle
	}

	normalizedPriority, err := normalizeOneOffPriority(priority)
	if err != nil {
		return nil, err
	}

	settings, err := s.ensureOneOffReminderSettings(ctx, userID)
	if err != nil {
		return nil, err
	}

	now := s.now().UTC()
	task := &domain.OneOffTask{
		UserID:    userID,
		Title:     cleanTitle,
		Priority:  normalizedPriority,
		Status:    domain.OneOffTaskStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	nextReminder := now.Add(time.Duration(oneOffReminderIntervalMinutes(settings, normalizedPriority)) * time.Minute)
	task.NextReminderAt = &nextReminder

	for i, itemTitle := range checklistTitles {
		cleanItemTitle := strings.TrimSpace(itemTitle)
		if cleanItemTitle == "" {
			continue
		}

		task.Items = append(task.Items, domain.OneOffTaskItem{
			Title:     cleanItemTitle,
			SortOrder: i + 1,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}

	if err := s.repo.SaveOneOffTask(ctx, task); err != nil {
		return nil, err
	}

	return task, nil
}

func (s *Service) GetOneOffTask(ctx context.Context, userID, taskID int64) (*domain.OneOffTask, error) {
	return s.repo.GetOneOffTask(ctx, userID, taskID)
}

func (s *Service) ListOneOffTasks(ctx context.Context, userID int64) ([]domain.OneOffTask, error) {
	return s.repo.ListOneOffTasks(ctx, userID)
}

func (s *Service) DeleteOneOffTask(ctx context.Context, userID, taskID int64) error {
	return s.repo.DeleteOneOffTask(ctx, userID, taskID)
}

func (s *Service) ToggleOneOffTaskItem(ctx context.Context, userID, taskID, itemID int64, now time.Time) (*domain.OneOffTask, error) {
	task, err := s.repo.GetOneOffTask(ctx, userID, taskID)
	if err != nil {
		return nil, err
	}
	if task.Status == domain.OneOffTaskStatusCompleted {
		return nil, fmt.Errorf("one-off task is closed")
	}

	stamp := now.UTC()
	found := false
	for i := range task.Items {
		if task.Items[i].ID != itemID {
			continue
		}
		found = true
		task.Items[i].Completed = !task.Items[i].Completed
		task.Items[i].UpdatedAt = stamp
		if task.Items[i].Completed {
			task.Items[i].CompletedAt = &stamp
		} else {
			task.Items[i].CompletedAt = nil
		}
		break
	}
	if !found {
		return nil, domain.ErrNotFound
	}

	task.UpdatedAt = stamp
	if oneOffAllItemsCompleted(task) {
		completeOneOffTask(task, stamp)
	} else {
		settings, err := s.ensureOneOffReminderSettings(ctx, userID)
		if err != nil {
			return nil, err
		}
		nextReminder := stamp.Add(time.Duration(oneOffReminderIntervalMinutes(settings, task.Priority)) * time.Minute)
		task.NextReminderAt = &nextReminder
		task.CompletedAt = nil
		task.Status = domain.OneOffTaskStatusActive
	}

	if err := s.repo.SaveOneOffTask(ctx, task); err != nil {
		return nil, err
	}

	return task, nil
}

func (s *Service) CompleteOneOffTask(ctx context.Context, userID, taskID int64, now time.Time) (*domain.OneOffTask, error) {
	task, err := s.repo.GetOneOffTask(ctx, userID, taskID)
	if err != nil {
		return nil, err
	}
	if task.Status == domain.OneOffTaskStatusCompleted {
		return task, nil
	}

	stamp := now.UTC()
	for i := range task.Items {
		if task.Items[i].Completed {
			continue
		}
		task.Items[i].Completed = true
		task.Items[i].CompletedAt = &stamp
		task.Items[i].UpdatedAt = stamp
	}
	completeOneOffTask(task, stamp)

	if err := s.repo.SaveOneOffTask(ctx, task); err != nil {
		return nil, err
	}

	return task, nil
}

func (s *Service) PickOneOffReminder(ctx context.Context, userID int64, now time.Time) (*domain.OneOffTask, error) {
	tasks, err := s.repo.ListOneOffTasks(ctx, userID)
	if err != nil {
		return nil, err
	}

	task := pickDueOneOffTask(tasks, now.UTC())
	if task == nil {
		return nil, domain.ErrNotFound
	}

	settings, err := s.ensureOneOffReminderSettings(ctx, userID)
	if err != nil {
		return nil, err
	}

	stamp := now.UTC()
	nextReminder := stamp.Add(time.Duration(oneOffReminderIntervalMinutes(settings, task.Priority)) * time.Minute)
	task.NextReminderAt = &nextReminder
	task.UpdatedAt = stamp

	if err := s.repo.SaveOneOffTask(ctx, task); err != nil {
		return nil, err
	}

	return task, nil
}

func (s *Service) ensureOneOffReminderSettings(ctx context.Context, userID int64) (*domain.OneOffReminderSettings, error) {
	settings, err := s.repo.GetOneOffReminderSettings(ctx, userID)
	if err == nil {
		return settings, nil
	}
	if err != domain.ErrNotFound {
		return nil, err
	}

	now := s.now().UTC()
	settings = &domain.OneOffReminderSettings{
		UserID:                userID,
		LowPriorityMinutes:    defaultOneOffLowPriorityMinutes,
		MediumPriorityMinutes: defaultOneOffMediumPriorityMinutes,
		HighPriorityMinutes:   defaultOneOffHighPriorityMinutes,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if err := s.repo.SaveOneOffReminderSettings(ctx, settings); err != nil {
		return nil, err
	}

	return settings, nil
}

func normalizeOneOffPriority(priority domain.OneOffTaskPriority) (domain.OneOffTaskPriority, error) {
	switch priority {
	case domain.OneOffTaskPriorityLow, domain.OneOffTaskPriorityMedium, domain.OneOffTaskPriorityHigh:
		return priority, nil
	default:
		return "", fmt.Errorf("invalid one-off priority: %s", priority)
	}
}

func oneOffReminderIntervalMinutes(settings *domain.OneOffReminderSettings, priority domain.OneOffTaskPriority) int {
	switch priority {
	case domain.OneOffTaskPriorityLow:
		return settings.LowPriorityMinutes
	case domain.OneOffTaskPriorityHigh:
		return settings.HighPriorityMinutes
	default:
		return settings.MediumPriorityMinutes
	}
}

func oneOffAllItemsCompleted(task *domain.OneOffTask) bool {
	if len(task.Items) == 0 {
		return false
	}
	for _, item := range task.Items {
		if !item.Completed {
			return false
		}
	}
	return true
}

func pickDueOneOffTask(tasks []domain.OneOffTask, now time.Time) *domain.OneOffTask {
	var picked *domain.OneOffTask
	for i := range tasks {
		task := &tasks[i]
		if task.Status != domain.OneOffTaskStatusActive || task.NextReminderAt == nil || task.NextReminderAt.After(now) {
			continue
		}
		if picked == nil || isHigherPriorityOneOffTask(task, picked) {
			picked = task
		}
	}
	return picked
}

func isHigherPriorityOneOffTask(left, right *domain.OneOffTask) bool {
	leftRank := oneOffPriorityRank(left.Priority)
	rightRank := oneOffPriorityRank(right.Priority)
	if leftRank != rightRank {
		return leftRank > rightRank
	}

	leftReminder := time.Time{}
	if left.NextReminderAt != nil {
		leftReminder = left.NextReminderAt.UTC()
	}
	rightReminder := time.Time{}
	if right.NextReminderAt != nil {
		rightReminder = right.NextReminderAt.UTC()
	}
	if !leftReminder.Equal(rightReminder) {
		return leftReminder.Before(rightReminder)
	}

	return left.ID < right.ID
}

func oneOffPriorityRank(priority domain.OneOffTaskPriority) int {
	switch priority {
	case domain.OneOffTaskPriorityHigh:
		return 3
	case domain.OneOffTaskPriorityMedium:
		return 2
	default:
		return 1
	}
}

func completeOneOffTask(task *domain.OneOffTask, stamp time.Time) {
	task.Status = domain.OneOffTaskStatusCompleted
	task.CompletedAt = &stamp
	task.NextReminderAt = nil
	task.UpdatedAt = stamp
}

func (s *Service) rescheduleActiveOneOffReminders(ctx context.Context, userID int64, settings *domain.OneOffReminderSettings, now time.Time) error {
	tasks, err := s.repo.ListOneOffTasks(ctx, userID)
	if err != nil {
		return err
	}

	for i := range tasks {
		task := &tasks[i]
		if task.Status != domain.OneOffTaskStatusActive {
			continue
		}

		nextReminder := now.Add(time.Duration(oneOffReminderIntervalMinutes(settings, task.Priority)) * time.Minute)
		task.NextReminderAt = &nextReminder
		task.UpdatedAt = now

		if err := s.repo.SaveOneOffTask(ctx, task); err != nil {
			return err
		}
	}

	return nil
}
