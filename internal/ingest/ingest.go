package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"discover/internal/config"
	"discover/internal/matcher"
	"discover/internal/model"
	"discover/internal/store"
)

type Service struct {
	cfg           config.Config
	store         *store.Store
	client        *http.Client
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
		cfg:   cfg,
		store: st,
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
		rand:          rand.New(rand.NewSource(time.Now().UnixNano())),
		instanceBlock: make(map[string]time.Time),
	}
}

type searxResponse struct {
	Query   string       `json:"query"`
	Results []searxEntry `json:"results"`
}

type searxEntry struct {
	URL           string   `json:"url"`
	Title         string   `json:"title"`
	Content       string   `json:"content"`
	Thumbnail     string   `json:"thumbnail"`
	ImgSrc        string   `json:"img_src"`
	Engines       []string `json:"engines"`
	ParsedURL     []string `json:"parsed_url"`
	Score         float64  `json:"score"`
	PublishedDate string   `json:"publishedDate"`
	Pubdate       string   `json:"pubdate"`
}

var (
	reMetaTag    = regexp.MustCompile(`(?is)<meta\b[^>]*>`)
	reLinkTag    = regexp.MustCompile(`(?is)<link\b[^>]*>`)
	reAttrKV     = regexp.MustCompile(`(?is)\b([a-zA-Z_:][a-zA-Z0-9_:\-]*)\s*=\s*("([^"]*)"|'([^']*)')`)
	errNoResults = errors.New("no results from query on instance")
)

func (s *Service) Run(ctx context.Context) error {
	runStart := time.Now()
	topics, err := s.store.ListEnabledTopics(ctx)
	if err != nil {
		return err
	}
	if len(topics) == 0 {
		s.logf("ingest: no enabled topics; skipping")
		return nil
	}
	s.logf("ingest: started with %d topic(s)", len(topics))
	rules, err := s.store.ListEnabledNegativeRules(ctx)
	if err != nil {
		return err
	}
	ingestedAt := time.Now().UTC()
	totalEntries := 0
	failedTopics := 0
	lastTopicErr := ""
	ruleApplyCounts := map[int64]int64{}

	for i, topic := range topics {
		if i > 0 {
			delay := time.Duration(s.cfg.PerQueryDelaySeconds) * time.Second
			jitter := time.Duration(s.rand.Intn(maxInt(s.cfg.PerQueryJitterSeconds, 0)+1)) * time.Second
			sleepFor := delay + jitter
			s.logf("ingest: sleeping %s before next topic (%d/%d)", sleepFor.Round(time.Second), i+1, len(topics))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(sleepFor):
			}
		}
		topicStart := time.Now()
		entries, err := s.fetchTopic(ctx, topic.Query)
		if err != nil {
			failedTopics++
			lastTopicErr = err.Error()
			s.logf("ingest: topic=%q error=%v", topic.Query, err)
			continue
		}
		totalEntries += len(entries)
		for _, e := range entries {
			norm, hash, domain, err := normalizeURL(e.URL)
			if err != nil || e.Title == "" {
				continue
			}
			penalty, matchedRuleIDs := computePenalty(rules, e.Title, e.Content, domain, e.URL)
			for _, ruleID := range matchedRuleIDs {
				ruleApplyCounts[ruleID]++
			}
			extra := termBoost(topic.Query, e.Title, e.Content)
			published := parsePublished(e.PublishedDate, e.Pubdate)
			thumb := bestThumbnailURL(e.Thumbnail, e.ImgSrc)
			input := store.UpsertArticleInput{
				URL:           e.URL,
				NormalizedURL: norm,
				URLHash:       hash,
				Title:         strings.TrimSpace(e.Title),
				Content:       strings.TrimSpace(e.Content),
				ThumbnailURL:  thumb,
				SourceDomain:  domain,
				PublishedAt:   published,
				IngestedAt:    ingestedAt,
				TopicID:       topic.ID,
				TopicWeight:   topic.Weight,
				Engines:       len(e.Engines),
				SearxScore:    e.Score,
				ExtraTitleHit: extra,
				Penalty:       penalty,
			}
			if err := s.store.UpsertArticleHit(ctx, input); err != nil {
				s.logf("ingest: upsert error url=%q err=%v", e.URL, err)
			}
		}
		s.logf("ingest: topic done (%d/%d) query=%q results=%d took=%s", i+1, len(topics), topic.Query, len(entries), time.Since(topicStart).Round(time.Millisecond))
	}
	if s.cfg.AutoHideBelowScore > -100 {
		hiddenCount, err := s.store.HideUnreadBelowScore(ctx, s.cfg.AutoHideBelowScore)
		if err != nil {
			s.logf("ingest: auto-hide error: %v", err)
		} else if hiddenCount > 0 {
			s.logf("ingest: auto-hidden %d unread article(s) with score < %.2f", hiddenCount, s.cfg.AutoHideBelowScore)
		}
	}
	dedupeStats, err := s.store.HideIngestTitleDuplicates(ctx, ingestedAt, s.cfg.DedupeTitleKeyChars)
	if err != nil {
		s.logf("ingest: title dedupe error: %v", err)
	} else if dedupeStats.SameRunHidden > 0 || dedupeStats.HistoricalHidden > 0 {
		s.logf("ingest: title dedupe hidden %d unread article(s) (same_run=%d, historical_seen=%d, key_chars=%d)",
			dedupeStats.SameRunHidden+dedupeStats.HistoricalHidden,
			dedupeStats.SameRunHidden,
			dedupeStats.HistoricalHidden,
			s.cfg.DedupeTitleKeyChars,
		)
	}
	if s.cfg.ThumbnailRefreshMaxPerRun > 0 {
		filled, scanned := s.refreshMissingThumbnails(ctx)
		if scanned > 0 {
			s.logf("ingest: thumbnail refresh scanned %d high-score unread article(s), filled %d", scanned, filled)
		}
	}
	if err := s.store.IncrementNegativeRuleAppliedCounts(ctx, ruleApplyCounts); err != nil {
		s.logf("ingest: negative rule counter update error: %v", err)
	}
	deleted, err := s.store.CullOldUnread(ctx, s.cfg.CullUnreadDays, s.cfg.CullMaxScore)
	if err != nil {
		s.logf("cull: error: %v", err)
	} else if deleted > 0 {
		s.logf("cull: deleted %d old unread low-score articles", deleted)
	}
	s.logf("ingest: all done in %s (topics=%d, fetched_entries=%d, failed_topics=%d)", time.Since(runStart).Round(time.Millisecond), len(topics), totalEntries, failedTopics)
	if failedTopics == len(topics) {
		if lastTopicErr == "" {
			lastTopicErr = "unknown fetch error"
		}
		return fmt.Errorf("all topic fetches failed (%d/%d): %s", failedTopics, len(topics), lastTopicErr)
	}
	return nil
}

func (s *Service) logf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Print(msg)
	s.mu.Lock()
	s.lastMessage = msg
	s.lastMessageAt = time.Now()
	s.lastMessages = append(s.lastMessages, progressEntry{Message: msg, At: s.lastMessageAt})
	if len(s.lastMessages) > 2 {
		s.lastMessages = s.lastMessages[len(s.lastMessages)-2:]
	}
	s.mu.Unlock()
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

func (s *Service) fetchTopic(ctx context.Context, q string) ([]searxEntry, error) {
	var lastErr error
	instances := append([]string(nil), s.cfg.SearxngInstances...)
	s.rand.Shuffle(len(instances), func(i, j int) { instances[i], instances[j] = instances[j], instances[i] })
	rateLimited := 0
	noResults := 0

	for _, base := range instances {
		if wait, blocked := s.blockRemaining(base); blocked {
			lastErr = fmt.Errorf("instance %s temporarily blocked for %s after previous 429", base, wait.Round(time.Second))
			continue
		}
		results, retryAfter, err := s.fetchHarvestFromInstance(ctx, base, q)
		if err == nil && len(results) > 0 {
			return results, nil
		}
		if errors.Is(err, errNoResults) || (err == nil && len(results) == 0) {
			noResults++
			continue
		}
		if retryAfter > 0 {
			rateLimited++
			s.setBlocked(base, retryAfter)
			lastErr = fmt.Errorf("instance %s rate-limited (429), retry after %s", base, retryAfter.Round(time.Second))
			continue
		}
		if err != nil {
			lastErr = err
		}
	}
	if len(instances) > 0 && noResults == len(instances) {
		// Distinguish "query had no matches" from real fetch errors.
		return nil, nil
	}
	if rateLimited == len(instances) && len(instances) > 0 {
		return nil, fmt.Errorf("all configured searx instances are rate-limited; add more instances or increase per_query_delay_seconds")
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no searx instance available")
	}
	return nil, lastErr
}

func (s *Service) fetchHarvestFromInstance(ctx context.Context, base, q string) ([]searxEntry, time.Duration, error) {
	categories := []string{"news", ""}
	timeRanges := []string{"day", "week"}
	pages := []int{1, 2}
	const count = 50

	out := make([]searxEntry, 0, 128)
	seen := make(map[string]struct{}, 256)
	var lastErr error

	for _, category := range categories {
		for _, timeRange := range timeRanges {
			for _, page := range pages {
				results, retryAfter, err := s.fetchFromInstance(ctx, base, q, category, timeRange, page, count)
				if retryAfter > 0 {
					return nil, retryAfter, err
				}
				if err != nil {
					lastErr = err
					continue
				}
				for _, r := range results {
					key := strings.TrimSpace(r.URL)
					if key == "" {
						continue
					}
					if _, ok := seen[key]; ok {
						continue
					}
					seen[key] = struct{}{}
					out = append(out, r)
				}
			}
		}
	}
	if len(out) == 0 && lastErr != nil {
		return nil, 0, lastErr
	}
	if len(out) == 0 {
		return nil, 0, errNoResults
	}
	return out, 0, nil
}

func (s *Service) fetchFromInstance(ctx context.Context, base, q, category, timeRange string, page, count int) ([]searxEntry, time.Duration, error) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil {
		return nil, 0, err
	}
	u.Path = path.Join(u.Path, "/search")
	params := u.Query()
	params.Set("q", q)
	params.Set("time_range", timeRange)
	params.Set("format", "json")
	params.Set("pageno", strconv.Itoa(page))
	params.Set("count", strconv.Itoa(count))
	if category != "" {
		params.Set("categories", category)
	}
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "discover/0.3")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, retryAfterDuration(resp.Header.Get("Retry-After")), fmt.Errorf("status %d", resp.StatusCode)
	}
	if resp.StatusCode >= 300 {
		return nil, 0, fmt.Errorf("status %d", resp.StatusCode)
	}
	var parsed searxResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, 0, err
	}
	return parsed.Results, 0, nil
}

func retryAfterDuration(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 2 * time.Minute
	}
	if seconds, err := strconv.Atoi(v); err == nil {
		if seconds < 1 {
			return 30 * time.Second
		}
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(v); err == nil {
		d := time.Until(when)
		if d < 30*time.Second {
			return 30 * time.Second
		}
		return d
	}
	return 2 * time.Minute
}

func (s *Service) blockRemaining(instance string) (time.Duration, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	until, ok := s.instanceBlock[instance]
	if !ok {
		return 0, false
	}
	remaining := time.Until(until)
	if remaining <= 0 {
		delete(s.instanceBlock, instance)
		return 0, false
	}
	return remaining, true
}

func (s *Service) setBlocked(instance string, d time.Duration) {
	if d < 30*time.Second {
		d = 30 * time.Second
	}
	s.mu.Lock()
	s.instanceBlock[instance] = time.Now().Add(d)
	s.mu.Unlock()
}

func parsePublished(primary, secondary string) time.Time {
	for _, v := range []string{primary, secondary} {
		v = strings.TrimSpace(v)
		if v == "" || v == "null" {
			continue
		}
		for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
			if t, err := time.Parse(layout, v); err == nil {
				return t.UTC()
			}
		}
	}
	return time.Time{}
}

func normalizeURL(raw string) (normalized, hash, domain string, err error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", "", "", err
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", "", "", fmt.Errorf("unsupported URL scheme")
	}
	u.Fragment = ""
	u.Host = strings.ToLower(u.Host)
	u.Scheme = scheme
	u.RawQuery = ""
	if u.Path == "" {
		u.Path = "/"
	}
	normalized = u.String()
	sum := sha256.Sum256([]byte(normalized))
	hash = hex.EncodeToString(sum[:])
	domain = u.Hostname()
	return normalized, hash, domain, nil
}

func computePenalty(rules []model.NegativeRule, title, content, domain, articleURL string) (float64, []int64) {
	pen := 0.0
	matched := make([]int64, 0, 2)
	for _, r := range rules {
		if matcher.MatchRule(r.Pattern, title, content, domain, articleURL) {
			pen += r.Penalty
			if r.ID > 0 {
				matched = append(matched, r.ID)
			}
		}
	}
	return pen, matched
}

func termBoost(query, title, content string) float64 {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return 0
	}
	boost := 0.0
	for _, term := range strings.Fields(query) {
		if len(term) < 3 {
			continue
		}
		if strings.Contains(strings.ToLower(title), term) {
			boost += 0.35
		}
		if strings.Contains(strings.ToLower(content), term) {
			boost += 0.1
		}
	}
	return boost
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func bestThumbnailURL(values ...string) string {
	for _, raw := range values {
		thumb := normalizeThumbnailURL(raw)
		if thumb != "" {
			return thumb
		}
	}
	return ""
}

func normalizeThumbnailURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "null") {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(raw), "data:image/") {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return ""
	}
	return u.String()
}

func (s *Service) refreshMissingThumbnails(ctx context.Context) (filled int, scanned int) {
	candidates, err := s.store.ListUnreadThumbnailCandidates(ctx, s.cfg.ThumbnailRefreshMinScore, s.cfg.ThumbnailRefreshMaxPerRun)
	if err != nil {
		s.logf("ingest: thumbnail candidate query error: %v", err)
		return 0, 0
	}
	for _, c := range candidates {
		select {
		case <-ctx.Done():
			return filled, scanned
		default:
		}
		scanned++
		thumb, err := s.fetchThumbnailFromArticle(ctx, c.URL)
		if err != nil || thumb == "" {
			continue
		}
		ok, err := s.store.SetThumbnailIfEmpty(ctx, c.ID, thumb)
		if err != nil {
			continue
		}
		if ok {
			filled++
		}
	}
	return filled, scanned
}

func (s *Service) fetchThumbnailFromArticle(ctx context.Context, articleURL string) (string, error) {
	articleURL = strings.TrimSpace(articleURL)
	if articleURL == "" {
		return "", nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, articleURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "discover/0.3")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	baseURL, _ := url.Parse(articleURL)

	// 1) Open Graph / Twitter image tags from meta headers.
	for _, tag := range reMetaTag.FindAllString(string(body), -1) {
		attrs := parseHTMLAttrs(tag)
		prop := strings.ToLower(strings.TrimSpace(firstNonEmpty(attrs["property"], attrs["name"])))
		if prop != "og:image" && prop != "twitter:image" && prop != "twitter:image:src" {
			continue
		}
		candidate := strings.TrimSpace(attrs["content"])
		if out := resolveThumbnailCandidate(baseURL, candidate); out != "" {
			return out, nil
		}
	}
	// 2) link rel=image_src fallback.
	for _, tag := range reLinkTag.FindAllString(string(body), -1) {
		attrs := parseHTMLAttrs(tag)
		rel := strings.ToLower(strings.TrimSpace(attrs["rel"]))
		if rel != "image_src" && rel != "preload image" {
			continue
		}
		candidate := strings.TrimSpace(firstNonEmpty(attrs["href"], attrs["content"]))
		if out := resolveThumbnailCandidate(baseURL, candidate); out != "" {
			return out, nil
		}
	}
	return "", nil
}

func parseHTMLAttrs(tag string) map[string]string {
	out := make(map[string]string, 8)
	for _, m := range reAttrKV.FindAllStringSubmatch(tag, -1) {
		if len(m) < 5 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(m[1]))
		v := ""
		if m[3] != "" {
			v = m[3]
		} else if m[4] != "" {
			v = m[4]
		}
		if k != "" && v != "" {
			out[k] = strings.TrimSpace(v)
		}
	}
	return out
}

func resolveThumbnailCandidate(base *url.URL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(raw), "data:image/") {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if base != nil {
		u = base.ResolveReference(u)
	}
	return normalizeThumbnailURL(u.String())
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
