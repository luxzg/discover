Task:

Build a self-hosted personal content feed service that will replace "Google Discover" on Android's Google Chrome. Build it as a single self-contained Go binary running on Debian/Ubuntu Linux. The goal is to replace Chrome/Google Discover behavior without using any Google apps, by pulling fresh results daily from 3rd party public SearXNG instances via RSS for a user-defined set of keyword searches, storing results in SQLite database, deduplicating, scoring, and serving a mobile-friendly web UI with “cards” and read/dismiss behavior similar to Discover.

Core requirements and constraints:

1. Runtime/Deployment

* Must be Go, standard library HTTP server (net/http), with direct TLS support (no reverse proxy, no Apache required for this service). The binary should be runnable on Debian/Ubuntu as a standalone binary and being able to easily set it up as a systemd service.
* Use SQLite with the “modernc” pure-Go driver (no CGO).
* Embed static frontend assets (HTML/JS/CSS) into the binary so deployment is “copy binary + config + run”.
* TLS cert/key come from existing Let’s Encrypt files on disk; service reads those paths from config and serves HTTPS directly.
* Use sensible HTTP server timeouts and safety defaults (read/write/idle), and avoid large request bodies.

2. Configuration model (static file + DB-managed dynamic config)

* The service must load a simple static editable configuration file (eg JSON ir ENV, NOT YAML) from disk at startup for “boot-time” settings:

  * listen address/port
  * tls cert path
  * tls key path
  * admin authentication secret
  * daily ingest time (fixed wall-clock time, local server time)
  * list of multiple SearXNG instance base URLs to use for fetching RSS search results
  * per-query delay (seconds) used to spread requests to avoid rate limiting
  * any other small operational toggles that are truly static
* On first run, if config file does not exist, the binary should generate a template/default config file with safe defaults, print a clear message that it was created and needs editing, then exit.
* The binary must refuse to start if the admin auth secret is still the default placeholder, printing a warning to edit config file. Do not run insecurely with default credentials.
* Anything “user editable day-to-day” must NOT be in this config file; it must be stored in SQLite DB, in a separate configuration table(s), and managed via a small admin UI. Specifically:

  * Topic searches (the list of keywords / search queries), with enable/disable and optional weight.
  * Negative rules / penalties (user-defined keywords and penalty weights).

3. Ingestion scheduling (internal scheduler, fixed daily time, no drift)

* Implement internal scheduler inside the Go process (no cron required).
* It must run at a fixed time of day (e.g. 07:30 local time) anchored to wall-clock, not “24h after finish”, to avoid time drift.

  * Compute next run time based on current server local time and configured daily ingest time.
  * Sleep until next run time.
  * Execute ingestion.
  * After ingestion completes (whether fast or slow), compute the next run again and sleep until that next fixed wall-clock time.
* During ingestion, spread outbound requests to third-party SearXNG instance(s) to avoid rate limits:

  * Execute queries sequentially and sleep between requests (can be configurable in static config file, e.g. 60–120 seconds, add small jitter for randomization).
  * Total is small (around 20-30 topics/day), so this should be gentle on public instances.
* Allow manual “ingest now” trigger from admin UI, but still keep the daily schedule intact.

4. Data sources (public SearXNG instances, JSON per search)

Example search query (working example):
https://search.unredacted.org/search?q=archaeological+discovery&time_range=week&format=json

* For each enabled topic query, build a request to SearXNG (JSON output) to fetch results for “today/past week” freshness. Use SearXNG JSON support. The output is JSON.
* This system intentionally does not self-host SearXNG initially; it uses public instances (e.g. from searx.space), but the design should allow swapping instance URLs in config file later.
* Fetching is read-only; do not rely on any stable session features of the instance.

JSON samples from the example search query endpoint are available in:
`~/dev/discover/JSON_SAMPLES.md`. If required more samples can be added.

From analyzing real JSON responses from a public SearXNG instance:

* The **top-level object** includes `"query"`, `"results"` array, and sometimes `"answers"`, `"suggestions"`, `"unresponsive_engines"`, etc.
* Results are contained in a `"results"` array, each representing a search hit.

  * Fields reliably present per result include:

    * `url` (string)
    * `title` (string)
    * `content` (string snippet)
    * `engine` (primary engine label)
    * `engines` (array of contributing engines)
    * `parsed_url` (array where index `[1]` is the domain)
    * `positions` (positions per engine)
    * `score` (numeric relevance score)
    * `category` (e.g., `"news"`)
  * Optional but useful fields when supported:

    * `publishedDate` or `pubdate`: ISO timestamps for news articles (not always present, but often in news category)
    * `thumbnail`: URL to a preview image (present on some news results but frequently missing)
* When JSON format is enabled on the instance, **JSON API returns structured fields**, which is significantly easier to parse than RSS/XML and provides more metadata per result. ([docs.searxng.org][1])
* Some entries contain image proxy URLs (e.g., via startpage or Bing news), and some have NULL/empty `thumbnail`/`img_src`. The UI logic must defensively handle missing image fields.
* Instances may list **`unresponsive_engines`** in the root response when specific engines are rate-limited or unavailable — this can be ignored.
* The `number_of_results` field is not a reliable count of actual results; instead, count entries by inspecting the length of the `results` array in JSON.
* The presence of `publishedDate` allows sorting by actual news timestamp when available; otherwise fallback to ingestion time or heuristic.

In summary:

* JSON results from SearXNG provide structured metadata ideal for ingestion.
* **Published dates and thumbnails are only sometimes present**, especially for news category results, so ingestion logic must be resilient.
* The `score` and `engines` fields are useful for weighted ranking and multi-engine consensus scoring.

5. Parsing + normalization + dedup

* JSON - parse robustly.
* Extract at minimum: title, link/URL, publication date (if present, if nos use ingestion date), source (derive from feed item or domain)
* Optional but retrieved if they are populated: thumbnail image and content (snippet/description).
* Normalize URLs for dedup (canonicalize enough to avoid trivial duplicates, remove tracking params if reasonable, but do not over-aggressively break legitimate unique pages).
* Deduplicate primarily by URL hash (e.g., sha256 of normalized URL). Do NOT dedupe by title or content.
* When the same URL appears across multiple topic searches, store only one article record but increase its “hit count” / score accordingly.

6. Scoring model (positive + negative; discover-ish ranking)

* Maintain a numeric score per article.
* Positive scoring:

  * When an article is newly ingested due to a topic match, increase score by a base amount, and allow per-topic weight to scale it.
  * If the same URL appears in multiple topics, score should increase for each hit (e.g. +1 per hit) so multi-topic matches float to the top.
  * If same URL appears in multiple search engines, increase score as well
  * If JSON returns score or ranking, use it to increase score as well
  * Optionally add extra points when the query terms appear in the title vs description, with different weights (title > description).
* Negative scoring:

  * Maintain user-defined “negative rules” as patterns (simple substring match is acceptable; no regex).
  * Apply penalties during ingestion so undesired content sinks:

    * Example patterns like “buy now” or “Trump” subtract points.
    * Support strong penalties (e.g. -10) for “don’t show” style rules.
    * When user creates new negative rule, apply retroactively to all "unread" entries in the database
* Persist both hit counts and score so ranking is stable and explainable.
* Ranking order for display should be primarily by score desc, then by publication date desc (and maybe tie-breakers).

7. State model: unread/seen/read/useful/hidden
   Replicate Discover-like behavior:

* Articles are ingested as “unread”.
* UI loads a small batch (e.g. 10 at a time) from the top of the unread pool (no offset pagination). Always “top unread”.
* When the user reaches the end of the current batch and does a “pull to refresh / load next”, the system should mark that batch as “seen” (neutral) automatically (similar to Discover where older ones effectively disappear as you move on).
* Provide explicit user actions per card:

  * “Thumb up” = mark as “useful” (and optionally boost score).
  * “Thumb down” = mark as hidden (and optionally penalize score).
  * “Mark as read” is NOT needed separately from "useful" or "seen".
  * “Don’t show” = prompt user for a negative pattern/rule and apply a strong penalty; also mark that article as hidden, and apply retroactively to all "unread" entries so they don't show after user applies the new rule.
* The system must store these states in SQLite. The unread selection query must exclude anything not currently unread (seen, useful, hidden).
* When a user clicks an article to open it, mark it as "read", this is only way to get the "read" status; keep it simple and consistent.
* If a user later uses "Thumb down" or "Don't show" on item with "read" status, change the status to "hidden" (overwriting "read") to account for click-baits

8. UI requirements (mobile-first, Discover-like cards)

* Build a simple web UI served by the Go app (embedded assets).
* It should look like a Discover feed:

  * dark theme
  * vertical list of simple cards
  * each card shows thumbnail (if available), headline/title, and source/domain
  * thumbnail is on the right, title is prominent, source/domain is smaller in the bottom
  * commands (thumb up/down, don't show) can be hidden behind a 3-dot or hamburger menu on the bottom right corner so they're not wasting UI space and aren't easy to mis-tap by accident
  * tap/click anywhere on the card (except the menu) opens article link in a new tab
* The web UI is the primary reader.
* Implement “lazy load” as “top unread batch”:

  * fetch 10 items (can be configurable in static config file, default to 10 if not set)
  * display each item in own card
  * when user requests more (scroll end + explicit action, or pull-to-refresh), mark the current batch as "seen" and fetch the next top unread batch (again limit 10 or as configured, no offsets).
* Provide card controls:

  * buttons rather than swipe actions
  * thumbs up, thumbs down, don’t show - behind a small menu button
  * all buttons use symbols, not dedicated graphics (no gif/png/svg) for clean and lightweight UI
* Implement the “batch state” correctly:

  * When UI fetches a batch, it should keep track of the IDs returned.
  * When user triggers “next” or "refresh", send those IDs back to backend to mark as "seen" (neutral), then request the next batch.
* Admin UI:

  * A simple /admin page protected by the admin auth secret.
  * Ideally whole /admin available only via local LAN address (maybe via configurable range in static config file)
  * Ability to add/edit/remove/enable/disable topics for searche queries.
  * Ability to add/edit/remove/enable/disable negative rules and their penalty weights.
  * Ability to trigger “ingest now” manually on demand.
  * Keep the admin UI minimal but functional (same dark theme, no images).

9. API/Backend behavior

* Provide endpoints or similar structures for:

  * fetching next batch of unread articles (limit parameter OK, default 10)
  * marking a set of article IDs as seen/useful/hidden
  * adding new negative rule (allowed for user via card's menu)
  * advanced managment of topics and negative rules (admin only)
* Ensure dedup and scoring happen in backend, not JS/front.
* Make sure operations are idempotent where practical.

10. Persistence, retention, and culling

* This will ingest ~200–500 articles/day; SQLite will handle it.
* Implement a retention/culling mechanism:

  * periodic job, configurable, can be triggered daily but only act on older records
  * delete very old low-value items (e.g., older than a month + low score + unread)
* The goal is: keep years of “read/useful” history, while pruning junk automatically.
* Provide a straightforward policy (configurable later, but include a sensible default).
* We will be keeping items manually marked or acted upon by the user, to provide information for later analysis and possible adjustments and betterment of the application.

11. Operational behavior and robustness

* On startup:

  * verify config presence and sanity
  * refuse to run with default admin credentials
  * open/create SQLite DB
  * run migrations automatically if needed
  * start HTTPS server
  * start scheduler goroutine
* Ingestion robustness:

  * handle timeouts, temporary failures, invalid JSON gracefully
  * if one instance fails, try next instance (config provides multiple)
  * log enough to diagnose, but keep it simple
* Concurrency:

  * Only one user, but ensure DB access is safe.
  * Ensure ingestion doesn’t break UI queries (use proper DB locking patterns; SQLite is fine for this scale).
  * Prevent simultaneous ingestion runs (e.g., block manual ingest while scheduled ingest is running).

12. Key non-goals / boundaries

* Do not attempt to build a full web crawler or search index. This relies on metasearch RSS results from SearXNG instances (public or private, but separate from this project).
* Do not integrate Google services or applications.
* Do not require Docker, Python venv, Node tooling, reverse proxy, or Apache/nginx style dedicated web servers.

Deliverable:

* A working Go application from a blank folder that, once configured and started, provides:

  * HTTPS web UI with Discover-like card feed
  * /admin to manage topics and negative rules (saved in SQLite)
  * daily scheduled ingestion anchored to a fixed time, with per-query delay spreading
  * dedup and scoring (positive and negative)
  * read/seen/useful/hidden states and batch “mark as seen” behavior
  * periodic culling preserving useful history

Make sensible engineering choices without overengineering, but do not skip any of the behaviors described above.

Testing:

* Testing will be done manually by user
* For testing purposes only application needs to allow disabling HTTPS/TLS and allow http://<localhost>:<port> style deployment, configurable in config file

Documentation:
* IDEA.md (this file) - describes the idea and goals of the project, acting as a prompt and early documentation for the Codex as well as the developer/user
* JSON_SAMPLES.md - contains sample JSON output of the actual real search queries given by 3rd party SearX service
* README.md - to be created by Codex agent, needs to include project goals, short description, and brief instructions to build, deploy, and configure
* INSTALL.md - detailed instructions how to deploy the binary, setup systemd service (with example systemd system unit file), and configure the configuration file
* USAGE.md - brief description how to access running service, the access to /admin web interface, and how to use it
* INITIAL_PROMPT.md - only used to show what was used to start the Codex actions.
* CHANGELOG.md - date and version, starting from today and version 0.1, with all changes made to either code or documentation
