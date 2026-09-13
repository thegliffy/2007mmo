package hub

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIPTrustedXFF(t *testing.T) {
	t.Setenv("HOLLOWMERE_TRUSTED_PROXIES", "172.16.2.1/32")
	t.Setenv("HOLLOWMERE_ALLOWED_ORIGINS", "")
	t.Setenv("HOLLOWMERE_SECURE_COOKIES", "")
	pt := loadProxyTrust()
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.RemoteAddr = "172.16.2.1:45678"
	// Caddy appends the peer it saw, so the real client is the last entry.
	r.Header.Set("X-Forwarded-For", "10.0.0.1, 203.0.113.9")
	if got := pt.clientIP(r); got != "203.0.113.9" {
		t.Fatalf("got %q want 203.0.113.9", got)
	}
}

// A client that sets its own X-Forwarded-For must not be able to choose
// the address its login attempts are counted against. Caddy appends to
// the header rather than replacing it, so trusting the leftmost entry
// would hand every attacker an unlimited supply of fresh identities.
func TestClientIPIgnoresClientSuppliedXFFPrefix(t *testing.T) {
	t.Setenv("HOLLOWMERE_TRUSTED_PROXIES", "172.16.2.1/32")
	pt := loadProxyTrust()
	r := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	r.RemoteAddr = "172.16.2.1:45678"
	// "1.2.3.4" is what the attacker sent; "203.0.113.9" is what Caddy saw.
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 203.0.113.9")
	got := pt.clientIP(r)
	if got == "1.2.3.4" {
		t.Fatal("spoofed X-Forwarded-For prefix was believed")
	}
	if got != "203.0.113.9" {
		t.Fatalf("got %q want 203.0.113.9", got)
	}
}

// Chained proxies: skip hops we trust, keep the first address we do not.
func TestClientIPSkipsTrustedHops(t *testing.T) {
	t.Setenv("HOLLOWMERE_TRUSTED_PROXIES", "172.16.2.1/32,10.0.0.0/8")
	pt := loadProxyTrust()
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.RemoteAddr = "172.16.2.1:45678"
	r.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.5, 10.0.0.6")
	if got := pt.clientIP(r); got != "203.0.113.9" {
		t.Fatalf("got %q want 203.0.113.9", got)
	}
}

func TestClientIPUntrustedIgnoresXFF(t *testing.T) {
	t.Setenv("HOLLOWMERE_TRUSTED_PROXIES", "172.16.2.1/32")
	pt := loadProxyTrust()
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.RemoteAddr = "198.51.100.2:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.9")
	if got := pt.clientIP(r); got != "198.51.100.2" {
		t.Fatalf("got %q want 198.51.100.2", got)
	}
}

func TestClientIPRealIPFallback(t *testing.T) {
	t.Setenv("HOLLOWMERE_TRUSTED_PROXIES", "172.16.2.1/32")
	pt := loadProxyTrust()
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.RemoteAddr = "172.16.2.1:45678"
	r.Header.Set("X-Real-IP", "203.0.113.9")
	if got := pt.clientIP(r); got != "203.0.113.9" {
		t.Fatalf("got %q want 203.0.113.9", got)
	}
}

// The allowlist admits the public origin; same-host still works too, so
// reaching the container directly on the LAN is not a 403.
func TestAllowOriginAllowsSameHostAlongsideAllowlist(t *testing.T) {
	t.Setenv("HOLLOWMERE_ALLOWED_ORIGINS", "2007.gliffy.tv")
	pt := loadProxyTrust()
	direct := httptest.NewRequest(http.MethodGet, "http://moneta:28080/ws", nil)
	direct.Host = "moneta:28080"
	direct.Header.Set("Origin", "http://moneta:28080")
	if !pt.allowOrigin(direct) {
		t.Fatal("same-host origin should be allowed even with an allowlist set")
	}
	evil := httptest.NewRequest(http.MethodGet, "http://moneta:28080/ws", nil)
	evil.Host = "moneta:28080"
	evil.Header.Set("Origin", "https://evil.example")
	if pt.allowOrigin(evil) {
		t.Fatal("a foreign origin must still be denied")
	}
}

func TestAllowOrigin(t *testing.T) {
	t.Setenv("HOLLOWMERE_ALLOWED_ORIGINS", "2007.gliffy.tv")
	t.Setenv("HOLLOWMERE_TRUSTED_PROXIES", "")
	pt := loadProxyTrust()
	ok := httptest.NewRequest(http.MethodGet, "/ws", nil)
	ok.Header.Set("Origin", "https://2007.gliffy.tv")
	if !pt.allowOrigin(ok) {
		t.Fatal("expected allowed")
	}
	bad := httptest.NewRequest(http.MethodGet, "/ws", nil)
	bad.Header.Set("Origin", "https://evil.example")
	if pt.allowOrigin(bad) {
		t.Fatal("expected denied")
	}
}

// With no allowlist set, the default must still be same-host rather than
// "allow everything" — otherwise a missing env var silently reopens
// cross-site WebSocket hijacking against the session cookie.
func TestAllowOriginDefaultsToSameHost(t *testing.T) {
	t.Setenv("HOLLOWMERE_ALLOWED_ORIGINS", "")
	t.Setenv("HOLLOWMERE_TRUSTED_PROXIES", "")
	pt := loadProxyTrust()

	same := httptest.NewRequest(http.MethodGet, "http://hollowmere.local/ws", nil)
	same.Host = "hollowmere.local"
	same.Header.Set("Origin", "http://hollowmere.local")
	if !pt.allowOrigin(same) {
		t.Fatal("same-host origin should be allowed with no allowlist")
	}

	cross := httptest.NewRequest(http.MethodGet, "http://hollowmere.local/ws", nil)
	cross.Host = "hollowmere.local"
	cross.Header.Set("Origin", "https://evil.example")
	if pt.allowOrigin(cross) {
		t.Fatal("cross-site origin must be denied even with no allowlist")
	}
}

// Non-browser clients (bot harness, smoke scripts) send no Origin.
func TestAllowOriginAbsentOrigin(t *testing.T) {
	t.Setenv("HOLLOWMERE_ALLOWED_ORIGINS", "2007.gliffy.tv")
	pt := loadProxyTrust()
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	if !pt.allowOrigin(r) {
		t.Fatal("a request with no Origin should be allowed")
	}
}

func TestCookieSecure(t *testing.T) {
	t.Setenv("HOLLOWMERE_SECURE_COOKIES", "")
	t.Setenv("HOLLOWMERE_TRUSTED_PROXIES", "172.16.2.1/32")
	pt := loadProxyTrust()

	// Plain local play: no Secure, or the cookie is never stored.
	local := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	local.RemoteAddr = "127.0.0.1:5555"
	if pt.cookieSecure(local) {
		t.Error("Secure must not be set on plain local http")
	}

	// Behind a trusted proxy terminating TLS.
	proxied := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	proxied.RemoteAddr = "172.16.2.1:45678"
	proxied.Header.Set("X-Forwarded-Proto", "https")
	if !pt.cookieSecure(proxied) {
		t.Error("Secure should be set when a trusted proxy reports https")
	}

	// An untrusted peer must not be able to claim https.
	spoof := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	spoof.RemoteAddr = "198.51.100.2:1234"
	spoof.Header.Set("X-Forwarded-Proto", "https")
	if pt.cookieSecure(spoof) {
		t.Error("an untrusted peer claiming https must not set Secure")
	}
}

func TestCookieSecureForcedByEnv(t *testing.T) {
	t.Setenv("HOLLOWMERE_SECURE_COOKIES", "1")
	t.Setenv("HOLLOWMERE_TRUSTED_PROXIES", "")
	pt := loadProxyTrust()
	r := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	r.RemoteAddr = "127.0.0.1:5555"
	if !pt.cookieSecure(r) {
		t.Fatal("HOLLOWMERE_SECURE_COOKIES=1 should force Secure")
	}
}

func TestEnvTruthy(t *testing.T) {
	for _, v := range []string{"1", "true", "TRUE", "yes", "on"} {
		t.Setenv("HOLLOWMERE_SECURE_COOKIES", v)
		if !envTruthy("HOLLOWMERE_SECURE_COOKIES") {
			t.Errorf("%q should be truthy", v)
		}
	}
	for _, v := range []string{"", "0", "false", "no", "off"} {
		t.Setenv("HOLLOWMERE_SECURE_COOKIES", v)
		if envTruthy("HOLLOWMERE_SECURE_COOKIES") {
			t.Errorf("%q should not be truthy", v)
		}
	}
}
