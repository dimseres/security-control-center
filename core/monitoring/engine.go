package monitoring

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"berkut-scc/core/store"
	"berkut-scc/core/utils"
	"berkut-scc/tasks"
)

type Engine struct {
	store             store.MonitoringStore
	incidents         store.IncidentsStore
	audits            store.AuditStore
	encryptor         *utils.Encryptor
	sender            TelegramSender
	incidentRegFormat string
	taskStore         tasks.Store
	logger            *utils.Logger
	cancel            context.CancelFunc
	running           bool
	wg                sync.WaitGroup
	mu                sync.Mutex
	inFlight          map[int64]struct{}
	sem               chan struct{}
	maxConcurrent     int
	lastSettingsAt    time.Time
	settings          store.MonitorSettings
	lastCleanupAt     time.Time
	lastMaintenanceAt time.Time
	lastSLAAt         time.Time
}

func NewEngine(store store.MonitoringStore, logger *utils.Logger) *Engine {
	return NewEngineWithDeps(store, nil, nil, "", nil, nil, logger)
}

func NewEngineWithDeps(store store.MonitoringStore, incidents store.IncidentsStore, audits store.AuditStore, regFormat string, encryptor *utils.Encryptor, sender TelegramSender, logger *utils.Logger) *Engine {
	return &Engine{
		store:             store,
		incidents:         incidents,
		audits:            audits,
		incidentRegFormat: regFormat,
		encryptor:         encryptor,
		sender:            sender,
		logger:            logger,
		inFlight:          map[int64]struct{}{},
	}
}

func (e *Engine) SetTaskStore(taskStore tasks.Store) {
	if e == nil {
		return
	}
	e.taskStore = taskStore
}

func (e *Engine) Start() {
	e.StartWithContext(context.Background())
}

func (e *Engine) StartWithContext(ctx context.Context) {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	e.running = true
	e.wg.Add(1)
	e.mu.Unlock()
	go e.loop(runCtx)
}

func (e *Engine) Stop() {
	_ = e.StopWithContext(context.Background())
}

func (e *Engine) StopWithContext(ctx context.Context) error {
	e.mu.Lock()
	if e.cancel == nil || !e.running {
		e.mu.Unlock()
		return nil
	}
	cancel := e.cancel
	e.cancel = nil
	e.mu.Unlock()
	cancel()
	waitDone := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
		e.mu.Lock()
		e.running = false
		e.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *Engine) CheckNow(ctx context.Context, monitorID int64) error {
	m, err := e.store.GetMonitor(ctx, monitorID)
	if err != nil || m == nil {
		return errors.New("common.notFound")
	}
	if TypeIsPassive(m.Type) {
		return errors.New("monitoring.error.passiveMonitor")
	}
	if m.IsPaused {
		return errors.New("monitoring.error.paused")
	}
	settings := e.currentSettingsFresh(ctx)
	if !settings.EngineEnabled {
		return errors.New("monitoring.error.engineDisabled")
	}
	if !e.acquireSlot(m.ID) {
		return errors.New("monitoring.error.busy")
	}
	defer e.releaseSlot(m.ID)
	checkTimeout := e.manualCheckTimeout(*m, settings)
	checkCtx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	manual := *m
	// Manual check should be responsive for UI: single attempt with hard deadline.
	manual.Retries = 0
	manual.RetryIntervalSec = 0
	return e.runCheck(checkCtx, manual, settings)
}

func (e *Engine) InvalidateSettings() {
	e.mu.Lock()
	e.lastSettingsAt = time.Time{}
	e.mu.Unlock()
}

func (e *Engine) manualCheckTimeout(m store.Monitor, settings store.MonitorSettings) time.Duration {
	timeoutSec := m.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = settings.DefaultTimeoutSec
	}
	if timeoutSec <= 0 {
		timeoutSec = 20
	}
	if timeoutSec > 20 {
		timeoutSec = 20
	}
	// Small grace for network close/write and DB update.
	return time.Duration(timeoutSec+2) * time.Second
}

func (e *Engine) loop(ctx context.Context) {
	defer e.wg.Done()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			settings := e.currentSettings(ctx)
			if !settings.EngineEnabled {
				continue
			}
			e.ensureSemaphore(settings.MaxConcurrentChecks)
			e.runDueChecks(ctx, settings)
			e.runMaintenance(ctx, settings)
			e.runRetention(ctx, settings)
			e.runSLAEvaluator(ctx, settings)
		case <-ctx.Done():
			return
		}
	}
}

func (e *Engine) currentSettings(ctx context.Context) store.MonitorSettings {
	e.mu.Lock()
	needFetch := e.settings.ID == 0 || time.Since(e.lastSettingsAt) > 10*time.Second
	e.mu.Unlock()
	if needFetch {
		if settings, err := e.store.GetSettings(ctx); err == nil && settings != nil {
			e.mu.Lock()
			e.settings = *settings
			e.lastSettingsAt = time.Now().UTC()
			e.mu.Unlock()
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.settings
}

func (e *Engine) currentSettingsFresh(ctx context.Context) store.MonitorSettings {
	if e.store == nil {
		return e.currentSettings(ctx)
	}
	settings, err := e.store.GetSettings(ctx)
	if err == nil && settings != nil {
		e.mu.Lock()
		e.settings = *settings
		e.lastSettingsAt = time.Now().UTC()
		e.mu.Unlock()
		return *settings
	}
	return e.currentSettings(ctx)
}

func (e *Engine) ensureSemaphore(max int) {
	if max <= 0 {
		max = 1
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.sem != nil && e.maxConcurrent == max {
		return
	}
	e.sem = make(chan struct{}, max)
	e.maxConcurrent = max
}

func (e *Engine) runDueChecks(ctx context.Context, settings store.MonitorSettings) {
	list, err := e.store.ListDueMonitors(ctx, time.Now().UTC())
	if err != nil {
		if e.logger != nil {
			e.logger.Errorf("monitoring due checks: %v", err)
		}
		return
	}
	for _, m := range list {
		if TypeIsPassive(m.Type) {
			continue
		}
		if !e.acquireSlot(m.ID) {
			continue
		}
		go func(mon store.Monitor) {
			defer e.releaseSlot(mon.ID)
			_ = e.runCheck(ctx, mon, settings)
		}(m)
	}
}

func (e *Engine) runRetention(ctx context.Context, settings store.MonitorSettings) {
	if settings.RetentionDays <= 0 {
		return
	}
	e.mu.Lock()
	last := e.lastCleanupAt
	e.mu.Unlock()
	if !last.IsZero() && time.Since(last) < time.Hour {
		return
	}
	before := time.Now().UTC().Add(-time.Duration(settings.RetentionDays) * 24 * time.Hour)
	if _, err := e.store.DeleteMetricsBefore(ctx, before); err != nil && e.logger != nil {
		e.logger.Errorf("monitoring retention: %v", err)
	}
	e.mu.Lock()
	e.lastCleanupAt = time.Now().UTC()
	e.mu.Unlock()
}

func (e *Engine) acquireSlot(id int64) bool {
	e.mu.Lock()
	if _, ok := e.inFlight[id]; ok {
		e.mu.Unlock()
		return false
	}
	e.inFlight[id] = struct{}{}
	sem := e.sem
	e.mu.Unlock()
	if sem == nil {
		return true
	}
	select {
	case sem <- struct{}{}:
		return true
	default:
		e.mu.Lock()
		delete(e.inFlight, id)
		e.mu.Unlock()
		return false
	}
}

func (e *Engine) releaseSlot(id int64) {
	e.mu.Lock()
	delete(e.inFlight, id)
	sem := e.sem
	e.mu.Unlock()
	if sem == nil {
		return
	}
	select {
	case <-sem:
	default:
	}
}

func (e *Engine) runCheck(ctx context.Context, m store.Monitor, settings store.MonitorSettings) error {
	result := CheckMonitor(ctx, m, settings)
	if result.CheckedAt.IsZero() {
		result.CheckedAt = time.Now().UTC()
	}
	var statusCode *int
	if result.StatusCode != nil {
		val := *result.StatusCode
		statusCode = &val
	}
	var errText *string
	if result.Error != "" {
		val := result.Error
		errText = &val
	}
	_, err := e.store.AddMetric(ctx, &store.MonitorMetric{
		MonitorID:  m.ID,
		TS:         result.CheckedAt,
		LatencyMs:  result.LatencyMs,
		OK:         result.OK,
		StatusCode: statusCode,
		Error:      errText,
	})
	if err != nil && e.logger != nil {
		e.logger.Errorf("monitoring add metric: %v", err)
	}
	tlsRecord := e.updateTLS(ctx, m, result, settings)
	return e.updateState(ctx, m, result, tlsRecord, settings)
}

func (e *Engine) updateState(ctx context.Context, m store.Monitor, result CheckResult, tlsRecord *store.MonitorTLS, settings store.MonitorSettings) error {
	prev, _ := e.store.GetMonitorState(ctx, m.ID)
	rawStatus := "down"
	if result.OK {
		rawStatus = "up"
	}
	now := result.CheckedAt
	maintenanceActive := false
	if list, err := e.store.ActiveMaintenanceFor(ctx, m.ID, m.Tags, now); err == nil && len(list) > 0 {
		maintenanceActive = true
	}
	status := rawStatus
	if m.IsPaused {
		status = "paused"
	} else if maintenanceActive {
		status = "maintenance"
	}
	next := &store.MonitorState{
		MonitorID:         m.ID,
		Status:            status,
		LastResultStatus:  rawStatus,
		MaintenanceActive: maintenanceActive,
		LastCheckedAt:     &now,
		LastError:         result.Error,
	}
	if result.StatusCode != nil {
		val := *result.StatusCode
		next.LastStatusCode = &val
	}
	if result.LatencyMs > 0 {
		val := result.LatencyMs
		next.LastLatencyMs = &val
	}
	if rawStatus == "up" {
		next.LastUpAt = &now
	} else {
		next.LastDownAt = &now
	}
	if prev != nil {
		if next.LastUpAt == nil {
			next.LastUpAt = prev.LastUpAt
		}
		if next.LastDownAt == nil {
			next.LastDownAt = prev.LastDownAt
		}
		shouldLog := false
		if prev.LastCheckedAt == nil || prev.LastResultStatus == "" {
			shouldLog = rawStatus == "down"
		} else if prev.LastResultStatus != rawStatus {
			shouldLog = true
		} else if rawStatus == "down" {
			prevCode := prev.LastStatusCode
			currCode := result.StatusCode
			codeChanged := (prevCode == nil) != (currCode == nil)
			if prevCode != nil && currCode != nil && *prevCode != *currCode {
				codeChanged = true
			}
			if result.Error != "" && (prev.LastError != result.Error || codeChanged) {
				shouldLog = true
			}
		}
		if shouldLog {
			msg := result.Error
			if msg == "" && result.StatusCode != nil {
				msg = "status_" + strconv.Itoa(*result.StatusCode)
			}
			_, _ = e.store.AddEvent(ctx, &store.MonitorEvent{
				MonitorID: m.ID,
				TS:        now,
				EventType: rawStatus,
				Message:   msg,
			})
		}
		if prev.MaintenanceActive != maintenanceActive {
			eventType := "maintenance_start"
			if !maintenanceActive {
				eventType = "maintenance_end"
			}
			_, _ = e.store.AddEvent(ctx, &store.MonitorEvent{
				MonitorID: m.ID,
				TS:        now,
				EventType: eventType,
				Message:   "",
			})
		}
	}
	if tlsRecord != nil {
		next.TLSNotAfter = &tlsRecord.NotAfter
		days := int(time.Until(tlsRecord.NotAfter).Hours() / 24)
		next.TLSDaysLeft = &days
		if settings.TLSExpiringDays > 0 {
			prevDays := 99999
			if prev != nil && prev.TLSDaysLeft != nil {
				prevDays = *prev.TLSDaysLeft
			}
			if days <= settings.TLSExpiringDays && prevDays > settings.TLSExpiringDays {
				_, _ = e.store.AddEvent(ctx, &store.MonitorEvent{
					MonitorID: m.ID,
					TS:        now,
					EventType: "tls_expiring",
					Message:   "monitoring.event.tlsExpiring",
				})
			}
		}
	} else if prev != nil {
		next.TLSNotAfter = prev.TLSNotAfter
		next.TLSDaysLeft = prev.TLSDaysLeft
	}
	if err := e.fillStats(ctx, m.ID, next, now); err != nil && e.logger != nil {
		e.logger.Errorf("monitoring stats: %v", err)
	}
	e.handleAutomation(ctx, m, prev, next, result, tlsRecord, settings)
	return e.store.UpsertMonitorState(ctx, next)
}

func (e *Engine) fillStats(ctx context.Context, monitorID int64, st *store.MonitorState, now time.Time) error {
	ok24, total24, avg24, err := e.store.MetricsSummary(ctx, monitorID, now.Add(-24*time.Hour))
	if err != nil {
		return err
	}
	ok30, total30, _, err := e.store.MetricsSummary(ctx, monitorID, now.Add(-30*24*time.Hour))
	if err != nil {
		return err
	}
	if total24 > 0 {
		st.Uptime24h = (float64(ok24) / float64(total24)) * 100
	} else {
		st.Uptime24h = 0
	}
	if total30 > 0 {
		st.Uptime30d = (float64(ok30) / float64(total30)) * 100
	} else {
		st.Uptime30d = 0
	}
	st.AvgLatency24h = avg24
	return nil
}

func (e *Engine) runMaintenance(ctx context.Context, settings store.MonitorSettings) {
	e.mu.Lock()
	last := e.lastMaintenanceAt
	e.mu.Unlock()
	if !last.IsZero() && time.Since(last) < time.Minute {
		return
	}
	items, err := e.store.ListMonitors(ctx, store.MonitorFilter{})
	if err != nil {
		return
	}
	now := time.Now().UTC()
	for _, mon := range items {
		state, err := e.store.GetMonitorState(ctx, mon.ID)
		if err != nil || state == nil {
			continue
		}
		active := false
		if list, err := e.store.ActiveMaintenanceFor(ctx, mon.ID, mon.Tags, now); err == nil && len(list) > 0 {
			active = true
		}
		rawStatus := state.LastResultStatus
		if rawStatus == "" {
			rawStatus = "down"
		}
		display := rawStatus
		if mon.IsPaused {
			display = "paused"
		} else if active {
			display = "maintenance"
		}
		prevMaint := state.MaintenanceActive
		if prevMaint == active && state.Status == display {
			continue
		}
		state.MaintenanceActive = active
		state.Status = display
		if prevMaint != active {
			eventType := "maintenance_start"
			if !active {
				eventType = "maintenance_end"
			}
			_, _ = e.store.AddEvent(ctx, &store.MonitorEvent{
				MonitorID: mon.ID,
				TS:        now,
				EventType: eventType,
				Message:   "",
			})
		}
		_ = e.store.UpsertMonitorState(ctx, state)
	}
	e.mu.Lock()
	e.lastMaintenanceAt = time.Now().UTC()
	e.mu.Unlock()
}

func (e *Engine) updateTLS(ctx context.Context, m store.Monitor, result CheckResult, settings store.MonitorSettings) *store.MonitorTLS {
	kind := strings.ToLower(strings.TrimSpace(m.Type))
	if !TypeSupportsTLSMetadata(kind) {
		return nil
	}
	if !(strings.HasPrefix(strings.ToLower(strings.TrimSpace(m.URL)), "https://") || strings.HasPrefix(strings.ToLower(strings.TrimSpace(m.URL)), "grpcs://")) {
		return nil
	}
	now := result.CheckedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if result.TLS != nil {
		if settings.TLSRefreshHours > 0 {
			if existing, _ := e.store.GetTLS(ctx, m.ID); existing != nil {
				if time.Since(existing.CheckedAt) < time.Duration(settings.TLSRefreshHours)*time.Hour {
					return existing
				}
			}
		}
		record := &store.MonitorTLS{
			MonitorID:         m.ID,
			CheckedAt:         now,
			NotAfter:          result.TLS.NotAfter,
			NotBefore:         result.TLS.NotBefore,
			CommonName:        result.TLS.CommonName,
			Issuer:            result.TLS.Issuer,
			SANs:              result.TLS.SANs,
			FingerprintSHA256: result.TLS.FingerprintSHA256,
			LastError:         nil,
		}
		_ = e.store.UpsertTLS(ctx, record)
		return record
	}
	if result.Error == "monitoring.error.tlsHandshakeFailed" {
		if existing, _ := e.store.GetTLS(ctx, m.ID); existing != nil {
			val := result.Error
			existing.LastError = &val
			existing.CheckedAt = now
			_ = e.store.UpsertTLS(ctx, existing)
			return existing
		}
	}
	return nil
}
