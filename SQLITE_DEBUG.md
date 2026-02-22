# SQLite Debug Guide

## Install sqlite3

```bash
sudo apt update
sudo apt install -y sqlite3
```

## Open Discover Database

```bash
sqlite3 /home/discover/apps/discover/discover.db
```

Optional output formatting inside sqlite shell:

```sql
.headers on
.mode column
.width 70 8 10 22 90
```

## Database Schema (Quick Map)

Main tables used by Discover:

- `articles`
  - Core feed data
  - Key columns: `id`, `title`, `url`, `normalized_url`, `url_hash`, `score`, `status`, `published_at`, `ingested_at`, `source_domain`
- `topics`
  - Positive query topics
  - Key columns: `id`, `query`, `weight`, `enabled`
- `negative_rules`
  - Negative match rules / penalties
  - Key columns: `id`, `pattern`, `penalty`, `enabled`, `applied_count`
- `article_topics`
  - Join table linking articles to topics
  - Key columns: `article_id`, `topic_id`
- `app_settings`
  - Generic key/value app state
  - Key columns: `key`, `value`

Inspect schema directly:

```sql
.tables
.schema articles
.schema topics
.schema negative_rules
.schema article_topics
.schema app_settings
```

## Common Debug Queries

Find by title fragment:

```sql
SELECT title, score, url
FROM articles
WHERE title LIKE '%AMD GPU Prices Fall%'
ORDER BY score DESC, id DESC
LIMIT 100;
```

Case-insensitive title search:

```sql
SELECT title, score, url
FROM articles
WHERE lower(title) LIKE lower('%amd gpu prices fall%')
ORDER BY score DESC, id DESC
LIMIT 100;
```

Include status and dates:

```sql
SELECT title, score, status, published_at, ingested_at, url
FROM articles
WHERE title LIKE '%AMD GPU Prices Fall%'
ORDER BY score DESC, id DESC
LIMIT 100;
```

Find by domain:

```sql
SELECT title, score, status, source_domain, url
FROM articles
WHERE source_domain LIKE '%msn.com%'
ORDER BY score DESC, id DESC
LIMIT 100;
```

Top scored unread:

```sql
SELECT id, title, score, source_domain, published_at
FROM articles
WHERE status='unread'
ORDER BY score DESC, COALESCE(published_at, ingested_at) DESC
LIMIT 50;
```

Topic list:

```sql
SELECT id, query, weight, enabled
FROM topics
ORDER BY id;
```

Negative rules list:

```sql
SELECT id, pattern, penalty, enabled, applied_count
FROM negative_rules
ORDER BY id;
```

Article status counts:

```sql
SELECT status, COUNT(*) AS count
FROM articles
GROUP BY status
ORDER BY count DESC;
```

## Exit sqlite

```sql
.quit
```
