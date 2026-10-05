# Usage

## Feed UI

- Open `/` in browser
- Sign in with `user_name` and `user_secret`
- While signed in, feed header/auth row shows current app version next to `Sign Out`
- Feed shows eligible unread stories ordered by freshness-adjusted relevance (stored score is unchanged).
- One card represents a conservative normalized-title story; expand `Other sources` for up to 20 alternate URLs. Reading an alternate also handles that story.
- Card metadata is `domain | score | Published today` or `First seen 3 days ago` when publication is unknown. A database discovery date is never presented as publication.
- Tap card to open article (marks it as `read`)
- Card menu actions:
  - `👍 Useful` -> `useful`
  - `👎 Hide` -> `hidden`
  - `🚫 Hide This` -> prompts for pattern + editable penalty, creates/updates negative rule, retroactively adjusts unread, hides card
  - `🌐 Hide Domain` -> extracts domain from article URL, prompts editable penalty, creates/updates negative rule, retroactively adjusts unread, hides card
- Positive actions (`👍 Useful`, click/read) keep the card visible in the current batch until reload/next batch
- `👍 Useful` closes the opened per-card menu automatically after action
- Negative actions remove card(s) from the current view after the server confirms completion
- `Load Next` marks current batch as `seen`, loads next top unread batch, and scrolls to top
- If `Load Next` finds zero cards, feed triggers manual ingest refresh automatically (subject to scheduler cooldown/running guards)
- User session uses long-lived cookie/session refresh behavior to reduce surprise sign-outs during normal use
- `Load Next` is disabled while its request/refresh is active. Refresh ingestion returns immediately and is polled; network errors never masquerade as empty results or trigger refresh. Cooldown messages remain visible.

### Background Hide Actions

After confirming Hide This/Hide Domain, the menu closes and the card shows
submission progress, followed by an acknowledgement that the backend is applying
the hide. You can keep reading other cards or close the page: an accepted hide
does not depend on keeping the browser connection alive. The card and matching
visible cards disappear once completion is confirmed. `Load Next` waits while
the page is tracking this update so the current batch is not marked seen midway
through it. Only one rule-based hide runs at a time.

If the connection fails, the UI does not falsely claim the hide failed or
completed. Retrying the same action on that card reuses its request ID and checks
the accepted job. On reload/sign-in, the page checks for an active hide before
loading the feed. Recent completion results are cached for the last 32 jobs in
memory, not permanently; restarting the service cancels unfinished work and
rolls back its transaction. After a restart or an unavailable result, reload and
verify the rule/card state before retrying. This is not an offline action queue:
the backend must first receive and accept the request.

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

### Article Age And Archive

The default feed age limit is 30 days. In **Article Age**, enter whole days
(for example 90), click **Preview**, then **Apply Limit And Archive** and confirm.
The action saves that ongoing feed limit in SQLite and archives older unhandled
rows; it does not delete rows or modify reading/voting status. Increasing the
limit restores qualifying age-archived rows, not deliberately hidden articles.
`0` disables age filtering and restores age-archived rows. The saved Admin limit
takes precedence over `feed_max_age_days` in JSON and persists across restarts.

Every feed query enforces the active limit, even before running archive or ingest.
Ingest maintains archive marks after enrichment. Admin counts show archived rows
separately from unread/hidden counts. Preview counts both rows that would newly
archive and rows whose age archive would be restored at the proposed limit.
Counts include automatic duplicate/score-hidden alternatives, not distinct stories.
Read/seen/useful and deliberate/legacy hides are not archived by this action.

Age uses the earliest immutable first-seen timestamp across the conservative
story group, or an earlier known publication date. Neither a new syndicated
copy nor an updated publisher date resets the age clock. Existing rows recover
`first_seen_at` from `created_at`; only malformed creation dates fall back to
the timestamp still available in `ingested_at`. Already deleted history cannot
be reconstructed. Increasing the age limit does not guarantee previously
handled stories will reappear: existing handled-history rules still apply.
Restoring removes only the age archive. Minimum-score, duplicate and handled
filters still apply, and freshness-adjusted ordering can put a restored article
on a later page even when its displayed raw score is higher than the first card.
For example, with 7-day decay, a 60-day-old article scored 150 ranks around 15.7,
below a fresh article scored 85. Reload the feed to fetch the new eligible batch;
restoring does not change its scores, dates or deliberately recorded actions.

### Reading By Domain

Expand **Reading By Domain** and click **Generate / Refresh Report**. It queries
retained database history on demand, with up to 200 main domains ordered by
positive article count. Subdomain counts merge before filtering: for example
`www.pcmag.com` and `au.pcmag.com` become `pcmag.com`. Only merged domains with
at least two positive reads appear. Grouping uses the bundled
[public suffix list](https://pkg.go.dev/golang.org/x/net/publicsuffix), preserving
multi-label suffixes such as `co.uk` and separate hosted sites such as distinct
`blogspot.com` tenants. Stored sources and Hide Domain rules are not changed.
Read means opened; Useful and Read can overlap, but Positive
counts an article once. A current explicit hide wins over an earlier read/useful
hint. Automatic score/duplicate/age filtering is not a dislike. Unknown-origin
legacy hides are not counted as proven explicit downvotes.

Hidden counts deliberate hides, not automatic filters. Shown is recorded
exposure through read/useful or batch advancement with Load Next, not literal
scroll tracking or every rendered impression. The API's `seen` key is unchanged.
Total includes
all retained rows, including duplicates and archived entries. These are article
counts, not click-event counts or guaranteed complete yearly history. Legacy
read timestamps have limited provenance; no ranking/search preferences are
automatically changed by this report yet.

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
- Topic delay/jitter remains `per_query_delay_seconds`/`per_query_jitter_seconds`.
  Individual HTTP searches, including page/category/time-range changes and
  failover, additionally pause `search_request_delay_seconds` (default 5) plus
  random `0..search_request_jitter_seconds` (default 2). The first search of a run
  starts immediately; set both request keys to 0 only for intentional unpaced use.
  Missing keys inherit defaults without rewriting config. Publisher metadata
  requests use the existing separate caps/retry rules, not the search pause.
- Request pauses are logged separately; topic `took` excludes those pauses and
  `paused` reports them. Total run time includes all waiting. Manual and scheduled
  runs have a two-hour safety deadline, cancel promptly on shutdown, and cannot
  overlap. A 32-topic default run can take around half an hour; this is expected
  background pacing, not a stalled browser request.
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
- Date enrichment shares the secure publisher fetch with thumbnails. An additional
  `date_refresh_max_per_run` batch (default 40, 0 disables date-only candidates)
  targets missing publication dates even if images already exist. It reads
  Article/NewsArticle/BlogPosting JSON-LD `datePublished`, publication meta tags
  and explicitly marked datePublished time elements, never dateModified.
  Page-level publication tags take precedence; conflicting structured article
  dates are left unknown rather than guessed from another article in a graph.
  The union makes at most thumbnail cap + date cap requests; shared candidates
  are fetched once and either batch can fill both missing fields. Failed/empty
  metadata attempts wait at least a day before retry. Publication extraction
  can still fail or be inaccurate on misleading publisher markup; no full-text
  article content is stored by this enrichment.

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
- Rule edits evaluate only the changed rule against eligible entries; unrelated rules are not rematched. Topic edits reuse recorded rule effects.
- Disabling, renaming or deleting a rule recomputes its current effect on unread entries and known duplicate alternatives transactionally. Read/seen/useful and manual/legacy/score-hidden history is not rescored or unhidden. A duplicate alternative can become the representative when it has the highest current score.
- `applied_count` records distinct article matches per rule going forward, not each re-ingestion. Legacy counts are retained as a historical baseline.

## Scoring And Migration

New articles start with a zero baseline. For each associated enabled topic, the
score uses its current weight plus the strongest observed relevance for that
topic: `1 + max(engine_count, 1)*0.25 + searx_score*0.25 + term boosts`.
Term boosts are `0.35` for each query term found in the title and `0.1` in content
(terms shorter than three characters are ignored). Repeated identical evidence
does not accumulate. A newly matching topic or stronger evidence can increase
the score only within `score_evidence_window_hours` (default 36) of the earliest
first-seen occurrence of that story. Afterward associations, hit counts,
metadata and last-ingested diagnostics can still update, but additional search
evidence cannot add points. Topic/rule edits and explicit votes still work.
Current enabled negative rules subtract their penalty once. Useful
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
a story prefer the oldest article ID consistently; feed-wide sorting uses
`score / (1 + age_days / feed_freshness_decay_days)` for positive scores,
then effective date/ID. `0` decay days disables that adjustment. Minimum score
filtering and displayed score still use the stored score. Previously counted duplicates are
not counted again if the representative changes.

Known publication dates survive empty later results and are emitted as RFC3339.
Legacy Go-formatted SQLite timestamps are converted. Old publication timestamps
exactly equal to ingestion timestamps are treated as the former unknown-date
fallback; other historical fallback values cannot be identified with certainty.
No publication date is invented when neither search nor publisher metadata
supplies one; First seen remains only a discovery-date fallback. Time-filter
parameters are always sent, but engines can still return stale or undated stories.
