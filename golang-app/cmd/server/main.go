// Package main is the entry point of the item service.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/darrell/golang-system/golang-app/internal/app"
	"github.com/darrell/golang-system/golang-app/internal/config"
	"github.com/darrell/golang-system/golang-commons/logger"
)

func main() {
	// --- Load configuration ---
	cfg := config.Load()

	// --- Setup structured logger ---
	logger.Setup(cfg.LogLevel, logger.Mode(cfg.AppMode))

	log := *zerolog.DefaultContextLogger
	log.Info().
		Str("version", config.Version).
		Str("mode", cfg.AppMode).
		Int("port", cfg.AppPort).
		Msg("Starting application")

	// --- Create root context with cancellation ---
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// --- Bootstrap and run the application ---
	application, err := app.New(ctx, cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize application")
	}

	// --- Start server in background ---
	errch := make(chan error, 1)
	go func() {
		errch <- application.Start()
	}()

	// --- Wait for shutdown signal ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		log.Info().Str("signal", sig.String()).Msg("Shutdown signal received")
	case err := <-errch:
		if err != nil {
			log.Error().Err(err).Msg("Server error")
		}
	}

	// --- Graceful shutdown with timeout ---
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := application.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Shutdown error")
		os.Exit(1)
	}

	log.Info().Msg("Application stopped gracefully")
}
