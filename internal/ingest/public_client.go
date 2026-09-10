package ingest

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var errNonPublicAddress = errors.New("article destination is not public")
var errInvalidArticleURL = errors.New("article URL must be HTTP(S), with a host and no userinfo")

// Conservatively exclude special-use networks, including transition mechanisms
// that can translate an apparently public IPv6 destination into private IPv4.
var nonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
}
var globalIPv6 = netip.MustParsePrefix("2000::/3")

func isPublicAddress(ip netip.Addr) bool {
	if !ip.IsValid() || ip.Zone() != "" {
		return false
	}
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || (ip.Is6() && !globalIPv6.Contains(ip)) {
		return false
	}
	for _, prefix := range nonPublicPrefixes {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

func validateArticleURL(u *url.URL) error {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Opaque != "" {
		return errInvalidArticleURL
	}
	host := u.Hostname()
	if strings.Contains(host, "%") {
		return errNonPublicAddress
	}
	if ip, err := netip.ParseAddr(host); err == nil && !isPublicAddress(ip) {
		return errNonPublicAddress
	}
	return nil
}

type publicDialer struct {
	lookup func(context.Context, string, string) ([]netip.Addr, error)
	dial   func(context.Context, string, string) (net.Conn, error)
}

func (d publicDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, errInvalidArticleURL
	}
	var addresses []netip.Addr
	if ip, err := netip.ParseAddr(host); err == nil {
		addresses = []netip.Addr{ip}
	} else {
		addresses, err = d.lookup(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
	}
	if len(addresses) == 0 {
		return nil, errNonPublicAddress
	}
	// Validate the whole answer before dialing anything. Never hand a hostname
	// back to net.Dialer: doing so would open a DNS-rebinding TOCTOU window.
	for _, ip := range addresses {
		if !isPublicAddress(ip) {
			return nil, errNonPublicAddress
		}
	}
	for _, ip := range addresses {
		var conn net.Conn
		conn, err = d.dial(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
		if err == nil {
			return conn, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return nil, err
}

type publicTransport struct{ transport *http.Transport }

func (t publicTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := validateArticleURL(req.URL); err != nil {
		return nil, err
	}
	return t.transport.RoundTrip(req)
}
func (t publicTransport) CloseIdleConnections() { t.transport.CloseIdleConnections() }

func newArticleClient() *http.Client {
	d := publicDialer{lookup: net.DefaultResolver.LookupNetIP, dial: (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext}
	// No environment proxy: a proxy would resolve/dial outside this policy.
	transport := &http.Transport{
		DialContext: d.DialContext, ForceAttemptHTTP2: true,
		TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 8 * time.Second,
		IdleConnTimeout: 30 * time.Second, MaxIdleConns: 8, MaxIdleConnsPerHost: 2,
		MaxConnsPerHost: 2, MaxResponseHeaderBytes: 64 << 10,
	}
	return &http.Client{Transport: publicTransport{transport}, Timeout: 12 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("article redirect limit exceeded")
			}
			return validateArticleURL(req.URL)
		},
	}
}
