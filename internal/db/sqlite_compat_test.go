package db

import (
	"os"
	"path/filepath"
	"testing"
)

// The helper builds this same test against old and current module locks in
// disposable checkouts. Ordinary tests skip it; no production DB is opened.
func TestSQLiteEngineCompatibility(t *testing.T) {
	dir := os.Getenv("DISCOVER_SQLITE_COMPAT_DIR")
	if dir == "" {
		t.Skip("run bash scripts/test-sqlite-upgrade.sh for cross-driver validation")
	}
	mode := os.Getenv("DISCOVER_SQLITE_COMPAT_MODE")
	if mode != "create" && mode != "verify" {
		t.Fatal("invalid compatibility mode")
	}
	path := filepath.Join(dir, "synthetic.db")
	if mode == "create" {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("create requires a new synthetic database")
		}
	} else if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := d.Close(); err != nil {
			t.Error(err)
		}
	})
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := d.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	var version, journal string
	var foreignKeys int
	if err := d.QueryRow("SELECT sqlite_version()").Scan(&version); err != nil {
		t.Fatal(err)
	}
	t.Logf("SQLite %s: %s", version, mode)
	if err := d.QueryRow("PRAGMA journal_mode").Scan(&journal); err != nil || journal != "wal" {
		t.Fatalf("journal=%s: %v", journal, err)
	}
	if err := d.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil || foreignKeys != 1 {
		t.Fatalf("foreign keys=%d: %v", foreignKeys, err)
	}
	const title = "Synthetic battery \u2013 test headline"
	if mode == "create" {
		exec(`INSERT INTO topics(id,query,weight) VALUES(1,'battery',2.5)`)
		exec(`INSERT INTO negative_rules(id,pattern,penalty,applied_count) VALUES(1,'example.test',10,1)`)
		exec(`INSERT INTO articles(id,url,normalized_url,url_hash,title,ingested_at,published_at,
		 status,hidden_reason,score,score_base,story_key,dedupe_counted)
		 VALUES(1,'https://example.test/article','https://example.test/article','fixture',?,
		 '2026-09-12T10:00:00Z','2026-09-11T09:00:00Z','hidden','manual',26.125,30.125,'story',1)`, title)
		exec(`INSERT INTO article_topics VALUES(1,1)`)
		exec(`INSERT INTO article_evidence VALUES(1,1,2.4)`)
		exec(`INSERT INTO article_rule_effects VALUES(1,1,10,1)`)
		exec(`INSERT INTO app_settings(key,value) VALUES('dedupe_hidden_total','7')`)
	}
	var gotTitle, status, reason, published, counter string
	var score, baseline, evidence, penalty float64
	var counted int
	err = d.QueryRow(`SELECT a.title,a.status,a.hidden_reason,a.score,a.score_base,
	 CAST(a.published_at AS TEXT),e.relevance,r.penalty,a.dedupe_counted
	 FROM articles a JOIN article_evidence e ON e.article_id=a.id
	 JOIN article_rule_effects r ON r.article_id=a.id WHERE a.id=1`).Scan(
		&gotTitle, &status, &reason, &score, &baseline, &published, &evidence, &penalty, &counted)
	if err != nil {
		t.Fatal(err)
	}
	if gotTitle != title || status != "hidden" || reason != "manual" || score != 26.125 || baseline != 30.125 ||
		published != "2026-09-11T09:00:00Z" || evidence != 2.4 || penalty != 10 || counted != 1 {
		t.Fatalf("fixture changed: %q %s %s %v %v %s %v %v %d", gotTitle, status, reason, score, baseline, published, evidence, penalty, counted)
	}
	if err := d.QueryRow(`SELECT value FROM app_settings WHERE key='dedupe_hidden_total'`).Scan(&counter); err != nil || counter != "7" {
		t.Fatalf("counter=%q: %v", counter, err)
	}
	if _, err := d.Exec(`INSERT INTO article_topics VALUES(999,1)`); err == nil {
		t.Fatal("orphan reference accepted")
	}
	tx, err := d.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE articles SET score=-999 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := d.QueryRow(`SELECT score FROM articles WHERE id=1`).Scan(&score); err != nil || score != 26.125 {
		t.Fatalf("rollback score=%v: %v", score, err)
	}
	// Every engine commits a marker that the next engine must be able to read.
	var visits int
	err = d.QueryRow(`SELECT COALESCE((SELECT CAST(value AS INTEGER) FROM app_settings WHERE key='compat_visits'),0)`).Scan(&visits)
	if err != nil {
		t.Fatal(err)
	}
	if mode == "verify" && visits < 1 {
		t.Fatal("previous engine's committed marker missing")
	}
	exec(`INSERT INTO app_settings(key,value) VALUES('compat_visits',?)
	 ON CONFLICT(key) DO UPDATE SET value=excluded.value`, visits+1)
	var integrity string
	if err := d.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("integrity=%s: %v", integrity, err)
	}
	rows, err := d.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign-key violation in fixture")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}
