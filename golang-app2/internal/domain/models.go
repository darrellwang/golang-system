package domain

import "time"

// OrderStatus is the lifecycle state of an order.
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusConfirmed OrderStatus = "confirmed"
	OrderStatusShipped   OrderStatus = "shipped"
	OrderStatusDelivered OrderStatus = "delivered"
	OrderStatusCanceled  OrderStatus = "canceled"
)

// Order is the aggregate root.
type Order struct {
	ID        string      `json:"id"`
	Status    OrderStatus `json:"status"`
	Total     float64     `json:"total"`
	Note      string      `json:"note"`
	Items     []OrderItem `json:"items"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// OrderItem is one line item in an order.
type OrderItem struct {
	ID       string  `json:"id"`
	ItemID   string  `json:"item_id"`
	ItemName string  `json:"item_name"`
	Price    float64 `json:"price"`
	Quantity int     `json:"quantity"`
}

// CreateOrderRequest is the DTO for creating an order.
type CreateOrderRequest struct {
	Note  string               `json:"note"`
	Items []CreateOrderItemReq `json:"items"`
}

// CreateOrderItemReq is the DTO for one line item at creation.
type CreateOrderItemReq struct {
	ItemID   string  `json:"item_id"`
	Quantity int     `json:"quantity"`
	Price    float64 `json:"price"`
}

// UpdateOrderRequest is the DTO for partial order updates.
type UpdateOrderRequest struct {
	Status *OrderStatus `json:"status,omitempty"`
	Note   *string      `json:"note,omitempty"`
}

// ItemView is the projection of an item used by the order service.
type ItemView struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Tags        []string `json:"tags,omitempty"`
}
