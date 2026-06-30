// Package apperror provides structured application error types and constructors.
package apperror

import (
	"errors"
	"fmt"
	"net/http"
)

// AppError represents an application-level error with an HTTP status code.
type AppError struct {
	Code    int    `json:"-"`
	Message string `json:"message"`
	Err     error  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Err
}

// Predefined error constructors (common pattern: *NotFound, *Internal, etc.)
func NewNotFound(message string) *AppError {
	return &AppError{Code: http.StatusNotFound, Message: message}
}

func NewBadInput(message string) *AppError {
	return &AppError{Code: http.StatusBadRequest, Message: message}
}

func NewInternal(message string) *AppError {
	return &AppError{Code: http.StatusInternalServerError, Message: message}
}

func NewConflict(message string) *AppError {
	return &AppError{Code: http.StatusConflict, Message: message}
}

// NotFound / BadInput / Internal / Conflict with wrapped cause
func NotFound(message string, err error) *AppError {
	return &AppError{Code: http.StatusNotFound, Message: message, Err: err}
}

func BadInput(message string, err error) *AppError {
	return &AppError{Code: http.StatusBadRequest, Message: message, Err: err}
}

func Internal(message string, err error) *AppError {
	return &AppError{Code: http.StatusInternalServerError, Message: message, Err: err}
}

func Conflict(message string, err error) *AppError {
	return &AppError{Code: http.StatusConflict, Message: message, Err: err}
}

// IsAppError extracts *AppError from an error chain.
func IsAppError(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}
