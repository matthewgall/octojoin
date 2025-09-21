# OctoJoin

A Go application that monitors Octopus Energy (UK) saving sessions and free electricity periods, automatically joining eligible sessions with a real-time web dashboard.

## What it does

- **Saving Sessions**: Automatically joins sessions that earn OctoPoints for reducing energy usage
- **Free Electricity**: Monitors and alerts for periods of completely free electricity  
- **Real-time Dashboard**: Live web interface with countdown timers and session status
- **Smart Monitoring**: Configurable points threshold and continuous daemon mode

## Quick Start

### Basic Usage
```bash
# Run once and exit
./octojoin -account=A-1234ABCD -key=sk_live_xxx

# Continuous monitoring with web dashboard
./octojoin -daemon -web -account=A-1234ABCD -key=sk_live_xxx
```

### Using Environment Variables
```bash
export OCTOPUS_ACCOUNT_ID=A-1234ABCD
export OCTOPUS_API_KEY=sk_live_xxx
./octojoin -daemon -web
```

### Using Configuration File
```bash
cp config.example.yaml config.yaml
# Edit config.yaml with your credentials
./octojoin -config=config.yaml
```

**Web Dashboard**: Access at `http://localhost:8080` (daemon mode only)

## Configuration

### Command Line Options
| Option | Description | Default |
|--------|-------------|---------|
| `-config` | Path to YAML configuration file | - |
| `-account` | Octopus Energy Account ID | `OCTOPUS_ACCOUNT_ID` env var |
| `-key` | Octopus Energy API Key | `OCTOPUS_API_KEY` env var |
| `-daemon` | Run continuously | false |
| `-web` | Enable web dashboard | false |
| `-port` | Web UI port | 8080 |
| `-min-points` | Minimum points to join session | 0 (join all) |
| `-debug` | Enable debug logging | false |

### Configuration File (config.yaml)
```yaml
account_id: "A-1234ABCD"
api_key: "sk_live_xxxxxxxxxx"
daemon: true
web_ui: true
web_port: 8080
min_points: 100
debug: false
```

### Getting Your Credentials
1. **Account ID**: Found in your Octopus Energy dashboard
2. **API Key**: Available in account settings → API section

## Linux System Service

Install as a systemd service for continuous monitoring:

```bash
# 1. Create system user and directories
sudo useradd --system --create-home --home-dir /var/lib/octojoin --shell /bin/false octojoin
sudo mkdir -p /opt/octojoin

# 2. Install binary and config
sudo cp octojoin /opt/octojoin/
sudo cp config.example.yaml /opt/octojoin/config.yaml
sudo chown root:octojoin /opt/octojoin/config.yaml
sudo chmod 640 /opt/octojoin/config.yaml

# 3. Edit configuration
sudo nano /opt/octojoin/config.yaml

# 4. Install and start service
sudo cp octojoin.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now octojoin
```

### Service Management
```bash
sudo systemctl status octojoin          # Check status
sudo journalctl -u octojoin -f          # Follow logs
sudo systemctl restart octojoin         # Restart service
```

## Monitoring & Metrics

### Prometheus Metrics
The web server exposes metrics at `/metrics` for Grafana/Prometheus monitoring:

| Metric | Description |
|--------|-------------|
| `octojoin_account_balance_pounds` | Account balance |
| `octojoin_octopoints_total` | OctoPoints balance |
| `octojoin_saving_sessions_total` | Joined saving sessions |
| `octojoin_wheel_spins_total{fuel_type}` | Wheel of Fortune spins |
| `octojoin_free_electricity_sessions_upcoming` | Upcoming free sessions |
| `octojoin_cache_age_seconds{cache_type}` | Cache age monitoring |

### Example Grafana Queries
```promql
octojoin_account_balance_pounds              # Account balance over time
octojoin_octopoints_total                    # OctoPoints over time  
sum(octojoin_wheel_spins_total)              # Total spins available
octojoin_cache_age_seconds < 300             # Cache freshness check
```

## Features

- **Dual Session Support**: Monitors saving sessions and free electricity periods
- **Automatic Joining**: Joins eligible saving sessions based on points threshold  
- **Real-time Dashboard**: Live web interface with countdown timers
- **Smart Caching**: Intelligent API caching based on real-world update patterns
- **Multiple Run Modes**: One-shot, continuous daemon, or systemd service
- **Robust Error Handling**: JWT token management, exponential backoff, rate limiting
- **Comprehensive Monitoring**: Prometheus metrics for cache effectiveness and system health

## Building

```bash
go build -o octojoin
```

## Example Output

```
2025/01/15 14:30:00 Starting Octopus Energy Saving Session Monitor
2025/01/15 14:30:00 Web UI enabled at http://localhost:8080
2025/01/15 14:30:00 Running in daemon mode - continuous monitoring

Feature Status:
✅ Saving Sessions: ENABLED
✅ Free Electricity: ENABLED

2025/01/15 14:30:01 Current points in wallet: 13,492
2025/01/15 14:30:01 💰 SAVING SESSION FOUND
   Date: Tuesday, Jan 15 at 17:30 • Duration: 1h • Reward: 150 points
   Meets criteria (150 >= 100 points) → Successfully joined!

2025/01/15 14:30:02 🔋 FREE ELECTRICITY SESSION
   Date: Wednesday, Jan 16 at 10:00 • Duration: 4h
   No action needed - automatically free!
```

## How it Works

- **One-Shot Mode**: Run once and exit (default) - ideal for cron jobs
- **Daemon Mode**: Continuous monitoring every 15 minutes  
- **Smart Filtering**: Only joins sessions meeting your points threshold
- **Intelligent Caching**: Optimized API usage based on real-world update patterns
- **State Persistence**: Session tracking stored in `~/.config/octojoin/`
- **Free Electricity Alerts**: Smart alerting at key intervals to avoid spam

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