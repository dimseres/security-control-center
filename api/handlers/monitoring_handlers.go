package handlers

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"berkut-scc/core/monitoring"
	"berkut-scc/core/rbac"
	"berkut-scc/core/store"
	"berkut-scc/core/utils"
)

type MonitoringHandler struct {
	store        store.MonitoringStore
	audits       store.AuditStore
	engine       *monitoring.Engine
	policy       *rbac.Policy
	encryptor    *utils.Encryptor
	checkNowMu   sync.Mutex
	lastCheckNow map[int64]time.Time
}

func NewMonitoringHandler(store store.MonitoringStore, audits store.AuditStore, engine *monitoring.Engine, policy *rbac.Policy, encryptor *utils.Encryptor) *MonitoringHandler {
	return &MonitoringHandler{
		store:        store,
		audits:       audits,
		engine:       engine,
		policy:       policy,
		encryptor:    encryptor,
		lastCheckNow: map[int64]time.Time{},
	}
}

func (h *MonitoringHandler) ListMonitors(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := store.MonitorFilter{
		Query:  strings.TrimSpace(q.Get("q")),
		Status: strings.TrimSpace(q.Get("status")),
		Tags:   splitCSV(q.Get("tag")),
	}
	if active := strings.TrimSpace(q.Get("active")); active != "" {
		val := active == "1" || strings.ToLower(active) == "true"
		filter.Active = &val
	}
	items, err := h.store.ListMonitors(r.Context(), filter)
	if err != nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *MonitoringHandler) CreateMonitor(w http.ResponseWriter, r *http.Request) {
	var payload monitorPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, errBadRequest, http.StatusBadRequest)
		return
	}
	if requiresIncidentLink(payload) && !hasPermission(r, h.policy, "monitoring.incidents.link") {
		http.Error(w, "monitoring.forbiddenIncidentLink", http.StatusForbidden)
		return
	}
	settings, _ := h.store.GetSettings(r.Context())
	mon, err := payloadToMonitor(payload, settings, sessionUserID(r))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id, err := h.store.CreateMonitor(r.Context(), mon)
	if err != nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	mon.ID = id
	_ = h.store.UpsertMonitorState(r.Context(), &store.MonitorState{
		MonitorID:        mon.ID,
		Status:           initialStatus(mon.IsPaused),
		LastResultStatus: "down",
	})
	h.requestImmediateCheck(mon.ID)
	h.audit(r, monitorAuditMonitorCreate, strconv.FormatInt(id, 10))
	writeJSON(w, http.StatusCreated, mon)
}

func (h *MonitoringHandler) GetMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(pathParams(r)["id"])
	if err != nil {
		http.Error(w, errBadRequest, http.StatusBadRequest)
		return
	}
	mon, err := h.store.GetMonitor(r.Context(), id)
	if err != nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	if mon == nil {
		http.Error(w, errNotFound, http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, mon)
}

func (h *MonitoringHandler) UpdateMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(pathParams(r)["id"])
	if err != nil {
		http.Error(w, errBadRequest, http.StatusBadRequest)
		return
	}
	existing, err := h.store.GetMonitor(r.Context(), id)
	if err != nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	if existing == nil {
		http.Error(w, errNotFound, http.StatusNotFound)
		return
	}
	var payload monitorPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, errBadRequest, http.StatusBadRequest)
		return
	}
	if requiresIncidentLink(payload) && !hasPermission(r, h.policy, "monitoring.incidents.link") {
		http.Error(w, "monitoring.forbiddenIncidentLink", http.StatusForbidden)
		return
	}
	slaChanged := false
	if payload.SLATargetPct != nil {
		if existing.SLATargetPct == nil || *existing.SLATargetPct != *payload.SLATargetPct {
			slaChanged = true
		}
	}
	settings, _ := h.store.GetSettings(r.Context())
	mon, err := mergeMonitor(existing, payload, settings)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.store.UpdateMonitor(r.Context(), mon); err != nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	if existing.IsPaused != mon.IsPaused {
		_ = h.store.SetMonitorPaused(r.Context(), id, mon.IsPaused)
	}
	_ = h.store.MarkMonitorDueNow(r.Context(), id)
	h.requestImmediateCheck(id)
	h.audit(r, monitorAuditMonitorUpdate, strconv.FormatInt(id, 10))
	if slaChanged {
		h.audit(r, monitorAuditSLAUpdate, strconv.FormatInt(id, 10))
		target := 90.0
		if mon.SLATargetPct != nil && *mon.SLATargetPct > 0 && *mon.SLATargetPct <= 100 {
			target = *mon.SLATargetPct
		} else if settings != nil && settings.DefaultSLATargetPct > 0 && settings.DefaultSLATargetPct <= 100 {
			target = settings.DefaultSLATargetPct
		}
		minCoverage := 80.0
		if policy, err := h.store.GetMonitorSLAPolicy(r.Context(), id); err == nil && policy != nil && policy.MinCoveragePct > 0 && policy.MinCoveragePct <= 100 {
			minCoverage = policy.MinCoveragePct
		}
		_ = h.store.SyncSLAPeriodTarget(r.Context(), id, target, minCoverage)
	}
	writeJSON(w, http.StatusOK, mon)
}

func (h *MonitoringHandler) DeleteMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(pathParams(r)["id"])
	if err != nil {
		http.Error(w, errBadRequest, http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteMonitor(r.Context(), id); err != nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	h.audit(r, monitorAuditMonitorDelete, strconv.FormatInt(id, 10))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *MonitoringHandler) PauseMonitor(w http.ResponseWriter, r *http.Request) {
	h.setPaused(w, r, true, monitorAuditMonitorPause)
}

func (h *MonitoringHandler) ResumeMonitor(w http.ResponseWriter, r *http.Request) {
	h.setPaused(w, r, false, monitorAuditMonitorResume)
}

func (h *MonitoringHandler) setPaused(w http.ResponseWriter, r *http.Request, paused bool, audit string) {
	id, err := parseID(pathParams(r)["id"])
	if err != nil {
		http.Error(w, errBadRequest, http.StatusBadRequest)
		return
	}
	if err := h.store.SetMonitorPaused(r.Context(), id, paused); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, errNotFound, http.StatusNotFound)
			return
		}
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	h.audit(r, audit, strconv.FormatInt(id, 10))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *MonitoringHandler) CheckNow(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(pathParams(r)["id"])
	if err != nil {
		http.Error(w, errBadRequest, http.StatusBadRequest)
		return
	}
	if !h.allowCheckNow(id) {
		http.Error(w, "monitoring.error.tooFrequent", http.StatusTooManyRequests)
		return
	}
	if h.engine == nil {
		http.Error(w, errServiceUnavailable, http.StatusServiceUnavailable)
		return
	}
	if err := h.engine.CheckNow(r.Context(), id); err != nil {
		switch strings.TrimSpace(err.Error()) {
		case "monitoring.error.busy":
			writeJSON(w, http.StatusAccepted, map[string]string{"status": "busy"})
		case "monitoring.error.engineDisabled":
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
		case "common.notFound":
			http.Error(w, err.Error(), http.StatusNotFound)
		default:
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		return
	}
	h.audit(r, monitorAuditMonitorCheckNow, strconv.FormatInt(id, 10))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *MonitoringHandler) allowCheckNow(monitorID int64) bool {
	h.checkNowMu.Lock()
	defer h.checkNowMu.Unlock()
	now := time.Now().UTC()
	last := h.lastCheckNow[monitorID]
	if !last.IsZero() && now.Sub(last) < 2*time.Second {
		return false
	}
	h.lastCheckNow[monitorID] = now
	if len(h.lastCheckNow) > 2000 {
		cutoff := now.Add(-10 * time.Minute)
		for id, ts := range h.lastCheckNow {
			if ts.Before(cutoff) {
				delete(h.lastCheckNow, id)
			}
		}
	}
	return true
}

func (h *MonitoringHandler) CloneMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(pathParams(r)["id"])
	if err != nil {
		http.Error(w, errBadRequest, http.StatusBadRequest)
		return
	}
	existing, err := h.store.GetMonitor(r.Context(), id)
	if err != nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	if existing == nil {
		http.Error(w, errNotFound, http.StatusNotFound)
		return
	}
	clone := *existing
	clone.ID = 0
	clone.Name = strings.TrimSpace(existing.Name) + " (copy)"
	if monitoring.TypeIsPassive(clone.Type) {
		clone.RequestBody = randomPushToken()
	}
	clone.CreatedBy = sessionUserID(r)
	clone.CreatedAt = time.Time{}
	clone.UpdatedAt = time.Time{}
	newID, err := h.store.CreateMonitor(r.Context(), &clone)
	if err != nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	clone.ID = newID
	if items, err := h.store.ListMonitorNotifications(r.Context(), id); err == nil && len(items) > 0 {
		cloneItems := make([]store.MonitorNotification, 0, len(items))
		for _, item := range items {
			if item.NotificationChannelID <= 0 {
				continue
			}
			cloneItems = append(cloneItems, store.MonitorNotification{
				MonitorID:             newID,
				NotificationChannelID: item.NotificationChannelID,
				Enabled:               item.Enabled,
			})
		}
		if len(cloneItems) > 0 {
			_ = h.store.ReplaceMonitorNotifications(r.Context(), newID, cloneItems)
		}
	}
	_ = h.store.UpsertMonitorState(r.Context(), &store.MonitorState{
		MonitorID:        clone.ID,
		Status:           initialStatus(clone.IsPaused),
		LastResultStatus: "down",
	})
	h.requestImmediateCheck(clone.ID)
	h.audit(r, monitorAuditMonitorClone, strconv.FormatInt(newID, 10))
	writeJSON(w, http.StatusCreated, clone)
}

func (h *MonitoringHandler) requestImmediateCheck(monitorID int64) {
	if h == nil || h.engine == nil || monitorID <= 0 {
		return
	}
	go func(id int64) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		// If the worker pool is busy, retry briefly; otherwise the monitor may stay stale until next interval.
		deadline := time.Now().Add(18 * time.Second)
		for {
			err := h.engine.CheckNow(ctx, id)
			if err == nil {
				return
			}
			if strings.TrimSpace(err.Error()) != "monitoring.error.busy" {
				return
			}
			if time.Now().After(deadline) {
				return
			}
			time.Sleep(450 * time.Millisecond)
		}
	}(monitorID)
}

func randomPushToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback: not ideal, but better than empty token.
		return strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	}
	return hex.EncodeToString(b)
}
