package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestReportDomain(t *testing.T) {
	for raw, want := range map[string]string{
		"WWW.PCMAG.COM.": "pcmag.com", "au.pcmag.com": "pcmag.com",
		"news.example.co.uk": "example.co.uk", "www.example.com.au": "example.com.au",
		"alice.blogspot.com": "alice.blogspot.com", "bob.blogspot.com": "bob.blogspot.com",
		"localhost": "localhost", "": "", "192.0.2.1": "192.0.2.1",
		"[2001:db8::1]:8080": "2001:db8::1",
	} {
		if got := reportDomain(raw); got != want {
			t.Errorf("%q: got %q want %q", raw, got, want)
		}
	}
}

func TestDomainReportMergesBeforeFiltering(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for i, host := range []string{"www.pcmag.com", "au.pcmag.com", "uk.pcmag.com", "www.single.example"} {
		id := hit(t, s, "https://"+host+"/one", fmt.Sprintf("Distinct report article %d", i), 0, time.Time{})
		if err := s.MarkRead(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := s.DomainReport(ctx)
	if err != nil || len(rows) != 1 || rows[0].Domain != "pcmag.com" || rows[0].Read != 3 || rows[0].Total != 3 || rows[0].Seen != 3 {
		t.Fatal(rows, err)
	}
	var originals int
	if err := s.db.QueryRow(`SELECT COUNT(DISTINCT source_domain) FROM articles`).Scan(&originals); err != nil || originals != 4 {
		t.Fatal("report rewrote stored domains", originals, err)
	}
}

func TestDomainReportCapAfterAggregation(t *testing.T) {
	s := testStore(t)
	// 201 registrable domains, one read at each of two subdomains. Limiting
	// the host query first would omit valid merged groups or miscount their reads.
	_, err := s.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<402)
 INSERT INTO articles(url,normalized_url,url_hash,title,source_domain,ingested_at,status,read_at)
 SELECT 'https://fixture.test/'||x,'https://fixture.test/'||x,'report-'||x,'Article '||x,
 CASE WHEN x%2=0 THEN 'www.' ELSE 'news.' END||printf('site%03d.com',(x+1)/2),CURRENT_TIMESTAMP,'read',CURRENT_TIMESTAMP FROM n`)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.DomainReport(context.Background())
	if err != nil || len(rows) != 200 || rows[0].Domain != "site001.com" || rows[199].Domain != "site200.com" {
		t.Fatal("cap or stable tie ordering", len(rows), err)
	}
	for _, row := range rows {
		if row.Read != 2 || row.Positive != 2 || row.Total != 2 {
			t.Fatal("partial domain counts", row)
		}
	}
}
