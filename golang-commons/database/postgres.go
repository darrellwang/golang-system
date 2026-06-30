// Package database provides PostgreSQL configuration, DSN builder, and health checker.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Config holds PostgreSQL connection configuration.
type Config struct {
	Host     string
	Port     int
	User     string
	Name     string
	Password string
	SSLMode  string
}

// DSN builds a libpq-style PostgreSQL DSN string.
func (c Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Name, c.SSLMode,
	)
}

// NewHealthChecker returns a function that pings the pool to verify DB connectivity.
func NewHealthChecker(pool *pgxpool.Pool) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		return pool.Ping(ctx)
	}
}
