package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"
)

func main() {
	var accountID, apiKey, configPath string
	var daemon, webUI, debug bool
	var minPoints, webPort int
	
	flag.StringVar(&configPath, "config", "", "Path to configuration file")
	flag.StringVar(&accountID, "account", os.Getenv("OCTOPUS_ACCOUNT_ID"), "Octopus Energy Account ID")
	flag.StringVar(&apiKey, "key", os.Getenv("OCTOPUS_API_KEY"), "Octopus Energy API Key")
	flag.BoolVar(&daemon, "daemon", false, "Run in daemon mode (continuous monitoring)")
	flag.BoolVar(&webUI, "web", false, "Enable web UI dashboard (daemon mode only)")
	flag.BoolVar(&debug, "debug", false, "Enable debug logging")
	flag.IntVar(&minPoints, "min-points", 0, "Minimum points threshold to join a session (0 = join all sessions)")
	flag.IntVar(&webPort, "port", 8080, "Web UI port (default: 8080)")
	flag.Parse()

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
	
	// Initialize monitor
	monitor := NewSavingSessionMonitor(client, accountID)
	monitor.SetMinPointsThreshold(minPoints)
	
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