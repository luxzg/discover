package ingest

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

func bestThumbnailURL(values ...string) string {
	for _, raw := range values {
		if thumb := normalizeThumbnailURL(raw); thumb != "" {
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
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return ""
	}
	return u.String()
}

func (s *Service) refreshMissingThumbnails(ctx context.Context) (filled, scanned int, err error) {
	failures := &PartialRunError{}
	candidates, err := s.store.ListUnreadThumbnailCandidates(ctx, s.cfg.ThumbnailRefreshMinScore, s.cfg.ThumbnailRefreshMaxPerRun)
	if err != nil {
		failures.add(dbFailure("thumbnail_candidates", err))
		return 0, 0, failures.err()
	}
	for _, c := range candidates {
		if scanned >= s.cfg.ThumbnailRefreshMaxPerRun {
			break
		}
		if ctx.Err() != nil {
			failures.add(dbFailure("thumbnail_fetch", ctx.Err()))
			break
		}
		scanned++
		thumb, err := s.fetchThumbnailFromArticle(ctx, c.URL)
		if err != nil {
			f := Failure{Stage: "thumbnail_fetch", Code: errorCode(err), ArticleID: c.ID, cause: err}
			if remote, ok := err.(Failure); ok {
				f.Code = remote.Code
			}
			failures.add(f)
			continue
		}
		if thumb == "" {
			continue
		}
		ok, err := s.store.SetThumbnailIfEmpty(ctx, c.ID, thumb)
		if err != nil {
			f := dbFailure("thumbnail_update", err)
			f.ArticleID = c.ID
			failures.add(f)
			continue
		}
		if ok {
			filled++
		}
	}
	return filled, scanned, failures.err()
}

func (s *Service) fetchThumbnailFromArticle(ctx context.Context, articleURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(articleURL), nil)
	if err != nil {
		return "", Failure{Stage: "thumbnail_fetch", Code: "invalid_url", cause: err}
	}
	if err := validateArticleURL(req.URL); err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "discover")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := s.articleClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", Failure{Stage: "thumbnail_fetch", Code: fmt.Sprintf("http_%d", resp.StatusCode)}
	}
	const maxBody = 1 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return "", Failure{Stage: "thumbnail_fetch", Code: "response_read", cause: err}
	}
	// Resolve against the final validated redirect target, not the starting URL.
	return thumbnailFromHTML(resp.Request.URL, body), nil
}

func thumbnailFromHTML(base *url.URL, body []byte) string {
	tokenizer := html.NewTokenizer(bytes.NewReader(body))
	fallback := ""
	baseSeen := false
	for {
		typ := tokenizer.Next()
		if typ == html.ErrorToken {
			return fallback
		}
		if typ != html.StartTagToken && typ != html.SelfClosingTagToken {
			continue
		}
		token := tokenizer.Token()
		if token.Data != "meta" && token.Data != "link" && token.Data != "base" {
			continue
		}
		attrs := make(map[string]string, len(token.Attr))
		for _, attr := range token.Attr {
			// HTML uses the first duplicate attribute. Token() already unescapes
			// entities once, with HTML attribute-context rules.
			if _, exists := attrs[attr.Key]; !exists {
				attrs[attr.Key] = strings.TrimSpace(attr.Val)
			}
		}
		if token.Data == "base" {
			if !baseSeen && attrs["href"] != "" {
				baseSeen = true
				if candidate, err := url.Parse(attrs["href"]); err == nil {
					if base != nil {
						candidate = base.ResolveReference(candidate)
					}
					if candidate.Scheme == "http" || candidate.Scheme == "https" {
						base = candidate
					}
				}
			}
			continue
		}
		if token.Data == "meta" {
			prop := strings.ToLower(firstNonEmpty(attrs["property"], attrs["name"]))
			if prop == "og:image" || prop == "twitter:image" || prop == "twitter:image:src" {
				if out := resolveThumbnailCandidate(base, attrs["content"]); out != "" {
					return out
				}
			}
		} else if fallback == "" {
			rels := strings.Fields(strings.ToLower(attrs["rel"]))
			for _, rel := range rels {
				if rel == "image_src" || (rel == "preload" && strings.EqualFold(attrs["as"], "image")) {
					fallback = resolveThumbnailCandidate(base, attrs["href"])
				}
			}
		}
	}
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
	// This only records a browser image URL. It never fetches/proxies the image.
	return normalizeThumbnailURL(u.String())
}
