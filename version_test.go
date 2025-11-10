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
	"testing"

	"golang.org/x/mod/semver"
)

func TestVersionComparison(t *testing.T) {
	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int // -1: v1 < v2, 0: v1 == v2, 1: v1 > v2
	}{
		{
			name:     "same version",
			v1:       "v1.5.0",
			v2:       "v1.5.0",
			expected: 0,
		},
		{
			name:     "major version difference",
			v1:       "v1.5.0",
			v2:       "v2.0.0",
			expected: -1,
		},
		{
			name:     "minor version difference",
			v1:       "v1.5.0",
			v2:       "v1.6.0",
			expected: -1,
		},
		{
			name:     "patch version difference",
			v1:       "v1.5.1",
			v2:       "v1.5.2",
			expected: -1,
		},
		{
			name:     "double digit minor version",
			v1:       "v1.9.0",
			v2:       "v1.10.0",
			expected: -1,
		},
		{
			name:     "newer major version",
			v1:       "v2.0.0",
			v2:       "v1.9.0",
			expected: 1,
		},
		{
			name:     "newer minor version",
			v1:       "v1.10.0",
			v2:       "v1.9.0",
			expected: 1,
		},
		{
			name:     "prerelease version",
			v1:       "v1.5.0-beta",
			v2:       "v1.5.0",
			expected: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := semver.Compare(tt.v1, tt.v2)
			if result != tt.expected {
				t.Errorf("semver.Compare(%s, %s) = %d, want %d", tt.v1, tt.v2, result, tt.expected)
			}
		})
	}
}

func TestGetVersion(t *testing.T) {
	v := GetVersion()
	if v == "" {
		t.Error("GetVersion() should not return empty string")
	}
}

func TestGetUserAgent(t *testing.T) {
	ua := GetUserAgent()
	if ua == "" {
		t.Error("GetUserAgent() should not return empty string")
	}
	if !contains(ua, "matthewgall/octojoin") {
		t.Errorf("GetUserAgent() = %s, should contain 'matthewgall/octojoin'", ua)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
