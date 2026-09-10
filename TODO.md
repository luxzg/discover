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

## RSS / Atom Sources

Explore direct RSS/Atom ingestion for regularly followed sites, alongside SearXNG.
Reuse URL/story identity, scoring and rule handling. Compare date and thumbnail
quality, and use bounded conditional polling rather than repeatedly downloading feeds.
