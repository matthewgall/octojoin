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

## System Service Installation (Linux)

For continuous monitoring on Linux servers, you can install OctoJoin as a systemd service:

### Installation Steps

1. **Create system user and directories:**
   ```bash
   sudo useradd --system --create-home --home-dir /var/lib/octojoin --shell /bin/false octojoin
   sudo mkdir -p /opt/octojoin
   ```

2. **Install the binary:**
   ```bash
   sudo cp octojoin /opt/octojoin/
   sudo chown root:root /opt/octojoin/octojoin
   sudo chmod 755 /opt/octojoin/octojoin
   ```

3. **Create configuration:**
   ```bash
   sudo cp config.example.yaml /opt/octojoin/config.yaml
   sudo chown root:octojoin /opt/octojoin/config.yaml
   sudo chmod 640 /opt/octojoin/config.yaml
   # Edit the configuration with your credentials
   sudo nano /opt/octojoin/config.yaml
   ```

4. **Install and start the service:**
   ```bash
   sudo cp octojoin.service /etc/systemd/system/
   sudo systemctl daemon-reload
   sudo systemctl enable octojoin
   sudo systemctl start octojoin
   ```

5. **Check service status:**
   ```bash
   sudo systemctl status octojoin
   sudo journalctl -u octojoin -f  # Follow logs
   ```

### Service Management

```bash
# Start/stop/restart the service
sudo systemctl start octojoin
sudo systemctl stop octojoin
sudo systemctl restart octojoin

# Check status and logs
sudo systemctl status octojoin
sudo journalctl -u octojoin --since "1 hour ago"

# Disable/enable auto-start
sudo systemctl disable octojoin
sudo systemctl enable octojoin
```

### Web Dashboard Access

If you enabled the web UI in `/etc/octojoin/config.yaml`, access it at:
- `http://your-server:8080` (or your configured port)

### Prometheus Metrics

The web server exposes Prometheus-compatible metrics at `/metrics` for monitoring with Grafana, Prometheus, etc.

**Available metrics:**
- `octojoin_info` - Build and version information
- `octojoin_up` - Application uptime status
- `octojoin_account_balance_pounds` - Current account balance in pounds
- `octojoin_octopoints_total` - Current OctoPoints balance
- `octojoin_saving_sessions_total` - Number of joined saving sessions
- `octojoin_campaign_enrolled` - Campaign enrollment status
- `octojoin_campaign_status{campaign="name"}` - Status by campaign type
- `octojoin_wheel_spins_total{fuel_type="gas|electricity"}` - Wheel of Fortune spins by fuel type
- `octojoin_wheel_spins_combined` - Total combined Wheel of Fortune spins
- `octojoin_free_electricity_sessions_total` - Total free electricity sessions
- `octojoin_free_electricity_sessions_upcoming` - Upcoming free electricity sessions
- `octojoin_known_sessions_total` - Total sessions tracked in state
- `octojoin_cache_age_seconds{cache_type="name"}` - Age of cached data in seconds

**Example Grafana queries:**
```promql
# Account balance over time
octojoin_account_balance_pounds

# OctoPoints balance over time
octojoin_octopoints_total

# Wheel of Fortune spins availability
sum(octojoin_wheel_spins_total)

# Cache hit efficiency
octojoin_cache_age_seconds < 300
```

### Security Notes

- The service runs as a dedicated `octojoin` user with restricted permissions
- Configuration files should be readable only by root and the octojoin group
- State files are stored in `/var/lib/octojoin/` with appropriate permissions
- The service uses systemd security features like `NoNewPrivileges` and `ProtectSystem`

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
- **Prometheus Metrics**: `/metrics` endpoint for monitoring with Grafana, Prometheus, etc.

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

## Support the Project

If you find OctoJoin useful, here are some ways to support its continued development:

### 💷 Join Octopus Energy

Not an Octopus Energy customer yet? Use my referral link to join and we'll both get £50 credit:

**[Join Octopus Energy - Get £50 credit](https://share.octopus.energy/maize-ape-570)**

This helps fund development of OctoJoin and you'll get access to:
- Saving Sessions (earn money for reducing usage during peak times)
- Free electricity sessions (completely free electricity during certain periods)
- Competitive energy rates and excellent customer service
- The greenest energy supplier in the UK

### ❤️ GitHub Sponsor

Support ongoing development and maintenance:

**[Become a GitHub Sponsor](https://github.com/sponsors/matthewgall)**

Your sponsorship helps with:
- Adding new features and improvements
- Maintaining compatibility with API changes  
- Providing support and bug fixes
- Keeping the project free and open source

### ⭐ Star the Repository

Show your appreciation by starring the repository on GitHub - it helps others discover the project!

## Disclaimer

This is an unofficial third-party application developed independently. "Octopus Energy" is a trademark of Octopus Energy Group Limited. This application is not affiliated with, endorsed by, or connected to Octopus Energy Group Limited in any way.

The application interacts with publicly documented Octopus Energy APIs for personal account management purposes. Users are responsible for ensuring their use complies with Octopus Energy's terms of service.

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.