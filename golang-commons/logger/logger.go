// Package logger provides structured logging initialization backed by zerolog.
//
// In DEV mode it produces human-readable console output; in PROD mode it produces
// JSON suitable for shipping to log aggregation systems.
package logger

import (
	"os"

	"github.com/rs/zerolog"
)

// Mode selects the output format.
type Mode string

const (
	ModeDev  Mode = "DEV"
	ModeProd Mode = "PROD"
)

// Setup initializes the global zerolog logger and stores it as the default
// context logger (zerolog.DefaultContextLogger).
func Setup(level string, mode Mode) {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)
	zerolog.TimeFieldFormat = "2006-01-02T15:04:05.999999999Z07:00"

	var log zerolog.Logger
	if mode == ModeDev {
		// Human-readable console output for development.
		log = zerolog.New(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "2006-01-02T15:04:05",
		}).With().Timestamp().Logger()
		if lvl == zerolog.DebugLevel {
			log = log.With().Caller().Logger()
		}
	} else {
		// JSON structured output for production.
		log = zerolog.New(os.Stdout).With().Timestamp().Logger()
	}
	zerolog.DefaultContextLogger = &log
}

// Global returns the global default context logger.
// If Setup has not been called, it returns zerolog's built-in disabled logger.
func Global() *zerolog.Logger { return zerolog.DefaultContextLogger }

// Sync is a no-op for zerolog (writes are unbuffered).
func Sync() {}
