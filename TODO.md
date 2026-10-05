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

## Repeatable SearXNG Upgrade

The operator confirmed the existing private installation is under
`/usr/local/searxng`. Inventory is complete: see SEARXNG.md for the February 15
source revision, Python 3.12.12, dedicated service and confirmed source/venv paths.
Review the exact launch contract and current upstream compatibility, then build
a reviewed server-side update helper
for this actual layout, with config preservation, preflight/dry-run, retained
old source/environment, bounded JSON search checks and documented rollback.
Do not apply generic upstream install-script upgrades blindly to the custom
pyenv layout, execute service-owned scripts as root or expose the listener.
Review a production application server separately from engine/package upgrades.

## RSS / Atom Sources

Explore direct RSS/Atom ingestion for regularly followed sites, alongside SearXNG.
Reuse URL/story identity, scoring and rule handling. Compare date and thumbnail
quality, and use bounded conditional polling rather than repeatedly downloading feeds.
