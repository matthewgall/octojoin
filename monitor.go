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
	"fmt"
	"log"
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
	client             *OctopusClient
	state              *AppState
	accountID          string
	checkInterval      time.Duration
	stopCh             chan struct{}
	minPointsThreshold int
	webServer          *WebServer
}

func NewSavingSessionMonitor(client *OctopusClient, accountID string) *SavingSessionMonitor {
	state, err := LoadState(accountID)
	if err != nil {
		log.Printf("Failed to load state, starting fresh: %v", err)
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
		checkInterval:      10 * time.Minute,
		stopCh:             make(chan struct{}),
		minPointsThreshold: 0,
	}
}

func (m *SavingSessionMonitor) SetMinPointsThreshold(threshold int) {
	m.minPointsThreshold = threshold
}

func (m *SavingSessionMonitor) SetCheckInterval(interval time.Duration) {
	m.checkInterval = interval
}

func (m *SavingSessionMonitor) EnableWebUI(port int) {
	m.webServer = NewWebServer(m, port)
}

func (m *SavingSessionMonitor) Start() {
	log.Println("Starting saving session monitoring...")
	
	// Start web server if enabled
	if m.webServer != nil {
		go func() {
			if err := m.webServer.Start(); err != nil {
				log.Printf("Web server error: %v", err)
			}
		}()
	}
	
	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	m.checkForNewSessions()

	for {
		select {
		case <-ticker.C:
			m.checkForNewSessions()
		case <-m.stopCh:
			log.Println("Stopping saving session monitoring...")
			return
		}
	}
}

func (m *SavingSessionMonitor) Stop() {
	close(m.stopCh)
}

func (m *SavingSessionMonitor) checkForNewSessions() {
	log.Println("Checking for new sessions...")
	
	// Check saving sessions
	m.checkSavingSessions()
	
	// Check free electricity sessions
	m.checkFreeElectricitySessions()
	
	// Save state after checks
	if err := m.state.Save(m.accountID); err != nil {
		log.Printf("Warning: Failed to save state: %v", err)
	}
}

func (m *SavingSessionMonitor) checkSavingSessions() {
	response, err := m.client.GetSavingSessionsWithCache(m.state)
	if err != nil {
		log.Printf("Error fetching saving sessions: %v", err)
		return
	}

	log.Printf("Current points in wallet: %d", response.Data.OctoPoints.Account.CurrentPointsInWallet)

	for _, session := range response.Data.SavingSessions.Account.JoinedEvents {
		if !m.state.KnownSessions[session.EventID] {
			now := time.Now()
			duration := session.EndAt.Sub(session.StartAt)
			
			if session.StartAt.After(now) {
				// Upcoming session
				timeUntil := session.StartAt.Sub(now)
				log.Printf("💰 SAVING SESSION FOUND")
				log.Printf("   Date: %s at %s", 
					session.StartAt.Format("Monday, Jan 2"), 
					session.StartAt.Format("15:04"))
				log.Printf("   Duration: %s", m.formatDuration(duration))
				log.Printf("   Reward: %d points", session.OctoPoints)
				
				if timeUntil < 24*time.Hour {
					log.Printf("   Starts in %s", m.formatTimeUntil(timeUntil))
				} else {
					log.Printf("   Starts %s", m.formatDaysUntil(timeUntil))
				}

				if m.shouldJoinSession(session) {
					log.Printf("   Meets criteria (%d >= %d points), attempting to join...", 
						session.OctoPoints, m.minPointsThreshold)
					if err := m.joinSession(session.EventID); err != nil {
						log.Printf("   Failed to join: %v", err)
					} else {
						log.Printf("   Successfully joined session!")
					}
				} else {
					log.Printf("   Skipped - insufficient points (%d < %d minimum)", 
						session.OctoPoints, m.minPointsThreshold)
				}
			} else {
				log.Printf("⏰ Saving session %d has already started/ended - not joining", session.EventID)
			}

			m.state.KnownSessions[session.EventID] = true
		}
	}

	if len(response.Data.SavingSessions.Account.JoinedEvents) == 0 {
		log.Println("No saving sessions found")
	}
}

func (m *SavingSessionMonitor) checkFreeElectricitySessions() {
	response, err := m.client.GetFreeElectricitySessionsWithCache(m.state)
	if err != nil {
		log.Printf("Error fetching free electricity sessions: %v", err)
		return
	}

	currentSessionsFound := 0
	for _, session := range response.Data {
		now := time.Now()
		
		// Skip sessions that have already ended
		if session.EndAt.Before(now) {
			continue
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
			log.Printf("⚡ FREE ELECTRICITY SESSION ACTIVE NOW!")
			log.Printf("   Your electricity is currently FREE")
			log.Printf("   Time remaining: %s", m.formatTimeUntil(timeLeft))
			log.Printf("   Ends at %s", session.EndAt.Format("15:04"))
		} else {
			// Upcoming session
			log.Printf("🔋 FREE ELECTRICITY SESSION - %s", alertType)
			log.Printf("   Date: %s at %s", 
				session.StartAt.Format("Monday, Jan 2"), 
				session.StartAt.Format("15:04"))
			log.Printf("   Duration: %s", m.formatDuration(duration))
			if timeUntil < 24*time.Hour {
				log.Printf("   Starts in %s", m.formatTimeUntil(timeUntil))
			} else {
				log.Printf("   Starts %s", m.formatDaysUntil(timeUntil))
			}
			log.Printf("   No action needed - automatically free!")
		}
	}

	if currentSessionsFound == 0 {
		log.Println("No current or upcoming free electricity sessions found")
	}
}

func (m *SavingSessionMonitor) CheckOnce() {
	m.displayCampaignStatus()
	m.checkForNewSessions()
}

func (m *SavingSessionMonitor) displayCampaignStatus() {
	campaigns, err := m.client.getCampaignStatus()
	if err != nil {
		log.Printf("Warning: Could not check campaign status: %v", err)
		return
	}

	log.Printf("Feature Status:")
	
	// Check saving sessions requirements
	octoplusOK := campaigns["octoplus"]
	savingSessionsOK := campaigns["octoplus-saving-sessions"]
	freeElectricityOK := campaigns["free_electricity"]
	
	if octoplusOK && savingSessionsOK {
		log.Printf("✅ Saving Sessions: ENABLED (octoplus + octoplus-saving-sessions)")
	} else {
		log.Printf("❌ Saving Sessions: DISABLED")
		if !octoplusOK {
			log.Printf("   Missing: octoplus campaign")
		}
		if !savingSessionsOK {
			log.Printf("   Missing: octoplus-saving-sessions campaign")
		}
		log.Printf("   To enable: Sign up for OctoPlus and Saving Sessions at octopus.energy")
	}
	
	if freeElectricityOK {
		log.Printf("✅ Free Electricity: ENABLED (free_electricity)")
	} else {
		log.Printf("❌ Free Electricity: DISABLED")
		log.Printf("   Missing: free_electricity campaign")
		log.Printf("   To enable: Sign up for Free Electricity sessions at octopus.energy")
	}
	
	log.Printf("")
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
	if timeUntil <= 15*time.Minute && !alert.FinalAlert {
		alert.FinalAlert = true
		return true, "STARTING SOON"
	} else if timeUntil <= 6*time.Hour && !alert.SixHourAlert {
		alert.SixHourAlert = true
		return true, "6-HOUR REMINDER"
	} else if timeUntil <= 12*time.Hour && !alert.TwelveHourAlert {
		alert.TwelveHourAlert = true
		return true, "12-HOUR REMINDER"
	} else if timeUntil <= 24*time.Hour && !alert.DayOfAlert {
		alert.DayOfAlert = true
		return true, "DAY-OF REMINDER"
	} else if !alert.InitialAlert {
		alert.InitialAlert = true
		return true, "INITIAL ALERT"
	}
	
	return false, ""
}