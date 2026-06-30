package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/darrell/golang-system/golang-app2/internal/domain"
	"github.com/darrell/golang-system/golang-events"
)

type mockOrderRepo struct {
	mu    sync.Mutex
	items map[string]*domain.Order
}

func (m *mockOrderRepo) Create(_ context.Context, o *domain.Order) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[o.ID] = o
	return nil
}
func (m *mockOrderRepo) GetByID(_ context.Context, id string) (*domain.Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.items[id]; ok {
		return v, nil
	}
	return nil, errNF{id: id}
}
func (m *mockOrderRepo) List(_ context.Context, _, _ int, _ string) ([]*domain.Order, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*domain.Order, 0, len(m.items))
	for _, v := range m.items {
		out = append(out, v)
	}
	return out, len(out), nil
}
func (m *mockOrderRepo) Update(_ context.Context, o *domain.Order) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[o.ID] = o
	return nil
}

type errNF struct{ id string }

func (e errNF) Error() string { return "not found: " + e.id }

type mockPub struct{}

func (mockPub) PublishOrderCreated(_ context.Context, _ *domain.Order) error    { return nil }
func (mockPub) PublishOrderUpdated(_ context.Context, _ *domain.Order) error    { return nil }
func (mockPub) PublishOrderCanceled(_ context.Context, _ *domain.Order, _ string) error { return nil }

type mockItemClient struct{}

func (mockItemClient) GetByID(_ context.Context, id string) (*domain.ItemView, error) {
	return &domain.ItemView{ID: id, Name: "n", Status: "active"}, nil
}

func newSvc() (OrderService, *mockOrderRepo) {
	repo := &mockOrderRepo{items: map[string]*domain.Order{}}
	return NewOrderService(repo, mockPub{}, mockItemClient{}, zerolog.Nop()), repo
}

func TestCreateOrder(t *testing.T) {
	svc, _ := newSvc()
	order, err := svc.CreateOrder(context.Background(), &domain.CreateOrderRequest{
		Note: "n", Items: []domain.CreateOrderItemReq{{ItemID: "i1", Quantity: 1, Price: 10}},
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if order.Status != domain.OrderStatusPending || order.Total != 10 {
		t.Fatalf("unexpected: %+v", order)
	}
}

func TestHandleItemDeleted_AutoCancels(t *testing.T) {
	svc, repo := newSvc()
	order, _ := svc.CreateOrder(context.Background(), &domain.CreateOrderRequest{
		Items: []domain.CreateOrderItemReq{{ItemID: "to-delete", Quantity: 1, Price: 5}},
	})
	if _, err := repo.GetByID(context.Background(), order.ID); err != nil {
		t.Fatal(err)
	}
	evt := &events.Event[events.ItemDeletedData]{Data: events.ItemDeletedData{ID: "to-delete"}}
	if err := svc.HandleItemDeleted(context.Background(), evt); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	canceled, _ := repo.GetByID(context.Background(), order.ID)
	if canceled.Status != domain.OrderStatusCanceled {
		t.Fatalf("expected canceled, got %s", canceled.Status)
	}
}

// Ensure uuid is referenced to avoid unused-import warning
var _ = uuid.New
var _ = time.Now
