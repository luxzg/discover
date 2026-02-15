# Changelog

## 2026-02-15 - v1.4

- Updated default SearXNG config to local self-hosted instance:
  - `searxng_instances` default now `http://localhost:8888`
  - `per_query_delay_seconds` default now `5`
  - `per_query_jitter_seconds` default now `5`
- Applied the same defaults to `config.example.json`.

## 2026-02-15 - Pre-Push Hygiene (no version change)

- Added `.gitignore` to prevent committing local secrets/state/artifacts (`config.json`, DB/WAL/SHM, cookies, binaries, logs, IDE folders, build outputs).
- Sanitized `GITHUB.md` to replace machine/user-specific path and remote URL with placeholders for public sharing.

## 2026-02-14 - v1.3

- Updated feed card rendering so missing thumbnails no longer show a black placeholder block.
- Cards without images now allow text content to use full available width.

## 2026-02-14 - v1.2

- Added admin `edit` buttons for topics and negative rules next to delete actions.
- Edit action now prefills the Add/Update inputs (text, weight/penalty, enabled state) for quick correction/update.

## 2026-02-14 - v1.1

- Updated UI text wrapping for `<pre>` status blocks to keep long messages inside panel bounds.
- Added `pre-wrap` and aggressive word wrapping (`overflow-wrap:anywhere`, `word-break:break-word`) for admin/feed status readability.

## 2026-02-14 - v1.0

- Updated ingest run result semantics so complete fetch outage is treated as a run error.
- Ingestion now returns an error when all enabled topics fail to fetch, which populates scheduler/admin `last_error`.
- Final ingest summary message now includes `failed_topics` count.

## 2026-02-14 - v0.9

- Added ingest progress snapshot support to expose the latest ingestion progress message in admin status API.
- `/admin/api/status` now includes:
  - `ingest.state` (existing scheduler state)
  - `ingest.last_message` (latest ingest log line, e.g. topic done/all done)
  - `ingest.last_message_at` timestamp
- Updated admin UI ingestion panel to display `last_message` and `last_message_at` alongside running/source/error fields.

## 2026-02-14 - v0.8

- Improved admin manual-ingest button feedback:
  - clear disabled styling (`not-allowed` cursor, reduced opacity/saturation)
  - busy visual state and dynamic label (`Run Now (Running...)`)
- Improved manual-ingest status messaging:
  - duplicate clicks while running show explicit ignored message
  - cooldown responses are shown as `manual ingest cooldown` instead of generic failure
- Switched admin action status timestamps to local-time formatting for readability.

## 2026-02-14 - v0.7

- Added scheduler guard to reject immediate back-to-back ingestion runs for a short cooldown window after completion.
- Hardened admin manual-ingest UX against duplicate triggers:
  - disables `Run Now` while a manual request is in-flight
  - keeps button disabled while ingestion is currently running (from live status)
- Retained source-aware scheduler logs (`manual`/`scheduled`) to help diagnose run origin in repeated-run scenarios.

## 2026-02-14 - v0.6

- Improved ingestion progress logs to separate network/work duration from inter-topic delay.
- Added explicit CLI sleep log between topics: `ingest: sleeping ... before next topic`.
- Adjusted per-topic timing so `topic done ... took=...` reflects fetch/process time only (not delay).
- Updated admin UI layout so action/status messages are shown in a dedicated `Actions` panel, separate from article status counts.

## 2026-02-14 - v0.5

- Added clearer ingestion lifecycle visibility:
  - CLI logs now show ingest start, per-topic completion, and final completion with elapsed duration.
  - Scheduler logs now show run source (`manual`/`scheduled`) plus total run duration and errors.
- Added admin runtime status endpoint `/admin/api/status` with:
  - live ingest state (`running`, source, started_at, last_completed_at, last_duration_ms, last_error)
  - article status counters (`unread`, `seen`, `read`, `useful`, `hidden`)
- Updated admin UI:
  - shows live ingest state and status counters
  - polls status every 3 seconds
  - shows immediate “manual ingest requested (running...)” message when triggering manual ingestion
- Improved feed card UX:
  - card thumbnail moved to the left, title/content area on the right
  - only one action menu can be open at a time
  - tapping outside any menu closes open menus

## 2026-02-14 - v0.4

- Fixed feed retrieval robustness by parsing SQLite datetime fields from multiple storage formats in unread query results.
- Updated feed frontend to show explicit API/load/action errors instead of silently rendering empty list.
- Added feed status panel to display loaded card counts and operation outcomes.

## 2026-02-14 - v0.3

- Improved ingestion resilience against SearXNG `429` rate limits.
- Added per-instance temporary cooldown tracking using `Retry-After` (with safe defaults).
- Randomized instance order per topic to spread load.
- Added fallback query mode: try `categories=news` first, then retry without category filter.
- Improved 429 error reporting with actionable guidance when all instances are rate-limited.

## 2026-02-14 - v0.2

- Fixed admin UI auth flow for API calls by auto-using `?secret=` from page URL as request fallback.
- Prefilled admin secret input from URL secret when local saved secret is absent.
- Added explicit success/failure status messages for topic/rule add/delete and manual ingest actions.
- Clarified admin input label to indicate it is API authentication secret, not runtime credential mutation.

## 2026-02-14 - v0.1

- Initialized Go project structure for Discover service.
- Added static config bootstrap/validation, including refusal to start with default admin secret.
- Implemented SQLite migrations and persistence layer for topics, negative rules, and articles.
- Implemented SearXNG JSON ingestion with instance failover, URL normalization, dedup hash strategy, scoring, and penalties.
- Implemented fixed-time daily scheduler plus manual ingest locking.
- Implemented HTTP(S) server with feed/admin APIs and admin guard (secret + CIDR checks).
- Added embedded mobile-first feed UI and minimal admin UI.
- Added deployment and usage documentation (`README.md`, `INSTALL.md`, `USAGE.md`).
