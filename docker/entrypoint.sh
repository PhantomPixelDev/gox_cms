#!/bin/sh
# Generates /app/config/config.yaml on first start from env vars, then execs
# the CMS. Existing config is never overwritten, so redeploys keep settings.
set -eu

CONFIG_FILE="/app/config/config.yaml"

mkdir -p /app/data /app/static/uploads /app/config

if [ ! -f "$CONFIG_FILE" ]; then
  if [ -z "${GOX_SECRET:-}" ]; then
    echo "ERROR: GOX_SECRET is not set. Generate one with:" >&2
    echo "  openssl rand -hex 32" >&2
    exit 1
  fi

  APP_URL="${APP_URL:-http://localhost:3000}"

  cat > "$CONFIG_FILE" <<EOF
# Generated on first start by docker/entrypoint.sh — edit freely, it is kept.
database:
  driver: sqlite
  sqlite.dsn: "/app/data/database.sqlite?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL"
server:
  host: "0.0.0.0"
  port: 3000
  prefork: false
  body_limit: 50
  # Empty on purpose. Anything listed here makes the app take the client IP
  # from X-Forwarded-For, so an attacker whose address falls in the list can
  # rotate that header and reset their login-throttle and rate-limit budget.
  # Set this ONLY to the exact address of a proxy that really is in front,
  # e.g. ["172.18.0.2"] for a specific Docker network peer. The login throttle
  # also keys on the un-spoofable socket peer, so it stays correct either way.
  trusted_proxies: []
build:
  mode: production
app:
  name: "GoXCMS"
  version: "0.0.1"
  domain: "localhost"
  url: "${APP_URL}"
  secret: "${GOX_SECRET}"
  admin_password: "${ADMIN_PASSWORD:-}"
  session_hours: ${GOX_SESSION_HOURS:-12}
auth:
  login_max_attempts: ${GOX_LOGIN_MAX_ATTEMPTS:-10}
  login_window_minutes: ${GOX_LOGIN_WINDOW_MINUTES:-5}
upload:
  max_size_mb: 50
redis:
  enabled: false
cors:
  allowed_origins: ["${APP_URL}"]
  allow_credentials: false
ratelimiter:
  enabled: true
  max_requests: 100
captcha:
  enabled: false
  public_key: ""
  secret_key: ""
EOF
  echo "Wrote $CONFIG_FILE"
  if [ -z "${ADMIN_PASSWORD:-}" ]; then
    echo "ADMIN_PASSWORD not set — a random admin password will be printed in the container log on first start. Change it after logging in."
  fi
fi

exec "$@"
