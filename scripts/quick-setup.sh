#!/usr/bin/env bash
#
# NyxProxy installer / uninstaller for Debian and Ubuntu.
#
# Install:
#   sudo bash quick-setup.sh
#
# Uninstall:
#   sudo bash quick-setup.sh --uninstall [--keep-config] [--yes]
#
# Installed paths:
#   /usr/local/bin/nyxproxy
#   /etc/nyxproxy/config.yaml
#   /etc/systemd/system/nyxproxy.service        (optional)
#   /etc/sysctl.d/99-nyxproxy-ipv6.conf
#   /etc/ndppd.conf                             (previous file backed up)
#   /var/lib/nyxproxy/install-state             (used by --uninstall)

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

REPO="ikenxuan/nyxproxy-oss"
BIN_PATH="/usr/local/bin/nyxproxy"
CONFIG_DIR="/etc/nyxproxy"
CONFIG_PATH="$CONFIG_DIR/config.yaml"
STATE_DIR="/var/lib/nyxproxy"
STATE_FILE="$STATE_DIR/install-state"
SERVICE_PATH="/etc/systemd/system/nyxproxy.service"
SYSCTL_FILE="/etc/sysctl.d/99-nyxproxy-ipv6.conf"
NDPPD_CONF="/etc/ndppd.conf"
NDPPD_BACKUP="/etc/ndppd.conf.nyxproxy.bak"
RELEASE_BASE="https://github.com/$REPO/releases/latest/download"

MODE="install"
KEEP_CONFIG=0
ASSUME_YES=0
PROG=$(basename "$0")
# When piped into bash, $0 is just "bash"; show a useful name instead.
case "$PROG" in bash|sh|-bash|-sh) PROG="quick-setup.sh" ;; esac

info() { echo -e "${BLUE}$*${NC}"; }
ok()   { echo -e "${GREEN}$*${NC}"; }
warn() { echo -e "${YELLOW}$*${NC}"; }
fail() { echo -e "${RED}$*${NC}" >&2; }

usage() {
    cat <<EOF
Usage:
  sudo bash $PROG
      Install or reconfigure NyxProxy.
  sudo bash $PROG --uninstall [--keep-config] [--yes]
      Remove NyxProxy and the system changes it made.

Options:
  --keep-config   Keep $CONFIG_DIR during uninstall
  --yes, -y       Skip the uninstall confirmation prompt
  --help, -h      Show this help
EOF
}

while [ $# -gt 0 ]; do
    case "$1" in
        --uninstall|uninstall) MODE="uninstall"; shift ;;
        --keep-config) KEEP_CONFIG=1; shift ;;
        --yes|-y) ASSUME_YES=1; shift ;;
        --help|-h) usage; exit 0 ;;
        *) fail "Unknown option: $1"; usage; exit 1 ;;
    esac
done

if [ "${EUID:-$(id -u)}" -ne 0 ]; then
    fail "This script must be run as root."
    echo "  Run: sudo bash $PROG"
    exit 1
fi

if [ "$(uname -s)" != "Linux" ]; then
    fail "This script only works on Linux."
    exit 1
fi

# Keep prompts working even if the script is piped to bash. Do not replace
# fd 0: when the script itself arrives on stdin, bash would then read the
# remaining commands from the terminal instead of the pipe.
PROMPT_FD=0
if [ ! -t 0 ]; then
    if [ -r /dev/tty ]; then
        exec 3</dev/tty
        PROMPT_FD=3
    elif [ "$MODE" = "install" ] || [ "$ASSUME_YES" -ne 1 ]; then
        fail "This script is interactive and needs a terminal."
        echo "  Use --uninstall --yes for a non-interactive uninstall."
        exit 1
    fi
fi

TMP_DIR=$(mktemp -d)
cleanup() { rm -rf "$TMP_DIR"; }
trap cleanup EXIT

ask_yes_no() {
    local prompt=$1 default=${2:-n} reply
    read -r -p "$prompt" reply <&"$PROMPT_FD" || true
    reply=${reply:-$default}
    [[ $reply =~ ^[Yy]$ ]]
}

# Stores the validated value in PROMPT_RESULT (avoids command-substitution
# issues with prompts and warnings).
prompt_number() {
    local label=$1 default=$2 value
    while true; do
        read -r -p "$label" value <&"$PROMPT_FD" || true
        value=${value:-$default}
        if [[ $value =~ ^[0-9]+$ ]]; then
            PROMPT_RESULT=$value
            return 0
        fi
        echo -e "${YELLOW}  Please enter a non-negative integer.${NC}" >&2
    done
}

yaml_escape() {
    local value=$1
    value=${value//\\/\\\\}
    value=${value//\"/\\\"}
    printf '%s' "$value"
}

restore_sysctl() {
    local key=$1 value=${2:-}
    if [ -n "$value" ]; then
        sysctl -w "$key=$value" >/dev/null 2>&1 || true
    fi
}

require_apt() {
    if ! command -v apt-get >/dev/null 2>&1; then
        fail "This installer supports Debian/Ubuntu (apt-get not found)."
        exit 1
    fi
}

require_systemd() {
    if ! command -v systemctl >/dev/null 2>&1; then
        fail "systemd is required (systemctl not found)."
        exit 1
    fi
}

install_nyxproxy() {
    echo -e "${GREEN}╔═══════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║   NyxProxy Quick Setup (Debian)       ║${NC}"
    echo -e "${GREEN}╚═══════════════════════════════════════╝${NC}"
    echo

    require_apt
    require_systemd

    # Ask before touching the system so cancelling is safe.
    if [ -f "$CONFIG_PATH" ]; then
        warn "An existing configuration was found at $CONFIG_PATH"
        if ! ask_yes_no "Replace it? A timestamped backup will be kept. (y/N) " n; then
            echo "Setup cancelled. Existing configuration left untouched."
            exit 0
        fi
        CONFIG_BACKUP="$CONFIG_PATH.bak.$(date +%Y%m%d%H%M%S)"
        cp -a "$CONFIG_PATH" "$CONFIG_BACKUP"
        ok "  ✓ Existing configuration backed up to $CONFIG_BACKUP"
        echo
    fi

    # Reuse the original system values recorded by a previous install.
    HAD_STATE=0
    if [ -f "$STATE_FILE" ]; then
        # shellcheck disable=SC1090
        . "$STATE_FILE"
        HAD_STATE=1
    fi
    NDPPD_INSTALLED_BY_US=${NDPPD_INSTALLED_BY_US:-0}
    NDPPD_CONF_EXISTED=${NDPPD_CONF_EXISTED:-0}

    info "[1/7] Installing dependencies..."
    apt-get update -qq
    apt-get install -y -qq curl ca-certificates iproute2 >/dev/null

    if ! command -v ndppd >/dev/null 2>&1; then
        echo "  Installing ndppd..."
        apt-get install -y -qq ndppd >/dev/null
        NDPPD_INSTALLED_BY_US=1
        ok "  ✓ ndppd installed"
    else
        ok "  ✓ ndppd already installed"
    fi
    echo

    info "[2/7] Detecting network configuration..."
    INTERFACE=$(ip -6 route 2>/dev/null | awk '$1 == "default" {print $5; exit}')
    if [ -z "$INTERFACE" ]; then
        INTERFACE=$(ip -6 addr show scope global 2>/dev/null | awk '
            /^[0-9]+:/ { name=$2; sub(/:$/, "", name) }
            /inet6/ && !/temporary|deprecated/ { print name; exit }
        ')
    fi
    if [ -z "$INTERFACE" ]; then
        fail "✗ Could not detect a network interface with IPv6."
        echo "  Available interfaces:"
        ip link show | grep -E "^[0-9]+:" | awk '{print "    - " $2}' | sed 's/:$//'
        exit 1
    fi
    ok "  ✓ Interface: $INTERFACE"

    IPV6_SUBNET=$(ip -6 route show dev "$INTERFACE" 2>/dev/null | awk '$1 ~ /\/64$/ {print $1; exit}')
    IPV6_ADDR=$(ip -6 addr show dev "$INTERFACE" scope global 2>/dev/null | awk '
        /inet6/ && !/temporary|deprecated/ { split($2, parts, "/"); print parts[1]; exit }
    ')
    if [ -z "$IPV6_SUBNET" ] && [ -n "$IPV6_ADDR" ]; then
        # Only derive the prefix when the address has four explicit hextets;
        # a compressed prefix such as 2001:db8::1 cannot be cut safely.
        if [[ $IPV6_ADDR =~ ^([0-9a-fA-F]{1,4}:){3}[0-9a-fA-F]{1,4}: ]]; then
            IPV6_BASE=$(echo "$IPV6_ADDR" | cut -d':' -f1-4)
            IPV6_SUBNET="${IPV6_BASE}::/64"
        fi
    fi
    if [ -z "$IPV6_SUBNET" ]; then
        fail "✗ No IPv6 /64 subnet found on $INTERFACE."
        exit 1
    fi
    ok "  ✓ IPv6 Subnet: $IPV6_SUBNET"
    echo

    # Record the values we are about to change. A re-install keeps the values
    # captured by the first installation so --uninstall restores the originals.
    if [ "$HAD_STATE" -eq 0 ]; then
        SYSCTL_ALL_FORWARDING=$(sysctl -n net.ipv6.conf.all.forwarding 2>/dev/null || true)
        SYSCTL_ALL_PROXY_NDP=$(sysctl -n net.ipv6.conf.all.proxy_ndp 2>/dev/null || true)
        SYSCTL_IP_NONLOCAL_BIND=$(sysctl -n net.ipv6.ip_nonlocal_bind 2>/dev/null || true)
        SYSCTL_IFACE_KEY="net.ipv6.conf.$INTERFACE.proxy_ndp"
        SYSCTL_IFACE_PROXY_NDP=$(sysctl -n "$SYSCTL_IFACE_KEY" 2>/dev/null || true)
    else
        SYSCTL_ALL_FORWARDING=${SYSCTL_ALL_FORWARDING:-}
        SYSCTL_ALL_PROXY_NDP=${SYSCTL_ALL_PROXY_NDP:-}
        SYSCTL_IP_NONLOCAL_BIND=${SYSCTL_IP_NONLOCAL_BIND:-}
        if [ -z "${SYSCTL_IFACE_KEY:-}" ]; then
            SYSCTL_IFACE_KEY="net.ipv6.conf.$INTERFACE.proxy_ndp"
            SYSCTL_IFACE_PROXY_NDP=$(sysctl -n "$SYSCTL_IFACE_KEY" 2>/dev/null || true)
        fi
        SYSCTL_IFACE_PROXY_NDP=${SYSCTL_IFACE_PROXY_NDP:-}
    fi

    # Back up the previous ndppd configuration before overwriting it.
    if [ -f "$NDPPD_CONF" ] && grep -q "NyxProxy NDP Proxy Configuration" "$NDPPD_CONF"; then
        # Our own generated file from an earlier run: keep the recorded state.
        :
    elif [ -f "$NDPPD_CONF" ] && [ ! -f "$NDPPD_BACKUP" ]; then
        cp -a "$NDPPD_CONF" "$NDPPD_BACKUP"
        NDPPD_CONF_EXISTED=1
        ok "  ✓ Existing ndppd configuration backed up"
    elif [ -f "$NDPPD_BACKUP" ]; then
        NDPPD_CONF_EXISTED=1
    fi

    # Capture ndppd's current state before enabling and starting it.
    if [ "$HAD_STATE" -eq 0 ]; then
        if systemctl is-enabled --quiet ndppd 2>/dev/null; then
            NDPPD_WAS_ENABLED=1
        else
            NDPPD_WAS_ENABLED=0
        fi
        if systemctl is-active --quiet ndppd 2>/dev/null; then
            NDPPD_WAS_ACTIVE=1
        else
            NDPPD_WAS_ACTIVE=0
        fi
    else
        NDPPD_WAS_ENABLED=${NDPPD_WAS_ENABLED:-1}
        NDPPD_WAS_ACTIVE=${NDPPD_WAS_ACTIVE:-1}
    fi

    # Persist the rollback information before changing the system, so a
    # failure or Ctrl+C can still be cleaned up with --uninstall.
    install -d -m 0755 "$STATE_DIR"
    {
        echo "# NyxProxy install state - used by --uninstall"
        echo "# Generated on $(date -u +%Y-%m-%dT%H:%M:%SZ)"
        printf 'INTERFACE=%q\n' "$INTERFACE"
        printf 'IPV6_SUBNET=%q\n' "$IPV6_SUBNET"
        printf 'NDPPD_INSTALLED_BY_US=%q\n' "$NDPPD_INSTALLED_BY_US"
        printf 'NDPPD_CONF_EXISTED=%q\n' "$NDPPD_CONF_EXISTED"
        printf 'NDPPD_WAS_ENABLED=%q\n' "$NDPPD_WAS_ENABLED"
        printf 'NDPPD_WAS_ACTIVE=%q\n' "$NDPPD_WAS_ACTIVE"
        printf 'SYSCTL_ALL_FORWARDING=%q\n' "$SYSCTL_ALL_FORWARDING"
        printf 'SYSCTL_ALL_PROXY_NDP=%q\n' "$SYSCTL_ALL_PROXY_NDP"
        printf 'SYSCTL_IP_NONLOCAL_BIND=%q\n' "$SYSCTL_IP_NONLOCAL_BIND"
        printf 'SYSCTL_IFACE_KEY=%q\n' "${SYSCTL_IFACE_KEY:-}"
        printf 'SYSCTL_IFACE_PROXY_NDP=%q\n' "${SYSCTL_IFACE_PROXY_NDP:-}"
    } > "$STATE_FILE"
    chmod 600 "$STATE_FILE"

    info "[3/7] Configuring kernel parameters..."
    sysctl -w net.ipv6.conf.all.forwarding=1 >/dev/null
    sysctl -w net.ipv6.conf.all.proxy_ndp=1 >/dev/null
    sysctl -w "net.ipv6.conf.$INTERFACE.proxy_ndp=1" >/dev/null
    sysctl -w net.ipv6.ip_nonlocal_bind=1 >/dev/null

    cat > "$SYSCTL_FILE" <<EOF
# NyxProxy IPv6 Rotation
net.ipv6.conf.all.forwarding=1
net.ipv6.conf.all.proxy_ndp=1
net.ipv6.conf.$INTERFACE.proxy_ndp=1
net.ipv6.ip_nonlocal_bind=1
EOF
    chmod 644 "$SYSCTL_FILE"
    ok "  ✓ Kernel parameters configured"
    echo

    info "[4/7] Configuring ndppd..."
    cat > "$NDPPD_CONF" <<EOF
# NyxProxy NDP Proxy Configuration
route-ttl 30000

proxy $INTERFACE {
    router no
    timeout 500
    ttl 30000

    rule $IPV6_SUBNET {
        auto
    }
}
EOF

    systemctl enable ndppd >/dev/null 2>&1 || true
    if ! systemctl restart ndppd; then
        fail "✗ ndppd failed to start."
        echo "  Check: journalctl -u ndppd -n 20"
        exit 1
    fi
    if systemctl is-active --quiet ndppd; then
        ok "  ✓ ndppd service running"
    else
        fail "✗ ndppd is not running."
        echo "  Check: journalctl -u ndppd -n 20"
        exit 1
    fi
    echo

    info "[5/7] Creating NyxProxy configuration..."
    install -d -m 0755 "$CONFIG_DIR"

    read -r -p "  Proxy username [admin]: " PROXY_USER <&"$PROMPT_FD" || true
    PROXY_USER=${PROXY_USER:-admin}

    while true; do
        if ! read -r -s -p "  Proxy password: " PROXY_PASS <&"$PROMPT_FD"; then
            echo
            fail "✗ Input closed; aborting."
            exit 1
        fi
        echo
        [ -n "$PROXY_PASS" ] && break
        warn "  Password cannot be empty."
    done

    echo
    echo "  IPv6 Rotation Settings:"
    prompt_number "  IP Pool size [200]: " 200
    POOL_SIZE=$PROMPT_RESULT
    prompt_number "  Max uses per IP before rotation [100]: " 100
    MAX_USAGE=$PROMPT_RESULT
    prompt_number "  Max age in minutes before rotation [30]: " 30
    MAX_AGE=$PROMPT_RESULT

    echo
    WEB_ENABLED=false
    WEB_USER=admin
    WEB_PASS=""
    WEB_PORT=18081
    if ask_yes_no "  Enable web console? (y/N) " n; then
        WEB_ENABLED=true
        read -r -p "  Web username [admin]: " WEB_USER <&"$PROMPT_FD" || true
        WEB_USER=${WEB_USER:-admin}
        while true; do
            if ! read -r -s -p "  Web password: " WEB_PASS <&"$PROMPT_FD"; then
                echo
                fail "✗ Input closed; aborting."
                exit 1
            fi
            echo
            [ -n "$WEB_PASS" ] && break
            warn "  Password cannot be empty."
        done
        prompt_number "  Web port [18081]: " 18081
        WEB_PORT=$PROMPT_RESULT
    fi

    PROXY_USER_YAML=$(yaml_escape "$PROXY_USER")
    PROXY_PASS_YAML=$(yaml_escape "$PROXY_PASS")
    WEB_USER_YAML=$(yaml_escape "$WEB_USER")
    WEB_PASS_YAML=$(yaml_escape "$WEB_PASS")

    OLD_UMASK=$(umask)
    umask 077
    {
        printf '# NyxProxy Configuration\n'
        printf '# Auto-generated on %s\n\n' "$(date)"
        printf 'proxy:\n'
        printf '  type: https\n'
        printf '  listen_address: "0.0.0.0"\n'
        printf '  listen_port: 8080\n'
        printf '  username: "%s"\n' "$PROXY_USER_YAML"
        printf '  password: "%s"\n\n' "$PROXY_PASS_YAML"
        printf 'network:\n'
        printf '  interface_name: "%s"\n' "$INTERFACE"
        printf '  ipv4_enabled: false\n'
        printf '  ipv6_enabled: true\n'
        printf '  rotate_ipv6: true\n'
        printf '  ipv6_subnet: "%s"\n' "$IPV6_SUBNET"
        printf '  ipv6_pool_size: %s\n' "$POOL_SIZE"
        printf '  ipv6_max_usage: %s\n' "$MAX_USAGE"
        printf '  ipv6_max_age: %s\n\n' "$MAX_AGE"
        printf 'monitoring:\n'
        printf '  enabled: true\n'
        printf '  port: 9090\n'
        printf '  allow_remote: false\n\n'
        printf 'logging:\n'
        printf '  debug_level: 0\n\n'
        printf 'web:\n'
        printf '  enabled: %s\n' "$WEB_ENABLED"
        printf '  listen_address: "127.0.0.1"\n'
        printf '  port: %s\n' "$WEB_PORT"
        printf '  username: "%s"\n' "$WEB_USER_YAML"
        printf '  password: "%s"\n' "$WEB_PASS_YAML"
        printf '  tls_cert_file: ""\n'
        printf '  tls_key_file: ""\n'
        printf '  trust_proxy_headers: false\n'
        printf '  cookie_secure: false\n'
    } > "$CONFIG_PATH"
    umask "$OLD_UMASK"
    chmod 600 "$CONFIG_PATH"
    ok "  ✓ Configuration written to $CONFIG_PATH (mode 0600)"
    echo

    info "[6/7] Downloading NyxProxy..."
    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64) ARCH_NAME="amd64" ;;
        aarch64|arm64) ARCH_NAME="arm64" ;;
        *)
            fail "✗ Unsupported architecture: $ARCH"
            exit 1
            ;;
    esac
    ok "  ✓ Architecture: $ARCH_NAME"

    ASSET="nyxproxy-linux-$ARCH_NAME"
    if ! curl -fL --retry 3 --retry-delay 2 -o "$TMP_DIR/$ASSET" "$RELEASE_BASE/$ASSET"; then
        fail "✗ Failed to download $ASSET"
        echo "  URL: $RELEASE_BASE/$ASSET"
        exit 1
    fi

    if curl -fsSL --retry 3 --retry-delay 2 -o "$TMP_DIR/sha256sums.txt" "$RELEASE_BASE/sha256sums.txt"; then
        EXPECTED=$(awk -v name="$ASSET" '$2 == name {print $1; exit}' "$TMP_DIR/sha256sums.txt")
        ACTUAL=$(sha256sum "$TMP_DIR/$ASSET" | awk '{print $1}')
        if [ -z "$EXPECTED" ]; then
            fail "✗ Could not find $ASSET in sha256sums.txt"
            exit 1
        fi
        if [ "$EXPECTED" != "$ACTUAL" ]; then
            fail "✗ Checksum verification failed for $ASSET"
            exit 1
        fi
        ok "  ✓ SHA-256 verified"
    else
        warn "  ⚠ Could not download sha256sums.txt; skipping checksum verification."
    fi

    if systemctl is-active --quiet nyxproxy 2>/dev/null; then
        systemctl stop nyxproxy
        SERVICE_WAS_RUNNING=1
    else
        SERVICE_WAS_RUNNING=0
    fi
    install -m 0755 "$TMP_DIR/$ASSET" "$BIN_PATH"
    ok "  ✓ Installed $BIN_PATH"
    echo

    info "[7/7] Installing systemd service..."
    if ask_yes_no "  Install NyxProxy as a systemd service? (y/N) " n; then
        cat > "$SERVICE_PATH" <<EOF
[Unit]
Description=NyxProxy IPv6 Rotating Proxy
Documentation=https://github.com/$REPO
After=network-online.target ndppd.service
Wants=network-online.target
Requires=ndppd.service

[Service]
Type=simple
User=root
WorkingDirectory=$CONFIG_DIR
ExecStart=$BIN_PATH -config $CONFIG_PATH
Restart=on-failure
RestartSec=10
StandardOutput=journal
StandardError=journal
SyslogIdentifier=nyxproxy

[Install]
WantedBy=multi-user.target
EOF
        systemctl daemon-reload
        systemctl enable nyxproxy >/dev/null 2>&1 || true
        ok "  ✓ Service installed and enabled"

        if ask_yes_no "  Start NyxProxy now? (Y/n) " y; then
            if ! systemctl start nyxproxy; then
                fail "  ✗ Service failed to start."
                echo "    Check: journalctl -u nyxproxy -n 20"
            else
                sleep 2
                if systemctl is-active --quiet nyxproxy; then
                    ok "  ✓ NyxProxy is running"
                else
                    fail "  ✗ Service failed to start."
                    echo "    Check: journalctl -u nyxproxy -n 20"
                fi
            fi
        else
            echo "  Service installed but not started."
        fi
    else
        if [ -f "$SERVICE_PATH" ]; then
            warn "  An existing service unit remains at $SERVICE_PATH and may start on boot."
        fi
        if [ "$SERVICE_WAS_RUNNING" -eq 1 ]; then
            warn "  The previous service was stopped. Start it manually after reviewing the config."
        fi
        echo "  Manual start: sudo $BIN_PATH -config $CONFIG_PATH"
    fi
    echo

    ok "╔═══════════════════════════════════════╗"
    ok "║        Setup Complete! ✓              ║"
    ok "╚═══════════════════════════════════════╝"
    echo
    echo "  Interface:    $INTERFACE"
    echo "  IPv6 subnet:  $IPV6_SUBNET"
    echo "  Proxy:        HTTPS on 0.0.0.0:8080"
    echo "  Username:     $PROXY_USER"
    echo "  Binary:       $BIN_PATH"
    echo "  Config:       $CONFIG_PATH"
    echo
    if [ "$WEB_ENABLED" = "true" ]; then
        echo "  Web console:  http://127.0.0.1:$WEB_PORT"
        echo "  Remote:       ssh -L $WEB_PORT:127.0.0.1:$WEB_PORT root@YOUR_SERVER"
        echo "  Web user:     $WEB_USER"
        echo
    fi
    echo "  Uninstall:"
    echo "    Re-run the installation command with --uninstall"
    echo "    (add --keep-config to keep $CONFIG_PATH)"
    echo
    echo "  Test:"
    echo "    curl --proxy http://$PROXY_USER:***@YOUR_SERVER_IP:8080 https://api6.ipify.org"
    echo
}

uninstall_nyxproxy() {
    echo -e "${GREEN}╔═══════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║   NyxProxy Uninstaller                ║${NC}"
    echo -e "${GREEN}╚═══════════════════════════════════════╝${NC}"
    echo

    HAD_STATE=0
    if [ -f "$STATE_FILE" ]; then
        # shellcheck disable=SC1090
        . "$STATE_FILE"
        HAD_STATE=1
    fi

    NDPPD_MANAGED=0
    if [ -f "$NDPPD_CONF" ] && grep -q "NyxProxy NDP Proxy Configuration" "$NDPPD_CONF"; then
        NDPPD_MANAGED=1
    fi

    if [ ! -e "$BIN_PATH" ] && [ ! -e "$CONFIG_DIR" ] && [ ! -e "$SERVICE_PATH" ] \
        && [ ! -e "$SYSCTL_FILE" ] && [ ! -e "$STATE_FILE" ] \
        && [ ! -e "$NDPPD_BACKUP" ] && [ "$NDPPD_MANAGED" -eq 0 ]; then
        echo "No NyxProxy installation was found."
        exit 0
    fi

    # Remember paths from an older, CWD-based installation for a warning.
    # Unknown files are never deleted automatically.
    LEGACY_EXEC=""
    LEGACY_WORKDIR=""
    if [ -f "$SERVICE_PATH" ]; then
        LEGACY_EXEC=$(sed -n 's/^ExecStart=//p' "$SERVICE_PATH" | head -n 1)
        LEGACY_WORKDIR=$(sed -n 's/^WorkingDirectory=//p' "$SERVICE_PATH" | head -n 1)
    fi

    echo "This will remove:"
    echo "  - the nyxproxy systemd service"
    echo "  - $BIN_PATH"
    echo "  - $SYSCTL_FILE (and restore the previous kernel values)"
    echo "  - the NyxProxy ndppd configuration"
    echo "  - $STATE_DIR"
    if [ "$KEEP_CONFIG" -eq 1 ]; then
        echo "  - everything except $CONFIG_PATH"
    else
        echo "  - $CONFIG_DIR (including configuration backups)"
    fi
    if [ "${NDPPD_INSTALLED_BY_US:-0}" = "1" ]; then
        echo "  - the ndppd package (it was installed by NyxProxy)"
    fi
    echo

    if [ "$ASSUME_YES" -ne 1 ]; then
        if ! ask_yes_no "Continue? (y/N) " n; then
            echo "Cancelled."
            exit 0
        fi
    fi

    # 1. Stop and remove the service, then any manually started process.
    if [ -f "$SERVICE_PATH" ]; then
        systemctl stop nyxproxy 2>/dev/null || true
        systemctl disable nyxproxy 2>/dev/null || true
        rm -f "$SERVICE_PATH"
        rm -rf "$SERVICE_PATH.d"
        systemctl daemon-reload 2>/dev/null || true
        ok "  ✓ Service removed"
    fi

    if pgrep -x nyxproxy >/dev/null 2>&1; then
        pkill -TERM -x nyxproxy 2>/dev/null || true
        for _ in 1 2 3 4 5 6 7 8 9 10; do
            pgrep -x nyxproxy >/dev/null 2>&1 || break
            sleep 1
        done
        pkill -KILL -x nyxproxy 2>/dev/null || true
        ok "  ✓ Running nyxproxy process stopped"
    fi

    # 2. Remove the sysctl drop-in and restore the pre-install values.
    rm -f "$SYSCTL_FILE"
    restore_sysctl net.ipv6.conf.all.forwarding "${SYSCTL_ALL_FORWARDING:-}"
    restore_sysctl net.ipv6.conf.all.proxy_ndp "${SYSCTL_ALL_PROXY_NDP:-}"
    if [ -n "${SYSCTL_IFACE_KEY:-}" ]; then
        restore_sysctl "$SYSCTL_IFACE_KEY" "${SYSCTL_IFACE_PROXY_NDP:-}"
    fi
    restore_sysctl net.ipv6.ip_nonlocal_bind "${SYSCTL_IP_NONLOCAL_BIND:-}"
    if [ "$HAD_STATE" -eq 1 ]; then
        ok "  ✓ Kernel settings restored"
    else
        warn "  ⚠ No saved kernel values were found; current sysctl values were left unchanged."
    fi

    # 3. Remove ndppd only when this installer installed it. Do this before
    # restoring the configuration, because purging the package also removes
    # its conffiles.
    if [ "${NDPPD_INSTALLED_BY_US:-0}" = "1" ]; then
        systemctl stop ndppd 2>/dev/null || true
        systemctl disable ndppd 2>/dev/null || true
        if apt-get purge -y ndppd >/dev/null 2>&1; then
            ok "  ✓ ndppd removed"
        else
            warn "  ⚠ Could not remove ndppd; run: sudo apt-get purge ndppd"
        fi
    fi

    # 4. Restore the previous ndppd configuration, or remove ours.
    if [ -f "$NDPPD_BACKUP" ]; then
        if [ "$NDPPD_MANAGED" -eq 1 ] || [ ! -f "$NDPPD_CONF" ]; then
            mv -f "$NDPPD_BACKUP" "$NDPPD_CONF"
            ok "  ✓ Previous ndppd configuration restored"
        else
            warn "  ⚠ $NDPPD_CONF was changed after installation; keeping it."
            warn "    The previous file remains at $NDPPD_BACKUP"
        fi
    elif [ "$NDPPD_MANAGED" -eq 1 ]; then
        rm -f "$NDPPD_CONF"
        ok "  ✓ NyxProxy ndppd configuration removed"
    fi

    # 5. Restore ndppd's previous enabled/active state and reload the
    # restored configuration.
    if [ "${NDPPD_INSTALLED_BY_US:-0}" != "1" ]; then
        if [ "${NDPPD_WAS_ENABLED:-1}" != "1" ]; then
            systemctl disable ndppd 2>/dev/null || true
        fi
        if [ "${NDPPD_WAS_ACTIVE:-1}" != "1" ]; then
            systemctl stop ndppd 2>/dev/null || true
        elif command -v ndppd >/dev/null 2>&1; then
            systemctl restart ndppd 2>/dev/null || true
        fi
    fi

    # 6. Remove the binary and application data.
    rm -f "$BIN_PATH"
    rm -rf "$STATE_DIR"
    if [ "$KEEP_CONFIG" -eq 1 ]; then
        ok "  ✓ Configuration kept at $CONFIG_PATH"
    else
        rm -rf "$CONFIG_DIR"
        ok "  ✓ Configuration removed"
    fi

    echo
    ok "Uninstall complete."

    if [ -n "$LEGACY_EXEC" ] && [ "${LEGACY_EXEC%% *}" != "$BIN_PATH" ]; then
        warn "Note: the previous service used $LEGACY_EXEC."
        warn "      Remove that file manually if it still exists."
    fi
    if [ -n "$LEGACY_WORKDIR" ] && [ "$LEGACY_WORKDIR" != "$CONFIG_DIR" ]; then
        warn "Note: an older installation may have left files in $LEGACY_WORKDIR."
    fi
}

if [ "$MODE" = "uninstall" ]; then
    uninstall_nyxproxy
else
    install_nyxproxy
fi
