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

prefix() {
	if [ -n "$STATIC_PREFIX" ]; then
		printf '%s' "$STATIC_PREFIX"
		return 0
	fi
	p="$(ip -6 route show dev "$IFACE" proto ra 2>/dev/null | awk '$1 ~ /\/64$/ {print $1; exit}')"
	if [ -z "$p" ]; then
		p="$(ip -6 route show dev "$IFACE" 2>/dev/null | awk '$1 ~ /\/64$/ {print $1; exit}')"
	fi
	printf '%s' "$p"
}

sed_escape() {
	printf '%s' "$1" | sed -e 's/[\\&|]/\\&/g'
}

yaml_quote() {
	printf "'%s'" "$(printf '%s' "$1" | sed "s/'/''/g")"
}

render_config() {
	p="$1"
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
		-e "s|__DNS_SOURCE__|$(sed_escape "$(yaml_quote "$DNS_SOURCE_ADDRESS")")|g" \
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

	export NYXPROXY_PREFIX="$p"
	export NYXPROXY_INTERFACE="$IFACE"

	if [ ! -f "$CONFIG_PATH" ]; then
		log "seeding $CONFIG_PATH from environment"
		render_config "$p"
	fi

	log "starting nyxproxy with prefix $p (config: $CONFIG_PATH)"
	nyxproxy -config "$CONFIG_PATH" &
	child=$!
	current_prefix="$p"

	while kill -0 "$child" 2>/dev/null; do
		wait_sleep 10
		kill -0 "$child" 2>/dev/null || break
		now_prefix="$(prefix)"
		if [ "$now_prefix" != "$current_prefix" ]; then
			log "prefix changed from $current_prefix to ${now_prefix:-<none>}; restarting"
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
