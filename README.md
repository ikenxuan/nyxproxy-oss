# NyxProxy-OSS

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.27+-00ADD8?logo=go)](https://go.dev/)
[![CI](https://github.com/ikenxuan/nyxproxy-oss/actions/workflows/ci.yml/badge.svg)](https://github.com/ikenxuan/nyxproxy-oss/actions/workflows/ci.yml)
[![Latest Release](https://img.shields.io/github/v/release/ikenxuan/nyxproxy-oss)](https://github.com/ikenxuan/nyxproxy-oss/releases)

**High-Performance Proxy Server with Automatic IPv6 Rotation** - Get a new IPv6 address for every request from your own /64 subnet!

English | [中文](README-ZH.md)

---

## 📋 Table of Contents

- [Why NyxProxy?](#-why-nyxproxy)
- [Key Features](#-key-features)
- [How IPv6 Rotation Works](#-how-ipv6-rotation-works)
- [Quick Start](#-quick-start)
  - [Docker (recommended)](#docker-recommended)
  - [Quick Setup (Debian/Ubuntu)](#quick-setup-debianubuntu)
  - [Uninstall (Debian/Ubuntu)](#uninstall-debianubuntu)
  - [Manual Setup](#manual-setup)
- [Configuration](#-configuration)
- [Running as Daemon (Systemd)](#-running-as-daemon-systemd)
- [Docker Deployment](#-docker-deployment)
- [Web Console](#-web-console)
- [Usage Examples](#-usage-examples)
- [IPv6 Rotation Explained](#-ipv6-rotation-explained)
- [Requirements](#-requirements)
- [Troubleshooting](#-troubleshooting)
- [License](#-license)

---

## 🎯 Why NyxProxy?

NyxProxy is a **proxy server that gives you a different IPv6 address for every request** - automatically!

### The Problem
Most proxies use a single IP address, which makes you:
- ❌ Easy to track across requests
- ❌ Vulnerable to rate limiting
- ❌ Identifiable by fingerprinting
- ❌ Blocked after too many requests

### The Solution: IPv6 Rotation
NyxProxy uses your provider's **IPv6 /64 subnet** (that's 18 quintillion IPs!) to rotate automatically:
- ✅ **Different IP for each request** - appears as different users
- ✅ **No rate limiting** - each IP is "fresh" and unused
- ✅ **Anti-fingerprinting** - impossible to track across requests
- ✅ **Intelligent rotation** - IPs are replaced after 100 uses or 30 minutes
- ✅ **Fast** - pre-populated pool of 200 IPs ready to use

### Real-World Use Cases
- **Web Scraping**: Avoid rate limits by appearing as different users
- **API Testing**: Test with multiple "users" from different IPs
- **Privacy**: Each request appears to come from a different source
- **Load Distribution**: Spread load across multiple IPs

---

## ✨ Key Features

### 🔄 Automatic IPv6 Rotation (Main Feature!)
- **200+ IPv6 addresses** in rotation pool
- **Intelligent rotation**: Replace IPs after 100 uses OR 30 minutes
- **Fully configurable**: Adjust pool size, rotation frequency, and age
- **Background refresh**: Old IPs are automatically replaced
- **No manual intervention**: Set it and forget it

### 🚀 Performance
- **< 100ms per request** - pre-populated IP pool
- **Concurrent connections** - handle thousands of connections
- **Efficient rotation** - no delays or blocking

### 🔧 Flexible Configuration
- **SOCKS5 or HTTPS proxy** - choose your protocol
- **IPv4 + IPv6 support** - dual stack capable
- **Interface selection** - auto-detect or manual
- **Authentication** - username/password protection
- **Monitoring endpoint** - built-in health checks

### 🐧 Linux Native
- **Debian/Ubuntu** - one-command setup
- **Systemd service** - run as daemon
- **Auto-start on boot** - reliable deployment
- **ARM64 support** - Raspberry Pi compatible

### 🐳 Container Ready
- **Multi-arch image** - published to GHCR for `linux/amd64` and `linux/arm64`
- **Automatic prefix handling** - waits for the IPv6 `/64` and restarts on prefix changes
- **Clean shutdown** - removes pool addresses on `SIGTERM`
- **Compose file included** - host networking, `NET_ADMIN`, read-only root filesystem

### 🖥️ Built-in Web Console
- **Browser-based management** - dashboard, configuration editor, and live logs
- **Transactional apply** - invalid changes are rejected; failed applies roll back to the last good config
- **Single binary and single container** - the Next.js UI is embedded; no Node runtime in production
- **Secure by default** - password login, CSRF protection, login rate limiting, optional TLS

---

## 🔍 How IPv6 Rotation Works

### Understanding IPv6 /64 Subnets

When you rent a server with a routed IPv6 block, you typically get:
- 1 primary IPv6 address (e.g., `2a05:f480:1800:25db::1`)
- 1 entire **/64 subnet** (e.g., `2a05:f480:1800:25db::/64`)

**What does /64 mean?**
- The first 64 bits are your subnet prefix
- The last 64 bits can be **any value you want**
- That's `2^64 = 18,446,744,073,709,551,616` possible IPv6 addresses!

### NyxProxy's Smart Rotation

```
Your /64 Subnet: 2a05:f480:1800:25db::/64
                  └─────────┬──────────┘└─can be anything─┘
                      Your prefix (fixed)   Your IPs (18 quintillion!)

NyxProxy generates random IPs:
├─ 2a05:f480:1800:25db:1a2b:3c4d:5e6f:7890  ← Request 1
├─ 2a05:f480:1800:25db:9988:7766:5544:3322  ← Request 2
├─ 2a05:f480:1800:25db:aaaa:bbbb:cccc:dddd  ← Request 3
└─ ... (200 IPs in pool, automatically rotated)
```

### The 3-Stage Process

**Stage 1: Startup (20-30 seconds)**
```
1. Generate 200 random IPs from your /64 subnet
2. Add each IP to your network interface
3. Ready! All IPs can be used immediately
```

**Stage 2: Request Handling (< 100ms)**
```
1. New proxy request comes in
2. Pick next IP from pool (round-robin)
3. Bind outgoing connection to this IP
4. Track usage (count++, lastUsed = now)
```

**Stage 3: Background Rotation (every 5 minutes)**
```
Check each IP:
  - Used 100+ times? → Replace with fresh IP
  - Older than 30min? → Replace with fresh IP
  - Otherwise: Keep it
```

---

## 🚀 Quick Start

### Docker (recommended)

```bash
cp .env.docker.example .env.docker
# edit .env.docker: set IFACE, PROXY_PASS, and WEB_PASS

docker compose pull
docker compose up -d
docker compose logs -f nyxproxy
```

Open `http://YOUR_SERVER:18081` and sign in with `WEB_USER` and `WEB_PASS`
from `.env.docker`.

The compose file pulls `ghcr.io/ikenxuan/nyxproxy-oss:latest` by default. See
[Docker Deployment](#-docker-deployment) for details and the
[Chinese README](README-ZH.md) for a full walkthrough.

### Quick Setup (Debian/Ubuntu)

The easiest way to get started — pipe the installer straight into `bash`:

```bash
curl -fsSL https://raw.githubusercontent.com/ikenxuan/nyxproxy-oss/main/scripts/quick-setup.sh | sudo bash
```

The script runs in your current session and prompts on the terminal, so nothing
is left behind in the current directory. The release binary is downloaded into
a temporary directory and verified against `sha256sums.txt` when that file is
available, then installed to `/usr/local/bin/nyxproxy`.

**That's it!** The script will:
1. Auto-detect your network interface and IPv6 subnet
2. Install and configure ndppd (NDP proxy daemon)
3. Set kernel parameters for IPv6 routing
4. Create `/etc/nyxproxy/config.yaml` (mode 0600) with your settings
5. **Download the latest NyxProxy binary** from GitHub releases
6. **Optionally install as systemd service** (daemon mode)
7. **Optionally start the proxy immediately**

Installed files:

| Path | Purpose |
| --- | --- |
| `/usr/local/bin/nyxproxy` | Binary |
| `/etc/nyxproxy/config.yaml` | Configuration |
| `/etc/systemd/system/nyxproxy.service` | Service unit (optional) |
| `/etc/sysctl.d/99-nyxproxy-ipv6.conf` | Kernel parameters |
| `/etc/ndppd.conf` | ndppd configuration (previous file backed up to `/etc/ndppd.conf.nyxproxy.bak`) |
| `/var/lib/nyxproxy/install-state` | Rollback data used by `--uninstall` |

**During setup you'll be asked:**
- Proxy username (default: admin)
- Proxy password (required)
- IP pool size (default: 200)
- Max uses per IP (default: 100)
- IP age limit in minutes (default: 30)
- Enable web console? (y/N)
- Web console username, password, and port (default: 18081)
- Install as systemd service? (y/N)
- Start NyxProxy now? (Y/n)

**Expected output:**
```
✓ IPv6 rotation mode: IP Pool with dynamic rotation
  Interface: enp1s0
  Subnet: 2a05:f480:1800:25db::/64
  Pool size: 200 IPs
  Rotation: Every 100 uses or 30m0s
  Initializing IP pool...
  Progress: 50/200 IPs added
  Progress: 100/200 IPs added
  Progress: 150/200 IPs added
  Progress: 200/200 IPs added
✓ IP pool ready with 200 addresses
✓ Background IP rotation started

Starting https proxy on 0.0.0.0:8080 (Protocol: IPv6)
```

### Uninstall (Debian/Ubuntu)

The same script removes everything it installed. Pass `--uninstall` after
`-s --`:

```bash
curl -fsSL https://raw.githubusercontent.com/ikenxuan/nyxproxy-oss/main/scripts/quick-setup.sh | sudo bash -s -- --uninstall
```

The uninstaller:

- stops and removes the `nyxproxy` service and any running `nyxproxy` process
- deletes `/usr/local/bin/nyxproxy`, `/etc/nyxproxy`, and `/var/lib/nyxproxy`
- removes `/etc/sysctl.d/99-nyxproxy-ipv6.conf` and restores the kernel values
  that were active before installation
- restores the previous `/etc/ndppd.conf` from
  `/etc/ndppd.conf.nyxproxy.bak`, or removes the generated one
- removes the `ndppd` package only if the installer installed it

Use `--keep-config` to keep `/etc/nyxproxy/config.yaml`, and `--yes` to skip
the confirmation prompt:

```bash
curl -fsSL https://raw.githubusercontent.com/ikenxuan/nyxproxy-oss/main/scripts/quick-setup.sh | sudo bash -s -- --uninstall --keep-config --yes
```

If an older CWD-based installation is detected, the script prints the old
binary and working directory instead of deleting files it does not own.

---

### Manual Setup

<details>
<summary>Click to expand manual setup instructions</summary>

#### 1. Download NyxProxy

```bash
wget https://github.com/ikenxuan/nyxproxy-oss/releases/latest/download/nyxproxy-linux-amd64 -O nyxproxy
chmod +x nyxproxy
```

#### 2. Configure IPv6 Rotation

```bash
# Install ndppd
sudo apt update && sudo apt install -y ndppd

# Create ndppd config
sudo tee /etc/ndppd.conf <<EOF
route-ttl 30000

proxy enp1s0 {
    router no
    timeout 500
    ttl 30000

    rule 2a05:f480:1800:25db::/64 {
        auto
    }
}
EOF

# Set kernel parameters
sudo sysctl -w net.ipv6.conf.all.proxy_ndp=1
sudo sysctl -w net.ipv6.conf.enp1s0.proxy_ndp=1
sudo sysctl -w net.ipv6.ip_nonlocal_bind=1

# Start ndppd
sudo systemctl enable ndppd
sudo systemctl start ndppd
```

#### 3. Create config.yaml

```yaml
proxy:
  type: https
  listen_address: "0.0.0.0"
  listen_port: 8080
  username: "admin"
  password: "your-secure-password"

network:
  interface_name: "enp1s0"                        # Your interface
  ipv4_enabled: false
  ipv6_enabled: true
  rotate_ipv6: true
  ipv6_subnet: "2a05:f480:1800:25db::/64"        # Your /64 subnet

  # Rotation settings
  ipv6_pool_size: 200     # Number of IPs in pool
  ipv6_max_usage: 100     # Replace after 100 uses
  ipv6_max_age: 30        # Replace after 30 minutes

monitoring:
  enabled: true
  port: 9090
  allow_remote: false

logging:
  debug_level: 0
```

#### 4. Start NyxProxy

```bash
./nyxproxy
```

</details>

---

## ⚙️ Configuration

### Basic Configuration

```yaml
proxy:
  type: https              # or "socks5"
  listen_address: "0.0.0.0"
  listen_port: 8080
  username: "admin"
  password: "secure-password"
```

### Web Console

```yaml
web:
  enabled: true
  listen_address: "127.0.0.1"   # use 0.0.0.0 only behind a firewall/proxy
  port: 18081
  username: "admin"
  password: "change_me_too"
  tls_cert_file: ""
  tls_key_file: ""
  trust_proxy_headers: false
  cookie_secure: false
```

### IPv6 Rotation Settings

```yaml
network:
  rotate_ipv6: true
  ipv6_subnet: "2a05:f480:1800:25db::/64"

  # Pool configuration
  ipv6_pool_size: 200     # Number of IPs (default: 200)
  ipv6_max_usage: 100     # Max uses per IP (default: 100)
  ipv6_max_age: 30        # Max age in minutes (default: 30)
```

### Rotation Strategies

#### Aggressive (Anti-Fingerprinting)
```yaml
ipv6_pool_size: 500      # Lots of IPs
ipv6_max_usage: 25       # Replace quickly
ipv6_max_age: 10         # Short lifetime
```
→ Maximum privacy, IPs change frequently

#### Moderate (Recommended)
```yaml
ipv6_pool_size: 200      # Good balance
ipv6_max_usage: 100      # Standard
ipv6_max_age: 30         # 30 minutes
```
→ Best balance between performance and privacy

#### Conservative (Performance)
```yaml
ipv6_pool_size: 100      # Fewer IPs, faster startup
ipv6_max_usage: 500      # Use IPs longer
ipv6_max_age: 120        # 2 hours
```
→ Less overhead, better performance

---

## 🔧 Running as Daemon (Systemd)

### Create Systemd Service

```bash
sudo mkdir -p /etc/nyxproxy
sudo chmod 700 /etc/nyxproxy
sudo cp config.yaml /etc/nyxproxy/config.yaml
sudo chmod 600 /etc/nyxproxy/config.yaml
```

```bash
sudo tee /etc/systemd/system/nyxproxy.service <<EOF
[Unit]
Description=NyxProxy IPv6 Rotating Proxy Server
After=network.target ndppd.service
Requires=ndppd.service

[Service]
Type=simple
User=root
WorkingDirectory=/etc/nyxproxy
ExecStart=/root/nyxproxy -config /etc/nyxproxy/config.yaml
Restart=on-failure
RestartSec=10
StandardOutput=journal
StandardError=journal

# Security hardening (optional)
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF
```

### Manage Service

```bash
# Enable auto-start on boot
sudo systemctl enable nyxproxy

# Start service
sudo systemctl start nyxproxy

# Check status
sudo systemctl status nyxproxy

# View logs
sudo journalctl -u nyxproxy -f

# Restart service
sudo systemctl restart nyxproxy

# Stop service
sudo systemctl stop nyxproxy
```

---

## 🐳 Docker Deployment

The repository ships a Docker image that waits for the IPv6 prefix at startup,
follows prefix changes automatically, and cleans up its pool addresses on
`SIGTERM`.

### Requirements

- A routed IPv6 `/64` with working NDP (see [Requirements](#-requirements)).
- **Host networking** (`network_mode: host`) - bridge networking cannot add
  addresses to the host interface.
- `NET_ADMIN` capability.
- The image already includes `iproute2` (the `ip` command NyxProxy uses).

### Use the published image (recommended)

The repository publishes multi-arch images to GitHub Container Registry:

```text
ghcr.io/ikenxuan/nyxproxy-oss:latest
```

```bash
cp .env.docker.example .env.docker
# edit .env.docker: set IFACE, PROXY_PASS, and WEB_PASS

docker compose pull
docker compose up -d
docker compose logs -f nyxproxy
```

`docker-compose.yml` uses host networking, `NET_ADMIN`, a read-only root
filesystem, and the published GHCR image by default.

### Build from source

```bash
cp .env.docker.example .env.docker
# edit .env.docker: set IFACE, PROXY_PASS, and WEB_PASS

docker compose -f docker/docker-compose.example.yml up -d --build
```

Or build and run manually:

```bash
docker build -t nyxproxy-oss .
docker run -d \
  --name nyxproxy \
  --network host \
  --cap-drop ALL --cap-add NET_ADMIN \
  --security-opt no-new-privileges:true \
  --read-only --tmpfs /run \
  -v nyxproxy-data:/data \
  -e IFACE=eth0 \
  -e LISTEN_PORT=18080 \
  -e POOL_SIZE=64 \
  -e PROXY_USER=admin \
  -e PROXY_PASS=change_me \
  -e WEB_ENABLED=true \
  -e WEB_PASS=change_me_too \
  nyxproxy-oss
```

### Environment variables

| Variable | Default | Description |
| --- | --- | --- |
| `IFACE` | default-route interface | Interface that owns the IPv6 `/64` |
| `PREFIX` | auto-detected | Pin the prefix, e.g. `2001:db8:1:2::/64` (disables automatic following) |
| `LISTEN_PORT` | `1080` | SOCKS5 listen port |
| `POOL_SIZE` | `64` | Number of rotating addresses |
| `MAX_USAGE` | `100` | Rotate an address after this many uses |
| `MAX_AGE` | `30` | Rotate an address after this many minutes |
| `PROXY_USER` | `admin` | SOCKS5 username |
| `PROXY_PASS` | *(required)* | SOCKS5 password |
| `DEBUG_LEVEL` | `1` | NyxProxy debug level |
| `DNS_SOURCE_ADDRESS` | auto | Pin the DNS source address (auto-detected from the current prefix when empty) |
| `WEB_ENABLED` | `true` | Enable the web console |
| `WEB_LISTEN` | `0.0.0.0` | Console listen address (host networking) |
| `WEB_PORT` | `18081` | Console port |
| `WEB_USER` | `admin` | Console username |
| `WEB_PASS` | *(required when enabled)* | Console password; only used when seeding a new volume |
| `WEB_TLS_CERT_FILE` | empty | Container path to a TLS certificate (read-only bind mount) |
| `WEB_TLS_KEY_FILE` | empty | Container path to the TLS private key |

The first start seeds `/data/config.yaml` from these environment variables. After
that, the file is authoritative: changing `.env.docker` no longer changes the
running configuration. Use the web console to edit settings, or delete the
`nyxproxy-data` volume to seed again. The entrypoint keeps `ipv6_subnet` and
`dns_source_address` in sync with the detected prefix; use `PREFIX` and
`DNS_SOURCE_ADDRESS` to pin either value.

The entrypoint reads `ip -6 addr show` and picks the most preferred
non-deprecated global `/64` on `IFACE` (ignoring deprecated, temporary, and
`/128` pool addresses), so a stale route left behind by a PPPoE re-dial cannot
pin the pool to a dead prefix. When the prefix changes it rewrites those two
config fields and restarts NyxProxy.

### Publishing

`.github/workflows/docker.yml` builds `linux/amd64` and `linux/arm64` images and
pushes them to GHCR on pushes to `main`, on `v*` tags, and on manual
`workflow_dispatch`. `latest` is published for both `main` and tag builds;
version tags such as `v1.2.3` are published as well.

```text
ghcr.io/ikenxuan/nyxproxy-oss:latest
```

For a Chinese walkthrough, see [README-ZH.md](README-ZH.md).

---

## 🖥️ Web Console

The console is served by the same binary and container as the proxy. It provides:

- a live dashboard (proxy state, address pool, traffic counters, last apply result)
- a full configuration editor with server-side validation
- automatic apply with rollback when a component cannot start
- a live log stream over Server-Sent Events

### Docker

Set `WEB_PASS` in `.env.docker`, start the stack, and open
`http://YOUR_SERVER:18081`. With host networking the port is exposed directly on
the host; restrict it with a firewall or a reverse proxy.

For built-in TLS, mount certificates read-only and set:

```yaml
web:
  tls_cert_file: /certs/tls.crt
  tls_key_file: /certs/tls.key
```

```bash
docker run ... -v "$PWD/certs:/certs:ro" ...
```

### Bare metal

Enable the console in `config.yaml`:

```yaml
web:
  enabled: true
  listen_address: "127.0.0.1"
  port: 18081
  username: "admin"
  password: "change_me_too"
  tls_cert_file: ""
  tls_key_file: ""
  trust_proxy_headers: false
  cookie_secure: false
```

The bare-metal default binds to loopback. Use an SSH tunnel
(`ssh -L 18081:127.0.0.1:18081 user@server`) or a reverse proxy. If you put the
console behind nginx or Caddy, preserve the `Host` header, disable proxy
buffering for `/api/logs/stream`, and only set `trust_proxy_headers: true` when
the console is not directly exposed.

Saving a configuration restarts the proxy (and monitoring, if its settings
changed). Existing proxy connections are drained for up to 5 seconds. Invalid
configurations are rejected before anything is stopped. If the new configuration
cannot start, the old one is restored and the file on disk is left unchanged.

---

## 💻 Usage Examples

### cURL

```bash
# Single request
curl --proxy http://admin:password@your-server:8080 https://api6.ipify.org

# 10 requests - each with different IPv6
for i in {1..10}; do
  echo "Request $i:"
  curl --proxy http://admin:password@your-server:8080 https://api6.ipify.org
done
```

### Python

```python
import requests

proxies = {
    'http': 'http://admin:password@your-server:8080',
    'https': 'http://admin:password@your-server:8080'
}

# Each request uses a different IPv6
for i in range(10):
    response = requests.get('https://api6.ipify.org', proxies=proxies)
    print(f"Request {i+1}: {response.text}")
```

### Node.js

```javascript
const axios = require('axios');

const proxy = {
  host: 'your-server',
  port: 8080,
  auth: {
    username: 'admin',
    password: 'password'
  }
};

// Each request uses a different IPv6
for (let i = 0; i < 10; i++) {
  axios.get('https://api6.ipify.org', { proxy })
    .then(res => console.log(`Request ${i+1}: ${res.data}`));
}
```

---

## 📚 IPv6 Rotation Explained

### Why 200 IPs in the Pool?

**Startup Time vs. Diversity Trade-off:**
- More IPs = Longer startup (but only once!)
- More IPs = More diversity, less repetition
- 200 IPs is the sweet spot for most use cases

**Calculation:**
```
200 IPs × 100 uses = 20,000 requests before first rotation
20,000 requests ÷ 100 req/min = 200 minutes of unique IPs
```

### Why Replace After 100 Uses?

**Rate Limiting Protection:**
- Most APIs rate-limit per IP
- 100 requests is typically below most thresholds
- After 100 uses, IP is "burned" → replace it

### Why Replace After 30 Minutes?

**Time-Based Tracking:**
- Some services track IPs over time
- Fresh IPs are less likely to be flagged
- 30 minutes is a good balance

### Background Rotation

Every 5 minutes, NyxProxy checks all IPs:
```
For each IP in pool:
  If (usageCount >= 100) OR (age > 30 minutes):
    1. Remove old IP from interface
    2. Generate new random IP
    3. Add new IP to interface
    4. Update pool
```

**Example log:**
```
14:35:00 - ✓ Rotated 12 IPs (used 100+ times)
15:05:00 - ✓ Rotated 8 IPs (age 30m+)
15:35:00 - ✓ Rotated 5 IPs (used 100+ times)
```

---

## 📋 Requirements

### Server Requirements
- **OS**: Linux (Debian, Ubuntu, CentOS, etc.)
- **Architecture**: amd64 or arm64
- **RAM**: >= 512 MB
- **Disk**: >= 100 MB
- **Docker**: Docker Engine 20.10+ with Compose v2 (for the container deployment)
- **Go**: 1.27+ when building from source
- **Node**: 24 LTS and pnpm 12.14.0 when building the frontend

### Network Requirements
- **IPv6 /64 subnet** actually routed to your server
- A provider that routes the whole `/64` to your server and lets you add addresses on the interface
- A `/124` or smaller delegation is too small for a useful rotation pool
- For Docker: host networking (`network_mode: host`) and `NET_ADMIN`

### Check Your Setup

```bash
# Check if you have a /64 subnet
ip -6 addr show | grep "scope global"

# Should show something like:
# inet6 2a05:f480:1800:25db::1/64 scope global

# Test IPv6 connectivity
ping6 google.com

# Check if ndppd is installed
which ndppd
```

---

## 🔧 Troubleshooting

### Problem: "Cannot find device ''"

**Cause:** Network interface not configured

**Solution:**
```bash
# Find your interface
ip link show

# Set in config.yaml
network:
  interface_name: "enp1s0"  # Your interface name
```

### Problem: "ndppd service is not running"

**Cause:** ndppd not installed or configured

**Solution:**
```bash
# Install ndppd
sudo apt install -y ndppd

# Or re-run the installer
curl -fsSL https://raw.githubusercontent.com/ikenxuan/nyxproxy-oss/main/scripts/quick-setup.sh | sudo bash
```

### Problem: Requests timeout

**Cause:** IPs not properly routed

**Solution:**
```bash
# Check if subnet is routed to you
curl --interface 2a05:f480:1800:25db::9999 -6 https://api6.ipify.org

# If it works, the subnet is routed correctly
# If not, contact your provider
```

### Problem: Slow startup

**Cause:** Large IP pool

**Solution:**
```yaml
# Reduce pool size for faster startup
network:
  ipv6_pool_size: 100  # Instead of 200
```

### View Logs

```bash
# NyxProxy logs (if running as service)
sudo journalctl -u nyxproxy -f

# ndppd logs
sudo journalctl -u ndppd -f

# Check system logs
dmesg | grep -i ipv6
```

---

## 🌟 Advanced Usage

### Multiple Proxy Instances

By default NyxProxy reads `./config.yaml`. Use `-config` or `NYXPROXY_CONFIG`
to point at a different file.

Run multiple proxies on different ports by using separate directories (each with its own `config.yaml`):

```bash
mkdir -p proxy-8080 proxy-8081
cp nyxproxy proxy-8080/nyxproxy
cp nyxproxy proxy-8081/nyxproxy

# Create and edit separate configs
cp config.example.yaml proxy-8080/config.yaml
cp config.example.yaml proxy-8081/config.yaml

(cd proxy-8080 && sudo ./nyxproxy -config /root/proxy-8080/config.yaml) &
(cd proxy-8081 && sudo ./nyxproxy -config /root/proxy-8081/config.yaml) &
```

An explicit `-config` path is independent of the working directory;
`NYXPROXY_CONFIG` provides the same override through the environment.

### Monitoring

```bash
# Check proxy stats
curl http://localhost:9090/stats

# Output:
# {
#   "uptime": "5h23m15s",
#   "active_connections": 0,
#   "total_connections": 0,
#   "total_requests": 0,
#   "bytes_sent": 0,
#   "bytes_received": 0,
#   "interface": "eth0",
#   "ip_pool_size": 200,
#   "ips_rotated": 38
# }
```

---

## 📝 License

MIT License - see [LICENSE](LICENSE) file for details.

---

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

---

## 🙏 Acknowledgments

- Built with Go
- Uses ndppd for NDP proxy functionality
- Inspired by the need for better proxy rotation

---

## 📞 Support

- **Issues**: [GitHub Issues](https://github.com/ikenxuan/nyxproxy-oss/issues)
- **Documentation**: See `docs/` folder
- **German Documentation**: See `SETUP_DE.md`

---

**Made with ❤️ for the privacy-conscious and web scraping community**
