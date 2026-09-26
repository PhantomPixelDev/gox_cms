# GoX CMS — multi-stage image (Debian, cgo enabled for mattn/go-sqlite3).
#
#   docker compose up -d --build
#
# Config is generated on first start by docker/entrypoint.sh from env vars
# (GOX_SECRET required) and persisted in the config volume/file. Data
# (SQLite) and uploads live in /app/data and /app/static/uploads — mount
# named volumes there (see compose.yaml).

FROM golang:1.26-bookworm AS base
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Runs the same suite as CI (minus -race for speed; CI covers -race).
FROM base AS test
RUN go test -count=1 ./...

FROM base AS build
RUN CGO_ENABLED=1 go build -trimpath -o /out/goxcms .

FROM debian:bookworm-slim AS runtime
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -r -u 10001 -d /app app
WORKDIR /app
COPY --from=build /out/goxcms /app/goxcms
COPY --chown=app views ./views
COPY --chown=app static ./static
COPY --chown=app config/config-example.yaml ./config/config-example.yaml
COPY --chown=app docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh \
    && mkdir -p /app/data /app/static/uploads /app/config \
    && chown -R app /app/data /app/static/uploads /app/config
USER app
EXPOSE 3000
ENTRYPOINT ["/entrypoint.sh"]
CMD ["/app/goxcms"]
