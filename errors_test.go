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
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestAPIError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		endpoint   string
		message    string
		err        error
		wantString string
		retryable  bool
	}{
		{
			name:       "retryable 429 error",
			statusCode: http.StatusTooManyRequests,
			endpoint:   "/api/sessions",
			message:    "rate limited",
			err:        nil,
			wantString: "API error (429) at /api/sessions: rate limited",
			retryable:  true,
		},
		{
			name:       "non-retryable 404 error",
			statusCode: http.StatusNotFound,
			endpoint:   "/api/account",
			message:    "not found",
			err:        nil,
			wantString: "API error (404) at /api/account: not found",
			retryable:  false,
		},
		{
			name:       "error with underlying cause",
			statusCode: http.StatusInternalServerError,
			endpoint:   "/api/sessions",
			message:    "server error",
			err:        errors.New("connection timeout"),
			wantString: "connection timeout",
			retryable:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiErr := NewAPIError(tt.statusCode, tt.endpoint, tt.message, tt.err)

			// Check error string contains expected parts
			errStr := apiErr.Error()
			if !strings.Contains(errStr, tt.wantString) {
				t.Errorf("Error() = %q, want to contain %q", errStr, tt.wantString)
			}

			// Check retryable status
			if apiErr.Retryable != tt.retryable {
				t.Errorf("Retryable = %v, want %v", apiErr.Retryable, tt.retryable)
			}

			// Check unwrap
			if tt.err != nil && apiErr.Unwrap() != tt.err {
				t.Errorf("Unwrap() = %v, want %v", apiErr.Unwrap(), tt.err)
			}
		})
	}
}

func TestAuthError(t *testing.T) {
	t.Run("with error code", func(t *testing.T) {
		authErr := &AuthError{
			Code:    "KT-CT-1139",
			Message: "JWT token expired",
		}

		errStr := authErr.Error()
		if !strings.Contains(errStr, "KT-CT-1139") {
			t.Errorf("Error() = %q, want to contain error code", errStr)
		}
		if !strings.Contains(errStr, "JWT token expired") {
			t.Errorf("Error() = %q, want to contain message", errStr)
		}
	})

	t.Run("without error code", func(t *testing.T) {
		authErr := &AuthError{
			Message: "invalid credentials",
		}

		errStr := authErr.Error()
		if !strings.Contains(errStr, "invalid credentials") {
			t.Errorf("Error() = %q, want to contain message", errStr)
		}
	})
}

func TestCacheError(t *testing.T) {
	underlyingErr := errors.New("file not found")
	cacheErr := &CacheError{
		CacheType: "saving_sessions",
		Operation: "read",
		Err:       underlyingErr,
	}

	errStr := cacheErr.Error()
	if !strings.Contains(errStr, "saving_sessions") {
		t.Errorf("Error() = %q, want to contain cache type", errStr)
	}
	if !strings.Contains(errStr, "read") {
		t.Errorf("Error() = %q, want to contain operation", errStr)
	}
	if !strings.Contains(errStr, "file not found") {
		t.Errorf("Error() = %q, want to contain underlying error", errStr)
	}

	if cacheErr.Unwrap() != underlyingErr {
		t.Errorf("Unwrap() = %v, want %v", cacheErr.Unwrap(), underlyingErr)
	}
}

func TestValidationError(t *testing.T) {
	t.Run("with value", func(t *testing.T) {
		valErr := &ValidationError{
			Field:   "port",
			Value:   99999,
			Message: "port out of range",
		}

		errStr := valErr.Error()
		if !strings.Contains(errStr, "port") {
			t.Errorf("Error() = %q, want to contain field name", errStr)
		}
		if !strings.Contains(errStr, "99999") {
			t.Errorf("Error() = %q, want to contain value", errStr)
		}
	})

	t.Run("without value", func(t *testing.T) {
		valErr := &ValidationError{
			Field:   "account_id",
			Message: "required",
		}

		errStr := valErr.Error()
		if !strings.Contains(errStr, "account_id") {
			t.Errorf("Error() = %q, want to contain field name", errStr)
		}
	})
}

func TestSessionError(t *testing.T) {
	underlyingErr := errors.New("API timeout")
	sessErr := &SessionError{
		SessionID: "sess-123",
		Operation: "join",
		Err:       underlyingErr,
	}

	errStr := sessErr.Error()
	if !strings.Contains(errStr, "sess-123") {
		t.Errorf("Error() = %q, want to contain session ID", errStr)
	}
	if !strings.Contains(errStr, "join") {
		t.Errorf("Error() = %q, want to contain operation", errStr)
	}

	if sessErr.Unwrap() != underlyingErr {
		t.Errorf("Unwrap() = %v, want %v", sessErr.Unwrap(), underlyingErr)
	}
}

func TestIsRetryableStatus(t *testing.T) {
	tests := []struct {
		statusCode int
		retryable  bool
	}{
		{http.StatusOK, false},                  // 200
		{http.StatusBadRequest, false},          // 400
		{http.StatusUnauthorized, false},        // 401
		{http.StatusNotFound, false},            // 404
		{http.StatusTooManyRequests, true},      // 429
		{http.StatusInternalServerError, true},  // 500
		{http.StatusBadGateway, true},           // 502
		{http.StatusServiceUnavailable, true},   // 503
		{http.StatusGatewayTimeout, true},       // 504
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.statusCode), func(t *testing.T) {
			got := isRetryableStatus(tt.statusCode)
			if got != tt.retryable {
				t.Errorf("isRetryableStatus(%d) = %v, want %v", tt.statusCode, got, tt.retryable)
			}
		})
	}
}

func TestErrorsAsInterface(t *testing.T) {
	// Test that errors can be checked with errors.As
	apiErr := NewAPIError(429, "/api/test", "rate limited", nil)

	var targetAPIErr *APIError
	if !errors.As(apiErr, &targetAPIErr) {
		t.Error("errors.As should work with APIError")
	}

	if targetAPIErr.StatusCode != 429 {
		t.Errorf("StatusCode = %d, want 429", targetAPIErr.StatusCode)
	}
}
