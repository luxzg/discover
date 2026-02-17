# TODO

## Automatic Topic Suggestions From Reading History

Build an admin-side helper that analyzes articles marked as `read` or `useful`, extracts frequent meaningful keywords/phrases, removes terms already covered by existing topics, and proposes a ranked list of candidate topics for one-click prefill into the Topic editor (with editable weight and enabled state before save).

## Advanced Subject Similarity Dedupe

Add an optional high-similarity filter for feed output that suppresses near-duplicate headlines when subject overlap is above a configurable threshold (for example 90% token overlap), to reduce repeated rewrites of the same story across syndication-heavy sources.
