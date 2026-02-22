package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"discover/internal/matcher"
	"discover/internal/model"
)

type Store struct {
	db *sql.DB
}

type StatusCounts struct {
	Unread int `json:"unread"`
	Seen   int `json:"seen"`
	Read   int `json:"read"`
	Useful int `json:"useful"`
	Hidden int `json:"hidden"`
}

type TopicStats struct {
	Unread int `json:"unread"`
	Total  int `json:"total"`
}

type IngestDedupeStats struct {
	SameRunHidden    int64 `json:"same_run_hidden"`
	HistoricalHidden int64 `json:"historical_hidden"`
}

const dedupeHiddenTotalSetting = "dedupe_hidden_total"

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) ListEnabledTopics(ctx context.Context) ([]model.Topic, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, query, weight, enabled FROM topics WHERE enabled=1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Topic
	for rows.Next() {
		var t model.Topic
		var en int
		if err := rows.Scan(&t.ID, &t.Query, &t.Weight, &en); err != nil {
			return nil, err
		}
		t.Enabled = en == 1
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) ListTopics(ctx context.Context) ([]model.Topic, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, query, weight, enabled FROM topics ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Topic
	for rows.Next() {
		var t model.Topic
		var en int
		if err := rows.Scan(&t.ID, &t.Query, &t.Weight, &en); err != nil {
			return nil, err
		}
		t.Enabled = en == 1
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) UpsertTopic(ctx context.Context, t model.Topic) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO topics(query, weight, enabled, updated_at)
		VALUES(?,?,?,CURRENT_TIMESTAMP)
		ON CONFLICT(query) DO UPDATE SET
			weight=excluded.weight,
			enabled=excluded.enabled,
			updated_at=CURRENT_TIMESTAMP
	`, strings.TrimSpace(t.Query), t.Weight, boolInt(t.Enabled))
	return err
}

func (s *Store) DeleteTopic(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM topics WHERE id=?`, id)
	return err
}

func (s *Store) ListEnabledNegativeRules(ctx context.Context) ([]model.NegativeRule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, pattern, penalty, enabled, applied_count FROM negative_rules WHERE enabled=1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.NegativeRule
	for rows.Next() {
		var r model.NegativeRule
		var en int
		if err := rows.Scan(&r.ID, &r.Pattern, &r.Penalty, &en, &r.AppliedCount); err != nil {
			return nil, err
		}
		r.Enabled = en == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListNegativeRules(ctx context.Context) ([]model.NegativeRule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, pattern, penalty, enabled, applied_count FROM negative_rules ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.NegativeRule
	for rows.Next() {
		var r model.NegativeRule
		var en int
		if err := rows.Scan(&r.ID, &r.Pattern, &r.Penalty, &en, &r.AppliedCount); err != nil {
			return nil, err
		}
		r.Enabled = en == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) UpsertNegativeRule(ctx context.Context, rule model.NegativeRule) error {
	pattern := strings.TrimSpace(rule.Pattern)
	if pattern == "" {
		return errors.New("empty pattern")
	}
	var prevPenalty float64
	var prevEnabled int
	err := s.db.QueryRowContext(ctx, `SELECT penalty, enabled FROM negative_rules WHERE pattern=?`, pattern).Scan(&prevPenalty, &prevEnabled)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO negative_rules(pattern, penalty, enabled, updated_at)
		VALUES(?,?,?,CURRENT_TIMESTAMP)
		ON CONFLICT(pattern) DO UPDATE SET
			penalty=excluded.penalty,
			enabled=excluded.enabled,
			updated_at=CURRENT_TIMESTAMP
	`, pattern, rule.Penalty, boolInt(rule.Enabled))
	if err != nil {
		return err
	}

	prevActive := 0.0
	if prevEnabled == 1 {
		prevActive = prevPenalty
	}
	newActive := 0.0
	if rule.Enabled {
		newActive = rule.Penalty
	}
	delta := newActive - prevActive
	if delta == 0 {
		return nil
	}
	return s.ApplyRuleRetroactively(ctx, pattern, delta)
}

func (s *Store) DeleteNegativeRule(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM negative_rules WHERE id=?`, id)
	return err
}

func (s *Store) ApplyRuleRetroactively(ctx context.Context, pattern string, penalty float64) error {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return errors.New("empty pattern")
	}
	type unreadArticle struct {
		id           int64
		title        string
		content      string
		sourceDomain string
		articleURL   string
	}
	articles := make([]unreadArticle, 0, 128)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, content, source_domain, url
		FROM articles
		WHERE status='unread'
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var a unreadArticle
		if err := rows.Scan(&a.id, &a.title, &a.content, &a.sourceDomain, &a.articleURL); err != nil {
			return err
		}
		articles = append(articles, a)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `UPDATE articles SET score = score - ?, updated_at=CURRENT_TIMESTAMP WHERE id=?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	matches := int64(0)

	for _, a := range articles {
		if !matcher.MatchRule(pattern, a.title, a.content, a.sourceDomain, a.articleURL) {
			continue
		}
		if _, err := stmt.ExecContext(ctx, penalty, a.id); err != nil {
			return err
		}
		matches++
	}
	if matches > 0 && penalty > 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE negative_rules
			SET applied_count = applied_count + ?, updated_at=CURRENT_TIMESTAMP
			WHERE pattern=?
		`, matches, pattern); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) IncrementNegativeRuleAppliedCounts(ctx context.Context, counts map[int64]int64) error {
	if len(counts) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `UPDATE negative_rules SET applied_count = applied_count + ?, updated_at=CURRENT_TIMESTAMP WHERE id=?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for id, n := range counts {
		if id <= 0 || n <= 0 {
			continue
		}
		if _, err := stmt.ExecContext(ctx, n, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) TopicStats(ctx context.Context) (map[int64]TopicStats, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT at.topic_id,
		       COUNT(*) AS total_count,
		       SUM(CASE WHEN a.status='unread' THEN 1 ELSE 0 END) AS unread_count
		FROM article_topics at
		JOIN articles a ON a.id = at.article_id
		GROUP BY at.topic_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64]TopicStats)
	for rows.Next() {
		var topicID int64
		var total int
		var unread int
		if err := rows.Scan(&topicID, &total, &unread); err != nil {
			return nil, err
		}
		out[topicID] = TopicStats{Unread: unread, Total: total}
	}
	return out, rows.Err()
}

type UpsertArticleInput struct {
	URL           string
	NormalizedURL string
	URLHash       string
	Title         string
	Content       string
	ThumbnailURL  string
	SourceDomain  string
	PublishedAt   time.Time
	IngestedAt    time.Time
	TopicID       int64
	TopicWeight   float64
	Engines       int
	SearxScore    float64
	ExtraTitleHit float64
	Penalty       float64
}

func (s *Store) UpsertArticleHit(ctx context.Context, in UpsertArticleInput) error {
	base := 1.0 + in.TopicWeight + float64(maxInt(in.Engines, 1))*0.25 + in.SearxScore*0.25 + in.ExtraTitleHit - in.Penalty
	if base < -10 {
		base = -10
	}
	if in.PublishedAt.IsZero() {
		in.PublishedAt = in.IngestedAt
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO articles(
			url, normalized_url, url_hash, title, content, thumbnail_url,
			source_domain, published_at, ingested_at, status, score, hit_count,
			engine_count, searx_score, updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,'unread',?,?,?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(url_hash) DO UPDATE SET
			title=excluded.title,
			content=excluded.content,
			thumbnail_url=CASE WHEN excluded.thumbnail_url <> '' THEN excluded.thumbnail_url ELSE articles.thumbnail_url END,
			source_domain=excluded.source_domain,
			published_at=COALESCE(excluded.published_at, articles.published_at),
			ingested_at=excluded.ingested_at,
			score=articles.score + ?,
			hit_count=articles.hit_count + 1,
			engine_count=MAX(articles.engine_count, excluded.engine_count),
			searx_score=MAX(articles.searx_score, excluded.searx_score),
			updated_at=CURRENT_TIMESTAMP
	`, in.URL, in.NormalizedURL, in.URLHash, in.Title, in.Content, in.ThumbnailURL,
		in.SourceDomain, in.PublishedAt.UTC(), in.IngestedAt.UTC(), base, 1, in.Engines, in.SearxScore, base)
	if err != nil {
		return err
	}

	var articleID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM articles WHERE url_hash=?`, in.URLHash).Scan(&articleID); err != nil {
		return err
	}

	if in.TopicID > 0 {
		_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO article_topics(article_id, topic_id) VALUES(?,?)`, articleID, in.TopicID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) FetchTopUnread(ctx context.Context, limit int, minScore float64) ([]model.Article, error) {
	queryLimit := limit * 6
	if queryLimit < 50 {
		queryLimit = 50
	}
	if queryLimit > 600 {
		queryLimit = 600
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, url, normalized_url, url_hash, title, content, thumbnail_url,
			source_domain, COALESCE(published_at, ingested_at), ingested_at,
			status, score, hit_count, engine_count, searx_score
		FROM articles
		WHERE status='unread' AND score >= ?
		ORDER BY score DESC, COALESCE(published_at, ingested_at) DESC, id DESC
		LIMIT ?
	`, minScore, queryLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.Article, 0, limit)
	seenSubject := make(map[string]struct{}, limit*2)
	for rows.Next() {
		var a model.Article
		var status string
		var publishedRaw any
		var ingestedRaw any
		if err := rows.Scan(&a.ID, &a.URL, &a.NormalizedURL, &a.URLHash, &a.Title, &a.Content, &a.ThumbnailURL,
			&a.SourceDomain, &publishedRaw, &ingestedRaw, &status, &a.Score, &a.HitCount, &a.EngineCount, &a.SearxScore); err != nil {
			return nil, err
		}
		a.PublishedAt = parseDBTime(publishedRaw)
		a.IngestedAt = parseDBTime(ingestedRaw)
		a.Status = model.ArticleStatus(status)
		key := subjectKey(a.Title)
		if key != "" {
			if _, ok := seenSubject[key]; ok {
				continue
			}
			seenSubject[key] = struct{}{}
		}
		out = append(out, a)
		if len(out) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) MarkIDsAsSeen(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	q, args := inClause(ids)
	_, err := s.db.ExecContext(ctx, `UPDATE articles SET status='seen', last_seen_at=CURRENT_TIMESTAMP, updated_at=CURRENT_TIMESTAMP WHERE status='unread' AND id IN (`+q+`)`, args...)
	return err
}

func (s *Store) MarkIDStatus(ctx context.Context, id int64, status model.ArticleStatus, delta float64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE articles
		SET status=?, score=score+?, updated_at=CURRENT_TIMESTAMP
		WHERE id=?
	`, string(status), delta, id)
	return err
}

func (s *Store) MarkRead(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE articles SET status='read', updated_at=CURRENT_TIMESTAMP WHERE id=?`, id)
	return err
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO app_settings(key, value, updated_at) VALUES(?,?,CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=CURRENT_TIMESTAMP
	`, key, value)
	return err
}

func (s *Store) GetSettingInt(ctx context.Context, key string, defaultValue int) (int, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key=?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultValue, nil
	}
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue, nil
	}
	return n, nil
}

func (s *Store) CullOldUnread(ctx context.Context, olderThanDays int, maxScore float64) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM articles
		WHERE status='unread'
		  AND score <= ?
		  AND ingested_at < datetime('now', ?)
	`, maxScore, fmt.Sprintf("-%d days", olderThanDays))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) HideUnreadBelowScore(ctx context.Context, threshold float64) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE articles
		SET status='hidden', updated_at=CURRENT_TIMESTAMP
		WHERE status='unread' AND score < ?
	`, threshold)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) HideIngestTitleDuplicates(ctx context.Context, ingestedAt time.Time, keyChars int) (IngestDedupeStats, error) {
	if keyChars < 10 {
		keyChars = 10
	}
	type row struct {
		id    int64
		title string
		score float64
	}
	runRows := make([]row, 0, 256)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, score
		FROM articles
		WHERE status='unread' AND ingested_at=?
	`, ingestedAt.UTC())
	if err != nil {
		return IngestDedupeStats{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.title, &r.score); err != nil {
			return IngestDedupeStats{}, err
		}
		runRows = append(runRows, r)
	}
	if err := rows.Err(); err != nil {
		return IngestDedupeStats{}, err
	}
	if len(runRows) == 0 {
		return IngestDedupeStats{}, nil
	}

	groups := make(map[string][]row, len(runRows))
	for _, r := range runRows {
		key := dedupeTitleKey(r.title, keyChars)
		if key == "" {
			continue
		}
		groups[key] = append(groups[key], r)
	}
	if len(groups) == 0 {
		return IngestDedupeStats{}, nil
	}

	historical := make(map[string]struct{}, len(groups))
	otherRows, err := s.db.QueryContext(ctx, `
		SELECT title
		FROM articles
		WHERE status<>'unread'
	`)
	if err != nil {
		return IngestDedupeStats{}, err
	}
	defer otherRows.Close()
	for otherRows.Next() {
		var title string
		if err := otherRows.Scan(&title); err != nil {
			return IngestDedupeStats{}, err
		}
		key := dedupeTitleKey(title, keyChars)
		if key == "" {
			continue
		}
		if _, ok := groups[key]; ok {
			historical[key] = struct{}{}
		}
	}
	if err := otherRows.Err(); err != nil {
		return IngestDedupeStats{}, err
	}

	toHide := make(map[int64]string, len(runRows))
	stats := IngestDedupeStats{}
	for key, items := range groups {
		if len(items) == 0 {
			continue
		}
		if _, seenBefore := historical[key]; seenBefore {
			for _, it := range items {
				toHide[it.id] = "historical"
			}
			stats.HistoricalHidden += int64(len(items))
			continue
		}
		if len(items) == 1 {
			continue
		}
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].score == items[j].score {
				return items[i].id < items[j].id
			}
			return items[i].score > items[j].score
		})
		for _, dupe := range items[1:] {
			toHide[dupe.id] = "same_run"
		}
		stats.SameRunHidden += int64(len(items) - 1)
	}
	if len(toHide) == 0 {
		return IngestDedupeStats{}, nil
	}

	ids := make([]int64, 0, len(toHide))
	for id := range toHide {
		ids = append(ids, id)
	}
	q, args := inClause(ids)
	_, err = s.db.ExecContext(ctx, `UPDATE articles SET status='hidden', updated_at=CURRENT_TIMESTAMP WHERE status='unread' AND id IN (`+q+`)`, args...)
	if err != nil {
		return IngestDedupeStats{}, err
	}
	if err := s.addToSettingInt(ctx, dedupeHiddenTotalSetting, stats.SameRunHidden+stats.HistoricalHidden); err != nil {
		return IngestDedupeStats{}, err
	}
	return stats, nil
}

func (s *Store) HideAllUnreadTitleDuplicates(ctx context.Context, keyChars int) (IngestDedupeStats, error) {
	if keyChars < 10 {
		keyChars = 10
	}
	type row struct {
		id    int64
		title string
		score float64
	}
	runRows := make([]row, 0, 1024)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, score
		FROM articles
		WHERE status='unread'
	`)
	if err != nil {
		return IngestDedupeStats{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.title, &r.score); err != nil {
			return IngestDedupeStats{}, err
		}
		runRows = append(runRows, r)
	}
	if err := rows.Err(); err != nil {
		return IngestDedupeStats{}, err
	}
	if len(runRows) == 0 {
		return IngestDedupeStats{}, nil
	}

	groups := make(map[string][]row, len(runRows))
	for _, r := range runRows {
		key := dedupeTitleKey(r.title, keyChars)
		if key == "" {
			continue
		}
		groups[key] = append(groups[key], r)
	}
	if len(groups) == 0 {
		return IngestDedupeStats{}, nil
	}

	historical := make(map[string]struct{}, len(groups))
	otherRows, err := s.db.QueryContext(ctx, `
		SELECT title
		FROM articles
		WHERE status<>'unread'
	`)
	if err != nil {
		return IngestDedupeStats{}, err
	}
	defer otherRows.Close()
	for otherRows.Next() {
		var title string
		if err := otherRows.Scan(&title); err != nil {
			return IngestDedupeStats{}, err
		}
		key := dedupeTitleKey(title, keyChars)
		if key == "" {
			continue
		}
		if _, ok := groups[key]; ok {
			historical[key] = struct{}{}
		}
	}
	if err := otherRows.Err(); err != nil {
		return IngestDedupeStats{}, err
	}

	toHide := make(map[int64]struct{}, len(runRows))
	stats := IngestDedupeStats{}
	for key, items := range groups {
		if len(items) == 0 {
			continue
		}
		if _, seenBefore := historical[key]; seenBefore {
			for _, it := range items {
				toHide[it.id] = struct{}{}
			}
			stats.HistoricalHidden += int64(len(items))
			continue
		}
		if len(items) == 1 {
			continue
		}
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].score == items[j].score {
				return items[i].id < items[j].id
			}
			return items[i].score > items[j].score
		})
		for _, dupe := range items[1:] {
			toHide[dupe.id] = struct{}{}
		}
		stats.SameRunHidden += int64(len(items) - 1)
	}
	if len(toHide) == 0 {
		return IngestDedupeStats{}, nil
	}

	ids := make([]int64, 0, len(toHide))
	for id := range toHide {
		ids = append(ids, id)
	}
	q, args := inClause(ids)
	_, err = s.db.ExecContext(ctx, `UPDATE articles SET status='hidden', updated_at=CURRENT_TIMESTAMP WHERE status='unread' AND id IN (`+q+`)`, args...)
	if err != nil {
		return IngestDedupeStats{}, err
	}
	if err := s.addToSettingInt(ctx, dedupeHiddenTotalSetting, stats.SameRunHidden+stats.HistoricalHidden); err != nil {
		return IngestDedupeStats{}, err
	}
	return stats, nil
}

func (s *Store) DedupeHiddenTotal(ctx context.Context) (int, error) {
	return s.GetSettingInt(ctx, dedupeHiddenTotalSetting, 0)
}

func (s *Store) ArticleStatusCounts(ctx context.Context) (StatusCounts, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM articles GROUP BY status`)
	if err != nil {
		return StatusCounts{}, err
	}
	defer rows.Close()
	var out StatusCounts
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return StatusCounts{}, err
		}
		switch status {
		case string(model.StatusUnread):
			out.Unread = count
		case string(model.StatusSeen):
			out.Seen = count
		case string(model.StatusRead):
			out.Read = count
		case string(model.StatusUseful):
			out.Useful = count
		case string(model.StatusHidden):
			out.Hidden = count
		}
	}
	return out, rows.Err()
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func inClause(ids []int64) (string, []any) {
	parts := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		parts[i] = "?"
		args[i] = id
	}
	return strings.Join(parts, ","), args
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func parseDBTime(v any) time.Time {
	switch t := v.(type) {
	case time.Time:
		return t.UTC()
	case string:
		return parseDBTimeString(t)
	case []byte:
		return parseDBTimeString(string(t))
	default:
		return time.Time{}
	}
}

func parseDBTimeString(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func (s *Store) addToSettingInt(ctx context.Context, key string, delta int64) error {
	if delta == 0 {
		return nil
	}
	current, err := s.GetSettingInt(ctx, key, 0)
	if err != nil {
		return err
	}
	return s.SetSetting(ctx, key, strconv.Itoa(current+int(delta)))
}

func subjectKey(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	if title == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(title))
	lastSpace := false
	for _, r := range title {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	key := strings.Join(strings.Fields(b.String()), " ")
	return key
}

func dedupeTitleKey(title string, maxChars int) string {
	title = strings.ToLower(strings.TrimSpace(title))
	if title == "" || maxChars <= 0 {
		return ""
	}
	var b strings.Builder
	b.Grow(len(title))
	for _, r := range title {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
			if b.Len() >= maxChars {
				break
			}
		}
	}
	return b.String()
}
