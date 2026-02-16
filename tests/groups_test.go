package tests

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"berkut-scc/api/handlers"
	"berkut-scc/config"
	"berkut-scc/core/auth"
	"berkut-scc/core/rbac"
	"berkut-scc/core/store"
	"berkut-scc/core/utils"
)

type mockSessions struct{ killed int }

func (m *mockSessions) SaveSession(ctx context.Context, sess *store.SessionRecord) error { return nil }
func (m *mockSessions) GetSession(ctx context.Context, id string) (*store.SessionRecord, error) {
	return nil, nil
}
func (m *mockSessions) ListByUser(ctx context.Context, userID int64) ([]store.SessionRecord, error) {
	return nil, nil
}
func (m *mockSessions) ListAll(ctx context.Context) ([]store.SessionRecord, error)    { return nil, nil }
func (m *mockSessions) DeleteSession(ctx context.Context, id string, by string) error { return nil }
func (m *mockSessions) DeleteAllForUser(ctx context.Context, userID int64, by string) error {
	m.killed++
	return nil
}
func (m *mockSessions) DeleteAll(ctx context.Context, by string) error { return nil }
func (m *mockSessions) UpdateActivity(ctx context.Context, id string, now time.Time, extendBy time.Duration) error {
	return nil
}

func TestEffectivePermissionsUnion(t *testing.T) {
	policy := rbac.NewPolicy(rbac.DefaultRoles())
	user := &store.User{ClearanceLevel: 1, ClearanceTags: []string{"alpha"}}
	groups := []store.Group{
		{
			ID:              1,
			Name:            "g1",
			Roles:           []string{"auditor"},
			ClearanceLevel:  3,
			ClearanceTags:   []string{"bravo"},
			MenuPermissions: []string{"accounts"},
		},
	}
	eff := auth.CalculateEffectiveAccess(user, []string{"doc_viewer"}, groups, policy)
	if eff.ClearanceLevel != 3 {
		t.Fatalf("expected clearance 3, got %d", eff.ClearanceLevel)
	}
	if !sliceContains(eff.Roles, "doc_viewer") || !sliceContains(eff.Roles, "auditor") {
		t.Fatalf("expected roles union, got %v", eff.Roles)
	}
	if !sliceContains(eff.Permissions, "logs.view") {
		t.Fatalf("expected permission from group role")
	}
	if !sliceContains(eff.ClearanceTags, "alpha") || !sliceContains(eff.ClearanceTags, "bravo") {
		t.Fatalf("expected clearance tags union, got %v", eff.ClearanceTags)
	}
}

func TestGroupsStoreGetNotFound(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.AppConfig{DBPath: filepath.Join(dir, "groups.db"), Pepper: "pepper"}
	logger := utils.NewLogger()
	db, err := store.NewDB(cfg, logger)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	defer db.Close()
	if err := store.ApplyMigrations(context.Background(), db, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	groups := store.NewGroupsStore(db)
	group, members, roles, err := groups.Get(context.Background(), 999)
	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
	if group != nil || members != nil || roles != nil {
		t.Fatalf("expected nil results on not found")
	}
}

func TestMenuEndpointRespectsGroupPermissions(t *testing.T) {
	acc, authHandler, users, groups, policy, _, cleanup := setupGroupHandlers(t)
	defer cleanup()
	ph := auth.MustHashPassword("p", "pepper")
	user := &store.User{Username: "menu-user", PasswordHash: ph.Hash, Salt: ph.Salt, PasswordSet: true, Active: true}
	userID, _ := users.Create(context.Background(), user, []string{})
	group := &store.Group{Name: "menu-group", ClearanceLevel: 1, MenuPermissions: []string{"accounts"}}
	gid, _ := groups.Create(context.Background(), group, []string{"admin"}, []int64{userID})
	if gid == 0 {
		t.Fatalf("group not created")
	}
	req := httptest.NewRequest(http.MethodGet, "/api/app/menu", nil)
	sr := &store.SessionRecord{Username: "menu-user"}
	req = req.WithContext(context.WithValue(req.Context(), auth.SessionContextKey, sr))
	rr := httptest.NewRecorder()
	authHandler.Menu(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp struct {
		Menu []map[string]string `json:"menu"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Menu) != 1 || resp.Menu[0]["name"] != "accounts" {
		t.Fatalf("expected only accounts menu, got %+v", resp.Menu)
	}
	_ = policy // silence unused in case
	_ = acc    // keep accounts handler constructed
}

func TestRemoveGroupMemberKillsSessions(t *testing.T) {
	acc, _, users, groups, _, sessions, cleanup := setupGroupHandlers(t)
	defer cleanup()
	ph := auth.MustHashPassword("p", "pepper")
	admin := &store.User{Username: "admin", PasswordHash: ph.Hash, Salt: ph.Salt, PasswordSet: true, Active: true, ClearanceLevel: 5}
	adminID, _ := users.Create(context.Background(), admin, []string{"admin"})
	target := &store.User{Username: "member", PasswordHash: ph.Hash, Salt: ph.Salt, PasswordSet: true, Active: true}
	targetID, _ := users.Create(context.Background(), target, []string{})
	group := &store.Group{Name: "g1", ClearanceLevel: 1}
	gid, _ := groups.Create(context.Background(), group, []string{"admin"}, []int64{targetID})
	req := httptest.NewRequest(http.MethodDelete, "/api/accounts/groups", nil)
	req = makeSessionContext(req, "admin", adminID, []string{"admin"})
	rr := httptest.NewRecorder()
	acc.RemoveGroupMember(rr, withURLParams(req, map[string]string{"id": strconv.FormatInt(gid, 10), "user_id": strconv.FormatInt(targetID, 10)}))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if sessions.killed == 0 {
		t.Fatalf("expected sessions to be killed")
	}
}

func TestCannotAssignHigherClearanceGroup(t *testing.T) {
	acc, _, users, groups, _, _, cleanup := setupGroupHandlers(t)
	defer cleanup()
	ph := auth.MustHashPassword("p", "pepper")
	admin := &store.User{Username: "low-admin", PasswordHash: ph.Hash, Salt: ph.Salt, PasswordSet: true, Active: true, ClearanceLevel: 1}
	adminID, _ := users.Create(context.Background(), admin, []string{"admin"})
	target := &store.User{Username: "member2", PasswordHash: ph.Hash, Salt: ph.Salt, PasswordSet: true, Active: true}
	targetID, _ := users.Create(context.Background(), target, []string{})
	group := &store.Group{Name: "high", ClearanceLevel: 5}
	gid, _ := groups.Create(context.Background(), group, []string{"admin"}, []int64{})
	payload, _ := json.Marshal(map[string]int64{"user_id": targetID})
	req := httptest.NewRequest(http.MethodPost, "/api/accounts/groups", bytes.NewBuffer(payload))
	req = makeSessionContext(req, "low-admin", adminID, []string{"admin"})
	rr := httptest.NewRecorder()
	acc.AddGroupMember(rr, withURLParams(req, map[string]string{"id": strconv.FormatInt(gid, 10)}))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func setupGroupHandlers(t *testing.T) (*handlers.AccountsHandler, *handlers.AuthHandler, store.UsersStore, store.GroupsStore, *rbac.Policy, *mockSessions, func()) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.AppConfig{DBPath: filepath.Join(dir, "groups.db"), Pepper: "pepper"}
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
	groups := store.NewGroupsStore(db)
	roles := store.NewRolesStore(db)
	if err := roles.EnsureBuiltIn(context.Background(), convertRoles(rbac.DefaultRoles())); err != nil {
		t.Fatalf("ensure roles: %v", err)
	}
	policy := rbac.NewPolicy(rbac.DefaultRoles())
	sessions := &mockSessions{}
	audits := store.NewAuditStore(db)
	incidents := store.NewIncidentsStore(db)
	acc := handlers.NewAccountsHandler(users, groups, roles, sessions, policy, auth.NewSessionManager(sessions, cfg, logger), cfg, audits, logger, nil)
	authHandler := handlers.NewAuthHandler(cfg, users, sessions, incidents, auth.NewSessionManager(sessions, cfg, logger), policy, audits, logger)
	return acc, authHandler, users, groups, policy, sessions, func() { db.Close() }
}

func convertRoles(in []rbac.Role) []store.Role {
	out := make([]store.Role, 0, len(in))
	for _, r := range in {
		perms := make([]string, 0, len(r.Permissions))
		for _, p := range r.Permissions {
			perms = append(perms, string(p))
		}
		out = append(out, store.Role{Name: r.Name, Permissions: perms, BuiltIn: true})
	}
	return out
}

func sliceContains(arr []string, target string) bool {
	for _, v := range arr {
		if strings.EqualFold(v, target) {
			return true
		}
	}
	return false
}

func makeSessionContext(r *http.Request, username string, userID int64, roles []string) *http.Request {
	rec := &store.SessionRecord{Username: username, UserID: userID, Roles: roles}
	ctx := context.WithValue(r.Context(), auth.SessionContextKey, rec)
	return r.WithContext(ctx)
}
