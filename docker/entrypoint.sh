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
  sqlite.dsn: "/app/data/database.sqlite"
server:
  host: "0.0.0.0"
  port: 3000
  prefork: false
  body_limit: 50
  trusted_proxies: ["10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"]
build:
  mode: production
app:
  name: "GoXCMS"
  version: "0.0.1"
  domain: "localhost"
  url: "${APP_URL}"
  secret: "${GOX_SECRET}"
  admin_password: "${ADMIN_PASSWORD:-}"
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
