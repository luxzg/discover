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
