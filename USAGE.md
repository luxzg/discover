# Usage

## Feed UI

- Open `/` in browser
- Sign in with `user_name` and `user_secret`
- Feed shows top unread cards sorted by score/date
- Tap card to open article (marks it as `read`)
- Card menu actions:
  - `👍 Useful` -> `useful`
  - `👎 Hide` -> `hidden`
  - `🚫 Don't show` -> creates negative rule, retroactively penalizes unread, hides card
- `Load Next` marks current batch as `seen`, loads next top unread batch, and scrolls to top

## Admin UI

- Open `/admin` and sign in using the Admin Secret field
- Admin routes can be CIDR-restricted by config
- Manage topics (query, weight, enabled)
- Manage negative rules (pattern, penalty, enabled)
- Run ingestion manually from UI

## Ingestion Behavior

- Scheduled once daily at fixed local wall-clock `daily_ingest_time`
- Queries are run sequentially with configurable delay+jitter
- Query scope uses `time_range=week`
- Ingest tries `categories=news` first, then falls back to general search when needed
- If one SearXNG instance fails, the next is tried
- Dedup is by hash(normalized URL)

## Query And Rule Tips

- Topic query can be plain words: `first person shooter`
- Domain-focused topic query: `site:wccftech.com gpu`
- Negative rule matching is token-based (no regex): `get off` matches text containing both words, even with words between
- Domain block rule example: `theinformation.com`
- Negative rules apply immediately and retroactively to current `unread` entries
