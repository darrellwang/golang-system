package httputil

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// RequestIDMiddleware adds a request ID to context and response header.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.New().String()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), "request_id", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// LoggingMiddleware logs every HTTP request via zerolog.
func LoggingMiddleware(log zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			log.Info().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", sw.status).
				Dur("duration", time.Since(start)).
				Str("request_id", GetRequestID(r.Context())).
				Msg("http_request")
		})
	}
}

// AuthContextMiddleware extracts user/organization IDs from headers into context.
func AuthContextMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if orgID := r.Header.Get("X-Organization-ID"); orgID != "" {
			ctx = context.WithValue(ctx, "organization_id", orgID)
		}
		if userID := r.Header.Get("X-User-ID"); userID != "" {
			ctx = context.WithValue(ctx, "user_id", userID)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// MaxBodySizeMiddleware limits request body size.
func MaxBodySizeMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// ContentTypeMiddleware enforces Content-Type on requests with a body.
func ContentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > 0 && r.Header.Get("Content-Type") == "" {
			r.Header.Set("Content-Type", "application/json")
		}
		next.ServeHTTP(w, r)
	})
}

// RecoveryMiddleware recovers from panics and returns 500.
func RecoveryMiddleware(log zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error().
						Interface("panic", rec).
						Str("path", r.URL.Path).
						Msg("panic_recovered")
					w.WriteHeader(http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// GetRequestID retrieves the request ID from context.
func GetRequestID(ctx context.Context) string {
	if v, ok := ctx.Value("request_id").(string); ok {
		return v
	}
	return ""
}

// GetUserID retrieves the user ID from context.
func GetUserID(ctx context.Context) string {
	if v, ok := ctx.Value("user_id").(string); ok {
		return v
	}
	return ""
}

// GetOrganizationID retrieves the organization ID from context.
func GetOrganizationID(ctx context.Context) string {
	if v, ok := ctx.Value("organization_id").(string); ok {
		return v
	}
	return ""
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}
