package handlers

import (
	"bytes"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"berkut-scc/core/docs"
	"berkut-scc/core/store"
	"berkut-scc/core/utils"
)

const (
	onlyOfficeTokenTTL       = 10 * time.Minute
	onlyOfficeUserDataPrefix = "berkut:"
)

type onlyOfficeCallbackPayload struct {
	Key           string   `json:"key"`
	Status        int      `json:"status"`
	URL           string   `json:"url"`
	Users         []string `json:"users"`
	Error         int      `json:"error"`
	UserData      string   `json:"userdata"`
	ForceSaveType int      `json:"forcesavetype"`
}

type onlyOfficePendingSave struct {
	DocID     int64
	Version   int
	Username  string
	Reason    string
	ExpiresAt time.Time
}

func (h *DocsHandler) OnlyOfficeConfig(w http.ResponseWriter, r *http.Request) {
	lang := preferredLang(r)
	if !h.cfg.Docs.OnlyOffice.Enabled {
		http.Error(w, localized(lang, "docs.onlyoffice.disabled"), http.StatusBadRequest)
		return
	}
	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	if mode != "edit" {
		mode = "view"
	}
	required := "view"
	if mode == "edit" {
		required = "edit"
	}
	doc, user, ok := h.loadDocForAccess(w, r, required)
	if !ok {
		return
	}
	ver, err := h.store.GetVersion(r.Context(), doc.ID, doc.CurrentVersion)
	if err != nil || ver == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if strings.ToLower(strings.TrimSpace(ver.Format)) != docs.FormatDocx {
		http.Error(w, localized(lang, "docs.onlyoffice.unsupportedFormat"), http.StatusBadRequest)
		return
	}
	baseURL := strings.TrimRight(strings.TrimSpace(h.cfg.Docs.OnlyOffice.AppInternalURL), "/")
	if baseURL == "" {
		http.Error(w, localized(lang, "docs.onlyoffice.misconfigured"), http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	fileToken, err := signOnlyOfficeLinkToken(h.cfg.Docs.OnlyOffice.JWTSecret, onlyOfficeLinkToken{
		Purpose:  "file",
		DocID:    doc.ID,
		Version:  ver.Version,
		Username: user.Username,
		Iat:      now.Unix(),
		Exp:      now.Add(onlyOfficeTokenTTL).Unix(),
	})
	if err != nil {
		http.Error(w, localized(lang, "docs.onlyoffice.misconfigured"), http.StatusInternalServerError)
		return
	}
	callbackToken, err := signOnlyOfficeLinkToken(h.cfg.Docs.OnlyOffice.JWTSecret, onlyOfficeLinkToken{
		Purpose:  "callback",
		DocID:    doc.ID,
		Version:  ver.Version,
		Username: user.Username,
		Iat:      now.Unix(),
		Exp:      now.Add(onlyOfficeTokenTTL).Unix(),
	})
	if err != nil {
		http.Error(w, localized(lang, "docs.onlyoffice.misconfigured"), http.StatusInternalServerError)
		return
	}

	fileURL := fmt.Sprintf("%s/api/docs/%d/office/file?token=%s", baseURL, doc.ID, fileToken)
	callbackURL := fmt.Sprintf("%s/api/docs/%d/office/callback?token=%s", baseURL, doc.ID, callbackToken)
	title := strings.TrimSpace(doc.Title)
	if title == "" {
		title = doc.RegNumber
	}
	if title == "" {
		title = fmt.Sprintf("doc-%d", doc.ID)
	}
	title = title + ".docx"

	baseKey := buildOnlyOfficeKey(doc.ID, ver.Version)
	sessionKey := baseKey + "-s" + strconv.FormatInt(time.Now().UTC().UnixNano(), 36)
	if b, genErr := utils.RandBytes(6); genErr == nil {
		sessionKey = baseKey + "-s" + strings.ToLower(hex.EncodeToString(b))
	}
	document := map[string]any{
		"fileType": "docx",
		"key":      sessionKey,
		"title":    title,
		"url":      fileURL,
		"permissions": map[string]bool{
			"edit":      mode == "edit",
			"download":  false,
			"print":     false,
			"comment":   mode == "edit",
			"fillForms": true,
			"copy":      false,
		},
	}
	editorConfig := map[string]any{
		"callbackUrl": callbackURL,
		"mode":        mode,
		"lang":        lang,
		"customization": map[string]any{
			// Transport sync must be enabled so modal force-save can capture current edits.
			// Version persistence is still gated by SCC explicit save token in callback userdata.
			"autosave":  true,
			"forcesave": false,
		},
		"user": map[string]string{
			"id":   strconv.FormatInt(user.ID, 10),
			"name": user.Username,
		},
	}
	cfg := map[string]any{
		"documentType": "word",
		"type":         "desktop",
		"document":     document,
		"editorConfig": editorConfig,
	}
	if secret := strings.TrimSpace(h.cfg.Docs.OnlyOffice.JWTSecret); secret != "" {
		claims := map[string]any{
			"iat":          now.Unix(),
			"exp":          now.Add(time.Hour).Unix(),
			"iss":          h.cfg.Docs.OnlyOffice.JWTIssuer,
			"aud":          h.cfg.Docs.OnlyOffice.JWTAudience,
			"document":     document,
			"editorConfig": editorConfig,
		}
		token, err := buildOnlyOfficeJWT(secret, claims)
		if err != nil {
			http.Error(w, localized(lang, "docs.onlyoffice.misconfigured"), http.StatusInternalServerError)
			return
		}
		cfg["token"] = token
	}

	h.svc.Log(r.Context(), user.Username, "doc.onlyoffice.config", doc.RegNumber)
	writeJSON(w, http.StatusOK, map[string]any{
		"document_server_url": strings.TrimSpace(h.cfg.Docs.OnlyOffice.PublicURL),
		"config":              cfg,
		"mode":                mode,
		"expires_at":          now.Add(onlyOfficeTokenTTL).Format(time.RFC3339),
	})
}

func (h *DocsHandler) OnlyOfficeForceSave(w http.ResponseWriter, r *http.Request) {
	lang := preferredLang(r)
	if !h.cfg.Docs.OnlyOffice.Enabled {
		http.Error(w, localized(lang, "docs.onlyoffice.disabled"), http.StatusBadRequest)
		return
	}
	doc, user, ok := h.loadDocForAccess(w, r, "edit")
	if !ok {
		return
	}
	ver, err := h.store.GetVersion(r.Context(), doc.ID, doc.CurrentVersion)
	if err != nil || ver == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if strings.ToLower(strings.TrimSpace(ver.Format)) != docs.FormatDocx {
		http.Error(w, localized(lang, "docs.onlyoffice.unsupportedFormat"), http.StatusBadRequest)
		return
	}
	var req struct {
		Reason string `json:"reason"`
		Key    string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, localized(lang, "docs.onlyoffice.forceSaveFailed"), http.StatusBadRequest)
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		http.Error(w, localized(lang, "editor.reasonRequired"), http.StatusBadRequest)
		return
	}
	if len(reason) > 512 {
		reason = reason[:512]
	}
	docKey := strings.TrimSpace(req.Key)
	if docKey == "" {
		docKey = buildOnlyOfficeKey(doc.ID, ver.Version)
	}
	if !strings.HasPrefix(docKey, fmt.Sprintf("doc-%d-v", doc.ID)) {
		http.Error(w, localized(lang, "docs.onlyoffice.invalidToken"), http.StatusForbidden)
		return
	}
	saveToken, err := utils.RandString(18)
	if err != nil {
		http.Error(w, localized(lang, "docs.onlyoffice.forceSaveFailed"), http.StatusInternalServerError)
		return
	}
	h.putOnlyOfficePendingSave(saveToken, onlyOfficePendingSave{
		DocID:     doc.ID,
		Version:   ver.Version,
		Username:  user.Username,
		Reason:    reason,
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	})
	if err := h.sendOnlyOfficeForceSave(r, docKey, onlyOfficeUserDataPrefix+saveToken); err != nil {
		if h.logger != nil {
			h.logger.Errorf("onlyoffice forcesave failed for doc %d: %v", doc.ID, err)
		}
		h.svc.Log(r.Context(), user.Username, "doc.onlyoffice.callback_error", fmt.Sprintf("%d|forcesave_failed", doc.ID))
		http.Error(w, localized(lang, "docs.onlyoffice.forceSaveFailed"), http.StatusBadGateway)
		return
	}
	h.svc.Log(r.Context(), user.Username, "doc.onlyoffice.forcesave", fmt.Sprintf("%d|v%d", doc.ID, ver.Version))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *DocsHandler) OnlyOfficeFile(w http.ResponseWriter, r *http.Request) {
	lang := preferredLang(r)
	if !h.cfg.Docs.OnlyOffice.Enabled {
		http.Error(w, localized(lang, "docs.onlyoffice.disabled"), http.StatusBadRequest)
		return
	}
	id, _ := strconv.ParseInt(pathParams(r)["id"], 10, 64)
	claims, err := parseOnlyOfficeLinkToken(h.cfg.Docs.OnlyOffice.JWTSecret, r.URL.Query().Get("token"), time.Now())
	if err != nil || claims.Purpose != "file" || claims.DocID != id {
		http.Error(w, localized(lang, "docs.onlyoffice.invalidToken"), http.StatusForbidden)
		return
	}
	ver, err := h.store.GetVersion(r.Context(), id, claims.Version)
	if err != nil || ver == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if strings.ToLower(strings.TrimSpace(ver.Format)) != docs.FormatDocx {
		http.Error(w, localized(lang, "docs.onlyoffice.unsupportedFormat"), http.StatusBadRequest)
		return
	}
	content, err := h.svc.LoadContent(r.Context(), ver)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	w.Header().Set("Content-Disposition", attachmentDisposition(fmt.Sprintf("doc-%d.docx", id)))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func (h *DocsHandler) OnlyOfficeCallback(w http.ResponseWriter, r *http.Request) {
	lang := preferredLang(r)
	writeErr := func(code int) {
		writeJSON(w, http.StatusOK, map[string]int{"error": code})
	}
	if !h.cfg.Docs.OnlyOffice.Enabled {
		writeErr(1)
		return
	}
	id, _ := strconv.ParseInt(pathParams(r)["id"], 10, 64)
	claims, err := parseOnlyOfficeLinkToken(h.cfg.Docs.OnlyOffice.JWTSecret, r.URL.Query().Get("token"), time.Now())
	if err != nil || claims.Purpose != "callback" || claims.DocID != id {
		if h.logger != nil {
			h.logger.Errorf("onlyoffice callback token rejected for doc %d: %v", id, err)
		}
		writeErr(1)
		return
	}
	if err := verifyOnlyOfficeJWTFromRequest(r, h.cfg.Docs.OnlyOffice.JWTSecret, h.cfg.Docs.OnlyOffice.JWTHeader, time.Now()); err != nil {
		if h.logger != nil {
			h.logger.Errorf("onlyoffice callback jwt rejected for doc %d: %v", id, err)
		}
		writeErr(1)
		return
	}

	var payload onlyOfficeCallbackPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeErr(1)
		return
	}
	expectedPrefix := buildOnlyOfficeKey(id, claims.Version)
	if payload.Key != "" && !strings.HasPrefix(payload.Key, expectedPrefix) {
		// stale editor session key, ignore to avoid breaking open editors
		writeErr(0)
		return
	}
	// 1/4 mean open/close without a persisted save event.
	if payload.Status == 1 || payload.Status == 4 {
		writeErr(0)
		return
	}
	// Persist new versions only for explicit force-save requests from app UI.
	if payload.Status != 6 {
		writeErr(0)
		return
	}
	// Accept only CommandService-triggered force-save (forcesavetype=0).
	// This blocks in-editor Save/Ctrl+S from persisting without SCC modal reason flow.
	if payload.ForceSaveType != 0 {
		writeErr(0)
		return
	}
	userData := strings.TrimSpace(payload.UserData)
	if !strings.HasPrefix(strings.ToLower(userData), onlyOfficeUserDataPrefix) {
		writeErr(0)
		return
	}
	saveToken := strings.TrimSpace(strings.TrimPrefix(userData, onlyOfficeUserDataPrefix))
	reason, ok := h.consumeOnlyOfficePendingSave(saveToken, id, claims.Version, claims.Username)
	if !ok {
		writeErr(0)
		return
	}
	if strings.TrimSpace(payload.URL) == "" {
		writeErr(0)
		return
	}
	doc, err := h.store.GetDocument(r.Context(), id)
	if err != nil || doc == nil || !h.isDocument(doc) {
		writeErr(1)
		return
	}
	if doc.CurrentVersion != claims.Version {
		// The document has already advanced to a newer version; ignore stale callback.
		writeErr(0)
		return
	}
	content, err := h.downloadOnlyOfficeResult(r, payload.URL)
	if err != nil {
		if h.logger != nil {
			h.logger.Errorf("onlyoffice callback download failed for doc %d: %v", id, err)
		}
		h.svc.Log(r.Context(), claims.Username, "doc.onlyoffice.callback_error", fmt.Sprintf("%d|download_failed", id))
		writeErr(1)
		return
	}
	currentVer, curErr := h.store.GetVersion(r.Context(), doc.ID, doc.CurrentVersion)
	if curErr == nil && currentVer != nil {
		currentContent, loadErr := h.svc.LoadContent(r.Context(), currentVer)
		if loadErr == nil && bytes.Equal(currentContent, content) {
			writeErr(0)
			return
		}
	}
	author := h.resolveOnlyOfficeAuthor(r, claims.Username, payload.Users, doc.CreatedBy)
	if author == nil {
		if h.logger != nil {
			h.logger.Errorf("onlyoffice callback author resolve failed for doc %d", id)
		}
		writeErr(1)
		return
	}
	if reason == "" {
		reason = localized(lang, "docs.onlyoffice.saveReason")
	}
	if reason == "docs.onlyoffice.saveReason" {
		reason = "onlyoffice edit"
	}
	v, err := h.svc.SaveVersion(r.Context(), docs.SaveRequest{
		Doc:      doc,
		Author:   author,
		Format:   docs.FormatDocx,
		Content:  content,
		Reason:   reason,
		IndexFTS: true,
	})
	if err != nil {
		if h.logger != nil {
			h.logger.Errorf("onlyoffice callback save failed for doc %d: %v", id, err)
		}
		h.svc.Log(r.Context(), author.Username, "doc.onlyoffice.callback_error", fmt.Sprintf("%d|save_failed", id))
		writeErr(1)
		return
	}
	doc.CurrentVersion = v.Version
	doc.Status = docs.StatusDraft
	_ = h.store.UpdateDocument(r.Context(), doc)
	h.svc.Log(r.Context(), author.Username, "doc.onlyoffice.callback_saved", fmt.Sprintf("%d|v%d", id, v.Version))
	writeErr(0)
}

func (h *DocsHandler) resolveOnlyOfficeAuthor(r *http.Request, tokenUser string, callbackUsers []string, fallbackID int64) *store.User {
	candidates := []string{strings.TrimSpace(tokenUser)}
	if len(callbackUsers) > 0 {
		candidates = append(candidates, strings.TrimSpace(callbackUsers[0]))
	}
	for _, name := range candidates {
		if name == "" {
			continue
		}
		u, _, err := h.users.FindByUsername(r.Context(), name)
		if err == nil && u != nil {
			return u
		}
	}
	if fallbackID > 0 {
		u, _, err := h.users.Get(r.Context(), fallbackID)
		if err == nil && u != nil {
			return u
		}
	}
	return nil
}

func (h *DocsHandler) downloadOnlyOfficeResult(r *http.Request, rawURL string) ([]byte, error) {
	downloadURL := strings.TrimSpace(rawURL)
	if rewritten := h.rewriteOnlyOfficeDownloadURL(downloadURL); rewritten != "" {
		downloadURL = rewritten
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, err
	}
	timeoutSec := h.cfg.Docs.OnlyOffice.RequestTimeout
	if timeoutSec <= 0 {
		timeoutSec = 20
	}
	client := &http.Client{
		Timeout: time.Duration(timeoutSec) * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: !h.cfg.Docs.OnlyOffice.VerifyTLS}, //nolint:gosec
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.New("callback download returned non-2xx")
	}
	limit := int64(100 << 20)
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

func (h *DocsHandler) rewriteOnlyOfficeDownloadURL(raw string) string {
	internal := strings.TrimSpace(h.cfg.Docs.OnlyOffice.InternalURL)
	if raw == "" || internal == "" {
		return ""
	}
	src, err := url.Parse(raw)
	if err != nil || src.Scheme == "" || src.Host == "" {
		return ""
	}
	dstBase, err := url.Parse(strings.TrimRight(internal, "/"))
	if err != nil || dstBase.Scheme == "" || dstBase.Host == "" {
		return ""
	}
	publicBase, _ := url.Parse(strings.TrimSpace(h.cfg.Docs.OnlyOffice.PublicURL))
	samePublicHost := publicBase != nil && publicBase.Host != "" && strings.EqualFold(src.Host, publicBase.Host)
	if !samePublicHost && !strings.EqualFold(src.Hostname(), "localhost") && !strings.EqualFold(src.Hostname(), "127.0.0.1") {
		return ""
	}
	rewritten := &url.URL{
		Scheme:   dstBase.Scheme,
		Host:     dstBase.Host,
		Path:     src.EscapedPath(),
		RawQuery: src.RawQuery,
	}
	return rewritten.String()
}

func (h *DocsHandler) putOnlyOfficePendingSave(token string, save onlyOfficePendingSave) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	now := time.Now().UTC()
	h.mu.Lock()
	defer h.mu.Unlock()
	for k, v := range h.officeSaves {
		if v.ExpiresAt.Before(now) {
			delete(h.officeSaves, k)
		}
	}
	h.officeSaves[token] = save
}

func (h *DocsHandler) consumeOnlyOfficePendingSave(token string, docID int64, version int, username string) (string, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}
	now := time.Now().UTC()
	userKey := strings.ToLower(strings.TrimSpace(username))
	h.mu.Lock()
	defer h.mu.Unlock()
	for k, v := range h.officeSaves {
		if v.ExpiresAt.Before(now) {
			delete(h.officeSaves, k)
		}
	}
	save, ok := h.officeSaves[token]
	if !ok {
		return "", false
	}
	delete(h.officeSaves, token)
	if save.DocID != docID || save.Version != version || strings.ToLower(strings.TrimSpace(save.Username)) != userKey {
		return "", false
	}
	return strings.TrimSpace(save.Reason), true
}

func (h *DocsHandler) sendOnlyOfficeForceSave(r *http.Request, docKey, userData string) error {
	baseURL := strings.TrimRight(strings.TrimSpace(h.cfg.Docs.OnlyOffice.InternalURL), "/")
	if baseURL == "" {
		return errors.New("empty onlyoffice internal url")
	}
	userData = strings.TrimSpace(userData)
	body := map[string]any{
		"c":        "forcesave",
		"key":      strings.TrimSpace(docKey),
		"userdata": userData,
	}
	requestBody := body
	headers := map[string]string{
		"Content-Type": "application/json",
	}
	secret := strings.TrimSpace(h.cfg.Docs.OnlyOffice.JWTSecret)
	if secret != "" {
		tokenClaims := map[string]any{
			"c":        body["c"],
			"key":      body["key"],
			"userdata": body["userdata"],
			"iat":      time.Now().UTC().Unix(),
			"exp":      time.Now().UTC().Add(5 * time.Minute).Unix(),
		}
		if iss := strings.TrimSpace(h.cfg.Docs.OnlyOffice.JWTIssuer); iss != "" {
			tokenClaims["iss"] = iss
		}
		if aud := strings.TrimSpace(h.cfg.Docs.OnlyOffice.JWTAudience); aud != "" {
			tokenClaims["aud"] = aud
		}
		token, tokenErr := buildOnlyOfficeJWT(secret, tokenClaims)
		if tokenErr != nil {
			return tokenErr
		}
		requestBody = map[string]any{
			"c":        body["c"],
			"key":      body["key"],
			"userdata": body["userdata"],
			"token":    token,
		}
		headerName := strings.TrimSpace(h.cfg.Docs.OnlyOffice.JWTHeader)
		if headerName == "" {
			headerName = "Authorization"
		}
		headers[headerName] = "Bearer " + token
	}
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}
	timeoutSec := h.cfg.Docs.OnlyOffice.RequestTimeout
	if timeoutSec <= 0 {
		timeoutSec = 20
	}
	client := &http.Client{
		Timeout: time.Duration(timeoutSec) * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: !h.cfg.Docs.OnlyOffice.VerifyTLS}, //nolint:gosec
		},
	}
	for attempt := 0; attempt < 8; attempt++ {
		req, err := http.NewRequestWithContext(
			r.Context(),
			http.MethodPost,
			baseURL+"/coauthoring/CommandService.ashx",
			bytes.NewReader(bodyBytes),
		)
		if err != nil {
			return err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		var cmdResp struct {
			Error int `json:"error"`
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			_ = json.NewDecoder(resp.Body).Decode(&cmdResp)
		}
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return errors.New("command service returned non-2xx")
		}
		if cmdResp.Error == 0 {
			return nil
		}
		// error=4 means no pending changes in docservice yet.
		if cmdResp.Error == 4 && attempt < 7 {
			time.Sleep(600 * time.Millisecond)
			continue
		}
		return fmt.Errorf("command service error: %d", cmdResp.Error)
	}
	return errors.New("command service retry exhausted")
}
