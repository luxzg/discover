package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const SessionCookieName = "discover_admin_session"

type Guard struct {
	secret []byte
	cidrs  []*net.IPNet

	mu       sync.Mutex
	sessions map[string]session
	attempts map[string]attempt
}

type session struct {
	ExpiresAt time.Time
	RemoteIP  string
}

type attempt struct {
	Fails        int
	WindowStart  time.Time
	BlockedUntil time.Time
}

var (
	ErrBlocked      = errors.New("too many failed auth attempts; try again later")
	ErrUnauthorized = errors.New("unauthorized")
)

func New(secret string, allowedCIDRs []string) (*Guard, error) {
	g := &Guard{
		secret:   []byte(strings.TrimSpace(secret)),
		sessions: make(map[string]session),
		attempts: make(map[string]attempt),
	}
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
		w.Header().Set("Referrer-Policy", "no-referrer")
		ip := remoteIP(r.RemoteAddr)
		if len(g.cidrs) > 0 && !g.allowIP(ip) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		if token, err := r.Cookie(SessionCookieName); err == nil && g.validSession(token.Value, ip) {
			next.ServeHTTP(w, r)
			return
		}

		secret := r.Header.Get("X-Admin-Secret")
		if strings.TrimSpace(secret) != "" {
			err := g.ValidateSecret(secret, ip)
			if err == nil {
				next.ServeHTTP(w, r)
				return
			}
			if errors.Is(err, ErrBlocked) {
				http.Error(w, err.Error(), http.StatusTooManyRequests)
				return
			}
		}

		w.Header().Set("WWW-Authenticate", `Bearer realm="admin"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

func (g *Guard) ValidateSecret(secret, remoteAddr string) error {
	ip := remoteIP(remoteAddr)
	if ip == "" {
		return ErrUnauthorized
	}
	if g.isBlocked(ip) {
		return ErrBlocked
	}
	if g.validSecret(secret) {
		g.clearAttempts(ip)
		return nil
	}
	g.recordFailure(ip)
	return ErrUnauthorized
}

func (g *Guard) AllowRemote(remoteAddr string) bool {
	ip := remoteIP(remoteAddr)
	if ip == "" {
		return false
	}
	if len(g.cidrs) == 0 {
		return true
	}
	return g.allowIP(ip)
}

func (g *Guard) NewSession(remoteAddr string, ttl time.Duration) (string, time.Time, error) {
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	tok := make([]byte, 32)
	if _, err := rand.Read(tok); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(tok)
	expires := time.Now().Add(ttl)
	ip := remoteIP(remoteAddr)
	g.mu.Lock()
	g.gcLocked()
	g.sessions[token] = session{ExpiresAt: expires, RemoteIP: ip}
	g.mu.Unlock()
	return token, expires, nil
}

func (g *Guard) DeleteSession(token string) {
	if token == "" {
		return
	}
	g.mu.Lock()
	delete(g.sessions, token)
	g.mu.Unlock()
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

func (g *Guard) validSession(token, remoteIP string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.gcLocked()
	s, ok := g.sessions[token]
	if !ok {
		return false
	}
	if s.RemoteIP != "" && remoteIP != "" && s.RemoteIP != remoteIP {
		return false
	}
	return true
}

func (g *Guard) isBlocked(ip string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	a, ok := g.attempts[ip]
	if !ok {
		return false
	}
	if !a.BlockedUntil.IsZero() && time.Now().Before(a.BlockedUntil) {
		return true
	}
	if !a.BlockedUntil.IsZero() && time.Now().After(a.BlockedUntil) {
		a.BlockedUntil = time.Time{}
		a.Fails = 0
		a.WindowStart = time.Time{}
		g.attempts[ip] = a
	}
	return false
}

func (g *Guard) recordFailure(ip string) {
	const window = 5 * time.Minute
	const blockFor = 10 * time.Minute
	const maxFails = 5

	now := time.Now()
	g.mu.Lock()
	defer g.mu.Unlock()
	a := g.attempts[ip]
	if a.WindowStart.IsZero() || now.Sub(a.WindowStart) > window {
		a.WindowStart = now
		a.Fails = 1
		a.BlockedUntil = time.Time{}
	} else {
		a.Fails++
	}
	if a.Fails >= maxFails {
		a.BlockedUntil = now.Add(blockFor)
	}
	g.attempts[ip] = a
}

func (g *Guard) clearAttempts(ip string) {
	g.mu.Lock()
	delete(g.attempts, ip)
	g.mu.Unlock()
}

func (g *Guard) gcLocked() {
	now := time.Now()
	for token, s := range g.sessions {
		if now.After(s.ExpiresAt) {
			delete(g.sessions, token)
		}
	}
}

func (g *Guard) allowIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
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

func remoteIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return ""
	}
	return ip.String()
}
