package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"discover/internal/matcher"
)

type searxResponse struct {
	Query               string            `json:"query"`
	Results             []searxEntry      `json:"results"`
	UnresponsiveEngines []json.RawMessage `json:"unresponsive_engines"`
	Error               json.RawMessage   `json:"error"`
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

func (s *Service) fetchTopic(ctx context.Context, q string) ([]searxEntry, error) {
	type instance struct {
		base  string
		index int
	}
	instances := make([]instance, 0, len(s.cfg.SearxngInstances))
	for i, base := range s.cfg.SearxngInstances {
		instances = append(instances, instance{base, i + 1})
	}
	s.rand.Shuffle(len(instances), func(i, j int) { instances[i], instances[j] = instances[j], instances[i] })
	failures := &PartialRunError{}
	out := make([]searxEntry, 0)
	seen := make(map[string]int)
	if len(instances) == 0 {
		failures.add(Failure{Stage: "fetch", Code: "no_instances"})
	}
	for _, instance := range instances {
		base := instance.base
		if ctx.Err() != nil {
			failures.add(dbFailure("fetch", ctx.Err()))
			break
		}
		if _, blocked := s.blockRemaining(base); blocked {
			failures.add(Failure{Stage: "fetch", Code: "instance_cooldown", Instance: instance.index})
			continue
		}
		results, retryAfter, err := s.fetchHarvestFromInstance(ctx, base, q)
		mergeEntries(&out, seen, results)
		failures.merge(err, 0, instance.index)
		if retryAfter > 0 {
			s.setBlocked(base, retryAfter)
		}
		if err == nil {
			return out, failures.err()
		}
	}
	return out, failures.err()
}

func (s *Service) fetchHarvestFromInstance(ctx context.Context, base, q string) ([]searxEntry, time.Duration, error) {
	out := make([]searxEntry, 0, 128)
	seen := make(map[string]int, 256)
	failures := &PartialRunError{}
	// This is deliberately a fixed eight-request harvest, not crawl-until-empty.
	for _, category := range []string{"news", "general"} {
		for _, timeRange := range []string{"day", "week"} {
			for _, page := range []int{1, 2} {
				if ctx.Err() != nil {
					failures.add(dbFailure("fetch", ctx.Err()))
					return out, 0, failures.err()
				}
				results, retryAfter, err := s.fetchFromInstance(ctx, base, q, category, timeRange, page)
				mergeEntries(&out, seen, results)
				if err != nil {
					code := errorCode(err)
					if f, ok := err.(Failure); ok {
						code = f.Code
					}
					failures.add(Failure{Stage: "fetch", Code: code, Category: category, TimeRange: timeRange, Page: page, cause: err})
				}
				if retryAfter > 0 {
					return out, retryAfter, failures.err()
				}
			}
		}
	}
	return out, 0, failures.err()
}

func mergeEntries(out *[]searxEntry, seen map[string]int, entries []searxEntry) {
	for _, entry := range entries {
		key, _, _, err := normalizeURL(entry.URL)
		if err != nil {
			continue
		}
		if index, ok := seen[key]; ok {
			existing := &(*out)[index]
			if strings.TrimSpace(existing.Title) == "" {
				existing.Title = entry.Title
			}
			if len(strings.TrimSpace(entry.Content)) > len(strings.TrimSpace(existing.Content)) {
				existing.Content = entry.Content
			}
			existing.Thumbnail = bestThumbnailURL(existing.Thumbnail, entry.Thumbnail)
			existing.ImgSrc = bestThumbnailURL(existing.ImgSrc, entry.ImgSrc)
			if parsePublished(existing.PublishedDate, existing.Pubdate).IsZero() && !parsePublished(entry.PublishedDate, entry.Pubdate).IsZero() {
				existing.PublishedDate, existing.Pubdate = entry.PublishedDate, entry.Pubdate
			}
			for _, engine := range entry.Engines {
				found := false
				for _, old := range existing.Engines {
					if engine == old {
						found = true
						break
					}
				}
				if !found {
					existing.Engines = append(existing.Engines, engine)
				}
			}
			if entry.Score > existing.Score {
				existing.Score = entry.Score
			}
			continue
		}
		seen[key] = len(*out)
		*out = append(*out, entry)
	}
}

func (s *Service) fetchFromInstance(ctx context.Context, base, q, category, timeRange string, page int) ([]searxEntry, time.Duration, error) {
	fail := func(code string, err error) ([]searxEntry, time.Duration, error) {
		return nil, 0, Failure{Stage: "fetch", Code: code, cause: err}
	}
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil {
		return fail("invalid_instance", err)
	}
	if u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fail("invalid_instance", nil)
	}
	u.Path = path.Join(u.Path, "search")
	u.RawPath = ""
	params := u.Query()
	for key := range params {
		if strings.HasPrefix(key, "category_") {
			params.Del(key)
		}
	}
	params.Del("count") // Not a SearXNG search API parameter.
	params.Set("q", matcher.NormalizeQuery(q))
	params.Set("time_range", timeRange)
	params.Set("format", "json")
	params.Set("pageno", strconv.Itoa(page))
	params.Set("categories", category)
	u.RawQuery = params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fail("invalid_request", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "discover")
	resp, err := s.client.Do(req)
	if err != nil {
		return fail(errorCode(err), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, retryAfterDuration(resp.Header.Get("Retry-After")), Failure{Stage: "fetch", Code: "http_429"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fail(fmt.Sprintf("http_%d", resp.StatusCode), nil)
	}
	const maxBody = 4 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return fail("response_read", err)
	}
	if len(body) > maxBody {
		return fail("response_too_large", nil)
	}
	var parsed searxResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fail("invalid_json", err)
	}
	if len(parsed.Error) > 0 && string(parsed.Error) != "null" && string(parsed.Error) != `""` {
		return parsed.Results, 0, Failure{Stage: "fetch", Code: "search_error"}
	}
	if len(parsed.UnresponsiveEngines) > 0 {
		// Remote engine names/messages can contain arbitrary data. Only emit a count.
		return parsed.Results, 0, Failure{Stage: "fetch", Code: fmt.Sprintf("engine_errors_%d", len(parsed.UnresponsiveEngines))}
	}
	if parsed.Results == nil {
		// Null/missing results is not the same contract as a successful empty array.
		return fail("missing_results", nil)
	}
	return parsed.Results, 0, nil
}

func retryAfterDuration(v string) time.Duration {
	const maximum = 24 * time.Hour
	v = strings.TrimSpace(v)
	if seconds, err := strconv.ParseInt(v, 10, 64); err == nil {
		if seconds < 30 {
			return 30 * time.Second
		}
		if seconds > int64(maximum/time.Second) {
			return maximum
		}
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(v); err == nil {
		d := time.Until(when)
		if d < 30*time.Second {
			return 30 * time.Second
		}
		if d > maximum {
			return maximum
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
	defer s.mu.Unlock()
	s.instanceBlock[instance] = time.Now().Add(d)
}
