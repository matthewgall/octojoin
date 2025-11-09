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
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	AccountID        string `yaml:"account_id"`
	APIKey           string `yaml:"api_key"`
	Daemon           bool   `yaml:"daemon"`
	MinPoints        int    `yaml:"min_points"`
	CheckInterval    int    `yaml:"check_interval_minutes"`
	WebUI            bool   `yaml:"web_ui"`
	WebPort          int    `yaml:"web_port"`
	Debug            bool   `yaml:"debug"`
	NoSmartIntervals bool   `yaml:"no_smart_intervals"`
}

func LoadConfig(configPath string) (*Config, error) {
	config := &Config{
		Daemon:        false,
		MinPoints:     0,
		CheckInterval: 10,
		WebUI:         false,
		WebPort:       8080,
		Debug:         false,
	}

	if configPath == "" {
		return config, nil
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file does not exist: %s", configPath)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, nil
}

func (c *Config) ApplyDefaults() {
	if c.CheckInterval <= 0 {
		c.CheckInterval = 10
	}
	if c.WebPort <= 0 {
		c.WebPort = 8080
	}
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	var errors []string

	// Validate account ID
	if c.AccountID == "" {
		errors = append(errors, "account ID is required")
	} else if !strings.HasPrefix(c.AccountID, "A-") {
		errors = append(errors, fmt.Sprintf("account ID should start with 'A-', got: %s", c.AccountID))
	} else if len(c.AccountID) < 3 {
		errors = append(errors, fmt.Sprintf("account ID appears too short: %s", c.AccountID))
	}

	// Validate API key
	if c.APIKey == "" {
		errors = append(errors, "API key is required")
	} else if !strings.HasPrefix(c.APIKey, "sk_live_") {
		errors = append(errors, "API key should start with 'sk_live_' (use your live API key, not test key)")
	} else if len(c.APIKey) < 20 {
		errors = append(errors, "API key appears too short (should be ~40+ characters)")
	}

	// Validate web port
	if c.WebPort < 1 || c.WebPort > 65535 {
		errors = append(errors, fmt.Sprintf("web port must be between 1-65535, got: %d", c.WebPort))
	}
	if c.WebPort < 1024 && c.WebPort != 0 {
		errors = append(errors, fmt.Sprintf("warning: port %d requires root privileges (consider using 8080 or higher)", c.WebPort))
	}

	// Validate check interval
	if c.CheckInterval < 1 {
		errors = append(errors, fmt.Sprintf("check interval must be at least 1 minute, got: %d", c.CheckInterval))
	}
	if c.CheckInterval > 1440 {
		errors = append(errors, fmt.Sprintf("check interval seems too long (%d minutes = %.1f hours), consider using a shorter interval", c.CheckInterval, float64(c.CheckInterval)/60.0))
	}

	// Validate min points
	if c.MinPoints < 0 {
		errors = append(errors, fmt.Sprintf("min points cannot be negative, got: %d", c.MinPoints))
	}
	if c.MinPoints > 10000 {
		errors = append(errors, fmt.Sprintf("warning: min points threshold very high (%d), you may miss most sessions", c.MinPoints))
	}

	// Logical validations
	if c.WebUI && !c.Daemon {
		errors = append(errors, "web UI requires daemon mode (use both -daemon and -web flags)")
	}

	if len(errors) > 0 {
		return fmt.Errorf("configuration validation failed:\n  - %s", strings.Join(errors, "\n  - "))
	}

	return nil
}