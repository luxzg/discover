# TODO

## Thumbnail Refresh For Empty Rows (Optional)

Add an optional background/manual refresh pass for rows with empty thumbnails:
- attempt to refresh missing thumbnails from new ingest hits
- optionally run targeted lookup for known proxy patterns (Startpage/Bing/Brave-derived image URLs)
- keep this disabled by default unless explicitly enabled/configured

## Admin Session Stability Follow-Up

User session stability has been improved, but admin session behavior under long-running mixed-network use may still need dedicated verification/tuning.

## Offline / Deferred Action Queue (Optional, Later)

Explore partial offline mode for feed interactions:
- allow queued local actions while server is temporarily unreachable
- sync queued actions to server later when connection is restored
- minimum scope: `read`, `upvote`, `downvote`

## Automatic Topic Suggestions From Reading History

Build an admin-side helper that analyzes articles marked as `read` or `useful`, extracts frequent meaningful keywords/phrases, removes terms already covered by existing topics, and proposes a ranked list of candidate topics for one-click prefill into the Topic editor (with editable weight and enabled state before save).

## Advanced Subject Similarity Dedupe

Add an optional high-similarity filter for feed output that suppresses near-duplicate headlines when subject overlap is above a configurable threshold (for example 90% token overlap), to reduce repeated rewrites of the same story across syndication-heavy sources.
