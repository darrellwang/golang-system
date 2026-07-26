// Package messaging contains the outbox-backed publisher adapter for item events.
package messaging

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/darrell/golang-system/golang-commons/outbox"
	"github.com/darrell/golang-system/golang-app/internal/domain"
	"github.com/darrell/golang-system/golang-events"
)

// OutboxPublisher enqueues item domain events into the transactional outbox.
type OutboxPublisher struct {
	store *outbox.Store
	log   zerolog.Logger
}

// NewOutboxPublisher creates a publisher that writes to the outbox table.
func NewOutboxPublisher(store *outbox.Store, log zerolog.Logger) *OutboxPublisher {
	return &OutboxPublisher{store: store, log: log}
}

func (p *OutboxPublisher) enqueue(ctx context.Context, subject string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return p.store.Insert(ctx, outbox.NewMessage(subject, data))
}

func (p *OutboxPublisher) PublishItemCreated(ctx context.Context, item *domain.Item) error {
	evt := events.Event[events.ItemCreatedData]{
		ID: uuid.New().String(), Type: events.SubjectItemCreated, Source: "golang-app",
		Version: events.SchemaVersion, Timestamp: time.Now().UTC(),
		Data: events.ItemCreatedData{
			ID: item.ID, Name: item.Name, Description: item.Description,
			Status: string(item.Status), Tags: item.Tags,
		},
	}
	return p.enqueue(ctx, events.SubjectItemCreated, evt)
}

func (p *OutboxPublisher) PublishItemUpdated(ctx context.Context, item *domain.Item) error {
	evt := events.Event[events.ItemUpdatedData]{
		ID: uuid.New().String(), Type: events.SubjectItemUpdated, Source: "golang-app",
		Version: events.SchemaVersion, Timestamp: time.Now().UTC(),
		Data: events.ItemUpdatedData{
			ID: item.ID, Name: item.Name, Description: item.Description,
			Status: string(item.Status), Tags: item.Tags,
		},
	}
	return p.enqueue(ctx, events.SubjectItemUpdated, evt)
}

func (p *OutboxPublisher) PublishItemDeleted(ctx context.Context, id string) error {
	evt := events.Event[events.ItemDeletedData]{
		ID: uuid.New().String(), Type: events.SubjectItemDeleted, Source: "golang-app",
		Version: events.SchemaVersion, Timestamp: time.Now().UTC(),
		Data: events.ItemDeletedData{ID: id},
	}
	return p.enqueue(ctx, events.SubjectItemDeleted, evt)
}
