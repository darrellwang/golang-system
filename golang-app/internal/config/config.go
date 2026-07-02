// Package config loads env-var based configuration for the item service.
package config

import (
	"os"
	"strconv"
	"strings"
)

// Version is injected at build time via -ldflags.
var Version = "dev"

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
}

// Load reads configuration from environment variables.
func Load() *Config {
	return &Config{
		AppPort:    envInt("APP_PORT", 8080),
		AppMode:    envStr("APP_MODE", "DEV"),
		AppName:    envStr("APP_NAME", "golang-app"),
		LogLevel:   envStr("LOG_LEVEL", "info"),
		DBHost:     envStr("DB_HOST", "localhost"),
		DBPort:     envInt("DB_PORT", 5432),
		DBUser:     envStr("DB_USER", "app"),
		DBName:     envStr("DB_NAME", "golang-app"),
		DBPassword: envStr("DB_PASSWORD", ""),
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
