package store

import (
	"context"
	"testing"
	"time"

	"discover/internal/model"
)

func TestFirstSeenImmutableAndEvidenceWindow(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	topic := addTopic(t, s, "cpu")
	start := time.Now().UTC().Add(-37 * time.Hour)
	id := hitAt(t, s, "https://a.example/one", "A shared CPU story", topic, time.Time{}, start)
	_, before := statusScore(t, s, id)
	frozenScore := before
	second := addTopic(t, s, "review")
	hitAt(t, s, "https://a.example/one", "A shared CPU story", second, time.Time{}, start.Add(36*time.Hour))
	_, after := statusScore(t, s, id)
	if before != after {
		t.Fatal("late topic inflated score", before, after)
	}
	if err := s.UpsertArticleHit(ctx, UpsertArticleInput{URLHash: "", Title: "invalid"}); err == nil {
		t.Fatal("invalid hit accepted")
	}
	var hash string
	if err := s.db.QueryRow(`SELECT url_hash FROM articles WHERE id=?`, id).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertArticleHit(ctx, UpsertArticleInput{URL: "https://a.example/one", URLHash: hash, Title: "A shared CPU story", TopicID: topic, Engines: 99, SearxScore: 999, IngestedAt: start.Add(36 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	_, after = statusScore(t, s, id)
	if after != frozenScore {
		t.Fatal("late stronger evidence boosted score", after)
	}
	var first, ing any
	if err := s.db.QueryRow(`SELECT first_seen_at,ingested_at FROM articles WHERE id=?`, id).Scan(&first, &ing); err != nil {
		t.Fatal(err)
	}
	if !parseDBTime(first).Equal(start) || !parseDBTime(ing).Equal(start.Add(36*time.Hour)) {
		t.Fatal(first, ing)
	}
	if _, err := s.db.Exec(`UPDATE articles SET first_seen_at=? WHERE id=?`, dbTimestamp(time.Now()), id); err == nil {
		t.Fatal("mutable first seen")
	}
	copy := hitAt(t, s, "https://b.example/two", "A shared CPU story", second, time.Time{}, start.Add(37*time.Hour))
	_, score := statusScore(t, s, copy)
	if score != 0 {
		t.Fatal("syndicated copy restarted evidence window", score)
	}
	fresh := hitAt(t, s, "https://a.example/new", "Another story", topic, time.Time{}, time.Now())
	_, before = statusScore(t, s, fresh)
	hitAt(t, s, "https://a.example/new", "Another story", second, time.Time{}, time.Now().Add(time.Hour))
	_, after = statusScore(t, s, fresh)
	if after <= before {
		t.Fatal("early new topic did not boost")
	}
	if err := s.UpsertNegativeRule(ctx, model.NegativeRule{Pattern: "cpu", Penalty: 1, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	_, after = statusScore(t, s, id)
	if after != frozenScore-1 {
		t.Fatal("late rule edit must still apply", after, frozenScore)
	}
}

func TestAgeArchiveReversibleAndStoryClock(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	s.ConfigureFreshness(30, 7, 36)
	topic := addTopic(t, s, "news")
	now := time.Now().UTC()
	old := hitAt(t, s, "https://a.example/old", "Old syndicated story", topic, time.Time{}, now.Add(-60*24*time.Hour))
	hitAt(t, s, "https://b.example/copy", "Old syndicated story", topic, time.Time{}, now)
	fresh := hitAt(t, s, "https://a.example/fresh", "Fresh headline", topic, time.Time{}, now)
	handled := hitAt(t, s, "https://a.example/read", "Handled headline", topic, time.Time{}, now.Add(-60*24*time.Hour))
	if err := s.MarkRead(ctx, handled); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE articles SET score=200 WHERE id=?`, old); err != nil {
		t.Fatal(err)
	}
	cards, err := s.FetchTopUnread(ctx, 10, 1)
	if err != nil || len(cards) != 1 || cards[0].ID != fresh {
		t.Fatalf("stale story returned %+v %v", cards, err)
	}
	preview, err := s.PreviewArchive(ctx, 30)
	if err != nil || preview.Candidates != 2 {
		t.Fatal(preview, err)
	}
	stats, err := s.ArchiveOldUnread(ctx, 30, true)
	if err != nil || stats.Archived != 2 {
		t.Fatal(stats, err)
	}
	topicCounts, err := s.TopicStats(ctx)
	if err != nil || topicCounts[topic].Unread != 1 {
		t.Fatal("archived rows counted as active unread", topicCounts, err)
	}
	preview, err = s.PreviewArchive(ctx, 90)
	if err != nil || preview.Candidates != 0 || preview.Restorable != 2 {
		t.Fatal("restore preview differs from eligible archives", preview, err)
	}
	stats, err = s.ArchiveOldUnread(ctx, 90, true)
	if err != nil || stats.Restored != 2 {
		t.Fatal(stats, err)
	}
	cards, err = s.FetchTopUnread(ctx, 10, 1)
	if err != nil || len(cards) != 2 {
		t.Fatal(cards, err)
	}
	if state, _ := statusScore(t, s, handled); state != "read" {
		t.Fatal("archive touched handled history")
	}
	if n, err := s.DedupeHiddenTotal(ctx); err != nil || n != 0 {
		t.Fatal("archive counted as dedupe", n, err)
	}
}

func TestFreshnessRankDoesNotRewriteScores(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	s.ConfigureFreshness(30, 7, 36)
	topic := addTopic(t, s, "news")
	now := time.Now().UTC()
	old := hitAt(t, s, "https://a.example/older", "Older higher raw score", topic, time.Time{}, now.Add(-21*24*time.Hour))
	fresh := hitAt(t, s, "https://a.example/latest", "Latest headline", topic, time.Time{}, now)
	if _, err := s.db.Exec(`UPDATE articles SET score=CASE WHEN id=? THEN 100 ELSE 40 END`, old); err != nil {
		t.Fatal(err)
	}
	cards, err := s.FetchTopUnread(ctx, 10, 1)
	if err != nil || len(cards) != 2 || cards[0].ID != fresh || cards[1].ID != old || cards[1].Score != 100 || cards[1].FeedRank >= cards[0].FeedRank {
		t.Fatal("freshness ordering or raw score changed", cards, err)
	}
	_, score := statusScore(t, s, old)
	if score != 100 {
		t.Fatal("freshness rewrote legacy score", score)
	}
}

func TestDomainReportHideOverridesClick(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	topic := addTopic(t, s, "news")
	a := hit(t, s, "https://a.example/one", "one", topic, time.Time{})
	b := hit(t, s, "https://a.example/two", "two", topic, time.Time{})
	c := hit(t, s, "https://a.example/three", "three", topic, time.Time{})
	d := hit(t, s, "https://a.example/four", "four", topic, time.Time{})
	for _, id := range []int64{a, b, d} {
		if err := s.MarkRead(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.MarkIDStatus(ctx, a, model.StatusUseful, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkIDStatus(ctx, b, model.StatusHidden, -2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE articles SET status='hidden',hidden_reason='duplicate' WHERE id=?`, c); err != nil {
		t.Fatal(err)
	}
	rows, err := s.DomainReport(ctx)
	if err != nil || len(rows) != 1 || rows[0].Positive != 2 || rows[0].Read != 2 || rows[0].Useful != 1 || rows[0].Hidden != 1 {
		t.Fatal(rows, err)
	}
}

func TestArchivePreviewRestoresRowsNotFirstPageRank(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	s.ConfigureFreshness(30, 7, 36)
	topic := addTopic(t, s, "news")
	now := time.Now().UTC()
	old := hitAt(t, s, "https://a.example/older", "Restored older story", topic, time.Time{}, now.Add(-60*24*time.Hour))
	copy := hitAt(t, s, "https://b.example/copy", "Restored older story", topic, time.Time{}, now.Add(-60*24*time.Hour))
	fresh := hitAt(t, s, "https://a.example/latest", "Fresh current story", topic, time.Time{}, now)
	expired := hitAt(t, s, "https://a.example/expired", "Older than ninety days", topic, time.Time{}, now.Add(-120*24*time.Hour))
	manual := hitAt(t, s, "https://a.example/manual", "Deliberately hidden story", topic, time.Time{}, now.Add(-60*24*time.Hour))
	if err := s.MarkIDStatus(ctx, manual, model.StatusHidden, -1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE articles SET score=CASE WHEN id=? THEN 150 WHEN id=? THEN 85 WHEN id=? THEN 200 ELSE 20 END`, old, fresh, expired); err != nil {
		t.Fatal(err)
	}
	if _, err := s.HideAllUnreadTitleDuplicates(ctx, 50); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ArchiveOldUnread(ctx, 30, true); err != nil {
		t.Fatal(err)
	}
	preview, err := s.PreviewArchive(ctx, 90)
	if err != nil || preview.Restorable != 2 || preview.Candidates != 0 {
		t.Fatal(preview, err)
	}
	active, err := s.FeedAgeLimit(ctx)
	if err != nil || active != 30 {
		t.Fatal("preview changed active age limit", active, err)
	}
	stats, err := s.ArchiveOldUnread(ctx, 90, true)
	if err != nil || stats.Restored != preview.Restorable {
		t.Fatal("restore did not match preview", stats, preview, err)
	}
	cards, err := s.FetchTopUnread(ctx, 1, 10)
	if err != nil || len(cards) != 1 || cards[0].ID != fresh {
		t.Fatal("older raw score unexpectedly topped fresh story", cards, err)
	}
	cards, err = s.FetchTopUnread(ctx, 10, 10)
	if err != nil || len(cards) != 2 || cards[1].ID != old || cards[1].Score != 150 {
		t.Fatal("restored representative missing from later feed", cards, err)
	}
	if state, _ := statusScore(t, s, copy); state != "hidden" {
		t.Fatal("restore undid duplicate state")
	}
	preview, err = s.PreviewArchive(ctx, 0)
	if err != nil || preview.Candidates != 0 || preview.Restorable != 1 {
		t.Fatal("disabling limit preview", preview, err)
	}
	stats, err = s.ArchiveOldUnread(ctx, 0, true)
	if err != nil || stats.Restored != preview.Restorable {
		t.Fatal(stats, err)
	}
	if state, _ := statusScore(t, s, manual); state != "hidden" {
		t.Fatal("restore undid deliberate hide")
	}
}

func TestDateOnlyMetadataAndRetryCooldown(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	topic := addTopic(t, s, "news")
	id := hitAt(t, s, "https://a.example/date", "Needs date", topic, time.Time{}, time.Now())
	if _, err := s.db.Exec(`UPDATE articles SET thumbnail_url='https://a.example/image' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	rows, err := s.ListMetadataCandidates(ctx, 60, 1, 40, 40)
	if err != nil || len(rows) != 1 || !rows[0].NeedDate || rows[0].NeedThumbnail {
		t.Fatal(rows, err)
	}
	image, dated, err := s.RecordMetadata(ctx, id, "", time.Now().Add(-time.Hour))
	if err != nil || image || !dated {
		t.Fatal(image, dated, err)
	}
	rows, err = s.ListMetadataCandidates(ctx, 60, 1, 40, 40)
	if err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
}
