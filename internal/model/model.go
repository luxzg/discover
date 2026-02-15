package model

import "time"

type ArticleStatus string

const (
	StatusUnread ArticleStatus = "unread"
	StatusSeen   ArticleStatus = "seen"
	StatusUseful ArticleStatus = "useful"
	StatusHidden ArticleStatus = "hidden"
	StatusRead   ArticleStatus = "read"
)

type Topic struct {
	ID      int64   `json:"id"`
	Query   string  `json:"query"`
	Weight  float64 `json:"weight"`
	Enabled bool    `json:"enabled"`
}

type NegativeRule struct {
	ID      int64   `json:"id"`
	Pattern string  `json:"pattern"`
	Penalty float64 `json:"penalty"`
	Enabled bool    `json:"enabled"`
}

type Article struct {
	ID            int64         `json:"id"`
	URL           string        `json:"url"`
	NormalizedURL string        `json:"normalized_url"`
	URLHash       string        `json:"url_hash"`
	Title         string        `json:"title"`
	Content       string        `json:"content"`
	ThumbnailURL  string        `json:"thumbnail_url"`
	SourceDomain  string        `json:"source_domain"`
	PublishedAt   time.Time     `json:"published_at"`
	IngestedAt    time.Time     `json:"ingested_at"`
	Status        ArticleStatus `json:"status"`
	Score         float64       `json:"score"`
	HitCount      int           `json:"hit_count"`
	EngineCount   int           `json:"engine_count"`
	SearxScore    float64       `json:"searx_score"`
}
