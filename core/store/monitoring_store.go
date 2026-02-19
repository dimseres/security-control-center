package store

import (
	"context"
	"database/sql"
	"time"
)

type MonitoringStore interface {
	CreateMonitor(ctx context.Context, m *Monitor) (int64, error)
	UpdateMonitor(ctx context.Context, m *Monitor) error
	DeleteMonitor(ctx context.Context, id int64) error
	GetMonitor(ctx context.Context, id int64) (*Monitor, error)
	ListMonitors(ctx context.Context, filter MonitorFilter) ([]MonitorSummary, error)
	ListDueMonitors(ctx context.Context, now time.Time) ([]Monitor, error)
	SetMonitorPaused(ctx context.Context, id int64, paused bool) error

	GetMonitorState(ctx context.Context, id int64) (*MonitorState, error)
	ListMonitorStates(ctx context.Context, ids []int64) ([]MonitorState, error)
	UpsertMonitorState(ctx context.Context, state *MonitorState) error
	MarkMonitorDueNow(ctx context.Context, monitorID int64) error
	AddMetric(ctx context.Context, metric *MonitorMetric) (int64, error)
	ListMetrics(ctx context.Context, monitorID int64, since time.Time) ([]MonitorMetric, error)
	ListEvents(ctx context.Context, monitorID int64, since time.Time) ([]MonitorEvent, error)
	ListEventsFeed(ctx context.Context, filter EventFilter) ([]MonitorEvent, error)
	AddEvent(ctx context.Context, event *MonitorEvent) (int64, error)
	MetricsSummary(ctx context.Context, monitorID int64, since time.Time) (int, int, float64, error)
	MetricsSummaryBetween(ctx context.Context, monitorID int64, since, until time.Time) (int, int, error)
	DeleteMetricsBefore(ctx context.Context, before time.Time) (int64, error)
	DeleteMonitorMetrics(ctx context.Context, monitorID int64) (int64, error)
	DeleteMonitorEvents(ctx context.Context, monitorID int64) (int64, error)

	GetTLS(ctx context.Context, monitorID int64) (*MonitorTLS, error)
	UpsertTLS(ctx context.Context, tls *MonitorTLS) error
	ListCerts(ctx context.Context, filter CertFilter) ([]MonitorCertSummary, error)

	ListMaintenance(ctx context.Context, filter MaintenanceFilter) ([]MonitorMaintenance, error)
	GetMaintenance(ctx context.Context, id int64) (*MonitorMaintenance, error)
	CreateMaintenance(ctx context.Context, m *MonitorMaintenance) (int64, error)
	UpdateMaintenance(ctx context.Context, m *MonitorMaintenance) error
	StopMaintenance(ctx context.Context, id int64, stoppedBy int64) error
	DeleteMaintenance(ctx context.Context, id int64) error
	ActiveMaintenanceFor(ctx context.Context, monitorID int64, tags []string, now time.Time) ([]MonitorMaintenance, error)
	MaintenanceWindowsFor(ctx context.Context, monitorID int64, tags []string, since, until time.Time) ([]MaintenanceWindow, error)

	GetSettings(ctx context.Context) (*MonitorSettings, error)
	UpdateSettings(ctx context.Context, settings *MonitorSettings) error

	ListNotificationChannels(ctx context.Context) ([]NotificationChannel, error)
	GetNotificationChannel(ctx context.Context, id int64) (*NotificationChannel, error)
	CreateNotificationChannel(ctx context.Context, ch *NotificationChannel) (int64, error)
	UpdateNotificationChannel(ctx context.Context, ch *NotificationChannel) error
	DeleteNotificationChannel(ctx context.Context, id int64) error

	ListMonitorNotifications(ctx context.Context, monitorID int64) ([]MonitorNotification, error)
	ReplaceMonitorNotifications(ctx context.Context, monitorID int64, items []MonitorNotification) error
	ListDefaultNotificationChannels(ctx context.Context) ([]NotificationChannel, error)
	ListNotificationDeliveries(ctx context.Context, limit int) ([]MonitorNotificationDelivery, error)
	AddNotificationDelivery(ctx context.Context, item *MonitorNotificationDelivery) (int64, error)
	AcknowledgeNotificationDelivery(ctx context.Context, id int64, userID int64) error

	GetNotificationState(ctx context.Context, monitorID int64) (*MonitorNotificationState, error)
	UpsertNotificationState(ctx context.Context, st *MonitorNotificationState) error

	GetMonitorSLAPolicy(ctx context.Context, monitorID int64) (*MonitorSLAPolicy, error)
	UpsertMonitorSLAPolicy(ctx context.Context, policy *MonitorSLAPolicy) error
	ListMonitorSLAPolicies(ctx context.Context, monitorIDs []int64) ([]MonitorSLAPolicy, error)
	UpsertSLAPeriodResult(ctx context.Context, item *MonitorSLAPeriodResult) (*MonitorSLAPeriodResult, error)
	ListSLAPeriodResults(ctx context.Context, filter MonitorSLAPeriodResultListFilter) ([]MonitorSLAPeriodResult, error)
	MarkSLAPeriodIncidentCreated(ctx context.Context, id int64) error
	SyncSLAPeriodTarget(ctx context.Context, monitorID int64, targetPct float64, minCoveragePct float64) error
}

type monitoringStore struct {
	db *sql.DB
}

func NewMonitoringStore(db *sql.DB) MonitoringStore {
	return &monitoringStore{db: db}
}
