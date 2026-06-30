// Package golang-events defines event contracts shared between services.
package events

import "time"

// ============================================================
// NATS Subject constants
// ============================================================

const (
	// Item events
	SubjectItemCreated = "item.created"
	SubjectItemUpdated = "item.updated"
	SubjectItemDeleted = "item.deleted"
	SubjectItemExternalCreate = "item.external.create"

	// Item request-reply
	SubjectItemGetByID = "item.get_by_id"

	// Order events
	SubjectOrderCreated  = "order.created"
	SubjectOrderUpdated  = "order.updated"
	SubjectOrderCanceled = "order.canceled"
)

// ============================================================
// Generic event envelope
// ============================================================

// Event is a generic envelope for all domain events.
type Event[T any] struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Source    string    `json:"source"`
	Timestamp time.Time `json:"timestamp"`
	Data      T         `json:"data"`
}

// ============================================================
// Item event payloads
// ============================================================

// ItemCreatedData is the payload for item.created.
type ItemCreatedData struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Tags        []string `json:"tags,omitempty"`
}

// ItemUpdatedData is the payload for item.updated.
type ItemUpdatedData struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Tags        []string `json:"tags,omitempty"`
}

// ItemDeletedData is the payload for item.deleted.
type ItemDeletedData struct {
	ID string `json:"id"`
}

// ============================================================
// Order event payloads
// ============================================================

// OrderItem is one line item in an order event payload.
type OrderItem struct {
	ItemID   string  `json:"item_id"`
	ItemName string  `json:"item_name"`
	Price    float64 `json:"price"`
	Quantity int     `json:"quantity"`
}

// OrderCreatedData is the payload for order.created.
type OrderCreatedData struct {
	ID     string      `json:"id"`
	Items  []OrderItem `json:"items"`
	Total  float64     `json:"total"`
	Status string      `json:"status"`
}

// OrderUpdatedData is the payload for order.updated.
type OrderUpdatedData struct {
	ID     string      `json:"id"`
	Items  []OrderItem `json:"items"`
	Total  float64     `json:"total"`
	Status string      `json:"status"`
}

// OrderCanceledData is the payload for order.canceled.
type OrderCanceledData struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// ============================================================
// Item request-reply payloads
// ============================================================

// GetItemRequest is the request body for item.get_by_id.
type GetItemRequest struct {
	ID string `json:"id"`
}

// GetItemResponse is the response body for item.get_by_id.
type GetItemResponse struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Tags        []string `json:"tags,omitempty"`
}
