package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"berkut-scc/core/store"
)

type monitoringSettingsPayload struct {
	RetentionDays           int     `json:"retention_days"`
	MaxConcurrentChecks     int     `json:"max_concurrent_checks"`
	DefaultTimeoutSec       int     `json:"default_timeout_sec"`
	DefaultIntervalSec      int     `json:"default_interval_sec"`
	DefaultRetries          int     `json:"default_retries"`
	DefaultRetryIntervalSec int     `json:"default_retry_interval_sec"`
	DefaultSLATargetPct     float64 `json:"default_sla_target_pct"`
	EngineEnabled           *bool   `json:"engine_enabled"`
	AllowPrivateNetworks    *bool   `json:"allow_private_networks"`
	TLSRefreshHours         int     `json:"tls_refresh_hours"`
	TLSExpiringDays         int     `json:"tls_expiring_days"`
	NotifySuppressMinutes   int     `json:"notify_suppress_minutes"`
	NotifyRepeatDownMinutes int     `json:"notify_repeat_down_minutes"`
	NotifyMaintenance       *bool   `json:"notify_maintenance"`
	AutoTaskOnDown          *bool   `json:"auto_task_on_down"`
	AutoTLSIncident         *bool   `json:"auto_tls_incident"`
	AutoTLSIncidentDays     int     `json:"auto_tls_incident_days"`
}

func (h *MonitoringHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.store.GetSettings(r.Context())
	if err != nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (h *MonitoringHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var payload monitoringSettingsPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, errBadRequest, http.StatusBadRequest)
		return
	}
	current, err := h.store.GetSettings(r.Context())
	if err != nil || current == nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	prevTLSRefresh := current.TLSRefreshHours
	prevTLSExpiring := current.TLSExpiringDays
	if payload.RetentionDays > 0 {
		current.RetentionDays = payload.RetentionDays
	}
	if payload.MaxConcurrentChecks > 0 {
		current.MaxConcurrentChecks = payload.MaxConcurrentChecks
	}
	if payload.DefaultTimeoutSec > 0 {
		current.DefaultTimeoutSec = payload.DefaultTimeoutSec
	}
	if payload.DefaultIntervalSec > 0 {
		current.DefaultIntervalSec = payload.DefaultIntervalSec
	}
	if payload.DefaultRetries >= 0 {
		current.DefaultRetries = payload.DefaultRetries
	}
	if payload.DefaultRetryIntervalSec > 0 {
		current.DefaultRetryIntervalSec = payload.DefaultRetryIntervalSec
	}
	if payload.DefaultSLATargetPct > 0 {
		current.DefaultSLATargetPct = payload.DefaultSLATargetPct
	}
	if payload.EngineEnabled != nil {
		current.EngineEnabled = *payload.EngineEnabled
	}
	if payload.AllowPrivateNetworks != nil {
		current.AllowPrivateNetworks = *payload.AllowPrivateNetworks
	}
	if payload.TLSRefreshHours > 0 {
		current.TLSRefreshHours = payload.TLSRefreshHours
	}
	if payload.TLSExpiringDays > 0 {
		current.TLSExpiringDays = payload.TLSExpiringDays
	}
	if payload.NotifySuppressMinutes > 0 {
		current.NotifySuppressMinutes = payload.NotifySuppressMinutes
	}
	if payload.NotifyRepeatDownMinutes > 0 {
		current.NotifyRepeatDownMinutes = payload.NotifyRepeatDownMinutes
	}
	if payload.NotifyMaintenance != nil {
		current.NotifyMaintenance = *payload.NotifyMaintenance
	}
	if payload.AutoTaskOnDown != nil {
		current.AutoTaskOnDown = *payload.AutoTaskOnDown
	}
	if payload.AutoTLSIncident != nil {
		current.AutoTLSIncident = *payload.AutoTLSIncident
	}
	if payload.AutoTLSIncidentDays > 0 {
		current.AutoTLSIncidentDays = payload.AutoTLSIncidentDays
	}
	if current.RetentionDays <= 0 || current.DefaultTimeoutSec <= 0 || current.DefaultIntervalSec <= 0 || current.MaxConcurrentChecks <= 0 {
		http.Error(w, "monitoring.error.invalidSettings", http.StatusBadRequest)
		return
	}
	if current.DefaultRetries < 0 || current.DefaultRetries > 5 || current.DefaultRetryIntervalSec <= 0 {
		http.Error(w, "monitoring.error.invalidSettings", http.StatusBadRequest)
		return
	}
	if current.DefaultSLATargetPct <= 0 || current.DefaultSLATargetPct > 100 {
		http.Error(w, "monitoring.error.invalidSettings", http.StatusBadRequest)
		return
	}
	if current.TLSRefreshHours <= 0 || current.TLSExpiringDays <= 0 {
		http.Error(w, "monitoring.error.invalidSettings", http.StatusBadRequest)
		return
	}
	if current.NotifySuppressMinutes < 0 || current.NotifyRepeatDownMinutes < 0 {
		http.Error(w, "monitoring.error.invalidSettings", http.StatusBadRequest)
		return
	}
	if current.AutoTLSIncidentDays <= 0 {
		http.Error(w, "monitoring.error.invalidSettings", http.StatusBadRequest)
		return
	}
	if err := h.store.UpdateSettings(r.Context(), current); err != nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	if h.engine != nil {
		h.engine.InvalidateSettings()
	}
	h.audit(r, monitorAuditSettingsUpdate, settingsDetails(current))
	if prevTLSRefresh != current.TLSRefreshHours || prevTLSExpiring != current.TLSExpiringDays {
		h.audit(r, monitorAuditCertsSettingsUpdate, settingsDetails(current))
	}
	writeJSON(w, http.StatusOK, current)
}

func settingsDetails(s *store.MonitorSettings) string {
	if s == nil {
		return ""
	}
	parts := []string{
		"retention=" + strconv.Itoa(s.RetentionDays),
		"max_concurrent=" + strconv.Itoa(s.MaxConcurrentChecks),
		"default_timeout=" + strconv.Itoa(s.DefaultTimeoutSec),
		"default_interval=" + strconv.Itoa(s.DefaultIntervalSec),
		"default_retries=" + strconv.Itoa(s.DefaultRetries),
		"default_retry_interval=" + strconv.Itoa(s.DefaultRetryIntervalSec),
		"default_sla=" + strconv.FormatFloat(s.DefaultSLATargetPct, 'f', 1, 64),
		"engine=" + strconv.FormatBool(s.EngineEnabled),
		"allow_private=" + strconv.FormatBool(s.AllowPrivateNetworks),
		"tls_refresh=" + strconv.Itoa(s.TLSRefreshHours),
		"tls_expiring=" + strconv.Itoa(s.TLSExpiringDays),
		"notify_suppress=" + strconv.Itoa(s.NotifySuppressMinutes),
		"notify_repeat=" + strconv.Itoa(s.NotifyRepeatDownMinutes),
		"notify_maintenance=" + strconv.FormatBool(s.NotifyMaintenance),
		"auto_task_on_down=" + strconv.FormatBool(s.AutoTaskOnDown),
		"auto_tls_incident=" + strconv.FormatBool(s.AutoTLSIncident),
		"auto_tls_incident_days=" + strconv.Itoa(s.AutoTLSIncidentDays),
	}
	return strings.Join(parts, "|")
}
