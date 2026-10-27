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
