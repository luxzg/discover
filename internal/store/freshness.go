package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// One group clock prevents a later syndicated copy from making a story new.
const ageCTE = `WITH clocks AS (
 SELECT CASE WHEN story_key='' THEN 'id:'||id ELSE story_key END AS identity,
 MIN(julianday(COALESCE(first_seen_at,created_at))) AS first_seen,
 MIN(CASE WHEN julianday(published_at)<=julianday('now','+1 day') THEN julianday(published_at) END) AS published
 FROM articles GROUP BY identity
), aged AS (
 SELECT a.*, c.first_seen AS story_first_seen, c.published AS story_published, MIN(c.first_seen,COALESCE(c.published,c.first_seen)) AS age_date
 FROM articles a JOIN clocks c ON c.identity=CASE WHEN a.story_key='' THEN 'id:'||a.id ELSE a.story_key END
) `

func (s *Store) prepareFirstSeen(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id,created_at,ingested_at FROM articles WHERE first_seen_at IS NULL`)
	if err != nil {
		return err
	}
	type entry struct {
		id    int64
		first time.Time
	}
	var entries []entry
	for rows.Next() {
		var e entry
		var created, ingested any
		if err := rows.Scan(&e.id, &created, &ingested); err != nil {
			rows.Close()
			return err
		}
		e.first = parseDBTime(created)
		if e.first.IsZero() {
			e.first = parseDBTime(ingested)
		}
		if e.first.IsZero() {
			rows.Close()
			return errors.New("article has no recoverable first-seen timestamp")
		}
		entries = append(entries, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, e := range entries {
		if _, err := tx.ExecContext(ctx, `UPDATE articles SET first_seen_at=? WHERE id=? AND first_seen_at IS NULL`, dbTimestamp(e.first), e.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) FeedAgeLimit(ctx context.Context) (int, error) {
	return s.GetSettingInt(ctx, "feed_max_age_days", s.maxAgeDays)
}

type ArchiveStats struct {
	Days       int   `json:"days"`
	Candidates int64 `json:"candidates"`
	Archived   int64 `json:"archived"`
	Restored   int64 `json:"restored"`
}

func validAgeDays(days int) bool { return days >= 0 && days <= 36500 }

func (s *Store) PreviewArchive(ctx context.Context, days int) (ArchiveStats, error) {
	stats := ArchiveStats{Days: days}
	if !validAgeDays(days) {
		return stats, errors.New("age days must be 0..36500")
	}
	err := s.db.QueryRowContext(ctx, ageCTE+`SELECT COUNT(*) FROM aged
 WHERE (status='unread' OR (status='hidden' AND hidden_reason IN ('duplicate','score')))
 AND archived_at IS NULL AND ?>0 AND age_date<julianday('now')-?`, days, days).Scan(&stats.Candidates)
	return stats, err
}

// Archive is independent of reading/voting/hiding; changing the age limit can undo it.
func (s *Store) ArchiveOldUnread(ctx context.Context, days int, persistLimit bool) (ArchiveStats, error) {
	stats := ArchiveStats{Days: days}
	if !validAgeDays(days) {
		return stats, errors.New("age days must be 0..36500")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return stats, err
	}
	defer tx.Rollback()
	if persistLimit {
		if _, err := tx.ExecContext(ctx, `INSERT INTO app_settings(key,value) VALUES('feed_max_age_days',CAST(? AS TEXT))
 ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=CURRENT_TIMESTAMP`, days); err != nil {
			return stats, err
		}
	}
	eligible := ` (status='unread' OR (status='hidden' AND hidden_reason IN ('duplicate','score'))) `
	res, err := tx.ExecContext(ctx, ageCTE+`UPDATE articles SET archived_at=CURRENT_TIMESTAMP WHERE id IN
 (SELECT id FROM aged WHERE `+eligible+` AND archived_at IS NULL AND ?>0 AND age_date<julianday('now')-?)`, days, days)
	if err != nil {
		return stats, err
	}
	stats.Archived, err = res.RowsAffected()
	if err != nil {
		return stats, err
	}
	res, err = tx.ExecContext(ctx, ageCTE+`UPDATE articles SET archived_at=NULL WHERE id IN
 (SELECT id FROM aged WHERE `+eligible+` AND archived_at IS NOT NULL AND (?=0 OR age_date>=julianday('now')-?))`, days, days)
	if err != nil {
		return stats, err
	}
	stats.Restored, err = res.RowsAffected()
	if err != nil {
		return stats, err
	}
	return stats, tx.Commit()
}

type DomainStats struct {
	Domain   string `json:"domain"`
	Total    int64  `json:"total"`
	Read     int64  `json:"read"`
	Useful   int64  `json:"useful"`
	Hidden   int64  `json:"hidden"`
	Positive int64  `json:"positive"`
	Seen     int64  `json:"seen"`
}

// Current deliberate hides take precedence over an earlier click. No auto-hide is a dislike.
func (s *Store) DomainReport(ctx context.Context) ([]DomainStats, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT lower(source_domain),COUNT(*),
 SUM(CASE WHEN (status='read' OR read_at IS NOT NULL) AND NOT (status='hidden' AND hidden_reason IN ('manual','legacy','')) THEN 1 ELSE 0 END),
 SUM(CASE WHEN status='useful' THEN 1 ELSE 0 END),
 SUM(CASE WHEN status='hidden' AND hidden_reason='manual' THEN 1 ELSE 0 END),
 SUM(CASE WHEN (status IN ('read','useful') OR read_at IS NOT NULL) AND NOT (status='hidden' AND hidden_reason IN ('manual','legacy','')) THEN 1 ELSE 0 END),
 SUM(CASE WHEN last_seen_at IS NOT NULL OR status IN ('seen','read','useful') THEN 1 ELSE 0 END)
 FROM articles GROUP BY lower(source_domain) ORDER BY 6 DESC,3 DESC,2 DESC,1 LIMIT 500`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DomainStats{}
	for rows.Next() {
		var d DomainStats
		if err := rows.Scan(&d.Domain, &d.Total, &d.Read, &d.Useful, &d.Hidden, &d.Positive, &d.Seen); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func storyFirstSeen(ctx context.Context, tx *sql.Tx, key string, fallback time.Time) (time.Time, error) {
	var raw any
	err := tx.QueryRowContext(ctx, `SELECT first_seen_at FROM articles WHERE story_key=? AND first_seen_at IS NOT NULL ORDER BY julianday(first_seen_at),id LIMIT 1`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return fallback, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	first := parseDBTime(raw)
	if first.IsZero() || fallback.Before(first) {
		return fallback, nil
	}
	return first, nil
}
