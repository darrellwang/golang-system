// Package client implements inter-service HTTP clients.
package client

import (
	"context"

	"github.com/darrell/golang-system/golang-commons/httpclient"
	"github.com/darrell/golang-system/golang-app2/internal/domain"
)

// ItemClient looks up items in the item service over HTTP.
type ItemClient struct {
	c *httpclient.Client
}

// NewItemClient creates a new ItemClient.
func NewItemClient(baseURL string) *ItemClient {
	return &ItemClient{c: httpclient.New(baseURL, httpclient.WithTimeout(10*1_000_000_000))}
}

// GetByID fetches an item by ID.
func (c *ItemClient) GetByID(ctx context.Context, id string) (*domain.ItemView, error) {
	var item domain.ItemView
	if err := c.c.Get(ctx, "/rest/v1/items/"+id, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

// List fetches a paginated item list.
func (c *ItemClient) List(ctx context.Context, offset, limit int) ([]domain.ItemView, error) {
	var page struct {
		Items  []domain.ItemView `json:"items"`
		Total  int               `json:"total"`
		Offset int               `json:"offset"`
		Limit  int               `json:"limit"`
	}
	if err := c.c.Get(ctx, "/rest/v1/items", &page); err != nil {
		return nil, err
	}
	return page.Items, nil
}
