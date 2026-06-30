package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/darrell/golang-system/golang-app2/internal/domain"
)

// OrderPostgres is the PostgreSQL implementation of OrderRepository.
type OrderPostgres struct {
	pool *pgxpool.Pool
}

func NewOrderPostgres(pool *pgxpool.Pool) *OrderPostgres {
	return &OrderPostgres{pool: pool}
}

func (r *OrderPostgres) Create(ctx context.Context, order *domain.Order) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`INSERT INTO orders (id, status, total, note, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6)`,
		order.ID, order.Status, order.Total, order.Note, order.CreatedAt, order.UpdatedAt,
	); err != nil {
		return fmt.Errorf("insert order: %w", err)
	}
	for _, it := range order.Items {
		if _, err := tx.Exec(ctx,
			`INSERT INTO order_items (id, order_id, item_id, item_name, price, quantity) VALUES ($1,$2,$3,$4,$5,$6)`,
			it.ID, order.ID, it.ItemID, it.ItemName, it.Price, it.Quantity,
		); err != nil {
			return fmt.Errorf("insert item: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (r *OrderPostgres) GetByID(ctx context.Context, id string) (*domain.Order, error) {
	order := &domain.Order{ID: id}
	err := r.pool.QueryRow(ctx,
		`SELECT status, total, note, created_at, updated_at FROM orders WHERE id=$1`, id,
	).Scan(&order.Status, &order.Total, &order.Note, &order.CreatedAt, &order.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("order %s: not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("query order: %w", err)
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, item_id, item_name, price, quantity FROM order_items WHERE order_id=$1`, id)
	if err != nil {
		return nil, fmt.Errorf("query items: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		it := domain.OrderItem{}
		if err := rows.Scan(&it.ID, &it.ItemID, &it.ItemName, &it.Price, &it.Quantity); err != nil {
			return nil, fmt.Errorf("scan item: %w", err)
		}
		order.Items = append(order.Items, it)
	}
	return order, nil
}

func (r *OrderPostgres) List(ctx context.Context, offset, limit int, status string) ([]*domain.Order, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM orders WHERE ($1 = '' OR status = $1)`, status).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count: %w", err)
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, status, total, note, created_at, updated_at FROM orders WHERE ($1 = '' OR status = $1) ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		status, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list: %w", err)
	}
	defer rows.Close()
	var out []*domain.Order
	for rows.Next() {
		o := &domain.Order{}
		if err := rows.Scan(&o.ID, &o.Status, &o.Total, &o.Note, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan: %w", err)
		}
		out = append(out, o)
	}
	return out, total, nil
}

func (r *OrderPostgres) Update(ctx context.Context, order *domain.Order) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE orders SET status=$1, total=$2, note=$3, updated_at=$4 WHERE id=$5`,
		order.Status, order.Total, order.Note, order.UpdatedAt, order.ID,
	)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	return nil
}
