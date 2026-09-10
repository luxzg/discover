package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"discover/internal/model"
)

type Store struct {
	db       *sql.DB
	keyChars int
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

type ThumbnailCandidate struct {
	ID    int64
	URL   string
	Score float64
}

type IngestDedupeStats struct {
	SameRunHidden    int64 `json:"same_run_hidden"`
	HistoricalHidden int64 `json:"historical_hidden"`
}

const dedupeHiddenTotalSetting = "dedupe_hidden_total"

func New(db *sql.DB) *Store {
	return &Store{db: db, keyChars: 50}
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
	t.Query = strings.TrimSpace(strings.ReplaceAll(t.Query, "+", " "))
	t.Query = strings.Join(strings.Fields(t.Query), " ")
	if t.Query == "" || t.ID < 0 || !finiteScore(t.Weight) {
		return errors.New("invalid topic")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := initializeUnreadScores(ctx, tx); err != nil {
		return err
	}
	if t.ID > 0 {
		res, err := tx.ExecContext(ctx, `UPDATE topics SET query=?,weight=?,enabled=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, t.Query, t.Weight, boolInt(t.Enabled), t.ID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return sql.ErrNoRows
		}
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO topics(query,weight,enabled) VALUES(?,?,?) ON CONFLICT(query) DO UPDATE SET weight=excluded.weight,enabled=excluded.enabled,updated_at=CURRENT_TIMESTAMP`, t.Query, t.Weight, boolInt(t.Enabled))
		if err != nil {
			return err
		}
	}
	if err := rescoreUnread(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteTopic(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := initializeUnreadScores(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM topics WHERE id=?`, id); err != nil {
		return err
	}
	if err := rescoreUnread(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := upsertRule(ctx, tx, rule); err != nil {
		return err
	}
	return tx.Commit()
}

func upsertRule(ctx context.Context, tx *sql.Tx, rule model.NegativeRule) error {
	pattern := strings.TrimSpace(rule.Pattern)
	if pattern == "" || rule.ID < 0 || !finiteScore(rule.Penalty) || rule.Penalty <= 0 {
		return errors.New("invalid negative rule")
	}
	if err := initializeUnreadScores(ctx, tx); err != nil {
		return err
	}
	if rule.ID > 0 {
		res, err := tx.ExecContext(ctx, `UPDATE negative_rules SET pattern=?,penalty=?,enabled=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, pattern, rule.Penalty, boolInt(rule.Enabled), rule.ID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return sql.ErrNoRows
		}
	} else {
		_, err := tx.ExecContext(ctx, `INSERT INTO negative_rules(pattern,penalty,enabled) VALUES(?,?,?) ON CONFLICT(pattern) DO UPDATE SET penalty=excluded.penalty,enabled=excluded.enabled,updated_at=CURRENT_TIMESTAMP`, pattern, rule.Penalty, boolInt(rule.Enabled))
		if err != nil {
			return err
		}
	}
	if err := rescoreUnread(ctx, tx); err != nil {
		return err
	}
	return nil
}

func (s *Store) DeleteNegativeRule(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := initializeUnreadScores(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM negative_rules WHERE id=?`, id); err != nil {
		return err
	}
	if err := rescoreUnread(ctx, tx); err != nil {
		return err
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
	Engines       int
	SearxScore    float64
	ExtraTitleHit float64
}

func (s *Store) UpsertArticleHit(ctx context.Context, in UpsertArticleInput) error {
	return s.upsertArticle(ctx, in)
}

func (s *Store) FetchTopUnread(ctx context.Context, limit int, minScore float64) ([]model.Article, error) {
	return s.fetchStories(ctx, limit, minScore)
}

func (s *Store) MarkIDsAsSeen(ctx context.Context, ids []int64) error {
	return s.markSeen(ctx, ids)
}

func (s *Store) MarkIDStatus(ctx context.Context, id int64, status model.ArticleStatus, delta float64) error {
	return s.markAction(ctx, id, status, delta)
}

func (s *Store) MarkRead(ctx context.Context, id int64) error {
	return s.markAction(ctx, id, model.StatusRead, 0)
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
		WHERE status IN ('unread','hidden') AND hidden_reason IN ('','score')
		  AND score <= ?
		  AND julianday(ingested_at) < julianday(?)
	`, maxScore, dbTimestamp(time.Now().AddDate(0, 0, -olderThanDays)))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) HideUnreadBelowScore(ctx context.Context, threshold float64) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE articles
		SET status='hidden', hidden_reason='score', updated_at=CURRENT_TIMESTAMP
		WHERE status='unread' AND score < ?
	`, threshold)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) ListUnreadThumbnailCandidates(ctx context.Context, minScore float64, limit int) ([]ThumbnailCandidate, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, url, score
		FROM articles
		WHERE status='unread'
		  AND score >= ?
		  AND (thumbnail_url='' OR thumbnail_url IS NULL)
		ORDER BY score DESC, id DESC
		LIMIT ?
	`, minScore, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ThumbnailCandidate, 0, limit)
	for rows.Next() {
		var c ThumbnailCandidate
		if err := rows.Scan(&c.ID, &c.URL, &c.Score); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) SetThumbnailIfEmpty(ctx context.Context, articleID int64, thumbnailURL string) (bool, error) {
	thumbnailURL = strings.TrimSpace(thumbnailURL)
	if articleID <= 0 || thumbnailURL == "" {
		return false, nil
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE articles
		SET thumbnail_url=?, updated_at=CURRENT_TIMESTAMP
		WHERE id=?
		  AND (thumbnail_url='' OR thumbnail_url IS NULL)
	`, thumbnailURL, articleID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *Store) HideIngestTitleDuplicates(ctx context.Context, ingestedAt time.Time, keyChars int) (IngestDedupeStats, error) {
	return s.dedupe(ctx, &ingestedAt, keyChars)
}

func (s *Store) HideAllUnreadTitleDuplicates(ctx context.Context, keyChars int) (IngestDedupeStats, error) {
	return s.dedupe(ctx, nil, keyChars)
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
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05.999999999 -0700",
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

func displayThumbnailURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "null") {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(raw), "data:image/") {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	pathLower := strings.ToLower(u.Path)

	// Startpage proxy: decode embedded direct image URL from piurl.
	if (host == "startpage.com" || strings.HasSuffix(host, ".startpage.com")) && strings.Contains(pathLower, "/av/proxy-image") {
		if direct := decodeStartpagePiURL(u); direct != "" {
			return direct
		}
	}
	// Brave proxy: decode base64 payload from path segment.
	if host == "imgs.search.brave.com" {
		if direct := decodeBraveProxyURL(u.Path); direct != "" {
			return direct
		}
	}
	return u.String()
}

func decodeStartpagePiURL(u *url.URL) string {
	piurl := strings.TrimSpace(u.Query().Get("piurl"))
	if piurl == "" {
		return ""
	}
	candidates := []string{piurl}
	if decoded, err := url.QueryUnescape(piurl); err == nil && decoded != "" {
		candidates = append(candidates, decoded)
	}
	for _, c := range candidates {
		du, err := url.Parse(strings.TrimSpace(c))
		if err != nil {
			continue
		}
		ds := strings.ToLower(du.Scheme)
		if (ds == "http" || ds == "https") && du.Hostname() != "" {
			return du.String()
		}
	}
	return ""
}

func decodeBraveProxyURL(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, part := range parts {
		if !strings.HasPrefix(part, "aHR0c") {
			continue
		}
		payload := strings.Join(parts[i:], "")
		for _, c := range decodeBase64Candidates(strings.TrimRight(payload, "=")) {
			du, err := url.Parse(c)
			if err != nil {
				continue
			}
			if (du.Scheme == "https" || du.Scheme == "http") && du.Hostname() != "" && du.User == nil {
				return du.String()
			}
		}
	}
	return ""
}

func decodeBase64Candidates(s string) []string {
	out := make([]string, 0, 2)
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		out = append(out, string(b))
	}
	if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		out = append(out, string(b))
	}
	return out
}
