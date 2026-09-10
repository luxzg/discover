# Usage

## Feed UI

- Open `/` in browser
- Sign in with `user_name` and `user_secret`
- While signed in, feed header/auth row shows current app version next to `Sign Out`
- Feed shows top unread cards sorted by score/date
- One card represents a conservative normalized-title story; expand `Other sources` for up to 20 alternate URLs. Reading an alternate also handles that story.
- Card metadata is `domain | score | published date` with short relative dates. Unknown/invalid dates and their separator are omitted. Sorting falls back to ingestion time without presenting it as publication time.
- Tap card to open article (marks it as `read`)
- Card menu actions:
  - `👍 Useful` -> `useful`
  - `👎 Hide` -> `hidden`
  - `🚫 Hide This` -> prompts for pattern + editable penalty, creates/updates negative rule, retroactively adjusts unread, hides card
  - `🌐 Hide Domain` -> extracts domain from article URL, prompts editable penalty, creates/updates negative rule, retroactively adjusts unread, hides card
- Positive actions (`👍 Useful`, click/read) keep the card visible in the current batch until reload/next batch
- `👍 Useful` closes the opened per-card menu automatically after action
- Negative actions remove card(s) immediately from the current view
- `Load Next` marks current batch as `seen`, loads next top unread batch, and scrolls to top
- If `Load Next` finds zero cards, feed triggers manual ingest refresh automatically (subject to scheduler cooldown/running guards)
- User session uses long-lived cookie/session refresh behavior to reduce surprise sign-outs during normal use
- `Load Next` is disabled while its request/refresh is active. Refresh ingestion returns immediately and is polled; network errors never masquerade as empty results or trigger refresh. Cooldown messages remain visible.

## Admin UI

- Open `/admin` and sign in using the Admin Secret field
- Admin routes can be CIDR-restricted by config
- Manage topics (query, weight, enabled)
- Manage negative rules (pattern, penalty, enabled)
- `Edit` retains the existing row ID, so correcting text updates rather than inserts. `Clear / New` leaves edit mode. Weight `0` is valid; negative-rule penalties must be positive.
- Run ingestion manually from UI
- Run retroactive title dedupe manually from UI (`Run Retroactive Dedupe`)
  - across all current `unread` items:
    - if the same key has `seen`, `read`, `useful`, manually hidden or legacy-hidden history, unread matches are hidden
    - otherwise highest-score unread is kept and remaining unread duplicates are hidden
- Automatically duplicate-hidden and score-hidden copies do not count as handled history, preventing successive dedupe runs from hiding the winner.
- Article Status Counts includes `dedupe_hidden_total` as cumulative all-time hidden-by-dedupe count
- Ingestion status panel includes build metadata (`version`, `commit`, `built_at`) for quick runtime verification after updates
- Ingestion status panel now shows the last two progress messages (`last_messages`) plus `last_message_at`
- Admin session now uses sliding refresh behavior and tolerates client IP drift (similar to feed session) to reduce surprise sign-outs

## Sessions

Admin idle lifetime is 24 hours; feed idle lifetime is 90 days. Valid API activity
renews server state and browser cookie expiry. Cookies are HttpOnly, SameSite
Strict and Secure when TLS is enabled. Credentials/tokens are not stored in
browser local storage. Server restarts invalidate in-memory sessions, so signing
in again after deployment is expected. Failed logout requests show a retry message
rather than falsely claiming the server session was removed.

## Ingestion Behavior

- Scheduling modes:
  - interval mode via `ingest_interval_minutes` (default every 2 hours)
  - daily mode via `daily_ingest_time` when interval mode is disabled (`ingest_interval_minutes=0`)
- Queries are run sequentially with configurable delay+jitter
- Query scope uses both `time_range=day` and `time_range=week`
- Ingest explicitly pulls both `categories=news` and `categories=general`
- Each query pulls pages 1 and 2: eight requests per instance/topic. There is no supported SearXNG `count` override; result volume, paging and time-filter support depend on enabled engines.
- If one SearXNG instance fails, the next is tried
- SearXNG redirects are rejected, including same-origin redirects. Configure the
  actual JSON-serving base URL, not a redirecting alias.
- An empty result array is success, not an unavailable instance. Partial results are retained, while HTTP, JSON, engine, persistence, thumbnail and maintenance failures produce bounded diagnostics and a final warning/error state. Recovered failover attempts remain visible as warnings.
- Failure `instance` numbers refer to the one-based position in `searxng_instances`; `topic_id` maps to Admin topics. Raw upstream response bodies and URLs are not included in failure summaries.
- Dedup is two-pass:
  - URL-based hash dedupe at ingest strips only recognized trackers/fragments, preserving identity query parameters such as `?id=123` and raw query ordering/encoding
  - ingest-time title dedupe for newly ingested unread:
    - title normalization uses lowercase alphanumeric-only key
    - first `dedupe_title_key_chars` (default `50`) are used as key prefix
    - within same ingest run, highest-score item is kept, others are hidden
    - handled history suppresses new unread copies; automatic duplicate/score hides do not
  - the same normalized key groups feed cards; this is not fuzzy/semantic matching
- Thumbnail handling in feed:
  - Startpage proxy thumbnails are normalized to direct `piurl` targets when valid
  - Brave proxy thumbnails are normalized by decoding their base64 payload when valid
  - inline `data:image/...` thumbnails are kept as-is
  - if an image fails to load in browser, card falls back to no-image rendering automatically
- High-score thumbnail enrichment:
  - during ingest, unread rows with empty thumbnails can be enriched from article metadata (`og:image`, `twitter:image`, `link rel=image_src`)
  - controlled by `thumbnail_refresh_min_score` and `thumbnail_refresh_max_per_run`
  - public destinations only; private/LAN/loopback/link-local addresses, unsafe redirects and environment proxy bypasses are rejected
  - metadata is parsed as HTML, including entities and relative URLs; stored original search thumbnail URLs remain unchanged by display decoding

## Query And Rule Tips

- Topic query can be plain words: `first person shooter`
- `intel+gpu` and `intel gpu` are normalized to the same search text. This convenience also means `+` is not treated as a literal character.
- Domain-focused topic query: `site:wccftech.com gpu`
- Negative rule matching is token-based (no regex):
  - `get+off` is the same as `get off`
  - `get off` is the same as `off get` (token order does not matter)
  - each token is a case-insensitive substring, not a whole-word or regex match; all tokens must exist somewhere in title/content/domain/url
- Domain block rule example: `theinformation.com`
- Negative rules apply immediately and retroactively to current `unread` entries
- Updating an existing rule penalty re-applies by delta to unread entries (for example changing `1` -> `100` applies an extra `99`)
- Disabling, renaming or deleting a rule recomputes its current effect on unread entries and known duplicate alternatives transactionally. Read/seen/useful and manual/legacy/score-hidden history is not rescored or unhidden. A duplicate alternative can become the representative when it has the highest current score.
- `applied_count` records distinct article matches per rule going forward, not each re-ingestion. Legacy counts are retained as a historical baseline.

## Scoring And Migration

New articles start with a zero baseline. For each associated enabled topic, the
score uses its current weight plus the strongest observed relevance for that
topic: `1 + max(engine_count, 1)*0.25 + searx_score*0.25 + term boosts`.
Term boosts are `0.35` for each query term found in the title and `0.1` in content
(terms shorter than three characters are ignored). Repeated identical evidence
does not accumulate. A newly matching topic or stronger evidence can increase
the score. Current enabled negative rules subtract their penalty once. Useful
adds one point once; reading preserves Useful status.

Existing scores are preserved at migration, with a derived baseline that allows
future topic/rule edits without resetting feed order. Legacy scores can therefore
remain much higher than newly ingested scores. Existing high feed/enrichment
thresholds may need operator adjustment after observing the new distribution;
the app never changes your config for you.

`auto_hide_below_score` applies to all unread entries at the end of ingestion.
`feed_min_score` is checked on every feed selection. Retention deletes old rows
at/below `cull_max_score` only when unread or automatically score-hidden;
handled history and duplicate history are retained. Dedupe counts survive restarts
and increment only once per newly duplicate-marked article, in the same transaction.

The upgrade reindexes stored original URLs and title keys, preserving all rows.
Already lost URLs cannot be recovered. Old hidden rows become `legacy` hides
because the previous schema did not record why they were hidden; they are not
automatically resurrected. Changing `dedupe_title_key_chars` on restart reindexes
groups and reconsiders only known duplicate hides, not deliberate/legacy hides.
Re-ingested headline changes also reconcile story membership. Handled articles retain their
original title/story identity so a publisher's later headline edit cannot revive
the already handled story. Tied scores within
a story prefer the oldest article ID consistently; feed-wide sorting still uses
score followed by publication/ingestion date. Previously counted duplicates are
not counted again if the representative changes.

Known publication dates survive empty later results and are emitted as RFC3339.
Legacy Go-formatted SQLite timestamps are converted. Old publication timestamps
exactly equal to ingestion timestamps are treated as the former unknown-date
fallback; other historical fallback values cannot be identified with certainty.
No date is invented when the upstream does not supply one. Time-filter parameters
are always sent, but engines can still return stale or undated stories.
