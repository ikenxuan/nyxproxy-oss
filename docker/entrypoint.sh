#!/bin/sh
set -eu

CONFIG_PATH="${CONFIG_PATH:-/data/config.yaml}"
TEMPLATE="${TEMPLATE:-/etc/nyxproxy/config.template.yaml}"

IFACE="${IFACE:-}"
LISTEN_PORT="${LISTEN_PORT:-1080}"
POOL_SIZE="${POOL_SIZE:-64}"
MAX_USAGE="${MAX_USAGE:-100}"
MAX_AGE="${MAX_AGE:-30}"
PROXY_USER="${PROXY_USER:-admin}"
PROXY_PASS="${PROXY_PASS:-}"
DEBUG_LEVEL="${DEBUG_LEVEL:-1}"
DNS_SOURCE_ADDRESS="${DNS_SOURCE_ADDRESS:-}"
STATIC_PREFIX="${PREFIX:-}"

WEB_ENABLED="${WEB_ENABLED:-true}"
WEB_LISTEN="${WEB_LISTEN:-0.0.0.0}"
WEB_PORT="${WEB_PORT:-18081}"
WEB_USER="${WEB_USER:-admin}"
WEB_PASS="${WEB_PASS:-}"
WEB_TLS_CERT_FILE="${WEB_TLS_CERT_FILE:-}"
WEB_TLS_KEY_FILE="${WEB_TLS_KEY_FILE:-}"
WEB_TRUST_PROXY_HEADERS="${WEB_TRUST_PROXY_HEADERS:-false}"
WEB_COOKIE_SECURE="${WEB_COOKIE_SECURE:-false}"

log() {
	printf '[nyxproxy] %s\n' "$*"
}

fatal() {
	log "FATAL: $*"
	exit 1
}

bool_value() {
	case "$2" in
		true|false) printf '%s' "$2" ;;
		*) fatal "$1 must be true or false" ;;
	esac
}

if [ -z "$IFACE" ]; then
	IFACE="$(ip route show default 2>/dev/null | awk 'NR==1 {print $5}')"
fi
[ -n "$IFACE" ] || fatal "cannot determine the network interface; set IFACE"
[ -n "$PROXY_PASS" ] || fatal "PROXY_PASS is required"

WEB_ENABLED="$(bool_value WEB_ENABLED "$WEB_ENABLED")"
WEB_TRUST_PROXY_HEADERS="$(bool_value WEB_TRUST_PROXY_HEADERS "$WEB_TRUST_PROXY_HEADERS")"
WEB_COOKIE_SECURE="$(bool_value WEB_COOKIE_SECURE "$WEB_COOKIE_SECURE")"
if [ "$WEB_ENABLED" = "true" ] && { [ -z "$WEB_USER" ] || [ -z "$WEB_PASS" ]; }; then
	fatal "WEB_USER and WEB_PASS are required when WEB_ENABLED=true"
fi

# detect_prefix prints the most preferred usable global /64 on IFACE.
# Deprecated, temporary, tentative, and non-/64 addresses are ignored; the
# prefix with the longest remaining preferred lifetime wins. This deliberately
# does not look at routes: after a PPPoE re-dial the old /64 route can linger
# for hours while its address is already deprecated.
detect_prefix() {
	ip -6 addr show dev "$IFACE" scope global 2>/dev/null | awk '
		function flush() {
			if (addr == "" || deprecated || !is64) return
			pref = (lft == "forever") ? 4294967295 : lft + 0
			print pref, prefix
		}
		/^[0-9]+:/ { flush(); addr=""; deprecated=0; is64=0; lft=0; next }
		/inet6/ {
			flush()
			addr=$2
			deprecated=0; is64=0; lft=0
			for (i=3; i<=NF; i++) if ($i == "deprecated") deprecated=1
			if (addr ~ /\/64$/) {
				is64=1
				split(addr, a, ":")
				prefix=a[1]":"a[2]":"a[3]":"a[4]"::/64"
			}
			next
		}
		/preferred_lft/ {
			for (i=1; i<=NF; i++) {
				if ($i == "preferred_lft") {
					lft=$(i+1)
					sub(/sec$/, "", lft)
					break
				}
			}
		}
		END { flush() }
	' | sort -k1,1nr | awk '!seen[$2]++ {print $2; exit}'
}

# stable_address prints the first non-deprecated, non-temporary global /64
# address on IFACE that belongs to prefix $1. Pool addresses are /128 and are
# therefore never selected.
stable_address() {
	prefix="$1"
	pfx="${prefix%::/64}:"
	ip -6 addr show dev "$IFACE" scope global 2>/dev/null | awk -v pfx="$pfx" '
		/inet6/ {
			addr=$2
			skip=0
			for (i=3; i<=NF; i++) if ($i == "deprecated" || $i == "temporary") skip=1
			if (!skip && addr ~ /\/64$/ && index(addr, pfx) == 1) {
				sub(/\/64$/, "", addr)
				print addr
				exit
			}
		}
	'
}

# dns_source_for prints the address DNS queries should originate from.
# DNS_SOURCE_ADDRESS always wins; otherwise use the stable address of the
# current prefix so a deprecated address can never pin name resolution.
dns_source_for() {
	if [ -n "$DNS_SOURCE_ADDRESS" ]; then
		printf '%s' "$DNS_SOURCE_ADDRESS"
		return 0
	fi
	stable_address "$1"
}

prefix() {
	if [ -n "$STATIC_PREFIX" ]; then
		printf '%s' "$STATIC_PREFIX"
		return 0
	fi
	detect_prefix
}

sed_escape() {
	printf '%s' "$1" | sed -e 's/[\\&|]/\\&/g'
}

yaml_quote() {
	printf "'%s'" "$(printf '%s' "$1" | sed "s/'/''/g")"
}

render_config() {
	p="$1"
	dns="$2"
	mkdir -p "$(dirname "$CONFIG_PATH")"
	chmod 700 "$(dirname "$CONFIG_PATH")"
	umask 077
	sed \
		-e "s|__PREFIX__|$(sed_escape "$(yaml_quote "$p")")|g" \
		-e "s|__PORT__|$(sed_escape "$LISTEN_PORT")|g" \
		-e "s|__USER__|$(sed_escape "$(yaml_quote "$PROXY_USER")")|g" \
		-e "s|__PASS__|$(sed_escape "$(yaml_quote "$PROXY_PASS")")|g" \
		-e "s|__IFACE__|$(sed_escape "$(yaml_quote "$IFACE")")|g" \
		-e "s|__POOL__|$(sed_escape "$POOL_SIZE")|g" \
		-e "s|__MAX_USAGE__|$(sed_escape "$MAX_USAGE")|g" \
		-e "s|__MAX_AGE__|$(sed_escape "$MAX_AGE")|g" \
		-e "s|__DEBUG_LEVEL__|$(sed_escape "$DEBUG_LEVEL")|g" \
		-e "s|__DNS_SOURCE__|$(sed_escape "$(yaml_quote "$dns")")|g" \
		-e "s|__WEB_ENABLED__|$(sed_escape "$WEB_ENABLED")|g" \
		-e "s|__WEB_LISTEN__|$(sed_escape "$(yaml_quote "$WEB_LISTEN")")|g" \
		-e "s|__WEB_PORT__|$(sed_escape "$WEB_PORT")|g" \
		-e "s|__WEB_USER__|$(sed_escape "$(yaml_quote "$WEB_USER")")|g" \
		-e "s|__WEB_PASS__|$(sed_escape "$(yaml_quote "$WEB_PASS")")|g" \
		-e "s|__WEB_TLS_CERT__|$(sed_escape "$(yaml_quote "$WEB_TLS_CERT_FILE")")|g" \
		-e "s|__WEB_TLS_KEY__|$(sed_escape "$(yaml_quote "$WEB_TLS_KEY_FILE")")|g" \
		-e "s|__WEB_TRUST_PROXY__|$(sed_escape "$WEB_TRUST_PROXY_HEADERS")|g" \
		-e "s|__WEB_COOKIE_SECURE__|$(sed_escape "$WEB_COOKIE_SECURE")|g" \
		"$TEMPLATE" > "$CONFIG_PATH"
	chmod 600 "$CONFIG_PATH"
}

# update_config rewrites the prefix-managed fields of an existing config so the
# web console and the Go process agree with the runtime values.
update_config() {
	cfg_prefix="$1"
	cfg_dns="$2"
	[ -f "$CONFIG_PATH" ] || return 0
	tmp="${CONFIG_PATH}.tmp.$$"
	sed \
		-e "s|^\( *ipv6_subnet:\).*|\1 $(yaml_quote "$cfg_prefix")|" \
		-e "s|^\( *dns_source_address:\).*|\1 $(yaml_quote "$cfg_dns")|" \
		"$CONFIG_PATH" > "$tmp"
	chmod 600 "$tmp"
	mv "$tmp" "$CONFIG_PATH"
}

child=""
shutting_down=0
backoff=2

cleanup() {
	shutting_down=1
	if [ -n "$child" ] && kill -0 "$child" 2>/dev/null; then
		kill -TERM "$child" 2>/dev/null || true
		wait "$child" 2>/dev/null || true
	fi
	exit 0
}
trap cleanup TERM INT

wait_sleep() {
	sleep "$1" &
	wait $! || true
}

while :; do
	p="$(prefix)"
	if [ -z "$p" ]; then
		log "waiting for an IPv6 /64 on $IFACE (set PREFIX to override detection)"
		wait_sleep 5
		continue
	fi

	dns="$(dns_source_for "$p")"

	export NYXPROXY_PREFIX="$p"
	export NYXPROXY_INTERFACE="$IFACE"

	if [ ! -f "$CONFIG_PATH" ]; then
		log "seeding $CONFIG_PATH from environment"
		render_config "$p" "$dns"
	else
		update_config "$p" "$dns"
	fi

	log "starting nyxproxy with prefix $p (dns source: ${dns:-auto}) (config: $CONFIG_PATH)"
	nyxproxy -config "$CONFIG_PATH" &
	child=$!
	current_prefix="$p"

	while kill -0 "$child" 2>/dev/null; do
		wait_sleep 10
		kill -0 "$child" 2>/dev/null || break
		now_prefix="$(prefix)"
		if [ -n "$now_prefix" ] && [ "$now_prefix" != "$current_prefix" ]; then
			now_dns="$(dns_source_for "$now_prefix")"
			log "prefix changed from $current_prefix to $now_prefix (dns source: ${now_dns:-auto}); restarting"
			update_config "$now_prefix" "$now_dns"
			kill -TERM "$child" 2>/dev/null || true
			wait "$child" 2>/dev/null || true
			child=""
			backoff=2
			break
		fi
	done

	if [ -n "$child" ]; then
		status=0
		wait "$child" || status=$?
		child=""
		if [ "$shutting_down" = "1" ]; then
			exit 0
		fi
		log "nyxproxy exited with status $status; restarting in ${backoff}s"
		wait_sleep "$backoff"
		backoff=$((backoff * 2))
		[ "$backoff" -le 10 ] || backoff=10
	fi
done
