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
	"context"
	"fmt"
	"time"
)

type FreeElectricityAlertState struct {
	Code          string
	InitialAlert  bool
	DayOfAlert    bool
	TwelveHourAlert bool
	SixHourAlert  bool
	FinalAlert    bool
}

type SavingSessionMonitor struct {
	client               *OctopusClient
	state                *AppState
	accountID            string
	checkInterval        time.Duration
	stopCh               chan struct{}
	minPointsThreshold   int
	webServer            *WebServer
	useSmartIntervals    bool
	consecutiveEmptyChecks int
	lastNewSessionTime   time.Time
	logger               *Logger
	daemonMode           bool // true if running with web UI
}

func NewSavingSessionMonitor(client *OctopusClient, accountID string) *SavingSessionMonitor {
	logger := NewLogger(client.debug).WithComponent("monitor").WithAccountID(accountID)

	state, err := LoadState(accountID)
	if err != nil {
		logger.Warn("Failed to load state, starting fresh", "error", err.Error())
		state = &AppState{
			AlertStates:                  make(map[string]*FreeElectricityAlertState),
			KnownSessions:                make(map[int]bool),
			KnownFreeElectricitySessions: make(map[string]bool),
		}
	}

	// Clean up expired sessions
	state.CleanupExpiredSessions()

	// Set state on client for JWT token caching
	client.SetState(state)
	
	return &SavingSessionMonitor{
		client:             client,
		state:              state,
		accountID:          accountID,
		checkInterval:      MonitorDefaultCheckInterval,
		stopCh:             make(chan struct{}),
		minPointsThreshold: 0,
		useSmartIntervals:  true,
		logger:             logger,
		daemonMode:         false, // default to standalone mode
	}
}

func (m *SavingSessionMonitor) SetMinPointsThreshold(threshold int) {
	m.minPointsThreshold = threshold
}

func (m *SavingSessionMonitor) SetCheckInterval(interval time.Duration) {
	m.checkInterval = interval
}

func (m *SavingSessionMonitor) SetSmartIntervals(enabled bool) {
	m.useSmartIntervals = enabled
}

// SetDaemonMode sets whether the monitor is running in daemon mode with web UI
func (m *SavingSessionMonitor) SetDaemonMode(enabled bool) {
	m.daemonMode = enabled
}

// getSmartInterval returns an intelligent check interval based on UK time and context
func (m *SavingSessionMonitor) getSmartInterval() time.Duration {
	if !m.useSmartIntervals {
		return m.checkInterval
	}
	
	// Load UK timezone
	ukLocation, err := time.LoadLocation("Europe/London")
	if err != nil {
		// Fallback to UTC if timezone loading fails
		ukLocation = time.UTC
	}
	
	now := time.Now().In(ukLocation)
	hour := now.Hour()
	weekday := now.Weekday()

	// Recently found new sessions - check more frequently for a batch
	if !m.lastNewSessionTime.IsZero() && time.Since(m.lastNewSessionTime) < IntervalAfterNewSession {
		return IntervalPeakAnnouncement
	}

	// Peak announcement window (2-4 PM UK time, weekdays)
	if hour >= UKPeakAnnouncementStartHour && hour < UKPeakAnnouncementEndHour && weekday >= time.Monday && weekday <= time.Friday {
		return IntervalPeakAnnouncement
	}

	// Business hours (9 AM - 6 PM, weekdays)
	if hour >= UKBusinessHoursStartHour && hour < UKBusinessHoursEndHour && weekday >= time.Monday && weekday <= time.Friday {
		return IntervalBusinessHours
	}

	// Event-driven backoff based on consecutive empty checks
	if m.consecutiveEmptyChecks > 0 {
		// Gradually increase intervals after consecutive empty checks (up to off-peak max)
		backoffMinutes := int(IntervalEventDrivenBase.Minutes()) + (int(IntervalEventDrivenIncrement.Minutes()) * m.consecutiveEmptyChecks)
		maxMinutes := int(IntervalOffPeak.Minutes())
		if backoffMinutes > maxMinutes {
			backoffMinutes = maxMinutes
		}
		return time.Duration(backoffMinutes) * time.Minute
	}

	// Off-peak hours (evenings, nights, weekends)
	return IntervalOffPeak
}

func (m *SavingSessionMonitor) EnableWebUI(port int) {
	m.webServer = NewWebServer(m, port)
}

func (m *SavingSessionMonitor) Start() {
	// Legacy method for backward compatibility
	ctx := context.Background()
	_ = m.StartWithContext(ctx)
}

func (m *SavingSessionMonitor) StartWithContext(ctx context.Context) error {
	m.logger.Info("Starting saving session monitoring")
	if m.useSmartIntervals {
		m.logger.Info("Smart interval adjustment enabled")
	}

	// Start web server if enabled
	if m.webServer != nil {
		go func() {
			if err := m.webServer.StartWithContext(ctx); err != nil && err != context.Canceled {
				m.logger.Error("Web server error", "error", err.Error())
			}
		}()
	}

	// Initial check
	m.checkForNewSessions()

	// Dynamic interval monitoring
	for {
		interval := m.getSmartInterval()
		timer := time.NewTimer(interval)

		if m.useSmartIntervals {
			m.logger.Debug("Next check scheduled", "interval", m.formatDuration(interval))
		}

		select {
		case <-timer.C:
			m.checkForNewSessions()
		case <-m.stopCh:
			timer.Stop()
			m.logger.Info("Stopping saving session monitoring")
			return nil
		case <-ctx.Done():
			timer.Stop()
			m.logger.Info("Stopping saving session monitoring (context canceled)")
			// Stop web server gracefully
			if m.webServer != nil {
				m.webServer.Stop()
			}
			return ctx.Err()
		}

		timer.Stop()
	}
}

func (m *SavingSessionMonitor) Stop() {
	close(m.stopCh)
}

func (m *SavingSessionMonitor) checkForNewSessions() {
	m.logger.Info("Checking for new sessions")

	foundNewSessions := false

	// Check saving sessions
	if m.checkSavingSessions() {
		foundNewSessions = true
	}

	// Check free electricity sessions
	if m.checkFreeElectricitySessions() {
		foundNewSessions = true
	}

	// Update event-driven tracking
	if foundNewSessions {
		m.lastNewSessionTime = time.Now()
		m.consecutiveEmptyChecks = 0
		if m.useSmartIntervals {
			m.logger.Info("New sessions found - will check more frequently for potential batches")
		}
	} else {
		m.consecutiveEmptyChecks++
		if m.useSmartIntervals && m.consecutiveEmptyChecks > 1 {
			m.logger.Info("No new sessions found - extending next interval",
				"consecutive_empty_checks", m.consecutiveEmptyChecks,
			)
		}
	}

	// Save state after checks
	if err := m.state.Save(m.accountID); err != nil {
		m.logger.Warn("Failed to save state", "error", err.Error())
	}
}

func (m *SavingSessionMonitor) checkSavingSessions() bool {
	response, err := m.client.GetSavingSessionsWithCache(m.state)
	if err != nil {
		m.logger.Error("Error fetching saving sessions", "error", err.Error())
		return false
	}

	foundNewSessions := false

	m.logger.Info("Current OctoPoints balance",
		"points", response.Data.OctoPoints.Account.CurrentPointsInWallet,
	)

	// Get and display Wheel of Fortune spins (with caching)
	spins, err := m.client.getWheelOfFortuneSpinsWithCache(m.state)
	if err != nil {
		m.logger.Warn("Could not get Wheel of Fortune spins", "error", err.Error())
	} else {
		totalSpins := spins.ElectricitySpins + spins.GasSpins
		if totalSpins > 0 {
			m.logger.Info("Wheel of Fortune spins available",
				"total", totalSpins,
				"electricity", spins.ElectricitySpins,
				"gas", spins.GasSpins,
			)

			// Auto-spin all available wheels
			m.logger.Info("Auto-spinning all available wheels")
			results, err := m.client.spinAllAvailableWheels(spins)
			if err != nil {
				m.logger.Error("Error during auto-spinning", "error", err.Error())
			} else if len(results) > 0 {
				totalPoints := 0
				electricityPoints := 0
				gasPoints := 0

				for _, result := range results {
					totalPoints += result.Prize
					if result.FuelType == "ELECTRICITY" {
						electricityPoints += result.Prize
					} else {
						gasPoints += result.Prize
					}
				}

				m.logger.Info("Auto-spin complete",
					"total_points", totalPoints,
					"electricity_points", electricityPoints,
					"gas_points", gasPoints,
				)
				
				// Clear the cached spins so we check for new ones on next run
				if m.state != nil {
					m.state.CachedWheelOfFortuneSpins = nil
				}
			} else {
				m.logger.Warn("No wheels were successfully spun")
			}
		} else {
			m.logger.Debug("No Wheel of Fortune spins available")
		}
	}

	for _, session := range response.Data.SavingSessions.Account.JoinedEvents {
		if !m.state.KnownSessions[session.EventID] {
			foundNewSessions = true
			now := time.Now()
			duration := session.EndAt.Sub(session.StartAt)

			if session.StartAt.After(now) {
				// Upcoming session
				timeUntil := session.StartAt.Sub(now)

				// Use user-friendly output in standalone mode, structured logging in daemon mode
				if m.daemonMode {
					m.logger.Info("SAVING SESSION FOUND",
						"event_id", session.EventID,
						"date", session.StartAt.Format("Monday, Jan 2"),
						"time", session.StartAt.Format("15:04"),
						"duration", m.formatDuration(duration),
						"reward_points", session.OctoPoints,
						"starts_in", m.formatTimeUntil(timeUntil),
					)
				} else {
					m.logger.UserMessage("🎉 SAVING SESSION FOUND")
					m.logger.UserMessage("   Date: %s at %s", session.StartAt.Format("Monday, Jan 2"), session.StartAt.Format("15:04"))
					m.logger.UserMessage("   Duration: %s", m.formatDuration(duration))
					m.logger.UserMessage("   Reward: %d OctoPoints", session.OctoPoints)
					m.logger.UserMessage("   Starts in %s", m.formatTimeUntil(timeUntil))
				}

				if m.shouldJoinSession(session) {
					if m.daemonMode {
						m.logger.Info("Attempting to join session",
							"event_id", session.EventID,
							"points", session.OctoPoints,
							"threshold", m.minPointsThreshold,
						)
					} else {
						m.logger.UserMessage("   Joining session (meets threshold of %d points)", m.minPointsThreshold)
					}
					if err := m.joinSession(session.EventID); err != nil {
						m.logger.Error("Failed to join session",
							"event_id", session.EventID,
							"error", err.Error(),
						)
					} else {
						m.logger.Info("Successfully joined session", "event_id", session.EventID)
					}
				} else {
					m.logger.Info("Skipped session - insufficient points",
						"event_id", session.EventID,
						"points", session.OctoPoints,
						"threshold", m.minPointsThreshold,
					)
				}
			} else {
				m.logger.Debug("Saving session already started/ended",
					"event_id", session.EventID,
				)
			}

			m.state.KnownSessions[session.EventID] = true
		}
	}

	if len(response.Data.SavingSessions.Account.JoinedEvents) == 0 {
		m.logger.Debug("No saving sessions found")
	}
	
	return foundNewSessions
}

func (m *SavingSessionMonitor) checkFreeElectricitySessions() bool {
	response, err := m.client.GetFreeElectricitySessionsWithCache(m.state)
	if err != nil {
		m.logger.Error("Error fetching free electricity sessions", "error", err.Error())
		return false
	}

	currentSessionsFound := 0
	foundNewSessions := false
	for _, session := range response.Data {
		now := time.Now()
		
		// Skip sessions that have already ended
		if session.EndAt.Before(now) {
			continue
		}
		
		// Check if this is a new session
		if !m.state.KnownFreeElectricitySessions[session.Code] {
			foundNewSessions = true
		}
		
		// Track that we've seen this session
		m.state.KnownFreeElectricitySessions[session.Code] = true
		currentSessionsFound++
		
		// Check if we should alert
		var timeUntil time.Duration
		if session.StartAt.After(now) {
			timeUntil = session.StartAt.Sub(now)
		} else {
			timeUntil = 0 // Currently active
		}
		
		shouldAlert, alertType := m.shouldAlert(session, timeUntil)
		if !shouldAlert {
			continue
		}
		
		// Display the appropriate alert
		duration := session.EndAt.Sub(session.StartAt)
		
		if session.StartAt.Before(now) && session.EndAt.After(now) {
			// Currently active
			timeLeft := session.EndAt.Sub(now)
			if m.daemonMode {
				m.logger.Info("FREE ELECTRICITY SESSION ACTIVE NOW",
					"time_remaining", m.formatTimeUntil(timeLeft),
					"ends_at", session.EndAt.Format("15:04"),
				)
			} else {
				m.logger.UserMessage("⚡ FREE ELECTRICITY SESSION ACTIVE NOW!")
				m.logger.UserMessage("   Your electricity is currently FREE")
				m.logger.UserMessage("   Time remaining: %s", m.formatTimeUntil(timeLeft))
				m.logger.UserMessage("   Ends at %s", session.EndAt.Format("15:04"))
			}
		} else {
			// Upcoming session
			startsIn := ""
			if timeUntil < DisplayThreshold24Hours {
				startsIn = m.formatTimeUntil(timeUntil)
			} else {
				startsIn = m.formatDaysUntil(timeUntil)
			}
			if m.daemonMode {
				m.logger.Info("FREE ELECTRICITY SESSION FOUND",
					"alert_type", alertType,
					"date", session.StartAt.Format("Monday, Jan 2"),
					"time", session.StartAt.Format("15:04"),
					"duration", m.formatDuration(duration),
					"starts_in", startsIn,
				)
			} else {
				m.logger.UserMessage("🔋 FREE ELECTRICITY SESSION - %s", alertType)
				m.logger.UserMessage("   Date: %s at %s", session.StartAt.Format("Monday, Jan 2"), session.StartAt.Format("15:04"))
				m.logger.UserMessage("   Duration: %s", m.formatDuration(duration))
				m.logger.UserMessage("   Starts in %s", startsIn)
				m.logger.UserMessage("   No action needed - automatically free!")
			}
		}
	}

	if currentSessionsFound == 0 {
		m.logger.Debug("No current or upcoming free electricity sessions found")
	}
	
	return foundNewSessions
}

func (m *SavingSessionMonitor) CheckOnce() {
	m.displayCampaignStatus()
	m.checkForNewSessions()
}

func (m *SavingSessionMonitor) displayCampaignStatus() {
	campaigns, err := m.client.getCampaignStatusWithCache(m.state)
	if err != nil {
		m.logger.Warn("Could not check campaign status", "error", err.Error())
		return
	}

	// Check saving sessions requirements
	octoplusOK := campaigns["octoplus"]
	savingSessionsOK := campaigns["octoplus-saving-sessions"]
	freeElectricityOK := campaigns["free_electricity"]

	if m.daemonMode {
		// Structured logging for daemon mode
		m.logger.Info("Feature Status Check",
			"saving_sessions_enabled", octoplusOK && savingSessionsOK,
			"has_octoplus", octoplusOK,
			"has_saving_sessions", savingSessionsOK,
			"free_electricity_enabled", freeElectricityOK,
		)

		if octoplusOK && savingSessionsOK {
			m.logger.Info("Saving Sessions: ENABLED", "campaigns", "octoplus + octoplus-saving-sessions")
		} else {
			missingCampaigns := []string{}
			if !octoplusOK {
				missingCampaigns = append(missingCampaigns, "octoplus")
			}
			if !savingSessionsOK {
				missingCampaigns = append(missingCampaigns, "octoplus-saving-sessions")
			}
			m.logger.Info("Saving Sessions: DISABLED", "missing_campaigns", missingCampaigns)
		}

		if freeElectricityOK {
			m.logger.Info("Free Electricity: ENABLED", "campaign", "free_electricity")
		} else {
			m.logger.Info("Free Electricity: DISABLED", "missing_campaign", "free_electricity")
		}
	} else {
		// User-friendly output for standalone mode
		m.logger.UserMessage("Feature Status:")

		if octoplusOK && savingSessionsOK {
			m.logger.UserMessage("✅ Saving Sessions: ENABLED (octoplus + octoplus-saving-sessions)")
		} else {
			m.logger.UserMessage("❌ Saving Sessions: DISABLED")
			if !octoplusOK {
				m.logger.UserMessage("   Missing: octoplus campaign")
			}
			if !savingSessionsOK {
				m.logger.UserMessage("   Missing: octoplus-saving-sessions campaign")
			}
			m.logger.UserMessage("   To enable: Sign up for OctoPlus and Saving Sessions at octopus.energy")
		}

		if freeElectricityOK {
			m.logger.UserMessage("✅ Free Electricity: ENABLED (free_electricity)")
		} else {
			m.logger.UserMessage("❌ Free Electricity: DISABLED")
			m.logger.UserMessage("   Missing: free_electricity campaign")
			m.logger.UserMessage("   To enable: Sign up for Free Electricity sessions at octopus.energy")
		}

		m.logger.UserMessage("")
	}
}

func (m *SavingSessionMonitor) shouldJoinSession(session SavingSession) bool {
	return session.OctoPoints >= m.minPointsThreshold
}

func (m *SavingSessionMonitor) joinSession(eventID int) error {
	return m.client.JoinSavingSession(eventID)
}

func (m *SavingSessionMonitor) formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	
	if hours > 0 && minutes > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	} else if hours > 0 {
		return fmt.Sprintf("%dh", hours)
	} else {
		return fmt.Sprintf("%dm", minutes)
	}
}

func (m *SavingSessionMonitor) formatTimeUntil(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	
	if hours > 0 && minutes > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	} else if hours > 0 {
		return fmt.Sprintf("%dh", hours)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm", minutes)
	} else {
		return "less than a minute"
	}
}

func (m *SavingSessionMonitor) formatDaysUntil(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	
	if days > 1 {
		if hours > 0 {
			return fmt.Sprintf("in %d days %dh", days, hours)
		} else {
			return fmt.Sprintf("in %d days", days)
		}
	} else if days == 1 {
		if hours > 0 {
			return fmt.Sprintf("tomorrow (%dh from now)", int(d.Hours()))
		} else {
			return "tomorrow"
		}
	} else {
		return m.formatTimeUntil(d)
	}
}

func (m *SavingSessionMonitor) shouldAlert(session FreeElectricitySession, timeUntil time.Duration) (bool, string) {
	code := session.Code
	now := time.Now()
	
	// Initialize alert state if not exists
	if _, exists := m.state.AlertStates[code]; !exists {
		m.state.AlertStates[code] = &FreeElectricityAlertState{
			Code: code,
		}
	}
	
	alert := m.state.AlertStates[code]
	
	// Check if session has ended - cleanup alert state
	if session.EndAt.Before(now) {
		delete(m.state.AlertStates, code)
		return false, ""
	}
	
	// Currently active - only alert once
	if session.StartAt.Before(now) && session.EndAt.After(now) {
		if !alert.FinalAlert {
			alert.FinalAlert = true
			return true, "ACTIVE NOW"
		}
		return false, ""
	}
	
	// Upcoming session - check intervals
	if timeUntil <= AlertIntervalFinal && !alert.FinalAlert {
		alert.FinalAlert = true
		return true, "STARTING SOON"
	} else if timeUntil <= AlertIntervalSixHour && !alert.SixHourAlert {
		alert.SixHourAlert = true
		return true, "6-HOUR REMINDER"
	} else if timeUntil <= AlertIntervalTwelveHour && !alert.TwelveHourAlert {
		alert.TwelveHourAlert = true
		return true, "12-HOUR REMINDER"
	} else if timeUntil <= AlertIntervalDayOf && !alert.DayOfAlert {
		alert.DayOfAlert = true
		return true, "DAY-OF REMINDER"
	} else if !alert.InitialAlert {
		alert.InitialAlert = true
		return true, "INITIAL ALERT"
	}
	
	return false, ""
}