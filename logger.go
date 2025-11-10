// Copyright 2025 Matthew Gall <me@matthewgall.dev>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"log/slog"
	"os"
)

// Logger wraps slog.Logger for structured logging throughout the application
type Logger struct {
	*slog.Logger
}

// NewLogger creates a new structured logger
func NewLogger(debug bool) *Logger {
	var level slog.Level
	if debug {
		level = slog.LevelDebug
	} else {
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	handler := slog.NewTextHandler(os.Stdout, opts)
	return &Logger{
		Logger: slog.New(handler),
	}
}

// NewJSONLogger creates a new JSON structured logger (useful for production/log aggregation)
func NewJSONLogger(debug bool) *Logger {
	var level slog.Level
	if debug {
		level = slog.LevelDebug
	} else {
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)
	return &Logger{
		Logger: slog.New(handler),
	}
}

// WithComponent returns a logger with a component field pre-set
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{
		Logger: l.Logger.With("component", component),
	}
}

// WithSessionID returns a logger with a session_id field pre-set
func (l *Logger) WithSessionID(sessionID string) *Logger {
	return &Logger{
		Logger: l.Logger.With("session_id", sessionID),
	}
}

// WithAccountID returns a logger with an account_id field pre-set
func (l *Logger) WithAccountID(accountID string) *Logger {
	// Mask account ID for privacy (show only prefix)
	maskedID := accountID
	if len(accountID) > 5 {
		maskedID = accountID[:5] + "***"
	}
	return &Logger{
		Logger: l.Logger.With("account_id", maskedID),
	}
}

// LogAPIRequest logs an API request with common fields
func (l *Logger) LogAPIRequest(method, endpoint string, statusCode int, duration float64) {
	l.Info("API request",
		"method", method,
		"endpoint", endpoint,
		"status_code", statusCode,
		"duration_ms", duration*1000,
	)
}

// LogAPIError logs an API error with details
func (l *Logger) LogAPIError(err error, endpoint string) {
	if apiErr, ok := err.(*APIError); ok {
		l.Error("API request failed",
			"endpoint", endpoint,
			"status_code", apiErr.StatusCode,
			"retryable", apiErr.Retryable,
			"error", apiErr.Message,
		)
	} else {
		l.Error("API request failed",
			"endpoint", endpoint,
			"error", err.Error(),
		)
	}
}

// LogSessionJoin logs when joining a saving session
func (l *Logger) LogSessionJoin(sessionID string, points int, startTime string) {
	l.Info("Joining saving session",
		"session_id", sessionID,
		"points", points,
		"start_time", startTime,
	)
}

// LogCacheHit logs a cache hit
func (l *Logger) LogCacheHit(cacheType string, age float64) {
	l.Debug("Cache hit",
		"cache_type", cacheType,
		"age_seconds", age,
	)
}

// LogCacheMiss logs a cache miss
func (l *Logger) LogCacheMiss(cacheType string, reason string) {
	l.Debug("Cache miss",
		"cache_type", cacheType,
		"reason", reason,
	)
}

// UserMessage outputs a user-friendly message (bypasses structured logging)
// Use this for primary user-facing output in non-daemon mode
func (l *Logger) UserMessage(format string, args ...interface{}) {
	fmt.Printf(format+"\n", args...)
}

// UserMessagef outputs a user-friendly message without newline
func (l *Logger) UserMessagef(format string, args ...interface{}) {
	fmt.Printf(format, args...)
}
