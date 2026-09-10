package ingest

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestPublicAddresses(t *testing.T) {
	for _, raw := range []string{
		"0.0.0.0", "10.1.2.3", "100.64.1.1", "127.0.0.1", "169.254.169.254",
		"172.16.1.2", "192.168.1.1", "192.0.0.1", "192.0.2.1", "192.88.99.1",
		"198.18.0.1", "198.51.100.1", "203.0.113.1", "224.0.0.1", "240.0.0.1",
		"255.255.255.255", "::", "::1", "::ffff:127.0.0.1", "::ffff:10.1.2.3",
		"fc00::1", "fe80::1", "fe80::1%eth0", "ff02::1", "64:ff9b::7f00:1",
		"64:ff9b:1::1", "2001::1", "2001:db8::1", "2002:7f00:1::", "3fff::1",
	} {
		t.Run(raw, func(t *testing.T) {
			if isPublicAddress(netip.MustParseAddr(raw)) {
				t.Fatal("special-use address allowed")
			}
		})
	}
	for _, raw := range []string{"8.8.8.8", "1.1.1.1", "::ffff:8.8.8.8", "2606:4700:4700::1111"} {
		if !isPublicAddress(netip.MustParseAddr(raw)) {
			t.Fatalf("public address rejected: %s", raw)
		}
	}
}

func TestPublicDialRejectsMixedDNSBeforeAnyConnection(t *testing.T) {
	for _, addresses := range [][]netip.Addr{
		{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("127.0.0.1")},
		{netip.MustParseAddr("2606:4700::1111"), netip.MustParseAddr("::ffff:192.168.1.1")},
		nil,
	} {
		d := publicDialer{
			lookup: func(context.Context, string, string) ([]netip.Addr, error) { return addresses, nil },
			dial: func(context.Context, string, string) (net.Conn, error) {
				t.Fatal("dialed an untrusted DNS answer")
				return nil, nil
			},
		}
		if _, err := d.DialContext(context.Background(), "tcp", "publisher.test:443"); !errors.Is(err, errNonPublicAddress) {
			t.Fatalf("got %v", err)
		}
	}
}

func TestPublicDialPinsIPAndRechecksEachConnection(t *testing.T) {
	lookups, dials := 0, 0
	dialErr := errors.New("test connection refused")
	d := publicDialer{
		lookup: func(context.Context, string, string) ([]netip.Addr, error) {
			lookups++
			if lookups == 1 {
				return []netip.Addr{netip.MustParseAddr("::ffff:8.8.8.8")}, nil
			}
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		},
		dial: func(_ context.Context, _, address string) (net.Conn, error) {
			dials++
			if address != "8.8.8.8:443" {
				t.Fatalf("not a pinned unmapped literal: %s", address)
			}
			return nil, dialErr
		},
	}
	if _, err := d.DialContext(context.Background(), "tcp", "publisher.test:443"); !errors.Is(err, dialErr) {
		t.Fatal(err)
	}
	if _, err := d.DialContext(context.Background(), "tcp", "publisher.test:443"); !errors.Is(err, errNonPublicAddress) {
		t.Fatal(err)
	}
	if lookups != 2 || dials != 1 {
		t.Fatalf("lookups=%d dials=%d", lookups, dials)
	}
}

// Use the production policy and transport, replacing only the DNS/dial boundary
// so public test names reach a disposable local HTTP server, never the network.
func articleTestClient(t *testing.T, target string) *http.Client {
	t.Helper()
	client := newArticleClient()
	transport := client.Transport.(publicTransport).transport
	d := publicDialer{
		lookup: func(_ context.Context, _, host string) ([]netip.Addr, error) {
			if host == "private.test" {
				return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
			}
			return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
		},
		dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(address)
			if err != nil || host != "8.8.8.8" {
				t.Errorf("unexpected dial target: %s", address)
				return nil, errNonPublicAddress
			}
			return (&net.Dialer{}).DialContext(ctx, network, target)
		},
	}
	transport.DialContext = d.DialContext
	t.Cleanup(client.CloseIdleConnections)
	return client
}

func TestArticleClientRejectsUnsafeInitialAndRedirectURLs(t *testing.T) {
	for _, target := range []string{
		"http://127.0.0.1/secret?token=private", "http://[::ffff:127.0.0.1]/",
		"http://169.254.169.254/latest/meta-data/", "http://private.test/",
		"http://user:password@publisher.test/", "ftp://publisher.test/a", "file:///etc/passwd",
	} {
		t.Run(target, func(t *testing.T) {
			hits := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits++
				if r.URL.Path != "/start" {
					t.Error("followed an unsafe redirect")
				}
				w.Header().Set("Location", target)
				w.WriteHeader(http.StatusFound)
			}))
			defer server.Close()
			client := articleTestClient(t, server.Listener.Addr().String())
			if resp, err := client.Get(target); err == nil {
				resp.Body.Close()
				t.Fatal("unsafe initial request accepted")
			}
			if hits != 0 {
				t.Fatalf("initial request reached server: %d", hits)
			}
			if resp, err := client.Get("http://publisher.test/start"); err == nil {
				resp.Body.Close()
				t.Fatal("unsafe redirect accepted")
			}
			if hits != 1 {
				t.Fatalf("unsafe destination contacted, hits=%d", hits)
			}
		})
	}
}

func TestArticleClientIgnoresProxyAndPreservesHostAndSNI(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("article fetch used environment proxy")
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer proxy.Close()
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		t.Setenv(key, proxy.URL)
	}
	t.Setenv("NO_PROXY", "")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "example.com" || r.TLS.ServerName != "example.com" {
			t.Errorf("lost original Host/SNI: %s / %s", r.Host, r.TLS.ServerName)
		}
		fmt.Fprint(w, "legitimate publisher")
	}))
	defer server.Close()
	client := articleTestClient(t, server.Listener.Addr().String())
	transport := client.Transport.(publicTransport).transport
	if transport.Proxy != nil {
		t.Fatal("proxy selection must be disabled")
	}
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	resp, err := client.Get("https://example.com/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil || string(body) != "legitimate publisher" {
		t.Fatalf("body=%q err=%v", body, err)
	}
}

func TestArticleClientRedirectLimit(t *testing.T) {
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Redirect(w, r, "/again", http.StatusFound)
	}))
	defer server.Close()
	client := articleTestClient(t, server.Listener.Addr().String())
	_, err := client.Get("http://publisher.test/again")
	if err == nil || !strings.Contains(err.Error(), "redirect limit") || hits != 5 {
		t.Fatalf("hits=%d err=%v", hits, err)
	}
}
