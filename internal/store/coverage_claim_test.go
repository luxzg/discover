package store

import (
	"fmt"
	"testing"
	"time"
)

func TestRelatedClaimScope(t *testing.T) {
	for _, tc := range []struct {
		a, b  string
		match bool
	}{
		{"River Works brings sixth generation Lumen Prism processors to market at rack scale", "Sixth generation Lumen Prism now available from Lumen", false},
		{"Orchid launches Zephyr Wave mini PC for coastal labs", "Juniper launches Zephyr Wave mini PC for coastal labs", false},
		{"Orchid Prism T16 review: Zephyr Wave workstation", "Orchid Prism GR1X review: Zephyr Wave workstation", false},
		{"Orchid Prism mobile and desktop details emerge with 28 cores", "Orchid Prism-S engineering sample spotted with 28 cores", false},
		{"Orchid Prism-H mobile processor specs reveal 4.8 GHz boost", "Orchid Prism H leak reveals 16 cores with 4800MHz boost", true},
		{"Orchid Prism-H mobile processor specs reveal 4.8 GHz boost", "Orchid Prism-S mobile processor specs reveal 4.8 GHz boost", false},
		{"Orchid claims Prism 7200+ launches with tidal cooling", "Orchid claims Prism 7200 launches with tidal cooling", false},
		{"Orchid Lira generator reportedly delayed past Polaris 2030", "Polaris 2030: Lira generator from Orchid reportedly postponed", true},
		{"Helios survey maps distant polar iridium deposits", "Distant polar iridium deposits mapped by Helios survey", true},
	} {
		a, b := parseCoverage(tc.a), parseCoverage(tc.b)
		if relatedClaim(a, b) != tc.match || relatedClaim(b, a) != tc.match {
			t.Errorf("%s / %s: want %v", tc.a, tc.b, tc.match)
		}
	}
}

func TestRelatedCoverageFeedAndNoBridge(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	now := time.Now().UTC()
	titles := []string{
		"Zephyr Wave mini PC now available for coastal labs",
		"Orchid launches Zephyr Wave mini PC for coastal labs",
		"Juniper launches Zephyr Wave mini PC for coastal labs",
	}
	var ids []int64
	for i, title := range titles {
		ids = append(ids, hitAt(t, s, fmt.Sprintf("https://example.invalid/coverage%d", i), title, 0, now, now))
	}
	// The generic anchor matches either launch. The two named makers do not
	// match one another, even when a named article becomes the top-ranked lead.
	if _, err := s.db.Exec("UPDATE articles SET score=CASE WHEN id=? THEN 20 ELSE 10 END", ids[1]); err != nil {
		t.Fatal(err)
	}
	cards, err := s.FetchGrouped(ctx, 10, 0)
	if err != nil || len(cards) != 2 || cards[0].ID != ids[1] || len(cards[0].Related) != 1 || cards[0].Related[0].ID != ids[0] {
		t.Fatalf("grouped: %+v %v", cards, err)
	}
	standard, err := s.FetchTopUnread(ctx, 10, 0)
	if err != nil || len(standard) != 3 {
		t.Fatalf("standard: %+v %v", standard, err)
	}
	if err := s.AdvanceFeed(ctx, []int64{ids[1]}, map[int64]int64{ids[1]: cards[0].EventID}); err != nil {
		t.Fatal(err)
	}
	for _, i := range []int{0, 2} {
		if state, _ := statusScore(t, s, ids[i]); state != "unread" {
			t.Fatalf("alternative status %s", state)
		}
	}
}

func TestUpgradeRelatedCoverageFromSeparateLegacyAnchors(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	now := time.Now().UTC()
	titles := []string{
		"Orchid launches Zephyr Wave mini PC for coastal labs",
		"Zephyr Wave mini PC from Orchid now available for coastal labs",
		"Tessel 820 begins shipping",
	}
	var ids []int64
	for i, title := range titles {
		ids = append(ids, hitAt(t, s, fmt.Sprintf("https://example.invalid/legacy%d", i), title, 0, now, now))
	}
	// Mimic the deployed v1 state, including event progress and no term index.
	if _, err := s.db.Exec("DELETE FROM coverage_events"); err != nil {
		t.Fatal(err)
	}
	var advanced int64
	for i, title := range titles {
		c, ok := headlineClaim(title)
		if !ok {
			t.Fatal("fixture must have a legacy anchor", title)
		}
		res, err := s.db.Exec("INSERT INTO coverage_events(kind,block,subject,anchor_time,advanced) VALUES(?,?,?,?,?)", c.kind, c.block, c.subject, now.Unix(), i == 2)
		if err != nil {
			t.Fatal(err)
		}
		g, err := res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		if i == 2 {
			advanced = g
		}
		if _, err = s.db.Exec("UPDATE articles SET event_id=?,score=15 WHERE id=?", g, ids[i]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.Exec("DELETE FROM app_settings WHERE key LIKE 'event_index_v2%'"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.Prepare(ctx, 50); err != nil {
			t.Fatal(err)
		}
		g, err := s.FetchGrouped(ctx, 10, 1)
		if err != nil || len(g) != 1 || len(g[0].Related) != 1 {
			t.Fatalf("upgraded grouped: %+v %v", g, err)
		}
		std, err := s.FetchTopUnread(ctx, 10, 1)
		if err != nil || len(std) != 3 {
			t.Fatal(std, err)
		}
		for _, a := range std {
			if a.Score != 15 || a.Status != "unread" {
				t.Fatal(a)
			}
		}
		var retained bool
		if err := s.db.QueryRow("SELECT advanced FROM coverage_events WHERE id=?", advanced).Scan(&retained); err != nil || !retained {
			t.Fatal("lost advance", err)
		}
	}
}
