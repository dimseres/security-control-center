package store

import "time"

type Monitor struct {
	ID                int64             `json:"id"`
	Name              string            `json:"name"`
	Type              string            `json:"type"`
	URL               string            `json:"url,omitempty"`
	Host              string            `json:"host,omitempty"`
	Port              int               `json:"port,omitempty"`
	Method            string            `json:"method,omitempty"`
	RequestBody       string            `json:"request_body,omitempty"`
	RequestBodyType   string            `json:"request_body_type,omitempty"`
	Headers           map[string]string `json:"headers,omitempty"`
	IntervalSec       int               `json:"interval_sec"`
	TimeoutSec        int               `json:"timeout_sec"`
	Retries           int               `json:"retries"`
	RetryIntervalSec  int               `json:"retry_interval_sec"`
	AllowedStatus     []string          `json:"allowed_status"`
	IgnoreTLSErrors   bool              `json:"ignore_tls_errors"`
	NotifyTLSExpiring bool              `json:"notify_tls_expiring"`
	IsActive          bool              `json:"is_active"`
	IsPaused          bool              `json:"is_paused"`
	Tags              []string          `json:"tags"`
	GroupID           *int64            `json:"group_id,omitempty"`
	SLATargetPct      *float64          `json:"sla_target_pct,omitempty"`
	AutoIncident      bool              `json:"auto_incident"`
	AutoTaskOnDown    bool              `json:"auto_task_on_down"`
	IncidentSeverity  string            `json:"incident_severity,omitempty"`
	IncidentTypeID    string            `json:"incident_type_id,omitempty"`
	CreatedBy         int64             `json:"created_by"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

type MonitorSummary struct {
	Monitor
	Status         string     `json:"status"`
	LastCheckedAt  *time.Time `json:"last_checked_at,omitempty"`
	LastUpAt       *time.Time `json:"last_up_at,omitempty"`
	LastDownAt     *time.Time `json:"last_down_at,omitempty"`
	LastLatencyMs  *int       `json:"last_latency_ms,omitempty"`
	LastStatusCode *int       `json:"last_status_code,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
}

type MonitorState struct {
	MonitorID         int64      `json:"monitor_id"`
	Status            string     `json:"status"`
	LastResultStatus  string     `json:"last_result_status,omitempty"`
	MaintenanceActive bool       `json:"maintenance_active"`
	LastCheckedAt     *time.Time `json:"last_checked_at,omitempty"`
	LastUpAt          *time.Time `json:"last_up_at,omitempty"`
	LastDownAt        *time.Time `json:"last_down_at,omitempty"`
	LastLatencyMs     *int       `json:"last_latency_ms,omitempty"`
	LastStatusCode    *int       `json:"last_status_code,omitempty"`
	LastError         string     `json:"last_error,omitempty"`
	Uptime24h         float64    `json:"uptime_24h"`
	Uptime30d         float64    `json:"uptime_30d"`
	AvgLatency24h     float64    `json:"avg_latency_24h"`
	TLSDaysLeft       *int       `json:"tls_days_left,omitempty"`
	TLSNotAfter       *time.Time `json:"tls_not_after,omitempty"`
}

type MonitorMetric struct {
	ID         int64     `json:"id"`
	MonitorID  int64     `json:"monitor_id"`
	TS         time.Time `json:"ts"`
	LatencyMs  int       `json:"latency_ms"`
	OK         bool      `json:"ok"`
	StatusCode *int      `json:"status_code,omitempty"`
	Error      *string   `json:"error,omitempty"`
}

type MonitorEvent struct {
	ID        int64     `json:"id"`
	MonitorID int64     `json:"monitor_id"`
	TS        time.Time `json:"ts"`
	EventType string    `json:"event_type"`
	Message   string    `json:"message"`
}

type MonitorTLS struct {
	MonitorID         int64     `json:"monitor_id"`
	CheckedAt         time.Time `json:"checked_at"`
	NotAfter          time.Time `json:"not_after"`
	NotBefore         time.Time `json:"not_before"`
	CommonName        string    `json:"common_name"`
	Issuer            string    `json:"issuer"`
	SANs              []string  `json:"sans"`
	FingerprintSHA256 string    `json:"fingerprint_sha256"`
	LastError         *string   `json:"last_error,omitempty"`
}

type MonitorCertSummary struct {
	MonitorID    int64      `json:"monitor_id"`
	Name         string     `json:"name"`
	URL          string     `json:"url"`
	Tags         []string   `json:"tags"`
	Status       string     `json:"status"`
	NotAfter     *time.Time `json:"not_after,omitempty"`
	NotBefore    *time.Time `json:"not_before,omitempty"`
	CheckedAt    *time.Time `json:"checked_at,omitempty"`
	CommonName   string     `json:"common_name,omitempty"`
	Issuer       string     `json:"issuer,omitempty"`
	DaysLeft     *int       `json:"days_left,omitempty"`
	ExpiringSoon bool       `json:"expiring_soon"`
	LastError    string     `json:"last_error,omitempty"`
}

type MonitorMaintenance struct {
	ID            int64               `json:"id"`
	Name          string              `json:"name"`
	DescriptionMD string              `json:"description_md,omitempty"`
	MonitorID     *int64              `json:"monitor_id,omitempty"`
	MonitorIDs    []int64             `json:"monitor_ids,omitempty"`
	Tags          []string            `json:"tags,omitempty"`
	StartsAt      time.Time           `json:"starts_at"`
	EndsAt        time.Time           `json:"ends_at"`
	Timezone      string              `json:"timezone,omitempty"`
	Strategy      string              `json:"strategy"`
	Schedule      MaintenanceSchedule `json:"schedule"`
	IsRecurring   bool                `json:"is_recurring"`
	RRuleText     string              `json:"rrule_text,omitempty"`
	CreatedBy     int64               `json:"created_by"`
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
	IsActive      bool                `json:"is_active"`
	StoppedAt     *time.Time          `json:"stopped_at,omitempty"`
	StoppedBy     *int64              `json:"stopped_by,omitempty"`
}

type MaintenanceSchedule struct {
	CronExpression string     `json:"cron_expression,omitempty"`
	DurationMin    int        `json:"duration_min,omitempty"`
	IntervalDays   int        `json:"interval_days,omitempty"`
	Weekdays       []int      `json:"weekdays,omitempty"`
	MonthDays      []int      `json:"month_days,omitempty"`
	UseLastDay     bool       `json:"use_last_day,omitempty"`
	WindowStart    string     `json:"window_start,omitempty"`
	WindowEnd      string     `json:"window_end,omitempty"`
	ActiveFrom     *time.Time `json:"active_from,omitempty"`
	ActiveUntil    *time.Time `json:"active_until,omitempty"`
}

type MaintenanceWindow struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type MonitorSettings struct {
	ID                      int64     `json:"id"`
	RetentionDays           int       `json:"retention_days"`
	MaxConcurrentChecks     int       `json:"max_concurrent_checks"`
	DefaultTimeoutSec       int       `json:"default_timeout_sec"`
	DefaultIntervalSec      int       `json:"default_interval_sec"`
	EngineEnabled           bool      `json:"engine_enabled"`
	AllowPrivateNetworks    bool      `json:"allow_private_networks"`
	TLSRefreshHours         int       `json:"tls_refresh_hours"`
	TLSExpiringDays         int       `json:"tls_expiring_days"`
	NotifySuppressMinutes   int       `json:"notify_suppress_minutes"`
	NotifyRepeatDownMinutes int       `json:"notify_repeat_down_minutes"`
	NotifyMaintenance       bool      `json:"notify_maintenance"`
	AutoTaskOnDown          bool      `json:"auto_task_on_down"`
	AutoTLSIncident         bool      `json:"auto_tls_incident"`
	AutoTLSIncidentDays     int       `json:"auto_tls_incident_days"`
	AutoIncidentCloseOnUp   bool      `json:"auto_incident_close_on_up"`
	DefaultRetries          int       `json:"default_retries"`
	DefaultRetryIntervalSec int       `json:"default_retry_interval_sec"`
	DefaultSLATargetPct     float64   `json:"default_sla_target_pct"`
	UpdatedAt               time.Time `json:"updated_at"`
}

type MonitorFilter struct {
	Query  string
	Tags   []string
	Status string
	Active *bool
}

type CertFilter struct {
	ExpiringLt int
	Tags       []string
	Status     string
}

type MaintenanceFilter struct {
	Active    *bool
	MonitorID *int64
}

type EventFilter struct {
	Since     time.Time
	Types     []string
	MonitorID *int64
	Tags      []string
	Limit     int
}

type NotificationChannel struct {
	ID                  int64     `json:"id"`
	Type                string    `json:"type"`
	Name                string    `json:"name"`
	TelegramBotTokenEnc []byte    `json:"-"`
	TelegramChatID      string    `json:"telegram_chat_id"`
	TelegramThreadID    *int64    `json:"telegram_thread_id,omitempty"`
	TemplateText        string    `json:"template_text"`
	QuietHoursEnabled   bool      `json:"quiet_hours_enabled"`
	QuietHoursStart     string    `json:"quiet_hours_start"`
	QuietHoursEnd       string    `json:"quiet_hours_end"`
	QuietHoursTZ        string    `json:"quiet_hours_tz"`
	Silent              bool      `json:"silent"`
	ProtectContent      bool      `json:"protect_content"`
	IsDefault           bool      `json:"is_default"`
	CreatedBy           int64     `json:"created_by"`
	CreatedAt           time.Time `json:"created_at"`
	IsActive            bool      `json:"is_active"`
}

type MonitorNotificationDelivery struct {
	ID                    int64      `json:"id"`
	MonitorID             *int64     `json:"monitor_id,omitempty"`
	NotificationChannelID int64      `json:"notification_channel_id"`
	EventType             string     `json:"event_type"`
	Status                string     `json:"status"`
	Error                 string     `json:"error,omitempty"`
	BodyPreview           string     `json:"body_preview"`
	CreatedAt             time.Time  `json:"created_at"`
	AcknowledgedAt        *time.Time `json:"acknowledged_at,omitempty"`
	AcknowledgedBy        *int64     `json:"acknowledged_by,omitempty"`
}

type MonitorNotification struct {
	ID                    int64 `json:"id"`
	MonitorID             int64 `json:"monitor_id"`
	NotificationChannelID int64 `json:"notification_channel_id"`
	Enabled               bool  `json:"enabled"`
}

type MonitorNotificationState struct {
	MonitorID                 int64      `json:"monitor_id"`
	LastNotifiedAt            *time.Time `json:"last_notified_at,omitempty"`
	LastDownNotifiedAt        *time.Time `json:"last_down_notified_at,omitempty"`
	LastUpNotifiedAt          *time.Time `json:"last_up_notified_at,omitempty"`
	LastTLSNotifiedAt         *time.Time `json:"last_tls_notified_at,omitempty"`
	LastMaintenanceNotifiedAt *time.Time `json:"last_maintenance_notified_at,omitempty"`
	DownStartedAt             *time.Time `json:"down_started_at,omitempty"`
	DownSequence              int        `json:"down_sequence"`
}

type MonitorSLAPolicy struct {
	MonitorID           int64     `json:"monitor_id"`
	IncidentOnViolation bool      `json:"incident_on_violation"`
	IncidentPeriod      string    `json:"incident_period"`
	MinCoveragePct      float64   `json:"min_coverage_pct"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type MonitorSLAPeriodResult struct {
	ID              int64     `json:"id"`
	MonitorID       int64     `json:"monitor_id"`
	PeriodType      string    `json:"period_type"`
	PeriodStart     time.Time `json:"period_start"`
	PeriodEnd       time.Time `json:"period_end"`
	UptimePct       float64   `json:"uptime_pct"`
	CoveragePct     float64   `json:"coverage_pct"`
	TargetPct       float64   `json:"target_pct"`
	Status          string    `json:"status"`
	IncidentCreated bool      `json:"incident_created"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type MonitorSLAPeriodResultListFilter struct {
	Limit        int
	MonitorID    *int64
	PeriodType   string
	Status       string
	OnlyViolates bool
}
