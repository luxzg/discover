package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"discover/internal/model"
	"discover/internal/urlnorm"
)

func dedupeTitleKey(title string, maxChars int) string {
	var b strings.Builder
	n := 0
	for _, r := range strings.ToLower(title) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
			n++
			if n >= maxChars {
				break
			}
		}
	}
	return b.String()
}

// Prepare upgrades derived identities and timestamp representations only once.
// Legacy hidden decisions remain hidden; their unknown origin is not guessed.
func (s *Store) Prepare(ctx context.Context, keyChars int) error {
	if keyChars < 10 || keyChars > 200 {
		return errors.New("invalid story key length")
	}
	s.keyChars = keyChars
	var previous string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key='story_model_version'`).Scan(&previous)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	wanted := "1:" + strconv.Itoa(keyChars)
	if previous == wanted {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	type entry struct {
		id                     int64
		title, url, hash, norm string
		published, ingested    time.Time
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,title,url,url_hash,normalized_url,published_at,ingested_at FROM articles ORDER BY id`)
	if err != nil {
		return err
	}
	all := []entry{}
	for rows.Next() {
		var a entry
		var pub, ing any
		if err := rows.Scan(&a.id, &a.title, &a.url, &a.hash, &a.norm, &pub, &ing); err != nil {
			rows.Close()
			return err
		}
		a.published = parseDBTime(pub)
		a.ingested = parseDBTime(ing)
		all = append(all, a)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	first := !strings.HasPrefix(previous, "1:")
	if first {
		// Free old hashes before refinement; retain colliding legacy copies as sources.
		if _, err := tx.ExecContext(ctx, `UPDATE articles SET url_hash='migration:'||id`); err != nil {
			return err
		}
	}
	hashes := map[string]bool{}
	for _, a := range all {
		if first {
			norm, hash, _, err := urlnorm.Normalize(a.url)
			if err == nil {
				a.norm, a.hash = norm, hash
			}
			if hashes[a.hash] {
				a.hash = "legacy:" + strconv.FormatInt(a.id, 10) + ":" + a.hash
			}
			hashes[a.hash] = true
			// The old implementation substituted precisely this timestamp for unknown dates.
			if a.published.Equal(a.ingested) {
				a.published = time.Time{}
			}
			if _, err := tx.ExecContext(ctx, `UPDATE articles SET normalized_url=?,url_hash=?,published_at=?,ingested_at=? WHERE id=?`, a.norm, a.hash, dbTimestamp(a.published), dbTimestamp(a.ingested), a.id); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE articles SET story_key=? WHERE id=?`, dedupeTitleKey(a.title, keyChars), a.id); err != nil {
			return err
		}
	}
	// Initialize all legacy rows now, before newly created rules can be mistaken
	// for pre-upgrade effects on a handled article.
	rules, err := txRules(ctx, tx)
	if err != nil {
		return err
	}
	for _, a := range all {
		var row scoredRow
		if err := tx.QueryRowContext(ctx, `SELECT id,title,content,source_domain,url,score,score_base FROM articles WHERE id=?`, a.id).Scan(&row.id, &row.title, &row.content, &row.domain, &row.url, &row.score, &row.base); err != nil {
			return err
		}
		if err := initializeScore(ctx, tx, row, rules); err != nil {
			return err
		}
	}
	if !first {
		// Reindexing may split a former group. Only derived hides can be reconsidered.
		if _, err := tx.ExecContext(ctx, `UPDATE articles SET status='unread',hidden_reason='',duplicate_of=NULL WHERE status='hidden' AND hidden_reason='duplicate'`); err != nil {
			return err
		}
		if err := rescoreUnread(ctx, tx); err != nil {
			return err
		}
		keys := map[string]bool{}
		for _, a := range all {
			keys[dedupeTitleKey(a.title, keyChars)] = true
		}
		for key := range keys {
			if _, err := hideGroup(ctx, tx, key); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO app_settings(key,value) VALUES('story_model_version',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, wanted); err != nil {
		return err
	}
	return tx.Commit()
}

type storyRow struct {
	id             int64
	status, reason string
	score          float64
	counted        int
}

func storyRows(ctx context.Context, tx *sql.Tx, key string) ([]storyRow, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id,status,hidden_reason,score,dedupe_counted FROM articles WHERE story_key=? ORDER BY score DESC,id ASC`, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []storyRow{}
	for rows.Next() {
		var a storyRow
		if err := rows.Scan(&a.id, &a.status, &a.reason, &a.score, &a.counted); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func hideGroup(ctx context.Context, tx *sql.Tx, key string) (IngestDedupeStats, error) {
	var stats IngestDedupeStats
	if key == "" {
		return stats, nil
	}
	members, err := storyRows(ctx, tx, key)
	if err != nil {
		return stats, err
	}
	winner := int64(0)
	historical := false
	for _, a := range members {
		if a.status == "seen" || a.status == "read" || a.status == "useful" || (a.status == "hidden" && (a.reason == "manual" || a.reason == "legacy" || a.reason == "")) {
			winner = a.id
			historical = true
			break
		}
	}
	if winner == 0 {
		for _, a := range members {
			if a.status == "unread" || (a.status == "hidden" && a.reason == "duplicate") {
				winner = a.id
				break
			}
		}
	}
	if winner == 0 {
		return stats, nil
	}
	if !historical {
		if _, err := tx.ExecContext(ctx, `UPDATE articles SET status='unread',hidden_reason='',duplicate_of=NULL WHERE id=? AND status='hidden' AND hidden_reason='duplicate'`, winner); err != nil {
			return stats, err
		}
	}
	newlyCounted := int64(0)
	for _, a := range members {
		if a.status == "hidden" && a.reason == "duplicate" && a.id != winner {
			if _, err := tx.ExecContext(ctx, `UPDATE articles SET duplicate_of=? WHERE id=?`, winner, a.id); err != nil {
				return stats, err
			}
		}
		if a.status != "unread" || a.id == winner {
			continue
		}
		res, err := tx.ExecContext(ctx, `UPDATE articles SET status='hidden',hidden_reason='duplicate',duplicate_of=?,dedupe_counted=1,updated_at=CURRENT_TIMESTAMP WHERE id=? AND status='unread'`, winner, a.id)
		if err != nil {
			return stats, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return stats, err
		}
		if historical {
			stats.HistoricalHidden += n
		} else {
			stats.SameRunHidden += n
		}
		if a.counted == 0 {
			newlyCounted += n
		}
	}
	if newlyCounted > 0 {
		_, err := tx.ExecContext(ctx, `INSERT INTO app_settings(key,value) VALUES('dedupe_hidden_total',CAST(? AS TEXT))
   ON CONFLICT(key) DO UPDATE SET value=CAST(CAST(app_settings.value AS INTEGER)+? AS TEXT),updated_at=CURRENT_TIMESTAMP`, newlyCounted, newlyCounted)
		if err != nil {
			return stats, err
		}
	}
	return stats, nil
}

// Reconsider only derived groups. Manual/legacy hides remain authoritative.
func reconcileDerivedStories(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT story_key FROM articles WHERE status='hidden' AND hidden_reason='duplicate'`)
	if err != nil {
		return err
	}
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return err
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, key := range keys {
		if _, err := hideGroup(ctx, tx, key); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) dedupe(ctx context.Context, at *time.Time, keyChars int) (IngestDedupeStats, error) {
	var stats IngestDedupeStats
	if keyChars != s.keyChars {
		return stats, errors.New("story key configuration changed; restart to reindex")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return stats, err
	}
	defer tx.Rollback()
	query := `SELECT DISTINCT story_key FROM articles WHERE status='unread'`
	args := []any{}
	if at != nil {
		query += ` AND ingested_at=?`
		args = append(args, dbTimestamp(*at))
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return stats, err
	}
	keys := []string{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			rows.Close()
			return stats, err
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return stats, err
	}
	rows.Close()
	for _, key := range keys {
		n, err := hideGroup(ctx, tx, key)
		if err != nil {
			return IngestDedupeStats{}, err
		}
		stats.SameRunHidden += n.SameRunHidden
		stats.HistoricalHidden += n.HistoricalHidden
	}
	if err := tx.Commit(); err != nil {
		return IngestDedupeStats{}, err
	}
	return stats, nil
}

func (s *Store) fetchStories(ctx context.Context, limit int, minScore float64) ([]model.Article, error) {
	if limit < 1 || limit > 100 {
		return nil, errors.New("limit must be 1..100")
	}
	rows, err := s.db.QueryContext(ctx, `WITH ranked AS (
 SELECT a.*,ROW_NUMBER() OVER(PARTITION BY CASE WHEN story_key='' THEN 'id:'||id ELSE story_key END ORDER BY score DESC,id ASC) AS rank
 FROM articles a WHERE status='unread' AND score>=?
 AND (story_key='' OR NOT EXISTS (
 SELECT 1 FROM articles h WHERE h.story_key=a.story_key AND
 (h.status IN ('seen','read','useful') OR (h.status='hidden' AND h.hidden_reason IN ('manual','legacy',''))))))
 SELECT id,url,normalized_url,url_hash,title,content,thumbnail_url,source_domain,published_at,ingested_at,status,score,hit_count,engine_count,searx_score,story_key
 FROM ranked WHERE rank=1 ORDER BY score DESC,COALESCE(published_at,ingested_at) DESC,id DESC LIMIT ?`, minScore, limit)
	if err != nil {
		return nil, err
	}
	out := []model.Article{}
	for rows.Next() {
		var a model.Article
		var pub, ing any
		if err := rows.Scan(&a.ID, &a.URL, &a.NormalizedURL, &a.URLHash, &a.Title, &a.Content, &a.ThumbnailURL, &a.SourceDomain, &pub, &ing, &a.Status, &a.Score, &a.HitCount, &a.EngineCount, &a.SearxScore, &a.StoryKey); err != nil {
			rows.Close()
			return nil, err
		}
		a.PublishedAt = parseDBTime(pub)
		a.IngestedAt = parseDBTime(ing)
		a.ThumbnailURL = displayThumbnailURL(a.ThumbnailURL)
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for i := range out {
		a := &out[i]
		if a.StoryKey == "" {
			continue
		}
		rows, err := s.db.QueryContext(ctx, `SELECT id,url,title,source_domain,published_at,thumbnail_url FROM articles
   WHERE story_key=? AND id<>? AND (status='unread' OR (status='hidden' AND hidden_reason='duplicate')) ORDER BY score DESC,id LIMIT 20`, a.StoryKey, a.ID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var source model.StorySource
			var pub any
			if err := rows.Scan(&source.ID, &source.URL, &source.Title, &source.SourceDomain, &pub, &source.ThumbnailURL); err != nil {
				rows.Close()
				return nil, err
			}
			source.PublishedAt = parseDBTime(pub)
			source.ThumbnailURL = displayThumbnailURL(source.ThumbnailURL)
			a.Sources = append(a.Sources, source)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
		// Prefer the representative's image; borrow a source image only when missing.
		if a.ThumbnailURL == "" {
			for _, source := range a.Sources {
				if source.ThumbnailURL != "" {
					a.ThumbnailURL = source.ThumbnailURL
					break
				}
			}
		}
	}
	return out, nil
}

func (s *Store) markSeen(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	keys := map[string]bool{}
	for _, id := range ids {
		var key string
		err := tx.QueryRowContext(ctx, `SELECT story_key FROM articles WHERE id=?`, id).Scan(&key)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE articles SET status='seen',hidden_reason='',duplicate_of=NULL,last_seen_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=? AND (status='unread' OR (status='hidden' AND hidden_reason='duplicate'))`, id); err != nil {
			return err
		}
		keys[key] = true
	}
	for k := range keys {
		if _, err := hideGroup(ctx, tx, k); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) markAction(ctx context.Context, id int64, status model.ArticleStatus, delta float64) error {
	if id <= 0 {
		return errors.New("invalid article id")
	}
	if status != model.StatusUseful && status != model.StatusHidden && status != model.StatusRead {
		return errors.New("invalid article action")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var old, reason, key string
	var vote int
	if err := tx.QueryRowContext(ctx, `SELECT status,hidden_reason,story_key,vote FROM articles WHERE id=?`, id).Scan(&old, &reason, &key, &vote); err != nil {
		return err
	}
	// Stale tabs must not undo a deliberate hide. Duplicate source links can be read.
	if old == "hidden" && reason != "duplicate" && status != model.StatusHidden {
		return tx.Commit()
	}
	if old == "useful" && status == model.StatusRead {
		status = model.StatusUseful
	}
	adjustment := 0.0
	newVote := vote
	if status == model.StatusUseful && vote != 1 {
		adjustment = 1
		newVote = 1
	}
	if status == model.StatusHidden && old != "hidden" {
		adjustment = delta
		newVote = -1
	}
	newReason := ""
	if status == model.StatusHidden {
		newReason = "manual"
	}
	_, err = tx.ExecContext(ctx, `UPDATE articles SET status=?,hidden_reason=?,duplicate_of=NULL,vote=?,score=score+?,score_base=CASE WHEN score_base IS NULL THEN NULL ELSE score_base+? END,
 read_at=CASE WHEN ? THEN CURRENT_TIMESTAMP ELSE read_at END,updated_at=CURRENT_TIMESTAMP WHERE id=?`, status, newReason, newVote, adjustment, adjustment, status == model.StatusRead || old == "useful", id)
	if err != nil {
		return err
	}
	if _, err := hideGroup(ctx, tx, key); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RecomputeUnread(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := initializeUnreadScores(ctx, tx); err != nil {
		return fmt.Errorf("initialize scores: %w", err)
	}
	if err := rescoreUnread(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}
