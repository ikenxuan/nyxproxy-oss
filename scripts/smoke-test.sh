#!/usr/bin/env bash
# Requires curl and python3. Runs a non-rotating IPv4 proxy so it works on a
# normal Linux host and on GitHub Actions; IPv6 address management is covered
# by the Go unit tests, not this smoke test.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="${1:-$ROOT/dist/nyxproxy}"
WEB_PORT="${SMOKE_WEB_PORT:-18081}"
PROXY_PORT="${SMOKE_PROXY_PORT:-18080}"

if [ ! -x "$BIN" ]; then
  echo "binary not found or not executable: $BIN" >&2
  exit 1
fi

WORK="$(mktemp -d)"
PID=""
cleanup() {
  if [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
  rm -rf "$WORK"
}
trap cleanup EXIT

cat >"$WORK/config.yaml" <<EOF
proxy:
  type: socks5
  listen_address: "127.0.0.1"
  listen_port: $PROXY_PORT
  username: "smoke"
  password: "smoke-pass"
network:
  interface_name: ""
  ipv4_enabled: true
  ipv6_enabled: false
  ipv6_subnet: ""
  dns_source_address: ""
  rotate_ipv6: false
  ipv6_pool_size: 1
  ipv6_max_usage: 1
  ipv6_max_age: 1
monitoring:
  enabled: false
  port: 19090
  allow_remote: false
logging:
  debug_level: 0
web:
  enabled: true
  listen_address: "127.0.0.1"
  port: $WEB_PORT
  username: "admin"
  password: "smoke-secret"
  tls_cert_file: ""
  tls_key_file: ""
  trust_proxy_headers: false
  cookie_secure: false
EOF

"$BIN" -config "$WORK/config.yaml" >"$WORK/stdout.log" 2>"$WORK/stderr.log" &
PID=$!

ready=false
for _ in $(seq 1 100); do
  if curl -fsS "http://127.0.0.1:$WEB_PORT/healthz" >/dev/null 2>&1; then
    ready=true
    break
  fi
  if ! kill -0 "$PID" 2>/dev/null; then
    echo "server exited before becoming ready" >&2
    cat "$WORK/stderr.log" >&2
    exit 1
  fi
  sleep 0.1
done
if [ "$ready" != "true" ]; then
  echo "server did not become ready" >&2
  cat "$WORK/stderr.log" >&2
  exit 1
fi

curl -fsS -c "$WORK/cookies.txt" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"smoke-secret"}' \
  "http://127.0.0.1:$WEB_PORT/api/auth/login" >"$WORK/login.json"

CSRF="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["csrf_token"])' "$WORK/login.json")"

curl -fsS -b "$WORK/cookies.txt" \
  "http://127.0.0.1:$WEB_PORT/api/status" >"$WORK/status.json"
python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); assert d["proxy"]["state"] == "running", d' "$WORK/status.json"

curl -fsS -b "$WORK/cookies.txt" \
  "http://127.0.0.1:$WEB_PORT/api/pool" >"$WORK/pool.json"
python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); assert "pool" in d and "addresses" in d, d' "$WORK/pool.json"

curl -fsS -b "$WORK/cookies.txt" \
  "http://127.0.0.1:$WEB_PORT/api/config" >"$WORK/config.json"

python3 - "$WORK/config.json" "$WORK/update.json" <<'PY'
import json
import sys

payload = json.load(open(sys.argv[1]))["config"]
payload["logging"]["debug_level"] = 1
json.dump(payload, open(sys.argv[2], "w"))
PY

curl -fsS -b "$WORK/cookies.txt" \
  -X PUT \
  -H "Content-Type: application/json" \
  -H "X-CSRF-Token: $CSRF" \
  --data-binary @"$WORK/update.json" \
  "http://127.0.0.1:$WEB_PORT/api/config" >"$WORK/apply.json"

python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); assert d["changed"] is True, d' "$WORK/apply.json"

if [ "$(stat -c %a "$WORK/config.yaml")" != "600" ]; then
  echo "config file mode is not 0600" >&2
  exit 1
fi

curl -sS -N --max-time 1 -D "$WORK/sse.headers" -o /dev/null \
  -b "$WORK/cookies.txt" \
  "http://127.0.0.1:$WEB_PORT/api/logs/stream" || true
grep -qi '^content-type: text/event-stream' "$WORK/sse.headers"

curl -fsS --socks5 "127.0.0.1:$PROXY_PORT" --proxy-user "smoke:smoke-pass" \
  "http://127.0.0.1:$WEB_PORT/healthz" >/dev/null

kill "$PID"
wait "$PID"
PID=""

echo "smoke test passed"
