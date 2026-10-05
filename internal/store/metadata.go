package store

import (
	"context"
	"time"
)

type MetadataCandidate struct {
	ID            int64
	URL           string
	NeedThumbnail bool
	NeedDate      bool
}

func (s *Store) ListMetadataCandidates(ctx context.Context, thumbScore, feedScore float64, thumbLimit, dateLimit int) ([]MetadataCandidate, error) {
	days, err := s.FeedAgeLimit(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, ageCTE+`, eligible AS (
 SELECT * FROM aged WHERE status='unread' AND archived_at IS NULL AND (?=0 OR age_date>=julianday('now')-?)
 AND (metadata_checked_at IS NULL OR julianday(metadata_checked_at)<julianday('now','-1 day'))
), candidates AS (
 SELECT id FROM (SELECT id FROM eligible WHERE score>=? AND COALESCE(thumbnail_url,'')='' ORDER BY score DESC,id LIMIT ?)
 UNION SELECT id FROM (SELECT id FROM eligible WHERE score>=? AND published_at IS NULL ORDER BY score DESC,id LIMIT ?)
) SELECT e.id,e.url,COALESCE(e.thumbnail_url,'')='',e.published_at IS NULL
 FROM eligible e JOIN candidates c ON c.id=e.id ORDER BY e.score DESC,e.id`, days, days, thumbScore, thumbLimit, feedScore, dateLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MetadataCandidate{}
	for rows.Next() {
		var c MetadataCandidate
		if err := rows.Scan(&c.ID, &c.URL, &c.NeedThumbnail, &c.NeedDate); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) RecordMetadata(ctx context.Context, id int64, thumb string, published time.Time) (bool, bool, error) {
	if published.Year() < 2000 || published.After(time.Now().Add(24*time.Hour)) {
		published = time.Time{}
	}
	var imageFilled, dateFilled bool
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, false, err
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(thumbnail_url,'')='' AND ?<>'',published_at IS NULL AND ? IS NOT NULL FROM articles WHERE id=?`, thumb, dbTimestamp(published), id).Scan(&imageFilled, &dateFilled)
	if err != nil {
		return false, false, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE articles SET
 thumbnail_url=CASE WHEN COALESCE(thumbnail_url,'')='' AND ?<>'' THEN ? ELSE thumbnail_url END,
 published_at=COALESCE(published_at,?),metadata_checked_at=CURRENT_TIMESTAMP WHERE id=?`, thumb, thumb, dbTimestamp(published), id)
	if err != nil {
		return false, false, err
	}
	return imageFilled, dateFilled, tx.Commit()
}
