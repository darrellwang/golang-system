// Package domain defines the item entity, DTOs, and pagination.
package domain

import "time"

// ItemStatus represents the lifecycle status of an item.
type ItemStatus string

const (
	ItemStatusActive   ItemStatus = "active"
	ItemStatusInactive ItemStatus = "inactive"
	ItemStatusArchived ItemStatus = "archived"
)

// Item is the aggregate root.
type Item struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Status      ItemStatus `json:"status"`
	Tags        []string   `json:"tags,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// CreateItemRequest is the DTO for creating an item.
type CreateItemRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
}

// UpdateItemRequest is the DTO for partial item updates.
type UpdateItemRequest struct {
	Name        *string     `json:"name,omitempty"`
	Description *string     `json:"description,omitempty"`
	Status      *ItemStatus `json:"status,omitempty"`
	Tags        *[]string   `json:"tags,omitempty"`
}

// ItemView is the projection returned by cross-service lookups.
type ItemView struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Status      ItemStatus `json:"status"`
	Tags        []string   `json:"tags,omitempty"`
}

// Page contains a paginated list and total.
type Page struct {
	Items  any `json:"items"`
	Total  int `json:"total"`
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}
