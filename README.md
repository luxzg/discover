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
- Manual retroactive unread dedupe trigger in admin UI
- URL normalization + hash dedup
- Conservative story groups with one highest-score unread card and expandable other sources
- Ingest-time title dedupe preserves handled history without letting automatic duplicate hides suppress the retained story
- Persistent dedupe counter:
  - stores cumulative hidden-duplicate total in DB and shows it in admin status
- Stable per-topic evidence scoring with reversible negative-rule effects; repeated identical results no longer inflate scores
- Publication dates shown when supplied, including previously stored dates in legacy SQLite formats
- State model: `unread`, `seen`, `useful`, `hidden`, `read`
- Batch behavior: current batch can be marked `seen` when fetching next
- Optional auto-hide for low-score unread items via `auto_hide_below_score`
- Retention culling for old low-value unread items

## Build

Requires Go 1.26.8 or newer (a supported security-patched toolchain), Git, Bash
and standard Linux utilities. The Go command can download the required toolchain
when `GOTOOLCHAIN=auto`; otherwise install it yourself. See `DEVELOPMENT.md` for
tests and dependencies.

```bash
git clone https://github.com/luxzg/discover.git
cd discover
./scripts/build.sh
./discover --version
```

## First Run

```bash
./discover -config config.json
```

If `config.json` does not exist, the app creates it and exits.
If `config.json` exists, it is never overwritten; startup warns if expected keys are missing.
Missing keys use in-memory defaults. Unknown keys, null values, invalid CIDRs,
and insecure default credentials fail validation instead of being silently ignored.

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
- `dedupe_title_key_chars` (default `50`; title-key prefix length used by ingest duplicate hiding)
- `thumbnail_refresh_min_score` (default `60`; only unread items at/above this score are considered for thumbnail enrichment)
- `thumbnail_refresh_max_per_run` (default `40`; max high-score empty-thumbnail rows processed per ingest run)
- `hide_rule_default_penalty` (default penalty prefill used by feed menu hide actions)

Then run again.

Check configuration first with `./discover --check-config -config config.json`.

Feed users sign in on `/` with `user_name` and `user_secret`.  
Admin sign-in is separate on `/admin` using `admin_secret`.
For topic/rule examples (`site:domain`, multi-word rule matching), see `USAGE.md`.

## Update Existing Install

If you run Discover via `systemd`, use this update flow:

```bash
git status --short
git pull --ff-only
# Review local scripts/deploy.sh before it is sent to the server with sudo.
./scripts/run_remote_update.sh
```

Or pass host/user as arguments:

```bash
./scripts/run_remote_update.sh -ip 10.10.10.10 -user myusername
```

For remote/server-side and manual fallback options, see `INSTALL.md` section `7`.

The wrapper builds before stopping the service, backs up SQLite/config/binary,
then replaces the binary and restarts. It never automatically restores a database.
The first upgraded start migrates derived story identities and date storage;
existing scores and deliberate/legacy hide decisions are preserved. See `USAGE.md`
for scoring and history limitations before changing thresholds.

## Project Docs

- `AGENTS.md` for project-specific AI agent workflow/rules
- `README.md` (this file)
- `INSTALL.md` for deployment and systemd setup
- `DEVELOPMENT.md` for validation scripts, architecture, and review checks
- `USAGE.md` for feed/admin usage
- `CHANGELOG.md` for versioned changes
- `TODO.md` for active open work
- `FINISHED_TASKS.md` for completed/deferred task archive
- `SEARXNG.md` for SearXNG install and uninstall

## Privacy Boundary

Feed and admin data require separate authentication; admin CIDRs are enforced
independently. Static HTML/JS/CSS contain no credentials. Thumbnail enrichment
fetches only public HTTP(S) destinations using a dedicated DNS/redirect-checked
client; the configured local SearXNG instance uses a separate client.
Displayed thumbnails still load directly in the browser, so image hosts receive
the reader's IP/browser request metadata and may receive their own cookies
according to browser policy. They do not receive Discover's authentication cookies.
Use HTTPS when the service is accessible beyond a trusted test environment.

For log checks and diagnostics commands, see `INSTALL.md` section `6.1 Diagnostics (journalctl)`.
For service uninstall/removal steps, see `INSTALL.md` section `8. Uninstall`.
