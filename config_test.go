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
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "test_config.yaml")

	// Create test config file
	configContent := `account_id: test-account-123
api_key: test-api-key-456
daemon: true
min_points: 100
check_interval_minutes: 5
web_ui: true
web_port: 9090
debug: true
`

	err := os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	// Test loading config
	config, err := LoadConfig(configFile)
	if err != nil {
		t.Errorf("Expected no error loading config, got %v", err)
	}

	if config.AccountID != "test-account-123" {
		t.Errorf("Expected AccountID 'test-account-123', got %s", config.AccountID)
	}

	if config.APIKey != "test-api-key-456" {
		t.Errorf("Expected APIKey 'test-api-key-456', got %s", config.APIKey)
	}

	if !config.Daemon {
		t.Error("Expected Daemon to be true")
	}

	if config.MinPoints != 100 {
		t.Errorf("Expected MinPoints 100, got %d", config.MinPoints)
	}

	if config.CheckInterval != 5 {
		t.Errorf("Expected CheckInterval 5, got %d", config.CheckInterval)
	}

	if !config.WebUI {
		t.Error("Expected WebUI to be true")
	}

	if config.WebPort != 9090 {
		t.Errorf("Expected WebPort 9090, got %d", config.WebPort)
	}

	if !config.Debug {
		t.Error("Expected Debug to be true")
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	// Test loading with empty config path to get defaults
	config, err := LoadConfig("")
	if err != nil {
		t.Errorf("Expected no error loading with empty config path, got %v", err)
	}

	// Check default values
	if config.CheckInterval != 10 {
		t.Errorf("Expected default CheckInterval 10, got %d", config.CheckInterval)
	}

	if config.WebUI != false {
		t.Error("Expected default WebUI to be false")
	}

	if config.WebPort != 8080 {
		t.Errorf("Expected default WebPort 8080, got %d", config.WebPort)
	}

	if config.Daemon != false {
		t.Error("Expected default Daemon to be false")
	}

	if config.MinPoints != 0 {
		t.Errorf("Expected default MinPoints 0, got %d", config.MinPoints)
	}

	if config.Debug != false {
		t.Error("Expected default Debug to be false")
	}
}

func TestLoadConfigInvalidYAML(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "invalid_config.yaml")

	// Create invalid YAML file
	invalidYAML := `account_id: test
api_key: [invalid: yaml: content
debug: true`

	err := os.WriteFile(configFile, []byte(invalidYAML), 0644)
	if err != nil {
		t.Fatalf("Failed to write invalid config file: %v", err)
	}

	// Test loading invalid config
	_, err = LoadConfig(configFile)
	if err == nil {
		t.Error("Expected error loading invalid YAML config, got nil")
	}
}

func TestConfigStruct(t *testing.T) {
	config := Config{
		AccountID:     "test-account",
		APIKey:        "test-key",
		Daemon:        true,
		MinPoints:     50,
		CheckInterval: 15,
		WebUI:         true,
		WebPort:       4000,
		Debug:         true,
	}

	if config.AccountID != "test-account" {
		t.Errorf("Expected AccountID 'test-account', got %s", config.AccountID)
	}

	if config.APIKey != "test-key" {
		t.Errorf("Expected APIKey 'test-key', got %s", config.APIKey)
	}

	if !config.Daemon {
		t.Error("Expected Daemon to be true")
	}

	if config.MinPoints != 50 {
		t.Errorf("Expected MinPoints 50, got %d", config.MinPoints)
	}

	if config.CheckInterval != 15 {
		t.Errorf("Expected CheckInterval 15, got %d", config.CheckInterval)
	}

	if !config.WebUI {
		t.Error("Expected WebUI to be true")
	}

	if config.WebPort != 4000 {
		t.Errorf("Expected WebPort 4000, got %d", config.WebPort)
	}

	if !config.Debug {
		t.Error("Expected Debug to be true")
	}
}

func TestConfigApplyDefaults(t *testing.T) {
	config := Config{
		AccountID:     "test",
		APIKey:        "test",
		CheckInterval: 0,    // Should be set to default
		WebPort:       0,    // Should be set to default
	}

	config.ApplyDefaults()

	if config.CheckInterval != 10 {
		t.Errorf("Expected CheckInterval to default to 10, got %d", config.CheckInterval)
	}

	if config.WebPort != 8080 {
		t.Errorf("Expected WebPort to default to 8080, got %d", config.WebPort)
	}

	// Test with valid values (should not change)
	config2 := Config{
		AccountID:     "test",
		APIKey:        "test", 
		CheckInterval: 5,
		WebPort:       3000,
	}

	config2.ApplyDefaults()

	if config2.CheckInterval != 5 {
		t.Errorf("Expected CheckInterval to remain 5, got %d", config2.CheckInterval)
	}

	if config2.WebPort != 3000 {
		t.Errorf("Expected WebPort to remain 3000, got %d", config2.WebPort)
	}
}