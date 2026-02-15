package auth

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
)

type Guard struct {
	secret []byte
	cidrs  []*net.IPNet
}

func New(secret string, allowedCIDRs []string) (*Guard, error) {
	g := &Guard{secret: []byte(strings.TrimSpace(secret))}
	for _, s := range allowedCIDRs {
		_, n, err := net.ParseCIDR(strings.TrimSpace(s))
		if err != nil {
			continue
		}
		g.cidrs = append(g.cidrs, n)
	}
	return g, nil
}

func (g *Guard) AdminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(g.cidrs) > 0 && !g.allowIP(r.RemoteAddr) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		secret := r.Header.Get("X-Admin-Secret")
		if secret == "" {
			secret = r.URL.Query().Get("secret")
		}
		if !g.validSecret(secret) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="admin"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (g *Guard) validSecret(v string) bool {
	vb := []byte(strings.TrimSpace(v))
	if len(vb) == 0 || len(g.secret) == 0 {
		return false
	}
	if len(vb) != len(g.secret) {
		return false
	}
	return subtle.ConstantTimeCompare(vb, g.secret) == 1
}

func (g *Guard) allowIP(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, cidr := range g.cidrs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}
