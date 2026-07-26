package service

import (
	"context"

	"github.com/darrell/golang-system/golang-app2/internal/domain"
	"github.com/darrell/golang-system/golang-events"
)

// OrderRepository is the persistence port for orders.
type OrderRepository interface {
	Create(ctx context.Context, order *domain.Order) error
	GetByID(ctx context.Context, id string) (*domain.Order, error)
	List(ctx context.Context, offset, limit int, status string) ([]*domain.Order, int, error)
	Update(ctx context.Context, order *domain.Order) error
	ListPendingIDsByItemID(ctx context.Context, itemID string) ([]string, error)
}

// EventPublisher publishes order domain events (outbox-backed).
type EventPublisher interface {
	PublishOrderCreated(ctx context.Context, order *domain.Order) error
	PublishOrderUpdated(ctx context.Context, order *domain.Order) error
	PublishOrderCanceled(ctx context.Context, order *domain.Order, reason string) error
}

// ItemServiceClient looks up items in the item service.
type ItemServiceClient interface {
	GetByID(ctx context.Context, id string) (*domain.ItemView, error)
}

// Transactor runs work inside a DB transaction.
type Transactor interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// OrderService is the inbound port for order business operations.
type OrderService interface {
	CreateOrder(ctx context.Context, req *domain.CreateOrderRequest) (*domain.Order, error)
	GetOrder(ctx context.Context, id string) (*domain.Order, error)
	ListOrders(ctx context.Context, offset, limit int, status string) ([]*domain.Order, int, error)
	UpdateOrder(ctx context.Context, id string, req *domain.UpdateOrderRequest) (*domain.Order, error)
	CancelOrder(ctx context.Context, id string) error

	HandleItemDeleted(ctx context.Context, evt *events.Event[events.ItemDeletedData]) error
	HandleItemUpdated(ctx context.Context, evt *events.Event[events.ItemUpdatedData]) error
}
