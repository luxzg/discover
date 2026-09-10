package store

import (
	"context"
	"database/sql"
	"discover/internal/db"
	"discover/internal/model"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOldSchemaUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = old.Exec(`CREATE TABLE articles (
 id INTEGER PRIMARY KEY AUTOINCREMENT,url TEXT NOT NULL,normalized_url TEXT NOT NULL,url_hash TEXT NOT NULL UNIQUE,
 title TEXT NOT NULL,content TEXT NOT NULL DEFAULT '',thumbnail_url TEXT NOT NULL DEFAULT '',source_domain TEXT NOT NULL DEFAULT '',
 published_at DATETIME,ingested_at DATETIME NOT NULL,status TEXT NOT NULL DEFAULT 'unread',score REAL NOT NULL DEFAULT 0,
 hit_count INTEGER NOT NULL DEFAULT 0,engine_count INTEGER NOT NULL DEFAULT 0,searx_score REAL NOT NULL DEFAULT 0,
 last_seen_at DATETIME,created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP);
 INSERT INTO articles(url,normalized_url,url_hash,title,published_at,ingested_at,score) VALUES
 ('https://example.com/article?id=1','https://example.com/article','oldhash','Legacy title','2026-05-20 07:02:00 +0000 UTC','2026-05-25 10:22:33.96586637 +0000 UTC',123);
 INSERT INTO articles(url,normalized_url,url_hash,title,ingested_at,status) VALUES
 ('https://example.com/hidden','https://example.com/hidden','hiddenhash','Hidden title','2026-05-25 10:22:33.96586637 +0000 UTC','hidden');`)
	if err != nil {
		t.Fatal(err)
	}
	old.Close()
	upgraded, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	s := New(upgraded)
	if err := s.Prepare(context.Background(), 50); err != nil {
		t.Fatal(err)
	}
	cards, err := s.FetchTopUnread(context.Background(), 10, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 || cards[0].Score != 123 || cards[0].PublishedAt.Day() != 20 || !strings.Contains(cards[0].NormalizedURL, "id=1") {
		t.Fatalf("%+v", cards)
	}
	var reason string
	if err := s.db.QueryRow(`SELECT hidden_reason FROM articles WHERE id=2`).Scan(&reason); err != nil || reason != "legacy" {
		t.Fatalf("%s %v", reason, err)
	}
}

func TestRuleHitCountsAndAtomicHide(t *testing.T) {
	s := testStore(t)
	topic := addTopic(t, s, "cpu")
	a := hit(t, s, "https://example.com/a", "cpu one", topic, time.Time{})
	b := hit(t, s, "https://example.com/b", "cpu two", topic, time.Time{})
	_, initial := statusScore(t, s, a)
	r := model.NegativeRule{Pattern: "cpu", Penalty: 1, Enabled: true}
	ids, err := s.HideWithRule(context.Background(), a, r)
	if err != nil || len(ids) != 2 {
		t.Fatalf("%v %v", ids, err)
	}
	_, score := statusScore(t, s, a)
	if score != initial-1 {
		t.Fatal("double penalty", score)
	}
	rules, _ := s.ListNegativeRules(context.Background())
	r = rules[0]
	for i := 0; i < 3; i++ {
		hit(t, s, "https://example.com/b", "cpu two", topic, time.Time{})
		if err := s.UpsertNegativeRule(context.Background(), r); err != nil {
			t.Fatal(err)
		}
	}
	rules, _ = s.ListNegativeRules(context.Background())
	if rules[0].AppliedCount != 2 {
		t.Fatal(rules)
	}
	if _, err := s.HideWithRule(context.Background(), 9999, model.NegativeRule{Pattern: "another", Penalty: 5, Enabled: true}); err == nil {
		t.Fatal("invalid id accepted")
	}
	rules, _ = s.ListNegativeRules(context.Background())
	if len(rules) != 1 {
		t.Fatal("failed hide saved rule")
	}
	state, _ := statusScore(t, s, b)
	if state != "unread" {
		t.Fatal("rule should rescore other unread, not change status directly")
	}
}

func TestRuleMatchUsesPersistedURL(t *testing.T) {
	s := testStore(t)
	topic := addTopic(t, s, "news")
	id := hit(t, s, "https://example.com/a?utm_campaign=blocked", "title", topic, time.Time{})
	if err := s.UpsertNegativeRule(context.Background(), model.NegativeRule{Pattern: "blocked", Penalty: 2, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	_, before := statusScore(t, s, id)
	hit(t, s, "https://example.com/a", "title", topic, time.Time{})
	if err := s.RecomputeUnread(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, after := statusScore(t, s, id)
	if before != after {
		t.Fatalf("%f != %f", before, after)
	}
}

func TestStoryReindexSplitAndCounter(t *testing.T) {
	s := testStore(t)
	topic := addTopic(t, s, "news")
	prefix := strings.Repeat("a", 50)
	hit(t, s, "https://example.com/one", prefix+"one", topic, time.Time{})
	b := hit(t, s, "https://example.com/two", prefix+"two", topic, time.Time{})
	if _, err := s.HideAllUnreadTitleDuplicates(context.Background(), 50); err != nil {
		t.Fatal(err)
	}
	_, before := statusScore(t, s, b)
	if err := s.UpsertNegativeRule(context.Background(), model.NegativeRule{Pattern: "example.com/two", Penalty: 2, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.Prepare(context.Background(), 60); err != nil {
		t.Fatal(err)
	}
	state, after := statusScore(t, s, b)
	if state != "unread" || after != before-2 {
		t.Fatal("split story stranded")
	}
	if err := s.Prepare(context.Background(), 50); err != nil {
		t.Fatal(err)
	}
	n, err := s.DedupeHiddenTotal(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("counter %d %v", n, err)
	}
}

func TestDerivedStoryReconcilesRuleAndHeadlineChanges(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	topic := addTopic(t, s, "news")
	a := hit(t, s, "https://one.example/a", "A shared headline", topic, time.Time{})
	b := hit(t, s, "https://two.example/b", "A shared headline", topic, time.Time{})
	if _, err := s.HideAllUnreadTitleDuplicates(ctx, 50); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertNegativeRule(ctx, model.NegativeRule{Pattern: "one.example", Penalty: 100, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	cards, err := s.FetchTopUnread(ctx, 10, 1)
	if err != nil || len(cards) != 1 || cards[0].ID != b {
		t.Fatalf("representative not replaced: %+v %v", cards, err)
	}
	hit(t, s, "https://one.example/a", "An entirely different headline", topic, time.Time{})
	state, _ := statusScore(t, s, a)
	if state != "unread" {
		t.Fatal("changed headline remained a duplicate", state)
	}
}

func TestSeenFromStaleDuplicateSuppressesStory(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	topic := addTopic(t, s, "news")
	hit(t, s, "https://one.example/a", "A shared headline", topic, time.Time{})
	b := hit(t, s, "https://two.example/b", "A shared headline", topic, time.Time{})
	if _, err := s.HideAllUnreadTitleDuplicates(ctx, 50); err != nil {
		t.Fatal(err)
	}
	if err := s.markSeen(ctx, []int64{b}); err != nil {
		t.Fatal(err)
	}
	cards, err := s.FetchTopUnread(ctx, 10, 0)
	if err != nil || len(cards) != 0 {
		t.Fatalf("handled story returned: %+v %v", cards, err)
	}
}

func TestLegacyReadNewRuleCount(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	topic := addTopic(t, s, "news")
	id := hit(t, s, "https://example.com/a", "Legacy handled news", topic, time.Time{})
	if _, err := s.db.Exec(`UPDATE articles SET status='read',score_base=NULL WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM app_settings WHERE key='story_model_version'`); err != nil {
		t.Fatal(err)
	}
	if err := s.Prepare(ctx, 50); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertNegativeRule(ctx, model.NegativeRule{Pattern: "example.com", Penalty: 2, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	hit(t, s, "https://example.com/a", "Legacy handled news", topic, time.Time{})
	rules, err := s.ListNegativeRules(ctx)
	if err != nil || len(rules) != 1 || rules[0].AppliedCount != 1 {
		t.Fatalf("%+v %v", rules, err)
	}
}

func TestUnknownDateSortingAndRetention(t *testing.T) {
	s := testStore(t)
	topic := addTopic(t, s, "news")
	old := hit(t, s, "https://example.com/old", "old", topic, time.Time{})
	fresh := hit(t, s, "https://example.com/fresh", "fresh", topic, time.Time{})
	now := time.Now().UTC()
	s.db.Exec(`UPDATE articles SET ingested_at=?,score=0 WHERE id=?`, dbTimestamp(now.Add(-48*time.Hour-time.Minute)), old)
	s.db.Exec(`UPDATE articles SET ingested_at=?,score=0 WHERE id=?`, dbTimestamp(now), fresh)
	cards, err := s.FetchTopUnread(context.Background(), 10, -1)
	if err != nil || len(cards) != 2 || cards[0].ID != fresh {
		t.Fatalf("%+v %v", cards, err)
	}
	n, err := s.CullOldUnread(context.Background(), 2, 0)
	if err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
}

func TestHandledHeadlineChangeKeepsHistoricalStory(t *testing.T) {
	for _, action := range []model.ArticleStatus{model.StatusRead, model.StatusHidden} {
		t.Run(string(action), func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			topic := addTopic(t, s, "news")
			a := hit(t, s, "https://example.com/a", "Original shared headline", topic, time.Time{})
			hit(t, s, "https://example.com/b", "Original shared headline", topic, time.Time{})
			if _, err := s.HideAllUnreadTitleDuplicates(ctx, 50); err != nil {
				t.Fatal(err)
			}
			if err := s.markAction(ctx, a, action, 0); err != nil {
				t.Fatal(err)
			}
			hit(t, s, "https://example.com/a", "Publisher completely changed headline", topic, time.Time{})
			cards, err := s.FetchTopUnread(ctx, 10, 0)
			if err != nil || len(cards) != 0 {
				t.Fatalf("handled story resurrected: %+v %v", cards, err)
			}
		})
	}
}
