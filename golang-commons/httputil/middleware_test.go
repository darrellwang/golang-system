package httputil

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	apperror "github.com/darrell/golang-system/golang-commons/apperror"
)

// --- Write / OK / Created / NoContent Tests ---

func TestWrite_SetsContentTypeAndStatus(t *testing.T) {
	w := httptest.NewRecorder()
	Write(w, http.StatusOK, map[string]string{"key": "value"})

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", w.Header().Get("Content-Type"))
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp JSON
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Success {
		t.Error("expected Success to be true")
	}
}

func TestWrite_ErrorStatus(t *testing.T) {
	w := httptest.NewRecorder()
	Write(w, http.StatusBadRequest, nil)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
	var resp JSON
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Success {
		t.Error("expected Success to be false for error status")
	}
}

func TestOK(t *testing.T) {
	w := httptest.NewRecorder()
	OK(w, "data")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestCreated(t *testing.T) {
	w := httptest.NewRecorder()
	Created(w, "new-resource")
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

func TestNoContent(t *testing.T) {
	w := httptest.NewRecorder()
	NoContent(w)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Error("expected no body for 204")
	}
}

// --- Error Response Tests ---

func TestError_AppError(t *testing.T) {
	w := httptest.NewRecorder()
	err := apperror.NotFound("item not found", nil)
	Error(w, err)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
	var resp JSON
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Success {
		t.Error("expected Success to be false for error response")
	}
	if resp.Error != "item not found" {
		t.Errorf("expected error 'item not found', got '%s'", resp.Error)
	}
}

func TestError_RandomError(t *testing.T) {
	w := httptest.NewRecorder()
	regularErr := fmt.Errorf("wrapped error")
	wrappedErr := apperror.Internal("wrap failed", regularErr)
	Error(w, wrappedErr)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestError_NonAppError(t *testing.T) {
	w := httptest.NewRecorder()
	Error(w, fmt.Errorf("not an app error"))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for non-AppError, got %d", w.Code)
	}
	var resp JSON
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error != "internal server error" {
		t.Errorf("expected 'internal server error', got '%s'", resp.Error)
	}
}

// --- Middleware Tests ---

func TestRequestIDMiddleware_GeneratesNewID(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := GetRequestID(r.Context())
		if id == "" {
			t.Error("expected request_id in context")
		}
	}))
	handler.ServeHTTP(w, r)

	headerID := w.Header().Get("X-Request-ID")
	if headerID == "" {
		t.Error("expected X-Request-ID header to be set")
	}
}

func TestRequestIDMiddleware_PreservesExisting(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Request-ID", "custom-id-123")
	var capturedID string
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = GetRequestID(r.Context())
	}))
	handler.ServeHTTP(w, r)

	if capturedID != "custom-id-123" {
		t.Errorf("expected custom ID 'custom-id-123', got '%s'", capturedID)
	}
	if w.Header().Get("X-Request-ID") != "custom-id-123" {
		t.Error("expected X-Request-ID header to preserve original")
	}
}

func TestAuthContextMiddleware_SetsUserID(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-User-ID", "user-42")

	var capturedUserID string
	handler := AuthContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserID = GetUserID(r.Context())
	}))
	handler.ServeHTTP(httptest.NewRecorder(), r)

	if capturedUserID != "user-42" {
		t.Errorf("expected user_id 'user-42', got '%s'", capturedUserID)
	}
}

func TestAuthContextMiddleware_SetsOrgID(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Organization-ID", "org-99")

	var capturedOrgID string
	handler := AuthContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedOrgID = GetOrganizationID(r.Context())
	}))
	handler.ServeHTTP(httptest.NewRecorder(), r)

	if capturedOrgID != "org-99" {
		t.Errorf("expected org_id 'org-99', got '%s'", capturedOrgID)
	}
}

func TestAuthContextMiddleware_MissingHeaders(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	handler := AuthContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if GetUserID(r.Context()) != "" {
			t.Error("expected empty user_id when header missing")
		}
		if GetOrganizationID(r.Context()) != "" {
			t.Error("expected empty org_id when header missing")
		}
	}))
	handler.ServeHTTP(httptest.NewRecorder(), r)
}

func TestGetRequestID_Empty(t *testing.T) {
	ctx := context.Background()
	if GetRequestID(ctx) != "" {
		t.Error("expected empty string for missing request_id")
	}
}

func TestGetUserID_Empty(t *testing.T) {
	ctx := context.Background()
	if GetUserID(ctx) != "" {
		t.Error("expected empty string for missing user_id")
	}
}
