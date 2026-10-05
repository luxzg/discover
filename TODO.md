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

v2.29 paces individual searches rather than just topics. Next, distinguish clean
empty results, partial success with engine warnings, searches without any usable
engine response, and storage/maintenance failures in summaries/Admin. Aggregate
safe diagnostic classes without exposing upstream bodies or private queries.
Consider bounded category/instance backoff when HTTP 200 carries engine suspension
or repeated all-empty failures, retaining partial results and letting healthy
categories continue. Avoid treating a legitimate empty topic as an outage or
clearing SearXNG suspensions to force retries. Adaptive pauses/harvest reduction
remain future work; request pacing alone is not guaranteed to prevent blocking.

## SearXNG Production Server And Settings Review

The installation-specific updater is implemented in v2.29; see SEARXNG.md for
operator preflight, update and rollback. Confirm the real server launch contract
and candidate compatibility in that preflight. Review converting the historical
full settings copy to minimal current overrides without losing secrets, JSON or
loopback settings. A production application-server migration (for example Granian)
is separate from engine/package upgrades; the helper intentionally retains the
existing `python -m searx.webapp` unit. Do not expand its supported launch contract
or remove preflight checks merely to get an update to run.

## RSS / Atom Sources

Explore direct RSS/Atom ingestion for regularly followed sites, alongside SearXNG.
Reuse URL/story identity, scoring and rule handling. Compare date and thumbnail
quality, and use bounded conditional polling rather than repeatedly downloading feeds.
