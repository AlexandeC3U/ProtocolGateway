// Package logging provides structured logging functionality.
package logging

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

// New creates a new structured logger.
func New(serviceName, version string) zerolog.Logger {
	// Configure zerolog
	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.DurationFieldUnit = time.Millisecond

	// Determine output
	var output io.Writer = os.Stdout

	// Check environment for log format preference
	logFormat := os.Getenv("LOG_FORMAT")
	if logFormat == "" {
		logFormat = "json"
	}

	// Use console writer for development
	if logFormat == "console" || logFormat == "text" {
		output = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
	}

	// Set log level
	logLevel := os.Getenv("LOG_LEVEL")
	level := zerolog.InfoLevel
	switch logLevel {
	case "debug":
		level = zerolog.DebugLevel
	case "warn":
		level = zerolog.WarnLevel
	case "error":
		level = zerolog.ErrorLevel
	case "trace":
		level = zerolog.TraceLevel
	}

	return zerolog.New(output).
		Level(level).
		With().
		Timestamp().
		Str("service", serviceName).
		Str("version", version).
		Logger()
}

// NewWithConfig creates a logger with the given configuration.
func NewWithConfig(serviceName, version string, config LogConfig) zerolog.Logger {
	zerolog.TimeFieldFormat = config.TimeFormat
	zerolog.DurationFieldUnit = time.Millisecond

	var output io.Writer

	// Determine output destination
	switch config.Output {
	case "stderr":
		output = os.Stderr
	case "stdout", "":
		output = os.Stdout
	default:
		// Assume it's a file path
		file, err := os.OpenFile(config.Output, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			output = os.Stdout
		} else {
			output = file
		}
	}

	// Apply console formatting if requested
	if config.Format == "console" || config.Format == "text" {
		output = zerolog.ConsoleWriter{
			Out:        output,
			TimeFormat: time.RFC3339,
			NoColor:    config.NoColor,
		}
	}

	// Set log level
	level := parseLogLevel(config.Level)

	return zerolog.New(output).
		Level(level).
		With().
		Timestamp().
		Str("service", serviceName).
		Str("version", version).
		Caller().
		Logger()
}

// LogConfig holds logging configuration.
type LogConfig struct {
	Level      string
	Format     string // "json" or "console"
	Output     string // "stdout", "stderr", or file path
	TimeFormat string
	NoColor    bool
}

// DefaultLogConfig returns default logging configuration.
func DefaultLogConfig() LogConfig {
	return LogConfig{
		Level:      "info",
		Format:     "json",
		Output:     "stdout",
		TimeFormat: time.RFC3339Nano,
		NoColor:    false,
	}
}

// parseLogLevel converts a string log level to zerolog.Level.
func parseLogLevel(level string) zerolog.Level {
	switch level {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	case "panic":
		return zerolog.PanicLevel
	default:
		return zerolog.InfoLevel
	}
}

// With returns a child logger with the given fields.
func With(logger zerolog.Logger, fields map[string]interface{}) zerolog.Logger {
	ctx := logger.With()
	for key, value := range fields {
		ctx = ctx.Interface(key, value)
	}
	return ctx.Logger()
}

// Error logs an error with additional context.
func Error(logger zerolog.Logger, err error, msg string) {
	logger.Error().Err(err).Stack().Msg(msg)
}

// WithDeviceContext adds device context to the logger.
func WithDeviceContext(logger zerolog.Logger, deviceID, deviceName string) zerolog.Logger {
	return logger.With().
		Str("device_id", deviceID).
		Str("device_name", deviceName).
		Logger()
}

// WithRequestContext adds request context to the logger.
func WithRequestContext(logger zerolog.Logger, requestID, method, path string) zerolog.Logger {
	return logger.With().
		Str("request_id", requestID).
		Str("method", method).
		Str("path", path).
		Logger()
}

