// Package config loads env-var based configuration for the item service.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Version is injected at build time via -ldflags.
var Version = "dev"

// defaultDevDBPassword matches docker-compose.yml for local development only.
// Production deployments must override it via the DB_PASSWORD env var.
const defaultDevDBPassword = "app_password"

// Config is the application configuration.
type Config struct {
	AppPort    int
	AppMode    string // "DEV" or "PROD"
	AppName    string
	LogLevel   string

	DBHost     string
	DBPort     int
	DBUser     string
	DBName     string
	DBPassword string
	DBSSLMode  string

	NATSURL            string
	QueueGroup         string

	TLSEnabled         bool
	TLSCertFile        string
	TLSKeyFile         string

	CORSAllowedOrigins []string
	CORSAllowedMethods []string
	CORSAllowedHeaders []string
	CORSAllowCredentials bool

	// AuthTrustHeaders allows X-User-ID fallback when JWT claim headers are absent.
	AuthTrustHeaders bool
	// AuthRequireJWT rejects REST requests with no authenticated user in context.
	AuthRequireJWT bool
}

// Load reads configuration from environment variables.
func Load() *Config {
	cfg := &Config{
		AppPort:    envInt("APP_PORT", 8080),
		AppMode:    envStr("APP_MODE", "DEV"),
		AppName:    envStr("APP_NAME", "golang-app"),
		LogLevel:   envStr("LOG_LEVEL", "info"),
		// DB defaults match docker-compose.yml (postgres-app) so the
		// service runs out of the box in local development.
		DBHost:     envStr("DB_HOST", "localhost"),
		DBPort:     envInt("DB_PORT", 5432),
		DBUser:     envStr("DB_USER", "app_user"),
		DBName:     envStr("DB_NAME", "app_db"),
		DBPassword: envStr("DB_PASSWORD", defaultDevDBPassword),
		DBSSLMode:  envStr("DB_SSLMODE", "disable"),
		NATSURL:    envStr("NATS_URL", "nats://localhost:4222"),
		QueueGroup: envStr("QUEUE_GROUP", "golang-app-queue"),

		TLSEnabled:           envBool("TLS_ENABLED", false),
		TLSCertFile:          envStr("TLS_CERT_FILE", ""),
		TLSKeyFile:           envStr("TLS_KEY_FILE", ""),
		CORSAllowedOrigins:   envStrSlice("CORS_ALLOWED_ORIGINS", []string{"*"}),
		CORSAllowedMethods:   envStrSlice("CORS_ALLOWED_METHODS", []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"}),
		CORSAllowedHeaders:   envStrSlice("CORS_ALLOWED_HEADERS", []string{"*"}),
		CORSAllowCredentials: envBool("CORS_ALLOW_CREDENTIALS", false),

		// DEV defaults trust headers for local curl; PROD chart sets AuthTrustHeaders=false.
		AuthTrustHeaders: envBool("AUTH_TRUST_HEADERS", true),
		AuthRequireJWT:   envBool("AUTH_REQUIRE_JWT", false),
	}
	if cfg.AppMode == "PROD" && os.Getenv("AUTH_TRUST_HEADERS") == "" {
		cfg.AuthTrustHeaders = false
	}
	if cfg.AppMode == "PROD" && os.Getenv("AUTH_REQUIRE_JWT") == "" {
		cfg.AuthRequireJWT = true
	}
	cfg.validateSecurityDefaults()
	return cfg
}

// validateSecurityDefaults warns about insecure CORS defaults when running
// in PROD mode. "*" origins/headers are convenient for local development
// but expose the service to cross-origin abuse in production.
func (c *Config) validateSecurityDefaults() {
	if c.AppMode != "PROD" {
		return
	}
	if c.DBPassword == defaultDevDBPassword {
		fmt.Fprintln(os.Stderr, "WARNING: using the default development DB password in PROD mode — set DB_PASSWORD to a strong secret.")
	}
	for _, o := range c.CORSAllowedOrigins {
		if o == "*" {
			fmt.Fprintln(os.Stderr, "WARNING: CORS_ALLOWED_ORIGINS is '*' in PROD mode — set explicit origins to prevent cross-origin abuse.")
			break
		}
	}
	for _, h := range c.CORSAllowedHeaders {
		if h == "*" {
			fmt.Fprintln(os.Stderr, "WARNING: CORS_ALLOWED_HEADERS is '*' in PROD mode — set explicit headers.")
			break
		}
	}
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		return v == "true" || v == "1" || v == "yes"
	}
	return def
}

func envStrSlice(key string, def []string) []string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	var result []string
	for _, item := range strings.Split(v, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	if len(result) == 0 {
		return def
	}
	return result
}
