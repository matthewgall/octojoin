package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
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
	
	return ws
}

func (ws *WebServer) Start() error {
	log.Printf("Starting web server on %s", ws.server.Addr)
	return ws.server.ListenAndServe()
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

	// Get campaign status
	campaigns, err := ws.monitor.client.getCampaignStatus()
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
		SavingSessions:             upcomingSavingSessions,
		FreeElectricitySessions:    upcomingFreeElectricitySessions,
		CampaignStatus:             campaignStatus,
		LastUpdated:                time.Now(),
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(data)
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
            grid-template-columns: repeat(auto-fit, minmax(400px, 1fr));
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
            
            .header h1 {
                font-size: 2rem;
            }
        }
    </style>
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
        
        <div class="footer">
            <p>Auto-refreshing every 30 seconds</p>
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
                            <div class="status-value">${data.campaign_status.saving_sessions_enabled ? 'Yes' : 'No'}</div>
                            <div>Sessions Available</div>
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
        
        // Initial load
        updateDashboard();
        
        // Auto-refresh every 30 seconds
        setInterval(updateDashboard, 30000);
    </script>
</body>
</html>`

	tmpl := template.Must(template.New("dashboard").Parse(dashboardHTML))
	w.Header().Set("Content-Type", "text/html")
	tmpl.Execute(w, nil)
}