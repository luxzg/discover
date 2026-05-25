# Finished Tasks

Archive of completed (or intentionally closed) TODO items.  
`TODO.md` should contain only active open work.

## 2026-05-25 11:06 CEST

### Completed

- **Thumbnail Backfill On URL Dedup Hits**
  - Implemented in `v2.13`:
    - on URL-dedupe ingest hits, thumbnail is backfilled only when existing row has no thumbnail and new hit has one
    - existing non-empty thumbnails are preserved (not overwritten)

- **Feed Action UX Consistency**
  - Implemented in `v2.13`:
    - positive actions (`upvote`, click/read) no longer remove cards immediately from current batch view
    - negative actions continue removing cards immediately (`downvote`, `hide this`, `hide domain`)

- **Session Stability / Idle Timeout**
  - Implemented in `v2.13` (user side):
    - user session TTL extended to 90 days
    - sliding session refresh on valid activity/session checks
    - user session validation no longer hard-fails on remote IP drift
    - `/api/session` refreshes cookie expiry
  - Follow-up kept active in `TODO.md`:
    - admin-session stability verification/tuning under long-running mixed-network use

## 2026-05-25 17:18 CEST

### Completed

- **Thumbnail-Focused Fallback Fetch (Optional)**
  - Implemented in `v2.14` via feed-display normalization without DB migration:
    - Startpage proxy thumbnails (`/av/proxy-image`) now decode embedded `piurl` and use direct URL when valid
    - Brave proxy thumbnails (`imgs.search.brave.com/...`) now decode base64 payload to direct URL when valid
    - `data:image/...` inline thumbnails are preserved as-is
  - Original stored thumbnail values remain unchanged in DB; normalization is applied when serving feed items.

- **Broken Thumbnail Rendering Diagnostics**
  - Sample analysis confirmed expiring/broken proxy URLs as a practical cause.
  - Implemented frontend fallback (`img onerror`) to remove failed images from card rendering instead of showing broken placeholders.
