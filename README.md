# Welcome to GoX CMS! 🎉

![GoXCMS Admin](https://i.imgur.com/ipHrr9x.png)

GoX CMS is a project that combines Go and HTMX to create a snappy, and enjoyable content management experience. It's a playground for experimenting, learning, and breaking things in a controlled environment.

## Features

- Blog
- Categories
- Tags
- Custom pages
- Comments
- Simple plugin system
    - Shop Plugin
    - Logger Plugin
    - Latest Post Plugin
- Many different themes
- Media manager

## Quick Start 🏁

Requirements: Go (see `go.mod` for the version) and a C compiler (the SQLite driver uses cgo).

1. Clone the repository: `git clone https://github.com/PhantomPixelDev/gox_cms.git`
2. Navigate into the project directory: `cd gox_cms`
3. Copy the example config: `cp config/config-example.yaml config/config.yaml`
4. Set `app.secret` in `config/config.yaml` to a long random value, e.g. the output of `openssl rand -hex 32`. In `build.mode: production` the server refuses to start without one.
5. Run the application: `go run .` (or `make run`), then open http://localhost:3000.

On first start an `admin` account is created. Its password comes from the `ADMIN_PASSWORD` environment variable or `app.admin_password`; if neither is set, a random password is generated and printed once in the log. Change it after logging in.

## Development

- `make test` runs the test suite (`go test ./...`).
- `make lint` runs gofmt, `go vet` and staticcheck, the same checks as CI.
- `make fmt` formats the code.

### Configuration notes

- `app.url`: set it to the public URL. Cookies are marked `Secure` when it starts with `https://`, and it is used for the sitemap and as the default CORS origin.
- `server.trusted_proxies`: list your reverse proxies so client IPs are read from `X-Forwarded-For`.
- `server.prefork`: leave it off with SQLite.
- Custom pages are served as soon as they are saved; no restart is needed.

Explore the code, experiment with changes, and don't hesitate to break things. Your discoveries and creations are what make this project thrive.
