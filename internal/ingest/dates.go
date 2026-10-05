package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"golang.org/x/net/html"
)

func publicationFromHTML(body []byte) time.Time {
	z := html.NewTokenizer(bytes.NewReader(body))
	var explicit, structured time.Time
	ambiguousStructured := false
	for {
		typ := z.Next()
		if typ == html.ErrorToken {
			break
		}
		if typ != html.StartTagToken && typ != html.SelfClosingTagToken {
			continue
		}
		t := z.Token()
		attrs := map[string]string{}
		for _, a := range t.Attr {
			if _, ok := attrs[a.Key]; !ok {
				attrs[a.Key] = a.Val
			}
		}
		key := strings.ToLower(firstNonEmpty(attrs["property"], attrs["name"], attrs["itemprop"]))
		if (t.Data == "meta" && (key == "article:published_time" || key == "datepublished" || key == "pubdate")) || (t.Data == "time" && strings.EqualFold(attrs["itemprop"], "datePublished")) {
			if d := metadataDate(firstNonEmpty(attrs["content"], attrs["datetime"])); explicit.IsZero() && !d.IsZero() {
				explicit = d
			}
		}
		if t.Data == "script" && strings.EqualFold(strings.TrimSpace(attrs["type"]), "application/ld+json") && z.Next() == html.TextToken {
			var value any
			if json.Unmarshal(z.Text(), &value) == nil {
				for _, d := range jsonLDPublications(value, 0) {
					if !structured.IsZero() && !structured.Equal(d) {
						ambiguousStructured = true
					}
					structured = d
				}
			}
		}
	}
	if !explicit.IsZero() {
		return explicit
	}
	if !ambiguousStructured {
		return structured
	}
	return time.Time{}
}

func metadataDate(raw string) time.Time {
	d := parsePublished(raw, "")
	if d.Year() < 2000 || d.After(time.Now().Add(24*time.Hour)) {
		return time.Time{}
	}
	return d
}

func jsonLDPublications(v any, depth int) []time.Time {
	if depth > 16 {
		return nil
	}
	var dates []time.Time
	switch x := v.(type) {
	case []any:
		for _, item := range x {
			dates = append(dates, jsonLDPublications(item, depth+1)...)
		}
	case map[string]any:
		isArticle := false
		types := []any{x["@type"]}
		if many, ok := x["@type"].([]any); ok {
			types = many
		}
		for _, typ := range types {
			name, _ := typ.(string)
			name = strings.TrimPrefix(strings.TrimPrefix(name, "https://schema.org/"), "http://schema.org/")
			if name == "Article" || name == "NewsArticle" || name == "BlogPosting" {
				isArticle = true
			}
		}
		if isArticle {
			if raw, ok := x["datePublished"].(string); ok {
				if d := metadataDate(raw); !d.IsZero() {
					return []time.Time{d}
				}
			}
		}
		// Do not take a related/recommended article's publication date.
		for _, key := range []string{"mainEntity", "@graph"} {
			dates = append(dates, jsonLDPublications(x[key], depth+1)...)
		}
	}
	return dates
}

func (s *Service) refreshMissingMetadata(ctx context.Context) (images, dates, scanned int, err error) {
	failures := &PartialRunError{}
	candidates, err := s.store.ListMetadataCandidates(ctx, s.cfg.ThumbnailRefreshMinScore, s.cfg.FeedMinScore, s.cfg.ThumbnailRefreshMaxPerRun, s.cfg.DateRefreshMaxPerRun)
	if err != nil {
		failures.add(dbFailure("metadata_candidates", err))
		return 0, 0, 0, failures.err()
	}
	for _, c := range candidates {
		if ctx.Err() != nil {
			failures.add(dbFailure("metadata_fetch", ctx.Err()))
			break
		}
		scanned++
		m, fetchErr := s.fetchArticleMetadata(ctx, c.URL)
		if fetchErr != nil {
			f := Failure{Stage: "metadata_fetch", Code: errorCode(fetchErr), ArticleID: c.ID, cause: fetchErr}
			var remote Failure
			if errors.As(fetchErr, &remote) {
				f.Code = remote.Code
			}
			failures.add(f)
		}
		imageFilled, dateFilled, writeErr := s.store.RecordMetadata(ctx, c.ID, m.Thumbnail, m.Published)
		if writeErr != nil {
			f := dbFailure("metadata_update", writeErr)
			f.ArticleID = c.ID
			failures.add(f)
			continue
		}
		if imageFilled {
			images++
		}
		if dateFilled {
			dates++
		}
	}
	return images, dates, scanned, failures.err()
}
