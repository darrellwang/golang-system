// Package messaging provides NATS publisher abstractions and connection utilities.
package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
)

// Publisher is the contract for event publishers.
type Publisher interface {
	Publish(ctx context.Context, subject string, payload any) error
}

// NatsPublisher publishes events over NATS.
type NatsPublisher struct {
	conn *nats.Conn
	log  zerolog.Logger
}

// NewNatsPublisher wraps a NATS connection.
func NewNatsPublisher(conn *nats.Conn, log zerolog.Logger) *NatsPublisher {
	return &NatsPublisher{conn: conn, log: log}
}

// Publish encodes payload as JSON and publishes to the given subject.
func (p *NatsPublisher) Publish(ctx context.Context, subject string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return p.conn.Publish(subject, data)
}

// NoopPublisher is a no-op Publisher useful for tests.
type NoopPublisher struct{}

// Publish does nothing.
func (NoopPublisher) Publish(_ context.Context, _ string, _ any) error { return nil }

// Connect establishes a NATS connection with retry.
func Connect(url string) (*nats.Conn, error) {
	var lastErr error
	for i := range 10 {
		nc, err := nats.Connect(url, nats.Timeout(5*time.Second))
		if err == nil {
			return nc, nil
		}
		lastErr = err
		time.Sleep(time.Duration(i+1) * time.Second)
	}
	return nil, errors.Join(errors.New("failed to connect to NATS"), lastErr)
}
