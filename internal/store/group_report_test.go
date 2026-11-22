package store

import (
	"testing"
	"time"
)

func TestGroupReportReflectsFeedAndRetainedHistory(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	report := func(page int) GroupReport {
		r, err := s.GroupReport(ctx, page, 1)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if r := report(1); r.Grouped != 0 || r.RetainedAnchors != 0 || len(r.Examples) != 0 {
		t.Fatal(r)
	}
	now := time.Now().UTC()
	add := func(path, title string) int64 {
		return hitAt(t, s, "https://example.invalid/"+path, title, 0, now, now)
	}
	const lead = "Quasar RTX 7290 starts shipping"
	const alt = "First Quasar RTX 7290 units reach buyers"
	a := add("one", lead)
	add("syndicated", lead)
	b := add("two", alt)
	c := add("singleton", "A separate report")
	if _, err := s.db.Exec("UPDATE articles SET score=CASE id WHEN ? THEN 10 WHEN ? THEN 8 WHEN ? THEN 6 ELSE 4 END", c, a, b); err != nil {
		t.Fatal(err)
	}
	r := report(1)
	if r.Standard != 3 || r.OtherSources != 1 || r.Grouped != 2 || r.RelatedGroups != 1 || r.Folded != 1 || r.Advanced != 0 || r.OnPage != 0 || r.RetainedGroups != 1 || r.RetainedAnchors != 1 || len(r.Examples) != 1 {
		t.Fatal(r)
	}
	e := r.Examples[0]
	if e.Position != 2 || e.Size != 2 || e.Lead != lead || e.Related != alt || report(2).OnPage != 1 {
		t.Fatal(e)
	}
	if _, err := s.db.Exec("UPDATE articles SET archived_at=CURRENT_TIMESTAMP WHERE id=?", b); err != nil {
		t.Fatal(err)
	}
	if r = report(2); r.RelatedGroups != 0 || r.Folded != 0 || r.RetainedGroups != 1 || r.OtherSources != 1 || len(r.Examples) != 0 {
		t.Fatal(r)
	}
	if _, err := s.db.Exec("UPDATE articles SET archived_at=NULL WHERE id=?", b); err != nil {
		t.Fatal(err)
	}
	cards, err := s.FetchGrouped(ctx, 2, 1)
	if err != nil || len(cards) != 2 {
		t.Fatal(cards, err)
	}
	if err := s.AdvanceFeed(ctx, []int64{a}, map[int64]int64{a: cards[1].EventID}); err != nil {
		t.Fatal(err)
	}
	if r = report(2); r.Standard != 2 || r.Grouped != 1 || r.Advanced != 1 || r.RetainedGroups != 1 || r.RelatedGroups != 0 {
		t.Fatal(r)
	}
}
