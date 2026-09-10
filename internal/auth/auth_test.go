package auth

import (
	"testing"
	"time"
)

func TestCIDRsFailClosed(t *testing.T) {
	for _, cidrs := range [][]string{nil, {"wrong"}, {"127.0.0.1/32", "wrong"}} {
		if _, err := New("test-secret", cidrs); err == nil {
			t.Fatalf("accepted %v", cidrs)
		}
	}
	g, err := New("test-secret", []string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	if g.AllowRemote("192.0.2.1:80") || !g.AllowRemote("10.1.2.3:80") {
		t.Fatal("CIDR boundary")
	}
}
func TestSlidingSessionAndCSRF(t *testing.T) {
	g, _ := New("test-secret", []string{})
	token, _, err := g.NewSession("10.0.0.1:80", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	csrf, ok := g.SessionCSRF(token, "10.0.0.1:80")
	if !ok {
		t.Fatal("no csrf")
	}
	if !g.ValidateSession(token, "10.0.0.2:80") || !g.ValidateCSRF(token, "10.0.0.2:80", csrf) || g.ValidateCSRF(token, "10.0.0.2:80", "wrong") {
		t.Fatal("session/CSRF")
	}
	g.mu.Lock()
	s := g.sessions[token]
	s.ExpiresAt = time.Now().Add(-time.Second)
	g.sessions[token] = s
	g.mu.Unlock()
	if g.ValidateSession(token, "10.0.0.2:80") {
		t.Fatal("expired session resurrected")
	}
}
