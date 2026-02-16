# Changelog

## 2026-02-16 - v2.5

- Ingest query fallback fix:
  - when `categories=news` returns HTTP 200 with zero results, ingest now falls back to general search instead of stopping early
  - improves coverage for `site:...` queries and other niche filters that may be missing from the news category

## 2026-02-16 - Docs Note (no version change)

- Clarified rule-token syntax in docs:
  - `get+off` is equivalent to `get off`
  - token order does not matter (`get off` == `off get`)

## 2026-02-16 - v2.4

- Admin auth/session improvements:
  - admin session cookie lifetime increased to 24 hours
  - added admin session restore endpoint (`/admin/api/session`)
  - admin UI now restores authenticated state after page reload when cookie is still valid
- Added admin statistics (Part A):
  - topics list now shows `unread` and `total` article counts per topic
  - negative rules list now shows lifetime `applied` count
- Added negative rule applied-count tracking:
  - new DB column `negative_rules.applied_count` with safe migration
  - counter increments during ingest when a rule penalty is matched
  - counter increments during retroactive apply for unread matches
- Added `TODO.md` with future roadmap item for automatic topic suggestions from reading history.

## 2026-02-16 - v2.3

- Config behavior hardening:
  - existing `config.json` is never overwritten
  - missing config keys now emit startup warnings
  - missing keys in older configs inherit defaults at runtime
- Added new config key `auto_hide_below_score` (default `1`):
  - after ingestion, unread items with score below threshold are auto-marked `hidden`
- Improved manual ingest stability:
  - manual ingest run no longer gets canceled by request/client disconnects
- Improved rule matching behavior:
  - negative rules now use token matching (space or `+` separated words)
  - matching now applies against title/content/domain/url
  - retroactive rule application on unread entries uses the same matcher
- Feed UX:
  - `Load Next` now scrolls to top automatically
- Admin UI:
  - added direct `Open Discover` link from `/admin` to `/`
  - added concise topic/rule examples with `Learn more` docs link
- Dedupe normalization:
  - URL normalization now strips query strings before hashing to reduce duplicate links with tracking parameters
- Docs updated:
  - `README.md`, `INSTALL.md`, `USAGE.md`, `config.example.json` aligned with new config key and query/rule guidance.

## 2026-02-15 - v2.2

- Added CSRF token validation for all mutating user and admin APIs.
- Added session CSRF token exposure on login and user session restore endpoint (`/api/session`) for UI usage.
- Updated feed/admin frontends to send `X-CSRF-Token` on mutating requests.
- Protected both logout endpoints with CSRF checks as well.

## 2026-02-15 - v2.1

- Updated Discover sign-in panel behavior after authentication:
  - hide panel title, username/password fields, and Sign In button
  - keep only Sign Out visible in the panel while signed in

## 2026-02-15 - Docs Consistency (no version change)

- Aligned `README.md` and `INSTALL.md` with current split-auth model:
  - feed sign-in uses `user_name` + `user_secret`
  - admin sign-in uses `admin_secret`
- Clarified manual test flow in `INSTALL.md` to include feed sign-in after successful ingestion.
- Added explicit uninstall/remove steps in `INSTALL.md` and linked them from `README.md`.

## 2026-02-15 - v2.0

- Added end-user access protection for feed service with `user_name` + `user_secret` from config.
- Added user sign-in/sign-out endpoints (`/api/login`, `/api/logout`) with 30-day session cookie (`HttpOnly`, `SameSite=Strict`, `Secure` when TLS enabled).
- Protected all feed APIs behind authenticated user session:
  - `/api/feed`
  - `/api/feed/seen`
  - `/api/articles/action`
  - `/api/articles/click`
  - `/api/articles/dontshow`
- Updated Discover web UI with user sign-in panel and session-aware behavior (including session resume on reload).
- Fixed admin stored-XSS risk by escaping visible topic/rule text before rendering list HTML.
- Hardened ingestion link safety by accepting only `http`/`https` URL schemes during normalization.
- Added required config keys and defaults:
  - `user_name`
  - `user_secret`
  and updated docs/examples accordingly.

## 2026-02-15 - v1.9

- Admin UI now hides operational panels until authentication succeeds.
- Visible before sign-in: Sign In/Out panel and Actions log panel.
- Hidden before sign-in: Topics, Negative Rules, Ingestion, and Article Status Counts panels.

## 2026-02-15 - v1.8

- Removed lingering admin-auth compatibility fallback on protected routes: admin APIs now require valid session cookie only.
- Restricted `/admin` HTML page itself with the same CIDR policy as admin APIs.
- Added `Cache-Control: no-store` on `/admin` response to reduce browser caching of admin page state.

## 2026-02-15 - v1.7

- Security hardening for admin authentication:
  - removed query-string admin auth flow from UI/docs (`?secret=` no longer used)
  - added cookie-based admin sessions (`/admin/api/login`, `/admin/api/logout`) with HttpOnly and SameSite=Strict
  - added auth brute-force protection with failed-attempt tracking and temporary blocking
- Added `Referrer-Policy: no-referrer` for admin page and admin auth API responses.
- Updated admin UI to explicit Sign In/Sign Out flow and session-aware behavior.
- Updated docs (`USAGE.md`, `INSTALL.md`) to match the new admin sign-in flow.

## 2026-02-15 - v1.6

- Added unobtrusive “Powered by luxzg/discover” project link in both Discover and Admin page headers.
- Increased Discover card title font size by ~30% for better readability.

## 2026-02-15 - Docs Update (no version change)

- Reworked `INSTALL.md` to follow dedicated-user deployment flow:
  - create `discover` user first
  - install Go and PATH for that user
  - clone/build under user home directory
  - run systemd using the same user and home-based paths
- Kept install instructions focused on required steps only (no optional tooling additions).
- Updated Go install example in `INSTALL.md` to user-local install (`$HOME/go`) without `sudo` for tar extraction.
- Added update workflow documentation in `README.md` and `INSTALL.md` for existing `systemd` deployments (`stop -> pull -> tidy -> build -> start`).

## 2026-02-15 - v1.5

- Updated `README.md` with official project home link: `https://github.com/luxzg/discover`.
- Updated build instructions in `README.md` and `INSTALL.md` to include `git clone` + `cd discover` before build commands.
- Updated remaining install config example from public SearXNG URL to local self-hosted default (`http://localhost:8888`).

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
