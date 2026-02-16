package handlers

import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"berkut-scc/core/monitoring"
	"berkut-scc/core/store"
)

type monitorPayload struct {
	Name              string            `json:"name"`
	Type              string            `json:"type"`
	URL               string            `json:"url"`
	Host              string            `json:"host"`
	Port              int               `json:"port"`
	Method            string            `json:"method"`
	RequestBody       string            `json:"request_body"`
	RequestBodyType   string            `json:"request_body_type"`
	Headers           map[string]string `json:"headers"`
	IntervalSec       int               `json:"interval_sec"`
	TimeoutSec        int               `json:"timeout_sec"`
	Retries           int               `json:"retries"`
	RetryIntervalSec  int               `json:"retry_interval_sec"`
	AllowedStatus     []string          `json:"allowed_status"`
	IsActive          *bool             `json:"is_active"`
	IsPaused          *bool             `json:"is_paused"`
	Tags              []string          `json:"tags"`
	GroupID           *int64            `json:"group_id"`
	SLATargetPct      *float64          `json:"sla_target_pct"`
	IgnoreTLSErrors   *bool             `json:"ignore_tls_errors"`
	NotifyTLSExpiring *bool             `json:"notify_tls_expiring"`
	AutoIncident      *bool             `json:"auto_incident"`
	AutoTaskOnDown    *bool             `json:"auto_task_on_down"`
	IncidentSeverity  string            `json:"incident_severity"`
	IncidentTypeID    string            `json:"incident_type_id"`
}

func payloadToMonitor(payload monitorPayload, settings *store.MonitorSettings, createdBy int64) (*store.Monitor, error) {
	m := &store.Monitor{
		Name:             strings.TrimSpace(payload.Name),
		Type:             monitoring.NormalizeType(payload.Type),
		URL:              strings.TrimSpace(payload.URL),
		Host:             strings.TrimSpace(payload.Host),
		Port:             payload.Port,
		Method:           strings.ToUpper(strings.TrimSpace(payload.Method)),
		RequestBody:      payload.RequestBody,
		RequestBodyType:  strings.ToLower(strings.TrimSpace(payload.RequestBodyType)),
		Headers:          payload.Headers,
		IntervalSec:      payload.IntervalSec,
		TimeoutSec:       payload.TimeoutSec,
		Retries:          payload.Retries,
		RetryIntervalSec: payload.RetryIntervalSec,
		AllowedStatus:    payload.AllowedStatus,
		Tags:             payload.Tags,
		GroupID:          payload.GroupID,
		SLATargetPct:     payload.SLATargetPct,
		IncidentSeverity: strings.ToLower(strings.TrimSpace(payload.IncidentSeverity)),
		IncidentTypeID:   strings.TrimSpace(payload.IncidentTypeID),
		CreatedBy:        createdBy,
	}
	if payload.IsActive != nil {
		m.IsActive = *payload.IsActive
	} else {
		m.IsActive = true
	}
	if payload.IsPaused != nil {
		m.IsPaused = *payload.IsPaused
	}
	if payload.AutoIncident != nil {
		m.AutoIncident = *payload.AutoIncident
	}
	if payload.AutoTaskOnDown != nil {
		m.AutoTaskOnDown = *payload.AutoTaskOnDown
	}
	if payload.IgnoreTLSErrors != nil {
		m.IgnoreTLSErrors = *payload.IgnoreTLSErrors
	}
	if payload.NotifyTLSExpiring != nil {
		m.NotifyTLSExpiring = *payload.NotifyTLSExpiring
	} else {
		m.NotifyTLSExpiring = true
	}
	if m.AutoIncident && m.IncidentSeverity == "" {
		m.IncidentSeverity = "low"
	}
	applyDefaults(m, settings)
	if err := validateMonitor(m); err != nil {
		return nil, err
	}
	return m, nil
}

func mergeMonitor(existing *store.Monitor, payload monitorPayload, settings *store.MonitorSettings) (*store.Monitor, error) {
	m := *existing
	if payload.Name != "" {
		m.Name = strings.TrimSpace(payload.Name)
	}
	if payload.Type != "" {
		m.Type = monitoring.NormalizeType(payload.Type)
	}
	if payload.URL != "" || monitoring.TypeUsesURL(strings.ToLower(payload.Type)) {
		m.URL = strings.TrimSpace(payload.URL)
	}
	if payload.Host != "" || monitoring.TypeUsesHostPort(strings.ToLower(payload.Type)) {
		m.Host = strings.TrimSpace(payload.Host)
	}
	if payload.Port > 0 {
		m.Port = payload.Port
	}
	if payload.Method != "" {
		m.Method = strings.ToUpper(strings.TrimSpace(payload.Method))
	}
	if payload.RequestBodyType != "" {
		m.RequestBodyType = strings.ToLower(strings.TrimSpace(payload.RequestBodyType))
	}
	if payload.RequestBody != "" || payload.RequestBodyType != "" {
		m.RequestBody = payload.RequestBody
	}
	if payload.Headers != nil {
		m.Headers = payload.Headers
	}
	if payload.IntervalSec > 0 {
		m.IntervalSec = payload.IntervalSec
	}
	if payload.TimeoutSec > 0 {
		m.TimeoutSec = payload.TimeoutSec
	}
	if payload.Retries >= 0 {
		m.Retries = payload.Retries
	}
	if payload.RetryIntervalSec > 0 {
		m.RetryIntervalSec = payload.RetryIntervalSec
	}
	if payload.AllowedStatus != nil {
		m.AllowedStatus = payload.AllowedStatus
	}
	if payload.Tags != nil {
		m.Tags = payload.Tags
	}
	if payload.GroupID != nil {
		m.GroupID = payload.GroupID
	}
	if payload.SLATargetPct != nil {
		m.SLATargetPct = payload.SLATargetPct
	}
	if payload.IgnoreTLSErrors != nil {
		m.IgnoreTLSErrors = *payload.IgnoreTLSErrors
	}
	if payload.NotifyTLSExpiring != nil {
		m.NotifyTLSExpiring = *payload.NotifyTLSExpiring
	}
	if payload.AutoIncident != nil {
		m.AutoIncident = *payload.AutoIncident
	}
	if payload.AutoTaskOnDown != nil {
		m.AutoTaskOnDown = *payload.AutoTaskOnDown
	}
	if payload.IncidentSeverity != "" || payload.AutoIncident != nil {
		m.IncidentSeverity = strings.ToLower(strings.TrimSpace(payload.IncidentSeverity))
		if m.AutoIncident && m.IncidentSeverity == "" {
			m.IncidentSeverity = "low"
		}
	}
	if payload.IncidentTypeID != "" || payload.AutoIncident != nil {
		m.IncidentTypeID = strings.TrimSpace(payload.IncidentTypeID)
	}
	if payload.IsActive != nil {
		m.IsActive = *payload.IsActive
	}
	if payload.IsPaused != nil {
		m.IsPaused = *payload.IsPaused
	}
	applyDefaults(&m, settings)
	if err := validateMonitor(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

func applyDefaults(m *store.Monitor, settings *store.MonitorSettings) {
	if settings != nil {
		if m.IntervalSec <= 0 {
			m.IntervalSec = settings.DefaultIntervalSec
		}
		if m.TimeoutSec <= 0 {
			m.TimeoutSec = settings.DefaultTimeoutSec
		}
		if m.Retries <= 0 {
			m.Retries = settings.DefaultRetries
		}
		if m.RetryIntervalSec <= 0 {
			m.RetryIntervalSec = settings.DefaultRetryIntervalSec
		}
		if m.SLATargetPct == nil && settings.DefaultSLATargetPct > 0 {
			val := settings.DefaultSLATargetPct
			m.SLATargetPct = &val
		}
	}
	if m.IntervalSec <= 0 {
		m.IntervalSec = 60
	}
	if m.TimeoutSec <= 0 {
		m.TimeoutSec = 5
	}
	if m.Retries < 0 {
		m.Retries = 0
	}
	if m.RetryIntervalSec <= 0 {
		m.RetryIntervalSec = 5
	}
	if m.RequestBodyType == "" {
		m.RequestBodyType = "none"
	}
	if m.Method == "" {
		m.Method = "GET"
	}
	switch monitoring.NormalizeType(m.Type) {
	case monitoring.TypeDNS:
		if m.Method == "" || m.Method == "GET" {
			m.Method = "A"
		}
	default:
		if m.Port <= 0 {
			if p := monitoring.DefaultPortForType(m.Type); p > 0 {
				m.Port = p
			}
		}
	}
	if len(m.AllowedStatus) == 0 {
		m.AllowedStatus = []string{"200-299"}
	}
}

func validateMonitor(m *store.Monitor) error {
	if m.Name == "" {
		return errors.New("monitoring.error.nameRequired")
	}
	if !monitoring.IsSupportedType(m.Type) {
		return errors.New("monitoring.error.invalidType")
	}
	switch {
	case monitoring.TypeUsesURL(m.Type):
		if err := validateHTTPMonitor(m); err != nil {
			return err
		}
	case monitoring.TypeUsesHostPort(m.Type):
		if err := validateTCPMonitor(m); err != nil {
			return err
		}
	case monitoring.TypeIsPassive(m.Type):
		// Passive monitors are updated externally and do not require target fields.
		if strings.TrimSpace(m.RequestBody) == "" {
			return errors.New("monitoring.error.pushTokenRequired")
		}
	}
	if m.IntervalSec <= 0 {
		return errors.New("monitoring.error.invalidInterval")
	}
	if m.TimeoutSec <= 0 {
		return errors.New("monitoring.error.invalidTimeout")
	}
	if m.Retries < 0 || m.Retries > 5 {
		return errors.New("monitoring.error.invalidRetries")
	}
	if m.Retries > 0 && m.RetryIntervalSec <= 0 {
		return errors.New("monitoring.error.invalidRetryInterval")
	}
	if m.RequestBodyType != "" && m.RequestBodyType != "none" && m.RequestBodyType != "json" && m.RequestBodyType != "xml" {
		return errors.New("monitoring.error.invalidBodyType")
	}
	if strings.EqualFold(m.Type, monitoring.TypeHTTPKeyword) && strings.TrimSpace(m.RequestBody) == "" {
		return errors.New("monitoring.error.keywordRequired")
	}
	if !validateStatusRanges(m.AllowedStatus) {
		return errors.New("monitoring.error.invalidStatusRange")
	}
	if m.SLATargetPct != nil {
		if *m.SLATargetPct <= 0 || *m.SLATargetPct > 100 {
			return errors.New("monitoring.error.invalidSLA")
		}
	}
	if m.AutoIncident {
		switch strings.ToLower(strings.TrimSpace(m.IncidentSeverity)) {
		case "low", "medium", "high", "critical":
		default:
			return errors.New("monitoring.error.invalidIncidentSeverity")
		}
	}
	if !validateHeaders(m.Headers) {
		return errors.New("monitoring.error.invalidHeaders")
	}
	return nil
}

func validateHTTPMonitor(m *store.Monitor) error {
	u, err := url.Parse(strings.TrimSpace(m.URL))
	if err != nil || u == nil || u.Scheme == "" || u.Host == "" {
		return errors.New("monitoring.error.invalidUrl")
	}
	scheme := strings.ToLower(u.Scheme)
	switch strings.ToLower(strings.TrimSpace(m.Type)) {
	case "postgres":
		if scheme != "postgres" && scheme != "postgresql" {
			return errors.New("monitoring.error.invalidUrl")
		}
	case "grpc_keyword":
		if scheme != "grpc" && scheme != "grpcs" {
			return errors.New("monitoring.error.invalidUrl")
		}
	default:
		if scheme != "http" && scheme != "https" {
			return errors.New("monitoring.error.invalidUrl")
		}
	}
	if strings.ToLower(strings.TrimSpace(m.Type)) == monitoring.TypePostgres {
		host := u.Hostname()
		m.Host = host
		if port := u.Port(); port != "" {
			if n, err := strconv.Atoi(port); err == nil {
				m.Port = n
			}
		}
		return nil
	}
	if strings.ToLower(strings.TrimSpace(m.Type)) != "grpc_keyword" {
		if m.Method != "GET" && m.Method != "POST" {
			return errors.New("monitoring.error.invalidMethod")
		}
	}
	host := u.Hostname()
	m.Host = host
	if port := u.Port(); port != "" {
		if n, err := strconv.Atoi(port); err == nil {
			m.Port = n
		}
	}
	return nil
}

func validateTCPMonitor(m *store.Monitor) error {
	m.Host = normalizeMonitorHost(m.Host)
	if m.Host == "" {
		return errors.New("monitoring.error.invalidHost")
	}
	if strings.EqualFold(m.Type, monitoring.TypeDNS) || strings.EqualFold(m.Type, monitoring.TypePing) || strings.EqualFold(m.Type, monitoring.TypeTailscalePing) {
		return nil
	}
	if m.Port <= 0 || m.Port > 65535 {
		return errors.New("monitoring.error.invalidPort")
	}
	return nil
}

func normalizeMonitorHost(raw string) string {
	host := strings.TrimSpace(raw)
	if host == "" {
		return ""
	}
	if strings.Contains(host, "://") {
		if u, err := url.Parse(host); err == nil && strings.TrimSpace(u.Hostname()) != "" {
			host = strings.TrimSpace(u.Hostname())
		}
	}
	return host
}

func monitorTypeUsesURL(kind string) bool {
	return monitoring.TypeUsesURL(kind)
}

func monitorTypeUsesHost(kind string) bool {
	return monitoring.TypeUsesHostPort(kind)
}

func validateHeaders(headers map[string]string) bool {
	for k := range headers {
		if strings.TrimSpace(k) == "" {
			return false
		}
	}
	return true
}

func validateStatusRanges(ranges []string) bool {
	for _, raw := range ranges {
		val := strings.TrimSpace(raw)
		if val == "" {
			continue
		}
		if strings.Contains(val, "-") {
			parts := strings.SplitN(val, "-", 2)
			if len(parts) != 2 {
				return false
			}
			min, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
			max, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err1 != nil || err2 != nil || min <= 0 || max < min {
				return false
			}
			continue
		}
		if _, err := strconv.Atoi(val); err != nil {
			return false
		}
	}
	return true
}

func requiresIncidentLink(payload monitorPayload) bool {
	if payload.AutoIncident != nil {
		return true
	}
	if strings.TrimSpace(payload.IncidentSeverity) != "" {
		return true
	}
	if strings.TrimSpace(payload.IncidentTypeID) != "" {
		return true
	}
	return false
}
