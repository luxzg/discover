# Discover (Self-Hosted Personal Feed)

Discover is a single-binary Go application that builds a personal, Discover-like feed by ingesting fresh search results from public SearXNG instances, ranking/deduplicating them in SQLite, and serving a mobile-first web UI.

## Goals

- Replace Google Discover-style daily reading with a private self-hosted service
- Use public SearXNG JSON endpoints (swappable instances)
- Keep deployment simple: one Go binary + one JSON config + SQLite file
- Provide `/admin` management for topic queries and negative rules

## Features

- Standalone `net/http` server (no reverse proxy required)
- HTTPS via cert/key paths from config (or HTTP for local testing)
- SQLite persistence with `modernc.org/sqlite` (pure Go, no CGO)
- Embedded frontend assets in the binary
- Fixed-time daily ingestion scheduler (wall-clock anchored, no drift)
- Manual ingest trigger in admin UI
- URL normalization + hash dedup
- Score model with positive and negative weights
- State model: `unread`, `seen`, `useful`, `hidden`, `read`
- Batch behavior: current batch can be marked `seen` when fetching next
- Retention culling for old low-value unread items

## Build

```bash
go mod tidy
go build -o discover ./cmd/discover
```

## First Run

```bash
./discover -config config.json
```

If `config.json` does not exist, the app creates it and exits.

Edit at least:
- `admin_secret`
- `enable_tls`
- `tls_cert_path` and `tls_key_path` when TLS is enabled
- `listen_address` and `searxng_instances`

Then run again.

## Project Docs

- `INSTALL.md` for deployment and systemd setup
- `USAGE.md` for feed/admin usage
- `CHANGELOG.md` for versioned changes
- `SEARXNG.md` for SearXNG install and uninstall

## END
