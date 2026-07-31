// Package repository contains the PostgreSQL adapter for ItemRepository.
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/darrell/golang-system/golang-app/internal/domain"
	"github.com/darrell/golang-system/golang-commons/database"
)

// ItemPostgres is the PostgreSQL implementation of ItemRepository.
type ItemPostgres struct {
	pool *pgxpool.Pool
}

// NewItemPostgres creates a new ItemPostgres adapter.
func NewItemPostgres(pool *pgxpool.Pool) *ItemPostgres {
	return &ItemPostgres{pool: pool}
}

func (r *ItemPostgres) Create(ctx context.Context, item *domain.Item) error {
	const q = `
		INSERT INTO items (id, name, description, status, tags, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := database.Conn(ctx, r.pool).Exec(ctx, q, item.ID, item.Name, item.Description, item.Status, item.Tags, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert item: %w", err)
	}
	return nil
}

func (r *ItemPostgres) GetByID(ctx context.Context, id string) (*domain.Item, error) {
	const q = `SELECT id, name, description, status, tags, created_at, updated_at FROM items WHERE id = $1`
	item := &domain.Item{}
	var tags []string
	err := database.Conn(ctx, r.pool).QueryRow(ctx, q, id).Scan(
		&item.ID, &item.Name, &item.Description, &item.Status, &tags, &item.CreatedAt, &item.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("item %s: not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("query item: %w", err)
	}
	item.Tags = tags
	return item, nil
}

func (r *ItemPostgres) List(ctx context.Context, offset, limit int, status string) ([]*domain.Item, int, error) {
	q := database.Conn(ctx, r.pool)
	var total int
	if err := q.QueryRow(ctx, `SELECT COUNT(*) FROM items WHERE ($1 = '' OR status = $1)`, status).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count items: %w", err)
	}
	rows, err := q.Query(ctx,
		`SELECT id, name, description, status, tags, created_at, updated_at FROM items WHERE ($1 = '' OR status = $1) ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		status, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list items: %w", err)
	}
	defer rows.Close()
	var out []*domain.Item
	for rows.Next() {
		it := &domain.Item{}
		var tags []string
		if err := rows.Scan(&it.ID, &it.Name, &it.Description, &it.Status, &tags, &it.CreatedAt, &it.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan item: %w", err)
		}
		it.Tags = tags
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("list items rows: %w", err)
	}
	return out, total, nil
}

func (r *ItemPostgres) Update(ctx context.Context, item *domain.Item) error {
	const q = `UPDATE items SET name=$1, description=$2, status=$3, tags=$4, updated_at=$5 WHERE id=$6`
	_, err := database.Conn(ctx, r.pool).Exec(ctx, q, item.Name, item.Description, item.Status, item.Tags, item.UpdatedAt, item.ID)
	if err != nil {
		return fmt.Errorf("update item: %w", err)
	}
	return nil
}

func (r *ItemPostgres) Delete(ctx context.Context, id string) error {
	_, err := database.Conn(ctx, r.pool).Exec(ctx, `DELETE FROM items WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete item: %w", err)
	}
	return nil
}
