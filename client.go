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
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// API endpoints
var octopusEndpoints = map[string]string{
	"api":             "https://api.octopus.energy/v1",
	"graphql":         "https://api.octopus.energy/v1/graphql/",
	"backend-graphql": "https://api.backend.octopus.energy/v1/graphql/",
}

// Helper function to get endpoint URLs
func getEndpoint(key string) string {
	if url, exists := octopusEndpoints[key]; exists {
		return url
	}
	// Fallback to main API if key doesn't exist
	return octopusEndpoints["api"]
}

// APIMetrics tracks API call performance and rate limiting
type APIMetrics struct {
	// API call durations by endpoint
	RequestDurations map[string][]float64 // endpoint -> list of durations in seconds

	// Rate limiting metrics
	TotalRequests     int64   // Total number of API requests
	RateLimitSleeps   int64   // Number of times rate limiting was triggered
	TotalSleepSeconds float64 // Total time spent sleeping due to rate limits
}

// NewAPIMetrics creates a new metrics tracker
func NewAPIMetrics() *APIMetrics {
	return &APIMetrics{
		RequestDurations: make(map[string][]float64),
	}
}

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
	logger         *Logger
	metrics        *APIMetrics
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

type WheelOfFortuneSpins struct {
	ElectricitySpins int `json:"electricity_spins"`
	GasSpins        int `json:"gas_spins"`
}

type WheelSpinResult struct {
	Prize    int    `json:"prize"`     // OctoPoints earned from the spin
	FuelType string `json:"fuel_type"` // "ELECTRICITY" or "GAS"
}

type WheelSpinResponse struct {
	Data struct {
		SpinWheelOfFortune struct {
			Prize struct {
				Value int `json:"value"`
			} `json:"prize"`
		} `json:"spinWheelOfFortune"`
	} `json:"data"`
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

type SmartDevice struct {
	DeviceID string `json:"deviceId"`
	Type     string `json:"type"`
}

type SmartDeviceNetwork struct {
	SmartDevices []SmartDevice `json:"smartDevices"`
}

type MeterEligibilityResponse struct {
	Data struct {
		Account struct {
			Properties []struct {
				ID                    string               `json:"id"`
				Address               string               `json:"address"`
				SmartDeviceNetworks   []SmartDeviceNetwork `json:"smartDeviceNetworks"`
			} `json:"properties"`
		} `json:"account"`
	} `json:"data"`
}

type UsageMeasurement struct {
	Value    string    `json:"value"` // API returns this as string, we'll parse it
	Unit     string    `json:"unit"`
	StartAt  time.Time `json:"startAt"`
	EndAt    time.Time `json:"endAt"`
	Duration int       `json:"durationInSeconds"`
	MetaData struct {
		Statistics []struct {
			CostInclTax struct {
				EstimatedAmount string `json:"estimatedAmount"` // API returns as string
				CostCurrency    string `json:"costCurrency"`
			} `json:"costInclTax"`
			CostExclTax struct {
				PricePerUnit struct {
					Amount string `json:"amount"` // API returns as string
				} `json:"pricePerUnit"`
				EstimatedAmount string `json:"estimatedAmount"` // API returns as string
				CostCurrency    string `json:"costCurrency"`
			} `json:"costExclTax"`
			Value       string `json:"value"`       // API returns as string
			Description string `json:"description"`
			Label       string `json:"label"`
			Type        string `json:"type"`
		} `json:"statistics"`
	} `json:"metaData"`
}

type UsageMeasurementsResponse struct {
	Data struct {
		Account struct {
			Properties []struct {
				ID           string `json:"id"`
				Measurements struct {
					Edges []struct {
						Node UsageMeasurement `json:"node"`
					} `json:"edges"`
					PageInfo struct {
						HasNextPage     bool   `json:"hasNextPage"`
						HasPreviousPage bool   `json:"hasPreviousPage"`
						StartCursor     string `json:"startCursor"`
						EndCursor       string `json:"endCursor"`
					} `json:"pageInfo"`
				} `json:"measurements"`
			} `json:"properties"`
		} `json:"account"`
	} `json:"data"`
}

func NewOctopusClient(accountID, apiKey string, debug bool) *OctopusClient {
	logger := NewLogger(debug).WithComponent("octopus_client")
	return &OctopusClient{
		AccountID:   accountID,
		APIKey:      apiKey,
		BaseURL:     getEndpoint("api"),
		minInterval: HTTPMinInterval,
		maxRetries:  HTTPMaxRetries,
		debug:       debug,
		logger:      logger,
		metrics:     NewAPIMetrics(),
		client: &http.Client{
			Timeout: HTTPClientTimeout,
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

func (c *OctopusClient) invalidateJWTToken() {
	c.debugLog("Invalidating expired JWT token")
	c.jwtToken = ""
	c.jwtExpiry = time.Time{}
	if c.state != nil {
		c.state.JWTToken = ""
		c.state.JWTTokenExpiry = time.Time{}
	}
}

func (c *OctopusClient) makeGraphQLRequest(query string, variables map[string]interface{}, retryOnAuth bool) (*http.Response, error) {
	return c.makeGraphQLRequestWithEndpoint(getEndpoint("graphql"), query, variables, retryOnAuth, "")
}

func (c *OctopusClient) makeGraphQLRequestWithEndpoint(endpoint, query string, variables map[string]interface{}, retryOnAuth bool, operationName string) (*http.Response, error) {
	if err := c.refreshJWTToken(); err != nil {
		return nil, fmt.Errorf("failed to get JWT token: %w", err)
	}

	requestBody := GraphQLRequest{
		Query:         query,
		Variables:     variables,
		OperationName: operationName,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.jwtToken)
	req.Header.Set("User-Agent", GetUserAgent())

	// Log GraphQL request details in debug mode
	c.debugLogRequest("POST", endpoint, req.Header, bodyBytes)

	startTime := time.Now()
	resp, err := c.client.Do(req)
	duration := time.Since(startTime).Seconds()
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	// Log GraphQL response in debug mode (before any body reading)
	if c.debug {
		// We need to be careful here since we might read the body later for error checking
		// So we'll only log a preview without consuming the body yet
		bodyBytes, err := io.ReadAll(resp.Body)
		if err == nil {
			resp.Body.Close()
			c.debugLogResponse(resp, bodyBytes, duration)
			// Restore the body
			resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}
	}

	// Check for authentication errors that indicate token expiration
	if (resp.StatusCode == 401 || resp.StatusCode == 403) && retryOnAuth {
		resp.Body.Close()
		c.debugLog("Got %d response, JWT token may be expired. Invalidating and retrying...", resp.StatusCode)
		c.invalidateJWTToken()
		
		// Retry once with fresh token
		return c.makeGraphQLRequestWithEndpoint(endpoint, query, variables, false, operationName)
	}

	// For GraphQL, we also need to check for JWT expiration in the response body
	if resp.StatusCode == 200 && retryOnAuth {
		// Read the response body to check for JWT expiration errors
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return resp, nil // Return original response if we can't read body
		}
		resp.Body.Close()

		// Check if the response contains JWT expiration error
		bodyStr := string(bodyBytes)
		if strings.Contains(bodyStr, "Signature of the JWT has expired") ||
		   strings.Contains(bodyStr, "JWT has expired") ||
		   strings.Contains(bodyStr, "Token has expired") ||
		   strings.Contains(bodyStr, OctopusErrorCodeJWTExpired) || // Octopus specific auth error code
		   strings.Contains(bodyStr, OctopusErrorCodeInvalidAuth) || // Invalid authorization header error
		   strings.Contains(bodyStr, "Authentication failed") {
			c.debugLog("GraphQL response contains JWT expiration/auth error. Invalidating token and retrying...")
			c.debugLog("Error details: %s", bodyStr)
			c.invalidateJWTToken()
			
			// Retry once with fresh token
			return c.makeGraphQLRequestWithEndpoint(endpoint, query, variables, false, operationName)
		}

		// Create new response with the body we read
		resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	return resp, nil
}

func (c *OctopusClient) debugLog(format string, args ...interface{}) {
	if c.debug {
		c.logger.Debug(format, args...)
	}
}

// debugLogRequest logs detailed request information in debug mode
func (c *OctopusClient) debugLogRequest(method, url string, headers http.Header, bodyBytes []byte) {
	if !c.debug {
		return
	}

	// Mask sensitive headers
	maskedHeaders := make(map[string]string)
	for key, values := range headers {
		if len(values) > 0 {
			if key == "Authorization" {
				// Show only first and last 4 chars of auth tokens
				val := values[0]
				if len(val) > 12 {
					maskedHeaders[key] = val[:6] + "..." + val[len(val)-4:]
				} else {
					maskedHeaders[key] = "***"
				}
			} else {
				maskedHeaders[key] = values[0]
			}
		}
	}

	c.logger.Debug("→ HTTP Request",
		"method", method,
		"url", url,
		"headers", maskedHeaders,
	)

	if len(bodyBytes) > 0 {
		bodyStr := string(bodyBytes)
		// Truncate long bodies
		if len(bodyStr) > 500 {
			bodyStr = bodyStr[:500] + "... (truncated)"
		}
		c.logger.Debug("  Request Body", "body", bodyStr)
	}
}

// debugLogResponse logs detailed response information in debug mode
func (c *OctopusClient) debugLogResponse(resp *http.Response, bodyPreview []byte, duration float64) {
	if !c.debug {
		return
	}

	c.logger.Debug("← HTTP Response",
		"status", resp.StatusCode,
		"status_text", resp.Status,
		"duration_ms", duration*1000,
		"content_type", resp.Header.Get("Content-Type"),
	)

	if len(bodyPreview) > 0 {
		bodyStr := string(bodyPreview)
		// Truncate long response bodies
		if len(bodyStr) > 500 {
			bodyStr = bodyStr[:500] + "... (truncated)"
		}
		c.logger.Debug("  Response Body", "body", bodyStr)
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
	req.Header.Set("User-Agent", GetUserAgent())

	// Log request details in debug mode
	c.debugLogRequest(method, url, req.Header, reqBody)

	startTime := time.Now()
	c.lastRequestTime = startTime
	resp, err := c.client.Do(req)
	duration := time.Since(startTime).Seconds()

	// Track total requests (including failed ones)
	c.metrics.TotalRequests++

	if err != nil {
		if attempt < c.maxRetries {
			backoff := c.calculateBackoff(attempt)
			c.logger.Warn("Request failed, retrying",
				"method", method,
				"endpoint", endpoint,
				"attempt", attempt+1,
				"max_attempts", c.maxRetries+1,
				"backoff_ms", backoff.Milliseconds(),
				"error", err.Error(),
			)
			time.Sleep(backoff)
			return c.makeRequestWithRetry(method, endpoint, body, attempt+1)
		}
		return nil, NewAPIError(0, endpoint, "request failed", err)
	}

	c.logger.LogAPIRequest(method, endpoint, resp.StatusCode, duration)

	// Track API call duration by endpoint
	c.metrics.RequestDurations[endpoint] = append(c.metrics.RequestDurations[endpoint], duration)

	// Log response details in debug mode (read preview without consuming body)
	if c.debug {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err == nil {
			resp.Body.Close()
			c.debugLogResponse(resp, bodyBytes, duration)
			// Restore the response body for the caller
			resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}
	}

	if c.shouldRetry(resp.StatusCode) && attempt < c.maxRetries {
		backoff := c.calculateBackoffFromResponse(resp, attempt)
		c.logger.Warn("Retrying due to status code",
			"status_code", resp.StatusCode,
			"attempt", attempt+1,
			"max_attempts", c.maxRetries+1,
			"backoff_ms", backoff.Milliseconds(),
		)
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
			c.logger.Debug("Rate limiting",
				"sleep_ms", sleep.Milliseconds(),
			)

			// Track rate limiting metrics
			c.metrics.RateLimitSleeps++
			c.metrics.TotalSleepSeconds += sleep.Seconds()

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

func (c *OctopusClient) getCampaignStatusWithCache(state *AppState) (map[string]bool, error) {
	// Check cache if state is provided - campaign status rarely changes
	if state != nil && state.CachedCampaignStatus != nil {
		if state.IsCacheValid(state.CachedCampaignStatus.Timestamp, CacheDurationCampaignStatus) {
			return state.CachedCampaignStatus.Data, nil
		}
	}

	// Get fresh campaign data
	campaigns, err := c.getCampaignStatus()
	if err != nil {
		return nil, err
	}

	// Update cache if state is provided
	if state != nil {
		state.CachedCampaignStatus = &CachedCampaignStatus{
			Data:      campaigns,
			Timestamp: time.Now(),
		}
	}

	return campaigns, nil
}

func (c *OctopusClient) getCampaignStatus() (map[string]bool, error) {
	query := `query checkCampaigns($accountNumber: String!) {
		account(accountNumber: $accountNumber) {
			campaigns {
				slug
			}
		}
	}`

	variables := map[string]interface{}{
		"accountNumber": c.AccountID,
	}

	resp, err := c.makeGraphQLRequest(query, variables, true)
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
	OperationName string                 `json:"operationName,omitempty"`
	Query         string                 `json:"query"`
	Variables     map[string]interface{} `json:"variables"`
}

func (c *OctopusClient) GetSavingSessionsWithCache(state *AppState) (*SavingSessionsResponse, error) {
	// Dynamic cache duration based on UK business hours for faster session detection
	cacheDuration := CacheDurationSavingSessionsOffPeak // Default: off-peak

	// Load UK timezone for smart caching
	ukLocation, err := time.LoadLocation("Europe/London")
	if err != nil {
		ukLocation = time.UTC
	}

	now := time.Now().In(ukLocation)
	hour := now.Hour()
	weekday := now.Weekday()

	// Peak announcement window (2-4 PM UK time, weekdays) - use 10 minute cache
	if hour >= UKPeakAnnouncementStartHour && hour < UKPeakAnnouncementEndHour && weekday >= time.Monday && weekday <= time.Friday {
		cacheDuration = CacheDurationSavingSessionsPeak
	} else if hour >= UKBusinessHoursStartHour && hour < UKBusinessHoursEndHour && weekday >= time.Monday && weekday <= time.Friday {
		// Business hours - 30 minute cache
		cacheDuration = CacheDurationSavingSessionsBusiness
	}

	// Check cache if state is provided
	if state != nil && state.CachedSavingSessions != nil {
		if state.IsCacheValid(state.CachedSavingSessions.Timestamp, cacheDuration) {
			return state.CachedSavingSessions.Data, nil
		}
	}

	// Get saving sessions from REST API
	savingSessions, err := c.getSavingSessionsREST()
	if err != nil {
		return nil, err
	}

	// Get OctoPoints from GraphQL API (with caching)
	c.debugLog("About to call getOctoPointsGraphQLWithCache()")
	points, err := c.getOctoPointsGraphQLWithCache(state)
	if err != nil {
		c.logger.Warn("Failed to get OctoPoints", "error", err)
		points = 0 // Default to 0 if GraphQL fails
	}
	c.debugLog("getOctoPointsGraphQLWithCache() returned %d points", points)

	// Get campaign enrollment status via GraphQL (with caching)
	campaigns, err := c.getCampaignStatusWithCache(state)
	var hasJoinedCampaign bool
	if err != nil {
		c.logger.Warn("Failed to get campaign status", "error", err)
		hasJoinedCampaign = false // Default to false if GraphQL fails
	} else {
		hasJoinedCampaign = campaigns["octoplus-saving-sessions"]
	}
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
	// Check if token is still valid (with buffer before expiry)
	if !c.jwtExpiry.IsZero() && time.Until(c.jwtExpiry) > JWTRefreshBuffer {
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
	req.Header.Set("User-Agent", GetUserAgent())

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

func (c *OctopusClient) getOctoPointsGraphQLWithCache(state *AppState) (int, error) {
	// Check cache if state is provided - OctoPoints change at most hourly
	if state != nil && state.CachedOctoPoints != nil {
		if state.IsCacheValid(state.CachedOctoPoints.Timestamp, CacheDurationOctoPoints) {
			return state.CachedOctoPoints.Data, nil
		}
	}

	// Get fresh OctoPoints data
	points, err := c.getOctoPointsGraphQL()
	if err != nil {
		return 0, err
	}

	// Update cache if state is provided
	if state != nil {
		state.CachedOctoPoints = &CachedOctoPoints{
			Data:      points,
			Timestamp: time.Now(),
		}
	}

	return points, nil
}

func (c *OctopusClient) getOctoPointsGraphQL() (int, error) {
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

	variables := map[string]interface{}{
		"accountNumber": c.AccountID,
	}

	resp, err := c.makeGraphQLRequest(query, variables, true)
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
	// Check cache if state is provided - static file with no rate limits, check frequently
	if state != nil && state.CachedFreeElectricity != nil {
		if state.IsCacheValid(state.CachedFreeElectricity.Timestamp, CacheDurationFreeElectricity) {
			return state.CachedFreeElectricity.Data, nil
		}
	}
	// Free electricity sessions with fallback endpoints for reliability
	urls := []string{
		"https://matthewgall.github.io/octoevents/free_electricity.json",           // Primary: GitHub Pages (fastest)
		"https://raw.githubusercontent.com/matthewgall/octoevents/refs/heads/main/free_electricity.json", // Fallback 1: GitHub Raw
		"https://oe-api.davidskendall.co.uk/free_electricity.json",                // Fallback 2: David's API
	}
	
	var lastErr error
	for i, url := range urls {
		c.debugLog("Trying free electricity endpoint %d: %s", i+1, url)
		
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			lastErr = fmt.Errorf("failed to create request for %s: %w", url, err)
			continue
		}
		
		req.Header.Set("User-Agent", GetUserAgent())

		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("failed to make request to %s: %w", url, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("API request to %s failed with status %d", url, resp.StatusCode)
			continue
		}

		var result FreeElectricitySessionsResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			lastErr = fmt.Errorf("failed to decode response from %s: %w", url, err)
			continue
		}
		
		c.debugLog("Successfully retrieved free electricity sessions from endpoint %d", i+1)
		
		// Update cache if state is provided
		if state != nil {
			state.CachedFreeElectricity = &CachedFreeElectricitySessions{
				Data:      &result,
				Timestamp: time.Now(),
			}
		}

		return &result, nil
	}
	
	// If all endpoints failed, return the last error
	return nil, fmt.Errorf("all free electricity endpoints failed, last error: %w", lastErr)
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

func (c *OctopusClient) getWheelOfFortuneSpinsWithCache(state *AppState) (*WheelOfFortuneSpins, error) {
	// Check cache if state is provided - Wheel of Fortune spins update once daily
	if state != nil && state.CachedWheelOfFortuneSpins != nil {
		if state.IsCacheValid(state.CachedWheelOfFortuneSpins.Timestamp, CacheDurationWheelSpins) {
			return state.CachedWheelOfFortuneSpins.Data, nil
		}
	}

	// Get fresh Wheel of Fortune data
	spins, err := c.getWheelOfFortuneSpins()
	if err != nil {
		return nil, err
	}

	// Update cache if state is provided
	if state != nil {
		state.CachedWheelOfFortuneSpins = &CachedWheelOfFortuneSpins{
			Data:      spins,
			Timestamp: time.Now(),
		}
	}

	return spins, nil
}

func (c *OctopusClient) getWheelOfFortuneSpins() (*WheelOfFortuneSpins, error) {
	c.debugLog("Requesting Wheel of Fortune spins...")

	query := `query getWheelOfFortuneSpinsAllowed($accountNumber: String!) {
		gasSpins: wheelOfFortuneSpinsAllowed(
			accountNumber: $accountNumber
			fuelType: GAS
		) {
			spinsAllowed
			__typename
		}
		electricitySpins: wheelOfFortuneSpinsAllowed(
			accountNumber: $accountNumber
			fuelType: ELECTRICITY
		) {
			spinsAllowed
			__typename
		}
	}`

	variables := map[string]interface{}{
		"accountNumber": c.AccountID,
	}

	// Use the backend endpoint for Wheel of Fortune with full JWT retry logic
	resp, err := c.makeGraphQLRequestWithEndpoint(getEndpoint("backend-graphql"), query, variables, true, "getWheelOfFortuneSpinsAllowed")
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	c.debugLog("Wheel of Fortune request status: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		c.debugLog("Wheel of Fortune request failed body: %s", string(bodyBytes))
		c.logger.Error("Wheel of Fortune GraphQL error",
			"status_code", resp.StatusCode,
			"body", string(bodyBytes))
		return nil, fmt.Errorf("GraphQL request failed with status %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			ElectricitySpins struct {
				SpinsAllowed int    `json:"spinsAllowed"`
				Typename     string `json:"__typename"`
			} `json:"electricitySpins"`
			GasSpins struct {
				SpinsAllowed int    `json:"spinsAllowed"`
				Typename     string `json:"__typename"`
			} `json:"gasSpins"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Errors) > 0 {
		errorMessages := make([]string, len(result.Errors))
		for i, err := range result.Errors {
			errorMessages[i] = err.Message
		}
		return nil, fmt.Errorf("GraphQL errors: %s", strings.Join(errorMessages, ", "))
	}

	spins := &WheelOfFortuneSpins{
		ElectricitySpins: result.Data.ElectricitySpins.SpinsAllowed,
		GasSpins:        result.Data.GasSpins.SpinsAllowed,
	}

	c.debugLog("Wheel of Fortune spins: Electricity=%d, Gas=%d", spins.ElectricitySpins, spins.GasSpins)

	return spins, nil
}

// spinWheelOfFortune performs a single spin of the Wheel of Fortune for the specified fuel type
func (c *OctopusClient) spinWheelOfFortune(fuelType string) (*WheelSpinResult, error) {
	c.debugLog("Spinning Wheel of Fortune for %s...", fuelType)

	query := `mutation spinWheelOfFortune($input: WheelOfFortuneSpinInput!) {
		spinWheelOfFortune(input: $input) {
			prize {
				value
			}
		}
	}`

	variables := map[string]interface{}{
		"input": map[string]interface{}{
			"accountNumber": c.AccountID,
			"fuelType":      fuelType,
		},
	}

	c.debugLog("Spin query: %s", query)
	c.debugLog("Spin variables: %+v", variables)

	resp, err := c.makeGraphQLRequestWithEndpoint(getEndpoint("backend-graphql"), query, variables, true, "spinWheelOfFortune")
	if err != nil {
		c.debugLog("Spin request failed: %v", err)
		return nil, fmt.Errorf("failed to execute spin request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body for debugging
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	c.debugLog("Spin response body: %s", string(bodyBytes))

	var result WheelSpinResponse
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		c.debugLog("Failed to decode spin response: %v", err)
		return nil, fmt.Errorf("failed to decode spin response: %w", err)
	}

	prize := result.Data.SpinWheelOfFortune.Prize.Value
	c.debugLog("Wheel spin successful! Won %d OctoPoints for %s", prize, fuelType)

	return &WheelSpinResult{
		Prize:    prize,
		FuelType: fuelType,
	}, nil
}

// spinAllAvailableWheels spins all available wheels and returns the total prizes won
func (c *OctopusClient) spinAllAvailableWheels(spins *WheelOfFortuneSpins) ([]WheelSpinResult, error) {
	var results []WheelSpinResult
	c.debugLog("Starting to spin wheels: Electricity=%d, Gas=%d", spins.ElectricitySpins, spins.GasSpins)
	
	// Spin electricity wheels
	for i := 0; i < spins.ElectricitySpins; i++ {
		c.debugLog("Spinning electricity wheel %d of %d", i+1, spins.ElectricitySpins)
		result, err := c.spinWheelOfFortune("ELECTRICITY")
		if err != nil {
			c.logger.Error("Failed to spin electricity wheel",
				"wheel_number", i+1,
				"error", err)
			continue
		}
		results = append(results, *result)
		c.logger.Info("Electricity wheel spin complete",
			"wheel_number", i+1,
			"prize_points", result.Prize)
		// Small delay between spins to be respectful to the API
		time.Sleep(WheelSpinDelay)
	}

	// Spin gas wheels
	for i := 0; i < spins.GasSpins; i++ {
		c.debugLog("Spinning gas wheel %d of %d", i+1, spins.GasSpins)
		result, err := c.spinWheelOfFortune("GAS")
		if err != nil {
			c.logger.Error("Failed to spin gas wheel",
				"wheel_number", i+1,
				"error", err)
			continue
		}
		results = append(results, *result)
		c.logger.Info("Gas wheel spin complete",
			"wheel_number", i+1,
			"prize_points", result.Prize)
		// Small delay between spins to be respectful to the API
		time.Sleep(WheelSpinDelay)
	}
	
	c.debugLog("Finished spinning wheels. Total results: %d", len(results))
	return results, nil
}

type AccountInfo struct {
	Balance     float64 `json:"balance"`
	AccountType string  `json:"accountType"`
}

func (c *OctopusClient) getAccountInfoWithCache(state *AppState) (*AccountInfo, error) {
	// Check cache if state is provided - account balance changes at most hourly, often less
	if state != nil && state.CachedAccountInfo != nil {
		if state.IsCacheValid(state.CachedAccountInfo.Timestamp, CacheDurationAccountInfo) {
			return state.CachedAccountInfo.Data, nil
		}
	}

	// Get fresh account info
	accountInfo, err := c.getAccountInfo()
	if err != nil {
		return nil, err
	}

	// Update cache if state is provided
	if state != nil {
		state.CachedAccountInfo = &CachedAccountInfo{
			Data:      accountInfo,
			Timestamp: time.Now(),
		}
	}

	return accountInfo, nil
}

func (c *OctopusClient) getAccountInfo() (*AccountInfo, error) {
	c.debugLog("Requesting account info...")

	query := `query getAccountInfo($accountNumber: String!) {
		account(accountNumber: $accountNumber) {
			activeReferralSchemes {
				domestic {
					referralUrl
					referrerRewardAmount
					__typename
				}
				__typename
			}
			balance
			accountType
			__typename
		}
	}`

	variables := map[string]interface{}{
		"accountNumber": c.AccountID,
	}

	resp, err := c.makeGraphQLRequest(query, variables, true)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	c.debugLog("Account info request status: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		c.debugLog("Account info request failed body: %s", string(bodyBytes))
		return nil, fmt.Errorf("GraphQL request failed with status %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			Account struct {
				Balance     float64 `json:"balance"`
				AccountType string  `json:"accountType"`
			} `json:"account"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Errors) > 0 {
		errorMessages := make([]string, len(result.Errors))
		for i, err := range result.Errors {
			errorMessages[i] = err.Message
		}
		return nil, fmt.Errorf("GraphQL errors: %s", strings.Join(errorMessages, ", "))
	}

	accountInfo := &AccountInfo{
		Balance:     result.Data.Account.Balance / 100.0, // Convert from pennies to pounds
		AccountType: result.Data.Account.AccountType,
	}

	c.debugLog("Account balance: £%.2f, Account type: %s", accountInfo.Balance, accountInfo.AccountType)

	return accountInfo, nil
}

// getSmartMeterDevices retrieves ESME (Electricity Smart Meter) device IDs
func (c *OctopusClient) getSmartMeterDevices() ([]string, error) {
	query := `query getEligibility($accountNumber: String!) {
		account(accountNumber: $accountNumber) {
			properties {
				id
				address
				smartDeviceNetworks {
					smartDevices {
						deviceId
						type
						__typename
					}
					__typename
				}
				__typename
			}
			__typename
		}
	}`

	variables := map[string]interface{}{
		"accountNumber": c.AccountID,
	}

	resp, err := c.makeGraphQLRequest(query, variables, true)
	if err != nil {
		return nil, fmt.Errorf("failed to execute meter eligibility request: %w", err)
	}
	defer resp.Body.Close()

	var result MeterEligibilityResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode meter eligibility response: %w", err)
	}

	var deviceIDs []string
	for _, property := range result.Data.Account.Properties {
		for _, network := range property.SmartDeviceNetworks {
			for _, device := range network.SmartDevices {
				// Only include ESME (Electricity Smart Meter) devices
				if device.Type == "ESME" {
					deviceIDs = append(deviceIDs, device.DeviceID)
					c.debugLog("Found ESME device: %s", device.DeviceID)
				}
			}
		}
	}

	c.debugLog("Found %d ESME devices", len(deviceIDs))
	return deviceIDs, nil
}

// getUsageMeasurements retrieves electricity usage measurements for the last N days
func (c *OctopusClient) getUsageMeasurements(deviceIDs []string, days int) ([]UsageMeasurement, error) {
	if len(deviceIDs) == 0 {
		return nil, fmt.Errorf("no device IDs provided")
	}

	// Use first device ID for now (most users have one electricity meter)
	deviceID := deviceIDs[0]
	
	// Calculate time range
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -days)
	
	c.debugLog("Fetching usage measurements: %d days from %s to %s", days, startTime.Format("2006-01-02 15:04"), endTime.Format("2006-01-02 15:04"))

	query := `query getMeasurements($accountNumber: String!, $first: Int!, $utilityFilters: [UtilityFiltersInput!], $startAt: DateTime, $endAt: DateTime, $timezone: String) {
		account(accountNumber: $accountNumber) {
			properties {
				id
				measurements(
					first: $first
					utilityFilters: $utilityFilters
					startAt: $startAt
					endAt: $endAt
					timezone: $timezone
				) {
					edges {
						node {
							value
							unit
							... on IntervalMeasurementType {
								startAt
								endAt
								durationInSeconds
								__typename
							}
							metaData {
								statistics {
									costExclTax {
										pricePerUnit {
											amount
											__typename
										}
										costCurrency
										estimatedAmount
										__typename
									}
									costInclTax {
										costCurrency
										estimatedAmount
										__typename
									}
									value
									description
									label
									type
									__typename
								}
								__typename
							}
							__typename
						}
						__typename
					}
					pageInfo {
						hasNextPage
						hasPreviousPage
						startCursor
						endCursor
						__typename
					}
					__typename
				}
				__typename
			}
			__typename
		}
	}`

	variables := map[string]interface{}{
		"accountNumber": c.AccountID,
		"first":         1000, // Adjust based on expected data volume
		"startAt":       startTime.Format(time.RFC3339),
		"endAt":         endTime.Format(time.RFC3339),
		"timezone":      "Europe/London",
		"utilityFilters": []map[string]interface{}{
			{
				"electricityFilters": map[string]interface{}{
					"readingFrequencyType": "RAW_INTERVAL",
					"readingDirection":     "CONSUMPTION",
					"deviceId":             deviceID,
				},
			},
		},
	}

	resp, err := c.makeGraphQLRequest(query, variables, true)
	if err != nil {
		return nil, fmt.Errorf("failed to execute measurements request: %w", err)
	}
	defer resp.Body.Close()

	var result UsageMeasurementsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode measurements response: %w", err)
	}

	var measurements []UsageMeasurement
	for _, property := range result.Data.Account.Properties {
		for _, edge := range property.Measurements.Edges {
			measurements = append(measurements, edge.Node)
		}
	}

	c.debugLog("Retrieved %d usage measurements for device %s", len(measurements), deviceID)
	
	// Debug: Show first few measurements to understand data structure
	if len(measurements) > 0 && c.debug {
		c.debugLog("Sample measurements:")
		sampleCount := len(measurements)
		if sampleCount > 3 {
			sampleCount = 3
		}
		for i, m := range measurements[:sampleCount] {
			costStr := "no cost data"
			if len(m.MetaData.Statistics) > 0 {
				costStr = m.MetaData.Statistics[0].CostInclTax.EstimatedAmount
			}
			c.debugLog("  %d. %s: %s %s (Cost: %s)", i+1, m.StartAt.Format("2006-01-02 15:04"), m.Value, m.Unit, costStr)
		}
	}
	
	return measurements, nil
}

// GetValueAsFloat64 parses the string value as float64
func (m *UsageMeasurement) GetValueAsFloat64() float64 {
	if val, err := strconv.ParseFloat(m.Value, 64); err == nil {
		return val
	}
	return 0.0
}

// getSmartMeterDevicesWithCache retrieves ESME device IDs with caching
func (c *OctopusClient) getSmartMeterDevicesWithCache(state *AppState) ([]string, error) {
	if state != nil && state.CachedMeterDevices != nil {
		if state.IsCacheValid(state.CachedMeterDevices.Timestamp, CacheDurationMeterDevices) {
			return state.CachedMeterDevices.Data, nil
		}
	}

	// Get fresh data
	devices, err := c.getSmartMeterDevices()
	if err != nil {
		return nil, err
	}

	// Cache the result
	if state != nil {
		state.CachedMeterDevices = &CachedMeterDevices{
			Data:      devices,
			Timestamp: time.Now(),
		}
	}

	return devices, nil
}

// getUsageMeasurementsWithCache retrieves usage measurements with caching
func (c *OctopusClient) getUsageMeasurementsWithCache(state *AppState, days int) ([]UsageMeasurement, error) {
	if state != nil && state.CachedUsageMeasurements != nil {
		// Cache is valid if it's less than duration old and covers the same or more days
		if state.IsCacheValid(state.CachedUsageMeasurements.Timestamp, CacheDurationUsageMeasurements) && 
		   state.CachedUsageMeasurements.Days >= days {
			c.debugLog("Using cached usage measurements (%d measurements, %d days, age: %v)", 
				len(state.CachedUsageMeasurements.Data), state.CachedUsageMeasurements.Days, 
				time.Since(state.CachedUsageMeasurements.Timestamp))
			
			// Filter cached data to only include the requested number of days
			cutoffTime := time.Now().AddDate(0, 0, -days)
			var filteredData []UsageMeasurement
			for _, measurement := range state.CachedUsageMeasurements.Data {
				if measurement.StartAt.After(cutoffTime) {
					filteredData = append(filteredData, measurement)
				}
			}
			return filteredData, nil
		}
	}

	// Get device IDs first
	devices, err := c.getSmartMeterDevicesWithCache(state)
	if err != nil {
		return nil, fmt.Errorf("failed to get meter devices: %w", err)
	}

	if len(devices) == 0 {
		return nil, fmt.Errorf("no ESME devices found")
	}

	// Get fresh usage data
	measurements, err := c.getUsageMeasurements(devices, days)
	if err != nil {
		return nil, err
	}

	// Cache the result
	if state != nil {
		state.CachedUsageMeasurements = &CachedUsageMeasurements{
			Data:      measurements,
			Timestamp: time.Now(),
			Days:      days,
		}
	}

	return measurements, nil
}