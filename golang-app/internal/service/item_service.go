package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/darrell/golang-system/golang-app/internal/domain"
	apperror "github.com/darrell/golang-system/golang-commons/apperror"
	"github.com/darrell/golang-system/golang-events"
)

type itemService struct {
	repo      ItemRepository
	publisher EventPublisher
	tx        Transactor
	log       zerolog.Logger
}

// NewItemService constructs an itemService.
func NewItemService(repo ItemRepository, publisher EventPublisher, tx Transactor, log zerolog.Logger) ItemService {
	return &itemService{repo: repo, publisher: publisher, tx: tx, log: log}
}

func (s *itemService) CreateItem(ctx context.Context, req *domain.CreateItemRequest) (*domain.Item, error) {
	if req.Name == "" {
		return nil, apperror.NewBadInput("name is required")
	}
	now := time.Now().UTC()
	item := &domain.Item{
		ID: uuid.New().String(), Name: req.Name, Description: req.Description,
		Status: domain.ItemStatusActive, Tags: req.Tags, CreatedAt: now, UpdatedAt: now,
	}
	err := s.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.repo.Create(ctx, item); err != nil {
			return apperror.Internal("create item", err)
		}
		return s.publisher.PublishItemCreated(ctx, item)
	})
	if err != nil {
		return nil, err
	}
	return item, nil
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
	err = s.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.repo.Update(ctx, item); err != nil {
			return apperror.Internal("update item", err)
		}
		return s.publisher.PublishItemUpdated(ctx, item)
	})
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (s *itemService) DeleteItem(ctx context.Context, id string) error {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return apperror.NotFound("item not found", err)
	}
	return s.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.repo.Delete(ctx, id); err != nil {
			return apperror.Internal("delete item", err)
		}
		return s.publisher.PublishItemDeleted(ctx, id)
	})
}

func (s *itemService) HandleOrderCreated(_ context.Context, evt *events.Event[events.OrderCreatedData]) error {
	s.log.Info().
		Str("order_id", evt.Data.ID).
		Int("items", len(evt.Data.Items)).
		Msg("order received, recording item references")
	return nil
}

func (s *itemService) HandleGetItemByID(ctx context.Context, req *events.GetItemRequest) (*events.GetItemResponse, error) {
	item, err := s.repo.GetByID(ctx, req.ID)
	if err != nil {
		return nil, apperror.NotFound("item not found", err)
	}
	return &events.GetItemResponse{
		ID: item.ID, Name: item.Name, Description: item.Description,
		Status: string(item.Status), Tags: item.Tags,
	}, nil
}

func (s *itemService) HandleExternalCreate(_ context.Context, evt *events.Event[events.ItemCreatedData]) error {
	s.log.Info().
		Str("item_id", evt.Data.ID).
		Str("name", evt.Data.Name).
		Msg("external item.create received")
	return nil
}
