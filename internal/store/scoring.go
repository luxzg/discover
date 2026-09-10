package store

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"
	"time"

	"discover/internal/matcher"
	"discover/internal/model"
)

func finiteScore(n float64) bool { return !math.IsNaN(n) && !math.IsInf(n, 0) && math.Abs(n) <= 10000 }

type scoredRow struct {
	id                          int64
	title, content, domain, url string
	score                       float64
	base                        sql.NullFloat64
}

func unreadScoreRows(ctx context.Context, tx *sql.Tx, uninitialized bool) ([]scoredRow, error) {
	query := `SELECT id,title,content,source_domain,url,score,score_base FROM articles WHERE (status='unread' OR (status='hidden' AND hidden_reason='duplicate'))`
	if uninitialized {
		query += ` AND score_base IS NULL`
	}
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []scoredRow{}
	for rows.Next() {
		var a scoredRow
		if err := rows.Scan(&a.id, &a.title, &a.content, &a.domain, &a.url, &a.score, &a.base); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func txRules(ctx context.Context, tx *sql.Tx) ([]model.NegativeRule, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id,pattern,penalty,enabled FROM negative_rules`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.NegativeRule{}
	for rows.Next() {
		var r model.NegativeRule
		var en int
		if err := rows.Scan(&r.ID, &r.Pattern, &r.Penalty, &en); err != nil {
			return nil, err
		}
		r.Enabled = en == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

// Legacy scores are retained as a baseline. Recording existing rule effects lets
// future edits be reversible without pretending to reconstruct old hit history.
func initializeScore(ctx context.Context, tx *sql.Tx, a scoredRow, rules []model.NegativeRule) error {
	if a.base.Valid {
		return nil
	}
	penalties := 0.0
	for _, r := range rules {
		if !r.Enabled || !matcher.MatchRule(r.Pattern, a.title, a.content, a.domain, a.url) {
			continue
		}
		penalties += r.Penalty
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO article_rule_effects(article_id,rule_id,penalty,counted) VALUES(?,?,?,1)`, a.id, r.ID, r.Penalty); err != nil {
			return err
		}
	}
	// Seed the strongest existing evidence without adding to the legacy total.
	rows, err := tx.QueryContext(ctx, `SELECT t.id,t.query,t.weight,t.enabled,a.engine_count,a.searx_score
 FROM article_topics at JOIN topics t ON t.id=at.topic_id JOIN articles a ON a.id=at.article_id WHERE at.article_id=?`, a.id)
	if err != nil {
		return err
	}
	type evidence struct {
		id                int64
		relevance, weight float64
		enabled           int
	}
	evidenceRows := []evidence{}
	for rows.Next() {
		var e evidence
		var query string
		var engines int
		var searx float64
		if err := rows.Scan(&e.id, &query, &e.weight, &e.enabled, &engines, &searx); err != nil {
			rows.Close()
			return err
		}
		e.relevance = 1 + float64(maxInt(engines, 1))*.25 + searx*.25
		for _, term := range strings.Fields(strings.ToLower(strings.ReplaceAll(query, "+", " "))) {
			if len(term) >= 3 {
				if strings.Contains(strings.ToLower(a.title), term) {
					e.relevance += .35
				}
				if strings.Contains(strings.ToLower(a.content), term) {
					e.relevance += .1
				}
			}
		}
		evidenceRows = append(evidenceRows, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	positive := 0.0
	for _, e := range evidenceRows {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO article_evidence(article_id,topic_id,relevance) VALUES(?,?,?)`, a.id, e.id, e.relevance); err != nil {
			return err
		}
		if e.enabled == 1 {
			positive += e.relevance + e.weight
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE articles SET score_base=? WHERE id=? AND score_base IS NULL`, a.score+penalties-positive, a.id)
	return err
}

func initializeUnreadScores(ctx context.Context, tx *sql.Tx) error {
	articles, err := unreadScoreRows(ctx, tx, true)
	if err != nil {
		return err
	}
	rules, err := txRules(ctx, tx)
	if err != nil {
		return err
	}
	for _, a := range articles {
		if err := initializeScore(ctx, tx, a, rules); err != nil {
			return err
		}
	}
	return nil
}

func rescoreRow(ctx context.Context, tx *sql.Tx, a scoredRow, rules []model.NegativeRule) error {
	for _, r := range rules {
		penalty := 0.0
		if r.Enabled && matcher.MatchRule(r.Pattern, a.title, a.content, a.domain, a.url) {
			penalty = r.Penalty
		}
		var counted int
		err := tx.QueryRowContext(ctx, `SELECT counted FROM article_rule_effects WHERE article_id=? AND rule_id=?`, a.id, r.ID).Scan(&counted)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if penalty > 0 && counted == 0 {
			if _, err := tx.ExecContext(ctx, `UPDATE negative_rules SET applied_count=applied_count+1 WHERE id=?`, r.ID); err != nil {
				return err
			}
			counted = 1
		}
		if penalty == 0 && counted == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO article_rule_effects(article_id,rule_id,penalty,counted) VALUES(?,?,?,?)
   ON CONFLICT(article_id,rule_id) DO UPDATE SET penalty=excluded.penalty,counted=excluded.counted`, a.id, r.ID, penalty, counted); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `UPDATE articles SET score=score_base+
  COALESCE((SELECT SUM(e.relevance+t.weight) FROM article_evidence e JOIN topics t ON t.id=e.topic_id WHERE e.article_id=articles.id AND t.enabled=1),0)-
  COALESCE((SELECT SUM(penalty) FROM article_rule_effects r WHERE r.article_id=articles.id),0),updated_at=CURRENT_TIMESTAMP
  WHERE id=? AND (status='unread' OR (status='hidden' AND hidden_reason='duplicate'))`, a.id)
	return err
}

func rescoreUnread(ctx context.Context, tx *sql.Tx) error {
	articles, err := unreadScoreRows(ctx, tx, false)
	if err != nil {
		return err
	}
	rules, err := txRules(ctx, tx)
	if err != nil {
		return err
	}
	for _, a := range articles {
		if err := rescoreRow(ctx, tx, a, rules); err != nil {
			return err
		}
	}
	return reconcileDerivedStories(ctx, tx)
}

func dbTimestamp(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func (s *Store) upsertArticle(ctx context.Context, in UpsertArticleInput) error {
	if in.URLHash == "" || strings.TrimSpace(in.Title) == "" || !finiteScore(in.SearxScore) || !finiteScore(in.ExtraTitleHit) {
		return errors.New("invalid article")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rules, err := txRules(ctx, tx)
	if err != nil {
		return err
	}
	var a scoredRow
	err = tx.QueryRowContext(ctx, `SELECT id,title,content,source_domain,url,score,score_base FROM articles WHERE url_hash=?`, in.URLHash).Scan(&a.id, &a.title, &a.content, &a.domain, &a.url, &a.score, &a.base)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	oldKey := dedupeTitleKey(a.title, s.keyChars)
	if a.id > 0 {
		if err := initializeScore(ctx, tx, a, rules); err != nil {
			return err
		}
		// A changed headline may leave a former duplicate in a different story.
		if dedupeTitleKey(a.title, s.keyChars) != dedupeTitleKey(in.Title, s.keyChars) {
			if _, err := tx.ExecContext(ctx, `UPDATE articles SET status='unread',hidden_reason='',duplicate_of=NULL WHERE id=? AND hidden_reason='duplicate' AND status='hidden'`, a.id); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE articles SET title=CASE WHEN status='unread' OR hidden_reason='duplicate' THEN ? ELSE title END,content=CASE WHEN ?<>'' THEN ? ELSE content END,
   thumbnail_url=CASE WHEN thumbnail_url='' AND ?<>'' THEN ? ELSE thumbnail_url END,
   published_at=COALESCE(?,published_at),ingested_at=?,story_key=CASE WHEN status='unread' OR hidden_reason='duplicate' THEN ? ELSE story_key END,
   hit_count=hit_count+1,engine_count=MAX(engine_count,?),searx_score=MAX(searx_score,?),updated_at=CURRENT_TIMESTAMP WHERE id=?`,
			in.Title, in.Content, in.Content, in.ThumbnailURL, in.ThumbnailURL, dbTimestamp(in.PublishedAt), dbTimestamp(in.IngestedAt), dedupeTitleKey(in.Title, s.keyChars), in.Engines, in.SearxScore, a.id)
	} else {
		res, e := tx.ExecContext(ctx, `INSERT INTO articles(url,normalized_url,url_hash,title,content,thumbnail_url,source_domain,published_at,ingested_at,story_key,score_base,hit_count,engine_count,searx_score)
   VALUES(?,?,?,?,?,?,?,?,?,?,0,1,?,?)`, in.URL, in.NormalizedURL, in.URLHash, in.Title, in.Content, in.ThumbnailURL, in.SourceDomain, dbTimestamp(in.PublishedAt), dbTimestamp(in.IngestedAt), dedupeTitleKey(in.Title, s.keyChars), in.Engines, in.SearxScore)
		if e != nil {
			return e
		}
		a.id, err = res.LastInsertId()
	}
	if err != nil {
		return err
	}
	if in.TopicID > 0 {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO article_topics(article_id,topic_id) VALUES(?,?)`, a.id, in.TopicID); err != nil {
			return err
		}
		relevance := 1 + float64(maxInt(in.Engines, 1))*.25 + in.SearxScore*.25 + in.ExtraTitleHit
		if _, err := tx.ExecContext(ctx, `INSERT INTO article_evidence(article_id,topic_id,relevance) VALUES(?,?,?)
   ON CONFLICT(article_id,topic_id) DO UPDATE SET relevance=MAX(article_evidence.relevance,excluded.relevance)`, a.id, in.TopicID, relevance); err != nil {
			return err
		}
	}
	a.title = in.Title
	if a.url == "" {
		a.domain, a.url = in.SourceDomain, in.URL
	}
	if in.Content != "" {
		a.content = in.Content
	}
	if err := rescoreRow(ctx, tx, a, rules); err != nil {
		return err
	}
	for _, key := range []string{oldKey, dedupeTitleKey(in.Title, s.keyChars)} {
		var hasDerived bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM articles WHERE story_key=? AND hidden_reason='duplicate' AND status='hidden')`, key).Scan(&hasDerived); err != nil {
			return err
		}
		if hasDerived {
			if _, err := hideGroup(ctx, tx, key); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
