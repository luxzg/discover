package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"time"
)

const UserSessionCookieName = "discover_user_session"

type UserGuard struct {
	username string
	secret   []byte

	mu       sync.Mutex
	sessions map[string]userSession
	attempts map[string]userAttempt
}

type userSession struct {
	ExpiresAt time.Time
	RemoteIP  string
}

type userAttempt struct {
	Fails        int
	WindowStart  time.Time
	BlockedUntil time.Time
}

var (
	ErrUserBlocked      = errors.New("too many failed sign-in attempts; try again later")
	ErrUserUnauthorized = errors.New("invalid credentials")
)

func NewUserGuard(username, secret string) (*UserGuard, error) {
	username = strings.TrimSpace(username)
	secret = strings.TrimSpace(secret)
	if username == "" || secret == "" {
		return nil, errors.New("user_name and user_secret are required")
	}
	return &UserGuard{
		username: username,
		secret:   []byte(secret),
		sessions: make(map[string]userSession),
		attempts: make(map[string]userAttempt),
	}, nil
}

func (g *UserGuard) ValidateCredentials(username, secret, remoteAddr string) error {
	ip := remoteIP(remoteAddr)
	if ip == "" {
		return ErrUserUnauthorized
	}
	if g.isBlocked(ip) {
		return ErrUserBlocked
	}
	if g.validUsername(username) && g.validSecret(secret) {
		g.clearAttempts(ip)
		return nil
	}
	g.recordFailure(ip)
	return ErrUserUnauthorized
}

func (g *UserGuard) NewSession(remoteAddr string, ttl time.Duration) (string, time.Time, error) {
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour
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
	g.sessions[token] = userSession{ExpiresAt: expires, RemoteIP: ip}
	g.mu.Unlock()
	return token, expires, nil
}

func (g *UserGuard) DeleteSession(token string) {
	if token == "" {
		return
	}
	g.mu.Lock()
	delete(g.sessions, token)
	g.mu.Unlock()
}

func (g *UserGuard) ValidSession(token, remoteAddr string) bool {
	ip := remoteIP(remoteAddr)
	g.mu.Lock()
	defer g.mu.Unlock()
	g.gcLocked()
	s, ok := g.sessions[token]
	if !ok {
		return false
	}
	if s.RemoteIP != "" && ip != "" && s.RemoteIP != ip {
		return false
	}
	return true
}

func (g *UserGuard) validUsername(v string) bool {
	v = strings.TrimSpace(v)
	if len(v) != len(g.username) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(v), []byte(g.username)) == 1
}

func (g *UserGuard) validSecret(v string) bool {
	vb := []byte(strings.TrimSpace(v))
	if len(vb) == 0 || len(g.secret) == 0 {
		return false
	}
	if len(vb) != len(g.secret) {
		return false
	}
	return subtle.ConstantTimeCompare(vb, g.secret) == 1
}

func (g *UserGuard) isBlocked(ip string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	a, ok := g.attempts[ip]
	if !ok {
		return false
	}
	now := time.Now()
	if !a.BlockedUntil.IsZero() && now.Before(a.BlockedUntil) {
		return true
	}
	if !a.BlockedUntil.IsZero() && now.After(a.BlockedUntil) {
		delete(g.attempts, ip)
		return false
	}
	if !a.WindowStart.IsZero() && now.Sub(a.WindowStart) > 5*time.Minute {
		delete(g.attempts, ip)
	}
	return false
}

func (g *UserGuard) recordFailure(ip string) {
	const window = 5 * time.Minute
	const blockFor = 10 * time.Minute
	const maxFails = 5

	now := time.Now()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.gcAttemptsLocked(now)
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

func (g *UserGuard) clearAttempts(ip string) {
	g.mu.Lock()
	delete(g.attempts, ip)
	g.mu.Unlock()
}

func (g *UserGuard) gcLocked() {
	now := time.Now()
	for token, s := range g.sessions {
		if now.After(s.ExpiresAt) {
			delete(g.sessions, token)
		}
	}
	g.gcAttemptsLocked(now)
}

func (g *UserGuard) gcAttemptsLocked(now time.Time) {
	for ip, a := range g.attempts {
		if (!a.BlockedUntil.IsZero() && now.After(a.BlockedUntil)) || (!a.WindowStart.IsZero() && now.Sub(a.WindowStart) > 30*time.Minute) {
			delete(g.attempts, ip)
		}
	}
}
