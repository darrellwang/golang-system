package messaging

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/darrell/golang-system/golang-app2/internal/domain"
	"github.com/darrell/golang-system/golang-commons/outbox"
	"github.com/darrell/golang-system/golang-events"
)

// OutboxPublisher enqueues order domain events into the transactional outbox.
type OutboxPublisher struct {
	store *outbox.Store
	log   zerolog.Logger
}

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

func (p *OutboxPublisher) PublishOrderCreated(ctx context.Context, o *domain.Order) error {
	evt := events.Event[events.OrderCreatedData]{
		ID: uuid.New().String(), Type: events.SubjectOrderCreated, Source: "golang-app2",
		Version: events.SchemaVersion, Timestamp: time.Now().UTC(),
		Data: events.OrderCreatedData{
			ID: o.ID, Items: toEventItems(o.Items), Total: o.Total, Status: string(o.Status),
		},
	}
	return p.enqueue(ctx, events.SubjectOrderCreated, evt)
}

func (p *OutboxPublisher) PublishOrderUpdated(ctx context.Context, o *domain.Order) error {
	evt := events.Event[events.OrderUpdatedData]{
		ID: uuid.New().String(), Type: events.SubjectOrderUpdated, Source: "golang-app2",
		Version: events.SchemaVersion, Timestamp: time.Now().UTC(),
		Data: events.OrderUpdatedData{
			ID: o.ID, Items: toEventItems(o.Items), Total: o.Total, Status: string(o.Status),
		},
	}
	return p.enqueue(ctx, events.SubjectOrderUpdated, evt)
}

func (p *OutboxPublisher) PublishOrderCanceled(ctx context.Context, o *domain.Order, reason string) error {
	evt := events.Event[events.OrderCanceledData]{
		ID: uuid.New().String(), Type: events.SubjectOrderCanceled, Source: "golang-app2",
		Version: events.SchemaVersion, Timestamp: time.Now().UTC(),
		Data: events.OrderCanceledData{ID: o.ID, Reason: reason},
	}
	return p.enqueue(ctx, events.SubjectOrderCanceled, evt)
}

func toEventItems(items []domain.OrderItem) []events.OrderItem {
	out := make([]events.OrderItem, len(items))
	for i, it := range items {
		out[i] = events.OrderItem{
			ItemID: it.ItemID, ItemName: it.ItemName, Price: it.Price, Quantity: it.Quantity,
		}
	}
	return out
}
