package main

import (
	"fmt"
	"os"
	"gopkg.in/yaml.v3"
)

type Config struct {
	AccountID    string `yaml:"account_id"`
	APIKey       string `yaml:"api_key"`
	Daemon       bool   `yaml:"daemon"`
	MinPoints    int    `yaml:"min_points"`
	CheckInterval int   `yaml:"check_interval_minutes"`
	WebUI        bool   `yaml:"web_ui"`
	WebPort      int    `yaml:"web_port"`
	Debug        bool   `yaml:"debug"`
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