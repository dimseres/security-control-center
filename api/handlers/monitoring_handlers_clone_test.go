package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"berkut-scc/config"
	"berkut-scc/core/auth"
	"berkut-scc/core/store"
	"berkut-scc/core/utils"
	"github.com/go-chi/chi/v5"
)

func setupMonitoringHandlerTestDB(t *testing.T) (store.MonitoringStore, func()) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.AppConfig{DBPath: filepath.Join(dir, "monitoring_handlers.db")}
	logger := utils.NewLogger()
	db, err := store.NewDB(cfg, logger)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	if err := store.ApplyMigrations(context.Background(), db, logger); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return store.NewMonitoringStore(db), func() { _ = db.Close() }
}

func TestCloneMonitor_CopiesNotificationBindings(t *testing.T) {
	ms, cleanup := setupMonitoringHandlerTestDB(t)
	defer cleanup()

	ch1 := &store.NotificationChannel{
		Type:                "telegram",
		Name:                "Ops",
		TelegramBotTokenEnc: []byte("x"),
		TelegramChatID:      "1",
		IsDefault:           false,
		IsActive:            true,
		CreatedBy:           1,
	}
	ch2 := &store.NotificationChannel{
		Type:                "telegram",
		Name:                "NOC",
		TelegramBotTokenEnc: []byte("y"),
		TelegramChatID:      "2",
		IsDefault:           true,
		IsActive:            true,
		CreatedBy:           1,
	}
	ch1ID, err := ms.CreateNotificationChannel(context.Background(), ch1)
	if err != nil {
		t.Fatalf("create channel 1: %v", err)
	}
	ch2ID, err := ms.CreateNotificationChannel(context.Background(), ch2)
	if err != nil {
		t.Fatalf("create channel 2: %v", err)
	}

	mon := &store.Monitor{
		Name:          "A",
		Type:          "http",
		URL:           "http://example.com",
		Method:        "GET",
		AllowedStatus: []string{"200-299"},
		IntervalSec:   60,
		TimeoutSec:    2,
		IsActive:      true,
		CreatedBy:     1,
	}
	monID, err := ms.CreateMonitor(context.Background(), mon)
	if err != nil {
		t.Fatalf("create monitor: %v", err)
	}
	if err := ms.ReplaceMonitorNotifications(context.Background(), monID, []store.MonitorNotification{
		{MonitorID: monID, NotificationChannelID: ch1ID, Enabled: true},
		{MonitorID: monID, NotificationChannelID: ch2ID, Enabled: false},
	}); err != nil {
		t.Fatalf("replace notifications: %v", err)
	}

	h := NewMonitoringHandler(ms, nil, nil, nil, nil)
	req := httptest.NewRequest("POST", "/api/monitoring/monitors/"+strconv.FormatInt(monID, 10)+"/clone", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.SessionContextKey, &store.SessionRecord{UserID: 123, Username: "u"}))
	req = withChiURLParam(req, "id", strconv.FormatInt(monID, 10))
	rec := httptest.NewRecorder()
	h.CloneMonitor(rec, req)
	if rec.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var cloned store.Monitor
	if err := json.Unmarshal(rec.Body.Bytes(), &cloned); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if cloned.ID <= 0 {
		t.Fatalf("expected cloned id")
	}
	if cloned.ID == monID {
		t.Fatalf("expected new id")
	}
	got, err := ms.ListMonitorNotifications(context.Background(), cloned.ID)
	if err != nil {
		t.Fatalf("list cloned notifications: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 bindings, got %d", len(got))
	}
	m := map[int64]bool{}
	for _, item := range got {
		m[item.NotificationChannelID] = item.Enabled
	}
	if m[ch1ID] != true {
		t.Fatalf("expected channel 1 enabled")
	}
	if m[ch2ID] != false {
		t.Fatalf("expected channel 2 disabled")
	}
}

func TestCloneMonitor_PassiveMonitorGetsNewToken(t *testing.T) {
	ms, cleanup := setupMonitoringHandlerTestDB(t)
	defer cleanup()

	mon := &store.Monitor{
		Name:        "Push",
		Type:        "push",
		RequestBody: "token-original",
		IntervalSec: 60,
		TimeoutSec:  2,
		IsActive:    true,
		CreatedBy:   1,
	}
	monID, err := ms.CreateMonitor(context.Background(), mon)
	if err != nil {
		t.Fatalf("create monitor: %v", err)
	}

	h := NewMonitoringHandler(ms, nil, nil, nil, nil)
	req := httptest.NewRequest("POST", "/api/monitoring/monitors/"+strconv.FormatInt(monID, 10)+"/clone", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.SessionContextKey, &store.SessionRecord{UserID: 123, Username: "u"}))
	req = withChiURLParam(req, "id", strconv.FormatInt(monID, 10))
	rec := httptest.NewRecorder()
	h.CloneMonitor(rec, req)
	if rec.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var cloned store.Monitor
	if err := json.Unmarshal(rec.Body.Bytes(), &cloned); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if cloned.ID <= 0 || cloned.ID == monID {
		t.Fatalf("expected new id")
	}
	if cloned.RequestBody == "" {
		t.Fatalf("expected cloned token to be set")
	}
	if cloned.RequestBody == mon.RequestBody {
		t.Fatalf("expected cloned token to differ")
	}
}

func withChiURLParam(req *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}
