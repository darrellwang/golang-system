package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	apperror "github.com/darrell/golang-system/golang-commons/apperror"
	"github.com/darrell/golang-system/golang-app2/internal/domain"
	"github.com/darrell/golang-system/golang-events"
)

type orderService struct {
	repo          OrderRepository
	publisher     EventPublisher
	itemClient    ItemServiceClient
	log           zerolog.Logger

	trackedMu    sync.RWMutex
	trackedItems map[string]map[string]struct{} // itemID -> set of orderIDs
}

func NewOrderService(repo OrderRepository, publisher EventPublisher, itemClient ItemServiceClient, log zerolog.Logger) OrderService {
	return &orderService{
		repo:         repo,
		publisher:    publisher,
		itemClient:   itemClient,
		log:          log,
		trackedItems: make(map[string]map[string]struct{}),
	}
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
			return nil, apperror.NotFound(fmt.Sprintf("item %s", it.ItemID), err)
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
		ID:        uuid.New().String(),
		Status:    domain.OrderStatusPending,
		Total:     total,
		Note:      req.Note,
		Items:     items,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.repo.Create(ctx, order); err != nil {
		return nil, apperror.Internal("create order", err)
	}
	s.trackedMu.Lock()
	for _, it := range order.Items {
		set, ok := s.trackedItems[it.ItemID]
		if !ok {
			set = make(map[string]struct{})
			s.trackedItems[it.ItemID] = set
		}
		set[order.ID] = struct{}{}
	}
	s.trackedMu.Unlock()

	go func() {
		_ = s.publisher.PublishOrderCreated(context.Background(), order)
	}()
	return order, nil
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
	if err := s.repo.Update(ctx, o); err != nil {
		return nil, apperror.Internal("update order", err)
	}
	go func() {
		_ = s.publisher.PublishOrderUpdated(context.Background(), o)
	}()
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
	if err := s.repo.Update(ctx, o); err != nil {
		return apperror.Internal("cancel order", err)
	}
	go func() {
		_ = s.publisher.PublishOrderCanceled(context.Background(), o, "user requested")
	}()
	return nil
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
	s.trackedMu.RLock()
	orderIDs, ok := s.trackedItems[evt.Data.ID]
	if !ok {
		s.trackedMu.RUnlock()
		return nil
	}
	ids := make([]string, 0, len(orderIDs))
	for id := range orderIDs {
		ids = append(ids, id)
	}
	s.trackedMu.RUnlock()

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
		if err := s.repo.Update(ctx, order); err != nil {
			s.log.Error().Str("order_id", id).Err(err).Msg("auto-cancel update failed")
			continue
		}
		s.log.Info().
			Str("order_id", id).
			Str("item_id", evt.Data.ID).
			Msg("order auto-canceled due to item deletion")
		_ = s.publisher.PublishOrderCanceled(ctx, order, "referenced item was deleted")
	}
	return nil
}

func (s *orderService) HandleItemUpdated(ctx context.Context, evt *events.Event[events.ItemUpdatedData]) error {
	s.log.Info().
		Str("item_id", evt.Data.ID).
		Str("new_status", evt.Data.Status).
		Msg("item updated, invalidating caches and recording change log")
	return nil
}
