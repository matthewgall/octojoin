# OctoJoin

A comprehensive Go application that monitors Octopus Energy (UK) saving sessions and free electricity periods, with automatic session joining and a real-time web dashboard.

## Overview

OctoJoin connects to the Octopus Energy API to monitor for:
- **Saving Sessions** - Events where customers earn OctoPoints for reducing energy usage during peak demand
- **Free Electricity Sessions** - Periods where electricity usage is completely free

The application automatically joins eligible saving sessions based on your configured points threshold and provides real-time monitoring through both CLI and web interface.

## Usage

### One-Shot Mode (Default)

By default, OctoJoin runs once, checks for new saving sessions, and exits:

```bash
./octojoin -account=<your_account_id> -key=<your_api_key>
```

### Daemon Mode

To run continuously and monitor for new sessions every 10 minutes:

```bash
./octojoin -daemon -account=<your_account_id> -key=<your_api_key>
```

### Web Dashboard

Enable the real-time web dashboard (daemon mode only):

```bash
./octojoin -daemon -web -account=<your_account_id> -key=<your_api_key>
```

Access the dashboard at: `http://localhost:8080`

The web dashboard shows:
- Current OctoPoints balance  
- Feature availability status
- Upcoming saving sessions with live countdowns
- Upcoming free electricity sessions with live countdowns
- Auto-refresh every 30 seconds

### Environment Variables

```bash
export OCTOPUS_ACCOUNT_ID=<your_account_id>
export OCTOPUS_API_KEY=<your_api_key>
./octojoin              # One-shot mode
./octojoin -daemon      # Daemon mode
```

### Configuration File

Create a `config.yaml` file for easier management:

```bash
cp config.example.yaml config.yaml
# Edit config.yaml with your credentials
./octojoin -config=config.yaml
```

### Parameters

- `-config`: Path to configuration file (YAML format)
- `-account`: Your Octopus Energy Account ID (or set `OCTOPUS_ACCOUNT_ID`)
- `-key`: Your Octopus Energy API Key (or set `OCTOPUS_API_KEY`)
- `-daemon`: Run in daemon mode for continuous monitoring
- `-web`: Enable web UI dashboard (requires daemon mode)
- `-port`: Web UI port (default: 8080)
- `-min-points`: Minimum points threshold to join a session (default: 0 = join all sessions)
- `-debug`: Enable debug logging for troubleshooting

**Configuration Precedence:** Command line arguments > Environment variables > Configuration file

### Configuration File Format

```yaml
# OctoJoin Configuration File
account_id: "A-1234ABCD"
api_key: "sk_live_xxxxxxxxxxxxxxxxxxxxxxxxx"
daemon: true
web_ui: true
web_port: 8080
min_points: 100
check_interval_minutes: 15
debug: false
```

**Configuration Options:**
- `account_id`: Your Octopus Energy Account ID
- `api_key`: Your Octopus Energy API Key
- `daemon`: Run in daemon mode (true/false)
- `web_ui`: Enable web dashboard (true/false)
- `web_port`: Web UI port number (integer)
- `min_points`: Minimum points threshold (integer)
- `check_interval_minutes`: Minutes between checks in daemon mode (integer)
- `debug`: Enable debug logging (true/false)

### Usage Examples

```bash
# Basic one-shot check
./octojoin -account=A-1234ABCD -key=sk_live_xxx

# Daemon mode with web dashboard
./octojoin -daemon -web -account=A-1234ABCD -key=sk_live_xxx

# Custom web port and debug mode
./octojoin -daemon -web -port=9000 -debug -account=A-1234ABCD -key=sk_live_xxx

# Only join sessions worth 100+ points
./octojoin -daemon -min-points=100 -account=A-1234ABCD -key=sk_live_xxx

# Use configuration file (recommended)
./octojoin -config=config.yaml

# Environment variables with web UI
export OCTOPUS_ACCOUNT_ID=A-1234ABCD
export OCTOPUS_API_KEY=sk_live_xxx
./octojoin -daemon -web
```

### Finding Your Credentials

1. **Account ID**: Found in your Octopus Energy account dashboard
2. **API Key**: Available in your account settings under the API section

## Features

### Core Functionality
- **One-Shot or Daemon Mode**: Run once and exit (default) or continuously monitor
- **Dual Session Support**: Monitors both saving sessions and free electricity sessions
- **Automatic Opt-in**: Joins eligible saving sessions based on points threshold
- **Free Electricity Detection**: Detects and reports free electricity periods (no joining required)
- **Session Tracking**: Keeps track of known sessions to avoid duplicate processing
- **Future Session Focus**: Only attempts to join sessions that haven't started

### User Interface
- **Real-time Web Dashboard**: Live countdown timers, session status, and auto-refresh
- **Feature Status Display**: Shows which Octopus campaigns you're enrolled in
- **Detailed CLI Logging**: Current points balance, session details, and feature availability
- **Smart Alerting**: Intelligent free electricity alerts to prevent log spam

### Technical Features
- **JWT Token Persistence**: Cached authentication tokens for improved performance
- **GraphQL Integration**: Real-time OctoPoints balance via Octopus GraphQL API
- **Account-Specific State**: Persistent state storage with account isolation
- **Multiple Configuration Methods**: Command line arguments, environment variables, or YAML config file
- **Points Threshold**: Set minimum points required before joining a session (0 = join all)
- **Rate Limiting**: Respects API rate limits with automatic throttling
- **Exponential Backoff**: Automatically retries failed requests with increasing delays
- **Robust Error Handling**: Handles 429, 5xx errors and network issues gracefully
- **Debug Mode**: Comprehensive debug logging for troubleshooting

## Building

```bash
go build -o octojoin
```

## Example Output

```
2025/01/15 14:30:00 Starting Octopus Energy Saving Session Monitor
2025/01/15 14:30:00 Account ID: A-1234ABCD
2025/01/15 14:30:00 API Key: sk_live_...
2025/01/15 14:30:00 Minimum points threshold: 100
2025/01/15 14:30:00 Web UI enabled at http://localhost:8080
2025/01/15 14:30:00 Running in daemon mode - continuous monitoring
2025/01/15 14:30:00 Checking for new sessions...

Feature Status:
✅ Saving Sessions: ENABLED (octoplus + octoplus-saving-sessions)
✅ Free Electricity: ENABLED (free_electricity)

2025/01/15 14:30:01 Current points in wallet: 13,492
2025/01/15 14:30:01 💰 SAVING SESSION FOUND
   Date: Tuesday, Jan 15 at 17:30
   Duration: 1h
   Reward: 150 points
   Starts in 3h
   Meets criteria (150 >= 100 points), attempting to join...
   Successfully joined session!

2025/01/15 14:30:02 🔋 FREE ELECTRICITY SESSION - INITIAL ALERT
   Date: Wednesday, Jan 16 at 10:00
   Duration: 4h
   Starts tomorrow
   No action needed - automatically free!
```

## Notes

### Operation Modes
- **One-Shot Mode**: Runs once and exits, ideal for cron jobs or manual execution
- **Daemon Mode**: Continues running and monitoring until interrupted (Ctrl+C)
- **Web Dashboard**: Real-time monitoring with live countdowns (daemon mode only)

### Session Management
- **Points Filtering**: Saving sessions below the minimum points threshold are automatically skipped
- **Free Electricity**: These sessions are automatic benefits - just use electricity during the period!
- **Smart Alerts**: Free electricity sessions use intelligent alerting to avoid log spam in daemon mode
- Sessions that have already started or ended will be logged but not joined

### State & Caching
- **Persistent State**: Session tracking and JWT tokens stored in `~/.config/octojoin/`
- **Account Isolation**: Multiple accounts can be used without data pollution
- **API Caching**: Reduces API calls with intelligent cache management
- **JWT Persistence**: Authentication tokens cached for improved performance

### Configuration
- **Configuration File**: Store credentials and settings in a YAML file for easy deployment  
- **Multiple Accounts**: Run separate instances for different Octopus accounts
- **Debug Mode**: Use `-debug` flag for detailed troubleshooting information

### Performance
- OctoPoints convert at ~800 points = £1 (rates may vary)
- In daemon mode, the monitoring interval is set to 10 minutes to avoid excessive API calls
- Use one-shot mode with a cron job for regular automated checks without keeping a process running
- Web dashboard auto-refreshes every 30 seconds with smart content updates

### Free Electricity Session Alerts

In daemon mode, free electricity sessions will only alert at key intervals to avoid log spam:
- **Initial Alert**: When first discovered
- **Day-of Alert**: Within 24 hours of start
- **12-hour Reminder**: 12 hours before start
- **6-hour Reminder**: 6 hours before start  
- **Final Alert**: 15 minutes before start or when active

In one-shot mode, relevant sessions are always displayed.

## Disclaimer

This is an unofficial third-party application developed independently. "Octopus Energy" is a trademark of Octopus Energy Group Limited. This application is not affiliated with, endorsed by, or connected to Octopus Energy Group Limited in any way.

The application interacts with publicly documented Octopus Energy APIs for personal account management purposes. Users are responsible for ensuring their use complies with Octopus Energy's terms of service.

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.