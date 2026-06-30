// Package messaging contains the NATS publisher adapter.
package messaging

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"

	"github.com/darrell/golang-system/golang-app/internal/domain"
	"github.com/darrell/golang-system/golang-events"
)

// NatsPublisher publishes item domain events via NATS.
type NatsPublisher struct {
	conn *nats.Conn
	log  zerolog.Logger
}

// NewNatsPublisher creates a new NatsPublisher.
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

func (p *NatsPublisher) PublishItemCreated(_ context.Context, item *domain.Item) error {
	evt := events.Event[events.ItemCreatedData]{
		ID:        uuid.New().String(),
		Type:      events.SubjectItemCreated,
		Source:    "golang-app",
		Timestamp: time.Now().UTC(),
		Data: events.ItemCreatedData{
			ID:          item.ID,
			Name:        item.Name,
			Description: item.Description,
			Status:      string(item.Status),
			Tags:        item.Tags,
		},
	}
	return p.publish(events.SubjectItemCreated, evt)
}

func (p *NatsPublisher) PublishItemUpdated(_ context.Context, item *domain.Item) error {
	evt := events.Event[events.ItemUpdatedData]{
		ID:        uuid.New().String(),
		Type:      events.SubjectItemUpdated,
		Source:    "golang-app",
		Timestamp: time.Now().UTC(),
		Data: events.ItemUpdatedData{
			ID:          item.ID,
			Name:        item.Name,
			Description: item.Description,
			Status:      string(item.Status),
			Tags:        item.Tags,
		},
	}
	return p.publish(events.SubjectItemUpdated, evt)
}

func (p *NatsPublisher) PublishItemDeleted(_ context.Context, id string) error {
	evt := events.Event[events.ItemDeletedData]{
		ID:        uuid.New().String(),
		Type:      events.SubjectItemDeleted,
		Source:    "golang-app",
		Timestamp: time.Now().UTC(),
		Data:      events.ItemDeletedData{ID: id},
	}
	return p.publish(events.SubjectItemDeleted, evt)
}
