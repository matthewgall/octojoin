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
	"net/http"
	"strings"
	"time"
)

// MetricsCollector collects and exposes metrics in Prometheus format
type MetricsCollector struct {
	client  *OctopusClient
	monitor *SavingSessionMonitor
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(client *OctopusClient, monitor *SavingSessionMonitor) *MetricsCollector {
	return &MetricsCollector{
		client:  client,
		monitor: monitor,
	}
}

// ServeHTTP handles the /metrics endpoint
func (m *MetricsCollector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	
	metrics := m.collectMetrics()
	fmt.Fprint(w, metrics)
}

// collectMetrics gathers all application metrics
func (m *MetricsCollector) collectMetrics() string {
	var metrics strings.Builder
	
	// Add metric descriptions and type definitions
	m.writeMetricHeader(&metrics, "octojoin_info", "gauge", "Build information")
	m.writeMetric(&metrics, "octojoin_info", map[string]string{
		"version":    GetVersion(),
		"user_agent": GetUserAgent(),
	}, 1)
	
	m.writeMetricHeader(&metrics, "octojoin_up", "gauge", "Whether the application is up and running")
	m.writeMetric(&metrics, "octojoin_up", nil, 1)
	
	m.writeMetricHeader(&metrics, "octojoin_last_check_timestamp", "gauge", "Unix timestamp of last successful check")
	m.writeMetric(&metrics, "octojoin_last_check_timestamp", nil, float64(time.Now().Unix()))
	
	// Get account balance
	accountInfo, err := m.client.getAccountInfo()
	if err == nil && accountInfo != nil {
		m.writeMetricHeader(&metrics, "octojoin_account_balance_pounds", "gauge", "Account balance in pounds")
		m.writeMetric(&metrics, "octojoin_account_balance_pounds", nil, accountInfo.Balance)
	}

	// Get current session data
	sessions, err := m.client.GetSavingSessionsWithCache(m.monitor.state)
	if err == nil && sessions != nil {
		// OctoPoints metrics
		m.writeMetricHeader(&metrics, "octojoin_octopoints_total", "gauge", "Total OctoPoints in wallet")
		m.writeMetric(&metrics, "octojoin_octopoints_total", nil, float64(sessions.Data.OctoPoints.Account.CurrentPointsInWallet))
		
		// Saving sessions metrics
		m.writeMetricHeader(&metrics, "octojoin_saving_sessions_total", "gauge", "Total number of joined saving sessions")
		m.writeMetric(&metrics, "octojoin_saving_sessions_total", nil, float64(len(sessions.Data.SavingSessions.Account.JoinedEvents)))
		
		// Campaign enrollment status
		m.writeMetricHeader(&metrics, "octojoin_campaign_enrolled", "gauge", "Whether enrolled in saving sessions campaign (1=yes, 0=no)")
		enrolled := 0
		if sessions.Data.SavingSessions.Account.HasJoinedCampaign {
			enrolled = 1
		}
		m.writeMetric(&metrics, "octojoin_campaign_enrolled", nil, float64(enrolled))
	}
	
	// Get campaign status
	campaigns, err := m.client.getCampaignStatusWithCache(m.monitor.state)
	if err == nil {
		m.writeMetricHeader(&metrics, "octojoin_campaign_status", "gauge", "Campaign enrollment status by type")
		for campaign, enrolled := range campaigns {
			value := 0
			if enrolled {
				value = 1
			}
			m.writeMetric(&metrics, "octojoin_campaign_status", map[string]string{
				"campaign": campaign,
			}, float64(value))
		}
	}
	
	// Wheel of Fortune spins (with caching)
	spins, err := m.client.getWheelOfFortuneSpinsWithCache(m.monitor.state)
	if err == nil && spins != nil {
		m.writeMetricHeader(&metrics, "octojoin_wheel_spins_total", "gauge", "Available Wheel of Fortune spins by fuel type")
		m.writeMetric(&metrics, "octojoin_wheel_spins_total", map[string]string{
			"fuel_type": "electricity",
		}, float64(spins.ElectricitySpins))
		m.writeMetric(&metrics, "octojoin_wheel_spins_total", map[string]string{
			"fuel_type": "gas",
		}, float64(spins.GasSpins))
		
		m.writeMetricHeader(&metrics, "octojoin_wheel_spins_combined", "gauge", "Total combined Wheel of Fortune spins")
		m.writeMetric(&metrics, "octojoin_wheel_spins_combined", nil, float64(spins.ElectricitySpins+spins.GasSpins))
	}
	
	// Free electricity sessions
	freeElectricity, err := m.client.GetFreeElectricitySessions()
	if err == nil && freeElectricity != nil {
		now := time.Now()
		upcomingSessions := 0
		for _, session := range freeElectricity.Data {
			if session.EndAt.After(now) {
				upcomingSessions++
			}
		}
		
		m.writeMetricHeader(&metrics, "octojoin_free_electricity_sessions_total", "gauge", "Total number of free electricity sessions")
		m.writeMetric(&metrics, "octojoin_free_electricity_sessions_total", nil, float64(len(freeElectricity.Data)))
		
		m.writeMetricHeader(&metrics, "octojoin_free_electricity_sessions_upcoming", "gauge", "Number of upcoming free electricity sessions")
		m.writeMetric(&metrics, "octojoin_free_electricity_sessions_upcoming", nil, float64(upcomingSessions))
	}
	
	// State metrics
	if m.monitor.state != nil {
		m.writeMetricHeader(&metrics, "octojoin_known_sessions_total", "gauge", "Total number of known sessions in state")
		m.writeMetric(&metrics, "octojoin_known_sessions_total", nil, float64(len(m.monitor.state.KnownSessions)))
		
		m.writeMetricHeader(&metrics, "octojoin_last_updated_timestamp", "gauge", "Unix timestamp of last state update")
		m.writeMetric(&metrics, "octojoin_last_updated_timestamp", nil, float64(m.monitor.state.LastUpdated.Unix()))
		
		// Cache metrics
		if m.monitor.state.CachedSavingSessions != nil {
			m.writeMetricHeader(&metrics, "octojoin_cache_age_seconds", "gauge", "Age of cached data in seconds")
			cacheAge := time.Since(m.monitor.state.CachedSavingSessions.Timestamp).Seconds()
			m.writeMetric(&metrics, "octojoin_cache_age_seconds", map[string]string{
				"cache_type": "saving_sessions",
			}, cacheAge)
		}
		
		if m.monitor.state.CachedCampaignStatus != nil {
			cacheAge := time.Since(m.monitor.state.CachedCampaignStatus.Timestamp).Seconds()
			m.writeMetric(&metrics, "octojoin_cache_age_seconds", map[string]string{
				"cache_type": "campaign_status",
			}, cacheAge)
		}
		
		if m.monitor.state.CachedFreeElectricity != nil {
			cacheAge := time.Since(m.monitor.state.CachedFreeElectricity.Timestamp).Seconds()
			m.writeMetric(&metrics, "octojoin_cache_age_seconds", map[string]string{
				"cache_type": "free_electricity",
			}, cacheAge)
		}
	}
	
	return metrics.String()
}

// writeMetricHeader writes metric description and type
func (m *MetricsCollector) writeMetricHeader(sb *strings.Builder, name, metricType, description string) {
	sb.WriteString(fmt.Sprintf("# HELP %s %s\n", name, description))
	sb.WriteString(fmt.Sprintf("# TYPE %s %s\n", name, metricType))
}

// writeMetric writes a metric with optional labels
func (m *MetricsCollector) writeMetric(sb *strings.Builder, name string, labels map[string]string, value float64) {
	if len(labels) > 0 {
		var labelPairs []string
		for key, val := range labels {
			labelPairs = append(labelPairs, fmt.Sprintf(`%s="%s"`, key, val))
		}
		sb.WriteString(fmt.Sprintf("%s{%s} %g\n", name, strings.Join(labelPairs, ","), value))
	} else {
		sb.WriteString(fmt.Sprintf("%s %g\n", name, value))
	}
}