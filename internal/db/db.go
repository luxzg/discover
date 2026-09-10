package db

import (
	"database/sql"
	"net/url"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	u := &url.URL{Scheme: "file", Path: path}
	dsn := u.String() + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS topics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			query TEXT NOT NULL UNIQUE,
			weight REAL NOT NULL DEFAULT 1.0,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS negative_rules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			pattern TEXT NOT NULL UNIQUE,
			penalty REAL NOT NULL DEFAULT 5,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS articles (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			url TEXT NOT NULL,
			normalized_url TEXT NOT NULL,
			url_hash TEXT NOT NULL UNIQUE,
			title TEXT NOT NULL,
			content TEXT NOT NULL DEFAULT '',
			thumbnail_url TEXT NOT NULL DEFAULT '',
			source_domain TEXT NOT NULL DEFAULT '',
			published_at DATETIME,
			ingested_at DATETIME NOT NULL,
			status TEXT NOT NULL DEFAULT 'unread',
			score REAL NOT NULL DEFAULT 0,
			hit_count INTEGER NOT NULL DEFAULT 0,
			engine_count INTEGER NOT NULL DEFAULT 0,
			searx_score REAL NOT NULL DEFAULT 0,
			last_seen_at DATETIME,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS article_topics (
			article_id INTEGER NOT NULL,
			topic_id INTEGER NOT NULL,
			PRIMARY KEY (article_id, topic_id),
			FOREIGN KEY (article_id) REFERENCES articles(id) ON DELETE CASCADE,
			FOREIGN KEY (topic_id) REFERENCES topics(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS app_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_articles_status_score_pub ON articles(status, score DESC, published_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_articles_ingested ON articles(ingested_at);`,
		`CREATE INDEX IF NOT EXISTS idx_articles_published ON articles(published_at);`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	if err := ensureColumn(db, "negative_rules", "applied_count", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	columns := []struct{ name, ddl string }{
		{"hidden_reason", "TEXT NOT NULL DEFAULT ''"},
		{"story_key", "TEXT NOT NULL DEFAULT ''"},
		{"duplicate_of", "INTEGER REFERENCES articles(id) ON DELETE SET NULL"},
		{"dedupe_counted", "INTEGER NOT NULL DEFAULT 0"},
		{"score_base", "REAL"},
		{"vote", "INTEGER NOT NULL DEFAULT 0"},
		{"read_at", "DATETIME"},
	}
	for _, col := range columns {
		if err := ensureColumn(db, "articles", col.name, col.ddl); err != nil {
			return err
		}
	}
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS idx_articles_story ON articles(story_key)`,
		`CREATE TABLE IF NOT EXISTS article_evidence (
   article_id INTEGER NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
   topic_id INTEGER NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
   relevance REAL NOT NULL, PRIMARY KEY(article_id, topic_id))`,
		`CREATE TABLE IF NOT EXISTS article_rule_effects (
   article_id INTEGER NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
   rule_id INTEGER NOT NULL REFERENCES negative_rules(id) ON DELETE CASCADE,
   penalty REAL NOT NULL DEFAULT 0, counted INTEGER NOT NULL DEFAULT 0,
   PRIMARY KEY(article_id, rule_id))`,
		`UPDATE articles SET hidden_reason='legacy' WHERE status='hidden' AND hidden_reason=''`,
		`UPDATE articles SET vote=1 WHERE status='useful' AND vote=0`,
		`DELETE FROM article_topics WHERE NOT EXISTS (SELECT 1 FROM articles WHERE id=article_id)
    OR NOT EXISTS (SELECT 1 FROM topics WHERE id=topic_id)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func ensureColumn(db *sql.DB, table, column, columnDDL string) error {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + columnDDL)
	return err
}
