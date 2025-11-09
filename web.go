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
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"
)

type CampaignStatus struct {
	SavingSessionsEnabled    bool `json:"saving_sessions_enabled"`
	FreeElectricityEnabled   bool `json:"free_electricity_enabled"`
	HasOctoplus             bool `json:"has_octoplus"`
	HasSavingSessions       bool `json:"has_saving_sessions"`
	HasFreeElectricity      bool `json:"has_free_electricity"`
}

type SessionData struct {
	CurrentPoints       int                      `json:"current_points"`
	AccountBalance      float64                  `json:"account_balance"`
	WheelOfFortuneSpins *WheelOfFortuneSpins     `json:"wheel_of_fortune_spins"`
	SavingSessions      []SavingSession          `json:"saving_sessions"`
	FreeElectricitySessions []FreeElectricitySession `json:"free_electricity_sessions"`
	CampaignStatus      CampaignStatus           `json:"campaign_status"`
	LastUpdated         time.Time                `json:"last_updated"`
}

type WebServer struct {
	monitor *SavingSessionMonitor
	server  *http.Server
}

func NewWebServer(monitor *SavingSessionMonitor, port int) *WebServer {
	mux := http.NewServeMux()
	
	ws := &WebServer{
		monitor: monitor,
		server: &http.Server{
			Addr:    fmt.Sprintf(":%d", port),
			Handler: mux,
		},
	}
	
	mux.HandleFunc("/", ws.handleDashboard)
	mux.HandleFunc("/api/sessions", ws.handleSessionsAPI)
	mux.HandleFunc("/api/usage", ws.handleUsageAPI)
	mux.HandleFunc("/api/usage/refresh", ws.handleUsageRefreshAPI)
	
	// Add Prometheus metrics endpoint
	metricsCollector := NewMetricsCollector(monitor.client, monitor)
	mux.Handle("/metrics", metricsCollector)
	
	return ws
}

func (ws *WebServer) Start() error {
	log.Printf("Starting web server on %s", ws.server.Addr)
	return ws.server.ListenAndServe()
}

func getCacheAge(cached *CachedUsageMeasurements) int {
	if cached == nil {
		return -1
	}
	return int(time.Since(cached.Timestamp).Seconds())
}

func (ws *WebServer) handleSessionsAPI(w http.ResponseWriter, r *http.Request) {
	// Get current session data
	sessions, err := ws.monitor.client.GetSavingSessionsWithCache(ws.monitor.state)
	if err != nil {
		log.Printf("Warning: Failed to get saving sessions: %v", err)
		sessions = nil // Will use default values
	}
	
	freeElectricity, err := ws.monitor.client.GetFreeElectricitySessionsWithCache(ws.monitor.state)
	if err != nil {
		log.Printf("Warning: Failed to get free electricity sessions: %v", err)
		freeElectricity = &FreeElectricitySessionsResponse{} // Empty response
	}
	
	// Filter upcoming sessions
	now := time.Now()
	var upcomingSavingSessions []SavingSession
	var upcomingFreeElectricitySessions []FreeElectricitySession
	
	// Filter saving sessions
	if sessions != nil && sessions.Data.SavingSessions.Account.JoinedEvents != nil {
		for _, session := range sessions.Data.SavingSessions.Account.JoinedEvents {
			if session.EndAt.After(now) {
				upcomingSavingSessions = append(upcomingSavingSessions, session)
			}
		}
	}
	
	// Filter free electricity sessions  
	if freeElectricity != nil {
		for _, session := range freeElectricity.Data {
			if session.EndAt.After(now) {
				upcomingFreeElectricitySessions = append(upcomingFreeElectricitySessions, session)
			}
		}
	}
	
	// Get current points
	currentPoints := 0
	if sessions != nil {
		currentPoints = sessions.Data.OctoPoints.Account.CurrentPointsInWallet
	}

	// Get account balance (with caching)
	accountBalance := 0.0
	accountInfo, err := ws.monitor.client.getAccountInfoWithCache(ws.monitor.state)
	if err != nil {
		log.Printf("Warning: Could not get account balance: %v", err)
	} else {
		accountBalance = accountInfo.Balance
	}

	// Get Wheel of Fortune spins (with caching)
	wheelSpins, err := ws.monitor.client.getWheelOfFortuneSpinsWithCache(ws.monitor.state)
	if err != nil {
		log.Printf("Warning: Could not get Wheel of Fortune spins: %v", err)
		wheelSpins = &WheelOfFortuneSpins{ElectricitySpins: 0, GasSpins: 0}
	}

	// Get campaign status (with caching)
	campaigns, err := ws.monitor.client.getCampaignStatusWithCache(ws.monitor.state)
	if err != nil {
		log.Printf("Warning: Could not get campaign status: %v", err)
		campaigns = map[string]bool{
			"octoplus": false,
			"octoplus-saving-sessions": false,
			"free_electricity": false,
		}
	}

	campaignStatus := CampaignStatus{
		HasOctoplus:             campaigns["octoplus"],
		HasSavingSessions:       campaigns["octoplus-saving-sessions"],
		HasFreeElectricity:      campaigns["free_electricity"],
		SavingSessionsEnabled:   campaigns["octoplus"] && campaigns["octoplus-saving-sessions"],
		FreeElectricityEnabled:  campaigns["free_electricity"],
	}
	
	// Ensure arrays are never nil
	if upcomingSavingSessions == nil {
		upcomingSavingSessions = []SavingSession{}
	}
	if upcomingFreeElectricitySessions == nil {
		upcomingFreeElectricitySessions = []FreeElectricitySession{}
	}
	
	data := SessionData{
		CurrentPoints:               currentPoints,
		AccountBalance:              accountBalance,
		WheelOfFortuneSpins:        wheelSpins,
		SavingSessions:             upcomingSavingSessions,
		FreeElectricitySessions:    upcomingFreeElectricitySessions,
		CampaignStatus:             campaignStatus,
		LastUpdated:                time.Now(),
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(data)
}

func (ws *WebServer) handleUsageAPI(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	daysParam := r.URL.Query().Get("days")
	days := WebDefaultUsageDays // default
	if daysParam != "" {
		if d, err := fmt.Sscanf(daysParam, "%d", &days); err == nil && d > 0 {
			if days > WebMaxUsageDays {
				days = WebMaxUsageDays // max days
			}
		}
	}
	
	// Get usage measurements with caching
	measurements, err := ws.monitor.client.getUsageMeasurementsWithCache(ws.monitor.state, days)
	if err != nil {
		log.Printf("Error getting usage measurements: %v", err)
		http.Error(w, "Failed to get usage data", http.StatusInternalServerError)
		return
	}
	
	// Transform measurements for Chart.js
	var chartData []map[string]interface{}
	for _, m := range measurements {
		costEstimate := 0.0
		if len(m.MetaData.Statistics) > 0 {
			if val, err := strconv.ParseFloat(m.MetaData.Statistics[0].CostInclTax.EstimatedAmount, 64); err == nil {
				costEstimate = val
			}
		}
		
		chartData = append(chartData, map[string]interface{}{
			"timestamp": m.StartAt.Unix() * 1000, // JavaScript timestamp
			"datetime":  m.StartAt.Format("2006-01-02T15:04:05Z07:00"),
			"value":     m.GetValueAsFloat64(),
			"unit":      m.Unit,
			"cost":      costEstimate,
			"duration":  m.Duration,
		})
	}
	
	response := map[string]interface{}{
		"success":      true,
		"days":         days,
		"measurements": len(measurements),
		"data":         chartData,
		"cache_age":    getCacheAge(ws.monitor.state.CachedUsageMeasurements),
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(response)
}

func (ws *WebServer) handleUsageRefreshAPI(w http.ResponseWriter, r *http.Request) {
	// Force cache invalidation by clearing cached usage measurements
	if ws.monitor.state != nil {
		ws.monitor.state.CachedUsageMeasurements = nil
		log.Println("Cleared usage measurements cache")
	}

	// Parse query parameters
	daysParam := r.URL.Query().Get("days")
	days := WebDefaultUsageDays // default
	if daysParam != "" {
		if d, err := fmt.Sscanf(daysParam, "%d", &days); err == nil && d > 0 {
			if days > WebMaxUsageDays {
				days = WebMaxUsageDays // max days
			}
		}
	}
	
	// Get fresh usage measurements (bypassing cache)
	measurements, err := ws.monitor.client.getUsageMeasurements([]string{}, days)
	if err != nil {
		// Get device IDs first
		devices, err := ws.monitor.client.getSmartMeterDevicesWithCache(ws.monitor.state)
		if err != nil {
			log.Printf("Error getting meter devices: %v", err)
			http.Error(w, "Failed to get meter devices", http.StatusInternalServerError)
			return
		}
		
		if len(devices) == 0 {
			http.Error(w, "No ESME devices found", http.StatusInternalServerError)
			return
		}
		
		measurements, err = ws.monitor.client.getUsageMeasurements(devices, days)
		if err != nil {
			log.Printf("Error getting fresh usage measurements: %v", err)
			http.Error(w, "Failed to get fresh usage data", http.StatusInternalServerError)
			return
		}
	}
	
	// Transform measurements for Chart.js
	var chartData []map[string]interface{}
	for _, m := range measurements {
		costEstimate := 0.0
		if len(m.MetaData.Statistics) > 0 {
			if val, err := strconv.ParseFloat(m.MetaData.Statistics[0].CostInclTax.EstimatedAmount, 64); err == nil {
				costEstimate = val
			}
		}
		
		chartData = append(chartData, map[string]interface{}{
			"timestamp": m.StartAt.Unix() * 1000, // JavaScript timestamp
			"datetime":  m.StartAt.Format("2006-01-02T15:04:05Z07:00"),
			"value":     m.GetValueAsFloat64(),
			"unit":      m.Unit,
			"cost":      costEstimate,
			"duration":  m.Duration,
		})
	}
	
	response := map[string]interface{}{
		"success":      true,
		"days":         days,
		"measurements": len(measurements),
		"data":         chartData,
		"cache_age":    0, // Fresh data
		"refreshed":    true,
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(response)
}

func (ws *WebServer) handleDashboard(w http.ResponseWriter, r *http.Request) {
	const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Octopus Energy Dashboard</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            min-height: 100vh;
            padding: 20px;
        }
        
        .container {
            max-width: 1200px;
            margin: 0 auto;
        }
        
        .header {
            text-align: center;
            margin-bottom: 40px;
        }
        
        .header h1 {
            font-size: 2.5rem;
            margin-bottom: 10px;
        }
        
        .status {
            background: rgba(255, 255, 255, 0.1);
            backdrop-filter: blur(10px);
            border-radius: 10px;
            padding: 20px;
            margin-bottom: 30px;
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 20px;
        }
        
        .status-item {
            text-align: center;
        }
        
        .status-value {
            font-size: 1.5rem;
            font-weight: bold;
            margin-bottom: 5px;
        }
        
        .sections {
            display: grid;
            grid-template-columns: 1fr 2fr;
            gap: 30px;
        }
        
        .sessions-column {
            display: flex;
            flex-direction: column;
            gap: 30px;
        }
        
        .section {
            background: rgba(255, 255, 255, 0.1);
            backdrop-filter: blur(10px);
            border-radius: 10px;
            padding: 25px;
        }
        
        .section h2 {
            margin-bottom: 20px;
            font-size: 1.5rem;
            border-bottom: 2px solid rgba(255, 255, 255, 0.3);
            padding-bottom: 10px;
        }
        
        .session {
            background: rgba(255, 255, 255, 0.1);
            border-radius: 8px;
            padding: 15px;
            margin-bottom: 15px;
        }
        
        .session:last-child {
            margin-bottom: 0;
        }
        
        .session-date {
            font-weight: bold;
            font-size: 1.1rem;
            margin-bottom: 8px;
        }
        
        .session-details {
            font-size: 0.9rem;
            opacity: 0.9;
        }
        
        .session-countdown {
            font-weight: bold;
            color: #ffd700;
            margin-top: 5px;
        }
        
        .no-sessions {
            text-align: center;
            opacity: 0.7;
            font-style: italic;
        }
        
        .footer {
            text-align: center;
            margin-top: 30px;
            opacity: 0.7;
        }
        
        .footer .disclaimer {
            font-size: 0.8rem;
            opacity: 0.6;
            margin-top: 10px;
            line-height: 1.4;
        }
        
        .loading {
            text-align: center;
            font-size: 1.2rem;
        }
        
        .campaign-item {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 10px 0;
            border-bottom: 1px solid rgba(255, 255, 255, 0.2);
        }
        
        .campaign-item:last-child {
            border-bottom: none;
        }
        
        .campaign-name {
            font-weight: bold;
        }
        
        .campaign-requirement {
            font-size: 0.8rem;
            opacity: 0.8;
            margin-top: 2px;
        }
        
        .status-enabled {
            color: #4ade80;
            font-weight: bold;
        }
        
        .status-disabled {
            color: #f87171;
            font-weight: bold;
        }
        
        .missing-campaigns {
            background: rgba(248, 113, 113, 0.1);
            border-left: 4px solid #f87171;
            padding: 10px;
            margin-top: 10px;
            border-radius: 4px;
        }
        
        .missing-campaigns ul {
            margin: 5px 0;
            padding-left: 20px;
        }
        
        @media (max-width: 768px) {
            .sections {
                grid-template-columns: 1fr;
            }
            
            .sessions-column {
                gap: 20px;
            }
            
            .header h1 {
                font-size: 2rem;
            }
        }
        
        .usage-section {
            background: rgba(255, 255, 255, 0.1);
            border-radius: 15px;
            padding: 25px;
            backdrop-filter: blur(10px);
        }
        
        .chart-container {
            position: relative;
            height: 500px;
            margin: 20px 0;
        }
        
        .usage-loading {
            display: flex;
            flex-direction: column;
            align-items: center;
            justify-content: center;
            height: 500px;
            color: rgba(255, 255, 255, 0.7);
        }
        
        .usage-spinner {
            width: 40px;
            height: 40px;
            border: 4px solid rgba(255, 255, 255, 0.3);
            border-left: 4px solid #4ade80;
            border-radius: 50%;
            animation: spin 1s linear infinite;
            margin-bottom: 15px;
        }
        
        @keyframes spin {
            0% { transform: rotate(0deg); }
            100% { transform: rotate(360deg); }
        }
        
        .usage-controls {
            margin: 15px 0;
            text-align: center;
        }
        
        .usage-controls button {
            background: rgba(255, 255, 255, 0.2);
            border: 1px solid rgba(255, 255, 255, 0.3);
            color: white;
            padding: 8px 16px;
            margin: 0 5px;
            border-radius: 8px;
            cursor: pointer;
            transition: all 0.3s ease;
        }
        
        .usage-controls button:hover {
            background: rgba(255, 255, 255, 0.3);
        }
        
        .usage-controls button.active {
            background: rgba(255, 255, 255, 0.4);
            border-color: rgba(255, 255, 255, 0.5);
        }
    </style>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/chartjs-adapter-date-fns"></script>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🐙 Octopus Energy Dashboard</h1>
            <div id="last-updated"></div>
        </div>
        
        <div class="status" id="status">
            <div class="loading">Loading...</div>
        </div>
        
        <div class="sections" id="sections" style="display: none;">
            <div class="sessions-column">
                <div class="section">
                    <h2>⚡ What Sessions Can I Join?</h2>
                    <div id="campaign-status"></div>
                </div>
                
                <div class="section">
                    <h2>💡 Saving Sessions</h2>
                    <div id="saving-sessions"></div>
                </div>
                
                <div class="section">
                    <h2>🔋 Free Electricity Sessions</h2>
                    <div id="free-electricity-sessions"></div>
                </div>
            </div>
            
            <div class="section usage-section">
                <h2>📊 Electricity Usage</h2>
                <div class="usage-controls">
                    <button onclick="loadUsageData(1)" id="btn-1day">1 Day</button>
                    <button onclick="loadUsageData(3)" id="btn-3days">3 Days</button>
                    <button onclick="loadUsageData(7)" id="btn-7days" class="active">7 Days</button>
                    <button onclick="loadUsageData(14)" id="btn-14days">14 Days</button>
                    <button onclick="loadUsageData(30)" id="btn-30days">30 Days</button>
                </div>
                <div class="chart-container">
                    <canvas id="usageChart"></canvas>
                </div>
                <div id="usage-stats"></div>
            </div>
        </div>
        
        <div class="footer">
            <p>Auto-refreshing every 30 seconds</p>
            <p class="disclaimer">
                This is an unofficial third-party application. 
                "Octopus Energy" is a trademark of Octopus Energy Group Limited. 
                This application is not affiliated with, endorsed by, or connected to Octopus Energy.
            </p>
        </div>
    </div>

    <script>
        let countdownIntervals = [];
        
        function clearCountdowns() {
            countdownIntervals.forEach(interval => clearInterval(interval));
            countdownIntervals = [];
        }
        
        function formatDuration(minutes) {
            if (minutes < 60) {
                return minutes + 'm';
            }
            const hours = Math.floor(minutes / 60);
            const mins = minutes % 60;
            return hours + 'h' + (mins > 0 ? ' ' + mins + 'm' : '');
        }
        
        function getTimeUntil(dateStr) {
            const target = new Date(dateStr);
            const now = new Date();
            const diff = target - now;
            
            if (diff <= 0) return 'Started';
            
            const days = Math.floor(diff / (1000 * 60 * 60 * 24));
            const hours = Math.floor((diff % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60));
            const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60));
            
            if (days > 0) return days + 'd ' + hours + 'h ' + minutes + 'm';
            if (hours > 0) return hours + 'h ' + minutes + 'm';
            return minutes + 'm';
        }
        
        function startCountdown(element, targetDate) {
            const interval = setInterval(() => {
                const timeUntil = getTimeUntil(targetDate);
                element.textContent = 'Starts in ' + timeUntil;
                
                if (timeUntil === 'Started') {
                    clearInterval(interval);
                    element.textContent = 'Started!';
                }
            }, 1000);
            countdownIntervals.push(interval);
        }
        
        function formatDate(dateStr) {
            const date = new Date(dateStr);
            return date.toLocaleDateString('en-GB', { 
                weekday: 'long', 
                month: 'short', 
                day: 'numeric',
                hour: '2-digit',
                minute: '2-digit'
            });
        }
        
        function updateDashboard() {
            fetch('/api/sessions')
                .then(response => response.json())
                .then(data => {
                    // Update status
                    const statusDiv = document.getElementById('status');
                    statusDiv.innerHTML = ` + "`" + `
                        <div class="status-item">
                            <div class="status-value">£${data.account_balance.toFixed(2)}</div>
                            <div>Account Balance</div>
                        </div>
                        <div class="status-item">
                            <div class="status-value">${data.current_points}</div>
                            <div>OctoPoints</div>
                        </div>
                        <div class="status-item">
                            <div class="status-value">${data.saving_sessions ? data.saving_sessions.length : 0}</div>
                            <div>Upcoming Saving Sessions</div>
                        </div>
                        <div class="status-item">
                            <div class="status-value">${data.free_electricity_sessions ? data.free_electricity_sessions.length : 0}</div>
                            <div>Upcoming Free Electricity Sessions</div>
                        </div>
                        <div class="status-item">
                            <div class="status-value">${(data.wheel_of_fortune_spins.electricity_spins + data.wheel_of_fortune_spins.gas_spins)}</div>
                            <div>Wheel of Fortune Spins</div>
                        </div>
                    ` + "`" + `;
                    
                    // Update campaign status
                    const campaignDiv = document.getElementById('campaign-status');
                    let campaignHTML = '';
                    
                    // Saving Sessions Status
                    campaignHTML += ` + "`" + `
                        <div class="campaign-item">
                            <div>
                                <div class="campaign-name">Saving Sessions</div>
                                <div class="campaign-requirement">Requires: octoplus + octoplus-saving-sessions</div>
                            </div>
                            <div class="${data.campaign_status.saving_sessions_enabled ? 'status-enabled' : 'status-disabled'}">
                                ${data.campaign_status.saving_sessions_enabled ? '✅ ENABLED' : '❌ DISABLED'}
                            </div>
                        </div>
                    ` + "`" + `;
                    
                    // Free Electricity Status
                    campaignHTML += ` + "`" + `
                        <div class="campaign-item">
                            <div>
                                <div class="campaign-name">Free Electricity Sessions</div>
                                <div class="campaign-requirement">Requires: free_electricity</div>
                            </div>
                            <div class="${data.campaign_status.free_electricity_enabled ? 'status-enabled' : 'status-disabled'}">
                                ${data.campaign_status.free_electricity_enabled ? '✅ ENABLED' : '❌ DISABLED'}
                            </div>
                        </div>
                    ` + "`" + `;
                    
                    // Show missing campaigns if any
                    let missingCampaigns = [];
                    if (!data.campaign_status.has_octoplus) missingCampaigns.push('octoplus');
                    if (!data.campaign_status.has_saving_sessions) missingCampaigns.push('octoplus-saving-sessions');
                    if (!data.campaign_status.has_free_electricity) missingCampaigns.push('free_electricity');
                    
                    if (missingCampaigns.length > 0) {
                        campaignHTML += ` + "`" + `
                            <div class="missing-campaigns">
                                <strong>To Enable Missing Features:</strong>
                                <ul>
                                    ${missingCampaigns.map(c => '<li>Sign up for ' + c.replace('_', ' ').replace('-', ' ') + '</li>').join('')}
                                </ul>
                                <small>Visit <a href="https://octopus.energy" target="_blank" style="color: #ffd700;">octopus.energy</a> to get started</small>
                            </div>
                        ` + "`" + `;
                    }
                    
                    campaignDiv.innerHTML = campaignHTML;
                    
                    // Update saving sessions
                    const savingDiv = document.getElementById('saving-sessions');
                    let newSavingContent = '';
                    if (!data.saving_sessions || data.saving_sessions.length === 0) {
                        if (data.campaign_status.saving_sessions_enabled) {
                            newSavingContent = '<div class="no-sessions">No upcoming saving sessions</div>';
                        } else {
                            newSavingContent = '<div class="no-sessions">Saving sessions disabled - missing required campaigns</div>';
                        }
                    } else {
                        newSavingContent = data.saving_sessions.map(session => {
                            const duration = Math.floor((new Date(session.endAt) - new Date(session.startAt)) / (1000 * 60));
                            return ` + "`" + `
                                <div class="session">
                                    <div class="session-date">${formatDate(session.startAt)}</div>
                                    <div class="session-details">
                                        Duration: ${formatDuration(duration)} | Points: ${session.octoPoints}
                                    </div>
                                    <div class="session-countdown" data-target="${session.startAt}"></div>
                                </div>
                            ` + "`" + `;
                        }).join('');
                    }
                    
                    // Check if saving sessions data has changed
                    const currentSessionsKey = JSON.stringify(data.saving_sessions || []);
                    if (window.lastSavingSessionsKey !== currentSessionsKey) {
                        clearCountdowns();
                        savingDiv.innerHTML = newSavingContent;
                        window.lastSavingSessionsKey = currentSessionsKey;
                        // Start countdowns
                        document.querySelectorAll('.session-countdown[data-target]').forEach(el => {
                            startCountdown(el, el.getAttribute('data-target'));
                        });
                    }
                    
                    // Update free electricity sessions
                    const freeDiv = document.getElementById('free-electricity-sessions');
                    let newFreeContent = '';
                    if (!data.free_electricity_sessions || data.free_electricity_sessions.length === 0) {
                        if (data.campaign_status.free_electricity_enabled) {
                            newFreeContent = '<div class="no-sessions">No upcoming free electricity sessions</div>';
                        } else {
                            newFreeContent = '<div class="no-sessions">Free electricity disabled - missing required campaign</div>';
                        }
                    } else {
                        newFreeContent = data.free_electricity_sessions.map(session => {
                            const startTime = new Date(session.start);
                            const endTime = new Date(session.end);
                            const duration = Math.floor((endTime - startTime) / (1000 * 60));
                            return ` + "`" + `
                                <div class="session">
                                    <div class="session-date">${formatDate(session.start)}</div>
                                    <div class="session-details">
                                        Duration: ${formatDuration(duration)} | Free electricity!
                                    </div>
                                    <div class="session-countdown" data-target="${session.start}"></div>
                                </div>
                            ` + "`" + `;
                        }).join('');
                    }
                    
                    // Check if free electricity sessions data has changed
                    const currentFreeSessionsKey = JSON.stringify(data.free_electricity_sessions || []);
                    if (window.lastFreeSessionsKey !== currentFreeSessionsKey) {
                        freeDiv.innerHTML = newFreeContent;
                        window.lastFreeSessionsKey = currentFreeSessionsKey;
                        // Start countdowns for free electricity sessions
                        document.querySelectorAll('#free-electricity-sessions .session-countdown[data-target]').forEach(el => {
                            startCountdown(el, el.getAttribute('data-target'));
                        });
                    }
                    
                    // Update last updated
                    document.getElementById('last-updated').textContent = 
                        'Last updated: ' + new Date(data.last_updated).toLocaleTimeString();
                    
                    // Show sections
                    document.getElementById('sections').style.display = 'grid';
                })
                .catch(error => {
                    console.error('Error fetching data:', error);
                    document.getElementById('status').innerHTML = 
                        '<div class="loading">Error loading data. Retrying...</div>';
                });
        }
        
        // Usage chart variables
        let usageChart = null;
        let currentDays = 7;
        
        function loadUsageData(days) {
            currentDays = days;
            
            // Update active button
            document.querySelectorAll('.usage-controls button').forEach(btn => btn.classList.remove('active'));
            document.getElementById('btn-' + days + (days === 1 ? 'day' : 'days')).classList.add('active');
            
            // Show loading spinner
            showUsageLoading();
            
            fetch('/api/usage?days=' + days)
                .then(response => response.json())
                .then(data => {
                    if (data.success) {
                        updateUsageChart(data);
                        updateUsageStats(data);
                    } else {
                        console.error('Failed to load usage data:', data);
                        showUsageError('Failed to load usage data');
                    }
                })
                .catch(error => {
                    console.error('Error loading usage data:', error);
                    showUsageError('Error loading usage data. Please try again.');
                });
        }
        
        function updateUsageChart(data) {
            // Destroy existing chart
            if (usageChart) {
                usageChart.destroy();
            }
            
            // Check if data is null or empty
            if (!data.data || data.data.length === 0) {
                // Show "No Data" message
                const chartContainer = document.querySelector('.chart-container');
                chartContainer.innerHTML = '<div style="text-align: center; padding: 50px; color: rgba(255, 255, 255, 0.7); font-size: 18px;">No Data Available</div>';
                return;
            }
            
            // Ensure canvas exists
            const chartContainer = document.querySelector('.chart-container');
            chartContainer.innerHTML = '<canvas id="usageChart"></canvas>';
            const ctx = document.getElementById('usageChart').getContext('2d');
            
            // Prepare data for Chart.js
            const chartData = data.data.map(point => ({
                x: new Date(point.timestamp),
                y: point.value,
                cost: point.cost
            }));
            
            usageChart = new Chart(ctx, {
                type: 'line',
                data: {
                    datasets: [{
                        label: 'Electricity Usage (kWh)',
                        data: chartData,
                        borderColor: 'rgba(75, 192, 192, 1)',
                        backgroundColor: 'rgba(75, 192, 192, 0.2)',
                        fill: true,
                        tension: 0.1
                    }]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    scales: {
                        x: {
                            type: 'time',
                            time: {
                                unit: currentDays <= 1 ? 'hour' : 'day'
                            },
                            grid: {
                                color: 'rgba(255, 255, 255, 0.1)'
                            },
                            ticks: {
                                color: 'rgba(255, 255, 255, 0.8)'
                            }
                        },
                        y: {
                            beginAtZero: true,
                            grid: {
                                color: 'rgba(255, 255, 255, 0.1)'
                            },
                            ticks: {
                                color: 'rgba(255, 255, 255, 0.8)'
                            }
                        }
                    },
                    plugins: {
                        legend: {
                            labels: {
                                color: 'rgba(255, 255, 255, 0.8)'
                            }
                        },
                        tooltip: {
                            callbacks: {
                                label: function(context) {
                                    const point = context.raw;
                                    return 'Usage: ' + point.y.toFixed(3) + ' kWh';
                                }
                            }
                        }
                    }
                }
            });
        }
        
        function updateUsageStats(data) {
            // Check if data is null or empty
            if (!data.data || data.data.length === 0) {
                const statsHTML = '<div style="display: flex; justify-content: space-around; margin-top: 15px;">' +
                    '<div><strong>Total Usage:</strong> No data</div>' +
                    '<div><strong>Average:</strong> No data</div>' +
                    '<div><strong>Data Points:</strong> 0</div>' +
                    '<div><strong>Period:</strong> ' + data.days + ' days</div>' +
                    '</div>';
                document.getElementById('usage-stats').innerHTML = statsHTML;
                return;
            }
            
            const totalUsage = data.data.reduce((sum, point) => sum + point.value, 0);
            const totalCost = data.data.reduce((sum, point) => sum + point.cost, 0);
            const avgUsage = totalUsage / data.measurements;
            
            const statsHTML = '<div style="display: flex; justify-content: space-around; margin-top: 15px;">' +
                '<div><strong>Total Usage:</strong> ' + totalUsage.toFixed(2) + ' kWh</div>' +
                '<div><strong>Average:</strong> ' + avgUsage.toFixed(3) + ' kWh/reading</div>' +
                '<div><strong>Data Points:</strong> ' + data.measurements + '</div>' +
                '<div><strong>Period:</strong> ' + data.days + ' days</div>' +
                '</div>';
            
            document.getElementById('usage-stats').innerHTML = statsHTML;
        }
        
        function showUsageLoading() {
            // Destroy existing chart
            if (usageChart) {
                usageChart.destroy();
            }
            
            // Show loading spinner
            const chartContainer = document.querySelector('.chart-container');
            chartContainer.innerHTML = '<div class="usage-loading"><div class="usage-spinner"></div><div>Loading usage data...</div></div>';
            
            // Clear stats
            document.getElementById('usage-stats').innerHTML = '';
        }
        
        function showUsageError(message) {
            // Destroy existing chart
            if (usageChart) {
                usageChart.destroy();
            }
            
            // Show error message
            const chartContainer = document.querySelector('.chart-container');
            chartContainer.innerHTML = '<div style="text-align: center; padding: 50px; color: rgba(248, 113, 113, 0.8); font-size: 18px;">' + message + '</div>';
            
            // Clear stats
            document.getElementById('usage-stats').innerHTML = '';
        }
        
        // Initial load
        updateDashboard();
        loadUsageData(7); // Load 7 days of usage data by default
        
        // Auto-refresh every 30 seconds
        setInterval(updateDashboard, 30000);
    </script>
</body>
</html>`

	tmpl := template.Must(template.New("dashboard").Parse(dashboardHTML))
	w.Header().Set("Content-Type", "text/html")
	tmpl.Execute(w, nil)
}