package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

func (s *monitoringStore) ListMaintenance(ctx context.Context, filter MaintenanceFilter) ([]MonitorMaintenance, error) {
	query := `
		SELECT id, name, description_md, monitor_id, monitor_ids_json, tags_json, starts_at, ends_at, timezone,
			strategy, strategy_json, is_recurring, rrule_text, created_by, created_at, updated_at, is_active, stopped_at, stopped_by
		FROM monitor_maintenance`
	var clauses []string
	var args []any
	if filter.Active != nil {
		clauses = append(clauses, "is_active=?")
		args = append(args, boolToInt(*filter.Active))
	}
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY created_at DESC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []MonitorMaintenance
	for rows.Next() {
		item, err := scanMaintenance(rows)
		if err != nil {
			return nil, err
		}
		if filter.MonitorID != nil && !maintenanceAppliesToMonitor(*item, *filter.MonitorID, nil) {
			continue
		}
		res = append(res, *item)
	}
	return res, rows.Err()
}

func (s *monitoringStore) GetMaintenance(ctx context.Context, id int64) (*MonitorMaintenance, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, description_md, monitor_id, monitor_ids_json, tags_json, starts_at, ends_at, timezone,
			strategy, strategy_json, is_recurring, rrule_text, created_by, created_at, updated_at, is_active, stopped_at, stopped_by
		FROM monitor_maintenance WHERE id=?`, id)
	item, err := scanMaintenance(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return item, nil
}

func (s *monitoringStore) CreateMaintenance(ctx context.Context, m *MonitorMaintenance) (int64, error) {
	now := time.Now().UTC()
	normalized := normalizeMaintenanceModel(m)
	tagsJSON := tagsToJSON(normalizeMonitorTags(normalized.Tags))
	monitorIDsJSON := int64SliceToJSON(normalized.MonitorIDs)
	strategyJSON := maintenanceScheduleToJSON(normalized.Schedule)
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO monitor_maintenance(
			name, description_md, monitor_id, monitor_ids_json, tags_json, starts_at, ends_at, timezone,
			strategy, strategy_json, is_recurring, rrule_text, created_by, created_at, updated_at, is_active, stopped_at, stopped_by
		)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		strings.TrimSpace(normalized.Name), strings.TrimSpace(normalized.DescriptionMD), nullableID(normalized.MonitorID), monitorIDsJSON, tagsJSON,
		normalized.StartsAt, normalized.EndsAt, strings.TrimSpace(normalized.Timezone),
		normalized.Strategy, strategyJSON, boolToInt(normalized.IsRecurring), strings.TrimSpace(normalized.RRuleText),
		normalized.CreatedBy, now, now, boolToInt(normalized.IsActive), nullTime(normalized.StoppedAt), nullableID(normalized.StoppedBy))
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if m != nil {
		m.ID = id
		m.CreatedAt = now
		m.UpdatedAt = now
	}
	return id, nil
}

func (s *monitoringStore) UpdateMaintenance(ctx context.Context, m *MonitorMaintenance) error {
	normalized := normalizeMaintenanceModel(m)
	tagsJSON := tagsToJSON(normalizeMonitorTags(normalized.Tags))
	monitorIDsJSON := int64SliceToJSON(normalized.MonitorIDs)
	strategyJSON := maintenanceScheduleToJSON(normalized.Schedule)
	_, err := s.db.ExecContext(ctx, `
		UPDATE monitor_maintenance
		SET
			name=?, description_md=?, monitor_id=?, monitor_ids_json=?, tags_json=?, starts_at=?, ends_at=?, timezone=?,
			strategy=?, strategy_json=?, is_recurring=?, rrule_text=?, updated_at=?, is_active=?, stopped_at=?, stopped_by=?
		WHERE id=?`,
		strings.TrimSpace(normalized.Name), strings.TrimSpace(normalized.DescriptionMD), nullableID(normalized.MonitorID), monitorIDsJSON, tagsJSON,
		normalized.StartsAt, normalized.EndsAt, strings.TrimSpace(normalized.Timezone),
		normalized.Strategy, strategyJSON, boolToInt(normalized.IsRecurring), strings.TrimSpace(normalized.RRuleText),
		time.Now().UTC(), boolToInt(normalized.IsActive), nullTime(normalized.StoppedAt), nullableID(normalized.StoppedBy), normalized.ID)
	return err
}

func (s *monitoringStore) StopMaintenance(ctx context.Context, id int64, stoppedBy int64) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE monitor_maintenance
		SET is_active=0, stopped_at=?, stopped_by=?, updated_at=?
		WHERE id=?`, now, nullableID(&stoppedBy), now, id)
	return err
}

func (s *monitoringStore) DeleteMaintenance(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM monitor_maintenance WHERE id=?`, id)
	return err
}

func (s *monitoringStore) ActiveMaintenanceFor(ctx context.Context, monitorID int64, tags []string, now time.Time) ([]MonitorMaintenance, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, description_md, monitor_id, monitor_ids_json, tags_json, starts_at, ends_at, timezone,
			strategy, strategy_json, is_recurring, rrule_text, created_by, created_at, updated_at, is_active, stopped_at, stopped_by
		FROM monitor_maintenance WHERE is_active=1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []MonitorMaintenance
	for rows.Next() {
		item, err := scanMaintenance(rows)
		if err != nil {
			return nil, err
		}
		if !maintenanceAppliesToMonitor(*item, monitorID, tags) {
			continue
		}
		if maintenanceActiveAt(*item, now) {
			res = append(res, *item)
		}
	}
	return res, rows.Err()
}

func (s *monitoringStore) MaintenanceWindowsFor(ctx context.Context, monitorID int64, tags []string, since, until time.Time) ([]MaintenanceWindow, error) {
	if !until.After(since) {
		return nil, nil
	}
	items, err := s.ListMaintenance(ctx, MaintenanceFilter{Active: boolPtr(true)})
	if err != nil {
		return nil, err
	}
	windows := make([]MaintenanceWindow, 0, len(items))
	for _, item := range items {
		if !maintenanceAppliesToMonitor(item, monitorID, tags) {
			continue
		}
		ranges := maintenanceWindowsWithin(item, since, until)
		for _, rng := range ranges {
			windows = append(windows, MaintenanceWindow{Start: rng.Start, End: rng.End})
		}
	}
	return mergeMaintenanceWindows(windows), nil
}

func scanMaintenance(row interface {
	Scan(dest ...any) error
}) (*MonitorMaintenance, error) {
	var m MonitorMaintenance
	var tagsRaw sql.NullString
	var monitorIDsRaw sql.NullString
	var monitorID sql.NullInt64
	var strategyJSON sql.NullString
	var recurring, active int
	var stoppedAt sql.NullTime
	var stoppedBy sql.NullInt64
	if err := row.Scan(
		&m.ID, &m.Name, &m.DescriptionMD, &monitorID, &monitorIDsRaw, &tagsRaw, &m.StartsAt, &m.EndsAt, &m.Timezone,
		&m.Strategy, &strategyJSON, &recurring, &m.RRuleText, &m.CreatedBy, &m.CreatedAt, &m.UpdatedAt, &active, &stoppedAt, &stoppedBy,
	); err != nil {
		return nil, err
	}
	if tagsRaw.Valid && tagsRaw.String != "" {
		_ = json.Unmarshal([]byte(tagsRaw.String), &m.Tags)
	}
	if monitorIDsRaw.Valid && monitorIDsRaw.String != "" {
		_ = json.Unmarshal([]byte(monitorIDsRaw.String), &m.MonitorIDs)
	}
	if monitorID.Valid {
		m.MonitorID = &monitorID.Int64
	}
	if strategyJSON.Valid && strings.TrimSpace(strategyJSON.String) != "" {
		_ = json.Unmarshal([]byte(strategyJSON.String), &m.Schedule)
	}
	m.IsRecurring = recurring == 1
	m.IsActive = active == 1
	if stoppedAt.Valid {
		m.StoppedAt = &stoppedAt.Time
	}
	if stoppedBy.Valid {
		val := stoppedBy.Int64
		m.StoppedBy = &val
	}
	norm := normalizeMaintenanceModel(&m)
	return &norm, nil
}

func maintenanceAppliesToMonitor(m MonitorMaintenance, monitorID int64, tags []string) bool {
	if len(m.MonitorIDs) > 0 {
		for _, id := range m.MonitorIDs {
			if id == monitorID {
				return true
			}
		}
		return false
	}
	if m.MonitorID != nil {
		return *m.MonitorID == monitorID
	}
	if len(m.Tags) > 0 {
		tagSet := map[string]struct{}{}
		for _, t := range normalizeMonitorTags(tags) {
			tagSet[t] = struct{}{}
		}
		return hasAnyTag(tagSet, m.Tags)
	}
	return true
}

func hasAnyTag(set map[string]struct{}, tags []string) bool {
	for _, t := range normalizeMonitorTags(tags) {
		if _, ok := set[t]; ok {
			return true
		}
	}
	return false
}

func normalizeMaintenanceModel(in *MonitorMaintenance) MonitorMaintenance {
	var out MonitorMaintenance
	if in != nil {
		out = *in
	}
	out.Name = strings.TrimSpace(out.Name)
	out.DescriptionMD = strings.TrimSpace(out.DescriptionMD)
	out.Timezone = strings.TrimSpace(out.Timezone)
	if out.Timezone == "" {
		out.Timezone = "UTC"
	}
	if out.Strategy == "" {
		if out.IsRecurring {
			out.Strategy = maintenanceStrategyRRule
		} else {
			out.Strategy = maintenanceStrategySingle
		}
	}
	out.Strategy = strings.ToLower(strings.TrimSpace(out.Strategy))
	out.MonitorIDs = normalizeMonitorIDs(out.MonitorIDs)
	if out.StartsAt.IsZero() {
		out.StartsAt = time.Now().UTC()
	}
	if out.EndsAt.IsZero() || !out.EndsAt.After(out.StartsAt) {
		out.EndsAt = out.StartsAt.Add(time.Hour)
	}
	if out.Strategy != maintenanceStrategyRRule {
		out.IsRecurring = out.Strategy != maintenanceStrategySingle
	}
	if out.Strategy != maintenanceStrategyRRule {
		out.RRuleText = ""
	}
	if !out.IsActive {
		now := time.Now().UTC()
		if out.StoppedAt == nil {
			out.StoppedAt = &now
		}
	} else {
		out.StoppedAt = nil
		out.StoppedBy = nil
	}
	out.Schedule = normalizeMaintenanceSchedule(out.Schedule, out.Strategy)
	return out
}

func normalizeMaintenanceSchedule(in MaintenanceSchedule, strategy string) MaintenanceSchedule {
	out := in
	out.CronExpression = strings.TrimSpace(out.CronExpression)
	out.WindowStart = normalizeHHMM(out.WindowStart)
	out.WindowEnd = normalizeHHMM(out.WindowEnd)
	if out.DurationMin < 0 {
		out.DurationMin = 0
	}
	if out.IntervalDays < 0 {
		out.IntervalDays = 0
	}
	if out.IntervalDays == 0 && strategy == maintenanceStrategyInterval {
		out.IntervalDays = 1
	}
	if out.DurationMin == 0 && strategy == maintenanceStrategyCron {
		out.DurationMin = 60
	}
	out.Weekdays = normalizeWeekdays(out.Weekdays)
	out.MonthDays = normalizeMonthDays(out.MonthDays)
	if out.ActiveFrom != nil {
		val := out.ActiveFrom.UTC()
		out.ActiveFrom = &val
	}
	if out.ActiveUntil != nil {
		val := out.ActiveUntil.UTC()
		out.ActiveUntil = &val
	}
	return out
}
