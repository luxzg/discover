package store

import (
	"context"
	"discover/internal/model"
	"testing"
	"time"
)

func TestRuleEditLargeUnreadDatabase(t *testing.T) {
	s := testStore(t)
	// Disposable scale fixture: 40k candidates and 80 unrelated enabled rules.
	_, err := s.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<40000)
 INSERT INTO articles(url,normalized_url,url_hash,title,source_domain,story_key,ingested_at,score,score_base)
 SELECT 'https://example.com/'||x,'https://example.com/'||x,'hash-'||x,'Story '||x,
 CASE WHEN x%100=0 THEN 'blocked.example' ELSE 'example.com' END,'story-'||x,CURRENT_TIMESTAMP,100,100 FROM n;
 WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<80)
 INSERT INTO negative_rules(pattern,penalty,enabled) SELECT 'irrelevant-'||x,1,1 FROM n;`)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	start := time.Now()
	if err := s.UpsertNegativeRule(ctx, model.NegativeRule{Pattern: "blocked.example", Penalty: 100, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	t.Logf("40k-article, 80-existing-rule update completed in %s", time.Since(start))
	var changed int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM articles WHERE score=0`).Scan(&changed); err != nil || changed != 400 {
		t.Fatal(changed, err)
	}
	rules, err := s.ListNegativeRules(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rule := rules[len(rules)-1]
	if rule.AppliedCount != 400 {
		t.Fatal(rule)
	}
	rule.Penalty = 50
	start = time.Now()
	if err := s.UpsertNegativeRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	t.Logf("rule penalty edit completed in %s", time.Since(start))
	start = time.Now()
	if err := s.DeleteNegativeRule(ctx, rule.ID); err != nil {
		t.Fatal(err)
	}
	t.Logf("rule deletion completed in %s", time.Since(start))
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM articles WHERE score<>100`).Scan(&changed); err != nil || changed != 0 {
		t.Fatal(changed, err)
	}
	start = time.Now()
	ids, err := s.HideWithRule(ctx, 100, model.NegativeRule{Pattern: "blocked.example", Penalty: 100, Enabled: true})
	if err != nil || len(ids) != 400 {
		t.Fatal(len(ids), err)
	}
	t.Logf("full Hide Domain completed in %s", time.Since(start))
	var status string
	var score float64
	if err := s.db.QueryRow(`SELECT status,score FROM articles WHERE id=100`).Scan(&status, &score); err != nil || status != "hidden" || score != 0 {
		t.Fatal(status, score, err)
	}
}

func TestRuleRemovalRestoresFractionalScoreAndStoryWinner(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	a := hit(t, s, "https://first.example/a", "Shared story", 0, time.Time{})
	b := hit(t, s, "https://second.example/b", "Shared story", 0, time.Time{})
	if _, err := s.db.Exec(`UPDATE articles SET score_base=3.85,score=3.85`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.HideAllUnreadTitleDuplicates(ctx, 50); err != nil {
		t.Fatal(err)
	}
	for _, removal := range []string{"disable", "rename", "delete"} {
		t.Run(removal, func(t *testing.T) {
			if err := s.UpsertNegativeRule(ctx, model.NegativeRule{Pattern: "first.example", Penalty: 100, Enabled: true}); err != nil {
				t.Fatal(err)
			}
			cards, err := s.FetchTopUnread(ctx, 10, 3.85)
			if err != nil || len(cards) != 1 || cards[0].ID != b {
				t.Fatalf("wrong penalized winner: %+v %v", cards, err)
			}
			rules, err := s.ListNegativeRules(ctx)
			if err != nil || len(rules) != 1 {
				t.Fatal(rules, err)
			}
			rule := rules[0]
			switch removal {
			case "disable":
				rule.Enabled = false
				err = s.UpsertNegativeRule(ctx, rule)
			case "rename":
				rule.Pattern = "not-present"
				err = s.UpsertNegativeRule(ctx, rule)
			case "delete":
				err = s.DeleteNegativeRule(ctx, rule.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			cards, err = s.FetchTopUnread(ctx, 10, 3.85)
			if err != nil || len(cards) != 1 || cards[0].ID != a || cards[0].Score != 3.85 {
				t.Fatalf("threshold eligibility or tied winner changed: %+v %v", cards, err)
			}
			if removal != "delete" {
				if err := s.DeleteNegativeRule(ctx, rule.ID); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
