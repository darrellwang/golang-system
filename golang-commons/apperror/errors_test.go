package apperror

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestNewNotFound(t *testing.T) {
	err := NewNotFound("resource not found")
	if err.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, err.Code)
	}
	if err.Message != "resource not found" {
		t.Errorf("expected message 'resource not found', got '%s'", err.Message)
	}
	if err.Err != nil {
		t.Error("expected Err to be nil")
	}
}

func TestNewBadInput(t *testing.T) {
	err := NewBadInput("invalid input")
	if err.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, err.Code)
	}
	if err.Message != "invalid input" {
		t.Errorf("expected message 'invalid input', got '%s'", err.Message)
	}
}

func TestNewInternal(t *testing.T) {
	err := NewInternal("internal error")
	if err.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, err.Code)
	}
	if err.Message != "internal error" {
		t.Errorf("expected message 'internal error', got '%s'", err.Message)
	}
}

func TestNewConflict(t *testing.T) {
	err := NewConflict("conflict detected")
	if err.Code != http.StatusConflict {
		t.Errorf("expected status %d, got %d", http.StatusConflict, err.Code)
	}
	if err.Message != "conflict detected" {
		t.Errorf("expected message 'conflict detected', got '%s'", err.Message)
	}
}

func TestNotFound_WithCause(t *testing.T) {
	cause := errors.New("database error")
	err := NotFound("not found", cause)
	if err.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, err.Code)
	}
	if err.Err != cause {
		t.Error("expected Err to be the provided cause")
	}
}

func TestBadInput_WithCause(t *testing.T) {
	cause := errors.New("validation failed")
	err := BadInput("bad input", cause)
	if err.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, err.Code)
	}
	if err.Err != cause {
		t.Error("expected Err to be the provided cause")
	}
}

func TestInternal_WithCause(t *testing.T) {
	cause := errors.New("panic occurred")
	err := Internal("internal error", cause)
	if err.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, err.Code)
	}
	if err.Err != cause {
		t.Error("expected Err to be the provided cause")
	}
}

func TestConflict_WithCause(t *testing.T) {
	cause := errors.New("unique constraint violated")
	err := Conflict("conflict", cause)
	if err.Code != http.StatusConflict {
		t.Errorf("expected status %d, got %d", http.StatusConflict, err.Code)
	}
	if err.Err != cause {
		t.Error("expected Err to be the provided cause")
	}
}

func TestError_WithoutCause(t *testing.T) {
	err := NewNotFound("not found")
	expected := "not found"
	if err.Error() != expected {
		t.Errorf("expected '%s', got '%s'", expected, err.Error())
	}
}

func TestError_WithCause(t *testing.T) {
	cause := errors.New("db error")
	err := NotFound("not found", cause)
	expected := "not found: db error"
	if err.Error() != expected {
		t.Errorf("expected '%s', got '%s'", expected, err.Error())
	}
}

func TestUnwrap(t *testing.T) {
	cause := errors.New("wrapped error")
	err := Internal("internal", cause)
	unwrapped := errors.Unwrap(err)
	if unwrapped != cause {
		t.Errorf("expected to unwrap to cause, got %v", unwrapped)
	}
}

func TestIsAppError_Success(t *testing.T) {
	original := NewNotFound("not found")
	appErr, ok := IsAppError(original)
	if !ok {
		t.Fatal("expected IsAppError to return true")
	}
	if appErr != original {
		t.Error("expected returned AppError to be the same instance")
	}
}

func TestIsAppError_Wrapped(t *testing.T) {
	original := NewBadInput("bad input")
	wrapped := fmt.Errorf("wrapped: %w", original)
	appErr, ok := IsAppError(wrapped)
	if !ok {
		t.Fatal("expected IsAppError to find AppError in wrap chain")
	}
	if appErr.Message != "bad input" {
		t.Errorf("expected message 'bad input', got '%s'", appErr.Message)
	}
}

func TestIsAppError_NonAppError(t *testing.T) {
	regular := errors.New("regular error")
	_, ok := IsAppError(regular)
	if ok {
		t.Error("expected IsAppError to return false for non-AppError")
	}
}

