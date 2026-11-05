// Copyright 2026 Matthew Gall
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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandleSessionsAPI(t *testing.T) {
	now := time.Now()

	client := NewOctopusClient("A-TEST1234", "sk_test_12345", false)
	state := &AppState{
		KnownSessions: make(map[int]bool),
		CachedSavingSessions: &CachedSavingSessions{
			Timestamp: now,
			Data: &SavingSessionsResponse{
				Data: struct {
					SavingSessions struct {
						Account struct {
							HasJoinedCampaign bool            `json:"hasJoinedCampaign"`
							JoinedEvents      []SavingSession `json:"joinedEvents"`
						} `json:"account"`
						Events []SavingSessionEvent `json:"events"`
					} `json:"savingSessions"`
					OctoPoints struct {
						Account struct {
							CurrentPointsInWallet int `json:"currentPointsInWallet"`
						} `json:"account"`
					} `json:"octoPoints"`
				}{
					SavingSessions: struct {
						Account struct {
							HasJoinedCampaign bool            `json:"hasJoinedCampaign"`
							JoinedEvents      []SavingSession `json:"joinedEvents"`
						} `json:"account"`
						Events []SavingSessionEvent `json:"events"`
					}{
						Account: struct {
							HasJoinedCampaign bool            `json:"hasJoinedCampaign"`
							JoinedEvents      []SavingSession `json:"joinedEvents"`
						}{
							HasJoinedCampaign: true,
							JoinedEvents: []SavingSession{
								{
									EventID:    101,
									Code:       "EVENT_JOINED_HAPPY_HOUR",
									StartAt:    now.Add(1 * time.Hour),
									EndAt:      now.Add(2 * time.Hour),
									OctoPoints: 0,
									Joined:     true,
								},
							},
						},
						Events: []SavingSessionEvent{
							{
								ID:                       101,
								Code:                     "EVENT_JOINED_HAPPY_HOUR",
								StartAt:                  now.Add(1 * time.Hour),
								EndAt:                    now.Add(2 * time.Hour),
								Status:                   "UPCOMING",
								EventType:                "WEEKEND_HAPPY_HOUR",
								RewardPerKwhInOctoPoints: 0,
							},
							{
								ID:                       102,
								Code:                     "EVENT_UNJOINED_HAPPY_HOUR",
								StartAt:                  now.Add(2 * time.Hour),
								EndAt:                    now.Add(3 * time.Hour),
								Status:                   "UPCOMING",
								EventType:                "WEEKEND_HAPPY_HOUR",
								RewardPerKwhInOctoPoints: 0,
							},
							{
								ID:                       103,
								Code:                     "EVENT_SAVING_SESSION_PEAK",
								StartAt:                  now.Add(4 * time.Hour),
								EndAt:                    now.Add(5 * time.Hour),
								Status:                   "UPCOMING",
								EventType:                "TURN_DOWN",
								RewardPerKwhInOctoPoints: 75,
							},
						},
					},
					OctoPoints: struct {
						Account struct {
							CurrentPointsInWallet int `json:"currentPointsInWallet"`
						} `json:"account"`
					}{
						Account: struct {
							CurrentPointsInWallet int `json:"currentPointsInWallet"`
						}{
							CurrentPointsInWallet: 850,
						},
					},
				},
			},
		},
		CachedFreeElectricity: &CachedFreeElectricitySessions{
			Timestamp: now,
			Data: &FreeElectricitySessionsResponse{
				Data: []FreeElectricitySession{
					{
						Code:    "EVENT_COMMUNITY_FREE_1",
						StartAt: now.Add(6 * time.Hour),
						EndAt:   now.Add(7 * time.Hour),
					},
					// Duplicate of EVENT_UNJOINED_HAPPY_HOUR to verify deduplication
					{
						Code:    "EVENT_UNJOINED_HAPPY_HOUR",
						StartAt: now.Add(2 * time.Hour),
						EndAt:   now.Add(3 * time.Hour),
					},
				},
			},
		},
		CachedCampaignStatus: &CachedCampaignStatus{
			Timestamp: now,
			Data: map[string]bool{
				"octoplus":                 true,
				"octoplus-saving-sessions": true,
				"free_electricity":         false, // Should be overridden to true due to upcoming free sessions
			},
		},
		CachedAccountInfo: &CachedAccountInfo{
			Timestamp: now,
			Data: &AccountInfo{
				Balance: 42.50,
			},
		},
		CachedWheelOfFortuneSpins: &CachedWheelOfFortuneSpins{
			Timestamp: now,
			Data: &WheelOfFortuneSpins{
				ElectricitySpins: 1,
				GasSpins:         0,
			},
		},
	}

	monitor := &SavingSessionMonitor{
		client: client,
		state:  state,
		logger: NewLogger(false),
	}

	ws := &WebServer{
		monitor: monitor,
		logger:  NewLogger(false),
	}

	req := httptest.NewRequest("GET", "/api/sessions", nil)
	rr := httptest.NewRecorder()

	ws.handleSessionsAPI(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rr.Code)
	}

	var resp SessionData
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to decode response JSON: %v", err)
	}

	// 1. Verify SavingSessions: only EVENT_SAVING_SESSION_PEAK should be here
	if len(resp.SavingSessions) != 1 {
		t.Fatalf("Expected 1 saving session, got %d", len(resp.SavingSessions))
	}
	if resp.SavingSessions[0].Code != "EVENT_SAVING_SESSION_PEAK" {
		t.Errorf("Expected saving session code EVENT_SAVING_SESSION_PEAK, got %s", resp.SavingSessions[0].Code)
	}
	if resp.SavingSessions[0].OctoPoints != 75 {
		t.Errorf("Expected saving session points 75, got %d", resp.SavingSessions[0].OctoPoints)
	}

	// 2. Verify FreeElectricitySessions: should have 3 distinct sessions (joined happy hour, unjoined happy hour, community session)
	if len(resp.FreeElectricitySessions) != 3 {
		t.Fatalf("Expected 3 free electricity sessions after deduplication, got %d", len(resp.FreeElectricitySessions))
	}

	// Check joined session
	joinedFound := false
	unjoinedFound := false
	communityFound := false

	for _, s := range resp.FreeElectricitySessions {
		if s.Code == "EVENT_JOINED_HAPPY_HOUR" {
			joinedFound = true
			if !s.Joined {
				t.Errorf("Expected EVENT_JOINED_HAPPY_HOUR to have Joined=true")
			}
		}
		if s.Code == "EVENT_UNJOINED_HAPPY_HOUR" {
			unjoinedFound = true
			if s.Joined {
				t.Errorf("Expected EVENT_UNJOINED_HAPPY_HOUR to have Joined=false")
			}
		}
		if s.Code == "EVENT_COMMUNITY_FREE_1" {
			communityFound = true
		}
	}

	if !joinedFound {
		t.Errorf("Expected EVENT_JOINED_HAPPY_HOUR in free electricity sessions")
	}
	if !unjoinedFound {
		t.Errorf("Expected EVENT_UNJOINED_HAPPY_HOUR in free electricity sessions")
	}
	if !communityFound {
		t.Errorf("Expected EVENT_COMMUNITY_FREE_1 in free electricity sessions")
	}

	// 3. Verify CampaignStatus: FreeElectricity should be enabled dynamically
	if !resp.CampaignStatus.HasFreeElectricity {
		t.Errorf("Expected HasFreeElectricity to be true")
	}
	if !resp.CampaignStatus.FreeElectricityEnabled {
		t.Errorf("Expected FreeElectricityEnabled to be true")
	}

	// 4. Verify points
	if resp.CurrentPoints != 850 {
		t.Errorf("Expected CurrentPoints 850, got %d", resp.CurrentPoints)
	}
}

func TestHandleHealth(t *testing.T) {
	client := NewOctopusClient("A-TEST1234", "sk_test_12345", false)

	monitor := &SavingSessionMonitor{
		client: client,
		state:  &AppState{},
		logger: NewLogger(false),
	}

	ws := &WebServer{
		monitor:   monitor,
		logger:    NewLogger(false),
		startTime: time.Now().Add(-5 * time.Second),
	}

	req := httptest.NewRequest("GET", "/api/health", nil)
	rr := httptest.NewRecorder()

	ws.handleHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rr.Code)
	}

	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}

	var resp HealthResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to decode health response JSON: %v", err)
	}

	if resp.Status != "healthy" {
		t.Errorf("Expected status healthy, got %s", resp.Status)
	}

	if resp.Version == "" {
		t.Errorf("Expected non-empty version")
	}

	if resp.Timestamp == 0 {
		t.Errorf("Expected non-zero timestamp")
	}

	if resp.UptimeSeconds < 4 || resp.UptimeSeconds > 10 {
		t.Errorf("Expected uptime around 5s, got %f", resp.UptimeSeconds)
	}
}
