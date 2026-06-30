package service

import (
	"context"

	"github.com/darrell/golang-system/golang-app/internal/domain"
	"github.com/darrell/golang-system/golang-events"
)

// ItemRepository is the persistence port for items.
type ItemRepository interface {
	Create(ctx context.Context, item *domain.Item) error
	GetByID(ctx context.Context, id string) (*domain.Item, error)
	List(ctx context.Context, offset, limit int, status string) ([]*domain.Item, int, error)
	Update(ctx context.Context, item *domain.Item) error
	Delete(ctx context.Context, id string) error
}

// EventPublisher is the port for publishing domain events.
type EventPublisher interface {
	PublishItemCreated(ctx context.Context, item *domain.Item) error
	PublishItemUpdated(ctx context.Context, item *domain.Item) error
	PublishItemDeleted(ctx context.Context, id string) error
}

// ItemService is the inbound port for item business operations.
type ItemService interface {
	CreateItem(ctx context.Context, req *domain.CreateItemRequest) (*domain.Item, error)
	GetItem(ctx context.Context, id string) (*domain.Item, error)
	ListItems(ctx context.Context, offset, limit int, status string) ([]*domain.Item, int, error)
	UpdateItem(ctx context.Context, id string, req *domain.UpdateItemRequest) (*domain.Item, error)
	DeleteItem(ctx context.Context, id string) error

	// HandleOrderCreated tracks which items are referenced by an order.
	HandleOrderCreated(ctx context.Context, evt *events.Event[events.OrderCreatedData]) error
	// HandleGetItemByID implements the request-reply handler.
	HandleGetItemByID(ctx context.Context, req *events.GetItemRequest) (*events.GetItemResponse, error)
	// HandleExternalCreate handles an item.created event from an external system.
	HandleExternalCreate(ctx context.Context, evt *events.Event[events.ItemCreatedData]) error
}
