package api

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"berkut-scc/config"
	"berkut-scc/core/auth"
	"berkut-scc/core/rbac"
	"berkut-scc/core/store"
)

func TestRequirePermissionDeniesMissingPermission(t *testing.T) {
	s := &Server{policy: rbac.NewPolicy(rbac.DefaultRoles())}
	handler := s.requirePermission("reports.edit")(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/reports/1/charts", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.SessionContextKey, &store.SessionRecord{
		Username: "manager",
		Roles:    []string{"manager"},
	}))
	rr := httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden, got %d", rr.Code)
	}
}

func TestIsHTTPSRequestWithTLS(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	req.TLS = &tls.ConnectionState{}
	if !isHTTPSRequest(req, &config.AppConfig{}) {
		t.Fatalf("expected https request when TLS state is present")
	}
}

func TestIsHTTPSRequestWithTrustedProxyForwardedProto(t *testing.T) {
	cfg := &config.AppConfig{
		Security: config.SecurityConfig{
			TrustedProxies: []string{"10.0.0.10"},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	req.RemoteAddr = "10.0.0.10:12345"
	req.Header.Set("X-Forwarded-Proto", "https")
	if !isHTTPSRequest(req, cfg) {
		t.Fatalf("expected https request behind trusted proxy with x-forwarded-proto=https")
	}
}

func TestIsHTTPSRequestIgnoresUntrustedProxyHeader(t *testing.T) {
	cfg := &config.AppConfig{
		Security: config.SecurityConfig{
			TrustedProxies: []string{"10.0.0.10"},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	req.RemoteAddr = "192.168.1.20:12345"
	req.Header.Set("X-Forwarded-Proto", "https")
	if isHTTPSRequest(req, cfg) {
		t.Fatalf("expected non-https for untrusted proxy source")
	}
}

func TestClientIPUsesNearestUntrustedXFFHop(t *testing.T) {
	s := &Server{
		cfg: &config.AppConfig{
			Security: config.SecurityConfig{
				TrustedProxies: []string{"10.0.0.10", "10.0.0.11"},
			},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/login", nil)
	req.RemoteAddr = "10.0.0.10:54321"
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.11")
	got := s.clientIP(req)
	if got != "203.0.113.9" {
		t.Fatalf("expected client ip 203.0.113.9, got %s", got)
	}
}

func TestClientIPIgnoresXFFForUntrustedRemote(t *testing.T) {
	s := &Server{
		cfg: &config.AppConfig{
			Security: config.SecurityConfig{
				TrustedProxies: []string{"10.0.0.10"},
			},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/login", nil)
	req.RemoteAddr = "192.168.1.20:54321"
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.10")
	got := s.clientIP(req)
	if got != "192.168.1.20" {
		t.Fatalf("expected remote addr ip for untrusted source, got %s", got)
	}
}

func TestClientIPInvalidXFFFallsBackToRealIP(t *testing.T) {
	s := &Server{
		cfg: &config.AppConfig{
			Security: config.SecurityConfig{
				TrustedProxies: []string{"10.0.0.10"},
			},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/login", nil)
	req.RemoteAddr = "10.0.0.10:54321"
	req.Header.Set("X-Forwarded-For", "garbage,not-an-ip")
	req.Header.Set("X-Real-IP", "198.51.100.8")
	got := s.clientIP(req)
	if got != "198.51.100.8" {
		t.Fatalf("expected fallback to valid X-Real-IP, got %s", got)
	}
}

func TestSecurityHeadersSetHSTSForTrustedProxyHTTPS(t *testing.T) {
	s := &Server{
		cfg: &config.AppConfig{
			Security: config.SecurityConfig{
				TrustedProxies: []string{"10.0.0.10"},
			},
		},
	}
	h := s.securityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	req.RemoteAddr = "10.0.0.10:12345"
	req.Header.Set("X-Forwarded-Proto", "https")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Header().Get("Strict-Transport-Security") == "" {
		t.Fatalf("expected HSTS header for trusted proxy https request")
	}
}

func TestSecurityHeadersSkipHSTSForUntrustedProxy(t *testing.T) {
	s := &Server{
		cfg: &config.AppConfig{
			Security: config.SecurityConfig{
				TrustedProxies: []string{"10.0.0.10"},
			},
		},
	}
	h := s.securityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	req.RemoteAddr = "192.168.1.20:12345"
	req.Header.Set("X-Forwarded-Proto", "https")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Header().Get("Strict-Transport-Security") != "" {
		t.Fatalf("expected no HSTS header for untrusted proxy source")
	}
}

func TestWithSessionRedirectsPageRequestToLoginWhenUnauthorized(t *testing.T) {
	s := &Server{}
	h := s.withSession(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/docs/10", nil)
	req.Header.Set("Accept", "text/html")
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected redirect status, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/login" {
		t.Fatalf("expected redirect to /login, got %q", loc)
	}
}

func TestWithSessionKeepsAPIUnauthorizedResponse(t *testing.T) {
	s := &Server{}
	h := s.withSession(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/docs", nil)
	req.Header.Set("Accept", "application/json")
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized status for api request, got %d", rr.Code)
	}
}
