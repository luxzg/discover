package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// Store tokens for immutable anchors only. Each indexed probe has a time
// range and a row limit; saturated terms are not useful for retrieval.
func coverageCandidates(ctx context.Context, tx *sql.Tx, keys []string, stamp, skip int64) (map[int64]int, error) {
	hits := map[int64]int{}
	for _, key := range keys {
		rows, err := tx.QueryContext(ctx, `SELECT event_id FROM coverage_anchor_terms WHERE term=? AND stamp BETWEEN ? AND ? LIMIT 33`, key, stamp-eventSpan, stamp+eventSpan)
		if err != nil {
			return nil, err
		}
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		if len(ids) <= 32 {
			for _, id := range ids {
				if id != skip {
					hits[id]++
				}
			}
		}
	}
	return hits, nil
}

func saveCoverageAnchor(ctx context.Context, tx *sql.Tx, group int64, title string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO coverage_anchor_titles(event_id,title) VALUES(?,?)`, group, title)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil || n == 0 {
		return err
	}
	for _, k := range keys {
		if _, err := tx.ExecContext(ctx, `INSERT INTO coverage_anchor_terms SELECT ?,anchor_time,id FROM coverage_events WHERE id=?`, k, group); err != nil {
			return err
		}
	}
	return nil
}

// Even if the ranked lead changes, there is direct evidence between it and
// every alternative. A headline which merely bridges two events cannot join.
func compatibleMembers(ctx context.Context, tx *sql.Tx, group, id int64, title string, c coverageClaim) (bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT title FROM articles WHERE event_id=? AND id<>? LIMIT 21`, group, id)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var other string
		if err := rows.Scan(&other); err != nil {
			return false, err
		}
		n++
		if n > 19 || (title != other && !relatedClaim(c, parseCoverage(other))) {
			return false, nil
		}
	}
	return true, rows.Err()
}

func indexEvent(ctx context.Context, tx *sql.Tx, id int64, title string, stamp time.Time) error {
	var old int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(event_id,0) FROM articles WHERE id=?`, id).Scan(&old); err != nil {
		return err
	}
	c := parseCoverage(title)
	keys := c.keys()
	group, err := chooseCoverage(ctx, tx, id, old, title, stamp, c, keys)
	if err != nil {
		return err
	}
	if group != old {
		if _, err := tx.ExecContext(ctx, `UPDATE articles SET event_id=NULLIF(?,0) WHERE id=?`, group, id); err != nil {
			return err
		}
		if old != 0 {
			_, err := tx.ExecContext(ctx, `DELETE FROM coverage_events WHERE id=? AND advanced=0 AND NOT EXISTS(SELECT 1 FROM articles WHERE event_id=?)`, old, old)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func chooseCoverage(ctx context.Context, tx *sql.Tx, id, old int64, title string, stamp time.Time, c coverageClaim, keys []string) (int64, error) {
	if stamp.IsZero() {
		return 0, nil
	}
	t := stamp.Unix()
	claim, legacy := headlineClaim(title)
	var exact int64
	if legacy {
		var err error
		exact, err = exactAnchor(ctx, tx, claim, t)
		if err != nil || exact < 0 {
			return 0, err
		}
	}
	if exact > 0 {
		var locked, mixed bool
		err := tx.QueryRowContext(ctx, `SELECT advanced OR EXISTS(SELECT 1 FROM articles WHERE event_id=e.id AND id<>?),
 EXISTS(SELECT 1 FROM coverage_anchor_titles WHERE event_id=e.id AND mixed=1) FROM coverage_events e WHERE id=?`, id, exact).Scan(&locked, &mixed)
		if err != nil {
			return 0, err
		}
		if mixed {
			ok, err := compatibleMembers(ctx, tx, exact, id, title, c)
			if err != nil || !ok {
				return 0, err
			}
		}
		if locked || exact != old {
			return exact, saveCoverageAnchor(ctx, tx, exact, title, keys)
		}
	}
	hits, err := coverageCandidates(ctx, tx, keys, t, exact)
	if err != nil {
		return 0, err
	}
	choice := int64(0)
	for g, n := range hits {
		if n < 2 {
			continue
		}
		var anchor string
		if err := tx.QueryRowContext(ctx, `SELECT title FROM coverage_anchor_titles WHERE event_id=?`, g).Scan(&anchor); err != nil {
			return 0, err
		}
		if title != anchor && !relatedClaim(c, parseCoverage(anchor)) {
			continue
		}
		ok, err := compatibleMembers(ctx, tx, g, id, title, c)
		if err != nil {
			return 0, err
		}
		if !ok {
			continue
		}
		if choice != 0 {
			return 0, nil
		}
		choice = g
	}
	if choice > 0 {
		_, err := tx.ExecContext(ctx, `UPDATE coverage_anchor_titles SET mixed=1 WHERE event_id=? AND mixed=0`, choice)
		return choice, err
	}
	if exact == 0 {
		if !legacy {
			if len(keys) == 0 {
				return 0, nil
			}
			claim = eventClaim{"headline evidence", keys[0], strings.TrimSpace(title)}
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO coverage_events(kind,block,subject,anchor_time) VALUES(?,?,?,?)`, claim.kind, claim.block, claim.subject, t)
		if err != nil {
			return 0, err
		}
		exact, err = res.LastInsertId()
		if err != nil {
			return 0, err
		}
	}
	return exact, saveCoverageAnchor(ctx, tx, exact, title, keys)
}
