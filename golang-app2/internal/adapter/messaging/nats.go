package messaging

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"

	"github.com/darrell/golang-system/golang-app2/internal/domain"
	"github.com/darrell/golang-system/golang-events"
)

// NatsPublisher publishes order domain events via NATS.
type NatsPublisher struct {
	conn *nats.Conn
	log  zerolog.Logger
}

func NewNatsPublisher(conn *nats.Conn, log zerolog.Logger) *NatsPublisher {
	return &NatsPublisher{conn: conn, log: log}
}

func (p *NatsPublisher) publish(subject string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return p.conn.Publish(subject, data)
}

func (p *NatsPublisher) PublishOrderCreated(_ context.Context, o *domain.Order) error {
	evt := events.Event[events.OrderCreatedData]{
		ID: uuid.New().String(), Type: events.SubjectOrderCreated, Source: "golang-app2",
		Timestamp: time.Now().UTC(),
		Data: events.OrderCreatedData{
			ID: o.ID, Items: toEventItems(o.Items), Total: o.Total, Status: string(o.Status),
		},
	}
	return p.publish(events.SubjectOrderCreated, evt)
}

func (p *NatsPublisher) PublishOrderUpdated(_ context.Context, o *domain.Order) error {
	evt := events.Event[events.OrderUpdatedData]{
		ID: uuid.New().String(), Type: events.SubjectOrderUpdated, Source: "golang-app2",
		Timestamp: time.Now().UTC(),
		Data: events.OrderUpdatedData{
			ID: o.ID, Items: toEventItems(o.Items), Total: o.Total, Status: string(o.Status),
		},
	}
	return p.publish(events.SubjectOrderUpdated, evt)
}

func (p *NatsPublisher) PublishOrderCanceled(_ context.Context, o *domain.Order, reason string) error {
	evt := events.Event[events.OrderCanceledData]{
		ID: uuid.New().String(), Type: events.SubjectOrderCanceled, Source: "golang-app2",
		Timestamp: time.Now().UTC(),
		Data: events.OrderCanceledData{ID: o.ID, Reason: reason},
	}
	return p.publish(events.SubjectOrderCanceled, evt)
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
