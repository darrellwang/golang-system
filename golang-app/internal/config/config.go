// Package config loads env-var based configuration for the item service.
package config

import (
	"os"
	"strconv"
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

	NATSURL    string
	QueueGroup string
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
