package handlers

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
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

type AuthHandler struct {
	cfg            *config.AppConfig
	users          store.UsersStore
	sessions       store.SessionStore
	incidents      store.IncidentsStore
	sessionManager *auth.SessionManager
	policy         *rbac.Policy
	audits         store.AuditStore
	logger         *utils.Logger
}

func NewAuthHandler(cfg *config.AppConfig, users store.UsersStore, sessions store.SessionStore, incidents store.IncidentsStore, sm *auth.SessionManager, policy *rbac.Policy, audits store.AuditStore, logger *utils.Logger) *AuthHandler {
	return &AuthHandler{cfg: cfg, users: users, sessions: sessions, incidents: incidents, sessionManager: sm, policy: policy, audits: audits, logger: logger}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	// Safety net: always ensure default admin exists before processing logins.
	if err := bootstrap.EnsureDefaultAdminWithStore(r.Context(), h.users, h.cfg, h.logger); err != nil && h.logger != nil {
		h.logger.Errorf("ensure default admin: %v", err)
	}
	lang := preferredLang(r)
	var cred auth.Credentials
	if err := json.NewDecoder(r.Body).Decode(&cred); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	cred.Username = strings.ToLower(strings.TrimSpace(cred.Username))
	if err := utils.ValidateUsername(cred.Username); err != nil {
		http.Error(w, "invalid username", http.StatusBadRequest)
		return
	}
	user, roles, err := h.users.FindByUsername(r.Context(), cred.Username)
	if err != nil || user == nil || !user.Active {
		h.audits.Log(r.Context(), cred.Username, "auth.login_failed", "user missing or inactive")
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	now := time.Now().UTC()
	if isPermanentLock(user) {
		h.audits.Log(r.Context(), cred.Username, "auth.login_blocked", "permanent")
		http.Error(w, localized(lang, "auth.lockedPermanent"), http.StatusForbidden)
		return
	}
	if user.LockedUntil != nil && now.Before(*user.LockedUntil) {
		msg := localizedUntil(lang, "auth.lockedUntil", *user.LockedUntil)
		h.audits.Log(r.Context(), cred.Username, "auth.login_blocked", msg)
		http.Error(w, msg, http.StatusForbidden)
		return
	}
	singleAttempt := user.LockStage >= 1
	if user.LockedUntil != nil && now.After(*user.LockedUntil) {
		user.LockedUntil = nil
		user.FailedAttempts = 0
	}
	ph, _ := auth.ParsePasswordHash(user.PasswordHash, user.Salt)
	ok, err := auth.VerifyPassword(cred.Password, h.cfg.Pepper, ph)
	if err != nil || !ok {
		user.LastFailedAt = &now
		if user.LockStage == 0 && !singleAttempt {
			user.FailedAttempts++
			if user.FailedAttempts >= 5 {
				applyLockout(user, 1, now, "auto")
				h.audits.Log(r.Context(), cred.Username, "auth.lockout", "stage=1 dur=1h")
				h.ensureAuthLockoutIncident(r.Context(), user, 1, now)
				_ = h.users.Update(r.Context(), user, nil)
				msg := localizedUntil(lang, "auth.lockedUntil", *user.LockedUntil)
				http.Error(w, msg, http.StatusForbidden)
				return
			}
			if user.FailedAttempts == 4 {
				_ = h.users.Update(r.Context(), user, nil)
				http.Error(w, localized(lang, "auth.lockoutSoon"), http.StatusUnauthorized)
				return
			}
			_ = h.users.Update(r.Context(), user, nil)
		} else {
			nextStage := user.LockStage + 1
			if nextStage > 6 {
				nextStage = 6
			}
			applyLockout(user, nextStage, now, "auto")
			h.audits.Log(r.Context(), cred.Username, "auth.lockout", "stage="+strconv.Itoa(nextStage))
			h.ensureAuthLockoutIncident(r.Context(), user, nextStage, now)
			_ = h.users.Update(r.Context(), user, nil)
			if isPermanentLock(user) {
				http.Error(w, localized(lang, "auth.lockedPermanent"), http.StatusForbidden)
				return
			}
			msg := localizedUntil(lang, "auth.lockedUntil", *user.LockedUntil)
			http.Error(w, msg, http.StatusForbidden)
			return
		}
		h.audits.Log(r.Context(), cred.Username, "auth.login_failed", "invalid password")
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	sess, err := h.sessionManager.Create(r.Context(), user, roles, clientIP(r, h.cfg), r.UserAgent())
	if err != nil {
		if h.logger != nil {
			h.logger.Errorf("auth login session create failed for %s: %v", cred.Username, err)
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	user.LastLoginAt = &now
	user.FailedAttempts = 0
	user.LockedUntil = nil
	user.LockReason = ""
	user.LockStage = 0
	user.LastFailedAt = nil
	_ = h.users.Update(r.Context(), user, nil)
	h.resolveAuthLockoutIncident(r.Context(), user, now)
	h.audits.Log(r.Context(), user.Username, "auth.login_success", "")
	cookieSecure := isSecureRequest(r, h.cfg)
	cookie := http.Cookie{
		Name:     SessionCookieName,
		Value:    sess.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   cookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  sess.ExpiresAt,
	}
	http.SetCookie(w, &cookie)
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    sess.CSRFToken,
		Path:     "/",
		HttpOnly: false,
		Secure:   cookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  sess.ExpiresAt,
	})
	groups, _ := h.users.UserGroups(r.Context(), user.ID)
	eff := auth.CalculateEffectiveAccess(user, roles, groups, h.policy)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": auth.UserDTO{
			ID:                    user.ID,
			Username:              user.Username,
			Roles:                 roles,
			Active:                user.Active,
			PasswordSet:           user.PasswordSet,
			RequirePasswordChange: user.RequirePasswordChange,
			PasswordChangedAt:     user.PasswordChangedAt,
			Permissions:           eff.Permissions,
			MenuPermissions:       eff.MenuPermissions,
		},
		"csrf_token": sess.CSRFToken,
		"session":    sess,
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(SessionCookieName)
	actor := ""
	if ctxSess := r.Context().Value(auth.SessionContextKey); ctxSess != nil {
		actor = ctxSess.(*store.SessionRecord).Username
	}
	if err == nil && cookie.Value != "" {
		_ = h.sessions.DeleteSession(r.Context(), cookie.Value, actor)
	}
	cookieSecure := isSecureRequest(r, h.cfg)
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	h.audits.Log(r.Context(), actor, "auth.logout", "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AuthHandler) Ping(w http.ResponseWriter, r *http.Request) {
	sr := r.Context().Value(auth.SessionContextKey).(*store.SessionRecord)
	now := time.Now().UTC()
	_ = h.sessions.UpdateActivity(r.Context(), sr.ID, now, h.cfg.EffectiveSessionTTL())
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "last_seen_at": now})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	sr := r.Context().Value(auth.SessionContextKey).(*store.SessionRecord)
	user, roles, err := h.users.FindByUsername(r.Context(), sr.Username)
	if err != nil || user == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	groups, _ := h.users.UserGroups(r.Context(), user.ID)
	eff := auth.CalculateEffectiveAccess(user, roles, groups, h.policy)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": auth.UserDTO{
			ID:                    user.ID,
			Username:              user.Username,
			Roles:                 roles,
			Active:                user.Active,
			PasswordSet:           user.PasswordSet,
			RequirePasswordChange: user.RequirePasswordChange,
			PasswordChangedAt:     user.PasswordChangedAt,
			Permissions:           eff.Permissions,
			MenuPermissions:       eff.MenuPermissions,
		},
		"csrf_token": sr.CSRFToken,
	})
}

func (h *AuthHandler) Menu(w http.ResponseWriter, r *http.Request) {
	sr := r.Context().Value(auth.SessionContextKey).(*store.SessionRecord)
	user, roles, err := h.users.FindByUsername(r.Context(), sr.Username)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	groups, _ := h.users.UserGroups(r.Context(), user.ID)
	eff := auth.CalculateEffectiveAccess(user, roles, groups, h.policy)
	menu := buildMenu(eff)
	writeJSON(w, http.StatusOK, map[string]interface{}{"menu": menu})
}

func buildMenu(eff store.EffectiveAccess) []map[string]string {
	entries := []struct {
		Perm rbac.Permission
		Name string
		Path string
	}{
		{Perm: "dashboard.view", Name: "dashboard", Path: "dashboard"},
		{Perm: "monitoring.view", Name: "monitoring", Path: "monitoring"},
		{Perm: "controls.view", Name: "controls", Path: "controls"},
		{Perm: "tasks.view", Name: "tasks", Path: "tasks"},
		{Perm: "incidents.view", Name: "incidents", Path: "incidents"},
		{Perm: "reports.view", Name: "reports", Path: "reports"},
		{Perm: "backups.read", Name: "backups", Path: "backups"},
		{Perm: "docs.view", Name: "docs", Path: "docs"},
		{Perm: "docs.approval.view", Name: "approvals", Path: "approvals"},
		{Perm: "accounts.view", Name: "accounts", Path: "accounts"},
		{Perm: "logs.view", Name: "logs", Path: "logs"},
		{Perm: "app.view", Name: "settings", Path: "settings"},
	}
	var menu []map[string]string
	allowed := map[string]struct{}{}
	for _, p := range eff.Permissions {
		allowed[p] = struct{}{}
	}
	menuAllowed := map[string]struct{}{}
	for _, m := range eff.MenuPermissions {
		menuAllowed[m] = struct{}{}
	}
	for _, e := range entries {
		_, ok := allowed[string(e.Perm)]
		if ok {
			if len(menuAllowed) > 0 {
				if _, ok := menuAllowed[e.Name]; !ok {
					if _, ok2 := menuAllowed[e.Path]; !ok2 {
						continue
					}
				}
			}
			menu = append(menu, map[string]string{"name": e.Name, "path": e.Path})
		}
	}
	return menu
}

func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	sr := r.Context().Value(auth.SessionContextKey).(*store.SessionRecord)
	var payload struct {
		Current  string `json:"current_password"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	user, _, err := h.users.Get(r.Context(), sr.UserID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if user == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err := utils.ValidatePassword(payload.Password); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if user.PasswordSet {
		phCurrent, _ := auth.ParsePasswordHash(user.PasswordHash, user.Salt)
		ok, _ := auth.VerifyPassword(payload.Current, h.cfg.Pepper, phCurrent)
		if !ok {
			http.Error(w, localized(preferredLang(r), "accounts.currentPasswordInvalid"), http.StatusBadRequest)
			return
		}
	}
	history, _ := h.users.PasswordHistory(r.Context(), sr.UserID, 10)
	if isPasswordReused(payload.Password, h.cfg.Pepper, user, history) {
		h.audits.Log(r.Context(), sr.Username, "auth.password_reuse_denied", "")
		http.Error(w, localized(preferredLang(r), "accounts.passwordReuseDenied"), http.StatusBadRequest)
		return
	}
	ph, err := auth.HashPassword(payload.Password, h.cfg.Pepper)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if err := h.users.UpdatePassword(r.Context(), sr.UserID, ph.Hash, ph.Salt, false); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.audits.Log(r.Context(), sr.Username, "auth.password_changed", "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func ServeStatic(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := gui.StaticFiles.ReadFile("static/" + name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, name, time.Now(), bytes.NewReader(data))
	}
}

func RedirectToApp(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

func (h *AuthHandler) PasswordChangePage(w http.ResponseWriter, r *http.Request) {
	sr := r.Context().Value(auth.SessionContextKey).(*store.SessionRecord)
	user, _, err := h.users.FindByUsername(r.Context(), sr.Username)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if user.PasswordSet && !user.RequirePasswordChange {
		http.Redirect(w, r, "/dashboard", http.StatusFound)
		return
	}
	data, err := gui.StaticFiles.ReadFile("static/password.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, "password.html", time.Now(), bytes.NewReader(data))
}

func lockDuration(stage int) time.Duration {
	switch stage {
	case 1:
		return time.Hour
	case 2:
		return 3 * time.Hour
	case 3:
		return 6 * time.Hour
	case 4:
		return 12 * time.Hour
	case 5:
		return 24 * time.Hour
	default:
		return 0
	}
}

func applyLockout(user *store.User, stage int, now time.Time, reason string) {
	user.LockStage = stage
	user.FailedAttempts = 0
	if stage >= 6 {
		user.LockedUntil = nil
		user.LockReason = reason
		return
	}
	dur := lockDuration(stage)
	if dur <= 0 {
		return
	}
	until := now.Add(dur)
	user.LockedUntil = &until
	user.LockReason = reason
}

func isPermanentLock(user *store.User) bool {
	return user != nil && user.LockStage >= 6 && (user.LockedUntil == nil || time.Now().UTC().Before(*user.LockedUntil))
}

func preferredLang(r *http.Request) string {
	al := r.Header.Get("Accept-Language")
	if strings.HasPrefix(strings.ToLower(al), "ru") {
		return "ru"
	}
	return "en"
}

func localized(lang, key string) string {
	ru := map[string]string{
		"auth.lockoutSoon":                 "Ваша учетная запись будет заблокирована на 1 час.",
		"auth.lockedPermanent":             "Аккаунт заблокирован. Обратитесь к администратору.",
		"accounts.passwordReuseDenied":     "Пароль уже использовался. Выберите другой.",
		"accounts.currentPasswordInvalid":  "Текущий пароль неверен",
		"accounts.clearanceTooHigh":        "Нельзя выдать допуск выше своего",
		"accounts.clearanceTagsNotAllowed": "Нельзя назначить эти теги допуска",
		"accounts.lastSuperadminProtected": "Нельзя изменить последнего супер-админа",
		"accounts.selfLockoutPrevented":    "Операция запрещена: привела бы к потере доступа",
		"accounts.roleSystemProtected":     "Системную роль нельзя изменить или удалить",
		"errors.roleTemplateNotFound":      "Шаблон роли не найден",
	}
	en := map[string]string{
		"auth.lockoutSoon":                 "Your account will be locked for 1 hour.",
		"auth.lockedPermanent":             "Account is locked. Contact administrator.",
		"accounts.passwordReuseDenied":     "Password was used recently. Choose a new one.",
		"accounts.currentPasswordInvalid":  "Current password is invalid",
		"accounts.clearanceTooHigh":        "Clearance level exceeds your own",
		"accounts.clearanceTagsNotAllowed": "Clearance tags are not allowed",
		"accounts.lastSuperadminProtected": "Cannot modify the last superadmin",
		"accounts.selfLockoutPrevented":    "Operation blocked to avoid self-lockout",
		"accounts.roleSystemProtected":     "System role cannot be modified or deleted",
		"errors.roleTemplateNotFound":      "Role template not found",
	}
	ru["accounts.groupSystemProtected"] = "Системную группу нельзя изменить или удалить"
	en["accounts.groupSystemProtected"] = "System group cannot be modified or deleted"
	ru["reports.error.chartNotFound"] = "График не найден"
	ru["reports.error.snapshotRequired"] = "Нужен снапшот"
	ru["reports.error.exportChartsUnavailable"] = "Для экспорта графиков нужен локальный конвертер"
	en["reports.error.chartNotFound"] = "Chart not found"
	en["reports.error.snapshotRequired"] = "Snapshot required"
	en["reports.error.exportChartsUnavailable"] = "Chart export requires a local converter"
	ru["docs.onlyoffice.disabled"] = "OnlyOffice отключен"
	ru["docs.onlyoffice.unsupportedFormat"] = "OnlyOffice поддерживает только DOCX"
	ru["docs.onlyoffice.invalidToken"] = "Недействительный токен OnlyOffice"
	ru["docs.onlyoffice.misconfigured"] = "OnlyOffice настроен некорректно"
	ru["docs.onlyoffice.saveReason"] = "Редактирование в OnlyOffice"
	ru["docs.onlyoffice.forceSaveFailed"] = "Не удалось выполнить сохранение в OnlyOffice"
	ru["docs.onlyoffice.forceSaveNoVersion"] = "Сохранение запрошено, но новая версия документа не была создана"
	en["docs.onlyoffice.disabled"] = "OnlyOffice is disabled"
	en["docs.onlyoffice.unsupportedFormat"] = "Only DOCX is supported for OnlyOffice editing"
	en["docs.onlyoffice.invalidToken"] = "Invalid OnlyOffice token"
	en["docs.onlyoffice.misconfigured"] = "OnlyOffice is misconfigured"
	en["docs.onlyoffice.saveReason"] = "Edited in OnlyOffice"
	en["docs.onlyoffice.forceSaveFailed"] = "OnlyOffice save failed"
	en["docs.onlyoffice.forceSaveNoVersion"] = "Save was requested, but a new document version was not created"
	if lang == "ru" {
		if v, ok := ru[key]; ok {
			return v
		}
	}
	if v, ok := en[key]; ok {
		return v
	}
	return key
}

func localizedUntil(lang, key string, until time.Time) string {
	format := "2006-01-02 15:04"
	if lang == "ru" {
		return "Аккаунт заблокирован до " + until.Format(format)
	}
	return "Account locked until " + until.Format(format)
}

func clientIP(r *http.Request, cfg *config.AppConfig) string {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip == "" {
		ip = r.RemoteAddr
	}
	ip = strings.TrimSpace(ip)
	if cfg == nil || !isTrustedProxy(ip, cfg.Security.TrustedProxies) {
		return ip
	}
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		if candidate := extractClientIPFromXFF(xff, cfg.Security.TrustedProxies); candidate != "" {
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

func isSecureRequest(r *http.Request, cfg *config.AppConfig) bool {
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
