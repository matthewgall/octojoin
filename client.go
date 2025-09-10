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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

type OctopusClient struct {
	AccountID      string
	APIKey         string
	BaseURL        string
	client         *http.Client
	lastRequestTime time.Time
	minInterval     time.Duration
	maxRetries      int
	jwtToken       string
	jwtExpiry      time.Time
	debug          bool
	state          *AppState
}

type SavingSession struct {
	EventID    int       `json:"eventId"`
	StartAt    time.Time `json:"startAt"`
	EndAt      time.Time `json:"endAt"`
	OctoPoints int       `json:"octopoints"`
}

type FreeElectricitySession struct {
	Code    string    `json:"code"`
	StartAt time.Time `json:"start"`
	EndAt   time.Time `json:"end"`
}

type FreeElectricitySessionsResponse struct {
	Data []FreeElectricitySession `json:"data"`
}

type SavingSessionsResponse struct {
	Data struct {
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
	} `json:"data"`
}

func NewOctopusClient(accountID, apiKey string, debug bool) *OctopusClient {
	return &OctopusClient{
		AccountID:   accountID,
		APIKey:      apiKey,
		BaseURL:     "https://api.octopus.energy/v1",
		minInterval: 1 * time.Second,
		maxRetries:  3,
		debug:       debug,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *OctopusClient) SetState(state *AppState) {
	c.state = state
	c.loadJWTFromState()
}

func (c *OctopusClient) loadJWTFromState() {
	if c.state != nil && c.state.JWTToken != "" {
		c.jwtToken = c.state.JWTToken
		c.jwtExpiry = c.state.JWTTokenExpiry
		c.debugLog("Loaded cached JWT token, expires: %v", c.jwtExpiry)
	}
}

func (c *OctopusClient) saveJWTToState() {
	if c.state != nil {
		c.state.JWTToken = c.jwtToken
		c.state.JWTTokenExpiry = c.jwtExpiry
		c.debugLog("Saved JWT token to state, expires: %v", c.jwtExpiry)
	}
}

func (c *OctopusClient) debugLog(format string, args ...interface{}) {
	if c.debug {
		log.Printf("DEBUG: "+format, args...)
	}
}

func (c *OctopusClient) makeRequest(method, endpoint string, body interface{}) (*http.Response, error) {
	return c.makeRequestWithRetry(method, endpoint, body, 0)
}

func (c *OctopusClient) makeRequestWithRetry(method, endpoint string, body interface{}, attempt int) (*http.Response, error) {
	c.enforceRateLimit()
	
	var reqBody []byte
	var err error
	
	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
	}

	url := c.BaseURL + endpoint
	req, err := http.NewRequest(method, url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.APIKey, "")
	req.Header.Set("Content-Type", "application/json")

	c.lastRequestTime = time.Now()
	resp, err := c.client.Do(req)
	if err != nil {
		if attempt < c.maxRetries {
			backoff := c.calculateBackoff(attempt)
			log.Printf("Request failed (attempt %d/%d): %v. Retrying in %v...", attempt+1, c.maxRetries+1, err, backoff)
			time.Sleep(backoff)
			return c.makeRequestWithRetry(method, endpoint, body, attempt+1)
		}
		return nil, err
	}

	if c.shouldRetry(resp.StatusCode) && attempt < c.maxRetries {
		backoff := c.calculateBackoffFromResponse(resp, attempt)
		log.Printf("Received %d status (attempt %d/%d). Retrying in %v...", resp.StatusCode, attempt+1, c.maxRetries+1, backoff)
		resp.Body.Close()
		time.Sleep(backoff)
		return c.makeRequestWithRetry(method, endpoint, body, attempt+1)
	}

	return resp, nil
}

func (c *OctopusClient) enforceRateLimit() {
	if !c.lastRequestTime.IsZero() {
		elapsed := time.Since(c.lastRequestTime)
		if elapsed < c.minInterval {
			sleep := c.minInterval - elapsed
			log.Printf("Rate limiting: sleeping for %v", sleep)
			time.Sleep(sleep)
		}
	}
}

func (c *OctopusClient) shouldRetry(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests ||
		statusCode == http.StatusInternalServerError ||
		statusCode == http.StatusBadGateway ||
		statusCode == http.StatusServiceUnavailable ||
		statusCode == http.StatusGatewayTimeout
}

func (c *OctopusClient) calculateBackoff(attempt int) time.Duration {
	base := float64(time.Second)
	backoff := base * math.Pow(2, float64(attempt))
	jitter := rand.Float64() * 0.1 * backoff
	return time.Duration(backoff + jitter)
}

func (c *OctopusClient) calculateBackoffFromResponse(resp *http.Response, attempt int) time.Duration {
	if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
		if seconds, err := strconv.Atoi(retryAfter); err == nil {
			return time.Duration(seconds) * time.Second
		}
	}
	return c.calculateBackoff(attempt)
}

func (c *OctopusClient) GetSavingSessions() (*SavingSessionsResponse, error) {
	return c.GetSavingSessionsWithCache(nil)
}

func (c *OctopusClient) checkSavingSessionCampaign() bool {
	// Ensure we have a valid JWT token
	if err := c.refreshJWTToken(); err != nil {
		c.debugLog("Failed to get JWT token for campaign check: %v", err)
		return false
	}

	query := `query checkCampaigns($accountNumber: String!) {
		account(accountNumber: $accountNumber) {
			campaigns {
				slug
			}
		}
	}`

	requestBody := GraphQLRequest{
		Query: query,
		Variables: map[string]interface{}{
			"accountNumber": c.AccountID,
		},
	}

	reqBody, err := json.Marshal(requestBody)
	if err != nil {
		c.debugLog("Failed to marshal campaign request: %v", err)
		return false
	}

	req, err := http.NewRequest("POST", "https://api.octopus.energy/v1/graphql/", bytes.NewBuffer(reqBody))
	if err != nil {
		c.debugLog("Failed to create campaign request: %v", err)
		return false
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.jwtToken)

	resp, err := c.client.Do(req)
	if err != nil {
		c.debugLog("Failed to execute campaign request: %v", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.debugLog("Campaign request failed with status %d", resp.StatusCode)
		return false
	}

	var result struct {
		Data struct {
			Account struct {
				Campaigns []struct {
					Slug string `json:"slug"`
				} `json:"campaigns"`
			} `json:"account"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		c.debugLog("Failed to decode campaign response: %v", err)
		return false
	}

	// Check if enrolled in saving sessions campaign
	for _, campaign := range result.Data.Account.Campaigns {
		if campaign.Slug == "octoplus-saving-sessions" {
			c.debugLog("Found octoplus-saving-sessions campaign")
			return true
		}
	}

	c.debugLog("Not enrolled in octoplus-saving-sessions campaign")
	return false
}

func (c *OctopusClient) getCampaignStatus() (map[string]bool, error) {
	// Ensure we have a valid JWT token
	if err := c.refreshJWTToken(); err != nil {
		return nil, fmt.Errorf("failed to get JWT token for campaign check: %w", err)
	}

	query := `query checkCampaigns($accountNumber: String!) {
		account(accountNumber: $accountNumber) {
			campaigns {
				slug
			}
		}
	}`

	requestBody := GraphQLRequest{
		Query: query,
		Variables: map[string]interface{}{
			"accountNumber": c.AccountID,
		},
	}

	reqBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal campaign request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.octopus.energy/v1/graphql/", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create campaign request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.jwtToken)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute campaign request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("campaign request failed with status %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			Account struct {
				Campaigns []struct {
					Slug string `json:"slug"`
				} `json:"campaigns"`
			} `json:"account"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode campaign response: %w", err)
	}

	// Build campaign status map
	campaigns := make(map[string]bool)
	campaigns["octoplus"] = false
	campaigns["octoplus-saving-sessions"] = false
	campaigns["free_electricity"] = false

	for _, campaign := range result.Data.Account.Campaigns {
		if _, exists := campaigns[campaign.Slug]; exists {
			campaigns[campaign.Slug] = true
		}
	}

	return campaigns, nil
}

type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

func (c *OctopusClient) GetSavingSessionsWithCache(state *AppState) (*SavingSessionsResponse, error) {
	// Check cache if state is provided
	if state != nil && state.CachedSavingSessions != nil {
		if state.IsCacheValid(state.CachedSavingSessions.Timestamp, 5*time.Minute) {
			return state.CachedSavingSessions.Data, nil
		}
	}

	// Get saving sessions from REST API
	savingSessions, err := c.getSavingSessionsREST()
	if err != nil {
		return nil, err
	}

	// Get OctoPoints from GraphQL API
	c.debugLog("About to call getOctoPointsGraphQL()")
	points, err := c.getOctoPointsGraphQL()
	if err != nil {
		log.Printf("Warning: Failed to get OctoPoints: %v", err)
		points = 0 // Default to 0 if GraphQL fails
	}
	c.debugLog("getOctoPointsGraphQL() returned %d points", points)

	// Check campaign enrollment via GraphQL
	hasJoinedCampaign := c.checkSavingSessionCampaign()
	c.debugLog("Campaign enrollment status: %v", hasJoinedCampaign)

	// Combine the data
	result := &SavingSessionsResponse{
		Data: struct {
			SavingSessions struct {
				Account struct {
					HasJoinedCampaign bool             `json:"hasJoinedCampaign"`
					JoinedEvents      []SavingSession  `json:"joinedEvents"`
				} `json:"account"`
			} `json:"savingSessions"`
			OctoPoints struct {
				Account struct {
					CurrentPointsInWallet int `json:"currentPointsInWallet"`
				} `json:"account"`
			} `json:"octoPoints"`
		}{
			SavingSessions: struct {
				Account struct {
					HasJoinedCampaign bool             `json:"hasJoinedCampaign"`
					JoinedEvents      []SavingSession  `json:"joinedEvents"`
				} `json:"account"`
			}{
				Account: struct {
					HasJoinedCampaign bool             `json:"hasJoinedCampaign"`
					JoinedEvents      []SavingSession  `json:"joinedEvents"`
				}{
					HasJoinedCampaign: hasJoinedCampaign,
					JoinedEvents:      savingSessions.Data.SavingSessions.Account.JoinedEvents,
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
					CurrentPointsInWallet: points,
				},
			},
		},
	}

	// Update cache if state is provided
	if state != nil {
		state.CachedSavingSessions = &CachedSavingSessions{
			Data:      result,
			Timestamp: time.Now(),
		}
	}

	return result, nil
}

func (c *OctopusClient) getSavingSessionsREST() (*SavingSessionsResponse, error) {
	endpoint := fmt.Sprintf("/accounts/%s/", c.AccountID)
	
	resp, err := c.makeRequest("GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	var result SavingSessionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

func (c *OctopusClient) refreshJWTToken() error {
	// Check if token is still valid (with 5 minute buffer)
	if !c.jwtExpiry.IsZero() && time.Until(c.jwtExpiry) > 5*time.Minute {
		c.debugLog("JWT token still valid until %v", c.jwtExpiry)
		return nil // Token still valid
	}

	c.debugLog("Requesting new JWT token...")

	// JWT token request endpoint
	tokenURL := "https://api.octopus.energy/v1/graphql/"
	
	// Query to get JWT token using API key
	query := `mutation obtainKrakenToken($input: ObtainJSONWebTokenInput!) {
		obtainKrakenToken(input: $input) {
			token
			refreshToken
			refreshExpiresIn
		}
	}`

	requestBody := GraphQLRequest{
		Query: query,
		Variables: map[string]interface{}{
			"input": map[string]interface{}{
				"APIKey": c.APIKey,
			},
		},
	}

	reqBody, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal token request: %w", err)
	}

	c.debugLog("Token request body: %s", string(reqBody))

	req, err := http.NewRequest("POST", tokenURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute token request: %w", err)
	}
	defer resp.Body.Close()

	c.debugLog("Token request status: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		// Read body for error details
		bodyBytes, _ := io.ReadAll(resp.Body)
		c.debugLog("Token request failed body: %s", string(bodyBytes))
		return fmt.Errorf("token request failed with status %d", resp.StatusCode)
	}

	var tokenResult struct {
		Data struct {
			ObtainKrakenToken struct {
				Token            string `json:"token"`
				RefreshToken     string `json:"refreshToken"`
				RefreshExpiresIn int    `json:"refreshExpiresIn"`
			} `json:"obtainKrakenToken"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResult); err != nil {
		return fmt.Errorf("failed to decode token response: %w", err)
	}

	if len(tokenResult.Errors) > 0 {
		c.debugLog("GraphQL errors: %v", tokenResult.Errors)
		return fmt.Errorf("GraphQL errors: %s", tokenResult.Errors[0].Message)
	}

	if tokenResult.Data.ObtainKrakenToken.Token == "" {
		return fmt.Errorf("empty token received")
	}

	c.jwtToken = tokenResult.Data.ObtainKrakenToken.Token
	c.jwtExpiry = time.Now().Add(time.Duration(tokenResult.Data.ObtainKrakenToken.RefreshExpiresIn) * time.Second)

	c.debugLog("JWT token obtained successfully, expires: %v", c.jwtExpiry)
	
	// Save token to persistent state
	c.saveJWTToState()

	return nil
}

func (c *OctopusClient) getOctoPointsGraphQL() (int, error) {
	// Ensure we have a valid JWT token
	if err := c.refreshJWTToken(); err != nil {
		return 0, fmt.Errorf("failed to get JWT token: %w", err)
	}

	c.debugLog("Requesting OctoPoints with JWT token...")

	query := `query octoplusData($accountNumber: String!) {
		loyaltyPointLedgers {
			balanceCarriedForward
		}
		account(accountNumber: $accountNumber) {
			campaigns {
				slug
			}
		}
		octoplusAccountInfo(accountNumber: $accountNumber) {
			enrollmentStatus
		}
	}`

	requestBody := GraphQLRequest{
		Query: query,
		Variables: map[string]interface{}{
			"accountNumber": c.AccountID,
		},
	}

	reqBody, err := json.Marshal(requestBody)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal request body: %w", err)
	}

	c.debugLog("OctoPoints request body: %s", string(reqBody))

	req, err := http.NewRequest("POST", "https://api.octopus.energy/v1/graphql/", bytes.NewBuffer(reqBody))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.jwtToken)

	c.debugLog("Using JWT token: %s...", c.jwtToken[:20])

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	c.debugLog("OctoPoints request status: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		c.debugLog("OctoPoints request failed body: %s", string(bodyBytes))
		return 0, fmt.Errorf("GraphQL request failed with status %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			LoyaltyPointLedgers []struct {
				BalanceCarriedForward string `json:"balanceCarriedForward"`
			} `json:"loyaltyPointLedgers"`
			Account struct {
				Campaigns []struct {
					Slug string `json:"slug"`
				} `json:"campaigns"`
			} `json:"account"`
			OctoplusAccountInfo struct {
				EnrollmentStatus string `json:"enrollmentStatus"`
			} `json:"octoplusAccountInfo"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	c.debugLog("OctoPoints response: %s", string(bodyBytes))

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return 0, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Errors) > 0 {
		c.debugLog("OctoPoints GraphQL errors: %v", result.Errors)
		return 0, fmt.Errorf("GraphQL errors: %s", result.Errors[0].Message)
	}

	// Debug campaign information
	c.debugLog("Enrollment status: %s", result.Data.OctoplusAccountInfo.EnrollmentStatus)
	c.debugLog("Campaigns:")
	for _, campaign := range result.Data.Account.Campaigns {
		c.debugLog("  - %s", campaign.Slug)
	}

	// Get the current balance (should be the first/latest entry)
	if len(result.Data.LoyaltyPointLedgers) > 0 {
		pointsStr := result.Data.LoyaltyPointLedgers[0].BalanceCarriedForward
		points, err := strconv.Atoi(pointsStr)
		if err != nil {
			c.debugLog("Failed to convert points string '%s' to int: %v", pointsStr, err)
			return 0, fmt.Errorf("failed to convert points to integer: %w", err)
		}
		c.debugLog("Found %d OctoPoints", points)
		return points, nil
	}

	c.debugLog("No OctoPoints data found")
	return 0, nil // No points data available
}

func (c *OctopusClient) GetFreeElectricitySessions() (*FreeElectricitySessionsResponse, error) {
	return c.GetFreeElectricitySessionsWithCache(nil)
}

func (c *OctopusClient) GetFreeElectricitySessionsWithCache(state *AppState) (*FreeElectricitySessionsResponse, error) {
	// Check cache if state is provided
	if state != nil && state.CachedFreeElectricity != nil {
		if state.IsCacheValid(state.CachedFreeElectricity.Timestamp, 10*time.Minute) {
			return state.CachedFreeElectricity.Data, nil
		}
	}
	// Free electricity sessions are available through a third-party API
	url := "https://oe-api.davidskendall.co.uk/free_electricity.json"
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	var result FreeElectricitySessionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Update cache if state is provided
	if state != nil {
		state.CachedFreeElectricity = &CachedFreeElectricitySessions{
			Data:      &result,
			Timestamp: time.Now(),
		}
	}

	return &result, nil
}

func (c *OctopusClient) JoinSavingSession(eventID int) error {
	endpoint := fmt.Sprintf("/accounts/%s/saving-sessions/%d/join", c.AccountID, eventID)
	
	resp, err := c.makeRequest("POST", endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to join saving session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to join saving session, status: %d", resp.StatusCode)
	}

	return nil
}