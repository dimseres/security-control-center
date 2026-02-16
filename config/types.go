package config

import "time"

type AppConfig struct {
	DBDriver       string          `yaml:"db_driver" env:"BERKUT_DB_DRIVER" env-default:"postgres"`
	DBURL          string          `yaml:"db_url" env:"BERKUT_DB_URL" env-default:"postgres://berkut:berkut@localhost:5432/berkut?sslmode=disable"`
	DBPath         string          `yaml:"db_path"` // deprecated
	ListenAddr     string          `yaml:"listen_addr" env:"BERKUT_LISTEN_ADDR" env-default:"0.0.0.0:8080"`
	SessionTTL     time.Duration   `yaml:"session_ttl" env:"BERKUT_SESSION_TTL" env-default:"3h"`
	AppEnv         string          `yaml:"app_env" env:"BERKUT_APP_ENV"`
	DeploymentMode string          `yaml:"deployment_mode" env:"BERKUT_DEPLOYMENT_MODE" env-default:"enterprise"`
	CSRFKey        string          `yaml:"csrf_key" env:"BERKUT_CSRF_KEY"`
	Pepper         string          `yaml:"pepper" env:"BERKUT_PEPPER"`
	TLSEnabled     bool            `yaml:"tls_enabled" env:"BERKUT_TLS_ENABLED" env-default:"false"`
	TLSCert        string          `yaml:"tls_cert" env:"BERKUT_TLS_CERT"`
	TLSKey         string          `yaml:"tls_key" env:"BERKUT_TLS_KEY"`
	Scheduler      SchedulerConfig `yaml:"scheduler"`
	Docs           DocsConfig      `yaml:"docs"`
	Security       SecurityConfig  `yaml:"security"`
	Incidents      IncidentsConfig `yaml:"incidents"`
	Backups        BackupsConfig   `yaml:"backups"`
}

func (c *AppConfig) IsHomeMode() bool {
	if c == nil {
		return false
	}
	return c.DeploymentMode == "home"
}

type DocsConfig struct {
	StoragePath        string            `yaml:"storage_path" env:"BERKUT_DOCS_STORAGE_PATH" env-default:"data/docs"`
	StorageDir         string            `yaml:"storage_dir" env:"BERKUT_DOCS_STORAGE_DIR" env-default:"data/docs"`
	EncryptionKey      string            `yaml:"encryption_key" env:"BERKUT_DOCS_ENCRYPTION_KEY"`
	EncryptionKeyID    string            `yaml:"encryption_key_id" env:"BERKUT_DOCS_ENCRYPTION_KEY_ID"`
	RegTemplate        string            `yaml:"reg_template" env:"BERKUT_DOCS_REG_TEMPLATE" env-default:"{level}.{year}.{seq}"`
	PerFolderSequence  bool              `yaml:"per_folder_sequence" env:"BERKUT_DOCS_PER_FOLDER_SEQUENCE" env-default:"false"`
	VersionLimit       int               `yaml:"version_limit" env:"BERKUT_DOCS_VERSION_LIMIT" env-default:"10"`
	Watermark          WatermarkConfig   `yaml:"watermark"`
	Converters         ConvertersConfig  `yaml:"converters"`
	OnlyOffice         OnlyOfficeConfig  `yaml:"onlyoffice"`
	DLP                DocsDLPConfig     `yaml:"dlp"`
	AllowDowngrade     bool              `yaml:"allow_downgrade"`
	WatermarkMinLevel  string            `yaml:"watermark_min_level"` // deprecated; kept for compatibility
	ClassificationTags map[string]string `yaml:"classification_tags"` // optional mapping of tag codes to descriptions
}

type DocsDLPConfig struct {
	Enabled                  bool `yaml:"enabled" env:"BERKUT_DOCS_DLP_ENABLED" env-default:"true"`
	DualApprovalMinutes      int  `yaml:"dual_approval_minutes" env:"BERKUT_DOCS_DLP_APPROVAL_MINUTES" env-default:"30"`
	ProtectClipboardAndPrint bool `yaml:"protect_clipboard_and_print" env:"BERKUT_DOCS_DLP_PROTECT_CLIPBOARD_PRINT" env-default:"true"`
}

type WatermarkConfig struct {
	Enabled      bool   `yaml:"enabled" env:"BERKUT_DOCS_WATERMARK_ENABLED" env-default:"true"`
	MinLevel     string `yaml:"min_level" env:"BERKUT_DOCS_WATERMARK_MIN_LEVEL" env-default:"CONFIDENTIAL"`
	TextTemplate string `yaml:"text_template" env:"BERKUT_DOCS_WATERMARK_TEMPLATE" env-default:"Berkut SCC - {classification} - {username} - {timestamp} - DocNo {reg_no}"`
	Placement    string `yaml:"placement" env:"BERKUT_DOCS_WATERMARK_PLACEMENT" env-default:"header"`
}

type ConvertersConfig struct {
	Enabled     bool   `yaml:"enabled" env:"BERKUT_DOCS_CONVERTERS_ENABLED" env-default:"false"`
	PandocPath  string `yaml:"pandoc_path" env:"BERKUT_DOCS_CONVERTERS_PANDOC_PATH" env-default:"pandoc"`
	SofficePath string `yaml:"soffice_path" env:"BERKUT_DOCS_CONVERTERS_SOFFICE_PATH" env-default:"soffice"`
	TimeoutSec  int    `yaml:"timeout_sec" env:"BERKUT_DOCS_CONVERTERS_TIMEOUT" env-default:"20"`
	TempDir     string `yaml:"temp_dir" env:"BERKUT_DOCS_CONVERTERS_TEMP_DIR"`
}

type OnlyOfficeConfig struct {
	Enabled        bool   `yaml:"enabled" env:"BERKUT_DOCS_ONLYOFFICE_ENABLED" env-default:"false"`
	PublicURL      string `yaml:"public_url" env:"BERKUT_DOCS_ONLYOFFICE_PUBLIC_URL" env-default:""`
	InternalURL    string `yaml:"internal_url" env:"BERKUT_DOCS_ONLYOFFICE_INTERNAL_URL" env-default:"http://onlyoffice/"`
	AppInternalURL string `yaml:"app_internal_url" env:"BERKUT_DOCS_ONLYOFFICE_APP_INTERNAL_URL" env-default:"http://berkut:8080"`
	JWTSecret      string `yaml:"jwt_secret" env:"BERKUT_DOCS_ONLYOFFICE_JWT_SECRET" env-default:""`
	JWTHeader      string `yaml:"jwt_header" env:"BERKUT_DOCS_ONLYOFFICE_JWT_HEADER" env-default:"Authorization"`
	JWTIssuer      string `yaml:"jwt_issuer" env:"BERKUT_DOCS_ONLYOFFICE_JWT_ISSUER" env-default:"berkut-scc"`
	JWTAudience    string `yaml:"jwt_audience" env:"BERKUT_DOCS_ONLYOFFICE_JWT_AUDIENCE" env-default:"onlyoffice-document-server"`
	VerifyTLS      bool   `yaml:"verify_tls" env:"BERKUT_DOCS_ONLYOFFICE_VERIFY_TLS" env-default:"true"`
	RequestTimeout int    `yaml:"request_timeout_sec" env:"BERKUT_DOCS_ONLYOFFICE_REQUEST_TIMEOUT" env-default:"20"`
}

type SecurityConfig struct {
	TagsSubsetEnforced  bool     `yaml:"tags_subset_enforced" env:"BERKUT_SECURITY_TAGS_SUBSET" env-default:"true"`
	OnlineWindowSec     int      `yaml:"online_window_sec" env:"BERKUT_SECURITY_ONLINE_WINDOW_SEC" env-default:"300"`
	LegacyImportEnabled bool     `yaml:"legacy_import_enabled" env:"BERKUT_SECURITY_LEGACY_IMPORT_ENABLED" env-default:"false"`
	TrustedProxies      []string `yaml:"trusted_proxies" env:"BERKUT_SECURITY_TRUSTED_PROXIES" env-separator:","`
	AuthLockoutIncident bool     `yaml:"auth_lockout_incident" env:"BERKUT_SECURITY_AUTH_LOCKOUT_INCIDENT" env-default:"true"`
}

type IncidentsConfig struct {
	RegNoFormat         string `yaml:"reg_no_format" env:"BERKUT_INCIDENTS_REG_NO_FORMAT" env-default:"INC-{year}-{seq:05}"`
	StorageDir          string `yaml:"storage_dir" env:"BERKUT_INCIDENTS_STORAGE_DIR" env-default:"data/incidents"`
	TimelineExportLimit int    `yaml:"timeline_export_limit"`
}

type SchedulerConfig struct {
	Enabled         bool `yaml:"enabled" env:"BERKUT_SCHEDULER_ENABLED" env-default:"true"`
	IntervalSeconds int  `yaml:"interval_seconds" env:"BERKUT_SCHEDULER_INTERVAL_SECONDS" env-default:"60"`
	MaxJobsPerTick  int  `yaml:"max_jobs_per_tick" env:"BERKUT_SCHEDULER_MAX_JOBS_PER_TICK" env-default:"20"`
}

type BackupsConfig struct {
	Path                     string `yaml:"path" env:"BERKUT_BACKUP_PATH" env-default:"data/backups"`
	EncryptionKey            string `yaml:"encryption_key" env:"BERKUT_BACKUP_ENCRYPTION_KEY"`
	MaxParallel              int    `yaml:"max_parallel" env:"BERKUT_BACKUP_MAX_PARALLEL" env-default:"1"`
	PGDumpBin                string `yaml:"pgdump_bin" env:"BERKUT_BACKUP_PGDUMP_BIN" env-default:"pg_dump"`
	UploadMaxBytes           int64  `yaml:"upload_max_bytes" env:"BERKUT_BACKUP_UPLOAD_MAX_BYTES" env-default:"536870912"`
	RestoreTestAutoEnabled   bool   `yaml:"restore_test_auto_enabled" env:"BERKUT_BACKUP_RESTORE_TEST_AUTO_ENABLED" env-default:"true"`
	RestoreTestIntervalHours int    `yaml:"restore_test_interval_hours" env:"BERKUT_BACKUP_RESTORE_TEST_INTERVAL_HOURS" env-default:"168"`
}

const maxUserSessionTTL = 3 * time.Hour

func (c *AppConfig) EffectiveSessionTTL() time.Duration {
	ttl := maxUserSessionTTL
	if c != nil && c.SessionTTL > 0 {
		ttl = c.SessionTTL
	}
	if ttl > maxUserSessionTTL {
		return maxUserSessionTTL
	}
	return ttl
}
