package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"berkut-scc/core/monitoring"
	"berkut-scc/core/store"
)

type notificationChannelPayload struct {
	Type              string `json:"type"`
	Name              string `json:"name"`
	TelegramBotToken  string `json:"telegram_bot_token"`
	TelegramChatID    string `json:"telegram_chat_id"`
	TelegramThreadID  *int64 `json:"telegram_thread_id"`
	TemplateText      string `json:"template_text"`
	QuietHoursEnabled bool   `json:"quiet_hours_enabled"`
	QuietHoursStart   string `json:"quiet_hours_start"`
	QuietHoursEnd     string `json:"quiet_hours_end"`
	QuietHoursTZ      string `json:"quiet_hours_tz"`
	Silent            bool   `json:"silent"`
	ProtectContent    bool   `json:"protect_content"`
	IsDefault         bool   `json:"is_default"`
	IsActive          *bool  `json:"is_active"`
	ApplyToAll        bool   `json:"apply_to_all"`
}

type notificationChannelView struct {
	ID                int64  `json:"id"`
	Type              string `json:"type"`
	Name              string `json:"name"`
	TelegramBotToken  string `json:"telegram_bot_token"`
	TelegramChatID    string `json:"telegram_chat_id"`
	TelegramThreadID  *int64 `json:"telegram_thread_id,omitempty"`
	TemplateText      string `json:"template_text"`
	QuietHoursEnabled bool   `json:"quiet_hours_enabled"`
	QuietHoursStart   string `json:"quiet_hours_start"`
	QuietHoursEnd     string `json:"quiet_hours_end"`
	QuietHoursTZ      string `json:"quiet_hours_tz"`
	Silent            bool   `json:"silent"`
	ProtectContent    bool   `json:"protect_content"`
	IsDefault         bool   `json:"is_default"`
	CreatedBy         int64  `json:"created_by"`
	CreatedAt         string `json:"created_at"`
	IsActive          bool   `json:"is_active"`
}

type notificationTokenView struct {
	TelegramBotToken string `json:"telegram_bot_token"`
}

func (h *MonitoringHandler) ListNotificationChannels(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.ListNotificationChannels(r.Context())
	if err != nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	var out []notificationChannelView
	for _, ch := range items {
		tokenValue := ""
		if len(ch.TelegramBotTokenEnc) > 0 {
			tokenValue = "******"
		}
		out = append(out, notificationChannelView{
			ID:                ch.ID,
			Type:              ch.Type,
			Name:              ch.Name,
			TelegramBotToken:  tokenValue,
			TelegramChatID:    ch.TelegramChatID,
			TelegramThreadID:  ch.TelegramThreadID,
			TemplateText:      ch.TemplateText,
			QuietHoursEnabled: ch.QuietHoursEnabled,
			QuietHoursStart:   ch.QuietHoursStart,
			QuietHoursEnd:     ch.QuietHoursEnd,
			QuietHoursTZ:      ch.QuietHoursTZ,
			Silent:            ch.Silent,
			ProtectContent:    ch.ProtectContent,
			IsDefault:         ch.IsDefault,
			CreatedBy:         ch.CreatedBy,
			CreatedAt:         ch.CreatedAt.UTC().Format(timeLayout),
			IsActive:          ch.IsActive,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *MonitoringHandler) CreateNotificationChannel(w http.ResponseWriter, r *http.Request) {
	if h.encryptor == nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	var payload notificationChannelPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, errBadRequest, http.StatusBadRequest)
		return
	}
	if strings.ToLower(strings.TrimSpace(payload.Type)) != "telegram" {
		http.Error(w, "monitoring.notifications.invalidType", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(payload.Name) == "" {
		http.Error(w, "monitoring.notifications.nameRequired", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(payload.TelegramBotToken) == "" || strings.TrimSpace(payload.TelegramChatID) == "" {
		http.Error(w, "monitoring.notifications.telegramRequired", http.StatusBadRequest)
		return
	}
	if err := validateQuietHoursPayload(payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	enc, err := h.encryptor.EncryptToBlob([]byte(strings.TrimSpace(payload.TelegramBotToken)))
	if err != nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	isActive := true
	if payload.IsActive != nil {
		isActive = *payload.IsActive
	}
	ch := &store.NotificationChannel{
		Type:                "telegram",
		Name:                strings.TrimSpace(payload.Name),
		TelegramBotTokenEnc: enc,
		TelegramChatID:      strings.TrimSpace(payload.TelegramChatID),
		TelegramThreadID:    payload.TelegramThreadID,
		TemplateText:        strings.TrimSpace(payload.TemplateText),
		QuietHoursEnabled:   payload.QuietHoursEnabled,
		QuietHoursStart:     strings.TrimSpace(payload.QuietHoursStart),
		QuietHoursEnd:       strings.TrimSpace(payload.QuietHoursEnd),
		QuietHoursTZ:        strings.TrimSpace(payload.QuietHoursTZ),
		Silent:              payload.Silent,
		ProtectContent:      payload.ProtectContent,
		IsDefault:           payload.IsDefault,
		IsActive:            isActive,
		CreatedBy:           sessionUserID(r),
	}
	id, err := h.store.CreateNotificationChannel(r.Context(), ch)
	if err != nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	applyCount := 0
	if payload.ApplyToAll {
		applyCount = h.applyChannelToAllMonitors(r.Context(), id)
		h.audit(r, monitorAuditNotifChannelApplyAll, strconv.FormatInt(id, 10)+"|"+strconv.Itoa(applyCount))
	}
	h.audit(r, monitorAuditNotifChannelCreate, strconv.FormatInt(id, 10))
	writeJSON(w, http.StatusCreated, notificationChannelView{
		ID:                id,
		Type:              ch.Type,
		Name:              ch.Name,
		TelegramBotToken:  maskToken(payload.TelegramBotToken),
		TelegramChatID:    ch.TelegramChatID,
		TelegramThreadID:  ch.TelegramThreadID,
		TemplateText:      ch.TemplateText,
		QuietHoursEnabled: ch.QuietHoursEnabled,
		QuietHoursStart:   ch.QuietHoursStart,
		QuietHoursEnd:     ch.QuietHoursEnd,
		QuietHoursTZ:      ch.QuietHoursTZ,
		Silent:            ch.Silent,
		ProtectContent:    ch.ProtectContent,
		IsDefault:         ch.IsDefault,
		CreatedBy:         ch.CreatedBy,
		CreatedAt:         ch.CreatedAt.UTC().Format(timeLayout),
		IsActive:          ch.IsActive,
	})
}

func (h *MonitoringHandler) UpdateNotificationChannel(w http.ResponseWriter, r *http.Request) {
	if h.encryptor == nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	id, err := parseID(pathParams(r)["id"])
	if err != nil {
		http.Error(w, errBadRequest, http.StatusBadRequest)
		return
	}
	existing, err := h.store.GetNotificationChannel(r.Context(), id)
	if err != nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	if existing == nil {
		http.Error(w, errNotFound, http.StatusNotFound)
		return
	}
	var payload notificationChannelPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, errBadRequest, http.StatusBadRequest)
		return
	}
	if payload.Type != "" && strings.ToLower(strings.TrimSpace(payload.Type)) != "telegram" {
		http.Error(w, "monitoring.notifications.invalidType", http.StatusBadRequest)
		return
	}
	if err := validateQuietHoursPayload(payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(payload.Name) != "" {
		existing.Name = strings.TrimSpace(payload.Name)
	}
	if payload.TelegramChatID != "" {
		existing.TelegramChatID = strings.TrimSpace(payload.TelegramChatID)
	}
	if payload.TelegramThreadID != nil {
		existing.TelegramThreadID = payload.TelegramThreadID
	}
	if payload.TemplateText != "" || existing.TemplateText != "" {
		existing.TemplateText = strings.TrimSpace(payload.TemplateText)
	}
	existing.QuietHoursEnabled = payload.QuietHoursEnabled
	if payload.QuietHoursStart != "" || existing.QuietHoursStart != "" {
		existing.QuietHoursStart = strings.TrimSpace(payload.QuietHoursStart)
	}
	if payload.QuietHoursEnd != "" || existing.QuietHoursEnd != "" {
		existing.QuietHoursEnd = strings.TrimSpace(payload.QuietHoursEnd)
	}
	if payload.QuietHoursTZ != "" || existing.QuietHoursTZ != "" {
		existing.QuietHoursTZ = strings.TrimSpace(payload.QuietHoursTZ)
	}
	if payload.TelegramBotToken != "" {
		enc, err := h.encryptor.EncryptToBlob([]byte(strings.TrimSpace(payload.TelegramBotToken)))
		if err != nil {
			http.Error(w, errServerError, http.StatusInternalServerError)
			return
		}
		existing.TelegramBotTokenEnc = enc
	}
	existing.Silent = payload.Silent
	existing.ProtectContent = payload.ProtectContent
	existing.IsDefault = payload.IsDefault
	if payload.IsActive != nil {
		existing.IsActive = *payload.IsActive
	}
	if err := h.store.UpdateNotificationChannel(r.Context(), existing); err != nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	h.audit(r, monitorAuditNotifChannelUpdate, strconv.FormatInt(id, 10))
	tokenMasked := "******"
	if payload.TelegramBotToken != "" {
		tokenMasked = maskToken(payload.TelegramBotToken)
	}
	writeJSON(w, http.StatusOK, notificationChannelView{
		ID:                existing.ID,
		Type:              existing.Type,
		Name:              existing.Name,
		TelegramBotToken:  tokenMasked,
		TelegramChatID:    existing.TelegramChatID,
		TelegramThreadID:  existing.TelegramThreadID,
		TemplateText:      existing.TemplateText,
		QuietHoursEnabled: existing.QuietHoursEnabled,
		QuietHoursStart:   existing.QuietHoursStart,
		QuietHoursEnd:     existing.QuietHoursEnd,
		QuietHoursTZ:      existing.QuietHoursTZ,
		Silent:            existing.Silent,
		ProtectContent:    existing.ProtectContent,
		IsDefault:         existing.IsDefault,
		CreatedBy:         existing.CreatedBy,
		CreatedAt:         existing.CreatedAt.UTC().Format(timeLayout),
		IsActive:          existing.IsActive,
	})
}

func (h *MonitoringHandler) DeleteNotificationChannel(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(pathParams(r)["id"])
	if err != nil {
		http.Error(w, errBadRequest, http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteNotificationChannel(r.Context(), id); err != nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	h.audit(r, monitorAuditNotifChannelDelete, strconv.FormatInt(id, 10))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *MonitoringHandler) TestNotificationChannel(w http.ResponseWriter, r *http.Request) {
	if h.encryptor == nil || h.engine == nil {
		http.Error(w, errServiceUnavailable, http.StatusServiceUnavailable)
		return
	}
	id, err := parseID(pathParams(r)["id"])
	if err != nil {
		http.Error(w, errBadRequest, http.StatusBadRequest)
		return
	}
	ch, err := h.store.GetNotificationChannel(r.Context(), id)
	if err != nil || ch == nil {
		http.Error(w, errNotFound, http.StatusNotFound)
		return
	}
	tokenRaw, err := h.encryptor.DecryptBlob(ch.TelegramBotTokenEnc)
	if err != nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	msg := monitoring.TelegramMessage{
		Token:          string(tokenRaw),
		ChatID:         ch.TelegramChatID,
		ThreadID:       ch.TelegramThreadID,
		Text:           monitoring.NotifyTestMessage("ru"),
		Silent:         ch.Silent,
		ProtectContent: ch.ProtectContent,
	}
	if err := h.engine.TestTelegram(r.Context(), msg); err != nil {
		http.Error(w, "monitoring.notifications.testFailed", http.StatusBadRequest)
		return
	}
	h.audit(r, monitorAuditNotifChannelTest, strconv.FormatInt(id, 10))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *MonitoringHandler) RevealNotificationChannelToken(w http.ResponseWriter, r *http.Request) {
	if h.encryptor == nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	id, err := parseID(pathParams(r)["id"])
	if err != nil {
		http.Error(w, errBadRequest, http.StatusBadRequest)
		return
	}
	ch, err := h.store.GetNotificationChannel(r.Context(), id)
	if err != nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	if ch == nil {
		http.Error(w, errNotFound, http.StatusNotFound)
		return
	}
	tokenRaw, err := h.encryptor.DecryptBlob(ch.TelegramBotTokenEnc)
	if err != nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	h.audit(r, monitorAuditNotifChannelReveal, strconv.FormatInt(id, 10))
	writeJSON(w, http.StatusOK, notificationTokenView{TelegramBotToken: string(tokenRaw)})
}

func (h *MonitoringHandler) ListMonitorNotifications(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(pathParams(r)["id"])
	if err != nil {
		http.Error(w, errBadRequest, http.StatusBadRequest)
		return
	}
	items, err := h.store.ListMonitorNotifications(r.Context(), id)
	if err != nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *MonitoringHandler) UpdateMonitorNotifications(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(pathParams(r)["id"])
	if err != nil {
		http.Error(w, errBadRequest, http.StatusBadRequest)
		return
	}
	var payload struct {
		Items []store.MonitorNotification `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, errBadRequest, http.StatusBadRequest)
		return
	}
	for i := range payload.Items {
		payload.Items[i].MonitorID = id
	}
	if err := h.store.ReplaceMonitorNotifications(r.Context(), id, payload.Items); err != nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	h.audit(r, monitorAuditNotifBindingsUpdate, strconv.FormatInt(id, 10))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *MonitoringHandler) applyChannelToAllMonitors(ctx context.Context, channelID int64) int {
	list, err := h.store.ListMonitors(ctx, store.MonitorFilter{})
	if err != nil {
		return 0
	}
	applied := 0
	for _, mon := range list {
		existing, err := h.store.ListMonitorNotifications(ctx, mon.ID)
		if err != nil {
			continue
		}
		found := false
		for _, item := range existing {
			if item.NotificationChannelID == channelID {
				found = true
				break
			}
		}
		if found {
			continue
		}
		existing = append(existing, store.MonitorNotification{
			MonitorID:             mon.ID,
			NotificationChannelID: channelID,
			Enabled:               true,
		})
		if err := h.store.ReplaceMonitorNotifications(ctx, mon.ID, existing); err == nil {
			applied++
		}
	}
	return applied
}

func (h *MonitoringHandler) ListNotificationDeliveries(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}
	items, err := h.store.ListNotificationDeliveries(r.Context(), limit)
	if err != nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *MonitoringHandler) AcknowledgeNotificationDelivery(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(pathParams(r)["id"])
	if err != nil {
		http.Error(w, errBadRequest, http.StatusBadRequest)
		return
	}
	if err := h.store.AcknowledgeNotificationDelivery(r.Context(), id, sessionUserID(r)); err != nil {
		http.Error(w, errServerError, http.StatusInternalServerError)
		return
	}
	h.audit(r, "monitoring.notification.delivery.ack", strconv.FormatInt(id, 10))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

const timeLayout = "2006-01-02 15:04:05"

func maskToken(token string) string {
	raw := strings.TrimSpace(token)
	if raw == "" {
		return ""
	}
	if len(raw) <= 8 {
		return "******"
	}
	return raw[:4] + "..." + raw[len(raw)-4:]
}

func validateQuietHoursPayload(payload notificationChannelPayload) error {
	if !payload.QuietHoursEnabled {
		return nil
	}
	valid := func(raw string) bool {
		v := strings.TrimSpace(raw)
		if len(v) != 5 || v[2] != ':' {
			return false
		}
		hh, errH := strconv.Atoi(v[:2])
		mm, errM := strconv.Atoi(v[3:])
		return errH == nil && errM == nil && hh >= 0 && hh <= 23 && mm >= 0 && mm <= 59
	}
	if !valid(payload.QuietHoursStart) || !valid(payload.QuietHoursEnd) {
		return errors.New("monitoring.notifications.quietHoursInvalid")
	}
	return nil
}
