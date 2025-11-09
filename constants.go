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

import "time"

// Cache durations - tuned based on real-world Octopus Energy API update patterns
const (
	// CacheDurationMeterDevices - Smart meter device list rarely changes
	CacheDurationMeterDevices = 7 * 24 * time.Hour

	// CacheDurationUsageMeasurements - Usage data updated every 30 minutes for balance between freshness and API limits
	CacheDurationUsageMeasurements = 30 * time.Minute

	// CacheDurationWheelSpins - Wheel of Fortune spins refresh daily, check twice per day
	CacheDurationWheelSpins = 12 * time.Hour

	// CacheDurationAccountInfo - Account balance updates hourly
	CacheDurationAccountInfo = 1 * time.Hour

	// CacheDurationCampaignStatus - Campaign enrollment status changes rarely
	CacheDurationCampaignStatus = 24 * time.Hour

	// CacheDurationOctoPoints - OctoPoints balance changes slowly, updated after wheel spins
	CacheDurationOctoPoints = 1 * time.Hour

	// CacheDurationFreeElectricity - Free electricity sessions from static JSON, check frequently
	CacheDurationFreeElectricity = 5 * time.Minute

	// CacheDurationSavingSessionsOffPeak - Saving sessions cache during off-peak hours
	CacheDurationSavingSessionsOffPeak = 2 * time.Hour

	// CacheDurationSavingSessionsPeak - Saving sessions cache during peak announcement window (2-4 PM)
	CacheDurationSavingSessionsPeak = 10 * time.Minute

	// CacheDurationSavingSessionsBusiness - Saving sessions cache during business hours
	CacheDurationSavingSessionsBusiness = 30 * time.Minute
)

// Smart interval durations - check frequency based on UK business hours and announcement patterns
const (
	// IntervalPeakAnnouncement - Peak announcement window (2-4 PM UK weekdays) when new sessions typically announced
	IntervalPeakAnnouncement = 5 * time.Minute

	// IntervalBusinessHours - Business hours (9 AM-6 PM UK weekdays) for occasional announcements
	IntervalBusinessHours = 10 * time.Minute

	// IntervalOffPeak - Off-peak hours (evenings/weekends) when announcements are rare
	IntervalOffPeak = 30 * time.Minute

	// IntervalEventDrivenBase - Base interval for event-driven backoff after finding new sessions
	IntervalEventDrivenBase = 15 * time.Minute

	// IntervalEventDrivenIncrement - Increment added per consecutive empty check (up to IntervalOffPeak max)
	IntervalEventDrivenIncrement = 5 * time.Minute

	// IntervalAfterNewSession - Check more frequently for 30 minutes after finding new sessions (batch detection)
	IntervalAfterNewSession = 30 * time.Minute
)

// JWT token settings
const (
	// JWTRefreshBuffer - Refresh JWT tokens this many minutes before expiry
	JWTRefreshBuffer = 5 * time.Minute
)

// HTTP client settings
const (
	// HTTPClientTimeout - Maximum time for HTTP requests
	HTTPClientTimeout = 30 * time.Second

	// HTTPMinInterval - Minimum time between API requests (rate limiting)
	HTTPMinInterval = 1 * time.Second

	// HTTPMaxRetries - Maximum number of retries for failed requests
	HTTPMaxRetries = 3
)

// Wheel of Fortune settings
const (
	// WheelSpinDelay - Delay between consecutive wheel spins to respect API rate limits
	WheelSpinDelay = 1 * time.Second
)

// Web dashboard settings
const (
	// WebDashboardRefreshInterval - Auto-refresh interval for web dashboard (client-side)
	WebDashboardRefreshInterval = 30 * time.Second

	// WebMaxUsageDays - Maximum number of days of usage data that can be requested
	WebMaxUsageDays = 30

	// WebDefaultUsageDays - Default number of days shown in usage graph
	WebDefaultUsageDays = 7
)

// UK business hours for smart interval calculation
const (
	// UKPeakAnnouncementStartHour - Start of peak announcement window (2 PM)
	UKPeakAnnouncementStartHour = 14

	// UKPeakAnnouncementEndHour - End of peak announcement window (4 PM)
	UKPeakAnnouncementEndHour = 16

	// UKBusinessHoursStartHour - Start of business hours (9 AM)
	UKBusinessHoursStartHour = 9

	// UKBusinessHoursEndHour - End of business hours (6 PM)
	UKBusinessHoursEndHour = 18
)

// Octopus Energy API error codes
const (
	// OctopusErrorCodeJWTExpired - JWT token has expired
	OctopusErrorCodeJWTExpired = "KT-CT-1139"

	// OctopusErrorCodeInvalidAuth - Invalid authorization header
	OctopusErrorCodeInvalidAuth = "KT-CT-1143"
)

// State management settings
const (
	// StateMaxWheelSpinHistory - Maximum number of wheel spin records to keep in history
	StateMaxWheelSpinHistory = 100

	// StateCleanupAge - Clean up alert states older than this duration
	StateCleanupAge = 7 * 24 * time.Hour
)

// Free electricity alert intervals - multi-stage alerting to prevent spam
const (
	// AlertIntervalFinal - Alert when session starts in 15 minutes or less
	AlertIntervalFinal = 15 * time.Minute

	// AlertIntervalSixHour - Alert 6 hours before session
	AlertIntervalSixHour = 6 * time.Hour

	// AlertIntervalTwelveHour - Alert 12 hours before session
	AlertIntervalTwelveHour = 12 * time.Hour

	// AlertIntervalDayOf - Alert on day of session (24 hours before)
	AlertIntervalDayOf = 24 * time.Hour
)

// Display thresholds for time formatting
const (
	// DisplayThreshold24Hours - Show days format after 24 hours
	DisplayThreshold24Hours = 24 * time.Hour
)

// Monitor settings
const (
	// MonitorDefaultCheckInterval - Default check interval when smart intervals disabled
	MonitorDefaultCheckInterval = 15 * time.Minute
)
