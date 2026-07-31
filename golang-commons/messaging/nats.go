// Package messaging provides NATS / JetStream publisher abstractions and connection utilities.
package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
)

const (
	// StreamName is the JetStream stream for domain events.
	StreamName = "GOLANG_SYSTEM"
)

// DefaultSubjects covered by the domain-events stream.
var DefaultSubjects = []string{"item.>", "order.>"}

// Publisher is the contract for event publishers.
type Publisher interface {
	Publish(ctx context.Context, subject string, payload any) error
}

// NatsPublisher publishes events over JetStream when available, else Core NATS.
type NatsPublisher struct {
	js  nats.JetStreamContext
	nc  *nats.Conn
	log zerolog.Logger
}

// NewNatsPublisher wraps a JetStream context (preferred) and underlying conn.
func NewNatsPublisher(nc *nats.Conn, js nats.JetStreamContext, log zerolog.Logger) *NatsPublisher {
	return &NatsPublisher{nc: nc, js: js, log: log}
}

// Publish encodes payload as JSON and publishes to the given subject.
func (p *NatsPublisher) Publish(ctx context.Context, subject string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if p.js != nil {
		_, err = p.js.Publish(subject, data, nats.Context(ctx))
		return err
	}
	return p.nc.Publish(subject, data)
}

// NoopPublisher is a no-op Publisher useful for tests.
type NoopPublisher struct{}

// Publish does nothing.
func (NoopPublisher) Publish(_ context.Context, _ string, _ any) error { return nil }

// Connect establishes a NATS connection with retry.
func Connect(url string) (*nats.Conn, error) {
	var lastErr error
	for i := range 10 {
		nc, err := nats.Connect(url, nats.Timeout(5*time.Second), nats.Name("golang-system"))
		if err == nil {
			return nc, nil
		}
		lastErr = err
		time.Sleep(time.Duration(i+1) * time.Second)
	}
	return nil, errors.Join(errors.New("failed to connect to NATS"), lastErr)
}

// ConnectJetStream connects and returns a JetStream context, ensuring the stream exists.
func ConnectJetStream(url string, subjects []string) (*nats.Conn, nats.JetStreamContext, error) {
	nc, err := Connect(url)
	if err != nil {
		return nil, nil, err
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("jetstream: %w", err)
	}
	if len(subjects) == 0 {
		subjects = DefaultSubjects
	}
	if err := EnsureStream(js, StreamName, subjects); err != nil {
		nc.Close()
		return nil, nil, err
	}
	return nc, js, nil
}

// EnsureStream creates or updates the JetStream stream for domain events.
func EnsureStream(js nats.JetStreamContext, name string, subjects []string) error {
	_, err := js.StreamInfo(name)
	if err == nil {
		_, err = js.UpdateStream(&nats.StreamConfig{
			Name:      name,
			Subjects:  subjects,
			Storage:   nats.FileStorage,
			Retention: nats.LimitsPolicy,
		})
		return err
	}
	_, err = js.AddStream(&nats.StreamConfig{
		Name:      name,
		Subjects:  subjects,
		Storage:   nats.FileStorage,
		Retention: nats.LimitsPolicy,
	})
	return err
}

// Handler processes a JetStream message. Return nil to Ack; non-nil to Nak.
type Handler func(ctx context.Context, subject string, data []byte) error

// QueueSubscribeDurable subscribes with a durable consumer and explicit Ack.
func QueueSubscribeDurable(js nats.JetStreamContext, subject, durable string, handler Handler, log zerolog.Logger) (*nats.Subscription, error) {
	return js.QueueSubscribe(subject, durable, func(m *nats.Msg) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := handler(ctx, m.Subject, m.Data); err != nil {
			log.Error().Err(err).Str("subject", m.Subject).Msg("jetstream handler failed")
			_ = m.Nak()
			return
		}
		_ = m.Ack()
	}, nats.Durable(durable), nats.ManualAck(), nats.AckWait(30*time.Second))
}
