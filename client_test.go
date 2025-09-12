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
	"testing"
	"time"
)

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