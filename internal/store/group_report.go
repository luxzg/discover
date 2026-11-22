package store

import (
	"context"
	"database/sql"
	"errors"
)

type GroupExample struct {
	Position int    `json:"position"`
	Size     int    `json:"size"`
	Lead     string `json:"lead"`
	Related  string `json:"related"`
	Reason   string `json:"reason"`
}

type GroupReport struct {
	PageSize        int            `json:"page_size"`
	Standard        int            `json:"standard"`
	OtherSources    int            `json:"other_sources"`
	Grouped         int            `json:"grouped"`
	RelatedGroups   int            `json:"related_groups"`
	Folded          int            `json:"folded"`
	Advanced        int            `json:"advanced"`
	OnPage          int            `json:"on_page"`
	RetainedAnchors int            `json:"retained_anchors"`
	RetainedGroups  int            `json:"retained_groups"`
	Examples        []GroupExample `json:"examples"`
}

// Use the feed's actual eligibility, partition and ordering. A retained anchor
// with several URLs for a single exact story is not a successful fuzzy match.
const groupReportSQL = eventMembersSQL + `, positions AS (
 SELECT id,bucket,size,kind,subject,ROW_NUMBER() OVER(ORDER BY feed_rank DESC,age_date DESC,id DESC) AS pos
 FROM members WHERE member=1
), retained AS (
 SELECT event_id,COUNT(DISTINCT CASE WHEN story_key='' THEN 'id:'||id ELSE story_key END) AS stories
 FROM articles WHERE event_id IS NOT NULL GROUP BY event_id
), standard AS (
 SELECT COUNT(*) AS cards,COUNT(*) FILTER(WHERE r.story_key<>'' AND EXISTS(
 SELECT 1 FROM articles a INDEXED BY idx_articles_story WHERE a.story_key=r.story_key AND a.id<>r.id
 AND (a.status='unread' OR (a.status='hidden' AND a.hidden_reason='duplicate')))) AS other
 FROM ranked r WHERE rank=1
), totals AS (
 SELECT COUNT(*) AS cards,COUNT(*) FILTER(WHERE size>1) AS groups,
 COALESCE(SUM(size-1),0) AS folded, COUNT(*) FILTER(WHERE size>1 AND pos<=?) AS on_page
 FROM positions
), examples AS (
 SELECT * FROM positions WHERE size>1 ORDER BY pos LIMIT 3
)
 SELECT s.cards,s.other,t.cards,t.groups,t.folded,s.cards-t.cards-t.folded,t.on_page,
 (SELECT COUNT(*) FROM retained),(SELECT COUNT(*) FROM retained WHERE stories>1),
 x.pos,x.size,lead.title,alt.title,x.kind||': '||x.subject
 FROM standard s CROSS JOIN totals t LEFT JOIN examples x ON TRUE
 LEFT JOIN articles lead ON lead.id=x.id
 LEFT JOIN members m ON m.bucket=x.bucket AND m.member=2
 LEFT JOIN articles alt ON alt.id=m.id ORDER BY x.pos`

func (s *Store) GroupReport(ctx context.Context, pageSize int, minScore float64) (GroupReport, error) {
	r := GroupReport{PageSize: pageSize, Examples: []GroupExample{}}
	if pageSize < 1 || pageSize > 100 {
		return r, errors.New("invalid page size")
	}
	days, err := s.FeedAgeLimit(ctx)
	if err != nil {
		return r, err
	}
	rows, err := s.db.QueryContext(ctx, groupReportSQL, s.decayDays, s.decayDays, days, days, minScore, pageSize)
	if err != nil {
		return r, err
	}
	defer rows.Close()
	for rows.Next() {
		var pos, size sql.NullInt64
		var lead, alt, reason sql.NullString
		if err := rows.Scan(&r.Standard, &r.OtherSources, &r.Grouped, &r.RelatedGroups, &r.Folded, &r.Advanced, &r.OnPage,
			&r.RetainedAnchors, &r.RetainedGroups, &pos, &size, &lead, &alt, &reason); err != nil {
			return r, err
		}
		if pos.Valid {
			r.Examples = append(r.Examples, GroupExample{int(pos.Int64), int(size.Int64), lead.String, alt.String, reason.String})
		}
	}
	return r, rows.Err()
}
