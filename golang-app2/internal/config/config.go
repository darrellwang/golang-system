// Package config loads env-var based configuration for the order service.
package config

import (
	"os"
	"strconv"
)

// Version is injected at build time via -ldflags.
var Version = "dev"

type Config struct {
	AppPort        int
	AppMode        string
	AppName        string
	LogLevel       string

	DBHost         string
	DBPort         int
	DBUser         string
	DBName         string
	DBPassword     string
	DBSSLMode      string

	NATSURL        string
	QueueGroup     string
	ItemServiceURL string
}

func Load() *Config {
	return &Config{
		AppPort:        envInt("APP_PORT", 8081),
		AppMode:        envStr("APP_MODE", "DEV"),
		AppName:        envStr("APP_NAME", "golang-app2"),
		LogLevel:       envStr("LOG_LEVEL", "info"),
		DBHost:         envStr("DB_HOST", "localhost"),
		DBPort:         envInt("DB_PORT", 5433),
		DBUser:         envStr("DB_USER", "app2"),
		DBName:         envStr("DB_NAME", "golang-app2"),
		DBPassword:     envStr("DB_PASSWORD", ""),
		DBSSLMode:      envStr("DB_SSLMODE", "disable"),
		NATSURL:        envStr("NATS_URL", "nats://localhost:4222"),
		QueueGroup:     envStr("QUEUE_GROUP", "golang-app2-queue"),
		ItemServiceURL: envStr("ITEM_SERVICE_URL", "http://localhost:8080"),
	}
}

func envStr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func envInt(k string, d int) int {
	if v := os.Getenv(k); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return d
}
