package monitoring

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"berkut-scc/core/store"
	"berkut-scc/tasks"
)

func (e *Engine) handleAutoTaskOnDown(ctx context.Context, m store.Monitor, prev, next *store.MonitorState, now time.Time) {
	if e.taskStore == nil || !m.AutoTaskOnDown {
		return
	}
	if next == nil || m.IsPaused || next.MaintenanceActive {
		return
	}
	prevStatus := monitorRawStatus(prev)
	if monitorRawStatus(next) != "down" || prevStatus == "down" {
		return
	}

	boardID, columnID, err := e.pickTaskDestination(ctx)
	if err != nil {
		if e.logger != nil {
			e.logger.Errorf("monitoring auto task destination: %v", err)
		}
		return
	}
	if boardID == 0 || columnID == 0 {
		return
	}

	actor := monitorActorID(m)
	title := fmt.Sprintf("Мониторинг: проверить недоступность %s", automationMonitorDisplayName(m))
	desc := buildDownTaskDescription(m, next, now)
	task := &tasks.Task{
		BoardID:     boardID,
		ColumnID:    columnID,
		Title:       title,
		Description: desc,
		Priority:    tasks.PriorityHigh,
		CreatedBy:   &actor,
	}
	if _, err := e.taskStore.CreateTask(ctx, task, nil); err != nil {
		if e.logger != nil {
			e.logger.Errorf("monitoring auto task create: %v", err)
		}
		return
	}
	if e.audits != nil {
		_ = e.audits.Log(ctx, "system", "monitoring.task.auto_create", fmt.Sprintf("monitor_id=%d|task_id=%d", m.ID, task.ID))
	}
	_, _ = e.store.AddEvent(ctx, &store.MonitorEvent{
		MonitorID: m.ID,
		TS:        now.UTC(),
		EventType: "task_created",
		Message:   fmt.Sprintf("task_id=%d", task.ID),
	})
}

func (e *Engine) handleAutoTLSIncident(ctx context.Context, m store.Monitor, prev, next *store.MonitorState, tlsRecord *store.MonitorTLS, now time.Time, settings store.MonitorSettings) {
	if e.incidents == nil || !settings.AutoTLSIncident || settings.AutoTLSIncidentDays <= 0 {
		return
	}
	if next == nil || m.IsPaused || next.MaintenanceActive || next.TLSDaysLeft == nil || tlsRecord == nil {
		return
	}

	threshold := settings.AutoTLSIncidentDays
	currentDays := *next.TLSDaysLeft
	prevDays := 99999
	if prev != nil && prev.TLSDaysLeft != nil {
		prevDays = *prev.TLSDaysLeft
	}
	src := "monitoring_tls"
	withinThreshold := currentDays <= threshold
	wasWithinThreshold := prevDays <= threshold

	if withinThreshold && !wasWithinThreshold {
		existing, _ := e.incidents.FindOpenIncidentBySource(ctx, src, m.ID)
		if existing != nil {
			return
		}
		owner := monitorActorID(m)
		title := fmt.Sprintf("TLS: истечение сертификата — %s", automationMonitorDisplayName(m))
		desc := fmt.Sprintf("Сертификат истекает через %d дн. (порог: %d).", currentDays, threshold)
		incident := &store.Incident{
			Title:       title,
			Description: desc,
			Severity:    tlsIncidentSeverity(currentDays),
			Status:      "open",
			OwnerUserID: owner,
			CreatedBy:   owner,
			UpdatedBy:   owner,
			Source:      src,
			SourceRefID: &m.ID,
			Meta: store.IncidentMeta{
				IncidentType:    "Риск истечения TLS",
				DetectionSource: "Мониторинг",
				DetectedAt:      now.UTC().Format(time.RFC3339),
				AffectedSystems: automationMonitorDisplayName(m),
				WhatHappened:    fmt.Sprintf("NotAfter: %s", tlsRecord.NotAfter.UTC().Format(time.RFC3339)),
			},
		}
		id, err := e.incidents.CreateIncident(ctx, incident, nil, nil, e.incidentRegFormat)
		if err != nil {
			if e.logger != nil {
				e.logger.Errorf("monitoring auto tls incident create: %v", err)
			}
			return
		}
		_, _ = e.incidents.AddIncidentTimeline(ctx, &store.IncidentTimelineEvent{
			IncidentID: id,
			EventType:  "monitoring.tls.auto_create",
			Message:    strconv.Itoa(currentDays),
			CreatedBy:  owner,
			EventAt:    now.UTC(),
		})
		if e.audits != nil {
			_ = e.audits.Log(ctx, "system", "monitoring.tls.incident.auto_create", fmt.Sprintf("incident_id=%d|monitor_id=%d", id, m.ID))
		}
		_, _ = e.store.AddEvent(ctx, &store.MonitorEvent{
			MonitorID: m.ID,
			TS:        now.UTC(),
			EventType: "tls_incident_open",
			Message:   fmt.Sprintf("incident_id=%d", id),
		})
		return
	}

	if !withinThreshold && wasWithinThreshold {
		existing, _ := e.incidents.FindOpenIncidentBySource(ctx, src, m.ID)
		if existing == nil {
			return
		}
		closed, err := e.incidents.CloseIncident(ctx, existing.ID, existing.OwnerUserID)
		if err != nil || closed == nil {
			if errors.Is(err, store.ErrConflict) {
				return
			}
			if e.logger != nil && err != nil {
				e.logger.Errorf("monitoring auto tls incident close: %v", err)
			}
			return
		}
		_, _ = e.incidents.AddIncidentTimeline(ctx, &store.IncidentTimelineEvent{
			IncidentID: existing.ID,
			EventType:  "monitoring.tls.auto_close",
			Message:    strconv.Itoa(currentDays),
			CreatedBy:  existing.OwnerUserID,
			EventAt:    now.UTC(),
		})
		if e.audits != nil {
			_ = e.audits.Log(ctx, "system", "monitoring.tls.incident.auto_close", fmt.Sprintf("incident_id=%d|monitor_id=%d", existing.ID, m.ID))
		}
		_, _ = e.store.AddEvent(ctx, &store.MonitorEvent{
			MonitorID: m.ID,
			TS:        now.UTC(),
			EventType: "tls_incident_close",
			Message:   fmt.Sprintf("incident_id=%d", existing.ID),
		})
	}
}

func (e *Engine) pickTaskDestination(ctx context.Context) (int64, int64, error) {
	boards, err := e.taskStore.ListBoards(ctx, tasks.BoardFilter{})
	if err != nil || len(boards) == 0 {
		return 0, 0, err
	}
	for _, board := range boards {
		columns, cErr := e.taskStore.ListColumns(ctx, board.ID, false)
		if cErr != nil || len(columns) == 0 {
			continue
		}
		for _, col := range columns {
			if !col.IsFinal && col.IsActive {
				return board.ID, col.ID, nil
			}
		}
		for _, col := range columns {
			if col.IsActive {
				return board.ID, col.ID, nil
			}
		}
	}
	return 0, 0, nil
}

func monitorRawStatus(st *store.MonitorState) string {
	if st == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(st.LastResultStatus))
}

func automationMonitorDisplayName(m store.Monitor) string {
	name := strings.TrimSpace(m.Name)
	if name != "" {
		return name
	}
	return fmt.Sprintf("Монитор #%d", m.ID)
}

func monitorActorID(m store.Monitor) int64 {
	if m.CreatedBy > 0 {
		return m.CreatedBy
	}
	return 1
}

func buildDownTaskDescription(m store.Monitor, next *store.MonitorState, now time.Time) string {
	lines := []string{
		"Автоматическая задача по событию мониторинга.",
		fmt.Sprintf("Монитор: %s", automationMonitorDisplayName(m)),
		fmt.Sprintf("Тип: %s", strings.ToUpper(strings.TrimSpace(m.Type))),
		fmt.Sprintf("Время: %s", now.UTC().Format(time.RFC3339)),
	}
	if m.URL != "" {
		lines = append(lines, fmt.Sprintf("URL: %s", strings.TrimSpace(m.URL)))
	}
	if m.Host != "" {
		lines = append(lines, fmt.Sprintf("Хост: %s", strings.TrimSpace(m.Host)))
	}
	if m.Port > 0 {
		lines = append(lines, fmt.Sprintf("Порт: %d", m.Port))
	}
	if next != nil && strings.TrimSpace(next.LastError) != "" {
		lines = append(lines, fmt.Sprintf("Ошибка: %s", strings.TrimSpace(next.LastError)))
	}
	if next != nil && next.LastStatusCode != nil {
		lines = append(lines, fmt.Sprintf("HTTP код: %d", *next.LastStatusCode))
	}
	return strings.Join(lines, "\n")
}

func tlsIncidentSeverity(daysLeft int) string {
	switch {
	case daysLeft <= 3:
		return "critical"
	case daysLeft <= 7:
		return "high"
	default:
		return "medium"
	}
}
