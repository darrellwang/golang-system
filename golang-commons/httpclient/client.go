// Package httpclient provides an inter-service HTTP client with automatic
// request header propagation, structured errors, and configurable timeouts.
package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/darrell/golang-system/golang-commons/httputil"
)

// Standard request headers propagated across services.
const (
	HeaderRequestID      = "X-Request-ID"
	HeaderOrganizationID = "X-Organization-ID"
	HeaderUserID         = "X-User-ID"
	HeaderTraceParent    = "traceparent"
)

// ServiceError is returned for non-2xx responses.
type ServiceError struct {
	StatusCode int
	Body       string
}

func (e *ServiceError) Error() string {
	return fmt.Sprintf("upstream service returned status %d: %s", e.StatusCode, e.Body)
}

// IsNotFound reports whether the error is a 404.
func IsNotFound(err error) bool {
	se, ok := err.(*ServiceError)
	return ok && se.StatusCode == http.StatusNotFound
}

// IsRetriable reports whether the error is a transient upstream/network failure.
func IsRetriable(err error) bool {
	if se, ok := err.(*ServiceError); ok {
		return se.StatusCode == http.StatusTooManyRequests ||
			se.StatusCode == http.StatusBadGateway ||
			se.StatusCode == http.StatusServiceUnavailable ||
			se.StatusCode == http.StatusGatewayTimeout ||
			se.StatusCode >= 500
	}
	return err != nil
}

// Client is an inter-service HTTP client.
type Client struct {
	baseURL    string
	http       *http.Client
	headers    map[string]string
	maxRetries int
}

// Option configures a Client.
type Option func(*Client)

// WithTimeout sets the request timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.http.Timeout = d }
}

// WithHeader adds a default header sent with every request.
func WithHeader(key, value string) Option {
	return func(c *Client) {
		if c.headers == nil {
			c.headers = make(map[string]string)
		}
		c.headers[key] = value
	}
}

// WithRetries sets max retries for idempotent GET requests (0 = no retry).
func WithRetries(n int) Option {
	return func(c *Client) { c.maxRetries = n }
}

// New creates a new Client targeting baseURL.
func New(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL:    baseURL,
		http:       &http.Client{Timeout: 30 * time.Second},
		headers:    map[string]string{},
		maxRetries: 2,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Get issues a GET request and unmarshals the response into out.
func (c *Client) Get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out)
}

// Post issues a POST request.
func (c *Client) Post(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPost, path, body, out)
}

// Put issues a PUT request.
func (c *Client) Put(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPut, path, body, out)
}

// Delete issues a DELETE request.
func (c *Client) Delete(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodDelete, path, nil, out)
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	attempts := 1
	if method == http.MethodGet && c.maxRetries > 0 {
		attempts = c.maxRetries + 1
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(i) * 100 * time.Millisecond):
			}
		}
		lastErr = c.doOnce(ctx, method, path, body, out)
		if lastErr == nil || !IsRetriable(lastErr) || method != http.MethodGet {
			return lastErr
		}
	}
	return lastErr
}

func (c *Client) doOnce(ctx context.Context, method, path string, body, out any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	propagateHeaders(ctx, req)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &ServiceError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	if out != nil && len(respBody) > 0 {
		var envelope struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(respBody, &envelope); err == nil && len(envelope.Data) > 0 {
			if err := json.Unmarshal(envelope.Data, out); err == nil {
				return nil
			}
		}
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}
	return nil
}

func propagateHeaders(ctx context.Context, req *http.Request) {
	if id := httputil.GetRequestID(ctx); id != "" {
		req.Header.Set(HeaderRequestID, id)
	}
	if id := httputil.GetUserID(ctx); id != "" {
		// Prefer JWT claim header for downstream AuthContextMiddleware.
		req.Header.Set("x-jwt-sub", id)
		req.Header.Set(HeaderUserID, id)
	}
	if id := httputil.GetOrganizationID(ctx); id != "" {
		req.Header.Set("x-jwt-org", id)
		req.Header.Set(HeaderOrganizationID, id)
	}
	if email := httputil.GetUserEmail(ctx); email != "" {
		req.Header.Set("x-jwt-email", email)
	}
	if auth := httputil.GetAuthorization(ctx); auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if tp, ok := ctx.Value(httputil.CtxTraceParent).(string); ok && tp != "" {
		req.Header.Set(HeaderTraceParent, tp)
	}
}
