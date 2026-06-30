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

// --- Tests ---

func newSvc() (ItemService, *mockRepo) {
	repo := &mockRepo{items: map[string]*domain.Item{}}
	return NewItemService(repo, mockPublisher{}, zerolog.Nop()), repo
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
