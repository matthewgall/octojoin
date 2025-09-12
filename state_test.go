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

func TestAppStateIsCacheValid(t *testing.T) {
	state := &AppState{}

	testCases := []struct {
		name      string
		timestamp time.Time
		duration  time.Duration
		expected  bool
	}{
		{
			name:      "Valid cache within duration",
			timestamp: time.Now().Add(-2 * time.Minute),
			duration:  5 * time.Minute,
			expected:  true,
		},
		{
			name:      "Invalid cache outside duration",
			timestamp: time.Now().Add(-10 * time.Minute),
			duration:  5 * time.Minute,
			expected:  false,
		},
		{
			name:      "Zero timestamp",
			timestamp: time.Time{},
			duration:  5 * time.Minute,
			expected:  false,
		},
		{
			name:      "Future timestamp",
			timestamp: time.Now().Add(1 * time.Hour),
			duration:  5 * time.Minute,
			expected:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := state.IsCacheValid(tc.timestamp, tc.duration)
			if result != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestLoadState(t *testing.T) {
	// Test loading non-existent state (should create new)
	accountID := "test-account-123"
	state, err := LoadState(accountID)
	if err != nil {
		t.Errorf("Expected no error for non-existent state, got %v", err)
	}
	if state == nil {
		t.Error("Expected new state to be created")
	}
	if len(state.KnownSessions) != 0 {
		t.Error("Expected empty KnownSessions map")
	}
	if len(state.AlertStates) != 0 {
		t.Error("Expected empty AlertStates map")
	}
	if len(state.KnownFreeElectricitySessions) != 0 {
		t.Error("Expected empty KnownFreeElectricitySessions map")
	}
}

func TestAppStateSave(t *testing.T) {
	accountID := "test-save-account"

	state := &AppState{
		AlertStates: make(map[string]*FreeElectricityAlertState),
		KnownSessions: map[int]bool{
			789: true,
		},
		KnownFreeElectricitySessions: make(map[string]bool),
		JWTToken:                     "save-test-token",
		JWTTokenExpiry:               time.Now().Add(2 * time.Hour),
	}

	// Test saving state
	err := state.Save(accountID)
	if err != nil {
		t.Errorf("Expected no error saving state, got %v", err)
	}

	// Load and verify content
	loadedState, err := LoadState(accountID)
	if err != nil {
		t.Errorf("Expected no error loading saved state, got %v", err)
	}

	if len(loadedState.KnownSessions) != 1 {
		t.Errorf("Expected 1 known session, got %d", len(loadedState.KnownSessions))
	}

	if !loadedState.KnownSessions[789] {
		t.Error("Expected session 789 to be true")
	}

	if loadedState.JWTToken != "save-test-token" {
		t.Errorf("Expected JWT token 'save-test-token', got %s", loadedState.JWTToken)
	}
}

func TestCachedSavingSessionsStruct(t *testing.T) {
	now := time.Now()
	testResponse := &SavingSessionsResponse{
		Data: struct {
			SavingSessions struct {
				Account struct {
					HasJoinedCampaign bool            `json:"hasJoinedCampaign"`
					JoinedEvents      []SavingSession `json:"joinedEvents"`
				} `json:"account"`
			} `json:"savingSessions"`
			OctoPoints struct {
				Account struct {
					CurrentPointsInWallet int `json:"currentPointsInWallet"`
				} `json:"account"`
			} `json:"octoPoints"`
		}{},
	}

	cached := CachedSavingSessions{
		Data:      testResponse,
		Timestamp: now,
	}

	if cached.Data != testResponse {
		t.Error("Expected cached data to match original response")
	}

	if cached.Timestamp != now {
		t.Errorf("Expected timestamp %v, got %v", now, cached.Timestamp)
	}
}