// AI-AGENT: Shop service logic for user-owned purchasable items and doubloon balance changes.
// Entry points are Service methods such as ListShopItems, SaveShopItem, DeleteShopItem, and BuyShopItem.
// Tightly coupled to Repository shop methods, domain.ShopItem, and Telegram shop handlers.
// Keep purchases atomic in repository implementations because balance updates must not race.
//
package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"jesterbot/internal/domain"
)

func (s *Service) ListShopItems(ctx context.Context, userID int64) ([]domain.ShopItem, error) {
	return s.repo.ListShopItems(ctx, userID)
}

func (s *Service) GetShopItem(ctx context.Context, userID, itemID int64) (*domain.ShopItem, error) {
	return s.repo.GetShopItem(ctx, userID, itemID)
}

func (s *Service) SaveShopItem(ctx context.Context, userID, itemID int64, title string, cost int) (*domain.ShopItem, error) {
	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		return nil, domain.ErrEmptyTitle
	}
	if cost < 1 {
		return nil, fmt.Errorf("shop item cost must be positive")
	}

	if itemID != 0 {
		if err := s.repo.UpdateShopItem(ctx, userID, itemID, cleanTitle, cost); err != nil {
			return nil, err
		}
		return s.repo.GetShopItem(ctx, userID, itemID)
	}

	now := s.now().UTC()
	item := &domain.ShopItem{
		UserID:    userID,
		Title:     cleanTitle,
		Cost:      cost,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.repo.CreateShopItem(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) DeleteShopItem(ctx context.Context, userID, itemID int64) error {
	return s.repo.DeleteShopItem(ctx, userID, itemID)
}

func (s *Service) BuyShopItem(ctx context.Context, userID, itemID int64, now time.Time) (*domain.ShopPurchase, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := s.resetDoubloonsIfPastDayEnd(ctx, user, now); err != nil {
		return nil, err
	}

	item, err := s.repo.GetShopItem(ctx, userID, itemID)
	if err != nil {
		return nil, err
	}
	if item.Cost < 1 {
		return nil, fmt.Errorf("shop item cost must be positive")
	}
	if user.DoubloonsBalance < item.Cost {
		return nil, fmt.Errorf("not enough doubloons")
	}

	balance, err := s.repo.AddUserDoubloons(ctx, userID, -item.Cost)
	if err != nil {
		return nil, err
	}
	return &domain.ShopPurchase{
		Item:    *item,
		Spent:   item.Cost,
		Balance: balance,
	}, nil
}
