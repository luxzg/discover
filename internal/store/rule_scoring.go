package store

import (
	"context"
	"database/sql"
	"discover/internal/matcher"
	"discover/internal/model"
)

// Topic edits reuse the already-current rule ledger instead of rematching rules.
func rescoreTopicEvidence(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `UPDATE articles SET score=score_base+
 COALESCE((SELECT SUM(e.relevance+t.weight) FROM article_evidence e JOIN topics t ON t.id=e.topic_id WHERE e.article_id=articles.id AND t.enabled=1),0)-
 COALESCE((SELECT SUM(penalty) FROM article_rule_effects r WHERE r.article_id=articles.id),0),updated_at=CURRENT_TIMESTAMP
 WHERE status='unread' OR (status='hidden' AND hidden_reason='duplicate')`)
	if err != nil {
		return err
	}
	return reconcileDerivedStories(ctx, tx)
}

// A rule edit only changes that rule's contribution, not every article/rule pair.
// Read candidates once, then write changed effects with prepared statements.
func rescoreChangedRule(ctx context.Context, tx *sql.Tx, rule model.NegativeRule) error {
	rows, err := tx.QueryContext(ctx, `SELECT a.id,a.title,a.content,a.source_domain,a.url,a.story_key,
 COALESCE(e.penalty,0),COALESCE(e.counted,0)
 FROM articles a LEFT JOIN article_rule_effects e ON e.article_id=a.id AND e.rule_id=?
 WHERE a.status='unread' OR (a.status='hidden' AND a.hidden_reason='duplicate')`, rule.ID)
	if err != nil {
		return err
	}
	type change struct {
		id                int64
		key               string
		previous, penalty float64
		counted           int
	}
	var changes []change
	var newMatches int64
	for rows.Next() {
		var c change
		var title, content, domain, url string
		if err := rows.Scan(&c.id, &title, &content, &domain, &url, &c.key, &c.previous, &c.counted); err != nil {
			rows.Close()
			return err
		}
		if rule.Enabled && matcher.MatchRule(rule.Pattern, title, content, domain, url) {
			c.penalty = rule.Penalty
		}
		newlyCounted := c.penalty > 0 && c.counted == 0
		if newlyCounted {
			newMatches++
			c.counted = 1
		}
		if c.penalty != c.previous || newlyCounted {
			changes = append(changes, c)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if len(changes) == 0 {
		return nil
	}
	effect, err := tx.PrepareContext(ctx, `INSERT INTO article_rule_effects(article_id,rule_id,penalty,counted) VALUES(?,?,?,?) ON CONFLICT(article_id,rule_id) DO UPDATE SET penalty=excluded.penalty,counted=excluded.counted`)
	if err != nil {
		return err
	}
	defer effect.Close()
	// Rebuild only changed scores from their ledger to avoid cumulative rounding
	// drift when a large penalty is added to and removed from a fractional score.
	score, err := tx.PrepareContext(ctx, `UPDATE articles SET score=score_base+
 COALESCE((SELECT SUM(e.relevance+t.weight) FROM article_evidence e JOIN topics t ON t.id=e.topic_id WHERE e.article_id=articles.id AND t.enabled=1),0)-
 COALESCE((SELECT SUM(penalty) FROM article_rule_effects r WHERE r.article_id=articles.id),0),updated_at=CURRENT_TIMESTAMP WHERE id=?`)
	if err != nil {
		return err
	}
	defer score.Close()
	keys := map[string]bool{}
	for _, c := range changes {
		if _, err := effect.ExecContext(ctx, c.id, rule.ID, c.penalty, c.counted); err != nil {
			return err
		}
		if _, err := score.ExecContext(ctx, c.id); err != nil {
			return err
		}
		keys[c.key] = true
	}
	if _, err := tx.ExecContext(ctx, `UPDATE negative_rules SET applied_count=applied_count+? WHERE id=?`, newMatches, rule.ID); err != nil {
		return err
	}
	for key := range keys {
		var derived bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM articles WHERE story_key=? AND status='hidden' AND hidden_reason='duplicate')`, key).Scan(&derived); err != nil {
			return err
		}
		if derived {
			if _, err := hideGroup(ctx, tx, key); err != nil {
				return err
			}
		}
	}
	return nil
}
