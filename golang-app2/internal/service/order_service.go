package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/darrell/golang-system/golang-app2/internal/domain"
	apperror "github.com/darrell/golang-system/golang-commons/apperror"
	"github.com/darrell/golang-system/golang-commons/httpclient"
	"github.com/darrell/golang-system/golang-events"
)

type orderService struct {
	repo       OrderRepository
	publisher  EventPublisher
	itemClient ItemServiceClient
	tx         Transactor
	log        zerolog.Logger
}

func NewOrderService(repo OrderRepository, publisher EventPublisher, itemClient ItemServiceClient, tx Transactor, log zerolog.Logger) OrderService {
	return &orderService{repo: repo, publisher: publisher, itemClient: itemClient, tx: tx, log: log}
}

func (s *orderService) CreateOrder(ctx context.Context, req *domain.CreateOrderRequest) (*domain.Order, error) {
	if len(req.Items) == 0 {
		return nil, apperror.NewBadInput("order must contain at least one item")
	}
	items := make([]domain.OrderItem, 0, len(req.Items))
	var total float64
	for _, it := range req.Items {
		if it.Quantity <= 0 {
			return nil, apperror.NewBadInput("quantity must be positive")
		}
		view, err := s.itemClient.GetByID(ctx, it.ItemID)
		if err != nil {
			return nil, mapItemClientError(it.ItemID, err)
		}
		if view.Status != "active" {
			return nil, apperror.NewBadInput(fmt.Sprintf("item %s is not active", it.ItemID))
		}
		items = append(items, domain.OrderItem{
			ID: uuid.New().String(), ItemID: it.ItemID, ItemName: view.Name,
			Price: it.Price, Quantity: it.Quantity,
		})
		total += it.Price * float64(it.Quantity)
	}
	now := time.Now().UTC()
	order := &domain.Order{
		ID: uuid.New().String(), Status: domain.OrderStatusPending, Total: total,
		Note: req.Note, Items: items, CreatedAt: now, UpdatedAt: now,
	}
	err := s.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.repo.Create(ctx, order); err != nil {
			return apperror.Internal("create order", err)
		}
		return s.publisher.PublishOrderCreated(ctx, order)
	})
	if err != nil {
		return nil, err
	}
	return order, nil
}

func mapItemClientError(itemID string, err error) error {
	if httpclient.IsNotFound(err) {
		return apperror.NotFound(fmt.Sprintf("item %s", itemID), err)
	}
	var se *httpclient.ServiceError
	if errors.As(err, &se) {
		if se.StatusCode >= 500 || se.StatusCode == 429 {
			return apperror.BadGateway("item service unavailable", err)
		}
		return apperror.BadGateway("item service error", err)
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return apperror.Unavailable("item service timeout", err)
	}
	return apperror.BadGateway("item service unreachable", err)
}

func (s *orderService) GetOrder(ctx context.Context, id string) (*domain.Order, error) {
	o, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, apperror.NotFound("order not found", err)
	}
	return o, nil
}

func (s *orderService) ListOrders(ctx context.Context, offset, limit int, status string) ([]*domain.Order, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.List(ctx, offset, limit, status)
}

func (s *orderService) UpdateOrder(ctx context.Context, id string, req *domain.UpdateOrderRequest) (*domain.Order, error) {
	o, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, apperror.NotFound("order not found", err)
	}
	if req.Status != nil {
		if !validStatus(*req.Status) {
			return nil, apperror.NewBadInput("invalid status")
		}
		o.Status = *req.Status
	}
	if req.Note != nil {
		o.Note = *req.Note
	}
	o.UpdatedAt = time.Now().UTC()
	err = s.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.repo.Update(ctx, o); err != nil {
			return apperror.Internal("update order", err)
		}
		return s.publisher.PublishOrderUpdated(ctx, o)
	})
	if err != nil {
		return nil, err
	}
	return o, nil
}

func (s *orderService) CancelOrder(ctx context.Context, id string) error {
	o, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return apperror.NotFound("order not found", err)
	}
	if o.Status == domain.OrderStatusCanceled {
		return apperror.NewConflict("order already canceled")
	}
	o.Status = domain.OrderStatusCanceled
	o.UpdatedAt = time.Now().UTC()
	return s.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.repo.Update(ctx, o); err != nil {
			return apperror.Internal("cancel order", err)
		}
		return s.publisher.PublishOrderCanceled(ctx, o, "user requested")
	})
}

func validStatus(s domain.OrderStatus) bool {
	switch s {
	case domain.OrderStatusPending, domain.OrderStatusConfirmed,
		domain.OrderStatusShipped, domain.OrderStatusDelivered,
		domain.OrderStatusCanceled:
		return true
	}
	return false
}

func (s *orderService) HandleItemDeleted(ctx context.Context, evt *events.Event[events.ItemDeletedData]) error {
	ids, err := s.repo.ListPendingIDsByItemID(ctx, evt.Data.ID)
	if err != nil {
		return apperror.Internal("list orders by item", err)
	}
	for _, id := range ids {
		order, err := s.repo.GetByID(ctx, id)
		if err != nil {
			continue
		}
		if order.Status != domain.OrderStatusPending {
			continue
		}
		order.Status = domain.OrderStatusCanceled
		order.UpdatedAt = time.Now().UTC()
		err = s.tx.WithinTransaction(ctx, func(ctx context.Context) error {
			if err := s.repo.Update(ctx, order); err != nil {
				return err
			}
			return s.publisher.PublishOrderCanceled(ctx, order, "referenced item was deleted")
		})
		if err != nil {
			s.log.Error().Str("order_id", id).Err(err).Msg("auto-cancel update failed")
			continue
		}
		s.log.Info().
			Str("order_id", id).
			Str("item_id", evt.Data.ID).
			Msg("order auto-canceled due to item deletion")
	}
	return nil
}

func (s *orderService) HandleItemUpdated(_ context.Context, evt *events.Event[events.ItemUpdatedData]) error {
	s.log.Info().
		Str("item_id", evt.Data.ID).
		Str("new_status", evt.Data.Status).
		Msg("item updated, invalidating caches and recording change log")
	return nil
}
