# SQLite Debug Guide

## Install sqlite3

```bash
sudo apt update
sudo apt install -y sqlite3
```

## Open Discover Database

```bash
sqlite3 -readonly /home/discover/apps/discover/discover.db
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
- `article_evidence`: strongest relevance per `(article_id, topic_id)`; repeated hits do not accumulate scores.
- `article_rule_effects`: current `penalty` and once-per-article `counted` marker per rule.
- Additional `articles` columns: `story_key`, `hidden_reason`, `duplicate_of`, `dedupe_counted`, `score_base`, `vote`, `read_at`.

`hidden_reason` is `manual`, `score`, `duplicate`, or `legacy` for hidden rows.
`duplicate_of` refers to the chosen representative/handled article. Existing
hidden rows with unknown provenance are `legacy` and are preserved. Publication
may be NULL; ingestion is not publication. New timestamp values use UTC RFC3339.
The application enables foreign keys on its connections; do not manually edit
this database without a backup and understanding the derived scoring tables.

Inspect schema directly:

```sql
.tables
.schema articles
.schema topics
.schema negative_rules
.schema article_topics
.schema app_settings
.schema article_evidence
.schema article_rule_effects
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

Top scored unread rows (not the complete feed query: the UI also applies your
configured minimum score, selects one representative per story and excludes
handled stories):

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

## Story And Scoring Diagnostics

Inspect all copies of a headline and why they were hidden:

```sql
SELECT id, status, hidden_reason, duplicate_of, dedupe_counted,
       round(score, 2) AS score, story_key, title, url
FROM articles
WHERE lower(title) LIKE '%amd gpu prices fall%'
ORDER BY score DESC, id;
```

Permanent dedupe counter versus currently hidden duplicates (these can differ
after representative changes or explicit actions):

```sql
SELECT value AS dedupe_hidden_total
FROM app_settings WHERE key='dedupe_hidden_total';
SELECT hidden_reason, COUNT(*) AS rows
FROM articles WHERE status='hidden' GROUP BY hidden_reason;
```

Inspect evidence and current negative effects for one article, replacing `123`:

```sql
SELECT a.id, a.score, a.score_base, t.query, t.weight, t.enabled, e.relevance
FROM articles a
JOIN article_evidence e ON e.article_id=a.id
JOIN topics t ON t.id=e.topic_id
WHERE a.id=123;

SELECT r.pattern, r.enabled, e.penalty, e.counted
FROM article_rule_effects e
JOIN negative_rules r ON r.id=e.rule_id
WHERE e.article_id=123;
```

Check actual stored publication-date availability:

```sql
SELECT status, COUNT(*) AS total,
       SUM(published_at IS NOT NULL AND published_at <> '') AS dated
FROM articles GROUP BY status;
```

## Exit sqlite

```sql
.quit
```
