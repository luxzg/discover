package store

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"discover/internal/db"
	"discover/internal/model"
)

func benchmarkStore(b *testing.B) *Store {
	b.Helper()
	d, err := db.Open(filepath.Join(b.TempDir(), "benchmark.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { d.Close() })
	s := New(d)
	if err := s.Prepare(b.Context(), 50); err != nil {
		b.Fatal(err)
	}
	return s
}

// Fixed synthetic payloads; fixture creation is outside the measurement.
func BenchmarkRemediationFeed(b *testing.B) {
	s := benchmarkStore(b)
	ctx := b.Context()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO articles(url,normalized_url,url_hash,title,content,source_domain,story_key,first_seen_at,ingested_at,score_base,score) VALUES(?,?,?,?,?,?,?,?,?,100,100)`)
	if err != nil {
		b.Fatal(err)
	}
	now := dbTimestamp(time.Now().UTC())
	content := strings.Repeat("synthetic payload ", 128)
	for i := 0; i < 40000; i++ {
		u := fmt.Sprintf("https://example.invalid/article/%d", i)
		key := fmt.Sprintf("group%06d", i/25)
		if _, err := stmt.ExecContext(ctx, u, u, fmt.Sprint(i), key, content, "example.invalid", key, now, now); err != nil {
			b.Fatal(err)
		}
	}
	stmt.Close()
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
	for _, limit := range []int{10, 100} {
		b.Run(fmt.Sprint(limit), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				cards, err := s.FetchTopUnread(ctx, limit, 1)
				if err != nil || len(cards) != limit || len(cards[0].Sources) != 20 {
					b.Fatalf("cards=%d err=%v", len(cards), err)
				}
			}
		})
	}
	cards, err := s.FetchTopUnread(ctx, 100, 1)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("sources", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for j := range cards {
				cards[j].Sources = nil
			}
			if err := s.fillStorySources(ctx, cards); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkRemediationEffects(b *testing.B) {
	s := benchmarkStore(b)
	ctx := b.Context()
	var words []string
	for i := 0; i < 80; i++ {
		word := fmt.Sprintf("word%03d", i)
		words = append(words, word)
		if _, err := s.db.ExecContext(ctx, `INSERT INTO negative_rules(pattern,penalty,enabled) VALUES(?,1,1)`, word); err != nil {
			b.Fatal(err)
		}
	}
	in := UpsertArticleInput{URL: "https://example.invalid/repeated", NormalizedURL: "https://example.invalid/repeated", URLHash: "benchmark", SourceDomain: "example.invalid", Title: strings.Join(words, " "), IngestedAt: time.Now().UTC()}
	if err := s.UpsertArticleHit(ctx, in); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := s.UpsertArticleHit(ctx, in); err != nil {
			b.Fatal(err)
		}
	}
}

func TestRepeatedEffectsAvoidUnchangedWrites(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.UpsertNegativeRule(ctx, model.NegativeRule{Pattern: "cpu", Penalty: 1, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	topic := addTopic(t, s, "cpu")
	id := hit(t, s, "https://example.invalid/cpu", "cpu story", topic, time.Time{})
	_, before := statusScore(t, s, id)
	for _, sql := range []string{`CREATE TABLE effect_writes(n INTEGER NOT NULL)`, `INSERT INTO effect_writes VALUES(0)`, `CREATE TRIGGER audit_effect_insert AFTER INSERT ON article_rule_effects BEGIN UPDATE effect_writes SET n=n+1; END`, `CREATE TRIGGER audit_effect_update AFTER UPDATE ON article_rule_effects BEGIN UPDATE effect_writes SET n=n+1; END`} {
		if _, err := s.db.Exec(sql); err != nil {
			t.Fatal(err)
		}
	}
	hit(t, s, "https://example.invalid/cpu", "cpu story", topic, time.Time{})
	var writes, count int
	if err := s.db.QueryRow(`SELECT n FROM effect_writes`).Scan(&writes); err != nil {
		t.Fatal(err)
	}
	if writes != 0 {
		t.Fatalf("unchanged rule effects rewritten %d times", writes)
	}
	if err := s.db.QueryRow(`SELECT applied_count FROM negative_rules`).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	_, after := statusScore(t, s, id)
	if after != before {
		t.Fatal(before, after)
	}
	// An existing penalty not counted yet must still acquire its first count.
	if _, err := s.db.Exec(`UPDATE article_rule_effects SET counted=0`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE negative_rules SET applied_count=0`); err != nil {
		t.Fatal(err)
	}
	hit(t, s, "https://example.invalid/cpu", "cpu story", topic, time.Time{})
	if err := s.db.QueryRow(`SELECT applied_count FROM negative_rules`).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	hit(t, s, "https://example.invalid/cpu", "cpu story", topic, time.Time{})
	if err := s.db.QueryRow(`SELECT applied_count FROM negative_rules`).Scan(&count); err != nil || count != 1 {
		t.Fatal("double count", count, err)
	}
	if _, err := s.db.Exec(`UPDATE article_rule_effects SET counted=0`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE negative_rules SET applied_count=0,enabled=0`); err != nil {
		t.Fatal(err)
	}
	hit(t, s, "https://example.invalid/cpu", "cpu story", topic, time.Time{})
	var penalty float64
	if err := s.db.QueryRow(`SELECT penalty FROM article_rule_effects`).Scan(&penalty); err != nil || penalty != 0 {
		t.Fatal("stale uncounted penalty", penalty, err)
	}
}

func TestStorySourceBatchBoundsOrderAndMetadata(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	topic := addTopic(t, s, "news")
	var cards []model.Article
	wants := make(map[int64][]int64)
	published := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	for _, n := range []int{0, 1, 20, 21} {
		title := fmt.Sprintf("Independent group size %d", n)
		id := hit(t, s, fmt.Sprintf("https://example.invalid/group%d", n), title, topic, time.Time{})
		for i := 0; i < n; i++ {
			pub := time.Time{}
			if i == 0 {
				pub = published
			}
			alt := hit(t, s, fmt.Sprintf("https://example.invalid/group%d/alt%d", n, i), title, topic, pub)
			if _, err := s.db.Exec(`UPDATE articles SET score=50-?,thumbnail_url=CASE WHEN ?=0 THEN 'https://www.startpage.com/av/proxy-image?piurl=https%3A%2F%2Fwww.bing.com%2Fth%3Fid%3Dtest' ELSE '' END WHERE id=?`, i, i, alt); err != nil {
				t.Fatal(err)
			}
			if i < 20 {
				wants[id] = append(wants[id], alt)
			}
		}
		// Exclude handled/manual/score/legacy sources even when ranked higher.
		for i, state := range []string{"seen", "read", "useful", "manual", "score", "legacy"} {
			bad := hit(t, s, fmt.Sprintf("https://example.invalid/group%d/bad%d", n, i), title, topic, time.Time{})
			status, reason := state, ""
			if i >= 3 {
				status, reason = "hidden", state
			}
			if _, err := s.db.Exec(`UPDATE articles SET status=?,hidden_reason=?,score=999 WHERE id=?`, status, reason, bad); err != nil {
				t.Fatal(err)
			}
		}
		cards = append(cards, model.Article{ID: id, StoryKey: dedupeTitleKey(title, 50)})
	}
	cards = append(cards, model.Article{ID: 99999, StoryKey: "", ThumbnailURL: "https://example.invalid/original"})
	if err := s.fillStorySources(ctx, cards); err != nil {
		t.Fatal(err)
	}
	for _, card := range cards {
		want := wants[card.ID]
		if len(card.Sources) != len(want) {
			t.Fatalf("id %d got %d want %d", card.ID, len(card.Sources), len(want))
		}
		for i, source := range card.Sources {
			if source.ID != want[i] || source.ID == card.ID {
				t.Fatal("order/cross-group mismatch", card.ID, source.ID, want)
			}
			if i == 0 {
				if !source.PublishedAt.Equal(published) || source.ThumbnailURL != "https://www.bing.com/th?id=test" || card.ThumbnailURL != source.ThumbnailURL {
					t.Fatal("metadata not preserved", card)
				}
			} else if !source.PublishedAt.IsZero() {
				t.Fatal("invented source date")
			}
		}
	}
	// A representative image still wins and query cancellation releases the connection.
	one := []model.Article{{ID: cards[1].ID, StoryKey: cards[1].StoryKey, ThumbnailURL: "https://example.invalid/own"}}
	if err := s.fillStorySources(ctx, one); err != nil || one[0].ThumbnailURL != "https://example.invalid/own" {
		t.Fatal(one, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.fillStorySources(canceled, one); err == nil {
		t.Fatal("canceled source query succeeded")
	}
	if err := s.db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestStorySourceBatchRetainsHistoricalClocks(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	s.ConfigureFreshness(30, 7, 36)
	topic := addTopic(t, s, "news")
	now := time.Now().UTC().Truncate(time.Millisecond)
	id := hitAt(t, s, "https://example.invalid/main", "Shared long-lived headline", topic, time.Time{}, now)
	for i := 0; i < 25; i++ {
		hitAt(t, s, fmt.Sprintf("https://example.invalid/copy/%d", i), "Shared long-lived headline", topic, time.Time{}, now)
	}
	oldest := now.Add(-90 * 24 * time.Hour)
	old := hitAt(t, s, "https://example.invalid/old", "Shared long-lived headline", topic, time.Time{}, oldest)
	if _, err := s.db.Exec(`UPDATE articles SET status='hidden',hidden_reason='score',score=-100,archived_at=CURRENT_TIMESTAMP WHERE id=?`, old); err != nil {
		t.Fatal(err)
	}
	cards, err := s.FetchTopUnread(ctx, 10, 1)
	if err != nil || len(cards) != 0 {
		t.Fatal("historical clock excluded", cards, err)
	}
	s.ConfigureFreshness(0, 7, 36)
	cards, err = s.FetchTopUnread(ctx, 10, 1)
	if err != nil || len(cards) != 1 || cards[0].ID != id || len(cards[0].Sources) != 20 || !cards[0].FirstSeenAt.Equal(oldest) {
		t.Fatal(cards, err)
	}
}

func TestStorySourceBatchQueryPlan(t *testing.T) {
	s := testStore(t)
	var tuples []string
	var args []any
	for i := 0; i < 100; i++ {
		tuples = append(tuples, "(?,?,?)")
		args = append(args, i, fmt.Sprintf("group%06d", i), i+1)
	}
	for i, query := range []struct {
		sql  string
		args []any
	}{{feedSelectionSQL, []any{0, 0, 0, 0, 1, 100}}, {fmt.Sprintf(storySourcesSQL, strings.Join(tuples, ",")), args}} {
		rows, err := s.db.QueryContext(t.Context(), "EXPLAIN QUERY PLAN "+query.sql, query.args...)
		if err != nil {
			t.Fatal(err)
		}
		indexed := false
		for rows.Next() {
			var a, b, c int
			var detail string
			if err := rows.Scan(&a, &b, &c, &detail); err != nil {
				t.Fatal(err)
			}
			t.Log(detail)
			if strings.Contains(detail, "SEARCH a USING INDEX idx_articles_story") {
				indexed = true
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
		if i == 1 && !indexed {
			t.Fatal("source batch lost indexed story-key lookup")
		}
	}
}
