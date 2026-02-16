package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"berkut-scc/config"
	"berkut-scc/core/auth"
	"berkut-scc/core/rbac"
	"berkut-scc/core/store"
)

func (s *Server) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if s.logger != nil {
					s.logger.Errorf("PANIC %s %s: %v\n%s", r.Method, r.URL.Path, rec, string(debug.Stack()))
				}
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

const (
	sessionCookie               = "berkut_session"
	csrfCookie                  = "berkut_csrf"
	sessionActivityInterval     = 30 * time.Second
	loginPayloadMaxBytes        = 64 * 1024
	loginLimiterTTL             = 10 * time.Minute
	loginLimiterCleanupInterval = time.Minute
	loginLimiterMaxBuckets      = 10000
)

type requestLimiter struct {
	mu              sync.Mutex
	buckets         map[string]*tokenBucket
	capacity        int
	refill          time.Duration
	ttl             time.Duration
	cleanupInterval time.Duration
	lastCleanup     time.Time
	maxBuckets      int
}

type tokenBucket struct {
	tokens   int
	last     time.Time
	lastSeen time.Time
}

type sessionActivity struct {
	mu   sync.Mutex
	last map[string]time.Time
}

func newSessionActivity() *sessionActivity {
	return &sessionActivity{last: map[string]time.Time{}}
}

func (sa *sessionActivity) shouldUpdate(id string, now time.Time, interval time.Duration) bool {
	sa.mu.Lock()
	defer sa.mu.Unlock()
	last, ok := sa.last[id]
	if !ok || now.Sub(last) >= interval {
		sa.last[id] = now
		return true
	}
	return false
}

func newLimiter(capacity int, refill time.Duration) *requestLimiter {
	return &requestLimiter{
		buckets:         make(map[string]*tokenBucket),
		capacity:        capacity,
		refill:          refill,
		ttl:             loginLimiterTTL,
		cleanupInterval: loginLimiterCleanupInterval,
		maxBuckets:      loginLimiterMaxBuckets,
	}
}

func (l *requestLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if l.cleanupInterval > 0 && now.Sub(l.lastCleanup) >= l.cleanupInterval {
		l.cleanup(now)
		l.lastCleanup = now
	}
	tb, ok := l.buckets[key]
	if !ok {
		l.buckets[key] = &tokenBucket{tokens: l.capacity - 1, last: now, lastSeen: now}
		return true
	}
	tb.lastSeen = now
	elapsed := now.Sub(tb.last)
	if elapsed >= l.refill {
		tb.tokens = l.capacity
		tb.last = now
	}
	if tb.tokens <= 0 {
		return false
	}
	tb.tokens--
	return true
}

func (l *requestLimiter) cleanup(now time.Time) {
	if l.ttl > 0 {
		for key, tb := range l.buckets {
			if now.Sub(tb.lastSeen) > l.ttl {
				delete(l.buckets, key)
			}
		}
	}
	if l.maxBuckets > 0 && len(l.buckets) > l.maxBuckets {
		for len(l.buckets) > l.maxBuckets {
			oldestKey := ""
			var oldest time.Time
			for key, tb := range l.buckets {
				if oldestKey == "" || tb.lastSeen.Before(oldest) {
					oldestKey = key
					oldest = tb.lastSeen
				}
			}
			if oldestKey == "" {
				break
			}
			delete(l.buckets, oldestKey)
		}
	}
}

func (s *Server) securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:; object-src 'none'; frame-ancestors 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if isHTTPSRequest(r, s.cfg) {
			w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) jsonMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if s.logger != nil {
			s.logger.Printf("REQ %s %s", r.Method, r.URL.Path)
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) maintenanceModeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s == nil || s.backupsSvc == nil || !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if isMaintenanceAllowedPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if !s.backupsSvc.IsMaintenanceMode(r.Context()) {
			next.ServeHTTP(w, r)
			return
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": map[string]string{
				"code":     "app.maintenance_mode",
				"i18n_key": "common.error.maintenanceMode",
			},
		})
	})
}

func isMaintenanceAllowedPath(path string) bool {
	p := strings.TrimSpace(path)
	if strings.HasPrefix(p, "/api/auth/") {
		return true
	}
	return strings.HasPrefix(p, "/api/backups/restores/")
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if s.logger != nil {
			user := "-"
			if v := r.Context().Value(auth.SessionContextKey); v != nil {
				sr := v.(*store.SessionRecord)
				user = sr.Username
			}
			s.logger.Printf("RESP %s %s user=%s status=%d dur=%s bytes=%d", r.Method, r.URL.Path, user, rec.status, time.Since(start), rec.size)
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	size   int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.size += n
	return n, err
}

func (s *Server) withSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil || cookie.Value == "" {
			if s.allowOnlyOfficeServiceAccess(r) {
				sr := &store.SessionRecord{
					ID:       "onlyoffice-service",
					Username: "onlyoffice",
					Roles:    []string{"doc_editor"},
				}
				ctx := context.WithValue(r.Context(), auth.SessionContextKey, sr)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			if s.logger != nil {
				s.logger.Printf("AUTH fail (missing cookie) %s %s", r.Method, r.URL.Path)
			}
			s.respondUnauthorized(w, r)
			return
		}
		sr, err := s.sessions.GetSession(r.Context(), cookie.Value)
		if err != nil || sr == nil {
			if s.logger != nil {
				s.logger.Printf("AUTH fail (session not found) %s %s: %v", r.Method, r.URL.Path, err)
			}
			s.respondUnauthorized(w, r)
			return
		}
		user, _, err := s.users.FindByUsername(r.Context(), sr.Username)
		if err != nil || user == nil || !user.Active {
			if s.logger != nil {
				s.logger.Printf("AUTH fail (user inactive/missing) %s %s: %v", r.Method, r.URL.Path, err)
			}
			_ = s.sessions.DeleteSession(r.Context(), sr.ID, sr.Username)
			s.respondUnauthorized(w, r)
			return
		}
		if user.RequirePasswordChange && !allowedForPasswordChange(r.URL.Path) && r.URL.Path != "/api/auth/me" {
			http.Error(w, "password change required", http.StatusForbidden)
			return
		}
		// CSRF for state-changing methods
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			csrfHeader := r.Header.Get("X-CSRF-Token")
			csrfCookieVal, _ := r.Cookie(csrfCookie)
			if csrfHeader == "" || csrfCookieVal == nil || csrfHeader != csrfCookieVal.Value || csrfHeader != sr.CSRFToken {
				if s.logger != nil {
					s.logger.Printf("AUTH fail (csrf) %s %s user=%s", r.Method, r.URL.Path, sr.Username)
				}
				http.Error(w, "csrf invalid", http.StatusForbidden)
				return
			}
		}
		ctx := context.WithValue(r.Context(), auth.SessionContextKey, sr)
		now := time.Now().UTC()
		interval := sessionActivityInterval
		if s.cfg != nil && s.cfg.Security.OnlineWindowSec > 0 {
			custom := time.Duration(s.cfg.Security.OnlineWindowSec/2) * time.Second
			if custom < sessionActivityInterval {
				custom = sessionActivityInterval
			}
			if custom > time.Minute {
				custom = time.Minute
			}
			interval = custom
		}
		if s.activityTracker == nil || s.activityTracker.shouldUpdate(sr.ID, now, interval) {
			_ = s.sessions.UpdateActivity(r.Context(), sr.ID, now, s.cfg.EffectiveSessionTTL())
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

func (s *Server) respondUnauthorized(w http.ResponseWriter, r *http.Request) {
	if shouldRedirectToLogin(r) {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

func shouldRedirectToLogin(r *http.Request) bool {
	if r == nil || r.URL == nil || r.Method != http.MethodGet {
		return false
	}
	path := strings.TrimSpace(r.URL.Path)
	if path == "" {
		return false
	}
	if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/static/") {
		return false
	}
	accept := strings.ToLower(strings.TrimSpace(r.Header.Get("Accept")))
	if accept == "" {
		return true
	}
	return strings.Contains(accept, "text/html") || strings.Contains(accept, "*/*")
}

func (s *Server) allowOnlyOfficeServiceAccess(r *http.Request) bool {
	if s == nil || s.cfg == nil || !s.cfg.Docs.OnlyOffice.Enabled {
		return false
	}
	path := strings.TrimSpace(r.URL.Path)
	if !strings.HasPrefix(path, "/api/docs/") {
		return false
	}
	purpose := ""
	switch {
	case strings.HasSuffix(path, "/office/file"):
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			return false
		}
		purpose = "file"
	case strings.HasSuffix(path, "/office/callback"):
		if r.Method != http.MethodPost {
			return false
		}
		purpose = "callback"
	default:
		return false
	}
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		return false
	}
	docID, ok := extractDocIDFromOnlyOfficePath(path)
	if !ok || docID <= 0 {
		return false
	}
	claims, err := parseOnlyOfficeLinkTokenClaims(strings.TrimSpace(s.cfg.Docs.OnlyOffice.JWTSecret), token, time.Now())
	if err != nil {
		return false
	}
	return claims.DocID == docID && claims.Purpose == purpose
}

func extractDocIDFromOnlyOfficePath(path string) (int64, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 4 {
		return 0, false
	}
	if parts[0] != "api" || parts[1] != "docs" {
		return 0, false
	}
	id, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

type onlyOfficeMiddlewareClaims struct {
	Purpose string `json:"purpose"`
	DocID   int64  `json:"doc_id"`
	Exp     int64  `json:"exp"`
}

func parseOnlyOfficeLinkTokenClaims(secret, token string, now time.Time) (*onlyOfficeMiddlewareClaims, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("empty secret")
	}
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 2 {
		return nil, errors.New("invalid token")
	}
	payloadPart := strings.TrimSpace(parts[0])
	signaturePart := strings.TrimSpace(parts[1])
	gotSig, err := base64.RawURLEncoding.DecodeString(signaturePart)
	if err != nil {
		return nil, errors.New("invalid signature")
	}
	wantSig := hmacSHA256Bytes([]byte(secret), []byte(payloadPart))
	if subtle.ConstantTimeCompare(gotSig, wantSig) != 1 {
		return nil, errors.New("bad signature")
	}
	rawPayload, err := base64.RawURLEncoding.DecodeString(payloadPart)
	if err != nil {
		return nil, errors.New("invalid payload")
	}
	var claims onlyOfficeMiddlewareClaims
	if err := json.Unmarshal(rawPayload, &claims); err != nil {
		return nil, errors.New("invalid payload")
	}
	if claims.DocID <= 0 || claims.Purpose == "" || claims.Exp <= 0 {
		return nil, errors.New("invalid claims")
	}
	if now.UTC().Unix() >= claims.Exp {
		return nil, errors.New("expired")
	}
	return &claims, nil
}

func hmacSHA256Bytes(secret, payload []byte) []byte {
	m := hmac.New(sha256.New, secret)
	_, _ = m.Write(payload)
	return m.Sum(nil)
}

func (s *Server) requirePermission(perm rbac.Permission) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			val := r.Context().Value(auth.SessionContextKey)
			if val == nil {
				if s.logger != nil {
					s.logger.Printf("PERM fail (no session) %s %s need=%s", r.Method, r.URL.Path, perm)
				}
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			sess := val.(*store.SessionRecord)
			if !s.policy.Allowed(sess.Roles, perm) {
				s.logBackupsDenied(r, sess.Username)
				if s.logger != nil {
					s.logger.Printf("PERM fail %s %s user=%s roles=%v need=%s", r.Method, r.URL.Path, sess.Username, sess.Roles, perm)
				}
				if strings.HasPrefix(r.URL.Path, "/api/backups") {
					writeJSON(w, http.StatusForbidden, map[string]any{
						"error": map[string]string{
							"code":     "backups.forbidden",
							"i18n_key": "backups.error.permissionDenied",
						},
					})
					return
				}
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		}
	}
}

func (s *Server) requireAnyPermission(perms ...rbac.Permission) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			val := r.Context().Value(auth.SessionContextKey)
			if val == nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			sess := val.(*store.SessionRecord)
			allowed := false
			for _, p := range perms {
				if s.policy.Allowed(sess.Roles, p) {
					allowed = true
					break
				}
			}
			if !allowed {
				if s.logger != nil {
					s.logger.Printf("PERM fail %s %s user=%s roles=%v need_any=%v", r.Method, r.URL.Path, sess.Username, sess.Roles, perms)
				}
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		}
	}
}

func (s *Server) requirePermissionFromPath(resolver func(string) rbac.Permission) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			val := r.Context().Value(auth.SessionContextKey)
			if val == nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			sess := val.(*store.SessionRecord)
			parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			name := parts[len(parts)-1]
			if name == "docs" {
				if !s.policy.Allowed(sess.Roles, "docs.view") && !s.policy.Allowed(sess.Roles, "docs.approval.view") {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			perm := resolver(name)
			if perm == "" || !s.policy.Allowed(sess.Roles, perm) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		}
	}
}

var loginLimiter = newLimiter(5, time.Minute)

func allowedForPasswordChange(path string) bool {
	if path == "/password-change" {
		return true
	}
	if strings.HasPrefix(path, "/api/auth/change-password") {
		return true
	}
	if strings.HasPrefix(path, "/api/auth/logout") {
		return true
	}
	return false
}

func (s *Server) rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := s.clientIP(r)
		r.Body = http.MaxBytesReader(w, r.Body, loginPayloadMaxBytes+1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		var cred auth.Credentials
		_ = json.Unmarshal(body, &cred)
		username := strings.ToLower(strings.TrimSpace(cred.Username))
		keyIP := strings.ToLower(ip)
		if !loginLimiter.allow(keyIP) {
			http.Error(w, "too many attempts", http.StatusTooManyRequests)
			return
		}
		if username != "" && !loginLimiter.allow("user|"+username) {
			http.Error(w, "too many attempts", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func (s *Server) clientIP(r *http.Request) string {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip == "" {
		ip = r.RemoteAddr
	}
	ip = strings.TrimSpace(ip)
	if s == nil || s.cfg == nil || !isTrustedProxy(ip, s.cfg.Security.TrustedProxies) {
		return ip
	}
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		if candidate := extractClientIPFromXFF(xff, s.cfg.Security.TrustedProxies); candidate != "" {
			return candidate
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		if parsed := net.ParseIP(realIP); parsed != nil {
			return parsed.String()
		}
	}
	return ip
}

func isHTTPSRequest(r *http.Request, cfg *config.AppConfig) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	if cfg == nil {
		return false
	}
	if cfg.TLSEnabled {
		return true
	}
	remoteIP, _, _ := net.SplitHostPort(r.RemoteAddr)
	if remoteIP == "" {
		remoteIP = strings.TrimSpace(r.RemoteAddr)
	}
	remoteIP = strings.TrimSpace(remoteIP)
	if !isTrustedProxy(remoteIP, cfg.Security.TrustedProxies) {
		return false
	}
	xffProto := strings.ToLower(strings.TrimSpace(strings.SplitN(r.Header.Get("X-Forwarded-Proto"), ",", 2)[0]))
	return xffProto == "https"
}

func extractClientIPFromXFF(xff string, trusted []string) string {
	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		candidate := strings.TrimSpace(parts[i])
		parsed := net.ParseIP(candidate)
		if parsed == nil {
			continue
		}
		val := parsed.String()
		if !isTrustedProxy(val, trusted) {
			return val
		}
	}
	return ""
}

func isTrustedProxy(ip string, trusted []string) bool {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return false
	}
	for _, raw := range trusted {
		val := strings.TrimSpace(raw)
		if val == "" {
			continue
		}
		if strings.Contains(val, "/") {
			if _, block, err := net.ParseCIDR(val); err == nil && block.Contains(parsed) {
				return true
			}
			continue
		}
		if parsed.Equal(net.ParseIP(val)) {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
