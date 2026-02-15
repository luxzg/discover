# Usage

## Feed UI

- Open `/` in browser
- Feed shows top unread cards sorted by score/date
- Tap card to open article (marks it as `read`)
- Card menu actions:
  - `👍 Useful` -> `useful`
  - `👎 Hide` -> `hidden`
  - `🚫 Don't show` -> creates negative rule, retroactively penalizes unread, hides card
- `Load Next` marks current batch as `seen` and loads next top unread batch

## Admin UI

- Open `/admin` and sign in using the Admin Secret field
- Admin routes can be CIDR-restricted by config
- Manage topics (query, weight, enabled)
- Manage negative rules (pattern, penalty, enabled)
- Run ingestion manually from UI

## Ingestion Behavior

- Scheduled once daily at fixed local wall-clock `daily_ingest_time`
- Queries are run sequentially with configurable delay+jitter
- If one SearXNG instance fails, the next is tried
- Dedup is by hash(normalized URL)
