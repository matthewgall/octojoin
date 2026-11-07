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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestIsAuthExpiredMessage(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "authorization header expired with quotes (real Octopus message)",
			body: `{"errors":[{"message":"'Authorization' header has expired"}]}`,
			want: true,
		},
		{
			name: "authorization header expired without quotes",
			body: `{"errors":[{"message":"Authorization header has expired"}]}`,
			want: true,
		},
		{
			name: "jwt has expired",
			body: `{"errors":[{"message":"JWT has expired"}]}`,
			want: true,
		},
		{
			name: "signature of the jwt has expired",
			body: `{"errors":[{"message":"Signature of the JWT has expired"}]}`,
			want: true,
		},
		{
			name: "token has expired",
			body: `{"errors":[{"message":"Token has expired"}]}`,
			want: true,
		},
		{
			name: "octopus jwt expired error code",
			body: `{"errors":[{"message":"KT-CT-1139"}]}`,
			want: true,
		},
		{
			name: "octopus invalid auth error code",
			body: `{"errors":[{"message":"KT-CT-1143"}]}`,
			want: true,
		},
		{
			name: "authentication failed",
			body: `{"errors":[{"message":"Authentication failed"}]}`,
			want: true,
		},
		{
			name: "case insensitive match",
			body: `{"errors":[{"message":"'authorization' header has EXPIRED"}]}`,
			want: true,
		},
		{
			name: "valid response is not expired",
			body: `{"data":{"account":{"number":"A-TEST"}}}`,
			want: false,
		},
		{
			name: "unrelated error is not expired",
			body: `{"errors":[{"message":"Internal server error"}]}`,
			want: false,
		},
		{
			name: "empty body is not expired",
			body: "",
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isAuthExpiredMessage(tc.body); got != tc.want {
				t.Errorf("expected isAuthExpiredMessage=%v, got %v", tc.want, got)
			}
		})
	}
}

func buildTestJWT(payload string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	encodedPayload := base64.RawURLEncoding.EncodeToString([]byte(payload))
	signature := base64.RawURLEncoding.EncodeToString([]byte("test-signature"))
	return header + "." + encodedPayload + "." + signature
}

func TestTokenRefreshBackoff(t *testing.T) {
	// Server that counts requests and returns a rate-limit GraphQL error
	// until told to succeed.
	var mu sync.Mutex
	requests := 0
	succeed := false

	token := buildTestJWT(`{"exp":1700000000}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		if succeed {
			fmt.Fprintf(w, `{"data":{"obtainKrakenToken":{"token":%q,"refreshToken":"r","refreshExpiresIn":3600}}}`, token)
		} else {
			fmt.Fprint(w, `{"errors":[{"message":"Too many requests."}]}`)
		}
		mu.Unlock()
	}))
	defer server.Close()

	client := NewOctopusClient("A-TEST", "sk_test", false)
	client.krakenTokenURL = server.URL
	client.tokenBackoffBase = 20 * time.Millisecond
	client.tokenBackoffMax = 80 * time.Millisecond

	// First attempt hits the server and is rate limited -> failure + backoff.
	if err := client.refreshJWTToken(); err == nil {
		t.Fatalf("expected rate-limit error, got nil")
	}

	mu.Lock()
	firstRequests := requests
	mu.Unlock()
	if firstRequests != 1 {
		t.Fatalf("expected 1 token request, got %d", firstRequests)
	}
	if client.tokenRefreshFailures != 1 {
		t.Fatalf("expected 1 recorded failure, got %d", client.tokenRefreshFailures)
	}

	// Within the backoff window, no further request should be made.
	if err := client.refreshJWTToken(); err == nil {
		t.Fatalf("expected backoff error, got nil")
	} else if !strings.Contains(err.Error(), "backoff") {
		t.Fatalf("expected backoff error, got %v", err)
	}

	mu.Lock()
	midRequests := requests
	mu.Unlock()
	if midRequests != firstRequests {
		t.Fatalf("expected no new token request during backoff (got %d, was %d)", midRequests, firstRequests)
	}

	// Wait for the backoff to lapse, then succeed.
	time.Sleep(30 * time.Millisecond)
	succeed = true
	if err := client.refreshJWTToken(); err != nil {
		t.Fatalf("expected successful refresh after backoff, got %v", err)
	}

	mu.Lock()
	finalRequests := requests
	mu.Unlock()
	if finalRequests != firstRequests+1 {
		t.Fatalf("expected exactly one more token request after backoff (got %d, was %d)", finalRequests, firstRequests)
	}

	if client.tokenRefreshFailures != 0 {
		t.Fatalf("expected failures reset after success, got %d", client.tokenRefreshFailures)
	}
	if client.jwtExpiry.IsZero() {
		t.Fatalf("expected expiry set from token exp claim")
	}
	if client.jwtToken != token {
		t.Fatalf("expected stored token to match returned token")
	}
}

func TestTokenRefreshBackoffDoubles(t *testing.T) {
	// Always-failing server: each consecutive failure should double the wait.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"errors":[{"message":"Too many requests."}]}`)
	}))
	defer server.Close()

	client := NewOctopusClient("A-TEST", "sk_test", false)
	client.krakenTokenURL = server.URL
	base := 10 * time.Millisecond
	client.tokenBackoffBase = base
	client.tokenBackoffMax = 40 * time.Millisecond

	// Fail and check the backoff window grows: 10ms, 20ms, 40ms, 40ms(capped).
	var got []time.Duration
	for i := 0; i < 4; i++ {
		if err := client.refreshJWTToken(); err == nil {
			t.Fatalf("attempt %d: expected error, got nil", i)
		}
		got = append(got, time.Until(client.tokenBackoffUntil).Round(time.Millisecond))
		// Let the current backoff lapse so the next attempt actually retries.
		time.Sleep(got[i] + 5*time.Millisecond)
	}

	expected := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 40 * time.Millisecond, 40 * time.Millisecond}
	for i, want := range expected {
		if got[i] < want-2*time.Millisecond || got[i] > want+2*time.Millisecond {
			t.Errorf("attempt %d: expected backoff ~%v, got %v", i, want, got[i])
		}
	}
}

func TestJwtExpiryFromToken(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		wantExp int64
		wantErr bool
	}{
		{
			name:    "valid jwt with exp claim",
			token:   buildTestJWT(`{"exp":1700000000,"scope":"octopus"}`),
			wantExp: 1700000000,
		},
		{
			name:    "valid jwt with zero exp claim is returned as epoch",
			token:   buildTestJWT(`{"exp":0}`),
			wantExp: 0,
		},
		{
			name:    "missing exp claim",
			token:   buildTestJWT(`{"scope":"octopus"}`),
			wantErr: true,
		},
		{
			name:    "not a jwt (no dots)",
			token:   "just-a-plain-string",
			wantErr: true,
		},
		{
			name:    "invalid base64 payload",
			token:   "header.!!not-base64!!.signature",
			wantErr: true,
		},
		{
			name:    "invalid json payload",
			token:   buildTestJWT(`{not valid json`),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := jwtExpiryFromToken(tc.token)
			if (err != nil) != tc.wantErr {
				t.Fatalf("expected err=%v, got err=%v", tc.wantErr, err)
			}
			if tc.wantErr {
				return
			}
			if got.Unix() != tc.wantExp {
				t.Errorf("expected exp %d, got %d", tc.wantExp, got.Unix())
			}
		})
	}
}

func TestNewOctopusClient(t *testing.T) {
	accountID := "test-account"
	apiKey := "test-api-key"
	debug := true

	client := NewOctopusClient(accountID, apiKey, debug)

	if client.AccountID != accountID {
		t.Errorf("Expected AccountID %s, got %s", accountID, client.AccountID)
	}

	if client.APIKey != apiKey {
		t.Errorf("Expected APIKey %s, got %s", apiKey, client.APIKey)
	}

	if client.BaseURL != getEndpoint("api") {
		t.Errorf("Expected BaseURL %s, got %s", getEndpoint("api"), client.BaseURL)
	}

	if client.debug != debug {
		t.Errorf("Expected debug %v, got %v", debug, client.debug)
	}

	if client.minInterval != 1*time.Second {
		t.Errorf("Expected minInterval %v, got %v", 1*time.Second, client.minInterval)
	}

	if client.maxRetries != 3 {
		t.Errorf("Expected maxRetries %d, got %d", 3, client.maxRetries)
	}

	if client.client == nil {
		t.Error("Expected HTTP client to be initialized")
	}

	if client.client.Timeout != 30*time.Second {
		t.Errorf("Expected HTTP timeout %v, got %v", 30*time.Second, client.client.Timeout)
	}
}

func TestGetEndpoint(t *testing.T) {
	testCases := []struct {
		name     string
		key      string
		expected string
	}{
		{
			name:     "API endpoint",
			key:      "api",
			expected: "https://api.octopus.energy/v1",
		},
		{
			name:     "GraphQL endpoint",
			key:      "graphql",
			expected: "https://api.octopus.energy/v1/graphql/",
		},
		{
			name:     "Backend GraphQL endpoint",
			key:      "backend-graphql",
			expected: "https://api.backend.octopus.energy/v1/graphql/",
		},
		{
			name:     "Fallback for unknown key",
			key:      "unknown",
			expected: "https://api.octopus.energy/v1", // Should fallback to "api"
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getEndpoint(tc.key)
			if result != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestOctopusClientSetState(t *testing.T) {
	client := NewOctopusClient("test", "test", false)
	state := &AppState{
		JWTToken:       "test-jwt-token",
		JWTTokenExpiry: time.Now().Add(1 * time.Hour),
	}

	client.SetState(state)

	if client.state != state {
		t.Error("Expected state to be set on client")
	}

	if client.jwtToken != state.JWTToken {
		t.Errorf("Expected JWT token %s, got %s", state.JWTToken, client.jwtToken)
	}

	if client.jwtExpiry != state.JWTTokenExpiry {
		t.Errorf("Expected JWT expiry %v, got %v", state.JWTTokenExpiry, client.jwtExpiry)
	}
}

func TestInvalidateJWTToken(t *testing.T) {
	client := NewOctopusClient("test", "test", false)
	state := &AppState{
		JWTToken:       "test-jwt-token",
		JWTTokenExpiry: time.Now().Add(1 * time.Hour),
	}
	client.SetState(state)

	// Set some JWT data
	client.jwtToken = "some-token"
	client.jwtExpiry = time.Now().Add(1 * time.Hour)

	client.invalidateJWTToken()

	// Check client state is cleared
	if client.jwtToken != "" {
		t.Errorf("Expected empty JWT token, got %s", client.jwtToken)
	}

	if !client.jwtExpiry.IsZero() {
		t.Errorf("Expected zero JWT expiry, got %v", client.jwtExpiry)
	}

	// Check app state is cleared
	if state.JWTToken != "" {
		t.Errorf("Expected empty JWT token in state, got %s", state.JWTToken)
	}

	if !state.JWTTokenExpiry.IsZero() {
		t.Errorf("Expected zero JWT expiry in state, got %v", state.JWTTokenExpiry)
	}
}

func TestWheelOfFortuneSpins(t *testing.T) {
	spins := WheelOfFortuneSpins{
		ElectricitySpins: 3,
		GasSpins:        2,
	}

	if spins.ElectricitySpins != 3 {
		t.Errorf("Expected 3 electricity spins, got %d", spins.ElectricitySpins)
	}

	if spins.GasSpins != 2 {
		t.Errorf("Expected 2 gas spins, got %d", spins.GasSpins)
	}
}

func TestSavingSessionJSONSerialization(t *testing.T) {
	session := SavingSession{
		EventID:    6170,
		Code:       "EVENT_73_110926",
		StartAt:    time.Date(2026, 9, 11, 18, 0, 0, 0, time.UTC),
		EndAt:      time.Date(2026, 9, 11, 19, 0, 0, 0, time.UTC),
		OctoPoints: 76,
		Status:     "UPCOMING",
		Joined:     true,
	}

	data, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("Failed to marshal session: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"octoPoints":76`) {
		t.Errorf("Expected json to contain '\"octoPoints\":76', got: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"joined":true`) {
		t.Errorf("Expected json to contain '\"joined\":true', got: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"code":"EVENT_73_110926"`) {
		t.Errorf("Expected json to contain '\"code\":\"EVENT_73_110926\"', got: %s", jsonStr)
	}
}

func TestFreeElectricitySessionJSONSerialization(t *testing.T) {
	session := FreeElectricitySession{
		Code:    "EVENT_69_130926",
		StartAt: time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC),
		EndAt:   time.Date(2026, 9, 13, 11, 0, 0, 0, time.UTC),
		Joined:  true,
	}

	data, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("Failed to marshal free electricity session: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"joined":true`) {
		t.Errorf("Expected json to contain '\"joined\":true', got: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"code":"EVENT_69_130926"`) {
		t.Errorf("Expected json to contain '\"code\":\"EVENT_69_130926\"', got: %s", jsonStr)
	}
}