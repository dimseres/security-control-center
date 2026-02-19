package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"berkut-scc/config"
	"berkut-scc/core/monitoring"
	"berkut-scc/core/store"
	"berkut-scc/core/utils"
	"berkut-scc/tasks"
	taskstore "berkut-scc/tasks/store"
)

type mockTelegramSender struct {
	sent []monitoring.TelegramMessage
}

func (m *mockTelegramSender) Send(ctx context.Context, msg monitoring.TelegramMessage) error {
	m.sent = append(m.sent, msg)
	return nil
}

func setupMonitoringDeps(t *testing.T) (store.MonitoringStore, store.IncidentsStore, tasks.Store, *utils.Encryptor, func()) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.AppConfig{DBPath: filepath.Join(dir, "monitoring_notify.db")}
	logger := utils.NewLogger()
	db, err := store.NewDB(cfg, logger)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.ApplyMigrations(context.Background(), db, logger); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	enc, err := utils.NewEncryptorFromString("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("encryptor: %v", err)
	}
	return store.NewMonitoringStore(db), store.NewIncidentsStore(db), taskstore.NewStore(db), enc, func() { db.Close() }
}

func addTelegramChannel(t *testing.T, ms store.MonitoringStore, enc *utils.Encryptor) int64 {
	t.Helper()
	tokenEnc, err := enc.EncryptToBlob([]byte("test-token"))
	if err != nil {
		t.Fatalf("encrypt token: %v", err)
	}
	ch := &store.NotificationChannel{
		Type:                "telegram",
		Name:                "Ops",
		TelegramBotTokenEnc: tokenEnc,
		TelegramChatID:      "12345",
		Silent:              true,
		ProtectContent:      true,
		IsDefault:           true,
		IsActive:            true,
		CreatedBy:           1,
	}
	id, err := ms.CreateNotificationChannel(context.Background(), ch)
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}
	return id
}

func createTaskDestination(t *testing.T, ts tasks.Store) (int64, int64) {
	t.Helper()
	createdBy := int64(1)
	space := &tasks.Space{
		Name:        "SOC",
		Description: "Automation",
		CreatedBy:   &createdBy,
		IsActive:    true,
	}
	spaceID, err := ts.CreateSpace(context.Background(), space, nil)
	if err != nil {
		t.Fatalf("create space: %v", err)
	}
	board := &tasks.Board{
		SpaceID:     spaceID,
		Name:        "Ops",
		Description: "Ops board",
		CreatedBy:   &createdBy,
		IsActive:    true,
	}
	boardID, err := ts.CreateBoard(context.Background(), board, nil)
	if err != nil {
		t.Fatalf("create board: %v", err)
	}
	column := &tasks.Column{
		BoardID:  boardID,
		Name:     "Todo",
		Position: 1,
		IsFinal:  false,
		IsActive: true,
	}
	columnID, err := ts.CreateColumn(context.Background(), column)
	if err != nil {
		t.Fatalf("create column: %v", err)
	}
	return boardID, columnID
}

func TestMonitoringTelegramDownUp(t *testing.T) {
	ms, is, _, enc, cleanup := setupMonitoringDeps(t)
	defer cleanup()
	settings, _ := ms.GetSettings(context.Background())
	settings.AllowPrivateNetworks = true
	settings.EngineEnabled = true
	settings.NotifySuppressMinutes = 0
	settings.NotifyRepeatDownMinutes = 10
	if err := ms.UpdateSettings(context.Background(), settings); err != nil {
		t.Fatalf("settings update: %v", err)
	}
	addTelegramChannel(t, ms, enc)
	var code int32 = 500
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(int(atomic.LoadInt32(&code)))
	}))
	defer srv.Close()
	mon := &store.Monitor{
		Name:          "Down monitor",
		Type:          "http",
		URL:           srv.URL,
		Method:        "GET",
		AllowedStatus: []string{"200-299"},
		IntervalSec:   60,
		TimeoutSec:    2,
		IsActive:      true,
		CreatedBy:     1,
	}
	id, err := ms.CreateMonitor(context.Background(), mon)
	if err != nil {
		t.Fatalf("create monitor: %v", err)
	}
	sender := &mockTelegramSender{}
	engine := monitoring.NewEngineWithDeps(ms, is, nil, "INC-{seq}", enc, sender, utils.NewLogger())
	if err := engine.CheckNow(context.Background(), id); err != nil {
		t.Fatalf("check down: %v", err)
	}
	atomic.StoreInt32(&code, 200)
	if err := engine.CheckNow(context.Background(), id); err != nil {
		t.Fatalf("check up: %v", err)
	}
	if len(sender.sent) < 2 {
		t.Fatalf("expected 2 notifications, got %d", len(sender.sent))
	}
	if !containsText(sender.sent[0].Text, "\u041c\u043e\u043d\u0438\u0442\u043e\u0440 \u043d\u0435\u0434\u043e\u0441\u0442\u0443\u043f\u0435\u043d") {
		t.Fatalf("expected down notification text")
	}
	if !containsText(sender.sent[0].Text, "500") {
		t.Fatalf("expected down notification to include error details")
	}
	if !containsText(sender.sent[1].Text, "\u041c\u043e\u043d\u0438\u0442\u043e\u0440 \u0432\u043e\u0441\u0441\u0442\u0430\u043d\u043e\u0432\u043b\u0435\u043d") {
		t.Fatalf("expected up notification text")
	}
}

func TestMonitoringSuppression(t *testing.T) {
	ms, is, _, enc, cleanup := setupMonitoringDeps(t)
	defer cleanup()
	settings, _ := ms.GetSettings(context.Background())
	settings.AllowPrivateNetworks = true
	settings.EngineEnabled = true
	settings.NotifySuppressMinutes = 10
	settings.NotifyRepeatDownMinutes = 30
	if err := ms.UpdateSettings(context.Background(), settings); err != nil {
		t.Fatalf("settings update: %v", err)
	}
	addTelegramChannel(t, ms, enc)
	var code int32 = 500
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(int(atomic.LoadInt32(&code)))
	}))
	defer srv.Close()
	mon := &store.Monitor{
		Name:          "Suppress monitor",
		Type:          "http",
		URL:           srv.URL,
		Method:        "GET",
		AllowedStatus: []string{"200-299"},
		IntervalSec:   60,
		TimeoutSec:    2,
		IsActive:      true,
		CreatedBy:     1,
	}
	id, err := ms.CreateMonitor(context.Background(), mon)
	if err != nil {
		t.Fatalf("create monitor: %v", err)
	}
	sender := &mockTelegramSender{}
	engine := monitoring.NewEngineWithDeps(ms, is, nil, "INC-{seq}", enc, sender, utils.NewLogger())
	if err := engine.CheckNow(context.Background(), id); err != nil {
		t.Fatalf("check down: %v", err)
	}
	atomic.StoreInt32(&code, 200)
	if err := engine.CheckNow(context.Background(), id); err != nil {
		t.Fatalf("check up: %v", err)
	}
	if len(sender.sent) != 2 {
		t.Fatalf("expected up notification to bypass suppression, got %d", len(sender.sent))
	}
}

func TestMonitoringAutoIncidentCreateAndClose(t *testing.T) {
	ms, is, _, enc, cleanup := setupMonitoringDeps(t)
	defer cleanup()
	settings, _ := ms.GetSettings(context.Background())
	settings.AllowPrivateNetworks = true
	settings.EngineEnabled = true
	settings.AutoIncidentCloseOnUp = true
	if err := ms.UpdateSettings(context.Background(), settings); err != nil {
		t.Fatalf("settings update: %v", err)
	}
	var code int32 = 500
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(int(atomic.LoadInt32(&code)))
	}))
	defer srv.Close()
	mon := &store.Monitor{
		Name:             "Incident monitor",
		Type:             "http",
		URL:              srv.URL,
		Method:           "GET",
		AllowedStatus:    []string{"200-299"},
		IntervalSec:      60,
		TimeoutSec:       2,
		IsActive:         true,
		CreatedBy:        1,
		AutoIncident:     true,
		IncidentSeverity: "high",
	}
	id, err := ms.CreateMonitor(context.Background(), mon)
	if err != nil {
		t.Fatalf("create monitor: %v", err)
	}
	sender := &mockTelegramSender{}
	engine := monitoring.NewEngineWithDeps(ms, is, nil, "INC-{seq}", enc, sender, utils.NewLogger())
	if err := engine.CheckNow(context.Background(), id); err != nil {
		t.Fatalf("check down: %v", err)
	}
	inc, err := is.FindOpenIncidentBySource(context.Background(), "monitoring", id)
	if err != nil || inc == nil {
		t.Fatalf("expected auto incident to be created")
	}
	if inc.Severity != "high" {
		t.Fatalf("expected incident severity to be high")
	}
	if err := engine.CheckNow(context.Background(), id); err != nil {
		t.Fatalf("check down again: %v", err)
	}
	inc2, _ := is.FindOpenIncidentBySource(context.Background(), "monitoring", id)
	if inc2 == nil || inc2.ID != inc.ID {
		t.Fatalf("expected no duplicate incident")
	}
	atomic.StoreInt32(&code, 200)
	if err := engine.CheckNow(context.Background(), id); err != nil {
		t.Fatalf("check up: %v", err)
	}
	closed, err := is.GetIncident(context.Background(), inc.ID)
	if err != nil || closed == nil || closed.Status != "closed" {
		t.Fatalf("expected incident to be closed")
	}
}

func TestMonitoringAutoIncidentNotClosedWhenFlagDisabled(t *testing.T) {
	ms, is, _, enc, cleanup := setupMonitoringDeps(t)
	defer cleanup()
	settings, _ := ms.GetSettings(context.Background())
	settings.AllowPrivateNetworks = true
	settings.EngineEnabled = true
	settings.AutoIncidentCloseOnUp = false
	if err := ms.UpdateSettings(context.Background(), settings); err != nil {
		t.Fatalf("settings update: %v", err)
	}
	var code int32 = 500
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(int(atomic.LoadInt32(&code)))
	}))
	defer srv.Close()
	mon := &store.Monitor{
		Name:             "Incident no-close monitor",
		Type:             "http",
		URL:              srv.URL,
		Method:           "GET",
		AllowedStatus:    []string{"200-299"},
		IntervalSec:      60,
		TimeoutSec:       2,
		IsActive:         true,
		CreatedBy:        1,
		AutoIncident:     true,
		IncidentSeverity: "high",
	}
	id, err := ms.CreateMonitor(context.Background(), mon)
	if err != nil {
		t.Fatalf("create monitor: %v", err)
	}
	sender := &mockTelegramSender{}
	engine := monitoring.NewEngineWithDeps(ms, is, nil, "INC-{seq}", enc, sender, utils.NewLogger())
	if err := engine.CheckNow(context.Background(), id); err != nil {
		t.Fatalf("check down: %v", err)
	}
	inc, err := is.FindOpenIncidentBySource(context.Background(), "monitoring", id)
	if err != nil || inc == nil {
		t.Fatalf("expected auto incident to be created")
	}
	atomic.StoreInt32(&code, 200)
	if err := engine.CheckNow(context.Background(), id); err != nil {
		t.Fatalf("check up: %v", err)
	}
	stillOpen, err := is.FindOpenIncidentBySource(context.Background(), "monitoring", id)
	if err != nil || stillOpen == nil || stillOpen.ID != inc.ID {
		t.Fatalf("expected incident to remain open when auto close is disabled")
	}
}

func TestMonitoringMaintenanceSuppression(t *testing.T) {
	ms, is, _, enc, cleanup := setupMonitoringDeps(t)
	defer cleanup()
	settings, _ := ms.GetSettings(context.Background())
	settings.AllowPrivateNetworks = true
	settings.EngineEnabled = true
	if err := ms.UpdateSettings(context.Background(), settings); err != nil {
		t.Fatalf("settings update: %v", err)
	}
	addTelegramChannel(t, ms, enc)
	var code int32 = 500
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(int(atomic.LoadInt32(&code)))
	}))
	defer srv.Close()
	mon := &store.Monitor{
		Name:          "Maintenance monitor",
		Type:          "http",
		URL:           srv.URL,
		Method:        "GET",
		AllowedStatus: []string{"200-299"},
		IntervalSec:   60,
		TimeoutSec:    2,
		IsActive:      true,
		CreatedBy:     1,
		AutoIncident:  true,
	}
	id, err := ms.CreateMonitor(context.Background(), mon)
	if err != nil {
		t.Fatalf("create monitor: %v", err)
	}
	now := time.Now().UTC()
	window := &store.MonitorMaintenance{
		Name:      "Maint",
		MonitorID: &id,
		StartsAt:  now.Add(-10 * time.Minute),
		EndsAt:    now.Add(10 * time.Minute),
		IsActive:  true,
	}
	if _, err := ms.CreateMaintenance(context.Background(), window); err != nil {
		t.Fatalf("create maintenance: %v", err)
	}
	sender := &mockTelegramSender{}
	engine := monitoring.NewEngineWithDeps(ms, is, nil, "INC-{seq}", enc, sender, utils.NewLogger())
	if err := engine.CheckNow(context.Background(), id); err != nil {
		t.Fatalf("check down: %v", err)
	}
	if len(sender.sent) != 0 {
		t.Fatalf("expected no notifications during maintenance")
	}
	inc, _ := is.FindOpenIncidentBySource(context.Background(), "monitoring", id)
	if inc != nil {
		t.Fatalf("expected no auto incident during maintenance")
	}
}

func TestMonitoringAutoTaskOnDown(t *testing.T) {
	ms, is, ts, enc, cleanup := setupMonitoringDeps(t)
	defer cleanup()
	settings, _ := ms.GetSettings(context.Background())
	settings.AllowPrivateNetworks = true
	settings.EngineEnabled = true
	settings.AutoTaskOnDown = true
	if err := ms.UpdateSettings(context.Background(), settings); err != nil {
		t.Fatalf("settings update: %v", err)
	}

	createTaskDestination(t, ts)

	var code int32 = 500
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(int(atomic.LoadInt32(&code)))
	}))
	defer srv.Close()

	mon := &store.Monitor{
		Name:           "Task monitor",
		Type:           "http",
		URL:            srv.URL,
		Method:         "GET",
		AllowedStatus:  []string{"200-299"},
		IntervalSec:    60,
		TimeoutSec:     2,
		IsActive:       true,
		AutoTaskOnDown: true,
		CreatedBy:      1,
	}
	id, err := ms.CreateMonitor(context.Background(), mon)
	if err != nil {
		t.Fatalf("create monitor: %v", err)
	}

	sender := &mockTelegramSender{}
	engine := monitoring.NewEngineWithDeps(ms, is, nil, "INC-{seq}", enc, sender, utils.NewLogger())
	engine.SetTaskStore(ts)

	if err := engine.CheckNow(context.Background(), id); err != nil {
		t.Fatalf("check down: %v", err)
	}
	if err := engine.CheckNow(context.Background(), id); err != nil {
		t.Fatalf("check down repeat: %v", err)
	}

	items, err := ts.ListTasks(context.Background(), tasks.TaskFilter{Search: "Task monitor"})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one automation task, got %d", len(items))
	}
}

func TestMonitoringAutoTLSIncidentBySettings(t *testing.T) {
	ms, is, _, enc, cleanup := setupMonitoringDeps(t)
	defer cleanup()
	settings, _ := ms.GetSettings(context.Background())
	settings.AllowPrivateNetworks = true
	settings.EngineEnabled = true
	settings.AutoTLSIncident = true
	settings.AutoTLSIncidentDays = 36500
	if err := ms.UpdateSettings(context.Background(), settings); err != nil {
		t.Fatalf("settings update: %v", err)
	}

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	mon := &store.Monitor{
		Name:              "TLS monitor",
		Type:              "http",
		URL:               srv.URL,
		Method:            "GET",
		AllowedStatus:     []string{"200-299"},
		IntervalSec:       60,
		TimeoutSec:        2,
		IsActive:          true,
		CreatedBy:         1,
		IgnoreTLSErrors:   true,
		NotifyTLSExpiring: false,
	}
	id, err := ms.CreateMonitor(context.Background(), mon)
	if err != nil {
		t.Fatalf("create monitor: %v", err)
	}

	sender := &mockTelegramSender{}
	engine := monitoring.NewEngineWithDeps(ms, is, nil, "INC-{seq}", enc, sender, utils.NewLogger())

	if err := engine.CheckNow(context.Background(), id); err != nil {
		t.Fatalf("check tls: %v", err)
	}
	inc, err := is.FindOpenIncidentBySource(context.Background(), "monitoring_tls", id)
	if err != nil || inc == nil {
		t.Fatalf("expected tls auto incident to be created")
	}
	if !strings.Contains(inc.Title, "TLS") {
		t.Fatalf("expected tls incident title, got %q", inc.Title)
	}

	seedNow := time.Now().UTC()
	seedDays := 1
	if err := ms.UpsertMonitorState(context.Background(), &store.MonitorState{
		MonitorID:        id,
		Status:           "up",
		LastResultStatus: "up",
		LastCheckedAt:    &seedNow,
		TLSDaysLeft:      &seedDays,
	}); err != nil {
		t.Fatalf("seed monitor state: %v", err)
	}

	settings.AutoTLSIncidentDays = 30
	if err := ms.UpdateSettings(context.Background(), settings); err != nil {
		t.Fatalf("settings update 2: %v", err)
	}
	engine.InvalidateSettings()
	if err := engine.CheckNow(context.Background(), id); err != nil {
		t.Fatalf("check tls second: %v", err)
	}
	closed, err := is.GetIncident(context.Background(), inc.ID)
	if err != nil || closed == nil || closed.Status != "closed" {
		t.Fatalf("expected tls auto incident to close")
	}
}

func containsText(haystack, needle string) bool {
	return needle != "" && strings.Contains(haystack, needle)
}
