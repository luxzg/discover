package store

import (
	"context"
	"database/sql"
	"math"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"discover/internal/db"
	"discover/internal/model"
	"discover/internal/urlnorm"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	s := New(d)
	if err := s.Prepare(context.Background(), 50); err != nil {
		t.Fatal(err)
	}
	return s
}
func addTopic(t *testing.T, s *Store, query string) int64 {
	t.Helper()
	if err := s.UpsertTopic(context.Background(), model.Topic{Query: query, Weight: 2, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := s.db.QueryRow(`SELECT id FROM topics ORDER BY id DESC LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
func hit(t *testing.T, s *Store, u, title string, topic int64, published time.Time) int64 {
	t.Helper()
	norm, hash, domain, err := urlnorm.Normalize(u)
	if err != nil {
		t.Fatal(err)
	}
	err = s.UpsertArticleHit(context.Background(), UpsertArticleInput{URL: u, NormalizedURL: norm, URLHash: hash, SourceDomain: domain, Title: title, TopicID: topic, Engines: 1, SearxScore: 1, PublishedAt: published, IngestedAt: time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := s.db.QueryRow(`SELECT id FROM articles WHERE url_hash=?`, hash).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
func statusScore(t *testing.T, s *Store, id int64) (string, float64) {
	t.Helper()
	var state string
	var score float64
	if err := s.db.QueryRow(`SELECT status,score FROM articles WHERE id=?`, id).Scan(&state, &score); err != nil {
		t.Fatal(err)
	}
	return state, score
}

func TestPublishedDateRoundTrip(t *testing.T) {
	s := testStore(t)
	topic := addTopic(t, s, "cpu")
	date := time.Date(2026, 9, 8, 14, 32, 10, 123456789, time.UTC)
	hit(t, s, "https://example.com/dated", "Dated story", topic, date)
	hit(t, s, "https://example.com/dated", "Dated story", topic, time.Time{})
	hit(t, s, "https://example.com/unknown", "Unknown date", topic, time.Time{})
	cards, err := s.FetchTopUnread(context.Background(), 10, -100)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range cards {
		if a.Title == "Dated story" && !a.PublishedAt.Equal(date) {
			t.Fatalf("date lost: %v", a.PublishedAt)
		}
		if a.Title == "Unknown date" && !a.PublishedAt.IsZero() {
			t.Fatal("unknown publication invented")
		}
	}
	if len(cards) != 2 {
		t.Fatalf("cards=%d", len(cards))
	}
}

func TestLegacyDateAndScoreMigration(t *testing.T) {
	s := testStore(t)
	topic := addTopic(t, s, "cpu")
	id := hit(t, s, "https://example.com/article?id=1", "cpu legacy date", topic, time.Time{})
	published := time.Date(2026, 5, 20, 7, 2, 0, 0, time.UTC)
	if _, err := s.db.Exec(`UPDATE articles SET published_at=?,ingested_at=?,score=123,score_base=NULL WHERE id=?`, published.String(), published.Add(time.Hour).String(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM article_evidence`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM app_settings WHERE key='story_model_version'`); err != nil {
		t.Fatal(err)
	}
	if err := s.Prepare(context.Background(), 50); err != nil {
		t.Fatal(err)
	}
	cards, err := s.FetchTopUnread(context.Background(), 10, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 || !cards[0].PublishedAt.Equal(published) || cards[0].Score != 123 {
		t.Fatalf("migration changed history: %+v", cards)
	}
	hit(t, s, "https://example.com/article?id=1", "cpu legacy date", topic, time.Time{})
	_, score := statusScore(t, s, id)
	if math.Abs(score-123) > 1e-8 {
		t.Fatalf("identical legacy evidence changed score: %v", score)
	}
	hit(t, s, "https://example.com/article?id=2", "Different story", topic, time.Time{})
	var n int
	s.db.QueryRow(`SELECT count(*) FROM articles`).Scan(&n)
	if n != 2 {
		t.Fatalf("query identities merged: %d", n)
	}
}

func TestDedupeIsRepeatableAndPreservesWinner(t *testing.T) {
	s := testStore(t)
	topic := addTopic(t, s, "news")
	a := hit(t, s, "https://one.example/a", "Same story!", topic, time.Time{})
	b := hit(t, s, "https://two.example/b", "same STORY", topic, time.Time{})
	if _, err := s.HideAllUnreadTitleDuplicates(context.Background(), 50); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.HideAllUnreadTitleDuplicates(context.Background(), 50); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	state, _ := statusScore(t, s, a)
	if state != "unread" {
		t.Fatalf("retained article hidden: %s", state)
	}
	state, _ = statusScore(t, s, b)
	if state != "hidden" {
		t.Fatal("duplicate not hidden")
	}
	total, err := s.DedupeHiddenTotal(context.Background())
	if err != nil || total != 1 {
		t.Fatalf("counter %d %v", total, err)
	}
	cards, err := s.FetchTopUnread(context.Background(), 10, 1)
	if err != nil || len(cards) != 1 || len(cards[0].Sources) != 1 {
		t.Fatalf("story sources: %+v %v", cards, err)
	}
	hit(t, s, "https://one.example/a", "Same story!", topic, time.Time{})
	if _, err := s.HideIngestTitleDuplicates(context.Background(), time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC), 50); err != nil {
		t.Fatal(err)
	}
	state, _ = statusScore(t, s, a)
	if state != "unread" {
		t.Fatal("reingest poisoned retained winner")
	}
	if err := s.MarkRead(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	c := hit(t, s, "https://three.example/c", "Same story", topic, time.Time{})
	if _, err := s.HideAllUnreadTitleDuplicates(context.Background(), 50); err != nil {
		t.Fatal(err)
	}
	state, _ = statusScore(t, s, c)
	if state != "hidden" {
		t.Fatal("read history not respected")
	}
}

func TestLowScoreHiddenDoesNotPoisonStory(t *testing.T) {
	s := testStore(t)
	topic := addTopic(t, s, "news")
	a := hit(t, s, "https://one.example/a", "Same title", topic, time.Time{})
	b := hit(t, s, "https://two.example/b", "Same title", topic, time.Time{})
	s.db.Exec(`UPDATE articles SET score=-5 WHERE id=?`, b)
	if _, err := s.HideUnreadBelowScore(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.HideAllUnreadTitleDuplicates(context.Background(), 50); err != nil {
		t.Fatal(err)
	}
	state, _ := statusScore(t, s, a)
	if state != "unread" {
		t.Fatal("low score copy suppressed good article")
	}
}

func TestStableEvidenceAndRuleEdits(t *testing.T) {
	s := testStore(t)
	topic := addTopic(t, s, "cpu")
	a := hit(t, s, "https://example.com/cpu", "cpu review", topic, time.Time{})
	_, initial := statusScore(t, s, a)
	for i := 0; i < 5; i++ {
		hit(t, s, "https://example.com/cpu", "cpu review", topic, time.Time{})
	}
	_, score := statusScore(t, s, a)
	if score != initial {
		t.Fatalf("frequency inflation: %v -> %v", initial, score)
	}
	r := model.NegativeRule{Pattern: "cpu", Penalty: 1, Enabled: true}
	if err := s.UpsertNegativeRule(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	s.db.QueryRow(`SELECT id FROM negative_rules`).Scan(&r.ID)
	r.Penalty = 100
	if err := s.UpsertNegativeRule(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	_, score = statusScore(t, s, a)
	if score != initial-100 {
		t.Fatalf("penalty %v", score)
	}
	r.Pattern = "absent"
	if err := s.UpsertNegativeRule(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	_, score = statusScore(t, s, a)
	if score != initial {
		t.Fatalf("rename left old penalty: %v", score)
	}
	var n int
	s.db.QueryRow(`SELECT count(*) FROM negative_rules`).Scan(&n)
	if n != 1 {
		t.Fatalf("rename inserted rule: %d", n)
	}
	r.Pattern = "cpu"
	s.UpsertNegativeRule(context.Background(), r)
	if err := s.DeleteNegativeRule(context.Background(), r.ID); err != nil {
		t.Fatal(err)
	}
	_, score = statusScore(t, s, a)
	if score != initial {
		t.Fatalf("delete left penalty: %v", score)
	}
}

func TestRuleAndScoreChangesRollbackTogether(t *testing.T) {
	s := testStore(t)
	topic := addTopic(t, s, "cpu")
	hit(t, s, "https://example.com/a", "cpu", topic, time.Time{})
	_, err := s.db.Exec(`CREATE TRIGGER fail_score BEFORE UPDATE OF score ON articles BEGIN SELECT RAISE(ABORT,'test failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertNegativeRule(context.Background(), model.NegativeRule{Pattern: "cpu", Penalty: 5, Enabled: true}); err == nil {
		t.Fatal("expected transaction failure")
	}
	var n int
	s.db.QueryRow(`SELECT count(*) FROM negative_rules`).Scan(&n)
	if n != 0 {
		t.Fatal("failed adjustment committed rule")
	}
}

func TestTopicRenameAndCascades(t *testing.T) {
	s := testStore(t)
	topic := addTopic(t, s, "wrong")
	a := hit(t, s, "https://example.com/a", "story", topic, time.Time{})
	if err := s.UpsertTopic(context.Background(), model.Topic{ID: topic, Query: "correct", Weight: 3, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	var n int
	s.db.QueryRow(`SELECT count(*) FROM topics`).Scan(&n)
	if n != 1 {
		t.Fatal("rename inserted topic")
	}
	if err := s.DeleteTopic(context.Background(), topic); err != nil {
		t.Fatal(err)
	}
	s.db.QueryRow(`SELECT count(*) FROM article_topics WHERE article_id=?`, a).Scan(&n)
	if n != 0 {
		t.Fatal("foreign key cascade disabled")
	}
}

func TestPositiveActionsIdempotent(t *testing.T) {
	s := testStore(t)
	topic := addTopic(t, s, "news")
	a := hit(t, s, "https://example.com/a", "story", topic, time.Time{})
	_, original := statusScore(t, s, a)
	for i := 0; i < 3; i++ {
		if err := s.MarkIDStatus(context.Background(), a, model.StatusUseful, 1); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.MarkRead(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	state, score := statusScore(t, s, a)
	if state != "useful" || score != original+1 {
		t.Fatalf("useful lost or inflated: %s %v", state, score)
	}
	if err := s.MarkIDStatus(context.Background(), a, model.StatusHidden, -2.5); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkRead(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	state, _ = statusScore(t, s, a)
	if state != "hidden" {
		t.Fatal("stale click undid hide")
	}
}

func TestThumbnailDecodersAndUnicodeKey(t *testing.T) {
	raw := "https://imgs.search.brave.com/signature/rs:fit:200:200:1:0/g:ce/aHR0cHM6Ly93d3cu/bm90ZWJvb2tjaGVj/ay5uZXQvZmlsZWFk/bWluL05vdGVib29r/cy9OZXdzL19uYzUv/SW50ZWwtVGl0YW4t/TGFrZS1SYXplci1M/YWtlLUhhbW1lci1M/YWtlLWFuZC1Ob3Zh/LUxha2UtQVgtQ1BV/LWFyY2hpdGVjdHVy/ZXMtaGF2ZS1sZWFr/ZWQtb25saW5lLmpw/Zw"
	want := "https://www.notebookcheck.net/fileadmin/Notebooks/News/_nc5/Intel-Titan-Lake-Razer-Lake-Hammer-Lake-and-Nova-Lake-AX-CPU-architectures-have-leaked-online.jpg"
	if got := displayThumbnailURL(raw); got != want {
		t.Fatalf("decoded=%s", got)
	}
	if got := dedupeTitleKey("ŽžŽžŽžŽžŽžŽž", 10); len([]rune(got)) != 10 {
		t.Fatalf("not rune cutoff: %q", got)
	}
	if parseDBTime(sql.NullTime{}).IsZero() != true {
		t.Fatal("unexpected date")
	}
}
