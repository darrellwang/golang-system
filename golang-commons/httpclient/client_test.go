package httpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNew_DefaultTimeout(t *testing.T) {
	c := New("http://example.com")
	if c.http.Timeout != 30*time.Second {
		t.Errorf("expected default timeout 30s, got %v", c.http.Timeout)
	}
}

func TestNew_WithCustomTimeout(t *testing.T) {
	c := New("http://example.com", WithTimeout(5*time.Second))
	if c.http.Timeout != 5*time.Second {
		t.Errorf("expected timeout 5s, got %v", c.http.Timeout)
	}
}

func TestNew_WithHeader(t *testing.T) {
	c := New("http://example.com", WithHeader("X-Tenant", "acme"))
	if c.headers["X-Tenant"] != "acme" {
		t.Errorf("expected header X-Tenant=acme, got %s", c.headers["X-Tenant"])
	}
}

func TestGet_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"id": "1", "name": "test"})
	}))
	defer server.Close()

	c := New(server.URL)
	var result map[string]string
	err := c.Get(context.Background(), "/items/1", &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["id"] != "1" {
		t.Errorf("expected id '1', got '%s'", result["id"])
	}
}

func TestGet_ServiceError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	}))
	defer server.Close()

	c := New(server.URL)
	var result map[string]string
	err := c.Get(context.Background(), "/items/999", &result)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !IsNotFound(err) {
		t.Error("expected IsNotFound to return true")
	}
}

func TestPost_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": "new-1"})
	}))
	defer server.Close()

	c := New(server.URL)
	input := map[string]string{"name": "item"}
	var result map[string]string
	err := c.Post(context.Background(), "/items", input, &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["id"] != "new-1" {
		t.Errorf("expected id 'new-1', got '%s'", result["id"])
	}
}

func TestDelete_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := New(server.URL)
	err := c.Delete(context.Background(), "/items/1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIsNotFound_True(t *testing.T) {
	err := &ServiceError{StatusCode: http.StatusNotFound, Body: "not found"}
	if !IsNotFound(err) {
		t.Error("expected IsNotFound to return true")
	}
}

func TestIsNotFound_False(t *testing.T) {
	err := &ServiceError{StatusCode: http.StatusBadRequest, Body: "bad request"}
	if IsNotFound(err) {
		t.Error("expected IsNotFound to return false")
	}
}

func TestIsNotFound_NonServiceError(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", context.DeadlineExceeded)
	if IsNotFound(err) {
		t.Error("expected IsNotFound to return false for non-ServiceError")
	}
}
