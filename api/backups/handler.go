package backups

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"berkut-scc/core/auth"
	corebackups "berkut-scc/core/backups"
	"berkut-scc/core/store"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	svc    corebackups.ServicePort
	audits store.AuditStore
}

func NewHandler(svc corebackups.ServicePort, audits store.AuditStore) *Handler {
	return &Handler{svc: svc, audits: audits}
}

func (h *Handler) ListBackups(w http.ResponseWriter, r *http.Request) {
	session := currentSession(r)
	corebackups.Log(h.audits, r.Context(), session.Username, corebackups.AuditListBackups, "success", "")
	items, err := h.svc.ListArtifacts(r.Context(), corebackups.ListArtifactsFilter{
		Limit:  parseIntDefault(r.URL.Query().Get("limit"), 100),
		Offset: parseIntDefault(r.URL.Query().Get("offset"), 0),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "backups.internal", "common.serverError")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) GetBackup(w http.ResponseWriter, r *http.Request) {
	session := currentSession(r)
	resource := strings.TrimSpace(r.URL.Query().Get("resource"))
	if resource == "" {
		resource = "artifact"
	}
	id, ok := pathInt64(chi.URLParam(r, "id"))
	if !ok {
		writeError(w, http.StatusBadRequest, corebackups.ErrorCodeInvalidRequest, corebackups.ErrorKeyInvalidRequest)
		return
	}
	item, err := h.svc.GetArtifact(r.Context(), id)
	if err != nil {
		if err == corebackups.ErrNotFound {
			corebackups.Log(h.audits, r.Context(), session.Username, corebackups.AuditReadBackup, "not_found", "id="+strconv.FormatInt(id, 10)+" resource="+resource)
			writeError(w, http.StatusNotFound, "backups.not_found", corebackups.ErrorKeyNotFound)
			return
		}
		writeError(w, http.StatusInternalServerError, "backups.internal", "common.serverError")
		return
	}
	corebackups.Log(h.audits, r.Context(), session.Username, corebackups.AuditReadBackup, "success", "id="+strconv.FormatInt(id, 10)+" resource="+resource)
	writeJSON(w, http.StatusOK, map[string]any{"item": item})
}

func (h *Handler) CreateBackup(w http.ResponseWriter, r *http.Request) {
	session := currentSession(r)
	corebackups.Log(h.audits, r.Context(), session.Username, corebackups.AuditCreateRequested, "requested", "event=backups.create.started")
	payload := struct {
		Label        string   `json:"label"`
		Scope        []string `json:"scope"`
		IncludeFiles bool     `json:"include_files"`
	}{}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&payload)
	}
	result, err := h.svc.CreateBackupWithOptions(r.Context(), corebackups.CreateBackupOptions{
		Label:        payload.Label,
		Scope:        payload.Scope,
		IncludeFiles: payload.IncludeFiles,
		RequestedBy:  session.Username,
	})
	if err != nil {
		if de, ok := corebackups.AsDomainError(err); ok {
			corebackups.Log(h.audits, r.Context(), session.Username, corebackups.AuditCreateFailed, "failed", "code="+de.Code)
			writeError(w, domainErrorHTTPStatus(de.Code), de.Code, de.I18NKey)
			return
		}
		corebackups.Log(h.audits, r.Context(), session.Username, corebackups.AuditCreateFailed, "failed", "code="+corebackups.ErrorCodeInternal)
		writeError(w, http.StatusInternalServerError, corebackups.ErrorCodeInternal, "common.serverError")
		return
	}
	details := "scope=" + strings.Join(normalizeScopeForLog(payload.Scope), ",") + " include_files=" + strconv.FormatBool(payload.IncludeFiles)
	if result != nil && result.Artifact.Filename != nil && result.Artifact.SizeBytes != nil {
		details = details + " filename=" + *result.Artifact.Filename + " size=" + strconv.FormatInt(*result.Artifact.SizeBytes, 10)
	}
	corebackups.Log(h.audits, r.Context(), session.Username, corebackups.AuditCreateSuccess, "success", details+" event=backups.create.success")
	writeJSON(w, http.StatusCreated, result)
}

func (h *Handler) DeleteBackup(w http.ResponseWriter, r *http.Request) {
	session := currentSession(r)
	id, ok := pathInt64(chi.URLParam(r, "id"))
	if !ok {
		writeError(w, http.StatusBadRequest, corebackups.ErrorCodeInvalidRequest, corebackups.ErrorKeyInvalidRequest)
		return
	}
	details := "resource_type=backup backup_id=" + strconv.FormatInt(id, 10)
	corebackups.Log(h.audits, r.Context(), session.Username, corebackups.AuditDeleteBackup, "requested", details)
	if err := h.svc.DeleteBackup(r.Context(), id); err != nil {
		if de, ok := corebackups.AsDomainError(err); ok {
			corebackups.Log(h.audits, r.Context(), session.Username, corebackups.AuditDeleteBackup, "failed", details+" reason_code="+de.Code)
			writeError(w, deleteErrorHTTPStatus(de.Code), de.Code, de.I18NKey)
			return
		}
		corebackups.Log(h.audits, r.Context(), session.Username, corebackups.AuditDeleteBackup, "failed", details+" reason_code="+corebackups.ErrorCodeInternal)
		writeError(w, http.StatusInternalServerError, corebackups.ErrorCodeInternal, corebackups.ErrorKeyInternal)
		return
	}
	corebackups.Log(h.audits, r.Context(), session.Username, corebackups.AuditDeleteBackup, "success", details)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) DownloadBackup(w http.ResponseWriter, r *http.Request) {
	session := currentSession(r)
	id, ok := pathInt64(chi.URLParam(r, "id"))
	if !ok {
		writeError(w, http.StatusBadRequest, corebackups.ErrorCodeInvalidRequest, corebackups.ErrorKeyInvalidRequest)
		return
	}
	details := "resource_type=backup backup_id=" + strconv.FormatInt(id, 10) + " user_id=" + strconv.FormatInt(session.UserID, 10) + " event=backups.download.started"
	corebackups.Log(h.audits, r.Context(), session.Username, corebackups.AuditDownloadRequested, "requested", details)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	artifact, err := h.svc.DownloadArtifact(ctx, id)
	if err != nil {
		if de, ok := corebackups.AsDomainError(err); ok {
			corebackups.Log(h.audits, r.Context(), session.Username, corebackups.AuditDownloadFailed, "failed", details+" reason_code="+de.Code)
			writeError(w, downloadErrorHTTPStatus(de.Code), de.Code, de.I18NKey)
			return
		}
		corebackups.Log(h.audits, r.Context(), session.Username, corebackups.AuditDownloadFailed, "failed", details+" reason_code="+corebackups.ErrorCodeInternal)
		writeError(w, http.StatusInternalServerError, corebackups.ErrorCodeInternal, "common.serverError")
		return
	}
	defer artifact.Reader.Close()

	filename := sanitizeDownloadFilename(artifact.Filename)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	if artifact.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(artifact.Size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, copyErr := io.CopyBuffer(w, artifact.Reader, make([]byte, 32*1024))
	if copyErr != nil {
		corebackups.Log(h.audits, r.Context(), session.Username, corebackups.AuditDownloadFailed, "failed", details+" reason_code="+corebackups.ErrorCodeStreamFailed)
		return
	}
	successDetails := details + " size_bytes=" + strconv.FormatInt(artifact.Size, 10) + " event=backups.download.success"
	corebackups.Log(h.audits, r.Context(), session.Username, corebackups.AuditDownloadSuccess, "success", successDetails)
}

func (h *Handler) StartRestore(w http.ResponseWriter, r *http.Request) {
	session := currentSession(r)
	id, ok := pathInt64(chi.URLParam(r, "id"))
	if !ok {
		writeError(w, http.StatusBadRequest, corebackups.ErrorCodeInvalidRequest, corebackups.ErrorKeyInvalidRequest)
		return
	}
	corebackups.Log(h.audits, r.Context(), session.Username, corebackups.AuditRestoreRequested, "requested", "backup_id="+strconv.FormatInt(id, 10))
	item, err := h.svc.StartRestore(r.Context(), id, session.Username)
	if err != nil {
		if de, ok := corebackups.AsDomainError(err); ok {
			corebackups.Log(h.audits, r.Context(), session.Username, corebackups.AuditRestoreFailed, "failed", "backup_id="+strconv.FormatInt(id, 10)+" reason_code="+de.Code)
			writeError(w, restoreErrorHTTPStatus(de.Code), de.Code, de.I18NKey)
			return
		}
		writeError(w, http.StatusInternalServerError, corebackups.ErrorCodeInternal, "common.serverError")
		return
	}
	corebackups.Log(h.audits, r.Context(), session.Username, corebackups.AuditStartRestore, "queued", "backup_id="+strconv.FormatInt(id, 10)+" restore_id="+strconv.FormatInt(item.ID, 10))
	writeJSON(w, http.StatusAccepted, map[string]any{"restore_id": item.ID, "item": item})
}

func (h *Handler) DryRunRestore(w http.ResponseWriter, r *http.Request) {
	session := currentSession(r)
	id, ok := pathInt64(chi.URLParam(r, "id"))
	if !ok {
		writeError(w, http.StatusBadRequest, corebackups.ErrorCodeInvalidRequest, corebackups.ErrorKeyInvalidRequest)
		return
	}
	corebackups.Log(h.audits, r.Context(), session.Username, corebackups.AuditRestoreDryRun, "requested", "backup_id="+strconv.FormatInt(id, 10))
	item, err := h.svc.StartRestoreDryRun(r.Context(), id, session.Username)
	if err != nil {
		if de, ok := corebackups.AsDomainError(err); ok {
			corebackups.Log(h.audits, r.Context(), session.Username, corebackups.AuditRestoreFailed, "failed", "backup_id="+strconv.FormatInt(id, 10)+" reason_code="+de.Code+" dry_run=true")
			writeError(w, restoreErrorHTTPStatus(de.Code), de.Code, de.I18NKey)
			return
		}
		writeError(w, http.StatusInternalServerError, corebackups.ErrorCodeInternal, "common.serverError")
		return
	}
	corebackups.Log(h.audits, r.Context(), session.Username, corebackups.AuditStartRestore, "queued", "backup_id="+strconv.FormatInt(id, 10)+" restore_id="+strconv.FormatInt(item.ID, 10)+" dry_run=true")
	writeJSON(w, http.StatusAccepted, map[string]any{"restore_id": item.ID, "item": item})
}

func (h *Handler) GetRestoreStatus(w http.ResponseWriter, r *http.Request) {
	session := currentSession(r)
	id, ok := pathInt64(chi.URLParam(r, "restore_id"))
	if !ok {
		writeError(w, http.StatusBadRequest, corebackups.ErrorCodeInvalidRequest, corebackups.ErrorKeyInvalidRequest)
		return
	}
	item, err := h.svc.GetRestoreRun(r.Context(), id)
	if err != nil {
		if err == corebackups.ErrNotFound {
			corebackups.Log(h.audits, r.Context(), session.Username, corebackups.AuditReadRestoreStatus, "not_found", "id="+strconv.FormatInt(id, 10))
			writeError(w, http.StatusNotFound, "backups.not_found", corebackups.ErrorKeyNotFound)
			return
		}
		writeError(w, http.StatusInternalServerError, "backups.internal", "common.serverError")
		return
	}
	corebackups.Log(h.audits, r.Context(), session.Username, corebackups.AuditReadRestoreStatus, "success", "id="+strconv.FormatInt(id, 10))
	writeJSON(w, http.StatusOK, map[string]any{"item": item})
}

func currentSession(r *http.Request) *store.SessionRecord {
	val := r.Context().Value(auth.SessionContextKey)
	if val == nil {
		return &store.SessionRecord{}
	}
	session, ok := val.(*store.SessionRecord)
	if !ok {
		return &store.SessionRecord{}
	}
	return session
}

func pathInt64(raw string) (int64, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func parseIntDefault(raw string, def int) int {
	value := strings.TrimSpace(raw)
	if value == "" {
		return def
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return def
	}
	return n
}

func domainErrorHTTPStatus(code string) int {
	switch code {
	case corebackups.ErrorCodeInvalidEncKey:
		return http.StatusBadRequest
	case corebackups.ErrorCodeConcurrent:
		return http.StatusConflict
	case corebackups.ErrorCodeStorageMissing:
		return http.StatusServiceUnavailable
	case corebackups.ErrorCodePGDumpFailed, corebackups.ErrorCodeEncryptFailed:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

func downloadErrorHTTPStatus(code string) int {
	switch code {
	case corebackups.ErrorCodeNotFound, corebackups.ErrorCodeFileMissing:
		return http.StatusNotFound
	case corebackups.ErrorCodeNotReady, corebackups.ErrorCodeConcurrent:
		return http.StatusConflict
	case corebackups.ErrorCodeInvalidRequest:
		return http.StatusBadRequest
	case corebackups.ErrorCodeCannotOpenFile, corebackups.ErrorCodeStreamFailed:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

func restoreErrorHTTPStatus(code string) int {
	switch code {
	case corebackups.ErrorCodeNotFound, corebackups.ErrorCodeFileMissing:
		return http.StatusNotFound
	case corebackups.ErrorCodeNotReady, corebackups.ErrorCodeIncompatible, corebackups.ErrorCodeConcurrent:
		return http.StatusConflict
	case corebackups.ErrorCodeInvalidEncKey:
		return http.StatusBadRequest
	case corebackups.ErrorCodeDecryptFailed, corebackups.ErrorCodeChecksumFailed:
		return http.StatusBadRequest
	case corebackups.ErrorCodeMaintenance:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func normalizeScopeForLog(scope []string) []string {
	if len(scope) == 0 {
		return []string{"ALL"}
	}
	out := make([]string, 0, len(scope))
	seen := map[string]struct{}{}
	for _, raw := range scope {
		v := strings.ToUpper(strings.TrimSpace(raw))
		if v == "" {
			continue
		}
		if v == "ALL" {
			return []string{"ALL"}
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	if len(out) == 0 {
		return []string{"ALL"}
	}
	return out
}

func deleteErrorHTTPStatus(code string) int {
	switch code {
	case corebackups.ErrorCodeNotFound:
		return http.StatusNotFound
	case corebackups.ErrorCodeConcurrent, corebackups.ErrorCodeFileBusy:
		return http.StatusConflict
	case corebackups.ErrorCodeInvalidRequest:
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func sanitizeDownloadFilename(in string) string {
	out := strings.TrimSpace(in)
	if out == "" {
		out = "backup.bscc"
	}
	out = strings.ReplaceAll(out, "\\", "_")
	out = strings.ReplaceAll(out, "/", "_")
	out = strings.ReplaceAll(out, "\"", "_")
	if !strings.HasSuffix(strings.ToLower(out), ".bscc") {
		out += ".bscc"
	}
	return out
}
