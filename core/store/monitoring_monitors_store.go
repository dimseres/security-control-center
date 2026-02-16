package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

func (s *monitoringStore) CreateMonitor(ctx context.Context, m *Monitor) (int64, error) {
	now := time.Now().UTC()
	headersJSON, _ := json.Marshal(normalizeHeaders(m.Headers))
	allowedJSON, _ := json.Marshal(normalizeStatusRanges(m.AllowedStatus))
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO monitors(name, type, url, host, port, method, request_body, request_body_type, headers_json, interval_sec, timeout_sec, retries, retry_interval_sec, allowed_status_json, ignore_tls_errors, notify_tls_expiring, is_active, is_paused, tags_json, group_id, sla_target_pct, auto_incident, auto_task_on_down, incident_severity, incident_type_id, created_by, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		strings.TrimSpace(m.Name), strings.ToLower(strings.TrimSpace(m.Type)), strings.TrimSpace(m.URL), strings.TrimSpace(m.Host),
		m.Port, strings.ToUpper(strings.TrimSpace(m.Method)), m.RequestBody, strings.ToLower(strings.TrimSpace(m.RequestBodyType)),
		string(headersJSON), m.IntervalSec, m.TimeoutSec, m.Retries, m.RetryIntervalSec, string(allowedJSON),
		boolToInt(m.IgnoreTLSErrors), boolToInt(m.NotifyTLSExpiring), boolToInt(m.IsActive), boolToInt(m.IsPaused),
		tagsToJSON(normalizeMonitorTags(m.Tags)), nullableID(m.GroupID), m.SLATargetPct,
		boolToInt(m.AutoIncident), boolToInt(m.AutoTaskOnDown), strings.TrimSpace(m.IncidentSeverity), strings.TrimSpace(m.IncidentTypeID),
		m.CreatedBy, now, now)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func (s *monitoringStore) UpdateMonitor(ctx context.Context, m *Monitor) error {
	headersJSON, _ := json.Marshal(normalizeHeaders(m.Headers))
	allowedJSON, _ := json.Marshal(normalizeStatusRanges(m.AllowedStatus))
	_, err := s.db.ExecContext(ctx, `
		UPDATE monitors
		SET name=?, type=?, url=?, host=?, port=?, method=?, request_body=?, request_body_type=?, headers_json=?, interval_sec=?, timeout_sec=?, retries=?, retry_interval_sec=?, allowed_status_json=?, ignore_tls_errors=?, notify_tls_expiring=?, is_active=?, is_paused=?, tags_json=?, group_id=?, sla_target_pct=?, auto_incident=?, auto_task_on_down=?, incident_severity=?, incident_type_id=?, updated_at=?
		WHERE id=?`,
		strings.TrimSpace(m.Name), strings.ToLower(strings.TrimSpace(m.Type)), strings.TrimSpace(m.URL), strings.TrimSpace(m.Host),
		m.Port, strings.ToUpper(strings.TrimSpace(m.Method)), m.RequestBody, strings.ToLower(strings.TrimSpace(m.RequestBodyType)),
		string(headersJSON), m.IntervalSec, m.TimeoutSec, m.Retries, m.RetryIntervalSec, string(allowedJSON),
		boolToInt(m.IgnoreTLSErrors), boolToInt(m.NotifyTLSExpiring), boolToInt(m.IsActive), boolToInt(m.IsPaused),
		tagsToJSON(normalizeMonitorTags(m.Tags)), nullableID(m.GroupID), m.SLATargetPct,
		boolToInt(m.AutoIncident), boolToInt(m.AutoTaskOnDown), strings.TrimSpace(m.IncidentSeverity), strings.TrimSpace(m.IncidentTypeID),
		time.Now().UTC(), m.ID)
	return err
}

func (s *monitoringStore) DeleteMonitor(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM monitors WHERE id=?`, id)
	return err
}

func (s *monitoringStore) GetMonitor(ctx context.Context, id int64) (*Monitor, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, type, url, host, port, method, request_body, request_body_type, headers_json, interval_sec, timeout_sec, retries, retry_interval_sec, allowed_status_json, ignore_tls_errors, notify_tls_expiring, is_active, is_paused, tags_json, group_id, sla_target_pct, auto_incident, auto_task_on_down, incident_severity, incident_type_id, created_by, created_at, updated_at
		FROM monitors WHERE id=?`, id)
	return scanMonitor(row)
}

func (s *monitoringStore) ListMonitors(ctx context.Context, filter MonitorFilter) ([]MonitorSummary, error) {
	query := `
		SELECT m.id, m.name, m.type, m.url, m.host, m.port, m.method, m.request_body, m.request_body_type, m.headers_json,
			m.interval_sec, m.timeout_sec, m.retries, m.retry_interval_sec, m.allowed_status_json, m.ignore_tls_errors, m.notify_tls_expiring, m.is_active, m.is_paused,
			m.tags_json, m.group_id, m.sla_target_pct, m.auto_incident, m.auto_task_on_down, m.incident_severity, m.incident_type_id, m.created_by, m.created_at, m.updated_at,
			COALESCE(s.status, ''), s.last_checked_at, s.last_up_at, s.last_down_at, s.last_latency_ms, s.last_status_code, s.last_error
		FROM monitors m
		LEFT JOIN monitor_state s ON s.monitor_id=m.id`
	var clauses []string
	var args []any
	if q := strings.TrimSpace(filter.Query); q != "" {
		p := "%" + strings.ToLower(q) + "%"
		clauses = append(clauses, "(LOWER(m.name) LIKE ? OR LOWER(m.url) LIKE ? OR LOWER(m.host) LIKE ?)")
		args = append(args, p, p, p)
	}
	if len(filter.Tags) > 0 {
		for _, tag := range normalizeMonitorTags(filter.Tags) {
			clauses = append(clauses, "m.tags_json LIKE ?")
			args = append(args, "%"+tag+"%")
		}
	}
	if st := strings.TrimSpace(filter.Status); st != "" {
		clauses = append(clauses, "LOWER(s.status)=?")
		args = append(args, strings.ToLower(st))
	}
	if filter.Active != nil {
		clauses = append(clauses, "m.is_active=?")
		args = append(args, boolToInt(*filter.Active))
	}
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY m.name"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []MonitorSummary
	for rows.Next() {
		item, err := scanMonitorSummary(rows)
		if err != nil {
			return nil, err
		}
		res = append(res, item)
	}
	return res, rows.Err()
}

func (s *monitoringStore) ListDueMonitors(ctx context.Context, now time.Time) ([]Monitor, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT m.id, m.name, m.type, m.url, m.host, m.port, m.method, m.request_body, m.request_body_type, m.headers_json,
			m.interval_sec, m.timeout_sec, m.retries, m.retry_interval_sec, m.allowed_status_json, m.ignore_tls_errors, m.notify_tls_expiring, m.is_active, m.is_paused,
			m.tags_json, m.group_id, m.sla_target_pct, m.auto_incident, m.auto_task_on_down, m.incident_severity, m.incident_type_id, m.created_by, m.created_at, m.updated_at,
			s.last_checked_at
		FROM monitors m
		LEFT JOIN monitor_state s ON s.monitor_id=m.id
		WHERE m.is_active=1 AND m.is_paused=0`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []Monitor
	for rows.Next() {
		var m Monitor
		var headersRaw, allowedRaw, tagsRaw string
		var isActive, isPaused, autoIncident, autoTaskOnDown, ignoreTLS, notifyTLS int
		var groupID sql.NullInt64
		var sla sql.NullFloat64
		var lastChecked sql.NullTime
		if err := rows.Scan(
			&m.ID, &m.Name, &m.Type, &m.URL, &m.Host, &m.Port, &m.Method, &m.RequestBody, &m.RequestBodyType, &headersRaw,
			&m.IntervalSec, &m.TimeoutSec, &m.Retries, &m.RetryIntervalSec, &allowedRaw, &ignoreTLS, &notifyTLS, &isActive, &isPaused,
			&tagsRaw, &groupID, &sla, &autoIncident, &autoTaskOnDown, &m.IncidentSeverity, &m.IncidentTypeID, &m.CreatedBy, &m.CreatedAt, &m.UpdatedAt,
			&lastChecked,
		); err != nil {
			return nil, err
		}
		m.IgnoreTLSErrors = ignoreTLS == 1
		m.NotifyTLSExpiring = notifyTLS == 1
		m.IsActive = isActive == 1
		m.IsPaused = isPaused == 1
		m.AutoIncident = autoIncident == 1
		m.AutoTaskOnDown = autoTaskOnDown == 1
		if headersRaw != "" {
			_ = json.Unmarshal([]byte(headersRaw), &m.Headers)
		}
		if allowedRaw != "" {
			_ = json.Unmarshal([]byte(allowedRaw), &m.AllowedStatus)
		}
		if tagsRaw != "" {
			_ = json.Unmarshal([]byte(tagsRaw), &m.Tags)
		}
		if groupID.Valid {
			m.GroupID = &groupID.Int64
		}
		if sla.Valid {
			val := sla.Float64
			m.SLATargetPct = &val
		}
		interval := m.IntervalSec
		if interval <= 0 {
			interval = 60
		}
		if !lastChecked.Valid || now.Sub(lastChecked.Time) >= time.Duration(interval)*time.Second {
			res = append(res, m)
		}
	}
	return res, rows.Err()
}

func (s *monitoringStore) SetMonitorPaused(ctx context.Context, id int64, paused bool) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `UPDATE monitors SET is_paused=?, updated_at=? WHERE id=?`, boolToInt(paused), now, id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}

	current, err := s.GetMonitorState(ctx, id)
	if err != nil {
		return err
	}
	status := "paused"
	maintenanceActive := 0
	lastResult := "down"
	var lastChecked *time.Time
	var lastUp *time.Time
	var lastDown *time.Time
	var lastLatency *int
	var lastStatusCode *int
	lastError := ""
	var uptime24h float64
	var uptime30d float64
	var avg24h float64
	var tlsDaysLeft *int
	var tlsNotAfter *time.Time
	if current != nil {
		if strings.TrimSpace(current.LastResultStatus) != "" {
			lastResult = strings.TrimSpace(current.LastResultStatus)
		}
		lastChecked = current.LastCheckedAt
		lastUp = current.LastUpAt
		lastDown = current.LastDownAt
		lastLatency = current.LastLatencyMs
		lastStatusCode = current.LastStatusCode
		lastError = current.LastError
		uptime24h = current.Uptime24h
		uptime30d = current.Uptime30d
		avg24h = current.AvgLatency24h
		tlsDaysLeft = current.TLSDaysLeft
		tlsNotAfter = current.TLSNotAfter
	}
	if !paused {
		status = strings.ToLower(strings.TrimSpace(lastResult))
		if status != "up" && status != "down" {
			if lastUp != nil && (lastDown == nil || lastUp.After(*lastDown)) {
				status = "up"
			} else {
				status = "down"
			}
		}
		mon, err := s.GetMonitor(ctx, id)
		if err != nil {
			return err
		}
		if mon != nil {
			list, err := s.ActiveMaintenanceFor(ctx, id, mon.Tags, now)
			if err != nil {
				return err
			}
			if len(list) > 0 {
				status = "maintenance"
				maintenanceActive = 1
			}
		}
	}
	return s.UpsertMonitorState(ctx, &MonitorState{
		MonitorID:         id,
		Status:            status,
		LastResultStatus:  lastResult,
		MaintenanceActive: maintenanceActive == 1,
		LastCheckedAt:     lastChecked,
		LastUpAt:          lastUp,
		LastDownAt:        lastDown,
		LastLatencyMs:     lastLatency,
		LastStatusCode:    lastStatusCode,
		LastError:         lastError,
		Uptime24h:         uptime24h,
		Uptime30d:         uptime30d,
		AvgLatency24h:     avg24h,
		TLSDaysLeft:       tlsDaysLeft,
		TLSNotAfter:       tlsNotAfter,
	})
}

func scanMonitor(row interface {
	Scan(dest ...any) error
}) (*Monitor, error) {
	var m Monitor
	var headersRaw, allowedRaw, tagsRaw string
	var isActive, isPaused, autoIncident, autoTaskOnDown, ignoreTLS, notifyTLS int
	var groupID sql.NullInt64
	var sla sql.NullFloat64
	if err := row.Scan(
		&m.ID, &m.Name, &m.Type, &m.URL, &m.Host, &m.Port, &m.Method, &m.RequestBody, &m.RequestBodyType, &headersRaw,
		&m.IntervalSec, &m.TimeoutSec, &m.Retries, &m.RetryIntervalSec, &allowedRaw, &ignoreTLS, &notifyTLS, &isActive, &isPaused,
		&tagsRaw, &groupID, &sla, &autoIncident, &autoTaskOnDown, &m.IncidentSeverity, &m.IncidentTypeID, &m.CreatedBy, &m.CreatedAt, &m.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	m.IgnoreTLSErrors = ignoreTLS == 1
	m.NotifyTLSExpiring = notifyTLS == 1
	m.IsActive = isActive == 1
	m.IsPaused = isPaused == 1
	m.AutoIncident = autoIncident == 1
	m.AutoTaskOnDown = autoTaskOnDown == 1
	if headersRaw != "" {
		_ = json.Unmarshal([]byte(headersRaw), &m.Headers)
	}
	if allowedRaw != "" {
		_ = json.Unmarshal([]byte(allowedRaw), &m.AllowedStatus)
	}
	if tagsRaw != "" {
		_ = json.Unmarshal([]byte(tagsRaw), &m.Tags)
	}
	if groupID.Valid {
		m.GroupID = &groupID.Int64
	}
	if sla.Valid {
		val := sla.Float64
		m.SLATargetPct = &val
	}
	return &m, nil
}

func scanMonitorSummary(rows *sql.Rows) (MonitorSummary, error) {
	var m MonitorSummary
	var headersRaw, allowedRaw, tagsRaw string
	var isActive, isPaused, autoIncident, autoTaskOnDown, ignoreTLS, notifyTLS int
	var groupID sql.NullInt64
	var sla sql.NullFloat64
	var status sql.NullString
	var lastChecked, lastUp, lastDown sql.NullTime
	var lastLatency, lastStatus sql.NullInt64
	if err := rows.Scan(
		&m.ID, &m.Name, &m.Type, &m.URL, &m.Host, &m.Port, &m.Method, &m.RequestBody, &m.RequestBodyType, &headersRaw,
		&m.IntervalSec, &m.TimeoutSec, &m.Retries, &m.RetryIntervalSec, &allowedRaw, &ignoreTLS, &notifyTLS, &isActive, &isPaused,
		&tagsRaw, &groupID, &sla, &autoIncident, &autoTaskOnDown, &m.IncidentSeverity, &m.IncidentTypeID, &m.CreatedBy, &m.CreatedAt, &m.UpdatedAt,
		&status, &lastChecked, &lastUp, &lastDown, &lastLatency, &lastStatus, &m.LastError); err != nil {
		return m, err
	}
	m.IgnoreTLSErrors = ignoreTLS == 1
	m.NotifyTLSExpiring = notifyTLS == 1
	m.IsActive = isActive == 1
	m.IsPaused = isPaused == 1
	m.AutoIncident = autoIncident == 1
	m.AutoTaskOnDown = autoTaskOnDown == 1
	if headersRaw != "" {
		_ = json.Unmarshal([]byte(headersRaw), &m.Headers)
	}
	if allowedRaw != "" {
		_ = json.Unmarshal([]byte(allowedRaw), &m.AllowedStatus)
	}
	if tagsRaw != "" {
		_ = json.Unmarshal([]byte(tagsRaw), &m.Tags)
	}
	if groupID.Valid {
		m.GroupID = &groupID.Int64
	}
	if sla.Valid {
		val := sla.Float64
		m.SLATargetPct = &val
	}
	if status.Valid {
		m.Status = status.String
	}
	if lastChecked.Valid {
		m.LastCheckedAt = &lastChecked.Time
	}
	if lastUp.Valid {
		m.LastUpAt = &lastUp.Time
	}
	if lastDown.Valid {
		m.LastDownAt = &lastDown.Time
	}
	if lastLatency.Valid {
		val := int(lastLatency.Int64)
		m.LastLatencyMs = &val
	}
	if lastStatus.Valid {
		val := int(lastStatus.Int64)
		m.LastStatusCode = &val
	}
	return m, nil
}

func normalizeMonitorTags(tags []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, t := range tags {
		val := strings.ToUpper(strings.TrimSpace(t))
		if val == "" {
			continue
		}
		if _, ok := seen[val]; ok {
			continue
		}
		seen[val] = struct{}{}
		out = append(out, val)
	}
	return out
}

func normalizeHeaders(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(v)
	}
	return out
}

func normalizeStatusRanges(in []string) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, raw := range in {
		val := strings.TrimSpace(raw)
		if val == "" {
			continue
		}
		if _, ok := seen[val]; ok {
			continue
		}
		seen[val] = struct{}{}
		out = append(out, val)
	}
	return out
}
