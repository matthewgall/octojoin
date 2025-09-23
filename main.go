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
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"
)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func getEstimatedCost(m UsageMeasurement) float64 {
	if len(m.MetaData.Statistics) > 0 {
		if val, err := strconv.ParseFloat(m.MetaData.Statistics[0].CostInclTax.EstimatedAmount, 64); err == nil {
			return val
		}
	}
	return 0.0
}

func main() {
	var accountID, apiKey, configPath string
	var daemon, webUI, debug, showVersion, noSmartIntervals, testMeter bool
	var minPoints, webPort int
	
	flag.StringVar(&configPath, "config", "", "Path to configuration file")
	flag.BoolVar(&showVersion, "version", false, "Show version information")
	flag.StringVar(&accountID, "account", os.Getenv("OCTOPUS_ACCOUNT_ID"), "Octopus Energy Account ID")
	flag.StringVar(&apiKey, "key", os.Getenv("OCTOPUS_API_KEY"), "Octopus Energy API Key")
	flag.BoolVar(&daemon, "daemon", false, "Run in daemon mode (continuous monitoring)")
	flag.BoolVar(&webUI, "web", false, "Enable web UI dashboard (daemon mode only)")
	flag.BoolVar(&debug, "debug", false, "Enable debug logging")
	flag.IntVar(&minPoints, "min-points", 0, "Minimum points threshold to join a session (0 = join all sessions)")
	flag.IntVar(&webPort, "port", 8080, "Web UI port (default: 8080)")
	flag.BoolVar(&noSmartIntervals, "no-smart-intervals", false, "Disable smart interval adjustment (use fixed intervals)")
	flag.BoolVar(&testMeter, "test-meter", false, "Test smart meter data retrieval and exit")
	flag.Parse()

	// Handle version flag
	if showVersion {
		fmt.Printf("octojoin %s\n", GetVersion())
		fmt.Printf("User-Agent: %s\n", GetUserAgent())
		os.Exit(0)
	}

	// Load configuration file if provided
	config, err := LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Error loading config file: %v", err)
	}
	config.ApplyDefaults()

	// Command line arguments and environment variables override config file
	if accountID == "" && config.AccountID != "" {
		accountID = config.AccountID
	}
	if apiKey == "" && config.APIKey != "" {
		apiKey = config.APIKey
	}
	if !daemon && config.Daemon {
		daemon = config.Daemon
	}
	if !webUI && config.WebUI {
		webUI = config.WebUI
	}
	if !debug && config.Debug {
		debug = config.Debug
	}
	if minPoints == 0 && config.MinPoints != 0 {
		minPoints = config.MinPoints
	}
	if webPort == 8080 && config.WebPort != 8080 && config.WebPort > 0 {
		webPort = config.WebPort
	}

	if accountID == "" || apiKey == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s -account=<account_id> -key=<api_key>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Or set environment variables: OCTOPUS_ACCOUNT_ID and OCTOPUS_API_KEY\n")
		fmt.Fprintf(os.Stderr, "Or use a configuration file with -config=<path>\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	log.Printf("Starting Octopus Energy Saving Session Monitor")
	log.Printf("Account ID: %s", accountID)
	log.Printf("API Key: %s...", apiKey[:8]) // Only show first 8 chars for security

	// Initialize API client
	client := NewOctopusClient(accountID, apiKey, debug)
	
	// Handle meter testing flag
	if testMeter {
		log.Println("Testing smart meter data retrieval...")
		
		// Initialize state for caching
		monitor := NewSavingSessionMonitor(client, accountID)
		
		// Test meter device discovery
		devices, err := client.getSmartMeterDevicesWithCache(monitor.state)
		if err != nil {
			log.Fatalf("Failed to get meter devices: %v", err)
		}
		
		log.Printf("✅ Found %d ESME devices:", len(devices))
		for i, device := range devices {
			log.Printf("   %d. %s", i+1, device)
		}
		
		if len(devices) > 0 {
			// Test usage measurements for last 7 days
			measurements, err := client.getUsageMeasurementsWithCache(monitor.state, 7)
			if err != nil {
				log.Fatalf("Failed to get usage measurements: %v", err)
			}
			
			log.Printf("✅ Retrieved %d usage measurements for last 7 days", len(measurements))
			if len(measurements) > 0 {
				// Show sample measurements
				log.Printf("Sample measurements:")
				for i, m := range measurements[:min(5, len(measurements))] {
					log.Printf("   %d. %s: %.3f %s (Cost: £%.4f)", 
						i+1, m.StartAt.Format("2006-01-02 15:04"), 
						m.GetValueAsFloat64(), m.Unit,
						getEstimatedCost(m))
				}
			}
		}
		
		log.Println("Meter testing completed successfully!")
		return
	}
	
	// Initialize monitor
	monitor := NewSavingSessionMonitor(client, accountID)
	monitor.SetMinPointsThreshold(minPoints)
	
	// Configure smart intervals (command line flag takes precedence over config)
	disableSmartIntervals := noSmartIntervals || config.NoSmartIntervals
	monitor.SetSmartIntervals(!disableSmartIntervals)
	
	// Set custom check interval if specified in config
	if config.CheckInterval > 0 && config.CheckInterval != 10 {
		monitor.SetCheckInterval(time.Duration(config.CheckInterval) * time.Minute)
		log.Printf("Using custom check interval: %d minutes", config.CheckInterval)
	}
	
	// Enable web UI if requested and in daemon mode
	if webUI && daemon {
		monitor.EnableWebUI(webPort)
		log.Printf("Web UI enabled at http://localhost:%d", webPort)
	} else if webUI && !daemon {
		log.Printf("Warning: Web UI can only be enabled in daemon mode")
	}
	
	if minPoints > 0 {
		log.Printf("Minimum points threshold: %d", minPoints)
	} else {
		log.Printf("No minimum points threshold - will join all sessions")
	}
	
	if daemon {
		log.Printf("Running in daemon mode - continuous monitoring")
		monitor.Start()
	} else {
		log.Printf("Running in one-shot mode")
		monitor.CheckOnce()
	}
}