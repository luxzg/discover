// Package urlnorm defines shared article identity for ingestion and migrations.
package urlnorm

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
)

// Normalize preserves unknown query parameters, including identity and signature
// values. Only known tracking keys and the fragment are removed. The original
// query encoding/order is retained because some publishers sign the raw query.
func Normalize(raw string) (normalized, hash, domain string, err error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", "", "", errors.New("invalid article URL")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Opaque != "" {
		return "", "", "", errors.New("article URL requires HTTP(S), host, and no userinfo")
	}
	u.Fragment, u.RawFragment = "", ""
	u.Host = strings.ToLower(u.Host)
	parts := strings.Split(u.RawQuery, "&")
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		key, _, _ := strings.Cut(part, "=")
		decoded, err := url.QueryUnescape(key)
		if err == nil && isTracker(strings.ToLower(decoded)) {
			continue
		}
		kept = append(kept, part)
	}
	u.RawQuery = strings.Join(kept, "&")
	if u.Path == "" {
		u.Path = "/"
	}
	normalized = u.String()
	sum := sha256.Sum256([]byte(normalized))
	return normalized, hex.EncodeToString(sum[:]), u.Hostname(), nil
}

func isTracker(key string) bool {
	switch key {
	case "utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content", "utm_id",
		"utm_source_platform", "utm_creative_format", "utm_marketing_tactic",
		"fbclid", "gclid", "dclid", "msclkid", "gbraid", "wbraid", "mc_cid", "mc_eid", "igshid", "_ga", "_gl":
		return true
	}
	return false
}
