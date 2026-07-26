// Package outbox implements a transactional outbox store and JetStream relay.
package outbox

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"

	"github.com/darrell/golang-system/golang-commons/database"
)

// Message is a pending domain event row.
type Message struct {
	ID        string
	Subject   string
	Payload   []byte
	CreatedAt time.Time
}

// NewMessage builds an outbox message with a fresh ID when empty.
func NewMessage(subject string, payload []byte) Message {
	return Message{
		ID:      uuid.New().String(),
		Subject: subject,
		Payload: payload,
	}
}

// Store persists outbox rows (must share the entity write transaction).
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates an outbox Store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Insert writes a pending outbox row using the tx bound to ctx when present.
func (s *Store) Insert(ctx context.Context, msg Message) error {
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	_, err := database.Conn(ctx, s.pool).Exec(ctx,
		`INSERT INTO outbox (id, subject, payload, created_at) VALUES ($1, $2, $3, NOW())`,
		msg.ID, msg.Subject, msg.Payload,
	)
	if err != nil {
		return fmt.Errorf("insert outbox: %w", err)
	}
	return nil
}

// ClaimPending returns unpublished rows for relay (at-least-once; consumers are idempotent).
func (s *Store) ClaimPending(ctx context.Context, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, subject, payload, created_at
		FROM outbox
		WHERE published_at IS NULL
		ORDER BY created_at
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("claim outbox: %w", err)
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.Subject, &m.Payload, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan outbox: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MarkPublished marks a row as successfully published.
func (s *Store) MarkPublished(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE outbox SET published_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("mark outbox published: %w", err)
	}
	return nil
}

// TryMarkProcessed records an event ID for consumer idempotency.
// Returns true when this is the first time the event is seen.
func TryMarkProcessed(ctx context.Context, pool *pgxpool.Pool, eventID string) (bool, error) {
	if eventID == "" {
		return true, nil
	}
	tag, err := pool.Exec(ctx,
		`INSERT INTO processed_events (event_id, processed_at) VALUES ($1, NOW()) ON CONFLICT DO NOTHING`,
		eventID,
	)
	if err != nil {
		return false, fmt.Errorf("mark processed: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// Relay publishes pending outbox rows to JetStream until ctx is cancelled.
func Relay(ctx context.Context, store *Store, js nats.JetStreamContext, log zerolog.Logger, every time.Duration) {
	if every <= 0 {
		every = 500 * time.Millisecond
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			msgs, err := store.ClaimPending(ctx, 50)
			if err != nil {
				log.Error().Err(err).Msg("outbox claim failed")
				continue
			}
			for _, m := range msgs {
				if _, err := js.Publish(m.Subject, m.Payload); err != nil {
					log.Error().Err(err).Str("subject", m.Subject).Str("id", m.ID).Msg("outbox publish failed")
					continue
				}
				if err := store.MarkPublished(ctx, m.ID); err != nil {
					log.Error().Err(err).Str("id", m.ID).Msg("outbox mark published failed")
				}
			}
		}
	}
}
