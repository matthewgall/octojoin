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
	"net/http"
)

// APIError represents an error from the Octopus Energy API
type APIError struct {
	StatusCode int
	Endpoint   string
	Message    string
	Retryable  bool
	Err        error // Underlying error if any
}

func (e *APIError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("API error (%d) at %s: %s (caused by: %v)", e.StatusCode, e.Endpoint, e.Message, e.Err)
	}
	return fmt.Sprintf("API error (%d) at %s: %s", e.StatusCode, e.Endpoint, e.Message)
}

func (e *APIError) Unwrap() error {
	return e.Err
}

// NewAPIError creates a new APIError with automatic retryable detection
func NewAPIError(statusCode int, endpoint, message string, err error) *APIError {
	return &APIError{
		StatusCode: statusCode,
		Endpoint:   endpoint,
		Message:    message,
		Retryable:  isRetryableStatus(statusCode),
		Err:        err,
	}
}

// isRetryableStatus determines if an HTTP status code is retryable
func isRetryableStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests, // 429
		http.StatusInternalServerError,     // 500
		http.StatusBadGateway,               // 502
		http.StatusServiceUnavailable,       // 503
		http.StatusGatewayTimeout:           // 504
		return true
	default:
		return false
	}
}

// AuthError represents authentication/authorization errors
type AuthError struct {
	Code    string // Error code from API (e.g., "KT-CT-1139")
	Message string
	Err     error
}

func (e *AuthError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("authentication error [%s]: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("authentication error: %s", e.Message)
}

func (e *AuthError) Unwrap() error {
	return e.Err
}

// CacheError represents errors related to cache operations
type CacheError struct {
	CacheType string // e.g., "saving_sessions", "meter_devices"
	Operation string // e.g., "read", "write", "validate"
	Err       error
}

func (e *CacheError) Error() string {
	return fmt.Sprintf("cache error for %s during %s: %v", e.CacheType, e.Operation, e.Err)
}

func (e *CacheError) Unwrap() error {
	return e.Err
}

// ValidationError represents configuration or input validation errors
type ValidationError struct {
	Field   string
	Value   interface{}
	Message string
}

func (e *ValidationError) Error() string {
	if e.Value != nil {
		return fmt.Sprintf("validation error for %s (value: %v): %s", e.Field, e.Value, e.Message)
	}
	return fmt.Sprintf("validation error for %s: %s", e.Field, e.Message)
}

// SessionError represents errors specific to saving session operations
type SessionError struct {
	SessionID string
	Operation string // e.g., "join", "leave", "fetch"
	Err       error
}

func (e *SessionError) Error() string {
	if e.SessionID != "" {
		return fmt.Sprintf("session error [%s] during %s: %v", e.SessionID, e.Operation, e.Err)
	}
	return fmt.Sprintf("session error during %s: %v", e.Operation, e.Err)
}

func (e *SessionError) Unwrap() error {
	return e.Err
}
