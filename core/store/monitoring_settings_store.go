package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func (s *monitoringStore) GetSettings(ctx context.Context) (*MonitorSettings, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, retention_days, max_concurrent_checks, default_timeout_sec, default_interval_sec, engine_enabled, allow_private_networks, tls_refresh_hours, tls_expiring_days, notify_suppress_minutes, notify_repeat_down_minutes, notify_maintenance, auto_task_on_down, auto_tls_incident, auto_tls_incident_days, auto_incident_close_on_up, default_retries, default_retry_interval_sec, default_sla_target_pct, updated_at
		FROM monitoring_settings ORDER BY id LIMIT 1`)
	var settings MonitorSettings
	var engineEnabled, allowPriv, notifyMaintenance, autoTaskOnDown, autoTLSIncident, autoIncidentCloseOnUp int
	if err := row.Scan(&settings.ID, &settings.RetentionDays, &settings.MaxConcurrentChecks, &settings.DefaultTimeoutSec, &settings.DefaultIntervalSec, &engineEnabled, &allowPriv, &settings.TLSRefreshHours, &settings.TLSExpiringDays, &settings.NotifySuppressMinutes, &settings.NotifyRepeatDownMinutes, &notifyMaintenance, &autoTaskOnDown, &autoTLSIncident, &settings.AutoTLSIncidentDays, &autoIncidentCloseOnUp, &settings.DefaultRetries, &settings.DefaultRetryIntervalSec, &settings.DefaultSLATargetPct, &settings.UpdatedAt); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		settings = defaultMonitoringSettings()
		if _, err := s.insertSettings(ctx, &settings); err != nil {
			return nil, err
		}
		return &settings, nil
	}
	settings.EngineEnabled = engineEnabled == 1
	settings.AllowPrivateNetworks = allowPriv == 1
	settings.NotifyMaintenance = notifyMaintenance == 1
	settings.AutoTaskOnDown = autoTaskOnDown == 1
	settings.AutoTLSIncident = autoTLSIncident == 1
	settings.AutoIncidentCloseOnUp = autoIncidentCloseOnUp == 1
	return &settings, nil
}

func (s *monitoringStore) UpdateSettings(ctx context.Context, settings *MonitorSettings) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE monitoring_settings
		SET retention_days=?, max_concurrent_checks=?, default_timeout_sec=?, default_interval_sec=?, engine_enabled=?, allow_private_networks=?, tls_refresh_hours=?, tls_expiring_days=?, notify_suppress_minutes=?, notify_repeat_down_minutes=?, notify_maintenance=?, auto_task_on_down=?, auto_tls_incident=?, auto_tls_incident_days=?, auto_incident_close_on_up=?, default_retries=?, default_retry_interval_sec=?, default_sla_target_pct=?, updated_at=?
		WHERE id=?`,
		settings.RetentionDays, settings.MaxConcurrentChecks, settings.DefaultTimeoutSec, settings.DefaultIntervalSec,
		boolToInt(settings.EngineEnabled), boolToInt(settings.AllowPrivateNetworks), settings.TLSRefreshHours, settings.TLSExpiringDays,
		settings.NotifySuppressMinutes, settings.NotifyRepeatDownMinutes, boolToInt(settings.NotifyMaintenance),
		boolToInt(settings.AutoTaskOnDown), boolToInt(settings.AutoTLSIncident), settings.AutoTLSIncidentDays, boolToInt(settings.AutoIncidentCloseOnUp),
		settings.DefaultRetries, settings.DefaultRetryIntervalSec, settings.DefaultSLATargetPct, now, settings.ID)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected > 0 {
		settings.UpdatedAt = now
		return nil
	}
	settings.UpdatedAt = now
	if _, err := s.insertSettings(ctx, settings); err != nil {
		return err
	}
	return nil
}

func (s *monitoringStore) insertSettings(ctx context.Context, settings *MonitorSettings) (int64, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO monitoring_settings(retention_days, max_concurrent_checks, default_timeout_sec, default_interval_sec, engine_enabled, allow_private_networks, tls_refresh_hours, tls_expiring_days, notify_suppress_minutes, notify_repeat_down_minutes, notify_maintenance, auto_task_on_down, auto_tls_incident, auto_tls_incident_days, auto_incident_close_on_up, default_retries, default_retry_interval_sec, default_sla_target_pct, updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		settings.RetentionDays, settings.MaxConcurrentChecks, settings.DefaultTimeoutSec, settings.DefaultIntervalSec,
		boolToInt(settings.EngineEnabled), boolToInt(settings.AllowPrivateNetworks), settings.TLSRefreshHours, settings.TLSExpiringDays,
		settings.NotifySuppressMinutes, settings.NotifyRepeatDownMinutes, boolToInt(settings.NotifyMaintenance),
		boolToInt(settings.AutoTaskOnDown), boolToInt(settings.AutoTLSIncident), settings.AutoTLSIncidentDays, boolToInt(settings.AutoIncidentCloseOnUp),
		settings.DefaultRetries, settings.DefaultRetryIntervalSec, settings.DefaultSLATargetPct, now)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	settings.ID = id
	settings.UpdatedAt = now
	return id, nil
}

func defaultMonitoringSettings() MonitorSettings {
	return MonitorSettings{
		RetentionDays:           30,
		MaxConcurrentChecks:     10,
		DefaultTimeoutSec:       20,
		DefaultIntervalSec:      30,
		EngineEnabled:           true,
		AllowPrivateNetworks:    false,
		TLSRefreshHours:         24,
		TLSExpiringDays:         30,
		NotifySuppressMinutes:   5,
		NotifyRepeatDownMinutes: 30,
		NotifyMaintenance:       false,
		AutoTaskOnDown:          true,
		AutoTLSIncident:         true,
		AutoTLSIncidentDays:     14,
		AutoIncidentCloseOnUp:   false,
		DefaultRetries:          2,
		DefaultRetryIntervalSec: 30,
		DefaultSLATargetPct:     90,
	}
}
