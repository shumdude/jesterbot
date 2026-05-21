// AI-AGENT: SQLite persistence for user shop items.
// Entry points implement service.Repository shop methods.
// Tightly coupled to migrations.go shop_items schema and internal/service/shop.go.
// Keep cost positive at service level; repository only enforces ownership and row existence.
//
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"jesterbot/internal/domain"
)

func (r *Repository) CreateShopItem(ctx context.Context, item *domain.ShopItem) error {
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO shop_items (user_id, title, cost, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`,
		item.UserID,
		item.Title,
		item.Cost,
		formatTime(item.CreatedAt),
		formatTime(item.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("create shop item: %w", err)
	}

	item.ID, err = result.LastInsertId()
	if err != nil {
		return fmt.Errorf("shop item last insert id: %w", err)
	}

	return nil
}

func (r *Repository) UpdateShopItem(ctx context.Context, userID, itemID int64, title string, cost int) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE shop_items
		SET title = ?, cost = ?, updated_at = ?
		WHERE id = ? AND user_id = ?`,
		title,
		cost,
		formatTime(time.Now().UTC()),
		itemID,
		userID,
	)
	if err != nil {
		return fmt.Errorf("update shop item: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("shop item rows affected: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}

	return nil
}

func (r *Repository) DeleteShopItem(ctx context.Context, userID, itemID int64) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM shop_items
		WHERE id = ? AND user_id = ?`,
		itemID,
		userID,
	)
	if err != nil {
		return fmt.Errorf("delete shop item: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("shop item rows affected: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}

	return nil
}

func (r *Repository) GetShopItem(ctx context.Context, userID, itemID int64) (*domain.ShopItem, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, title, cost, created_at, updated_at
		FROM shop_items
		WHERE id = ? AND user_id = ?`,
		itemID,
		userID,
	)
	return scanShopItem(row)
}

func (r *Repository) ListShopItems(ctx context.Context, userID int64) ([]domain.ShopItem, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, title, cost, created_at, updated_at
		FROM shop_items
		WHERE user_id = ?
		ORDER BY id`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list shop items: %w", err)
	}
	defer rows.Close()

	items := make([]domain.ShopItem, 0)
	for rows.Next() {
		item, err := scanShopItemRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("shop item rows: %w", err)
	}

	return items, nil
}

func scanShopItem(row *sql.Row) (*domain.ShopItem, error) {
	item, err := scanShopItemScanner(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return item, nil
}

func scanShopItemRows(rows *sql.Rows) (*domain.ShopItem, error) {
	return scanShopItemScanner(rows)
}

func scanShopItemScanner(scanner interface{ Scan(dest ...any) error }) (*domain.ShopItem, error) {
	var (
		item                 domain.ShopItem
		createdAt, updatedAt string
	)
	if err := scanner.Scan(
		&item.ID,
		&item.UserID,
		&item.Title,
		&item.Cost,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, fmt.Errorf("scan shop item: %w", err)
	}

	var err error
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
