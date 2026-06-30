// Package httputil provides HTTP response helpers and middleware.
package httputil

import (
	"encoding/json"
	"net/http"

	apperror "github.com/darrell/golang-system/golang-commons/apperror"
)

// JSON envelope for every response.
type JSON struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Write sends a JSON response with the given HTTP status and data payload.
func Write(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(JSON{
		Success: status >= 200 && status < 300,
		Data:    data,
	})
}

// OK sends a 200 response.
func OK(w http.ResponseWriter, data any) { Write(w, http.StatusOK, data) }

// Created sends a 201 response.
func Created(w http.ResponseWriter, data any) { Write(w, http.StatusCreated, data) }

// NoContent sends a 204 with no body.
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// Error sends an error response derived from an error.
func Error(w http.ResponseWriter, err error) {
	appErr, ok := apperror.IsAppError(err)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(JSON{Success: false, Error: "internal server error"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(appErr.Code)
	_ = json.NewEncoder(w).Encode(JSON{Success: false, Error: appErr.Message})
}
