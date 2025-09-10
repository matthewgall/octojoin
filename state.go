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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type CachedSavingSessions struct {
	Data      *SavingSessionsResponse `json:"data"`
	Timestamp time.Time               `json:"timestamp"`
}

type CachedFreeElectricitySessions struct {
	Data      *FreeElectricitySessionsResponse `json:"data"`
	Timestamp time.Time                        `json:"timestamp"`
}

type AppState struct {
	AlertStates                map[string]*FreeElectricityAlertState `json:"alert_states"`
	KnownSessions             map[int]bool                          `json:"known_sessions"`
	KnownFreeElectricitySessions map[string]bool                     `json:"known_free_electricity_sessions"`
	CachedSavingSessions      *CachedSavingSessions                 `json:"cached_saving_sessions,omitempty"`
	CachedFreeElectricity     *CachedFreeElectricitySessions        `json:"cached_free_electricity,omitempty"`
	JWTToken                  string                                `json:"jwt_token,omitempty"`
	JWTTokenExpiry            time.Time                             `json:"jwt_token_expiry,omitempty"`
	LastUpdated               time.Time                             `json:"last_updated"`
}

func getStateFilePath(accountID string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	
	configDir := filepath.Join(homeDir, ".config", "octojoin")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create config directory: %w", err)
	}
	
	// Use account ID in filename to separate cache per account
	return filepath.Join(configDir, fmt.Sprintf("state_%s.json", accountID)), nil
}

func LoadState(accountID string) (*AppState, error) {
	statePath, err := getStateFilePath(accountID)
	if err != nil {
		return nil, err
	}
	
	// If file doesn't exist, return empty state
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		return &AppState{
			AlertStates:                  make(map[string]*FreeElectricityAlertState),
			KnownSessions:                make(map[int]bool),
			KnownFreeElectricitySessions: make(map[string]bool),
			LastUpdated:                  time.Now(),
		}, nil
	}
	
	data, err := os.ReadFile(statePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}
	
	var state AppState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}
	
	// Initialize maps if they're nil (for backward compatibility)
	if state.AlertStates == nil {
		state.AlertStates = make(map[string]*FreeElectricityAlertState)
	}
	if state.KnownSessions == nil {
		state.KnownSessions = make(map[int]bool)
	}
	if state.KnownFreeElectricitySessions == nil {
		state.KnownFreeElectricitySessions = make(map[string]bool)
	}
	
	return &state, nil
}

func (s *AppState) Save(accountID string) error {
	statePath, err := getStateFilePath(accountID)
	if err != nil {
		return err
	}
	
	s.LastUpdated = time.Now()
	
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}
	
	if err := os.WriteFile(statePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}
	
	return nil
}

func (s *AppState) IsCacheValid(cacheTime time.Time, maxAge time.Duration) bool {
	return time.Since(cacheTime) < maxAge
}

func (s *AppState) CleanupExpiredSessions() {
	// Clean up alert states for sessions that have ended
	for code := range s.AlertStates {
		// We need to check against actual session data, but for now
		// we'll clean up very old alert states (older than 7 days)
		if time.Since(s.LastUpdated) > 7*24*time.Hour {
			delete(s.AlertStates, code)
		}
	}
}