package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/darrell/golang-system/golang-app/internal/domain"
	"github.com/darrell/golang-system/golang-events"
)

// --- Mocks ---

type mockRepo struct {
	items map[string]*domain.Item
}

func (m *mockRepo) Create(_ context.Context, item *domain.Item) error {
	m.items[item.ID] = item
	return nil
}
func (m *mockRepo) GetByID(_ context.Context, id string) (*domain.Item, error) {
	if v, ok := m.items[id]; ok {
		return v, nil
	}
	return nil, errNotFound{id: id}
}
func (m *mockRepo) List(_ context.Context, offset, limit int, _ string) ([]*domain.Item, int, error) {
	out := make([]*domain.Item, 0, len(m.items))
	for _, v := range m.items {
		out = append(out, v)
	}
	return out, len(out), nil
}
func (m *mockRepo) Update(_ context.Context, item *domain.Item) error {
	m.items[item.ID] = item
	return nil
}
func (m *mockRepo) Delete(_ context.Context, id string) error {
	delete(m.items, id)
	return nil
}

type errNotFound struct{ id string }

func (e errNotFound) Error() string { return "not found: " + e.id }

type mockPublisher struct{}

func (mockPublisher) PublishItemCreated(_ context.Context, _ *domain.Item) error { return nil }
func (mockPublisher) PublishItemUpdated(_ context.Context, _ *domain.Item) error { return nil }
func (mockPublisher) PublishItemDeleted(_ context.Context, _ string) error       { return nil }

type noopTx struct{}

func (noopTx) WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// --- Tests ---

func newSvc() (ItemService, *mockRepo) {
	repo := &mockRepo{items: map[string]*domain.Item{}}
	return NewItemService(repo, mockPublisher{}, noopTx{}, zerolog.Nop()), repo
}

func TestCreateItem_Success(t *testing.T) {
	svc, _ := newSvc()
	item, err := svc.CreateItem(context.Background(), &domain.CreateItemRequest{
		Name: "widget", Description: "desc", Tags: []string{"a"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.ID == "" || item.Status != domain.ItemStatusActive {
		t.Fatalf("unexpected item: %+v", item)
	}
}

func TestGetItem_NotFound(t *testing.T) {
	svc, _ := newSvc()
	_, err := svc.GetItem(context.Background(), uuid.New().String())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestUpdateItem_PartialFields(t *testing.T) {
	svc, repo := newSvc()
	original := &domain.Item{ID: uuid.New().String(), Name: "n", Status: domain.ItemStatusActive, CreatedAt: time.Now()}
	repo.items[original.ID] = original

	newName := "updated"
	item, err := svc.UpdateItem(context.Background(), original.ID, &domain.UpdateItemRequest{Name: &newName})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Name != "updated" || item.Status != domain.ItemStatusActive {
		t.Fatalf("unexpected update: %+v", item)
	}
}

func TestHandleGetItemByID(t *testing.T) {
	svc, repo := newSvc()
	repo.items["x"] = &domain.Item{ID: "x", Name: "n", Description: "d", Status: domain.ItemStatusActive}

	resp, err := svc.HandleGetItemByID(context.Background(), &events.GetItemRequest{ID: "x"})
	if err != nil || resp == nil || resp.Name != "n" {
		t.Fatalf("unexpected: %+v %v", resp, err)
	}
}

func TestCreateItem_EmptyName(t *testing.T) {
	svc, _ := newSvc()
	_, err := svc.CreateItem(context.Background(), &domain.CreateItemRequest{
		Name: "", Description: "desc", Tags: []string{},
	})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestDeleteItem_Success(t *testing.T) {
	svc, repo := newSvc()
	item := &domain.Item{ID: "to-delete", Name: "n", Status: domain.ItemStatusActive, CreatedAt: time.Now()}
	repo.items[item.ID] = item

	err := svc.DeleteItem(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, exists := repo.items["to-delete"]; exists {
		t.Fatal("expected item to be deleted from repo")
	}
}

func TestDeleteItem_NotFound(t *testing.T) {
	svc, _ := newSvc()
	err := svc.DeleteItem(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for deleting nonexistent item")
	}
}

func TestListItems_DefaultLimit(t *testing.T) {
	svc, repo := newSvc()
	// Add 5 items
	for i := 0; i < 5; i++ {
		repo.items[string(rune('a'+i))] = &domain.Item{ID: string(rune('a' + i)), Name: "item", Status: domain.ItemStatusActive, CreatedAt: time.Now()}
	}

	items, total, err := svc.ListItems(context.Background(), 0, 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(items) != 5 {
		t.Errorf("expected 5 items returned (default limit), got %d", len(items))
	}
}

func TestListItems_NegativeOffset(t *testing.T) {
	svc, _ := newSvc()
	_, _, err := svc.ListItems(context.Background(), -1, 10, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListItems_ExcessiveLimit(t *testing.T) {
	svc, repo := newSvc()
	repo.items["x"] = &domain.Item{ID: "x", Name: "n", Status: domain.ItemStatusActive, CreatedAt: time.Now()}

	_, total, err := svc.ListItems(context.Background(), 0, 200, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total 1, got %d", total)
	}
}

func TestUpdateItem_StatusChange(t *testing.T) {
	svc, repo := newSvc()
	item := &domain.Item{ID: "x", Name: "n", Status: domain.ItemStatusActive, CreatedAt: time.Now()}
	repo.items["x"] = item

	inactive := domain.ItemStatusInactive
	updated, err := svc.UpdateItem(context.Background(), "x", &domain.UpdateItemRequest{Status: &inactive})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Status != domain.ItemStatusInactive {
		t.Errorf("expected status inactive, got %s", updated.Status)
	}
}

func TestHandleExternalCreate(t *testing.T) {
	svc, _ := newSvc()
	evt := &events.Event[events.ItemCreatedData]{
		Data: events.ItemCreatedData{ID: "ext-1", Name: "external-item"},
	}
	err := svc.HandleExternalCreate(context.Background(), evt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleOrderCreated(t *testing.T) {
	svc, _ := newSvc()
	evt := &events.Event[events.OrderCreatedData]{
		Data: events.OrderCreatedData{ID: "order-1", Items: []events.OrderItem{{ItemID: "item-1"}}},
	}
	err := svc.HandleOrderCreated(context.Background(), evt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetItem_NotFound_Error(t *testing.T) {
	svc, _ := newSvc()
	_, err := svc.GetItem(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent item")
	}
}
