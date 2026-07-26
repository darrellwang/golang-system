package service

import (
	"context"
	"sync"
	"testing"

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

func (m *mockOrderRepo) ListPendingIDsByItemID(_ context.Context, itemID string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var ids []string
	for _, o := range m.items {
		if o.Status != domain.OrderStatusPending {
			continue
		}
		for _, it := range o.Items {
			if it.ItemID == itemID {
				ids = append(ids, o.ID)
				break
			}
		}
	}
	return ids, nil
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

type noopTx struct{}

func (noopTx) WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func newSvc() (OrderService, *mockOrderRepo) {
	repo := &mockOrderRepo{items: map[string]*domain.Order{}}
	return NewOrderService(repo, mockPub{}, mockItemClient{}, noopTx{}, zerolog.Nop()), repo
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

func TestCreateOrder_EmptyItems(t *testing.T) {
	svc, _ := newSvc()
	_, err := svc.CreateOrder(context.Background(), &domain.CreateOrderRequest{
		Note: "n", Items: []domain.CreateOrderItemReq{},
	})
	if err == nil {
		t.Fatal("expected error for empty items")
	}
}

func TestCreateOrder_ZeroQuantity(t *testing.T) {
	svc, _ := newSvc()
	_, err := svc.CreateOrder(context.Background(), &domain.CreateOrderRequest{
		Note:  "n",
		Items: []domain.CreateOrderItemReq{{ItemID: "i1", Quantity: 0, Price: 10}},
	})
	if err == nil {
		t.Fatal("expected error for zero quantity")
	}
}

func TestCreateOrder_MultipleItems(t *testing.T) {
	svc, _ := newSvc()
	order, err := svc.CreateOrder(context.Background(), &domain.CreateOrderRequest{
		Note: "multi",
		Items: []domain.CreateOrderItemReq{
			{ItemID: "i1", Quantity: 2, Price: 5},
			{ItemID: "i2", Quantity: 3, Price: 4},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if order.Total != 22 { // 2*5 + 3*4 = 22
		t.Errorf("expected total 22, got %f", order.Total)
	}
	if len(order.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(order.Items))
	}
}

func TestGetOrder_Success(t *testing.T) {
	svc, _ := newSvc()
	order, _ := svc.CreateOrder(context.Background(), &domain.CreateOrderRequest{
		Note: "get-test", Items: []domain.CreateOrderItemReq{{ItemID: "i1", Quantity: 1, Price: 10}},
	})
	retrieved, err := svc.GetOrder(context.Background(), order.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if retrieved.ID != order.ID {
		t.Errorf("expected id %s, got %s", order.ID, retrieved.ID)
	}
}

func TestGetOrder_NotFound(t *testing.T) {
	svc, _ := newSvc()
	_, err := svc.GetOrder(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent order")
	}
}

func TestListOrders_Empty(t *testing.T) {
	svc, _ := newSvc()
	items, total, err := svc.ListOrders(context.Background(), 0, 10, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 0 {
		t.Errorf("expected total 0, got %d", total)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestListOrders_DefaultLimit(t *testing.T) {
	svc, repo := newSvc()
	for i := 0; i < 5; i++ {
		order, _ := svc.CreateOrder(context.Background(), &domain.CreateOrderRequest{
			Note: "list-test", Items: []domain.CreateOrderItemReq{{ItemID: "i1", Quantity: 1, Price: 1}},
		})
		repo.items[order.ID] = order // mock repo already has it via CreateOrder, but ensure consistency
	}
	items, total, err := svc.ListOrders(context.Background(), 0, 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != total {
		t.Errorf("expected items length to match total, got %d vs %d", len(items), total)
	}
}

func TestListOrders_NegativeOffset(t *testing.T) {
	svc, _ := newSvc()
	_, _, err := svc.ListOrders(context.Background(), -1, 10, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateOrder_StatusToConfirmed(t *testing.T) {
	svc, repo := newSvc()
	order, _ := svc.CreateOrder(context.Background(), &domain.CreateOrderRequest{
		Note: "update-test", Items: []domain.CreateOrderItemReq{{ItemID: "i1", Quantity: 1, Price: 10}},
	})
	if _, err := repo.GetByID(context.Background(), order.ID); err != nil {
		t.Fatal(err)
	}

	confirmed := domain.OrderStatusConfirmed
	updated, err := svc.UpdateOrder(context.Background(), order.ID, &domain.UpdateOrderRequest{Status: &confirmed})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Status != domain.OrderStatusConfirmed {
		t.Errorf("expected status confirmed, got %s", updated.Status)
	}

	// Verify persisted
	persisted, _ := repo.GetByID(context.Background(), order.ID)
	if persisted.Status != domain.OrderStatusConfirmed {
		t.Errorf("expected persisted status confirmed, got %s", persisted.Status)
	}
}

func TestUpdateOrder_NoteChange(t *testing.T) {
	svc, _ := newSvc()
	order, _ := svc.CreateOrder(context.Background(), &domain.CreateOrderRequest{
		Note: "original-note", Items: []domain.CreateOrderItemReq{{ItemID: "i1", Quantity: 1, Price: 10}},
	})

	newNote := "updated-note"
	updated, err := svc.UpdateOrder(context.Background(), order.ID, &domain.UpdateOrderRequest{Note: &newNote})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Note != "updated-note" {
		t.Errorf("expected note 'updated-note', got '%s'", updated.Note)
	}
}

func TestUpdateOrder_InvalidStatus(t *testing.T) {
	svc, _ := newSvc()
	order, _ := svc.CreateOrder(context.Background(), &domain.CreateOrderRequest{
		Note: "test", Items: []domain.CreateOrderItemReq{{ItemID: "i1", Quantity: 1, Price: 10}},
	})

	invalidStatus := domain.OrderStatus("invalid-status")
	_, err := svc.UpdateOrder(context.Background(), order.ID, &domain.UpdateOrderRequest{Status: &invalidStatus})
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestUpdateOrder_NotFound(t *testing.T) {
	svc, _ := newSvc()
	updatedStatus := domain.OrderStatusConfirmed
	_, err := svc.UpdateOrder(context.Background(), "nonexistent", &domain.UpdateOrderRequest{Status: &updatedStatus})
	if err == nil {
		t.Fatal("expected error for nonexistent order")
	}
}

func TestCancelOrder_Success(t *testing.T) {
	svc, repo := newSvc()
	order, _ := svc.CreateOrder(context.Background(), &domain.CreateOrderRequest{
		Note: "cancel-test", Items: []domain.CreateOrderItemReq{{ItemID: "i1", Quantity: 1, Price: 10}},
	})

	err := svc.CancelOrder(context.Background(), order.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	canceled, _ := repo.GetByID(context.Background(), order.ID)
	if canceled.Status != domain.OrderStatusCanceled {
		t.Errorf("expected status canceled, got %s", canceled.Status)
	}
}

func TestCancelOrder_AlreadyCanceled(t *testing.T) {
	svc, _ := newSvc()
	order, _ := svc.CreateOrder(context.Background(), &domain.CreateOrderRequest{
		Note: "cancel-test", Items: []domain.CreateOrderItemReq{{ItemID: "i1", Quantity: 1, Price: 10}},
	})

	_ = svc.CancelOrder(context.Background(), order.ID)
	err := svc.CancelOrder(context.Background(), order.ID)
	if err == nil {
		t.Fatal("expected error for canceling already canceled order")
	}
}

func TestCancelOrder_NotFound(t *testing.T) {
	svc, _ := newSvc()
	err := svc.CancelOrder(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent order")
	}
}

func TestHandleItemUpdated(t *testing.T) {
	svc, _ := newSvc()
	evt := &events.Event[events.ItemUpdatedData]{
		Data: events.ItemUpdatedData{ID: "item-1", Status: "inactive"},
	}
	err := svc.HandleItemUpdated(context.Background(), evt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleItemDeleted_NoTracking(t *testing.T) {
	svc, _ := newSvc()
	evt := &events.Event[events.ItemDeletedData]{Data: events.ItemDeletedData{ID: "untracked-item"}}
	err := svc.HandleItemDeleted(context.Background(), evt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleItemDeleted_OrderNotPending(t *testing.T) {
	svc, repo := newSvc()
	order, _ := svc.CreateOrder(context.Background(), &domain.CreateOrderRequest{
		Items: []domain.CreateOrderItemReq{{ItemID: "tracked-item", Quantity: 1, Price: 5}},
	})
	// Confirm the order so it won't be auto-cancelled
	confirmed := domain.OrderStatusConfirmed
	svc.UpdateOrder(context.Background(), order.ID, &domain.UpdateOrderRequest{Status: &confirmed})

	evt := &events.Event[events.ItemDeletedData]{Data: events.ItemDeletedData{ID: "tracked-item"}}
	err := svc.HandleItemDeleted(context.Background(), evt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stillConfirmed, _ := repo.GetByID(context.Background(), order.ID)
	if stillConfirmed.Status != domain.OrderStatusConfirmed {
		t.Errorf("expected status to remain confirmed (not auto-cancelled), got %s", stillConfirmed.Status)
	}
}
