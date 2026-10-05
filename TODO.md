# TODO

## Offline / Deferred Action Queue (Optional, Later)

Explore partial offline mode for feed interactions:
- allow queued local actions while server is temporarily unreachable
- sync queued actions to server later when connection is restored
- minimum scope: `read`, `upvote`, `downvote`

## Automatic Topic Suggestions From Reading History

Build an admin-side helper that analyzes articles marked as `read` or `useful`, extracts frequent meaningful keywords/phrases, removes terms already covered by existing topics, and proposes a ranked list of candidate topics for one-click prefill into the Topic editor (with editable weight and enabled state before save).

## Advanced Subject Similarity Dedupe

Extend the conservative normalized-title story groups with optional fuzzy matching
(for example 90% token overlap). Evaluate false merges on real headlines before
enabling it; preserve separate sources and handled history rather than deleting URLs.
Keep exact/conservative same-story grouping as a distinct layer. Consider softer
matching during the first configurable 24-36 hours, stronger consolidation later,
using the immutable original story clock, entities/model numbers and a bounded
time window. Do not merge release announcements with reviews, benchmarks or new
findings solely because they share a product name. Avoid transitive-chain merges;
make derived grouping reversible and expose alternative sources.

## Coverage Fatigue / Enough Of This Story

Explore a separate temporary suppression action for repetitive event coverage
after the reader has opened one or several articles. Related coverage is not
necessarily duplicate coverage: preserve genuinely new benchmarks/prices/issues
and avoid broad product/domain bans. Prototype and evaluate separately from fuzzy
headline dedupe; no automatic thematic suppression is enabled yet.

## Bounded Domain Preferences

Extend Admin's Reading By Domain report (v2.27) before training ranking. Add
recent versus retained-lifetime summaries, reliable impression/event provenance,
story-aware counts and minimum sample sizes. Click/read is a positive interest
hint; later deliberate hide wins; automatic filtering is never a dislike.
Use small capped boosts and leave exploration room for unfamiliar sources.
Make reasons inspectable and preferences resettable. Automatic domain-focused
search expansion is a later opt-in stage, not inferred from raw click totals.

## Adaptive Search Backoff And Clearer Ingestion Outcomes

v2.29 paces individual searches rather than just topics; v2.31 adds an explicit
Admin sample diagnostic. For actual ingestion runs, next distinguish clean
empty results, partial success with engine warnings, searches without any usable
engine response, and storage/maintenance failures in summaries/Admin. Aggregate
safe diagnostic classes without exposing upstream bodies or private queries.
Consider bounded category/instance backoff when HTTP 200 carries engine suspension
or repeated all-empty failures, retaining partial results and letting healthy
categories continue. Avoid treating a legitimate empty topic as an outage or
clearing SearXNG suspensions to force retries. Adaptive pauses/harvest reduction
remain future work; request pacing alone is not guaranteed to prevent blocking.

### Per-Engine Cooldowns And Selective Recovery Probes

Use classified ingestion warnings and recent Admin diagnostics to temporarily
omit blocked engines from subsequent requests, scoped to instance/engine/category.
SearXNG already suspends engines internally; review those settings first rather
than duplicating or bypassing that protection. Prefer learning from normal
searches and reusing fresh observations over a mandatory extra check at every
ingest startup. If needed, make a bounded startup sample only when state is stale
and cooldowns permit it.

Evaluate category-aware explicit `engines` selection using the instance's actual
enabled-engine inventory. The current upstream parser accepts a comma-separated
allowlist, but combining it with `categories` adds category engines back; an
empty/invalid list can fall back to default engines. Verify the deployed parser
and locked preferences before relying on selection. Skip an exhausted category
instead of issuing an empty list that might retry all providers. Keep day/week
and paging constraints, unknown engines eligible, and partial results usable.

Persist bounded cooldown/recovery state so Discover restarts do not cause retry
bursts. Treat CAPTCHA/access denial/rate limits more conservatively than transient
timeouts; a single empty result is not a failed engine. After cooldown, permit
only sparse paced probes before gradual reintroduction, with longer backoff on
repeated failures. Recovery cannot be known without occasional probes, and some
CAPTCHAs require operator intervention. Never clear upstream suspensions or change
SearXNG settings automatically. Show skipped engines, reason and next probe time
in Admin; add synthetic selection, restart, all-blocked and recovery regressions
before opt-in field testing. This remains an idea, not active v2.31 behavior.

References checked 2026-10-05: [upstream request selection](https://github.com/searxng/searxng/blob/master/searx/webadapter.py)
and [engine suspension settings](https://docs.searxng.org/admin/settings/settings_search.html).

## SearXNG Production Server And Settings Review

The installation-specific updater is implemented in v2.29; see SEARXNG.md for
operator preflight, update and rollback. Review converting the historical
full settings copy to minimal current overrides without losing secrets, JSON or
loopback settings. The redacted-file comparison and proposed overrides are now
documented in SEARXNG.md (October 5), preserving longer cooldowns and the operator's
intentional Yahoo general/News enables. Applying inheritance, verifying loaded
engine metadata and observing scheduled results remain operator follow-up.
The operator completed the retained-path source/venv upgrade
and local health check on October 5; full upstream recovery remains unverified.
A subsequent v2.31 Admin sample returned usable news/general results from two
engines, while other engines still reported CAPTCHA, access denial, rate limiting
or HTTP errors. This is partial observed availability, not full recovery; the first
scheduled paced ingestion after deployment remains unverified.
A production application-server migration (for example Granian)
is separate from engine/package upgrades; the helper intentionally retains the
existing `python -m searx.webapp` unit. Do not expand its supported launch contract
or remove preflight checks merely to get an update to run.

## Opt-In News-Engine Harvest With Verified Freshness

Explore separate, capped Google News and Yahoo News ingestion paths. The operator's
October 5 inventory confirms Google News lacks time filtering; Yahoo News was
also deliberately enabled in the supplied settings but lacks time filtering.
Current mandatory day/week searches skip them. Enabling the engines is
not enough; do not silently remove filters from the existing harvest or pretend
the news category guarantees recency. Assess actual publication-date coverage,
enrich within existing secure metadata caps, and define an explicit policy for
undated results before integration. First discovery is not proof of publication.
Preserve story/URL dedupe, pacing and engine cooldowns; test separately with an
operator-enabled option and retain the current time-limited path as the default.
This is a future idea only; no unfiltered searches are enabled in v2.31.

Include Bing News in the comparison, but it already supplies the existing
time-filtered news harvest and does not need an unfiltered workaround. Review
its adapter's day-to-last-hour mapping and repeated-last-page behavior before
tuning coverage; do not treat engine labels as proof of date accuracy. Any
engine-specific requests must respect explicit-selection/category semantics and
avoid duplicating existing traffic. References: [Bing News adapter](https://docs.searxng.org/dev/engines/online/bing.html#bing-news)
and [installed settings defaults](https://github.com/searxng/searxng/blob/d48c4b5/searx/settings.yml).

## RSS / Atom Sources

Explore direct RSS/Atom ingestion for regularly followed sites, alongside SearXNG.
Reuse URL/story identity, scoring and rule handling. Compare date and thumbnail
quality, and use bounded conditional polling rather than repeatedly downloading feeds.
