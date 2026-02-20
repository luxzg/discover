# Usage

## Feed UI

- Open `/` in browser
- Sign in with `user_name` and `user_secret`
- Feed shows top unread cards sorted by score/date
- Tap card to open article (marks it as `read`)
- Card menu actions:
  - `👍 Useful` -> `useful`
  - `👎 Hide` -> `hidden`
  - `🚫 Hide This` -> prompts for pattern + editable penalty, creates/updates negative rule, retroactively adjusts unread, hides card
  - `🌐 Hide Domain` -> extracts domain from article URL, prompts editable penalty, creates/updates negative rule, retroactively adjusts unread, hides card
- `Load Next` marks current batch as `seen`, loads next top unread batch, and scrolls to top
- If `Load Next` finds zero cards, feed triggers manual ingest refresh automatically (subject to scheduler cooldown/running guards)

## Admin UI

- Open `/admin` and sign in using the Admin Secret field
- Admin routes can be CIDR-restricted by config
- Manage topics (query, weight, enabled)
- Manage negative rules (pattern, penalty, enabled)
- Run ingestion manually from UI

## Ingestion Behavior

- Scheduling modes:
  - interval mode via `ingest_interval_minutes` (default every 2 hours)
  - daily mode via `daily_ingest_time` when interval mode is disabled (`ingest_interval_minutes=0`)
- Queries are run sequentially with configurable delay+jitter
- Query scope uses both `time_range=day` and `time_range=week`
- Ingest pulls both `categories=news` and general search (no category)
- Each query pulls page 1 and page 2 with larger result count per request
- If one SearXNG instance fails, the next is tried
- Dedup is two-pass:
  - URL-based hash dedupe at ingest
  - subject/title dedupe at feed selection time

## Query And Rule Tips

- Topic query can be plain words: `first person shooter`
- Domain-focused topic query: `site:wccftech.com gpu`
- Negative rule matching is token-based (no regex):
  - `get+off` is the same as `get off`
  - `get off` is the same as `off get` (token order does not matter)
  - match succeeds when all tokens exist anywhere in title/content/domain/url
- Domain block rule example: `theinformation.com`
- Negative rules apply immediately and retroactively to current `unread` entries
- Updating an existing rule penalty re-applies by delta to unread entries (for example changing `1` -> `100` applies an extra `99`)
