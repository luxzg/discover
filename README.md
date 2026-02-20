# Discover (Self-Hosted Personal Feed)

Official project home: https://github.com/luxzg/discover

Discover is a single-binary Go application that builds a personal, Discover-like feed by ingesting fresh search results from SearXNG instances, ranking/deduplicating them in SQLite, and serving a mobile-first web UI.

## Goals

- Replace Google Discover-style daily reading with a private self-hosted service
- Use SearXNG JSON endpoints (self-hosted/local by default, swappable instances)
- Keep deployment simple: one Go binary + one JSON config + SQLite file
- Provide `/admin` management for topic queries and negative rules

## Features

- Standalone `net/http` server (no reverse proxy required)
- HTTPS via cert/key paths from config (or HTTP for local testing)
- Feed access protected by user login session (`user_name` + `user_secret`)
- SQLite persistence with `modernc.org/sqlite` (pure Go, no CGO)
- Embedded frontend assets in the binary
- Configurable scheduler:
  - interval mode (`ingest_interval_minutes`, default 120)
  - daily wall-clock mode (`daily_ingest_time`) when interval is disabled
- Manual ingest trigger in admin UI
- URL normalization + hash dedup
- Score model with positive and negative weights
- State model: `unread`, `seen`, `useful`, `hidden`, `read`
- Batch behavior: current batch can be marked `seen` when fetching next
- Optional auto-hide for low-score unread items via `auto_hide_below_score`
- Retention culling for old low-value unread items

## Build

```bash
git clone https://github.com/luxzg/discover.git
cd discover
go mod tidy
go build -o discover ./cmd/discover
```

## First Run

```bash
./discover -config config.json
```

If `config.json` does not exist, the app creates it and exits.
If `config.json` exists, it is never overwritten; startup warns if expected keys are missing.

Edit at least:
- `admin_secret`
- `user_name`
- `user_secret`
- `enable_tls`
- `tls_cert_path` and `tls_key_path` when TLS is enabled
- `listen_address` and `searxng_instances`
- `ingest_interval_minutes` (default `120`; set `0` to use `daily_ingest_time`)
- `feed_min_score` (recommended `1` to avoid low-score cards in feed)
- `auto_hide_below_score` (recommended `1` to suppress low-value unread entries)
- `hide_rule_default_penalty` (default penalty prefill used by feed menu hide actions)

Then run again.

Feed users sign in on `/` with `user_name` and `user_secret`.  
Admin sign-in is separate on `/admin` using `admin_secret`.
For topic/rule examples (`site:domain`, multi-word rule matching), see `USAGE.md`.

## Update Existing Install

If you run Discover via `systemd`, use this update flow:

```bash
sudo systemctl stop discover
sudo su - discover
cd ~/apps/discover
git pull
go mod tidy
go build -o discover ./cmd/discover
exit
sudo systemctl start discover
sudo systemctl status discover
```

## Project Docs

- `README.md` (this file)
- `INSTALL.md` for deployment and systemd setup
- `USAGE.md` for feed/admin usage
- `CHANGELOG.md` for versioned changes
- `SEARXNG.md` for SearXNG install and uninstall

For service uninstall/removal steps, see `INSTALL.md` section `8. Uninstall`.
