package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"berkut-scc/config"
	"berkut-scc/core/auth"
	"berkut-scc/core/bootstrap"
	"berkut-scc/core/rbac"
	"berkut-scc/core/store"
	"berkut-scc/core/utils"
	"berkut-scc/gui"
)

type AccountsHandler struct {
	users          store.UsersStore
	groups         store.GroupsStore
	roles          store.RolesStore
	policy         *rbac.Policy
	sessionManager *auth.SessionManager
	sessions       store.SessionStore
	cfg            *config.AppConfig
	audits         store.AuditStore
	logger         *utils.Logger
	refreshPolicy  func(context.Context) error
	imports        *userImportManager
}

func NewAccountsHandler(users store.UsersStore, groups store.GroupsStore, roles store.RolesStore, sessions store.SessionStore, policy *rbac.Policy, sm *auth.SessionManager, cfg *config.AppConfig, audits store.AuditStore, logger *utils.Logger, refreshPolicy func(context.Context) error) *AccountsHandler {
	return &AccountsHandler{
		users:          users,
		groups:         groups,
		roles:          roles,
		sessionManager: sm,
		sessions:       sessions,
		policy:         policy,
		cfg:            cfg,
		audits:         audits,
		logger:         logger,
		refreshPolicy:  refreshPolicy,
		imports:        newUserImportManager(),
	}
}

func (h *AccountsHandler) Page(w http.ResponseWriter, r *http.Request) {
	data, err := gui.StaticFiles.ReadFile("static/accounts.html")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	http.ServeContent(w, r, "accounts.html", time.Now(), bytes.NewReader(data))
}

func (h *AccountsHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	users, err := h.users.List(ctx)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	sessions, _ := h.sessions.ListAll(ctx)
	now := time.Now().UTC()
	windowSec := h.cfg.Security.OnlineWindowSec
	if windowSec <= 0 {
		windowSec = 300
	}
	cutoff := now.Add(-time.Duration(windowSec) * time.Second)
	onlineUsers := map[int64]time.Time{}
	for _, s := range sessions {
		if s.LastSeenAt.After(cutoff) {
			if prev, ok := onlineUsers[s.UserID]; !ok || s.LastSeenAt.After(prev) {
				onlineUsers[s.UserID] = s.LastSeenAt
			}
		}
	}
	var active, disabled, blocked, withoutPassword, requireChange, without2FA int
	var lastLogins []time.Time
	var lastBlocks []string
	var onlineList []map[string]any
	for _, u := range users {
		if u.Active {
			active++
		} else {
			disabled++
		}
		if u.LockedUntil != nil && u.LockedUntil.After(now) {
			blocked++
			lastBlocks = append(lastBlocks, u.Username)
		}
		if !u.PasswordSet {
			withoutPassword++
		}
		if u.RequirePasswordChange {
			requireChange++
		}
		if !u.TOTPEnabled {
			without2FA++
		}
		if u.LastLoginAt != nil {
			lastLogins = append(lastLogins, *u.LastLoginAt)
		}
		if lastSeen, ok := onlineUsers[u.ID]; ok {
			onlineList = append(onlineList, map[string]any{
				"id":         u.ID,
				"username":   u.Username,
				"full_name":  u.FullName,
				"department": u.Department,
				"last_seen":  lastSeen,
			})
		}
	}
	sort.Slice(lastLogins, func(i, j int) bool { return lastLogins[i].After(lastLogins[j]) })
	sort.Slice(onlineList, func(i, j int) bool {
		lt := onlineList[i]["last_seen"].(time.Time)
		rt := onlineList[j]["last_seen"].(time.Time)
		return lt.After(rt)
	})
	const onlineLimit = 10
	if len(onlineList) > onlineLimit {
		onlineList = onlineList[:onlineLimit]
	}
	payload := map[string]any{
		"total":                  len(users),
		"active":                 active,
		"disabled":               disabled,
		"blocked":                blocked,
		"online":                 len(onlineUsers),
		"online_count":           len(onlineUsers),
		"online_users":           onlineList,
		"without_password":       withoutPassword,
		"require_change":         requireChange,
		"without_2fa":            without2FA,
		"last_logins":            lastLogins,
		"last_blocked_usernames": lastBlocks,
		"online_window_sec":      windowSec,
	}
	writeJSON(w, http.StatusOK, payload)
}

func (h *AccountsHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Safety net: make sure default admin exists even if bootstrap was skipped or failed earlier.
	if err := bootstrap.EnsureDefaultAdminWithStore(ctx, h.users, h.cfg, h.logger); err != nil && h.logger != nil {
		h.logger.Errorf("ensure default admin: %v", err)
	}

	filter := store.UserFilter{}
	q := r.URL.Query()
	filter.Department = strings.TrimSpace(q.Get("department"))
	if gid := strings.TrimSpace(q.Get("group_id")); gid != "" {
		if v, err := strconv.ParseInt(gid, 10, 64); err == nil {
			filter.GroupID = v
		}
	}
	filter.Role = strings.TrimSpace(q.Get("role"))
	filter.Status = strings.TrimSpace(q.Get("status"))
	filter.PasswordStatus = strings.TrimSpace(q.Get("password_status"))
	if hp := strings.TrimSpace(q.Get("has_password")); hp != "" && filter.PasswordStatus == "" {
		val := hp == "1" || strings.ToLower(hp) == "true"
		filter.HasPassword = &val
	}
	if cl := strings.TrimSpace(q.Get("clearance_min")); cl != "" {
		if v, err := strconv.Atoi(cl); err == nil {
			filter.ClearanceMin = v
		}
	}
	if cl := strings.TrimSpace(q.Get("clearance_max")); cl != "" {
		if v, err := strconv.Atoi(cl); err == nil {
			filter.ClearanceMax = v
		}
	}
	filter.Query = strings.TrimSpace(q.Get("q"))

	users, err := h.users.ListFiltered(ctx, filter)
	if err != nil {
		if h.logger != nil {
			h.logger.Errorf("list users: %v", err)
			h.logger.Errorf("list users ctx err: %v", ctx.Err())
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if h.logger != nil {
		var names []string
		for _, u := range users {
			names = append(names, u.Username)
		}
		h.logger.Printf("accounts.list returning %d users: %v", len(users), names)
	}
	users = h.ensureAdminPresent(ctx, users)
	for i := range users {
		eff := auth.CalculateEffectiveAccess(&users[i].User, users[i].Roles, users[i].Groups, h.policy)
		users[i].EffectiveRoles = eff.Roles
		users[i].EffectivePermissions = eff.Permissions
		users[i].EffectiveClearanceLevel = eff.ClearanceLevel
		users[i].EffectiveClearanceTags = eff.ClearanceTags
		users[i].EffectiveMenuPermissions = eff.MenuPermissions
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"users": users})
}

type bulkRequest struct {
	Action  string          `json:"action"`
	UserIDs []int64         `json:"user_ids"`
	Payload json.RawMessage `json:"payload"`
}

type bulkFailure struct {
	UserID int64  `json:"user_id"`
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}

type bulkPasswordResult struct {
	UserID       int64  `json:"user_id"`
	Login        string `json:"login"`
	TempPassword string `json:"temp_password"`
}

func (h *AccountsHandler) BulkUsers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	var req bulkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action == "" || len(req.UserIDs) == 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	userIDs := uniqueIDs(req.UserIDs)
	sess := sessionFromCtx(r)
	if sess == nil || !h.policy.Allowed(sess.Roles, "accounts.manage") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	actor, actorRoles, _ := h.users.FindByUsername(ctx, currentUser(r))
	actorEff, _ := h.effectiveAccess(ctx, actor, actorRoles)
	var failures []bulkFailure
	var passwords []bulkPasswordResult
	success := 0

	for _, id := range userIDs {
		target, roles, err := h.users.Get(ctx, id)
		if err != nil {
			failures = append(failures, bulkFailure{UserID: id, Reason: "server_error"})
			continue
		}
		if target == nil {
			failures = append(failures, bulkFailure{UserID: id, Reason: "not_found"})
			continue
		}
		if isAdminUsername(target.Username) && (action == "lock" || action == "disable") {
			failures = append(failures, bulkFailure{UserID: id, Reason: "forbidden"})
			continue
		}
		if containsRole(roles, "superadmin") && h.isLastSuperadmin(ctx, target.ID) && (action == "lock" || action == "disable") {
			failures = append(failures, bulkFailure{UserID: id, Reason: "last_superadmin"})
			continue
		}
		if sess != nil && sess.UserID == target.ID && (action == "lock" || action == "disable") {
			failures = append(failures, bulkFailure{UserID: id, Reason: "self_lockout"})
			h.audits.Log(ctx, currentUser(r), "accounts.self_lockout_blocked", fmt.Sprintf("%d|bulk_%s", id, action))
			continue
		}
		switch action {
		case "assign_role":
			var p struct {
				RoleID string `json:"role_id"`
			}
			if err := decodePayload(req.Payload, &p); err != nil || strings.TrimSpace(p.RoleID) == "" {
				failures = append(failures, bulkFailure{UserID: id, Reason: "invalid_payload"})
				continue
			}
			roleName := strings.ToLower(strings.TrimSpace(p.RoleID))
			role, err := h.roles.FindByName(ctx, roleName)
			if err != nil {
				failures = append(failures, bulkFailure{UserID: id, Reason: "server_error"})
				continue
			}
			if role == nil {
				failures = append(failures, bulkFailure{UserID: id, Reason: "role_not_found"})
				continue
			}
			if strings.EqualFold(role.Name, "superadmin") && (sess == nil || !containsRole(sess.Roles, "superadmin")) {
				failures = append(failures, bulkFailure{UserID: id, Reason: "forbidden"})
				continue
			}
			newRoles := sanitizeRoles(roles, role.Name)
			if err := h.users.Update(ctx, target, newRoles); err != nil {
				failures = append(failures, bulkFailure{UserID: id, Reason: "server_error"})
				continue
			}
			_ = h.sessions.DeleteAllForUser(ctx, target.ID, currentUser(r))
			h.audits.Log(ctx, currentUser(r), "session.kill_all", fmt.Sprintf("%d|bulk_%s", target.ID, action))
			h.audits.Log(ctx, currentUser(r), "accounts.bulk.assign_role", fmt.Sprintf("%d|%s", target.ID, role.Name))
			success++
		case "assign_group":
			var p struct {
				GroupID int64 `json:"group_id"`
			}
			if err := decodePayload(req.Payload, &p); err != nil || p.GroupID <= 0 {
				failures = append(failures, bulkFailure{UserID: id, Reason: "invalid_payload"})
				continue
			}
			g, _, _, err := h.groups.Get(ctx, p.GroupID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					failures = append(failures, bulkFailure{UserID: id, Reason: "group_not_found"})
				} else {
					failures = append(failures, bulkFailure{UserID: id, Reason: "server_error"})
				}
				continue
			}
			if g == nil {
				failures = append(failures, bulkFailure{UserID: id, Reason: "group_not_found"})
				continue
			}
			if err := h.validateGroupAssignments(ctx, actorEff, []int64{p.GroupID}); err != nil {
				code := "server_error"
				if err.Error() == "clearance_too_high" {
					code = "clearance_too_high"
				} else if err.Error() == "clearance_tags_not_allowed" {
					code = "clearance_tags_not_allowed"
				}
				failures = append(failures, bulkFailure{UserID: id, Reason: code})
				continue
			}
			if err := h.groups.AddMember(ctx, p.GroupID, target.ID); err != nil {
				failures = append(failures, bulkFailure{UserID: id, Reason: "server_error"})
				continue
			}
			_ = h.sessions.DeleteAllForUser(ctx, target.ID, currentUser(r))
			h.audits.Log(ctx, currentUser(r), "session.kill_all", fmt.Sprintf("%d|bulk_%s", target.ID, action))
			h.audits.Log(ctx, currentUser(r), "accounts.bulk.assign_group", fmt.Sprintf("%d|%d", target.ID, p.GroupID))
			success++
		case "reset_password":
			var p struct {
				TempPassword string `json:"temp_password"`
				MustChange   *bool  `json:"must_change"`
			}
			if err := decodePayload(req.Payload, &p); err != nil {
				failures = append(failures, bulkFailure{UserID: id, Reason: "invalid_payload"})
				continue
			}
			tempPwd := strings.TrimSpace(p.TempPassword)
			generated := false
			if tempPwd == "" {
				tempPwd = generateStrongPassword()
				generated = true
			}
			if err := utils.ValidatePassword(tempPwd); err != nil {
				failures = append(failures, bulkFailure{UserID: id, Reason: "invalid_password", Detail: err.Error()})
				continue
			}
			history, _ := h.users.PasswordHistory(ctx, id, 10)
			if isPasswordReused(tempPwd, h.cfg.Pepper, target, history) {
				failures = append(failures, bulkFailure{UserID: id, Reason: "password_reused"})
				continue
			}
			ph, err := auth.HashPassword(tempPwd, h.cfg.Pepper)
			if err != nil {
				failures = append(failures, bulkFailure{UserID: id, Reason: "server_error"})
				continue
			}
			requireChange := true
			if p.MustChange != nil {
				requireChange = *p.MustChange
			}
			if err := h.users.UpdatePassword(ctx, id, ph.Hash, ph.Salt, requireChange); err != nil {
				failures = append(failures, bulkFailure{UserID: id, Reason: "server_error"})
				continue
			}
			_ = h.sessions.DeleteAllForUser(ctx, target.ID, currentUser(r))
			h.audits.Log(ctx, currentUser(r), "session.kill_all", fmt.Sprintf("%d|bulk_%s", target.ID, action))
			h.audits.Log(ctx, currentUser(r), "accounts.bulk.reset_password", fmt.Sprintf("%d", target.ID))
			if generated {
				passwords = append(passwords, bulkPasswordResult{UserID: target.ID, Login: target.Username, TempPassword: tempPwd})
			}
			success++
		case "lock":
			var p struct {
				Reason  string `json:"reason"`
				Stage   int    `json:"stage"`
				Minutes int    `json:"minutes"`
			}
			_ = decodePayload(req.Payload, &p)
			now := time.Now().UTC()
			target.LockReason = strings.TrimSpace(p.Reason)
			if p.Stage > 0 {
				target.LockStage = p.Stage
			} else {
				target.LockStage = 6
			}
			if target.LockStage >= 6 {
				target.LockedUntil = nil
			} else {
				minutes := p.Minutes
				if minutes <= 0 {
					minutes = 60
				}
				until := now.Add(time.Duration(minutes) * time.Minute)
				target.LockedUntil = &until
			}
			target.FailedAttempts = 0
			if err := h.users.Update(ctx, target, nil); err != nil {
				failures = append(failures, bulkFailure{UserID: id, Reason: "server_error"})
				continue
			}
			_ = h.sessions.DeleteAllForUser(ctx, target.ID, currentUser(r))
			h.audits.Log(ctx, currentUser(r), "session.kill_all", fmt.Sprintf("%d|bulk_%s", target.ID, action))
			h.audits.Log(ctx, currentUser(r), "accounts.bulk.lock", fmt.Sprintf("%d|%s", target.ID, target.LockReason))
			success++
		case "unlock":
			var p struct {
				Reason string `json:"reason"`
			}
			_ = decodePayload(req.Payload, &p)
			target.LockedUntil = nil
			target.LockReason = ""
			target.FailedAttempts = 0
			target.LockStage = 0
			if err := h.users.Update(ctx, target, nil); err != nil {
				failures = append(failures, bulkFailure{UserID: id, Reason: "server_error"})
				continue
			}
			_ = h.sessions.DeleteAllForUser(ctx, target.ID, currentUser(r))
			h.audits.Log(ctx, currentUser(r), "session.kill_all", fmt.Sprintf("%d|bulk_%s", target.ID, action))
			h.audits.Log(ctx, currentUser(r), "accounts.bulk.unlock", fmt.Sprintf("%d|%s", target.ID, strings.TrimSpace(p.Reason)))
			success++
		case "disable", "enable":
			enable := action == "enable"
			var p struct {
				Reason string `json:"reason"`
			}
			_ = decodePayload(req.Payload, &p)
			if err := h.users.SetActive(ctx, target.ID, enable); err != nil {
				failures = append(failures, bulkFailure{UserID: id, Reason: "server_error"})
				continue
			}
			_ = h.sessions.DeleteAllForUser(ctx, target.ID, currentUser(r))
			h.audits.Log(ctx, currentUser(r), "session.kill_all", fmt.Sprintf("%d|bulk_%s", target.ID, action))
			act := "accounts.bulk.disable"
			if enable {
				act = "accounts.bulk.enable"
			}
			h.audits.Log(ctx, currentUser(r), act, fmt.Sprintf("%d|%s", target.ID, strings.TrimSpace(p.Reason)))
			success++
		default:
			http.Error(w, "unsupported action", http.StatusBadRequest)
			return
		}
	}

	resp := map[string]any{
		"success_count": success,
		"failed_count":  len(failures),
		"failures":      failures,
	}
	if len(passwords) > 0 {
		resp["passwords"] = passwords
	}
	h.audits.Log(ctx, currentUser(r), "accounts.bulk_action", fmt.Sprintf("%s|%d|%d", action, success, len(failures)))
	writeJSON(w, http.StatusOK, resp)
}

type accountPayload struct {
	Username              string   `json:"username"`
	Email                 string   `json:"email"`
	Password              string   `json:"password"`
	Role                  string   `json:"role"`
	Roles                 []string `json:"roles"`
	Groups                []int64  `json:"groups"`
	FullName              string   `json:"full_name"`
	Department            string   `json:"department"`
	Position              string   `json:"position"`
	ClearanceLevel        int      `json:"clearance_level"`
	ClearanceTags         []string `json:"clearance_tags"`
	Status                string   `json:"status"`
	Active                *bool    `json:"active,omitempty"`
	RequirePasswordChange bool     `json:"require_password_change"`
}

func sanitizeRoles(in []string, fallback string) []string {
	m := map[string]struct{}{}
	if fallback != "" {
		in = append(in, fallback)
	}
	for _, r := range in {
		r = strings.ToLower(strings.TrimSpace(r))
		if r == "" {
			continue
		}
		m[r] = struct{}{}
	}
	out := make([]string, 0, len(m))
	for r := range m {
		out = append(out, r)
	}
	return out
}

var allowedMenuKeys = map[string]struct{}{
	"dashboard": {}, "tasks": {}, "controls": {}, "monitoring": {}, "incidents": {}, "docs": {},
	"reports": {}, "backups": {}, "accounts": {}, "settings": {}, "logs": {},
}

var menuKeyAliases = map[string]string{
	"documents": "docs",
	"document":  "docs",
	"approvals": "docs",
	"approval":  "docs",
}

func sanitizeMenuPermissions(perms []string) []string {
	set := map[string]struct{}{}
	for _, p := range perms {
		val := strings.ToLower(strings.TrimSpace(p))
		if val == "" {
			continue
		}
		if mapped, ok := menuKeyAliases[val]; ok {
			val = mapped
		}
		if _, ok := allowedMenuKeys[val]; ok {
			set[val] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func (h *AccountsHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var p accountPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		if h.logger != nil {
			h.logger.Errorf("create user decode: %v", err)
		}
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if h.logger != nil {
		h.logger.Printf("accounts.create start by=%s payload username=%s role=%s roles=%v status=%s", currentUser(r), p.Username, p.Role, p.Roles, p.Status)
	}
	sess := sessionFromCtx(r)
	actor, actorRoles, _ := h.users.FindByUsername(ctx, currentUser(r))
	actorEff, _ := h.effectiveAccess(ctx, actor, actorRoles)
	p.Username = strings.ToLower(strings.TrimSpace(p.Username))
	if err := utils.ValidateUsername(p.Username); err != nil {
		if h.logger != nil {
			h.logger.Errorf("create user invalid username=%s: %v", p.Username, err)
		}
		http.Error(w, "invalid username", http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(p.Email)
	fullName := strings.TrimSpace(p.FullName)
	dept := strings.TrimSpace(p.Department)
	pos := strings.TrimSpace(p.Position)
	clearanceLevel := p.ClearanceLevel
	if clearanceLevel < 0 {
		clearanceLevel = 0
	}
	if actor != nil && clearanceLevel > actorEff.ClearanceLevel {
		http.Error(w, localized(preferredLang(r), "accounts.clearanceTooHigh"), http.StatusForbidden)
		return
	}

	passwordSet := true
	passwordValue := strings.TrimSpace(p.Password)
	if passwordValue == "" {
		if h.logger != nil {
			h.logger.Printf("accounts.create username=%s no password provided; generating temp", p.Username)
		}
		passwordSet = false
		passwordValue, _ = utils.RandString(16)
	} else {
		if err := utils.ValidatePassword(passwordValue); err != nil {
			if h.logger != nil {
				h.logger.Errorf("create user invalid password username=%s: %v", p.Username, err)
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	ph, err := auth.HashPassword(passwordValue, h.cfg.Pepper)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	hash, salt := ph.Hash, ph.Salt

	active := true
	var disabledAt *time.Time
	if p.Status == "disabled" {
		active = false
		now := time.Now().UTC()
		disabledAt = &now
	}

	roles := sanitizeRoles(p.Roles, p.Role)
	if len(roles) == 0 {
		http.Error(w, "role required", http.StatusBadRequest)
		return
	}
	if containsRole(roles, "superadmin") && (sess == nil || !containsRole(sess.Roles, "superadmin")) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	tags := sanitizeTags(p.ClearanceTags)
	if !h.canAssignTags(&store.User{ClearanceTags: actorEff.ClearanceTags}, tags) {
		http.Error(w, localized(preferredLang(r), "accounts.clearanceTagsNotAllowed"), http.StatusForbidden)
		return
	}
	u := &store.User{
		Username:              p.Username,
		Email:                 email,
		FullName:              fullName,
		Department:            dept,
		Position:              pos,
		PasswordHash:          hash,
		Salt:                  salt,
		PasswordSet:           passwordSet,
		Active:                active,
		DisabledAt:            disabledAt,
		ClearanceLevel:        clearanceLevel,
		ClearanceTags:         tags,
		RequirePasswordChange: p.RequirePasswordChange || !passwordSet,
	}
	id, err := h.users.Create(r.Context(), u, roles)
	if err != nil {
		if h.logger != nil {
			h.logger.Errorf("create user (%s): %v", p.Username, err)
			h.logger.Errorf("create user ctx err: %v", ctx.Err())
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if h.logger != nil {
		h.logger.Printf("user created (%s) id=%d roles=%v", p.Username, id, roles)
	}
	if len(p.Groups) > 0 {
		groupIDs := uniqueIDs(p.Groups)
		if err := h.validateGroupAssignments(ctx, actorEff, groupIDs); err != nil {
			if err.Error() == "clearance_too_high" {
				http.Error(w, localized(preferredLang(r), "accounts.clearanceTooHigh"), http.StatusForbidden)
				return
			}
			if err.Error() == "clearance_tags_not_allowed" {
				http.Error(w, localized(preferredLang(r), "accounts.clearanceTagsNotAllowed"), http.StatusForbidden)
				return
			}
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		_ = h.groups.SetUserGroups(ctx, id, groupIDs)
	}
	h.audits.Log(r.Context(), currentUser(r), "create_user", p.Username)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *AccountsHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	idStr := pathParams(r)["id"]
	id, _ := strconv.ParseInt(idStr, 10, 64)
	existing, roles, err := h.users.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if existing == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if h.denyAdminChange(w, r, existing, true) {
		return
	}
	originalRoles := make([]string, len(roles))
	copy(originalRoles, roles)
	updatedRoles := roles
	rolesChanged := false
	var p accountPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	sess := sessionFromCtx(r)
	actor, actorRoles, _ := h.users.FindByUsername(r.Context(), currentUser(r))
	actorEff, _ := h.effectiveAccess(r.Context(), actor, actorRoles)
	if strings.TrimSpace(p.Email) != "" {
		existing.Email = strings.TrimSpace(p.Email)
	}
	if strings.TrimSpace(p.FullName) != "" {
		existing.FullName = strings.TrimSpace(p.FullName)
	}
	if strings.TrimSpace(p.Department) != "" {
		existing.Department = strings.TrimSpace(p.Department)
	}
	if strings.TrimSpace(p.Position) != "" {
		existing.Position = strings.TrimSpace(p.Position)
	}
	if p.ClearanceLevel > 0 {
		if actor != nil && p.ClearanceLevel > actorEff.ClearanceLevel {
			http.Error(w, localized(preferredLang(r), "accounts.clearanceTooHigh"), http.StatusForbidden)
			return
		}
		existing.ClearanceLevel = p.ClearanceLevel
	}
	if len(p.ClearanceTags) > 0 {
		tags := sanitizeTags(p.ClearanceTags)
		if !h.canAssignTags(&store.User{ClearanceTags: actorEff.ClearanceTags}, tags) {
			http.Error(w, localized(preferredLang(r), "accounts.clearanceTagsNotAllowed"), http.StatusForbidden)
			return
		}
		existing.ClearanceTags = tags
	}
	if p.Status != "" {
		if p.Status == "disabled" && containsRole(originalRoles, "superadmin") && h.isLastSuperadmin(r.Context(), id) {
			http.Error(w, localized(preferredLang(r), "accounts.lastSuperadminProtected"), http.StatusConflict)
			h.audits.Log(r.Context(), currentUser(r), "accounts.last_superadmin_blocked", idStr)
			return
		}
		if p.Status == "disabled" {
			existing.Active = false
			now := time.Now().UTC()
			existing.DisabledAt = &now
		} else {
			existing.Active = true
			existing.DisabledAt = nil
		}
	} else if p.Active != nil {
		if !*p.Active && containsRole(originalRoles, "superadmin") && h.isLastSuperadmin(r.Context(), id) {
			http.Error(w, localized(preferredLang(r), "accounts.lastSuperadminProtected"), http.StatusConflict)
			h.audits.Log(r.Context(), currentUser(r), "accounts.last_superadmin_blocked", idStr)
			return
		}
		existing.Active = *p.Active
		if *p.Active {
			existing.DisabledAt = nil
		} else {
			now := time.Now().UTC()
			existing.DisabledAt = &now
		}
	}
	if p.Roles != nil || p.Role != "" {
		updatedRoles = sanitizeRoles(p.Roles, p.Role)
		rolesChanged = true
		if containsRole(updatedRoles, "superadmin") && (sess == nil || !containsRole(sess.Roles, "superadmin")) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if id == sess.UserID && !h.policy.Allowed(updatedRoles, "accounts.manage") {
			http.Error(w, localized(preferredLang(r), "accounts.selfLockoutPrevented"), http.StatusConflict)
			h.audits.Log(r.Context(), currentUser(r), "accounts.self_lockout_blocked", idStr)
			return
		}
		if containsRole(originalRoles, "superadmin") && h.isLastSuperadmin(r.Context(), id) && !containsRole(updatedRoles, "superadmin") {
			http.Error(w, localized(preferredLang(r), "accounts.lastSuperadminProtected"), http.StatusConflict)
			h.audits.Log(r.Context(), currentUser(r), "accounts.last_superadmin_blocked", idStr)
			return
		}
	}
	if !existing.Active && isAdminUsername(existing.Username) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if p.RequirePasswordChange {
		existing.RequirePasswordChange = true
	}
	var directRoles []string
	if rolesChanged {
		directRoles = updatedRoles
	}
	if err := h.users.Update(r.Context(), existing, directRoles); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if p.Roles != nil || p.Role != "" {
		h.audits.Log(r.Context(), currentUser(r), "accounts.roles_changed", idStr)
	}
	if p.Groups != nil {
		h.audits.Log(r.Context(), currentUser(r), "accounts.groups_changed", idStr)
	}
	if p.ClearanceLevel > 0 || len(p.ClearanceTags) > 0 {
		h.audits.Log(r.Context(), currentUser(r), "accounts.clearance_changed", idStr)
	}
	if p.Groups != nil {
		groupIDs := uniqueIDs(p.Groups)
		if h.groups != nil && h.isLastSuperadmin(r.Context(), existing.ID) {
			directRoles, _ := h.users.UserDirectRoles(r.Context(), existing.ID)
			if !containsRole(directRoles, "superadmin") {
				groups, err := h.groupIDsToGroups(r.Context(), groupIDs)
				if err != nil {
					http.Error(w, "server error", http.StatusInternalServerError)
					return
				}
				hasSuperadminGroup := false
				for _, g := range groups {
					if containsRole(g.Roles, "superadmin") {
						hasSuperadminGroup = true
						break
					}
				}
				if !hasSuperadminGroup {
					http.Error(w, localized(preferredLang(r), "accounts.lastSuperadminProtected"), http.StatusConflict)
					h.audits.Log(r.Context(), currentUser(r), "accounts.last_superadmin_blocked", idStr)
					return
				}
			}
		}
		if err := h.validateGroupAssignments(r.Context(), actorEff, groupIDs); err != nil {
			if err.Error() == "clearance_too_high" {
				http.Error(w, localized(preferredLang(r), "accounts.clearanceTooHigh"), http.StatusForbidden)
				return
			}
			if err.Error() == "clearance_tags_not_allowed" {
				http.Error(w, localized(preferredLang(r), "accounts.clearanceTagsNotAllowed"), http.StatusForbidden)
				return
			}
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		_ = h.groups.SetUserGroups(r.Context(), existing.ID, groupIDs)
	}
	h.sessions.DeleteAllForUser(r.Context(), existing.ID, currentUser(r))
	h.audits.Log(r.Context(), currentUser(r), "session.kill_all", fmt.Sprintf("%d|security_change", existing.ID))
	h.audits.Log(r.Context(), currentUser(r), "accounts.status_changed", idStr)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AccountsHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	idStr := pathParams(r)["id"]
	id, _ := strconv.ParseInt(idStr, 10, 64)
	existing, _, err := h.users.Get(ctx, id)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if existing == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if h.denyAdminChange(w, r, existing, true) {
		return
	}
	var payload struct {
		Password      string `json:"password"`
		RequireChange bool   `json:"require_change"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := utils.ValidatePassword(payload.Password); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	history, _ := h.users.PasswordHistory(ctx, id, 10)
	if isPasswordReused(payload.Password, h.cfg.Pepper, existing, history) {
		h.audits.Log(r.Context(), currentUser(r), "auth.password_reuse_denied", idStr)
		http.Error(w, localized(preferredLang(r), "accounts.passwordReuseDenied"), http.StatusBadRequest)
		return
	}
	ph, err := auth.HashPassword(payload.Password, h.cfg.Pepper)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	reqChange := true
	if payload.RequireChange == false {
		reqChange = false
	}
	if err := h.users.UpdatePassword(ctx, id, ph.Hash, ph.Salt, reqChange); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.audits.Log(r.Context(), currentUser(r), "auth.password_reset", idStr)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AccountsHandler) LockUser(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	idStr := pathParams(r)["id"]
	id, _ := strconv.ParseInt(idStr, 10, 64)
	user, _, err := h.users.Get(ctx, id)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if user == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if sess := sessionFromCtx(r); sess != nil && sess.UserID == user.ID {
		http.Error(w, localized(preferredLang(r), "accounts.selfLockoutPrevented"), http.StatusConflict)
		h.audits.Log(r.Context(), currentUser(r), "accounts.self_lockout_blocked", idStr)
		return
	}
	if isAdminUsername(user.Username) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var payload struct {
		Reason  string `json:"reason"`
		Minutes int    `json:"minutes"`
	}
	_ = json.NewDecoder(r.Body).Decode(&payload)
	now := time.Now().UTC()
	reason := strings.TrimSpace(payload.Reason)
	if payload.Minutes <= 0 {
		user.LockStage = 6
		user.LockedUntil = nil
		user.LockReason = reason
		user.FailedAttempts = 0
	} else {
		user.LockStage++
		user.FailedAttempts = 0
		dur := time.Duration(payload.Minutes) * time.Minute
		if dur <= 0 {
			dur = time.Hour
		}
		until := now.Add(dur)
		user.LockedUntil = &until
		user.LockReason = reason
	}
	if err := h.users.Update(ctx, user, nil); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.sessions.DeleteAllForUser(ctx, user.ID, currentUser(r))
	h.audits.Log(r.Context(), currentUser(r), "session.kill_all", fmt.Sprintf("%d|security_change", user.ID))
	h.audits.Log(r.Context(), currentUser(r), "auth.lock_manual", fmt.Sprintf("%d|%s", id, user.LockReason))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AccountsHandler) UnlockUser(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	idStr := pathParams(r)["id"]
	id, _ := strconv.ParseInt(idStr, 10, 64)
	user, _, err := h.users.Get(ctx, id)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if user == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var payload struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&payload)
	user.LockedUntil = nil
	user.LockReason = ""
	user.FailedAttempts = 0
	user.LockStage = 0
	if err := h.users.Update(ctx, user, nil); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	_ = h.sessions.DeleteAllForUser(ctx, user.ID, currentUser(r))
	h.audits.Log(r.Context(), currentUser(r), "session.kill_all", fmt.Sprintf("%d|security_change", user.ID))
	h.audits.Log(r.Context(), currentUser(r), "auth.unlock", fmt.Sprintf("%s|%s", idStr, strings.TrimSpace(payload.Reason)))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AccountsHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	idStr := pathParams(r)["id"]
	id, _ := strconv.ParseInt(idStr, 10, 64)
	sess, err := h.sessions.ListByUser(ctx, id)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sess})
}

func (h *AccountsHandler) KillSession(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	sessID := pathParams(r)["session_id"]
	if sessID == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	sess, err := h.sessions.GetSession(ctx, sessID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if sess == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	_ = h.sessions.DeleteSession(ctx, sessID, currentUser(r))
	h.audits.Log(r.Context(), currentUser(r), "session.kill", fmt.Sprintf("%s|%d", sessID, sess.UserID))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AccountsHandler) KillAllSessions(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	idStr := pathParams(r)["id"]
	id, _ := strconv.ParseInt(idStr, 10, 64)
	if id <= 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	_ = h.sessions.DeleteAllForUser(ctx, id, currentUser(r))
	h.audits.Log(r.Context(), currentUser(r), "session.kill_all", idStr)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AccountsHandler) ListGroups(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	groups, err := h.groups.List(ctx)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": groups})
}

func (h *AccountsHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var payload struct {
		Name            string   `json:"name"`
		Description     string   `json:"description"`
		ClearanceLevel  int      `json:"clearance_level"`
		ClearanceTags   []string `json:"clearance_tags"`
		MenuPermissions []string `json:"menu_permissions"`
		Roles           []string `json:"roles"`
		Users           []int64  `json:"users"`
		IsSystem        bool     `json:"is_system"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(payload.Name) == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	actor, actorRoles, _ := h.users.FindByUsername(ctx, currentUser(r))
	actorEff, _ := h.effectiveAccess(ctx, actor, actorRoles)
	menu := sanitizeMenuPermissions(payload.MenuPermissions)
	clearanceTags := sanitizeTags(payload.ClearanceTags)
	if actor != nil && payload.ClearanceLevel > actorEff.ClearanceLevel {
		http.Error(w, localized(preferredLang(r), "accounts.clearanceTooHigh"), http.StatusForbidden)
		return
	}
	if !h.canAssignTags(&store.User{ClearanceTags: actorEff.ClearanceTags}, clearanceTags) {
		http.Error(w, localized(preferredLang(r), "accounts.clearanceTagsNotAllowed"), http.StatusForbidden)
		return
	}
	g := &store.Group{
		Name:            strings.TrimSpace(payload.Name),
		Description:     strings.TrimSpace(payload.Description),
		ClearanceLevel:  payload.ClearanceLevel,
		ClearanceTags:   clearanceTags,
		MenuPermissions: menu,
		IsSystem:        payload.IsSystem,
	}
	roles := sanitizeRoles(payload.Roles, "")
	userIDs := uniqueIDs(payload.Users)
	id, err := h.groups.Create(ctx, g, roles, userIDs)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	for _, uid := range userIDs {
		_ = h.sessions.DeleteAllForUser(ctx, uid, currentUser(r))
	}
	h.audits.Log(r.Context(), currentUser(r), "groups.create", payload.Name)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *AccountsHandler) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	idStr := pathParams(r)["id"]
	id, _ := strconv.ParseInt(idStr, 10, 64)
	existing, members, _, err := h.groups.Get(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
		} else {
			http.Error(w, "server error", http.StatusInternalServerError)
		}
		return
	}
	if existing == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	actor, actorRoles, _ := h.users.FindByUsername(ctx, currentUser(r))
	actorEff, _ := h.effectiveAccess(ctx, actor, actorRoles)
	originalRoles := append([]string{}, existing.Roles...)
	originalMembers := append([]int64{}, members...)
	var payload struct {
		Name            string   `json:"name"`
		Description     string   `json:"description"`
		ClearanceLevel  int      `json:"clearance_level"`
		ClearanceTags   []string `json:"clearance_tags"`
		MenuPermissions []string `json:"menu_permissions"`
		Roles           []string `json:"roles"`
		Users           []int64  `json:"users"`
		IsSystem        *bool    `json:"is_system"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(payload.Name) != "" {
		existing.Name = strings.TrimSpace(payload.Name)
	}
	if payload.Description != "" {
		existing.Description = payload.Description
	}
	if payload.ClearanceLevel > 0 {
		if actor != nil && payload.ClearanceLevel > actorEff.ClearanceLevel {
			http.Error(w, localized(preferredLang(r), "accounts.clearanceTooHigh"), http.StatusForbidden)
			return
		}
		existing.ClearanceLevel = payload.ClearanceLevel
	}
	if payload.ClearanceTags != nil {
		tags := sanitizeTags(payload.ClearanceTags)
		if !h.canAssignTags(&store.User{ClearanceTags: actorEff.ClearanceTags}, tags) {
			http.Error(w, localized(preferredLang(r), "accounts.clearanceTagsNotAllowed"), http.StatusForbidden)
			return
		}
		existing.ClearanceTags = tags
	}
	if payload.MenuPermissions != nil {
		existing.MenuPermissions = sanitizeMenuPermissions(payload.MenuPermissions)
	}
	if payload.IsSystem != nil {
		existing.IsSystem = *payload.IsSystem
	}
	var roleList []string
	if payload.Roles != nil {
		roleList = sanitizeRoles(payload.Roles, "")
	}
	var userIDs []int64
	if payload.Users != nil {
		userIDs = uniqueIDs(payload.Users)
		if actor != nil && existing.ClearanceLevel > actorEff.ClearanceLevel {
			http.Error(w, localized(preferredLang(r), "accounts.clearanceTooHigh"), http.StatusForbidden)
			return
		}
		if !h.canAssignTags(&store.User{ClearanceTags: actorEff.ClearanceTags}, existing.ClearanceTags) {
			http.Error(w, localized(preferredLang(r), "accounts.clearanceTagsNotAllowed"), http.StatusForbidden)
			return
		}
	}
	if err := h.groups.Update(ctx, existing, roleList, userIDs); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	affected := map[int64]struct{}{}
	for _, uid := range originalMembers {
		affected[uid] = struct{}{}
	}
	if payload.Users != nil {
		for _, uid := range userIDs {
			affected[uid] = struct{}{}
		}
	}
	for uid := range affected {
		_ = h.sessions.DeleteAllForUser(ctx, uid, currentUser(r))
	}
	if payload.Roles != nil && !equalStringSets(originalRoles, roleList) {
		h.audits.Log(r.Context(), currentUser(r), "groups.roles_changed", idStr)
	}
	if payload.ClearanceLevel > 0 || payload.ClearanceTags != nil {
		h.audits.Log(r.Context(), currentUser(r), "groups.clearance_changed", idStr)
	}
	if payload.MenuPermissions != nil {
		h.audits.Log(r.Context(), currentUser(r), "groups.menu_changed", idStr)
	}
	if payload.Users != nil {
		added, removed := diffInt64Sets(originalMembers, userIDs)
		if len(added) > 0 {
			h.audits.Log(r.Context(), currentUser(r), "groups.member_add", fmt.Sprintf("%s|%v", idStr, added))
		}
		if len(removed) > 0 {
			h.audits.Log(r.Context(), currentUser(r), "groups.member_remove", fmt.Sprintf("%s|%v", idStr, removed))
		}
	}
	h.audits.Log(r.Context(), currentUser(r), "groups.update", idStr)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AccountsHandler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	idStr := pathParams(r)["id"]
	id, _ := strconv.ParseInt(idStr, 10, 64)
	ctx := r.Context()
	group, members, roles, err := h.groups.Get(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
		} else {
			http.Error(w, "server error", http.StatusInternalServerError)
		}
		return
	}
	if group == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if group.IsSystem {
		http.Error(w, localized(preferredLang(r), "accounts.groupSystemProtected"), http.StatusForbidden)
		return
	}
	if containsRole(roles, "superadmin") {
		for _, uid := range members {
			if h.isLastSuperadmin(ctx, uid) && h.userReliesOnGroupForRole(ctx, uid, id, "superadmin") {
				http.Error(w, localized(preferredLang(r), "accounts.lastSuperadminProtected"), http.StatusConflict)
				return
			}
		}
	}
	if err := h.groups.Delete(ctx, id); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	for _, uid := range members {
		if h.sessions != nil {
			_ = h.sessions.DeleteAllForUser(ctx, uid, currentUser(r))
		}
	}
	h.audits.Log(r.Context(), currentUser(r), "groups.delete", idStr)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AccountsHandler) GetGroup(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	idStr := pathParams(r)["id"]
	id, _ := strconv.ParseInt(idStr, 10, 64)
	group, members, _, err := h.groups.Get(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
		} else {
			http.Error(w, "server error", http.StatusInternalServerError)
		}
		return
	}
	if group == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	group.UserCount = len(members)
	writeJSON(w, http.StatusOK, map[string]any{"group": group, "members": members})
}

func (h *AccountsHandler) AddGroupMember(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	idStr := pathParams(r)["id"]
	id, _ := strconv.ParseInt(idStr, 10, 64)
	group, _, _, err := h.groups.Get(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
		} else {
			http.Error(w, "server error", http.StatusInternalServerError)
		}
		return
	}
	if group == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var payload struct {
		UserID int64 `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.UserID <= 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	actor, actorRoles, _ := h.users.FindByUsername(ctx, currentUser(r))
	actorEff, _ := h.effectiveAccess(ctx, actor, actorRoles)
	if actor != nil && group.ClearanceLevel > actorEff.ClearanceLevel {
		http.Error(w, localized(preferredLang(r), "accounts.clearanceTooHigh"), http.StatusForbidden)
		return
	}
	if !h.canAssignTags(&store.User{ClearanceTags: actorEff.ClearanceTags}, group.ClearanceTags) {
		http.Error(w, localized(preferredLang(r), "accounts.clearanceTagsNotAllowed"), http.StatusForbidden)
		return
	}
	if err := h.groups.AddMember(ctx, id, payload.UserID); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if h.sessions != nil {
		_ = h.sessions.DeleteAllForUser(ctx, payload.UserID, currentUser(r))
	}
	h.audits.Log(r.Context(), currentUser(r), "groups.member_add", fmt.Sprintf("%s|%d", idStr, payload.UserID))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AccountsHandler) RemoveGroupMember(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	idStr := pathParams(r)["id"]
	userStr := pathParams(r)["user_id"]
	id, _ := strconv.ParseInt(idStr, 10, 64)
	userID, _ := strconv.ParseInt(userStr, 10, 64)
	group, _, roles, err := h.groups.Get(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
		} else {
			http.Error(w, "server error", http.StatusInternalServerError)
		}
		return
	}
	if group == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if containsRole(roles, "superadmin") && h.isLastSuperadmin(ctx, userID) && h.userReliesOnGroupForRole(ctx, userID, id, "superadmin") {
		http.Error(w, localized(preferredLang(r), "accounts.lastSuperadminProtected"), http.StatusConflict)
		return
	}
	if err := h.groups.RemoveMember(ctx, id, userID); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if h.sessions != nil {
		_ = h.sessions.DeleteAllForUser(ctx, userID, currentUser(r))
	}
	h.audits.Log(r.Context(), currentUser(r), "groups.member_remove", fmt.Sprintf("%s|%d", idStr, userID))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AccountsHandler) ListUserGroups(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	idStr := pathParams(r)["id"]
	id, _ := strconv.ParseInt(idStr, 10, 64)
	groups, err := h.users.UserGroups(ctx, id)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": groups})
}

func (h *AccountsHandler) ListRoles(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	roles, err := h.roles.List(ctx)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"roles": roles})
}

func (h *AccountsHandler) ListRoleTemplates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"templates": defaultRoleTemplates()})
}

func (h *AccountsHandler) CreateRole(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var payload store.Role
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	payload.Name = strings.ToLower(strings.TrimSpace(payload.Name))
	if payload.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	perms, invalidPerms := rbac.NormalizePermissionNames(payload.Permissions)
	if len(invalidPerms) > 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	payload.Permissions = perms
	payload.BuiltIn = false
	id, err := h.roles.Create(ctx, &payload)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if h.refreshPolicy != nil {
		_ = h.refreshPolicy(ctx)
	}
	h.audits.Log(r.Context(), currentUser(r), "accounts.role_create", payload.Name)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *AccountsHandler) CreateRoleFromTemplate(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var payload struct {
		TemplateID  string `json:"template_id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	tpl := findTemplate(strings.TrimSpace(payload.TemplateID))
	if tpl == nil {
		http.Error(w, localized(preferredLang(r), "errors.roleTemplateNotFound"), http.StatusNotFound)
		return
	}
	name := strings.ToLower(strings.TrimSpace(payload.Name))
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	role := &store.Role{
		Name:        name,
		Description: strings.TrimSpace(payload.Description),
		Permissions: nil,
		BuiltIn:     false,
		Template:    true,
	}
	perms, invalidPerms := rbac.NormalizePermissionNames(tpl.Permissions)
	if len(invalidPerms) > 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	role.Permissions = perms
	id, err := h.roles.Create(ctx, role)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if h.refreshPolicy != nil {
		_ = h.refreshPolicy(ctx)
	}
	h.audits.Log(r.Context(), currentUser(r), "accounts.role_create_from_template", tpl.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *AccountsHandler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	idStr := pathParams(r)["id"]
	rid, _ := strconv.ParseInt(idStr, 10, 64)
	existing, err := h.roles.FindByID(ctx, rid)
	if err != nil || existing == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if existing.BuiltIn {
		http.Error(w, localized(preferredLang(r), "accounts.roleSystemProtected"), http.StatusConflict)
		return
	}
	var payload store.Role
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	existing.Description = payload.Description
	perms, invalidPerms := rbac.NormalizePermissionNames(payload.Permissions)
	if len(invalidPerms) > 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	existing.Permissions = perms
	if err := h.roles.Update(ctx, existing); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if h.refreshPolicy != nil {
		_ = h.refreshPolicy(ctx)
	}
	h.audits.Log(r.Context(), currentUser(r), "accounts.role_update", idStr)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AccountsHandler) DeleteRole(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	idStr := pathParams(r)["id"]
	rid, _ := strconv.ParseInt(idStr, 10, 64)
	existing, err := h.roles.FindByID(ctx, rid)
	if err != nil || existing == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if existing.BuiltIn {
		http.Error(w, localized(preferredLang(r), "accounts.roleSystemProtected"), http.StatusConflict)
		return
	}
	if err := h.roles.Delete(ctx, rid); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if h.refreshPolicy != nil {
		_ = h.refreshPolicy(ctx)
	}
	h.audits.Log(r.Context(), currentUser(r), "accounts.role_delete", idStr)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AccountsHandler) ImportUsers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	sess := sessionFromCtx(r)
	if sess == nil || !h.policy.Allowed(sess.Roles, "accounts.manage") {
		http.Error(w, "forbidden", http.StatusForbidden)
		h.audits.Log(ctx, currentUser(r), "accounts.import_forbidden", "permission")
		return
	}
	if h.cfg == nil || !h.cfg.Security.LegacyImportEnabled || strings.ToLower(strings.TrimSpace(h.cfg.AppEnv)) != "dev" {
		http.Error(w, "legacy import disabled", http.StatusGone)
		h.audits.Log(ctx, currentUser(r), "accounts.legacy_import_blocked", "")
		return
	}
	actor, actorRoles, _ := h.users.FindByUsername(ctx, currentUser(r))
	actorEff, _ := h.effectiveAccess(ctx, actor, actorRoles)
	actorPerms := permissionSet(actorEff.Permissions)
	if err := parseMultipartFormLimited(w, r, 10<<20); err != nil {
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file required", http.StatusBadRequest)
		return
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		http.Error(w, "invalid csv", http.StatusBadRequest)
		return
	}
	if len(records) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"imported": 0})
		return
	}
	headers := normalizeHeaders(records[0])
	rows := records[1:]
	forceChange := r.FormValue("force_change") == "1" || strings.ToLower(r.FormValue("force_change")) == "true"
	var imported []string
	for _, row := range rows {
		rowMap := map[string]string{}
		for i, v := range row {
			if i < len(headers) {
				rowMap[headers[i]] = strings.TrimSpace(v)
			}
		}
		username := strings.ToLower(rowMap["username"])
		if username == "" {
			continue
		}
		if err := utils.ValidateUsername(username); err != nil {
			continue
		}
		fullName := rowMap["full_name"]
		dept := rowMap["department"]
		pos := rowMap["position"]
		role := rowMap["role"]
		roleName := strings.ToLower(strings.TrimSpace(role))
		if roleName != "" && !containsRole(sess.Roles, "superadmin") {
			if strings.EqualFold(roleName, "superadmin") {
				http.Error(w, "forbidden", http.StatusForbidden)
				h.audits.Log(ctx, currentUser(r), "accounts.import_forbidden", fmt.Sprintf("%s|%s", username, roleName))
				return
			}
			rolePerms, err := h.rolePermissions(ctx, roleName)
			if err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			if !permsSubset(rolePerms, actorPerms) {
				http.Error(w, "forbidden", http.StatusForbidden)
				h.audits.Log(ctx, currentUser(r), "accounts.import_forbidden", fmt.Sprintf("%s|%s", username, roleName))
				return
			}
		}
		password, _ := utils.RandString(16)
		ph, err := auth.HashPassword(password, h.cfg.Pepper)
		if err != nil {
			continue
		}
		u := &store.User{
			Username:              username,
			FullName:              fullName,
			Department:            dept,
			Position:              pos,
			PasswordHash:          ph.Hash,
			Salt:                  ph.Salt,
			PasswordSet:           true,
			RequirePasswordChange: forceChange,
			Active:                true,
		}
		_, err = h.users.Create(ctx, u, sanitizeRoles(nil, role))
		if err == nil {
			imported = append(imported, username)
		}
	}
	h.audits.Log(r.Context(), currentUser(r), "import_users", fmt.Sprintf("%d", len(imported)))
	writeJSON(w, http.StatusOK, map[string]any{"imported": len(imported), "usernames": imported})
}

func (h *AccountsHandler) Disable(w http.ResponseWriter, r *http.Request) {
	h.setActive(w, r, false, "disable_user")
}

func (h *AccountsHandler) Enable(w http.ResponseWriter, r *http.Request) {
	h.setActive(w, r, true, "enable_user")
}

func (h *AccountsHandler) setActive(w http.ResponseWriter, r *http.Request, active bool, action string) {
	idStr := pathParams(r)["id"]
	id, _ := strconv.ParseInt(idStr, 10, 64)
	existing, existingRoles, err := h.users.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if existing == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if isAdminUsername(existing.Username) && !active {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if !active && containsRole(existingRoles, "superadmin") && h.isLastSuperadmin(r.Context(), id) {
		http.Error(w, localized(preferredLang(r), "accounts.lastSuperadminProtected"), http.StatusConflict)
		return
	}
	if err := h.users.SetActive(r.Context(), id, active); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.sessions.DeleteAllForUser(r.Context(), id, currentUser(r))
	h.audits.Log(r.Context(), currentUser(r), "session.kill_all", fmt.Sprintf("%d|security_change", id))
	h.audits.Log(r.Context(), currentUser(r), action, idStr)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AccountsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := pathParams(r)["id"]
	id, _ := strconv.ParseInt(idStr, 10, 64)
	existing, roles, err := h.users.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if existing == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if isAdminUsername(existing.Username) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if containsRole(roles, "superadmin") && h.isLastSuperadmin(r.Context(), id) {
		http.Error(w, localized(preferredLang(r), "accounts.lastSuperadminProtected"), http.StatusConflict)
		return
	}
	if err := h.users.Delete(r.Context(), id); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.sessions.DeleteAllForUser(r.Context(), id, currentUser(r))
	h.audits.Log(r.Context(), currentUser(r), "delete_user", idStr)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AccountsHandler) Copy(w http.ResponseWriter, r *http.Request) {
	idStr := pathParams(r)["id"]
	id, _ := strconv.ParseInt(idStr, 10, 64)
	u, roles, err := h.users.Get(r.Context(), id)
	if err != nil || u == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if h.denyAdminChange(w, r, u, true) {
		return
	}
	newUsername, err := h.nextUsername(r.Context(), u.Username)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	copyUser := &store.User{
		Username:    newUsername,
		Email:       u.Email,
		FullName:    fmt.Sprintf("копия %s", u.FullName),
		Department:  u.Department,
		Position:    u.Position,
		PasswordSet: false,
		Active:      true,
	}
	tempPwd, _ := utils.RandString(16)
	ph, err := auth.HashPassword(tempPwd, h.cfg.Pepper)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	copyUser.PasswordHash = ph.Hash
	copyUser.Salt = ph.Salt
	newID, err := h.users.Create(r.Context(), copyUser, roles)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.audits.Log(r.Context(), currentUser(r), "copy_user", idStr)
	writeJSON(w, http.StatusOK, map[string]any{"id": newID, "username": newUsername})
}

func (h *AccountsHandler) nextUsername(ctx context.Context, base string) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "user"
	}
	for i := 1; i < 50; i++ {
		candidate := fmt.Sprintf("%s_copy%d", base, i)
		exists, _, err := h.users.FindByUsername(ctx, candidate)
		if err != nil {
			return "", err
		}
		if exists == nil {
			return candidate, nil
		}
	}
	return fmt.Sprintf("%s_copy_%d", base, time.Now().Unix()), nil
}

func hasAdminUser(users []store.UserWithRoles) bool {
	for _, u := range users {
		if isAdminUsername(u.Username) {
			return true
		}
	}
	return false
}

func (h *AccountsHandler) ensureAdminPresent(ctx context.Context, users []store.UserWithRoles) []store.UserWithRoles {
	if hasAdminUser(users) {
		return users
	}
	admin, roles, err := h.users.FindByUsername(ctx, "admin")
	if err != nil {
		if h.logger != nil {
			h.logger.Errorf("ensure admin present (find): %v", err)
		}
		return users
	}
	if admin == nil {
		return users
	}
	if h.logger != nil {
		h.logger.Printf("injecting admin into accounts list (not returned by list)")
	}
	return append([]store.UserWithRoles{{User: *admin, Roles: roles}}, users...)
}

func currentUser(r *http.Request) string {
	if sr := r.Context().Value(auth.SessionContextKey); sr != nil {
		return sr.(*store.SessionRecord).Username
	}
	return ""
}

func sessionFromCtx(r *http.Request) *store.SessionRecord {
	if sr := r.Context().Value(auth.SessionContextKey); sr != nil {
		if rec, ok := sr.(*store.SessionRecord); ok {
			return rec
		}
	}
	return nil
}

func isAdminUsername(username string) bool {
	return strings.EqualFold(username, "admin")
}

func (h *AccountsHandler) denyAdminChange(w http.ResponseWriter, r *http.Request, target *store.User, allowAdminActor bool) bool {
	if !isAdminUsername(target.Username) {
		return false
	}
	if allowAdminActor && isAdminUsername(currentUser(r)) {
		return false
	}
	http.Error(w, "forbidden", http.StatusForbidden)
	return true
}

func sanitizeTags(tags []string) []string {
	set := map[string]struct{}{}
	for _, t := range tags {
		val := strings.TrimSpace(t)
		if val == "" {
			continue
		}
		set[strings.ToLower(val)] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func decodePayload(data json.RawMessage, out interface{}) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

func generateStrongPassword() string {
	for i := 0; i < 5; i++ {
		if pwd, err := utils.RandString(16); err == nil && utils.ValidatePassword(pwd) == nil {
			return pwd
		}
	}
	fallback, _ := utils.RandString(16)
	candidate := "Aa1!" + fallback
	if utils.ValidatePassword(candidate) == nil {
		return candidate
	}
	return candidate + "!"
}

func uniqueIDs(ids []int64) []int64 {
	set := map[int64]struct{}{}
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		set[id] = struct{}{}
	}
	out := make([]int64, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	return out
}

func containsRole(roles []string, target string) bool {
	for _, r := range roles {
		if strings.EqualFold(r, target) {
			return true
		}
	}
	return false
}

func permissionSet(perms []string) map[string]struct{} {
	set := make(map[string]struct{}, len(perms))
	for _, p := range perms {
		p = strings.ToLower(strings.TrimSpace(p))
		if p != "" {
			set[p] = struct{}{}
		}
	}
	return set
}

func permsSubset(perms []string, allowed map[string]struct{}) bool {
	for _, p := range perms {
		val := strings.ToLower(strings.TrimSpace(p))
		if val == "" {
			continue
		}
		if _, ok := allowed[val]; !ok {
			return false
		}
	}
	return true
}

func (h *AccountsHandler) rolePermissions(ctx context.Context, roleName string) ([]string, error) {
	roleName = strings.ToLower(strings.TrimSpace(roleName))
	if roleName == "" {
		return nil, nil
	}
	if h.roles != nil {
		role, err := h.roles.FindByName(ctx, roleName)
		if err != nil {
			return nil, err
		}
		if role != nil && len(role.Permissions) > 0 {
			return role.Permissions, nil
		}
	}
	if h.policy == nil {
		return nil, nil
	}
	perms := h.policy.PermissionsForRoles([]string{roleName})
	out := make([]string, 0, len(perms))
	for _, p := range perms {
		out = append(out, string(p))
	}
	return out, nil
}

func equalStringSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := map[string]struct{}{}
	for _, v := range a {
		set[strings.ToLower(v)] = struct{}{}
	}
	for _, v := range b {
		if _, ok := set[strings.ToLower(v)]; !ok {
			return false
		}
	}
	return true
}

func diffInt64Sets(oldVals, newVals []int64) (added, removed []int64) {
	oldSet := map[int64]struct{}{}
	for _, v := range oldVals {
		oldSet[v] = struct{}{}
	}
	newSet := map[int64]struct{}{}
	for _, v := range newVals {
		newSet[v] = struct{}{}
		if _, ok := oldSet[v]; !ok {
			added = append(added, v)
		}
	}
	for v := range oldSet {
		if _, ok := newSet[v]; !ok {
			removed = append(removed, v)
		}
	}
	return
}

func (h *AccountsHandler) userReliesOnGroupForRole(ctx context.Context, userID, groupID int64, role string) bool {
	direct, _ := h.users.UserDirectRoles(ctx, userID)
	if containsRole(direct, role) {
		return false
	}
	groups, _ := h.users.UserGroups(ctx, userID)
	for _, g := range groups {
		if g.ID == groupID {
			continue
		}
		if containsRole(g.Roles, role) {
			return false
		}
	}
	return true
}

func normalizeHeaders(headers []string) []string {
	out := make([]string, len(headers))
	for i, h := range headers {
		out[i] = strings.ToLower(strings.TrimSpace(h))
	}
	return out
}

func (h *AccountsHandler) isLastSuperadmin(ctx context.Context, targetID int64) bool {
	users, err := h.users.List(ctx)
	if err != nil {
		return false
	}
	count := 0
	for _, u := range users {
		if containsRole(u.Roles, "superadmin") {
			count++
			if count > 1 {
				return false
			}
		}
	}
	if count != 1 {
		return false
	}
	for _, u := range users {
		if u.ID == targetID && containsRole(u.Roles, "superadmin") {
			return true
		}
	}
	return false
}

func (h *AccountsHandler) canAssignTags(actor *store.User, tags []string) bool {
	if actor == nil {
		return len(tags) == 0 || !h.cfg.Security.TagsSubsetEnforced
	}
	if !h.cfg.Security.TagsSubsetEnforced {
		return true
	}
	set := map[string]struct{}{}
	for _, t := range actor.ClearanceTags {
		set[strings.ToLower(t)] = struct{}{}
	}
	for _, t := range tags {
		if _, ok := set[strings.ToLower(t)]; !ok {
			return false
		}
	}
	return true
}

func (h *AccountsHandler) effectiveAccess(ctx context.Context, user *store.User, roles []string) (store.EffectiveAccess, []store.Group) {
	if user == nil || h.users == nil {
		return store.EffectiveAccess{}, nil
	}
	groups, _ := h.users.UserGroups(ctx, user.ID)
	eff := auth.CalculateEffectiveAccess(user, roles, groups, h.policy)
	return eff, groups
}

func (h *AccountsHandler) validateGroupAssignments(ctx context.Context, actorEff store.EffectiveAccess, groupIDs []int64) error {
	if h.groups == nil || len(groupIDs) == 0 {
		return nil
	}
	seen := map[int64]struct{}{}
	for _, gid := range groupIDs {
		if gid <= 0 {
			continue
		}
		if _, ok := seen[gid]; ok {
			continue
		}
		seen[gid] = struct{}{}
		g, _, _, err := h.groups.Get(ctx, gid)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("group not found")
			}
			return err
		}
		if g == nil {
			return fmt.Errorf("group not found")
		}
		if g.ClearanceLevel > actorEff.ClearanceLevel {
			return errors.New("clearance_too_high")
		}
		actorTags := map[string]struct{}{}
		for _, t := range actorEff.ClearanceTags {
			actorTags[strings.ToLower(t)] = struct{}{}
		}
		if h.cfg.Security.TagsSubsetEnforced {
			for _, t := range g.ClearanceTags {
				if _, ok := actorTags[strings.ToLower(t)]; !ok {
					return errors.New("clearance_tags_not_allowed")
				}
			}
		}
	}
	return nil
}

func (h *AccountsHandler) groupIDsToGroups(ctx context.Context, ids []int64) ([]store.Group, error) {
	if h.groups == nil {
		return nil, nil
	}
	var res []store.Group
	seen := map[int64]struct{}{}
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		g, _, _, err := h.groups.Get(ctx, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("group not found")
			}
			return nil, err
		}
		if g != nil {
			res = append(res, *g)
		}
	}
	return res, nil
}

type roleTemplate struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

func defaultRoleTemplates() []roleTemplate {
	return []roleTemplate{
		{ID: "doc_viewer", Name: "Doc Viewer", Description: "View documents and versions", Permissions: []string{"docs.view", "docs.versions.view"}},
		{ID: "doc_editor", Name: "Doc Editor", Description: "Edit documents and upload files", Permissions: []string{"docs.view", "docs.create", "docs.edit", "docs.upload", "docs.versions.view"}},
		{ID: "doc_admin", Name: "Doc Admin", Description: "Manage documents, approvals and templates", Permissions: []string{"docs.manage", "docs.classification.set", "docs.approval.start", "docs.approval.view", "docs.approval.approve", "folders.manage", "templates.manage"}},
		{ID: "auditor", Name: "Auditor", Description: "Read-only access with audit visibility", Permissions: []string{"docs.view", "logs.view", "accounts.view_dashboard"}},
		{ID: "security_officer", Name: "Security Officer", Description: "Security oversight for docs", Permissions: []string{"docs.classification.set", "logs.view"}},
	}
}

func findTemplate(id string) *roleTemplate {
	for _, tpl := range defaultRoleTemplates() {
		if tpl.ID == id {
			return &tpl
		}
	}
	return nil
}
