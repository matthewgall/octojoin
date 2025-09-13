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
	"runtime/debug"
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