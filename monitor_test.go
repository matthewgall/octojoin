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
	"testing"
	"time"
)

func TestShouldJoinSession(t *testing.T) {
	tests := []struct {
		name         string
		session      SavingSession
		threshold    int
		expectedJoin bool
	}{
		{
			name: "Turn-up free electricity session (0 pts) with threshold 0 joins",
			session: SavingSession{
				EventID:    101,
				Code:       "TURN_UP_1",
				OctoPoints: 0,
			},
			threshold:    0,
			expectedJoin: true,
		},
		{
			name: "Turn-up free electricity session (0 pts) with threshold > 0 skips",
			session: SavingSession{
				EventID:    102,
				Code:       "TURN_UP_2",
				OctoPoints: 0,
			},
			threshold:    100,
			expectedJoin: false,
		},
		{
			name: "Paid saving session meeting threshold joins",
			session: SavingSession{
				EventID:    103,
				Code:       "TURN_DOWN_1",
				OctoPoints: 85,
			},
			threshold:    50,
			expectedJoin: true,
		},
		{
			name: "Paid saving session below threshold skips",
			session: SavingSession{
				EventID:    104,
				Code:       "TURN_DOWN_2",
				OctoPoints: 40,
			},
			threshold:    50,
			expectedJoin: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := &SavingSessionMonitor{
				minPointsThreshold: tc.threshold,
			}
			actual := m.shouldJoinSession(tc.session)
			if actual != tc.expectedJoin {
				t.Errorf("expected shouldJoinSession=%v, got %v", tc.expectedJoin, actual)
			}
		})
	}
}

func TestGetSmartIntervalAt(t *testing.T) {
	loc, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatalf("failed to load Europe/London timezone: %v", err)
	}

	tests := []struct {
		name                 string
		now                  time.Time
		useSmartIntervals    bool
		checkInterval        time.Duration
		consecutiveEmptyChecks int
		lastNewSessionTime   time.Time
		expected             time.Duration
	}{
		{
			name:              "smart intervals disabled uses configured check interval",
			now:               time.Date(2026, 8, 6, 20, 0, 0, 0, loc),
			useSmartIntervals: false,
			checkInterval:     5 * time.Minute,
			expected:          5 * time.Minute,
		},
		{
			name:              "peak announcement window on weekday afternoon",
			now:               time.Date(2026, 8, 6, 14, 30, 0, 0, loc),
			useSmartIntervals: true,
			expected:          IntervalPeakAnnouncement,
		},
		{
			name:              "business hours on weekday morning",
			now:               time.Date(2026, 8, 6, 9, 18, 0, 0, loc),
			useSmartIntervals: true,
			expected:          IntervalBusinessHours,
		},
		{
			name:              "off-peak hours with no empty checks",
			now:               time.Date(2026, 8, 9, 20, 0, 0, 0, loc), // Sunday
			useSmartIntervals: true,
			expected:          IntervalOffPeak,
		},
		{
			name:                   "off-peak hours with consecutive empty checks extends interval",
			now:                    time.Date(2026, 8, 9, 20, 0, 0, 0, loc), // Sunday
			useSmartIntervals:      true,
			consecutiveEmptyChecks: 2,
			expected:               25 * time.Minute, // 15 + 2*5
		},
		{
			name:                   "off-peak hours with many empty checks caps at off-peak interval",
			now:                    time.Date(2026, 8, 9, 20, 0, 0, 0, loc), // Sunday
			useSmartIntervals:      true,
			consecutiveEmptyChecks: 100,
			expected:               IntervalOffPeak,
		},
		{
			name:              "recently found sessions overrides off-peak backoff",
			now:               time.Date(2026, 8, 9, 20, 0, 0, 0, loc),
			useSmartIntervals: true,
			lastNewSessionTime: time.Date(2026, 8, 9, 19, 45, 0, 0, loc),
			expected:          IntervalPeakAnnouncement,
		},
		{
			name:              "recently found sessions overrides business hours",
			now:               time.Date(2026, 8, 6, 9, 18, 0, 0, loc),
			useSmartIntervals: true,
			lastNewSessionTime: time.Date(2026, 8, 6, 9, 0, 0, 0, loc),
			expected:          IntervalPeakAnnouncement,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := &SavingSessionMonitor{
				useSmartIntervals:      tc.useSmartIntervals,
				checkInterval:          tc.checkInterval,
				consecutiveEmptyChecks: tc.consecutiveEmptyChecks,
				lastNewSessionTime:     tc.lastNewSessionTime,
				logger:                 NewLogger(false),
			}

			actual := m.getSmartIntervalAt(tc.now)
			if actual != tc.expected {
				t.Errorf("expected interval %v, got %v", tc.expected, actual)
			}
		})
	}
}
