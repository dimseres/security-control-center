package tests

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"berkut-scc/api/handlers"
	"berkut-scc/config"
	"berkut-scc/core/auth"
	"berkut-scc/core/rbac"
	"berkut-scc/core/store"
	"berkut-scc/core/utils"
)

func setupSessionEnv(t *testing.T) (store.SessionStore, store.UsersStore, *handlers.AccountsHandler, *handlers.AuthHandler, *config.AppConfig, *sql.DB, store.RolesStore, store.GroupsStore, *utils.Logger, func()) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.AppConfig{
		DBPath:     filepath.Join(dir, "sessions.db"),
		Pepper:     "pepper",
		SessionTTL: time.Hour,
		Security:   config.SecurityConfig{OnlineWindowSec: 300},
	}
	logger := utils.NewLogger()
	db, err := store.NewDB(cfg, logger)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.ApplyMigrations(context.Background(), db, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	users := store.NewUsersStore(db)
	sessions := store.NewSessionsStore(db)
	roles := store.NewRolesStore(db)
	groups := store.NewGroupsStore(db)
	audits := store.NewAuditStore(db)
	incidents := store.NewIncidentsStore(db)
	policy := rbac.NewPolicy([]rbac.Role{{Name: "admin", Permissions: []rbac.Permission{"accounts.manage"}}})
	sm := auth.NewSessionManager(sessions, cfg, logger)
	acc := handlers.NewAccountsHandler(users, groups, roles, sessions, policy, sm, cfg, audits, logger, nil)
	authHandler := handlers.NewAuthHandler(cfg, users, sessions, incidents, sm, policy, audits, logger)
	cleanup := func() { db.Close() }
	return sessions, users, acc, authHandler, cfg, db, roles, groups, logger, cleanup
}

func TestPingUpdatesLastSeen(t *testing.T) {
	sessions, users, _, authHandler, cfg, _, _, _, _, cleanup := setupSessionEnv(t)
	defer cleanup()
	ctx := context.Background()
	ph := auth.MustHashPassword("p1", cfg.Pepper)
	u := &store.User{Username: "alice", PasswordHash: ph.Hash, Salt: ph.Salt, PasswordSet: true, Active: true}
	uid, _ := users.Create(ctx, u, []string{"admin"})
	u.ID = uid
	sm := auth.NewSessionManager(sessions, cfg, nil)
	sess, err := sm.Create(ctx, u, []string{"admin"}, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	before := sess.LastSeenAt
	saved, _ := sessions.GetSession(ctx, sess.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/app/ping", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.SessionContextKey, saved))
	rr := httptest.NewRecorder()
	authHandler.Ping(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	updated, _ := sessions.GetSession(ctx, sess.ID)
	if updated == nil {
		t.Fatalf("session missing after ping")
	}
	if !updated.LastSeenAt.After(before) {
		t.Fatalf("last_seen_at not updated: %v -> %v", before, updated.LastSeenAt)
	}
}

func TestOnlineWindowCountsOnlyRecent(t *testing.T) {
	sessions, users, acc, _, cfg, _, _, _, _, cleanup := setupSessionEnv(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Now().UTC()
	cfg.Security.OnlineWindowSec = 300
	user1 := &store.User{Username: "u1", FullName: "User 1", PasswordHash: "h", Salt: "s", PasswordSet: true, Active: true}
	uid1, _ := users.Create(ctx, user1, []string{"admin"})
	user2 := &store.User{Username: "u2", FullName: "User 2", PasswordHash: "h", Salt: "s", PasswordSet: true, Active: true}
	uid2, _ := users.Create(ctx, user2, []string{"admin"})
	if err := sessions.SaveSession(ctx, &store.SessionRecord{
		ID:         "sess-new",
		UserID:     uid1,
		Username:   user1.Username,
		Roles:      []string{"admin"},
		IP:         "1.1.1.1",
		UserAgent:  "ua",
		CSRFToken:  "csrf1",
		CreatedAt:  now.Add(-time.Minute),
		LastSeenAt: now.Add(-time.Minute),
		ExpiresAt:  now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("save session1: %v", err)
	}
	if err := sessions.SaveSession(ctx, &store.SessionRecord{
		ID:         "sess-old",
		UserID:     uid2,
		Username:   user2.Username,
		Roles:      []string{"admin"},
		IP:         "2.2.2.2",
		UserAgent:  "ua2",
		CSRFToken:  "csrf2",
		CreatedAt:  now.Add(-30 * time.Minute),
		LastSeenAt: now.Add(-10 * time.Minute),
		ExpiresAt:  now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("save session2: %v", err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/accounts/dashboard", nil)
	acc.Dashboard(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("dashboard code %d", rr.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	onlineVal, ok := resp["online_count"].(float64)
	if !ok {
		t.Fatalf("online_count missing")
	}
	if int(onlineVal) != 1 {
		t.Fatalf("expected 1 online, got %d", int(onlineVal))
	}
	listVal, ok := resp["online_users"].([]any)
	if !ok {
		t.Fatalf("online_users missing")
	}
	if len(listVal) != 1 {
		t.Fatalf("expected 1 online user entry, got %d", len(listVal))
	}
}

func TestKillSessionAndKillAllRevoke(t *testing.T) {
	sessions, users, acc, _, cfg, db, _, _, _, cleanup := setupSessionEnv(t)
	defer cleanup()
	ctx := context.Background()
	ph := auth.MustHashPassword("p2", cfg.Pepper)
	u := &store.User{Username: "target", PasswordHash: ph.Hash, Salt: ph.Salt, PasswordSet: true, Active: true}
	uid, _ := users.Create(ctx, u, []string{"admin"})
	u.ID = uid
	sm := auth.NewSessionManager(sessions, cfg, nil)
	s1, err := sm.Create(ctx, u, []string{"admin"}, "10.0.0.1", "ua1")
	if err != nil {
		t.Fatalf("create session1: %v", err)
	}
	s2, err := sm.Create(ctx, u, []string{"admin"}, "10.0.0.2", "ua2")
	if err != nil {
		t.Fatalf("create session2: %v", err)
	}

	adminSess := &store.SessionRecord{Username: "admin", Roles: []string{"admin"}}
	req := httptest.NewRequest(http.MethodPost, "/api/accounts/sessions/"+s1.ID+"/kill", nil)
	req = withURLParams(req, map[string]string{"session_id": s1.ID})
	req = req.WithContext(context.WithValue(req.Context(), auth.SessionContextKey, adminSess))
	rr := httptest.NewRecorder()
	acc.KillSession(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("kill session code %d", rr.Code)
	}
	var revoked int
	var by string
	if err := db.QueryRowContext(ctx, `SELECT revoked, revoked_by FROM sessions WHERE id=?`, s1.ID).Scan(&revoked, &by); err != nil {
		t.Fatalf("query revoked: %v", err)
	}
	if revoked != 1 || by != "admin" {
		t.Fatalf("session not revoked correctly: revoked=%d by=%s", revoked, by)
	}

	reqAll := httptest.NewRequest(http.MethodPost, "/api/accounts/users/kill_all", nil)
	reqAll = withURLParams(reqAll, map[string]string{"id": strconv.FormatInt(uid, 10)})
	reqAll = reqAll.WithContext(context.WithValue(reqAll.Context(), auth.SessionContextKey, adminSess))
	rrAll := httptest.NewRecorder()
	acc.KillAllSessions(rrAll, reqAll)
	if rrAll.Code != http.StatusOK {
		t.Fatalf("kill all code %d", rrAll.Code)
	}
	if err := db.QueryRowContext(ctx, `SELECT revoked FROM sessions WHERE id=?`, s2.ID).Scan(&revoked); err != nil {
		t.Fatalf("query revoked2: %v", err)
	}
	if revoked != 1 {
		t.Fatalf("expected session 2 revoked")
	}
}

func TestAccessDeniedWithoutPermission(t *testing.T) {
	_, _, acc, _, _, _, _, _, _, cleanup := setupSessionEnv(t)
	defer cleanup()
	policy := rbac.NewPolicy([]rbac.Role{{Name: "viewer", Permissions: []rbac.Permission{}}})
	handler := wrapRequireAny(policy, []rbac.Permission{"accounts.manage"})(acc.KillAllSessions)
	req := httptest.NewRequest(http.MethodPost, "/api/accounts/users/1/sessions/kill_all", nil)
	req = withURLParams(req, map[string]string{"id": "1"})
	req = req.WithContext(context.WithValue(req.Context(), auth.SessionContextKey, &store.SessionRecord{Username: "bob", Roles: []string{"viewer"}}))
	rr := httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func wrapRequireAny(policy *rbac.Policy, perms []rbac.Permission) func(http.HandlerFunc) http.HandlerFunc {
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
				if policy.Allowed(sess.Roles, p) {
					allowed = true
					break
				}
			}
			if !allowed {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next(w, r)
		}
	}
}
