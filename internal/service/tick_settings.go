package service

import (
	"context"
	"fmt"

	"jesterbot/internal/domain"
)

const defaultUserTickIntervalMinutes = 1

func (s *Service) GetUserTickInterval(ctx context.Context, userID int64) (int, error) {
	minutes, err := s.repo.GetUserTickInterval(ctx, userID)
	if err == nil {
		return minutes, nil
	}
	if err != domain.ErrNotFound {
		return 0, err
	}

	if err := s.repo.SaveUserTickInterval(ctx, userID, defaultUserTickIntervalMinutes); err != nil {
		return 0, err
	}

	return defaultUserTickIntervalMinutes, nil
}

func (s *Service) UpdateUserTickInterval(ctx context.Context, userID int64, minutes int) error {
	if minutes <= 0 {
		return fmt.Errorf("tick interval must be positive")
	}

	return s.repo.SaveUserTickInterval(ctx, userID, minutes)
}
