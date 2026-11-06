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
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return time.Time{}
	}
	var headDates, articleDates, structured []time.Time
	articleRoots := 0
	depthExceeded := false
	var visit func(*html.Node, bool, int, int)
	visit = func(n *html.Node, inHead bool, articleDepth, depth int) {
		if depth > 128 {
			depthExceeded = true
			return
		}
		attrs := map[string]string{}
		for _, a := range n.Attr {
			if _, exists := attrs[a.Key]; !exists {
				attrs[a.Key] = a.Val
			}
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "aside", "nav", "footer", "template":
				return
			case "head":
				inHead = true
			}
			articleScope := n.Data == "article"
			for _, typ := range strings.Fields(attrs["itemtype"]) {
				name := strings.TrimPrefix(strings.TrimPrefix(typ, "https://schema.org/"), "http://schema.org/")
				articleScope = articleScope || name == "Article" || name == "NewsArticle" || name == "BlogPosting"
			}
			if articleScope {
				if articleDepth == 0 {
					articleRoots++
				}
				articleDepth++
			}
			// Nested articles are commonly comments/recommendations, not the page.
			if articleDepth > 1 {
				return
			}
			key := strings.ToLower(firstNonEmpty(attrs["property"], attrs["name"], attrs["itemprop"]))
			publication := n.Data == "meta" && (key == "article:published_time" || key == "datepublished" || key == "pubdate") || n.Data == "time" && key == "datepublished"
			if publication {
				d := metadataDate(firstNonEmpty(attrs["content"], attrs["datetime"]))
				if !d.IsZero() {
					if inHead {
						headDates = append(headDates, d)
					} else if articleDepth == 1 {
						articleDates = append(articleDates, d)
					}
				}
			}
			if n.Data == "script" && strings.EqualFold(strings.TrimSpace(attrs["type"]), "application/ld+json") {
				var text strings.Builder
				for child := n.FirstChild; child != nil; child = child.NextSibling {
					if child.Type == html.TextNode {
						text.WriteString(child.Data)
					}
				}
				var value any
				if json.Unmarshal([]byte(text.String()), &value) == nil {
					structured = append(structured, jsonLDPublications(value, 0)...)
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child, inHead, articleDepth, depth+1)
		}
	}
	visit(doc, false, 0, 0)
	if depthExceeded {
		return time.Time{}
	}
	if len(headDates) > 0 {
		return unambiguousPublication(headDates)
	}
	if articleRoots == 1 {
		structured = append(structured, articleDates...)
	}
	return unambiguousPublication(structured)
}

func unambiguousPublication(dates []time.Time) time.Time {
	if len(dates) == 0 {
		return time.Time{}
	}
	for _, d := range dates[1:] {
		if !dates[0].Equal(d) {
			return time.Time{}
		}
	}
	return dates[0]
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
