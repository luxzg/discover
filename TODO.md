# TODO

## Thumbnail Backfill On URL Dedup Hits

When an ingest hit resolves to an existing article (URL dedupe), if the existing DB row has no thumbnail but the new hit contains one, backfill and store the new thumbnail URL.

## Thumbnail-Focused Fallback Fetch (Optional)

Investigate an optional ingest/fetch path for rows missing thumbnails, to request/search for sources that are more likely to return thumbnail metadata.

## Broken Thumbnail Rendering Diagnostics

Investigate why some rows have thumbnail URLs but browser shows only a broken-image placeholder; collect examples and identify root causes (bad URLs, blocked hosts, hotlink restrictions, format issues, etc.).

## Feed Action UX Consistency

Keep positively acted cards (`upvote`, `read`) visible in the current active batch until next reload/batch refresh.  
Negative actions (`downvote`, `hide this`, `hide domain`) should continue removing affected cards immediately.

## Session Stability / Idle Timeout

Investigate unexpected user/admin sign-outs during active local-WiFi usage (buttons becoming unresponsive followed by logged-out state).  
Target very long idle timeout/session lifetime with stable session-refresh behavior.

## Offline / Deferred Action Queue (Optional, Later)

Explore partial offline mode for feed interactions:
- allow queued local actions while server is temporarily unreachable
- sync queued actions to server later when connection is restored
- minimum scope: `read`, `upvote`, `downvote`

## Automatic Topic Suggestions From Reading History

Build an admin-side helper that analyzes articles marked as `read` or `useful`, extracts frequent meaningful keywords/phrases, removes terms already covered by existing topics, and proposes a ranked list of candidate topics for one-click prefill into the Topic editor (with editable weight and enabled state before save).

## Advanced Subject Similarity Dedupe

Add an optional high-similarity filter for feed output that suppresses near-duplicate headlines when subject overlap is above a configurable threshold (for example 90% token overlap), to reduce repeated rewrites of the same story across syndication-heavy sources.
