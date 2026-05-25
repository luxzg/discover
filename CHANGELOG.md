# Changelog

## 2026-05-25 20:02 CEST - v2.18

- Admin session stability hardening:
  - admin session validation no longer hard-fails on client IP drift (IPv4/IPv6/WiFi changes)
  - admin session now uses sliding TTL refresh on valid activity
  - `/admin/api/session` now refreshes admin cookie expiry to keep browser/session state aligned
- Admin ingestion status visibility:
  - status API now returns `last_messages` (last two progress lines)
  - admin UI ingestion panel now renders `last_messages` while keeping `last_message_at`
- Docs/task alignment:
  - updated `USAGE.md` for admin session and `last_messages`
  - cleaned `TODO.md` (removed completed admin-session follow-up)
  - moved completed tasks into `FINISHED_TASKS.md`

## 2026-05-25 19:42 CEST - v2.17

- Ingest error semantics improved for zero-result topics:
  - when all configured instances are reachable but return no results for a query, ingest now treats that as `results=0` (non-error) instead of logging `no searx instance available`
  - real fetch/rate-limit failures remain logged as errors
- Admin version visibility improvement:
  - `build_version` is now shown in admin `Article Status Counts` panel
  - existing build metadata remains visible in ingestion status

## 2026-05-25 19:11 CEST - v2.16

- Added high-score thumbnail enrichment for empty-thumbnail unread rows:
  - new config key `thumbnail_refresh_min_score` (default `60`)
  - new config key `thumbnail_refresh_max_per_run` (default `40`)
  - during ingest, selected unread rows are scanned for article-page metadata images (`og:image`, `twitter:image`, `twitter:image:src`, `link rel=image_src`)
  - thumbnail updates use set-if-empty semantics to avoid overwriting existing thumbnails
- Config/docs updates:
  - added new keys to defaults, validation, missing-key warnings, and `config.example.json`
  - updated `README.md`, `INSTALL.md`, and `USAGE.md` with thumbnail enrichment behavior/settings
- Task tracking updates:
  - moved completed thumbnail refresh task from `TODO.md` to `FINISHED_TASKS.md`
  - added follow-up TODO for optional manual/admin thumbnail enrichment controls

## 2026-05-25 18:28 CEST - v2.15

- Runtime build/version visibility:
  - startup log now includes build info (`version`, `commit`, `built_at`)
  - admin status API now exposes build metadata under `build`
  - admin UI ingestion panel now shows current running build line for quick deployment verification
- Update script output cleanup:
  - removed nested/mixed step counters from `scripts/update_remote.sh` and `scripts/run_remote_update.sh`
  - kept clear section markers with blank-line separation while making sequence linear and easier to follow
- Docs update:
  - `USAGE.md` now mentions build metadata shown in admin ingestion status panel

## 2026-05-25 18:21 CEST - Workflow Note (no version change)

- Improved verbosity of remote update scripts:
  - added clear step-by-step section `echo` messages in `scripts/update_remote.sh` and `scripts/run_remote_update.sh`
  - added blank lines before each logical section to improve readability during interactive runs
  - remote wrapper output now clearly labels stop/build/start/status/log phases

## 2026-05-25 18:12 CEST - Workflow Note (no version change)

- Fixed remote update scripts for environments where `discover` is not in sudoers:
  - `scripts/update_remote.sh` is now build-only (`git pull`, `go mod tidy`, `go build`) and does not call `sudo`
  - `scripts/run_remote_update.sh` now handles service stop/start/status/logs with caller's sudo rights and runs build step as `discover`
- Updated `INSTALL.md` section `7` to document the corrected permission model and script responsibilities.

## 2026-05-25 18:05 CEST - Workflow Note (no version change)

- Hardened `scripts/run_remote_update.sh` to handle missing remote execute bit:
  - now runs remote `chmod +x /home/discover/apps/discover/scripts/update_remote.sh` before executing it
  - avoids `Permission denied` after pull when execute permissions are not preserved on remote checkout

## 2026-05-25 17:38 CEST - Workflow Note (no version change)

- Added remote-build update scripts:
  - `scripts/update_remote.sh` (run on server as `discover` user)
  - `scripts/run_remote_update.sh` (run locally to trigger remote script over SSH)
- Removed previous local-binary copy deploy helper and switched docs to script-first remote update flow.
- Updated `INSTALL.md` section `7` and `README.md` update section to use the new scripts and keep manual fallback documented.

## 2026-05-25 17:18 CEST - v2.14

- Thumbnail reliability improvements (no DB migration required):
  - feed-read normalization now decodes Startpage proxy thumbnails (`/av/proxy-image`) via `piurl` when valid
  - feed-read normalization now decodes Brave proxy thumbnails (`imgs.search.brave.com`) from base64 path payload when valid
  - inline `data:image/...` thumbnails are preserved as-is
- Rendering fallback for broken images:
  - feed cards now remove failed `<img>` elements on load error instead of showing broken placeholder icons
- Ingest thumbnail behavior adjustment:
  - ingest now keeps original thumbnail source URL shape in DB (validated/safe forms only) rather than rewriting proxy forms
- Docs/task updates:
  - updated `USAGE.md` thumbnail behavior notes
  - moved completed thumbnail fallback/diagnostics tasks from `TODO.md` to `FINISHED_TASKS.md`
  - added new active TODO for optional thumbnail refresh of currently empty-thumbnail rows

## 2026-05-25 11:03 CEST - Docs Note (no version change)

- Added `FINISHED_TASKS.md` and moved recently completed TODO items there
  (thumbnail backfill, feed action UX consistency, and user-session stability
  task closure details).
- Kept only active follow-up work in `TODO.md` per project workflow.

## 2026-05-25 10:58 CEST - v2.13

- Ingest URL-dedupe thumbnail backfill refinement:
  - when an existing article already has a thumbnail, ingest no longer overwrites it
  - thumbnail is now backfilled only when stored thumbnail is empty and new ingest hit provides one
- Feed action UX update:
  - positive actions (`👍 Useful`, click/read) no longer remove cards immediately from current batch view
  - negative actions continue removing cards immediately (`👎 Hide`, `🚫 Hide This`, `🌐 Hide Domain`)
- User session stability improvements:
  - user session TTL increased to 90 days
  - user session now uses sliding refresh on valid activity
  - user session validation/CSRF checks no longer hard-fail on remote IP drift (helps local WiFi/IPv4-IPv6 edge cases)
  - `/api/session` now refreshes user session cookie expiry on successful session restore
- Docs/task updates:
  - updated `USAGE.md` with action-visibility and session behavior notes
  - updated `TODO.md` by removing completed items and keeping admin-session follow-up as an active task

## 2026-05-25 09:52 CEST - Docs Note (no version change)

- Expanded `TODO.md` with newly requested backlog items:
  - thumbnail backfill on URL-dedup ingest hits
  - optional thumbnail-focused fallback fetch strategy
  - broken thumbnail rendering diagnostics task
  - feed action UX consistency for positive vs negative actions
  - session stability / long idle timeout investigation
  - optional deferred offline action queue idea
- Updated `AGENTS.md` git workflow wording to explicitly require multiline
  commit messages (summary + short details).

## 2026-05-25 09:27 CEST - Docs Note (no version change)

- Added project-specific `AGENTS.md` to align repository workflow with the
  coding skill rulebook and current Discover docs.
- Documented explicit read-first order, version/changelog policy, build/format
  defaults, git workflow, and Discover-specific doc-sync expectations for future
  agent sessions.
- Updated `README.md` project docs index to include `AGENTS.md`.

## 2026-02-22 09:47 CET - v2.12

- Admin UI cleanup for dense lists:
  - Topics and Negative Rules panels are now collapsible via arrow toggles
  - both sections are collapsed by default after sign-in to reduce visual clutter
  - expand only when you want to view/edit those lists

## 2026-02-22 09:29 CET - Docs Note (no version change)

- Added service diagnostics logging commands to install docs:
  - `journalctl -u discover --since today`
  - date-range filtering via `--since` / `--until`
  - tailing recent lines via `-n`
  - live follow mode via `-f`
- Added README pointer to `INSTALL.md` diagnostics section.

## 2026-02-22 09:23 CET - v2.11

- Added admin-triggered retroactive dedupe API and UI action:
  - new endpoint: `POST /admin/api/dedupe`
  - new admin button: `Run Retroactive Dedupe`
  - scans whole unread set and applies title-key duplicate suppression
- Retroactive dedupe behavior:
  - when non-unread history exists for a title key, all unread matches are hidden
  - when only unread duplicates exist for a title key, highest-score unread is kept and others are hidden
- Added persistent dedupe counter in DB settings:
  - cumulative all-time hidden duplicate count stored as `app_settings.dedupe_hidden_total`
  - counter increments on ingest-time dedupe and admin retroactive dedupe
  - exposed in `/admin/api/status` and shown in `Article Status Counts` as `dedupe_hidden_total`
- Docs updates:
  - updated `README.md` and `USAGE.md` with retroactive dedupe and persistent counter behavior

## 2026-02-22 09:15 CET - v2.10

- Added ingest-time title duplicate suppression for unread entries:
  - title key is normalized as lowercase alphanumeric-only text
  - key uses configurable prefix length via new `dedupe_title_key_chars` (default `50`)
  - within a single ingest run, the highest-score duplicate is kept unread and the rest are auto-marked `hidden`
  - if a matching title key already exists in non-unread history (`seen`, `read`, `useful`, `hidden`), newly ingested matches are auto-marked `hidden`
- Added ingest progress logging for title dedupe results:
  - reports same-run hidden count and historical-match hidden count
- Config/docs updates:
  - added new config key `dedupe_title_key_chars` to defaults and `config.example.json`
  - updated `README.md`, `USAGE.md`, and `INSTALL.md` with dedupe behavior and configuration details

## 2026-02-20 - v2.9

- Feed card metadata update:
  - added human-friendly relative publish-time label (`today`, `yesterday`, `N days/weeks/months ago`) when publish date is available
  - if publish date is missing/invalid, date text is omitted entirely and card meta remains `domain | score`

## 2026-02-20 - v2.8

- Fixed feed hide-action penalty prefill source:
  - `/api/login` and `/api/session` (user auth endpoints) now return `hide_rule_default_penalty`
  - feed popup penalty defaults now correctly follow `config.json` value (fallback remains `10`)

## 2026-02-20 - v2.7

- Feed negative-action improvements:
  - renamed menu action to `Hide This`
  - added new `Hide Domain` action that extracts URL hostname automatically
  - both actions now prompt for editable penalty (prefilled from config default)
- Added config key `hide_rule_default_penalty` (default `10`) used by feed hide-action penalty prefill.
- User auth API enhancements:
  - `/api/login` and `/api/session` now return `hide_rule_default_penalty` for feed UI defaults
- Negative rule update behavior corrected:
  - updating an existing rule now reapplies penalty by delta for current unread entries
  - disabling a rule reverses its unread penalty effect by delta
  - retroactive `applied_count` increments only on positive-penalty applications

## 2026-02-17 - v2.6

- Feed selection hardening:
  - added `feed_min_score` config filter to exclude low-score unread items from user feed query
  - added second-pass feed dedupe by normalized subject/title to suppress repeated story headlines
- Scheduler frequency improvements:
  - added configurable interval mode via `ingest_interval_minutes` (default 120 minutes)
  - daily mode remains available via `daily_ingest_time` when interval mode is disabled (`ingest_interval_minutes=0`)
- Ingestion expansion for more coverage:
  - for each topic, ingest now requests both `categories=news` and general search
  - ingest now requests both `time_range=day` and `time_range=week`
  - ingest now requests page 1 and page 2 with larger page size (`count=50`)
- Feed empty-state refresh:
  - added user endpoint `/api/feed/refresh` to run manual ingest from feed UI
  - `Load Next` now triggers refresh when zero cards are returned, while honoring running/cooldown guards
- Scheduler/API consistency:
  - introduced explicit cooldown error sentinel handling for both admin/manual and feed-triggered refresh paths
- Docs/config updates:
  - added new keys to config/docs/examples: `ingest_interval_minutes`, `feed_min_score`
  - updated usage docs with new ingest scope and refresh behavior.

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
