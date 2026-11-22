package store

import (
	"fmt"
	"testing"
	"time"
)

func TestEventClaimAgreement(t *testing.T) {
	for _, tc := range []struct {
		a, b  string
		match bool
	}{
		{"Qorix RTX 7090 starts shipping", "First Qorix RTX 7090 units reach buyers", true},
		{"First Tessel 82 graphics card units reach buyers", "Tessel 82 GPU begins shipping", true},
		{"Beneath polar ice, scientists find iridium crystals", "Iridium crystals discovered under polar ice", true},
		{"Orbit survey detects iridium beneath northern ice", "The Orbit survey detects iridium beneath the northern ice", true},
		{"Orbit survey detects iridium beneath northern polar ice", "Updated Orbit survey detects iridium beneath northern polar ice", true},
		{"Beneath northern polar ice, Orbit survey detects iridium", "Orbit survey detects iridium beneath northern polar ice", true},
		{"Qorix RTX 7090 starts shipping", "Qorix RTX 7091 starts shipping", false},
		{"Qorix RTX 7090+ starts shipping", "Qorix RTX 7090 starts shipping", false},
		{"Qorix RTX 7090 starts shipping", "Qorix RTX 7090 will start shipping", false},
		{"Qorix RTX 7090 starts shipping", "Qorix RTX 7090 starts shipping: first review", false},
		{"Qorix RTX 7090 price drops to $90", "Qorix RTX 7090 price drops to €90", false},
		{"Qorix RTX 7090 price drops from $100 to $90", "Qorix RTX 7090 price drops from $90 to $100", false},
		{"New GPU starts shipping", "First GPU units reach buyers", false},
	} {
		a, ok := headlineClaim(tc.a)
		b, yes := headlineClaim(tc.b)
		if got := ok && yes && a.block == b.block && a.kind == b.kind && similarClaim(a.subject, b.subject); got != tc.match {
			t.Errorf("%q / %q: %v, %#v %#v", tc.a, tc.b, got, a, b)
		}
	}
}

func TestGroupedFeedAdvance(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	now := time.Now().UTC()
	topic := addTopic(t, s, "hardware")
	add := func(n int, title string, offset time.Duration) int64 {
		return hitAt(t, s, fmt.Sprintf("https://paper%d.example/story", n), title, topic, now.Add(offset), now)
	}
	a := add(1, "Qorix RTX 7090 starts shipping", 0)
	b := add(2, "First Qorix RTX 7090 units reach buyers", time.Hour)
	c := add(3, "Independent Qorix RTX 7090 benchmark", 0)
	if _, err := s.db.Exec("UPDATE articles SET score=CASE id WHEN ? THEN 10 WHEN ? THEN 9 ELSE 8 END", a, b); err != nil {
		t.Fatal(err)
	}
	read := func(group bool) []int64 {
		cards, err := s.fetchView(ctx, 2, 0, group)
		if err != nil {
			t.Fatal(err)
		}
		var ids []int64
		for _, x := range cards {
			ids = append(ids, x.ID)
		}
		return ids
	}
	cards, err := s.FetchGrouped(ctx, 2, 0)
	if err != nil || len(cards) != 2 || cards[0].ID != a || cards[0].RelatedCount != 1 || cards[0].Related[0].ID != b || cards[1].ID != c {
		t.Fatalf("grouped page: %+v %v", cards, err)
	}
	if ids := read(false); len(ids) != 2 || ids[0] != a || ids[1] != b {
		t.Fatal("toggle consumed or regrouped Standard", ids)
	}
	if err := s.MarkRead(ctx, b); err != nil {
		t.Fatal(err)
	}
	if state, _ := statusScore(t, s, a); state != "unread" {
		t.Fatal("related click affected lead", state)
	}
	if err := s.AdvanceFeed(ctx, []int64{a}, map[int64]int64{a: cards[0].EventID}); err != nil {
		t.Fatal(err)
	}
	late := add(4, "Qorix RTX 7090 begins shipping", 2*time.Hour)
	if ids := read(true); len(ids) != 1 || ids[0] != c {
		t.Fatal("advanced event reappeared", ids)
	}
	if ids := read(false); len(ids) != 2 {
		t.Fatal("Standard lost the later unread story", ids)
	}
	if state, _ := statusScore(t, s, late); state != "unread" {
		t.Fatal(state)
	}
	if count, err := s.DedupeHiddenTotal(ctx); err != nil || count != 0 {
		t.Fatal(count, err)
	}
}

func TestEventClockAmbiguityAndCorrection(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	now := time.Now().UTC()
	create := func(n int, offset time.Duration) int64 {
		return hitAt(t, s, fmt.Sprintf("https://source.example/%d", n), fmt.Sprintf("Qorix RTX 7090 %s", []string{"starts shipping", "begins shipping", "now shipping", "is shipping"}[n]), 0, now.Add(offset), now)
	}
	group := func(id int64) int64 {
		var g int64
		if err := s.db.QueryRow("SELECT COALESCE(event_id,0) FROM articles WHERE id=?", id).Scan(&g); err != nil {
			t.Fatal(err)
		}
		return g
	}
	a := create(0, 0)
	b := create(1, 35*time.Hour)
	c := create(2, 60*time.Hour)
	d := create(3, 30*time.Hour)
	if group(a) == 0 || group(a) != group(b) || group(c) == 0 || group(c) == group(a) || group(d) != 0 {
		t.Fatal("transitive or ambiguous membership", group(a), group(b), group(c), group(d))
	}
	undated := hitAt(t, s, "https://source.example/unknown", "First Qorix RTX 7090 units reach buyers", 0, time.Time{}, now)
	if group(undated) != group(a) {
		t.Fatal("missing first-seen fallback")
	}
	if _, filled, err := s.RecordMetadata(ctx, undated, "", now.Add(-30*24*time.Hour)); err != nil || !filled || group(undated) == group(a) {
		t.Fatal("published-date correction", filled, err)
	}
}

func TestEventIndexUpgradeKeepsHistory(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	// Simulate rows from a pre-event database in more than one upgrade batch.
	_, err := s.db.Exec(`WITH RECURSIVE seq(i) AS (VALUES(1) UNION ALL SELECT i+1 FROM seq WHERE i<260)
 INSERT INTO articles(url,normalized_url,url_hash,title,ingested_at,score)
 SELECT 'https://example.invalid/'||i,'u'||i,'h'||i,'Qorix RTX 7090 begins shipping',CURRENT_TIMESTAMP,i FROM seq`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("UPDATE app_settings SET value='0' WHERE key='event_index_v1'"); err != nil {
		t.Fatal(err)
	}
	if err = s.Prepare(ctx, 50); err != nil {
		t.Fatal(err)
	}
	if err = s.Prepare(ctx, 50); err != nil {
		t.Fatal(err)
	}
	var rows, groups int
	if err = s.db.QueryRow(`SELECT count(*),count(DISTINCT event_id) FROM articles WHERE event_id IS NOT NULL AND status='unread' AND score=id`).Scan(&rows, &groups); err != nil || rows != 260 || groups != 1 {
		t.Fatal(rows, groups, err)
	}
	cards, err := s.FetchGrouped(ctx, 2, 0)
	if err != nil || len(cards) != 1 || cards[0].RelatedCount != 259 || len(cards[0].Related) != 20 {
		t.Fatal("related list limit", len(cards), err)
	}
}

func TestGroupedFourPagesAndStaleLead(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	now := time.Now().UTC()
	for n := 0; n < 8; n++ {
		for i, form := range []string{"Kestrel %d starts shipping", "First Kestrel %d units reach buyers", "Kestrel %d begins shipping"} {
			hitAt(t, s, fmt.Sprintf("https://source.example/%d/%d", n, i), fmt.Sprintf(form, 7200+n), 0, now, now)
		}
	}
	seen := map[int64]bool{}
	for page := 0; page < 4; page++ {
		cards, err := s.FetchGrouped(ctx, 2, -1)
		if err != nil || len(cards) != 2 {
			t.Fatal(page, len(cards), err)
		}
		var ids []int64
		events := map[int64]int64{}
		for _, a := range cards {
			if a.EventID == 0 || seen[a.EventID] || len(a.Related) != 2 {
				t.Fatalf("page %d repeated/incomplete event %+v", page, a)
			}
			seen[a.EventID] = true
			ids = append(ids, a.ID)
			events[a.ID] = a.EventID
		}
		if page == 0 {
			id := ids[0]
			if err := s.AdvanceFeed(ctx, []int64{id}, map[int64]int64{id: events[ids[1]]}); err != nil {
				t.Fatal(err)
			}
			if state, _ := statusScore(t, s, id); state != "unread" {
				t.Fatal("stale mapping consumed a different story", state)
			}
		}
		if err := s.AdvanceFeed(ctx, ids, events); err != nil {
			t.Fatal(err)
		}
	}
	if cards, err := s.FetchGrouped(ctx, 2, -1); err != nil || len(cards) != 0 {
		t.Fatal("not exhausted", len(cards), err)
	}
	if cards, err := s.FetchTopUnread(ctx, 100, -1); err != nil || len(cards) != 16 {
		t.Fatal("unopened alternatives missing", len(cards), err)
	}
}
