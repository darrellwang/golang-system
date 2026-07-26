package httputil

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	apperror "github.com/darrell/golang-system/golang-commons/apperror"
)

// Typed context keys to avoid collisions across packages.
type ctxKey int

const (
	CtxRequestID ctxKey = iota + 1
	CtxUserID
	CtxOrganizationID
	CtxUserEmail
	CtxAuthorization
	CtxTraceParent
)

// CORSConfig holds Cross-Origin Resource Sharing configuration.
type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	AllowCredentials bool
	MaxAge           time.Duration
}

// DefaultCORSConfig returns a permissive CORS configuration for development.
func DefaultCORSConfig() *CORSConfig {
	return &CORSConfig{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowedHeaders: []string{"*"},
		MaxAge:         5 * time.Minute,
	}
}

// CORSMiddleware adds CORS headers to responses.
func CORSMiddleware(cfg *CORSConfig) func(http.Handler) http.Handler {
	if cfg == nil {
		cfg = DefaultCORSConfig()
	}

	allowedMethods := strings.Join(cfg.AllowedMethods, ",")
	allowedHeaders := strings.Join(cfg.AllowedHeaders, ",")
	allowAll := len(cfg.AllowedOrigins) == 1 && cfg.AllowedOrigins[0] == "*"

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if allowAll {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if origin != "" && originAllowed(origin, cfg.AllowedOrigins) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
			}

			w.Header().Set("Access-Control-Allow-Methods", allowedMethods)
			w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
			if cfg.AllowCredentials && !allowAll {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			if cfg.MaxAge > 0 {
				w.Header().Set("Access-Control-Max-Age", intToString(int(cfg.MaxAge.Seconds())))
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func originAllowed(origin string, allowed []string) bool {
	for _, o := range allowed {
		if o == origin {
			return true
		}
	}
	return false
}

func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := make([]byte, 0, 12)
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}
	if neg {
		buf = append(buf, '-')
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}

// RequestIDMiddleware adds a request ID to context and response header.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.New().String()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), CtxRequestID, id)
		if tp := r.Header.Get("traceparent"); tp != "" {
			ctx = context.WithValue(ctx, CtxTraceParent, tp)
		}
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

// AuthConfig controls how identity is derived from inbound headers.
type AuthConfig struct {
	// TrustHeaders allows falling back to client-supplied X-User-ID /
	// X-Organization-ID when JWT claim headers are absent. Disable in
	// production behind a JWT-enforcing gateway.
	TrustHeaders bool
}

// AuthContextMiddleware extracts identity into context.
// Prefer Istio JWT claim headers (x-jwt-sub / x-jwt-org) over forgeable
// X-User-ID headers.
func AuthContextMiddleware(cfg *AuthConfig) func(http.Handler) http.Handler {
	if cfg == nil {
		cfg = &AuthConfig{TrustHeaders: true}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			userID := r.Header.Get("x-jwt-sub")
			if userID == "" && cfg.TrustHeaders {
				userID = r.Header.Get("X-User-ID")
			}
			if userID != "" {
				ctx = context.WithValue(ctx, CtxUserID, userID)
			}

			orgID := r.Header.Get("x-jwt-org")
			if orgID == "" && cfg.TrustHeaders {
				orgID = r.Header.Get("X-Organization-ID")
			}
			if orgID != "" {
				ctx = context.WithValue(ctx, CtxOrganizationID, orgID)
			}

			if email := r.Header.Get("x-jwt-email"); email != "" {
				ctx = context.WithValue(ctx, CtxUserEmail, email)
			}
			if auth := r.Header.Get("Authorization"); auth != "" {
				ctx = context.WithValue(ctx, CtxAuthorization, auth)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAuthenticated rejects requests that have no authenticated user in context.
// Use on REST route groups when AUTH_REQUIRE_JWT=true (defense in depth behind the gateway).
func RequireAuthenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if GetUserID(r.Context()) == "" {
			Error(w, apperror.NewUnauthorized("authentication required"))
			return
		}
		next.ServeHTTP(w, r)
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

// RecoveryMiddleware recovers from panics and returns a JSON 500 envelope.
func RecoveryMiddleware(log zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error().
						Interface("panic", rec).
						Str("path", r.URL.Path).
						Msg("panic_recovered")
					Error(w, apperror.NewInternal("internal server error"))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// GetRequestID retrieves the request ID from context.
func GetRequestID(ctx context.Context) string {
	if v, ok := ctx.Value(CtxRequestID).(string); ok {
		return v
	}
	return ""
}

// GetUserID retrieves the user ID from context.
func GetUserID(ctx context.Context) string {
	if v, ok := ctx.Value(CtxUserID).(string); ok {
		return v
	}
	return ""
}

// GetOrganizationID retrieves the organization ID from context.
func GetOrganizationID(ctx context.Context) string {
	if v, ok := ctx.Value(CtxOrganizationID).(string); ok {
		return v
	}
	return ""
}

// GetUserEmail retrieves the email claim from context when present.
func GetUserEmail(ctx context.Context) string {
	if v, ok := ctx.Value(CtxUserEmail).(string); ok {
		return v
	}
	return ""
}

// GetAuthorization retrieves the original Authorization header from context.
func GetAuthorization(ctx context.Context) string {
	if v, ok := ctx.Value(CtxAuthorization).(string); ok {
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
