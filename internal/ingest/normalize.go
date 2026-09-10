package ingest

import (
	"strings"
	"time"

	"discover/internal/matcher"
	"discover/internal/urlnorm"
)

func normalizeURL(raw string) (normalized, hash, domain string, err error) {
	return urlnorm.Normalize(raw)
}

func parsePublished(primary, secondary string) time.Time {
	for _, v := range []string{primary, secondary} {
		v = strings.TrimSpace(v)
		if v == "" || strings.EqualFold(v, "null") {
			continue
		}
		for _, layout := range []string{
			time.RFC3339Nano, time.RFC1123Z, time.RFC822Z,
			"2006-01-02 15:04:05Z07:00", "2006-01-02 15:04:05 -0700 MST",
			"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02",
		} {
			if t, err := time.Parse(layout, v); err == nil {
				return t.UTC()
			}
		}
		// Abbreviations are otherwise interpreted as fabricated zero-offset zones.
		fields := strings.Fields(v)
		if len(fields) > 0 {
			zones := map[string]int{"UTC": 0, "GMT": 0, "EST": -5 * 3600, "EDT": -4 * 3600, "CST": -6 * 3600, "CDT": -5 * 3600, "MST": -7 * 3600, "MDT": -6 * 3600, "PST": -8 * 3600, "PDT": -7 * 3600}
			zone := fields[len(fields)-1]
			if offset, ok := zones[zone]; ok {
				for _, layout := range []string{time.RFC1123, time.RFC822} {
					if t, err := time.ParseInLocation(layout, v, time.FixedZone(zone, offset)); err == nil {
						return t.UTC()
					}
				}
			}
		}
	}
	// Zero means absent/invalid, never substitute ingestion time for publication.
	return time.Time{}
}

func termBoost(query, title, content string) float64 {
	query = strings.ToLower(matcher.NormalizeQuery(query))
	title, content = strings.ToLower(title), strings.ToLower(content)
	boost := 0.0
	for _, term := range strings.Fields(query) {
		if len(term) < 3 {
			continue
		}
		if strings.Contains(title, term) {
			boost += 0.35
		}
		if strings.Contains(content, term) {
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
