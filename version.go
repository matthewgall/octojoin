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
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// These variables are set at build time via -ldflags
var (
	version = "dev"
	commit  = "unknown"
)

// GetVersion returns the application version
func GetVersion() string {
	if version != "dev" {
		return version
	}
	
	// Try to get version from git tags if available
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && len(setting.Value) >= 7 {
				return setting.Value[:7] // Short commit hash
			}
		}
	}
	
	// Fallback to commit variable if set
	if commit != "unknown" && len(commit) >= 7 {
		return commit[:7]
	}
	
	return "dev"
}

// GetUserAgent returns the properly formatted user-agent string
func GetUserAgent() string {
	return fmt.Sprintf("matthewgall/octojoin %s", GetVersion())
}

// GitHubRelease represents a GitHub release
type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	HTMLURL string `json:"html_url"`
}

// CheckForUpdates checks if a newer version is available on GitHub
func CheckForUpdates() (string, string, bool) {
	currentVersion := GetVersion()

	// Skip update check for dev builds, commit hashes, or non-tagged versions
	// Only check for proper semver releases (e.g., v1.2.3)
	if currentVersion == "dev" || !strings.HasPrefix(currentVersion, "v") || len(currentVersion) < 5 {
		return "", "", false
	}

	// Skip if version looks like a commit hash (7+ hex characters without dots)
	if len(currentVersion) <= 7 && !strings.Contains(currentVersion, ".") {
		return "", "", false
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("GET", "https://api.github.com/repos/matthewgall/octojoin/releases/latest", nil)
	if err != nil {
		return "", "", false
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", GetUserAgent())

	resp, err := client.Do(req)
	if err != nil {
		return "", "", false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", false
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", "", false
	}

	// Compare versions using semver.Compare
	// semver.Compare returns 1 if release.TagName > currentVersion
	if semver.Compare(release.TagName, currentVersion) > 0 {
		return release.TagName, release.HTMLURL, true
	}

	return "", "", false
}

// PrintUpdateNotification prints an update notification if available
func PrintUpdateNotification() {
	newVersion, url, available := CheckForUpdates()
	if available {
		fmt.Println()
		fmt.Println("╔════════════════════════════════════════════════════════════════╗")
		fmt.Printf("║  🎉 Update Available: %s → %s%s║\n",
			GetVersion(),
			newVersion,
			strings.Repeat(" ", 30-len(GetVersion())-len(newVersion)))
		fmt.Println("║                                                                ║")
		fmt.Printf("║  Download: %-51s ║\n", url)
		fmt.Println("╔════════════════════════════════════════════════════════════════╗")
		fmt.Println()
	}
}