package hub

import (
	"net"
	"net/http"
	"os"
	"strings"
)

// proxyTrust holds optional reverse-proxy settings for live Caddy fronting.
//
// It decides three things that all depend on knowing where a request
// really came from: which address the rate limiters count against, which
// origins may open a WebSocket or post credentials, and whether the
// session cookie is marked Secure.
type proxyTrust struct {
	nets    []*net.IPNet
	origins map[string]struct{} // host or full origin (lowercased)
	secure  bool
}

func loadProxyTrust() *proxyTrust {
	pt := &proxyTrust{
		origins: make(map[string]struct{}),
		secure:  envTruthy("HOLLOWMERE_SECURE_COOKIES"),
	}
	for _, part := range splitCSV(os.Getenv("HOLLOWMERE_TRUSTED_PROXIES")) {
		if strings.Contains(part, "/") {
			_, n, err := net.ParseCIDR(part)
			if err == nil {
				pt.nets = append(pt.nets, n)
			}
			continue
		}
		ip := net.ParseIP(part)
		if ip == nil {
			continue
		}
		bits := 32
		if ip.To4() == nil {
			bits = 128
		}
		pt.nets = append(pt.nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
	}
	for _, part := range splitCSV(os.Getenv("HOLLOWMERE_ALLOWED_ORIGINS")) {
		pt.origins[strings.ToLower(part)] = struct{}{}
		if u := strings.TrimPrefix(strings.TrimPrefix(part, "https://"), "http://"); u != part {
			pt.origins[strings.ToLower(u)] = struct{}{}
		} else if !strings.Contains(part, "://") {
			pt.origins["https://"+strings.ToLower(part)] = struct{}{}
			pt.origins["http://"+strings.ToLower(part)] = struct{}{}
		}
	}
	return pt
}

func (pt *proxyTrust) trusted(ip string) bool {
	if pt == nil || len(pt.nets) == 0 {
		return false
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, n := range pt.nets {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}

// allowOrigin decides whether a cross-origin request may proceed.
//
// With no allowlist configured it falls back to same-host: a page served
// from this host may connect, anything else may not. Returning true in
// that case would restore the old "accept every origin" behaviour
// whenever the env var is missing — which, now that the credential is a
// cookie, is exactly how a hostile page rides someone's session.
//
// An absent Origin is allowed: browsers always send one on a cross-site
// request, so only non-browser clients (the bot harness, the smoke
// scripts) land here, and those cannot be driven by a web page.
func (pt *proxyTrust) allowOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	o := strings.ToLower(origin)

	// Same-host is always allowed, allowlist or not. A browser sets Host
	// from the address it actually connected to, so an attacker cannot
	// forge a matching pair in a victim's browser. Allowing it keeps
	// direct access working (hitting the container port on the LAN,
	// bypassing Caddy) instead of failing with a bare 403.
	if strings.EqualFold(stripScheme(o), r.Host) {
		return true
	}

	if pt == nil || len(pt.origins) == 0 {
		return false
	}
	if _, ok := pt.origins[o]; ok {
		return true
	}
	if host := stripScheme(o); host != o {
		if _, ok := pt.origins[host]; ok {
			return true
		}
	}
	return false
}

// clientIP resolves the real caller. Behind Caddy every request arrives
// from the proxy, so without this every player would share one rate-limit
// bucket and one failed-login budget.
//
// X-Forwarded-For is only believed when the immediate peer is a trusted
// proxy, and then we take the RIGHTMOST entry that is not itself a
// trusted hop. Caddy *appends* the peer address to whatever the client
// sent, so the leftmost entry is attacker-controlled: a client that sends
// its own `X-Forwarded-For: 1.2.3.4` would otherwise choose the identity
// its login attempts are counted against, and could mint a fresh one per
// guess.
func (pt *proxyTrust) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if !pt.trusted(host) {
		return host
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			candidate := strings.TrimSpace(parts[i])
			if candidate == "" || pt.trusted(candidate) {
				continue
			}
			if ip := net.ParseIP(candidate); ip != nil {
				return candidate
			}
		}
		// Every hop in the chain was trusted; fall through to the peer.
	}
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		if ip := net.ParseIP(xri); ip != nil {
			return xri
		}
	}
	return host
}

// cookieSecure reports whether the session cookie should carry Secure.
// It must not be set on plain http://127.0.0.1 or local play breaks, and
// it must be set behind TLS or the cookie travels in clear.
func (pt *proxyTrust) cookieSecure(r *http.Request) bool {
	if pt != nil && pt.secure {
		return true
	}
	if r.TLS != nil {
		return true
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return pt.trusted(host) && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func stripScheme(s string) string {
	return strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func envTruthy(k string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(k)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}
