package ingest

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"discover/internal/config"
	"discover/internal/model"
	"discover/internal/store"
)

type ingestStore interface {
	ListEnabledTopics(context.Context) ([]model.Topic, error)
	UpsertArticleHit(context.Context, store.UpsertArticleInput) error
	HideUnreadBelowScore(context.Context, float64) (int64, error)
	HideIngestTitleDuplicates(context.Context, time.Time, int) (store.IngestDedupeStats, error)
	ListUnreadThumbnailCandidates(context.Context, float64, int) ([]store.ThumbnailCandidate, error)
	SetThumbnailIfEmpty(context.Context, int64, string) (bool, error)
	CullOldUnread(context.Context, int, float64) (int64, error)
}

type Service struct {
	cfg           config.Config
	store         ingestStore
	client        *http.Client
	articleClient *http.Client
	rand          *rand.Rand
	mu            sync.Mutex
	instanceBlock map[string]time.Time
	lastMessage   string
	lastMessageAt time.Time
	lastMessages  []progressEntry
}
type progressEntry struct {
	Message string
	At      time.Time
}

func New(cfg config.Config, st *store.Store) *Service {
	return &Service{
		cfg: cfg, store: st, client: &http.Client{Timeout: 20 * time.Second, CheckRedirect: searchRedirect},
		articleClient: newArticleClient(), rand: rand.New(rand.NewSource(time.Now().UnixNano())),
		instanceBlock: make(map[string]time.Time),
	}
}

func searchRedirect(_ *http.Request, _ []*http.Request) error {
	// A configured search API should answer directly; even same-origin redirects
	// could change address between requests through DNS rebinding.
	return http.ErrUseLastResponse
}

func (s *Service) Run(ctx context.Context) error {
	runStart := time.Now()
	failures := &PartialRunError{}
	finish := func() error {
		// One bounded batch of diagnostics per run; no raw remote/DB errors.
		for _, f := range failures.Failures {
			s.logf("ingest: warning stage=%s code=%s topic_id=%d article_id=%d instance=%d category=%s time_range=%s page=%d",
				f.Stage, f.Code, f.TopicID, f.ArticleID, f.Instance, f.Category, f.TimeRange, f.Page)
		}
		if failures.Total > 0 {
			s.logf("ingest: %v", failures)
		}
		return failures.err()
	}
	topics, err := s.store.ListEnabledTopics(ctx)
	if err != nil {
		failures.add(dbFailure("list_topics", err))
		return finish()
	}
	if len(topics) == 0 {
		s.logf("ingest: no enabled topics; skipping")
		return nil
	}
	s.logf("ingest: started with %d topic(s)", len(topics))
	ingestedAt := time.Now().UTC()
	totalEntries, failedTopics := 0, 0
	for i, topic := range topics {
		if ctx.Err() != nil {
			failures.add(dbFailure("context", ctx.Err()))
			break
		}
		if i > 0 {
			delay := time.Duration(s.cfg.PerQueryDelaySeconds) * time.Second
			jitter := time.Duration(s.rand.Intn(maxInt(s.cfg.PerQueryJitterSeconds, 0)+1)) * time.Second
			s.logf("ingest: sleeping %s before next topic (%d/%d)", (delay + jitter).Round(time.Second), i+1, len(topics))
			timer := time.NewTimer(delay + jitter)
			select {
			case <-ctx.Done():
				timer.Stop()
				failures.add(dbFailure("context", ctx.Err()))
				return finish()
			case <-timer.C:
			}
		}
		topicStart := time.Now()
		entries, err := s.fetchTopic(ctx, topic.Query)
		if err != nil {
			failedTopics++
			failures.merge(err, topic.ID, 0)
		}
		// Partial fetch success is still useful, even when a later request failed.
		totalEntries += len(entries)
		for _, e := range entries {
			if ctx.Err() != nil {
				failures.add(dbFailure("context", ctx.Err()))
				break
			}
			norm, hash, domain, err := normalizeURL(e.URL)
			if err != nil || strings.TrimSpace(e.Title) == "" {
				continue
			}
			input := store.UpsertArticleInput{
				URL: e.URL, NormalizedURL: norm, URLHash: hash,
				Title: strings.TrimSpace(e.Title), Content: strings.TrimSpace(e.Content),
				ThumbnailURL: bestThumbnailURL(e.Thumbnail, e.ImgSrc), SourceDomain: domain,
				PublishedAt: parsePublished(e.PublishedDate, e.Pubdate), IngestedAt: ingestedAt,
				TopicID: topic.ID, Engines: len(e.Engines), SearxScore: e.Score,
				ExtraTitleHit: termBoost(topic.Query, e.Title, e.Content),
			}
			// The store owns current-rule scoring and once-per-article rule counters.
			if err := s.store.UpsertArticleHit(ctx, input); err != nil {
				f := dbFailure("upsert", err)
				f.TopicID = topic.ID
				failures.add(f)
			}
		}
		s.logf("ingest: topic done (%d/%d) topic_id=%d query=%q results=%d took=%s", i+1, len(topics), topic.ID, topic.Query, len(entries), time.Since(topicStart).Round(time.Millisecond))
	}
	if s.cfg.AutoHideBelowScore > -100 {
		hidden, err := s.store.HideUnreadBelowScore(ctx, s.cfg.AutoHideBelowScore)
		if err != nil {
			failures.add(dbFailure("auto_hide", err))
		} else if hidden > 0 {
			s.logf("ingest: auto-hidden %d unread article(s)", hidden)
		}
	}
	stats, err := s.store.HideIngestTitleDuplicates(ctx, ingestedAt, s.cfg.DedupeTitleKeyChars)
	if err != nil {
		failures.add(dbFailure("title_dedupe", err))
	} else if stats.SameRunHidden+stats.HistoricalHidden > 0 {
		s.logf("ingest: title dedupe hidden %d unread article(s) (same_run=%d, historical_seen=%d)", stats.SameRunHidden+stats.HistoricalHidden, stats.SameRunHidden, stats.HistoricalHidden)
	}
	if s.cfg.ThumbnailRefreshMaxPerRun > 0 {
		filled, scanned, err := s.refreshMissingThumbnails(ctx)
		failures.merge(err, 0, 0)
		if scanned > 0 {
			s.logf("ingest: thumbnail refresh scanned %d high-score unread article(s), filled %d", scanned, filled)
		}
	}
	deleted, err := s.store.CullOldUnread(ctx, s.cfg.CullUnreadDays, s.cfg.CullMaxScore)
	if err != nil {
		failures.add(dbFailure("cull", err))
	} else if deleted > 0 {
		s.logf("cull: deleted %d old unread low-score articles", deleted)
	}
	s.logf("ingest: finished in %s (topics=%d, fetched_entries=%d, failed_topics=%d, failures=%d)", time.Since(runStart).Round(time.Millisecond), len(topics), totalEntries, failedTopics, failures.Total)
	return finish()
}

func (s *Service) logf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Print(msg)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastMessage, s.lastMessageAt = msg, time.Now()
	s.lastMessages = append(s.lastMessages, progressEntry{Message: msg, At: s.lastMessageAt})
	if len(s.lastMessages) > 2 {
		s.lastMessages = s.lastMessages[len(s.lastMessages)-2:]
	}
}
func (s *Service) LastProgress() (string, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastMessage, s.lastMessageAt
}
func (s *Service) LastProgressMessages(limit int) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || len(s.lastMessages) == 0 {
		return nil
	}
	if limit > len(s.lastMessages) {
		limit = len(s.lastMessages)
	}
	out := make([]string, limit)
	start := len(s.lastMessages) - limit
	for i := 0; i < limit; i++ {
		out[i] = s.lastMessages[start+i].Message
	}
	return out
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
