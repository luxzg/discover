package store

import (
	"context"
	"discover/internal/matcher"
	"discover/internal/model"
	"errors"
)

// HideWithRule commits the rule, unread score changes and selected hide together.
// The selected article is not charged a second penalty for the same rule.
func (s *Store) HideWithRule(ctx context.Context, id int64, rule model.NegativeRule) ([]int64, error) {
	if id <= 0 {
		return nil, errors.New("invalid article id")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var key string
	if err := tx.QueryRowContext(ctx, `SELECT story_key FROM articles WHERE id=?`, id).Scan(&key); err != nil {
		return nil, err
	}
	articles, err := unreadScoreRows(ctx, tx, false)
	if err != nil {
		return nil, err
	}
	ids := []int64{id}
	for _, a := range articles {
		if a.id != id && matcher.MatchRule(rule.Pattern, a.title, a.content, a.domain, a.url) {
			ids = append(ids, a.id)
		}
	}
	if err := upsertRule(ctx, tx, rule); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE articles SET status='hidden',hidden_reason='manual',duplicate_of=NULL,updated_at=CURRENT_TIMESTAMP WHERE id=?`, id); err != nil {
		return nil, err
	}
	if _, err := hideGroup(ctx, tx, key); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return ids, nil
}
