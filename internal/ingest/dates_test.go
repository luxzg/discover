package ingest

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"discover/internal/config"
	"discover/internal/db"
	"discover/internal/model"
	"discover/internal/store"
)

func TestPublicationMetadata(t *testing.T) {
	for _, tc := range []struct{ html, want string }{
		{`<meta property="article:published_time" content="2026-05-01T10:00:00+02:00">`, "2026-05-01T08:00:00Z"},
		{`<script type="application/ld+json">{"@graph":[{"@type":"NewsArticle","datePublished":"2026-05-02","dateModified":"2026-09-01"}]}</script>`, "2026-05-02T00:00:00Z"},
		{`<article><time itemprop="datePublished" datetime="2026-05-03"></time></article>`, "2026-05-03T00:00:00Z"},
		{`<script type="application/ld+json">{"@type":"NewsArticle","dateModified":"2026-09-01"}</script>`, ""},
		{`<script type="application/ld+json">{"@type":"WebSite","datePublished":"2026-01-01"}</script>`, ""},
		{`<meta property="article:published_time" content="2099-01-01">`, ""},
		{`<script type="application/ld+json">invalid</script>`, ""},
		{`<script type="application/ld+json">{"@graph":[{"@type":"NewsArticle","datePublished":"2026-05-02"},{"@type":"NewsArticle","datePublished":"2026-05-03"}]}</script>`, ""},
		{`<script type="application/ld+json">{"@type":"NewsArticle","datePublished":"2026-05-02"}</script><script type="application/ld+json">{"@type":"NewsArticle","datePublished":"2026-05-03"}</script>`, ""},
		{`<meta property="article:published_time" content="2026-05-01"><script type="application/ld+json">{"@type":"NewsArticle","datePublished":"2026-05-02"}</script>`, "2026-05-01T00:00:00Z"},
	} {
		d := publicationFromHTML([]byte(tc.html))
		got := ""
		if !d.IsZero() {
			got = d.Format("2006-01-02T15:04:05Z07:00")
		}
		if got != tc.want {
			t.Fatalf("%s: got %s want %s", tc.html, got, tc.want)
		}
	}
}

func TestSharedMetadataFetchAndPersistentCooldown(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "metadata.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	st := store.New(d)
	ctx := context.Background()
	if err := st.Prepare(ctx, 50); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertTopic(ctx, model.Topic{Query: "news", Weight: 100, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	topics, _ := st.ListTopics(ctx)
	if err := st.UpsertArticleHit(ctx, store.UpsertArticleInput{URL: "http://publisher.test/article", URLHash: "one", Title: "Synthetic article", TopicID: topics[0].ID, IngestedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		fmt.Fprint(w, `<meta property="og:image" content="/image"><meta property="article:published_time" content="2026-05-01">`)
	}))
	defer server.Close()
	s := New(config.Config{FeedMinScore: 1, ThumbnailRefreshMinScore: 60, ThumbnailRefreshMaxPerRun: 1, DateRefreshMaxPerRun: 1}, st)
	s.articleClient = articleTestClient(t, server.Listener.Addr().String())
	images, dates, scanned, err := s.refreshMissingMetadata(ctx)
	if err != nil || images != 1 || dates != 1 || scanned != 1 || hits != 1 {
		t.Fatal(images, dates, scanned, hits, err)
	}
	_, _, scanned, err = s.refreshMissingMetadata(ctx)
	if err != nil || scanned != 0 || hits != 1 {
		t.Fatal("metadata refetched", scanned, hits, err)
	}
}

func TestPublicationScope(t *testing.T) {
	for _, tc := range []struct{ name, page, want string }{
		{"sidebar", `<head><script type="application/ld+json">{"@type":"NewsArticle","datePublished":"2026-05-02"}</script></head><body><aside><time itemprop="datePublished" datetime="2020-01-01"></time></aside></body>`, "2026-05-02"},
		{"body meta", `<body><nav><meta itemprop="datePublished" content="2020-01-01"></nav></body>`, ""},
		{"multiple articles", `<article><time itemprop="datePublished" datetime="2020-01-01"></time></article><article><time itemprop="datePublished" datetime="2026-05-02"></time></article>`, ""},
		{"comment", `<article><article><time itemprop="datePublished" datetime="2020-01-01"></time></article><time itemprop="datePublished" datetime="2026-05-02"></time></article>`, "2026-05-02"},
		{"conflicting head", `<head><meta property="article:published_time" content="2020-01-01"><meta property="article:published_time" content="2026-05-02"></head>`, ""},
		{"unscoped time", `<time itemprop="datePublished" datetime="2020-01-01"></time>`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := publicationFromHTML([]byte(tc.page))
			if tc.want == "" {
				if !got.IsZero() {
					t.Fatal("unexpected publication", got)
				}
				return
			}
			if got.Format("2006-01-02") != tc.want {
				t.Fatal(got, tc.want)
			}
		})
	}
}

func TestMetadataBodyLimitDoesNotPublishTruncatedDates(t *testing.T) {
	const capBytes = 1 << 20
	for _, size := range []int{capBytes - 1, capBytes, capBytes + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			prefix := `<meta property="og:image" content="/image"><meta property="article:published_time" content="2026-05-01">`
			body := prefix + strings.Repeat(" ", size-len(prefix))
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer upstream.Close()
			s := New(config.Config{}, nil)
			s.articleClient = articleTestClient(t, upstream.Listener.Addr().String())
			m, err := s.fetchArticleMetadata(t.Context(), "http://publisher.test/article")
			if size > capBytes {
				if err == nil || !m.Published.IsZero() {
					t.Fatal("truncated page accepted", m.Published, err)
				}
			} else if err != nil || m.Published.IsZero() {
				t.Fatal(m.Published, err)
			}
			if m.Thumbnail != "http://publisher.test/image" {
				t.Fatal(m.Thumbnail)
			}
		})
	}
}
