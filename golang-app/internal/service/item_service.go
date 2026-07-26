package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	apperror "github.com/darrell/golang-system/golang-commons/apperror"
	"github.com/darrell/golang-system/golang-app/internal/domain"
	"github.com/darrell/golang-system/golang-events"
)

// itemService implements ItemService.
type itemService struct {
	repo      ItemRepository
	publisher EventPublisher
	log       zerolog.Logger
}

// NewItemService constructs an itemService.
func NewItemService(repo ItemRepository, publisher EventPublisher, log zerolog.Logger) ItemService {
	return &itemService{repo: repo, publisher: publisher, log: log}
}

func (s *itemService) CreateItem(ctx context.Context, req *domain.CreateItemRequest) (*domain.Item, error) {
	if req.Name == "" {
		return nil, apperror.NewBadInput("name is required")
	}
	now := time.Now().UTC()
	item := &domain.Item{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Description: req.Description,
		Status:      domain.ItemStatusActive,
		Tags:        req.Tags,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.repo.Create(ctx, item); err != nil {
		return nil, apperror.Internal("create item", err)
	}
	s.publishAsync(ctx, func(ctx context.Context) error {
		return s.publisher.PublishItemCreated(ctx, item)
	})
	return item, nil
}

// publishAsync publishes a domain event without blocking the request path.
// It inherits trace values (request_id, user_id, ...) from ctx via
// context.WithoutCancel, but detaches cancellation so the event is still
// published after the originating request has already returned. Publish
// failures are logged rather than silently dropped — losing domain events
// in an event-driven architecture is a P0 concern.
func (s *itemService) publishAsync(ctx context.Context, fn func(ctx context.Context) error) {
	go func() {
		if err := fn(context.WithoutCancel(ctx)); err != nil {
			s.log.Error().Err(err).Msg("failed to publish domain event")
		}
	}()
}

func (s *itemService) GetItem(ctx context.Context, id string) (*domain.Item, error) {
	item, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, apperror.NotFound("item not found", err)
	}
	return item, nil
}

func (s *itemService) ListItems(ctx context.Context, offset, limit int, status string) ([]*domain.Item, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.List(ctx, offset, limit, status)
}

func (s *itemService) UpdateItem(ctx context.Context, id string, req *domain.UpdateItemRequest) (*domain.Item, error) {
	item, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, apperror.NotFound("item not found", err)
	}
	if req.Name != nil {
		item.Name = *req.Name
	}
	if req.Description != nil {
		item.Description = *req.Description
	}
	if req.Status != nil {
		item.Status = *req.Status
	}
	if req.Tags != nil {
		item.Tags = *req.Tags
	}
	item.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, item); err != nil {
		return nil, apperror.Internal("update item", err)
	}
	s.publishAsync(ctx, func(ctx context.Context) error {
		return s.publisher.PublishItemUpdated(ctx, item)
	})
	return item, nil
}

func (s *itemService) DeleteItem(ctx context.Context, id string) error {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return apperror.NotFound("item not found", err)
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return apperror.Internal("delete item", err)
	}
	s.publishAsync(ctx, func(ctx context.Context) error {
		return s.publisher.PublishItemDeleted(ctx, id)
	})
	return nil
}

// HandleOrderCreated records which items are referenced by new orders.
func (s *itemService) HandleOrderCreated(ctx context.Context, evt *events.Event[events.OrderCreatedData]) error {
	s.log.Info().
		Str("order_id", evt.Data.ID).
		Int("items", len(evt.Data.Items)).
		Msg("order received, recording item references")
	for _, it := range evt.Data.Items {
		s.log.Debug().
			Str("item_id", it.ItemID).
			Str("order_id", evt.Data.ID).
			Msg("item referenced")
	}
	return nil
}

// HandleGetItemByID implements the request-reply handler for item.get_by_id.
func (s *itemService) HandleGetItemByID(ctx context.Context, req *events.GetItemRequest) (*events.GetItemResponse, error) {
	item, err := s.repo.GetByID(ctx, req.ID)
	if err != nil {
		return nil, apperror.NotFound("item not found", err)
	}
	return &events.GetItemResponse{
		ID:          item.ID,
		Name:        item.Name,
		Description: item.Description,
		Status:      string(item.Status),
		Tags:        item.Tags,
	}, nil
}

// HandleExternalCreate handles item.created events from external systems.
func (s *itemService) HandleExternalCreate(ctx context.Context, evt *events.Event[events.ItemCreatedData]) error {
	s.log.Info().
		Str("item_id", evt.Data.ID).
		Str("name", evt.Data.Name).
		Msg("external item.create received")
	return nil
}
