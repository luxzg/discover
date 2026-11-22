# SQLite Debug Guide

## Find possible missed groups in your reading history

On the server, as the Discover service user, run from the updated checkout:

```bash
python3 scripts/review-coverage.py --database discover.db
```

No rebuild, service stop, config access, downloads or Admin sign-in needed.
This Python 3 standard-library helper opens the existing SQLite database
read-only and includes committed WAL data. It prints private headlines and
article IDs to your terminal only. Do not commit the output.

The default is the last **seven rolling days of activity**, not publication or
ingestion. Rows qualify by first recorded open (`read_at`) **or** latest recorded
exposure (`last_seen_at`), not current status. It includes cards passed with
**Load Next** even if neither was clicked. Upvotes also record exposure.
These timestamps cannot reconstruct literal scrolling, every repeated click,
or an unadvanced page.

The separate **shown/shown**, **opened/shown**, and **opened/opened** shortlists
compare headline words, phrases and model numbers more loosely than the live
matcher. A shared topic does **not** prove the same event. Pairs already sharing
a current exact key or event assignment are omitted. Nothing is regrouped.
Identify both real repeats and similar-looking headlines that should stay separate.

Add `--strict` to preview a second, headline-only check: it asks whether the
articles report the same development, not just whether the subject overlaps.
Opens determine report sections, **not** whether a pair matches. For a fresh
test after labeling the default (up to five per section) broad shortlist:

```bash
python3 scripts/review-coverage.py --database discover.db --until <original-UTC-end> --holdout <original-shortlist-ID>
```

Replace both placeholders and reuse the original `--days` if not seven. The
helper recomputes and verifies the broad shortlist, then excludes **both
articles** from every reviewed pair before selecting new strict matches. A
changed shortlist stops the check rather than silently reusing examples.
These are ranked candidates, not a random sample or an accuracy estimate.
For private calibration without opening SQLite, `--replay-labels` reads tab-separated
`same`/`different`, title, title from standard input (optionally ID before each
title). End with EOF or a blank line. No titles are printed by replay or stored.

Options: `--days 1..31`, `--pairs 1..20` per shortlist, and `--until` with
the printed UTC end time to reuse a window. This is not a persistent snapshot:
later edits/exposures can change results. At most the 3,000 most recent qualifying
rows are analyzed; the report says if truncated. Use smaller windows if needed.
If obvious repeats don't make the shortlist, `--list-headlines` also prints
all selected titles (up to 480 characters each), without requiring a lexical match.
It can produce long, sensitive output; leave it off initially.

## Grouped Feed State

In v2.49, nullable `articles.event_id` links an article to a derived
`coverage_events` anchor. Its `advanced` flag affects only Grouped pagination,
not article status, votes, dedupe totals or Standard eligibility. A null ID
means no confident assignment; it does not indicate an ingestion error.

## Engine Versus CLI Version

Discover v2.26 embeds SQLite 3.53.4 through `modernc.org/sqlite` v1.58.0.
It does not use the system `sqlite3` executable for normal database access.
`sqlite3 --version` (or `SELECT sqlite_version()` inside that CLI) reports the
CLI's engine version, not Discover's. To see which driver the binary was built
with, run `go version -m ./discover` in its application directory.

This dependency upgrade reuses the existing database file and schema; there is
no manual conversion or re-ingestion step. Use the normal deployment workflow
in `INSTALL.md`, including its existing backup step.

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
  - Records which searches found an article and supports Admin's per-topic
    unread/total counts. One URL can retain several topic associations without
    duplicate article rows; relevance/penalties live in the separate ledgers below.
- `app_settings`
  - Generic key/value app state
  - Key columns: `key`, `value`
- `article_evidence`: strongest relevance per `(article_id, topic_id)`; repeated hits do not accumulate scores.
- `article_rule_effects`: current `penalty` and once-per-article `counted` marker per rule.
- Additional `articles` columns: `story_key`, `hidden_reason`, `duplicate_of`, `dedupe_counted`, `score_base`, `vote`, `read_at`.
- `last_seen_at` records batch/positive-action exposure, not discovery. A later
  hide preserves it and `read_at`; Admin's Shown report uses either as historical
  evidence. Missing legacy exposure is not invented. Use `first_seen_at` for age.
- v2.27 adds `first_seen_at` (immutable original discovery), `archived_at`
  (reversible age archive, separate from status) and `metadata_checked_at`
  (shared date/image enrichment retry clock). `created_at` remains row creation;
  `ingested_at` is the latest search rediscovery/run marker, not first discovery.
- `app_settings.feed_max_age_days` is the Admin override for the JSON age limit.

`hidden_reason` is `manual`, `score`, `duplicate`, or `legacy` for hidden rows.
`duplicate_of` refers to the chosen representative/handled article. Existing
hidden rows with unknown provenance are `legacy` and are preserved. Publication
may be NULL; ingestion is not publication. Search/publication/first-seen writes
use UTC RFC3339. SQLite-generated action/archive/audit timestamps use UTC
`YYYY-MM-DD HH:MM:SS`; readers support both storage forms. Public publication
dates are emitted as RFC3339, not raw SQLite timestamp strings.
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
SELECT title, score, status, published_at, first_seen_at, ingested_at, archived_at, url
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
configured minimum score/age, excludes archived/handled stories, selects one
representative and adjusts ordering for freshness):

```sql
SELECT id, title, score, source_domain, published_at
FROM articles
WHERE status='unread'
ORDER BY score DESC, COALESCE(published_at, first_seen_at) DESC
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

Raw status counts include age-archived rows, unlike Admin's separate Archived
bucket. Inspect retained archives without modifying them:

```sql
SELECT COUNT(*) AS archived FROM articles WHERE archived_at IS NOT NULL;
SELECT value AS active_feed_age_days FROM app_settings WHERE key='feed_max_age_days';
SELECT id, title, status, hidden_reason, first_seen_at, published_at, archived_at
FROM articles WHERE archived_at IS NOT NULL ORDER BY first_seen_at LIMIT 50;
```

## Story And Scoring Diagnostics

If an age restore succeeded but an article is missing from the first feed page,
inspect its retained state (replace the title fragment):

```sql
SELECT id, status, hidden_reason, archived_at, round(score, 2) AS raw_score,
       first_seen_at, published_at, story_key, duplicate_of, title
FROM articles
WHERE lower(title) LIKE lower('%title fragment%')
ORDER BY score DESC, id;
```

`archived_at IS NULL` confirms no age archive remains; it does not prove feed
eligibility. Read/seen/useful and deliberate hides, minimum score and duplicate
history still matter. Feed order uses derived freshness rank, not raw score:
with default 7-day decay a 60-day-old score of 150 ranks around 15.7. Group age
uses the earliest discovery/publication evidence across all its copies. Restoring
does not reset scores or dates, so eligible old stories can remain on later pages.

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
