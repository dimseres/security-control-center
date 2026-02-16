package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"berkut-scc/core/store"
)

type TelegramMessage struct {
	Token          string
	ChatID         string
	ThreadID       *int64
	Text           string
	Silent         bool
	ProtectContent bool
}

type TelegramSender interface {
	Send(ctx context.Context, msg TelegramMessage) error
}

type HTTPTelegramSender struct {
	client  *http.Client
	baseURL string
}

func NewHTTPTelegramSender() *HTTPTelegramSender {
	return &HTTPTelegramSender{
		client:  &http.Client{Timeout: 10 * time.Second},
		baseURL: "https://api.telegram.org",
	}
}

func (s *HTTPTelegramSender) Send(ctx context.Context, msg TelegramMessage) error {
	if strings.TrimSpace(msg.Token) == "" || strings.TrimSpace(msg.ChatID) == "" {
		return errors.New("telegram token or chat id missing")
	}
	body := map[string]any{
		"chat_id":              msg.ChatID,
		"text":                 msg.Text,
		"disable_notification": msg.Silent,
		"protect_content":      msg.ProtectContent,
	}
	if msg.ThreadID != nil {
		body["message_thread_id"] = *msg.ThreadID
	}
	raw, _ := json.Marshal(body)
	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", strings.TrimRight(s.baseURL, "/"), msg.Token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("telegram api status %d", resp.StatusCode)
}

func (e *Engine) TestTelegram(ctx context.Context, msg TelegramMessage) error {
	if e == nil || e.sender == nil {
		return errors.New("telegram sender unavailable")
	}
	return e.sender.Send(ctx, msg)
}

func (e *Engine) TestTLSNotification(ctx context.Context, monitorID int64) error {
	if e == nil || e.sender == nil || e.encryptor == nil {
		return errors.New("telegram sender unavailable")
	}
	if e.store == nil {
		return errors.New("monitor store unavailable")
	}
	mon, err := e.store.GetMonitor(ctx, monitorID)
	if err != nil || mon == nil {
		return errors.New("common.notFound")
	}
	channels, err := e.resolveNotificationChannels(ctx, monitorID)
	if err != nil || len(channels) == 0 {
		return errors.New("monitoring.notifications.testFailed")
	}
	now := time.Now().UTC()
	tlsRecord := &store.MonitorTLS{
		MonitorID:  mon.ID,
		CheckedAt:  now,
		NotBefore:  now.Add(-24 * time.Hour),
		NotAfter:   now.Add(30 * 24 * time.Hour),
		CommonName: strings.TrimSpace(mon.Name),
		Issuer:     "Test CA",
	}
	msg := buildNotificationMessage("tls_expiring", "ru", *mon, CheckResult{}, tlsRecord, now, false)
	if !e.dispatchNotification(ctx, channels, msg, "tls_expiring", &mon.ID) {
		return errors.New("monitoring.notifications.testFailed")
	}
	return nil
}

func (e *Engine) handleAutomation(ctx context.Context, m store.Monitor, prev, next *store.MonitorState, result CheckResult, tlsRecord *store.MonitorTLS, settings store.MonitorSettings) {
	if next == nil {
		return
	}
	now := time.Now().UTC()
	if next.LastCheckedAt != nil && !next.LastCheckedAt.IsZero() {
		now = next.LastCheckedAt.UTC()
	}
	rawStatus := strings.ToLower(strings.TrimSpace(next.LastResultStatus))
	if rawStatus == "" {
		rawStatus = "down"
	}
	st, _ := e.store.GetNotificationState(ctx, m.ID)
	if st == nil {
		st = &store.MonitorNotificationState{MonitorID: m.ID}
	}
	if rawStatus == "down" {
		if prev == nil || strings.ToLower(strings.TrimSpace(prev.LastResultStatus)) != "down" {
			st.DownStartedAt = &now
			st.DownSequence = 1
		} else {
			st.DownSequence++
		}
	} else {
		st.DownStartedAt = nil
		st.DownSequence = 0
	}
	e.handleNotifications(ctx, m, prev, next, rawStatus, now, st, tlsRecord, result, settings)
	e.handleAutoTaskOnDown(ctx, m, prev, next, now)
	e.handleAutoTLSIncident(ctx, m, prev, next, tlsRecord, now, settings)
	e.handleAutoIncident(ctx, m, prev, next, rawStatus, now, settings)
	_ = e.store.UpsertNotificationState(ctx, st)
}

func (e *Engine) handleNotifications(ctx context.Context, m store.Monitor, prev, next *store.MonitorState, rawStatus string, now time.Time, st *store.MonitorNotificationState, tlsRecord *store.MonitorTLS, result CheckResult, settings store.MonitorSettings) {
	if e.sender == nil || e.encryptor == nil {
		return
	}
	if m.IsPaused {
		return
	}
	maintenanceChanged := prev != nil && prev.MaintenanceActive != next.MaintenanceActive
	if next.MaintenanceActive && !maintenanceChanged {
		return
	}
	channels, err := e.resolveNotificationChannels(ctx, m.ID)
	if err != nil || len(channels) == 0 {
		return
	}
	suppress := time.Duration(settings.NotifySuppressMinutes) * time.Minute
	if suppress < 0 {
		suppress = 0
	}
	canSend := func(last *time.Time) bool {
		if suppress == 0 {
			return true
		}
		if last == nil {
			return true
		}
		return now.Sub(last.UTC()) >= suppress
	}
	canNotifyDownOutage := func() bool {
		// Send DOWN only once per outage window until an UP notification is sent.
		if st.LastDownNotifiedAt == nil {
			return true
		}
		if st.LastUpNotifiedAt == nil {
			return false
		}
		return st.LastDownNotifiedAt.Before(st.LastUpNotifiedAt.UTC())
	}
	canNotifyUpRecover := func() bool {
		// Send UP only when there was a previously notified DOWN in the current outage cycle.
		if st.LastDownNotifiedAt == nil {
			return false
		}
		if st.LastUpNotifiedAt == nil {
			return true
		}
		return st.LastDownNotifiedAt.After(st.LastUpNotifiedAt.UTC())
	}
	if maintenanceChanged && settings.NotifyMaintenance && canSend(st.LastNotifiedAt) && canSend(st.LastMaintenanceNotifiedAt) {
		kind := "maintenance_start"
		if !next.MaintenanceActive {
			kind = "maintenance_end"
		}
		if e.dispatchNotification(ctx, channels, buildNotificationMessage(kind, "ru", m, result, tlsRecord, now, st.DownSequence > 1), kind, &m.ID) {
			st.LastNotifiedAt = &now
			st.LastMaintenanceNotifiedAt = &now
		}
		return
	}
	if rawStatus == "down" &&
		(prev == nil || prev.LastCheckedAt == nil || strings.ToLower(strings.TrimSpace(prev.LastResultStatus)) != "down") &&
		canNotifyDownOutage() &&
		canSend(st.LastNotifiedAt) &&
		canSend(st.LastDownNotifiedAt) {
		if e.dispatchNotification(ctx, channels, buildNotificationMessage("down", "ru", m, result, tlsRecord, now, st.DownSequence > 1), "down", &m.ID) {
			st.LastNotifiedAt = &now
			st.LastDownNotifiedAt = &now
		}
		return
	}
	if rawStatus == "up" &&
		prev != nil &&
		strings.ToLower(strings.TrimSpace(prev.LastResultStatus)) == "down" &&
		canNotifyUpRecover() &&
		canSend(st.LastNotifiedAt) &&
		canSend(st.LastUpNotifiedAt) {
		if e.dispatchNotification(ctx, channels, buildNotificationMessage("up", "ru", m, result, tlsRecord, now, false), "up", &m.ID) {
			st.LastNotifiedAt = &now
			st.LastUpNotifiedAt = &now
		}
		return
	}
	if next.MaintenanceActive {
		return
	}
	if m.NotifyTLSExpiring && tlsRecord != nil && next.TLSDaysLeft != nil && settings.TLSExpiringDays > 0 {
		threshold := settings.TLSExpiringDays
		prevDays := 99999
		if prev != nil && prev.TLSDaysLeft != nil {
			prevDays = *prev.TLSDaysLeft
		}
		if *next.TLSDaysLeft <= threshold && prevDays > threshold && canSend(st.LastNotifiedAt) && canSend(st.LastTLSNotifiedAt) {
			if e.dispatchNotification(ctx, channels, buildNotificationMessage("tls_expiring", "ru", m, result, tlsRecord, now, false), "tls_expiring", &m.ID) {
				st.LastNotifiedAt = &now
				st.LastTLSNotifiedAt = &now
			}
		}
	}
}

func (e *Engine) dispatchNotification(ctx context.Context, channels []store.NotificationChannel, msg TelegramMessage, eventType string, monitorID *int64) bool {
	sent := false
	baseText := msg.Text
	for _, ch := range channels {
		if strings.ToLower(strings.TrimSpace(ch.Type)) != "telegram" || !ch.IsActive {
			continue
		}
		if isQuietHours(ch, time.Now().UTC()) {
			e.logNotificationDelivery(ctx, store.MonitorNotificationDelivery{
				MonitorID:             monitorID,
				NotificationChannelID: ch.ID,
				EventType:             eventType,
				Status:                "suppressed",
				Error:                 "quiet_hours",
				BodyPreview:           previewMessage(msg.Text),
			})
			continue
		}
		tokenRaw, err := e.encryptor.DecryptBlob(ch.TelegramBotTokenEnc)
		if err != nil {
			if e.logger != nil {
				e.logger.Errorf("monitoring decrypt token: %v", err)
			}
			e.logNotificationDelivery(ctx, store.MonitorNotificationDelivery{
				MonitorID:             monitorID,
				NotificationChannelID: ch.ID,
				EventType:             eventType,
				Status:                "failed",
				Error:                 "decrypt_failed",
				BodyPreview:           previewMessage(msg.Text),
			})
			continue
		}
		msg.Token = string(tokenRaw)
		msg.ChatID = ch.TelegramChatID
		msg.ThreadID = ch.TelegramThreadID
		msg.Silent = ch.Silent
		msg.ProtectContent = ch.ProtectContent
		msg.Text = applyNotificationTemplate(ch.TemplateText, baseText)
		if err := e.sender.Send(ctx, msg); err != nil {
			if e.logger != nil {
				e.logger.Errorf("monitoring telegram send: %v", err)
			}
			e.logNotificationDelivery(ctx, store.MonitorNotificationDelivery{
				MonitorID:             monitorID,
				NotificationChannelID: ch.ID,
				EventType:             eventType,
				Status:                "failed",
				Error:                 err.Error(),
				BodyPreview:           previewMessage(msg.Text),
			})
			continue
		}
		e.logNotificationDelivery(ctx, store.MonitorNotificationDelivery{
			MonitorID:             monitorID,
			NotificationChannelID: ch.ID,
			EventType:             eventType,
			Status:                "sent",
			BodyPreview:           previewMessage(msg.Text),
		})
		sent = true
	}
	return sent
}

func (e *Engine) logNotificationDelivery(ctx context.Context, item store.MonitorNotificationDelivery) {
	if e == nil || e.store == nil {
		return
	}
	if _, err := e.store.AddNotificationDelivery(ctx, &item); err != nil && e.logger != nil {
		e.logger.Errorf("monitoring delivery log: %v", err)
	}
}

func (e *Engine) resolveNotificationChannels(ctx context.Context, monitorID int64) ([]store.NotificationChannel, error) {
	links, err := e.store.ListMonitorNotifications(ctx, monitorID)
	if err != nil {
		return nil, err
	}
	var res []store.NotificationChannel
	if len(links) > 0 {
		seen := map[int64]struct{}{}
		for _, link := range links {
			if !link.Enabled {
				continue
			}
			if _, ok := seen[link.NotificationChannelID]; ok {
				continue
			}
			seen[link.NotificationChannelID] = struct{}{}
			ch, err := e.store.GetNotificationChannel(ctx, link.NotificationChannelID)
			if err != nil || ch == nil {
				continue
			}
			if ch.IsActive {
				res = append(res, *ch)
			}
		}
		return res, nil
	}
	defaults, err := e.store.ListDefaultNotificationChannels(ctx)
	if err != nil {
		return nil, err
	}
	for _, ch := range defaults {
		if ch.IsActive {
			res = append(res, ch)
		}
	}
	return res, nil
}

func buildNotificationMessage(kind, lang string, m store.Monitor, result CheckResult, tlsRecord *store.MonitorTLS, now time.Time, repeatDown bool) TelegramMessage {
	title := notifyText(lang, "monitoring.notify.downTitle")
	switch kind {
	case "up":
		title = notifyText(lang, "monitoring.notify.upTitle")
	case "tls_expiring":
		title = notifyText(lang, "monitoring.notify.tlsTitle")
	case "maintenance_start":
		title = notifyText(lang, "monitoring.notify.maintenanceStartTitle")
	case "maintenance_end":
		title = notifyText(lang, "monitoring.notify.maintenanceEndTitle")
	}
	lines := []string{title}
	if repeatDown && kind == "down" {
		lines = append(lines, notifyText(lang, "monitoring.notify.repeatDown"))
	}
	lines = append(lines, strings.TrimSpace(m.Name))
	lines = append(lines, monitorTarget(m))
	if kind == "tls_expiring" && tlsRecord != nil {
		lines = append(lines, fmt.Sprintf("%s: %s", notifyText(lang, "monitoring.notify.expires"), formatNotifyTime(tlsRecord.NotAfter)))
		days := int(time.Until(tlsRecord.NotAfter).Hours() / 24)
		lines = append(lines, fmt.Sprintf("%s: %d", notifyText(lang, "monitoring.notify.daysLeft"), days))
	} else if result.LatencyMs > 0 {
		lines = append(lines, fmt.Sprintf("%s: %d ms", notifyText(lang, "monitoring.notify.latency"), result.LatencyMs))
	}
	lines = append(lines, fmt.Sprintf("%s: %s", notifyText(lang, "monitoring.notify.time"), formatNotifyTime(now)))
	lines = append(lines, "")
	lines = append(lines, notifyText(lang, "monitoring.notify.footer"))
	return TelegramMessage{Text: strings.Join(lines, "\n")}
}

func formatNotifyTime(t time.Time) string {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		loc = time.FixedZone("MSK", 3*60*60)
	}
	return t.In(loc).Format("02.01.2006 15:04")
}

func monitorTarget(m store.Monitor) string {
	if strings.ToLower(strings.TrimSpace(m.Type)) == "tcp" {
		return fmt.Sprintf("%s:%d", strings.TrimSpace(m.Host), m.Port)
	}
	return strings.TrimSpace(m.URL)
}

func notifyText(lang, key string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	ru := map[string]string{
		"monitoring.notify.downTitle":             "\U0001F6A8 Монитор недоступен",
		"monitoring.notify.upTitle":               "\u2705 Монитор восстановлен",
		"monitoring.notify.tlsTitle":              "\u26A0\ufe0f Истекает сертификат",
		"monitoring.notify.maintenanceStartTitle": "\U0001F6E0\ufe0f Начало обслуживания",
		"monitoring.notify.maintenanceEndTitle":   "\u2705 Обслуживание завершено",
		"monitoring.notify.repeatDown":            "\u26A0\ufe0f повторное падение",
		"monitoring.notify.testTitle":             "\u2705 Тестовое уведомление",
		"monitoring.notify.latency":               "Задержка",
		"monitoring.notify.time":                  "Время",
		"monitoring.notify.expires":               "Истекает",
		"monitoring.notify.daysLeft":              "Дней осталось",
		"monitoring.notify.footer":                "Berkut SCC",
	}
	en := map[string]string{
		"monitoring.notify.downTitle":             "\U0001F6A8 Monitor down",
		"monitoring.notify.upTitle":               "\u2705 Monitor recovered",
		"monitoring.notify.tlsTitle":              "\u26A0\ufe0f TLS certificate expiring",
		"monitoring.notify.maintenanceStartTitle": "\U0001F6E0\ufe0f Maintenance started",
		"monitoring.notify.maintenanceEndTitle":   "\u2705 Maintenance ended",
		"monitoring.notify.repeatDown":            "\u26A0\ufe0f repeated outage",
		"monitoring.notify.testTitle":             "\u2705 Test notification",
		"monitoring.notify.latency":               "Latency",
		"monitoring.notify.time":                  "Time",
		"monitoring.notify.expires":               "Expires",
		"monitoring.notify.daysLeft":              "Days left",
		"monitoring.notify.footer":                "Berkut SCC",
	}
	if lang == "ru" {
		if v, ok := ru[key]; ok {
			return v
		}
	}
	if v, ok := en[key]; ok {
		return v
	}
	return key
}

func applyNotificationTemplate(templateText, text string) string {
	tpl := strings.TrimSpace(templateText)
	if tpl == "" {
		return text
	}
	return strings.ReplaceAll(tpl, "{message}", text)
}

func previewMessage(text string) string {
	raw := strings.TrimSpace(text)
	if len(raw) <= 240 {
		return raw
	}
	return raw[:240]
}

func isQuietHours(ch store.NotificationChannel, now time.Time) bool {
	if !ch.QuietHoursEnabled {
		return false
	}
	start := strings.TrimSpace(ch.QuietHoursStart)
	end := strings.TrimSpace(ch.QuietHoursEnd)
	if start == "" || end == "" {
		return false
	}
	loc := time.UTC
	if tz := strings.TrimSpace(ch.QuietHoursTZ); tz != "" {
		if parsed, err := time.LoadLocation(tz); err == nil {
			loc = parsed
		}
	}
	current := now.In(loc)
	curMin := current.Hour()*60 + current.Minute()
	parseMinutes := func(v string) (int, bool) {
		parts := strings.Split(v, ":")
		if len(parts) != 2 {
			return 0, false
		}
		hh, errH := strconv.Atoi(parts[0])
		mm, errM := strconv.Atoi(parts[1])
		if errH != nil || errM != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
			return 0, false
		}
		return hh*60 + mm, true
	}
	startMin, ok1 := parseMinutes(start)
	endMin, ok2 := parseMinutes(end)
	if !ok1 || !ok2 {
		return false
	}
	if startMin == endMin {
		return true
	}
	if startMin < endMin {
		return curMin >= startMin && curMin < endMin
	}
	return curMin >= startMin || curMin < endMin
}

func NotifyTestMessage(lang string) string {
	lines := []string{
		notifyText(lang, "monitoring.notify.testTitle"),
		"",
		notifyText(lang, "monitoring.notify.footer"),
	}
	return strings.Join(lines, "\n")
}
