package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"berkut-scc/core/controls"
	"berkut-scc/core/utils"
)

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		email TEXT NOT NULL DEFAULT '',
		password_hash TEXT NOT NULL,
		salt TEXT NOT NULL,
		require_password_change INTEGER NOT NULL DEFAULT 0,
		failed_attempts INTEGER NOT NULL DEFAULT 0,
		locked_until TIMESTAMP,
		lock_reason TEXT NOT NULL DEFAULT '',
		lock_stage INTEGER NOT NULL DEFAULT 0,
		last_login_at TIMESTAMP,
		last_failed_at TIMESTAMP,
		totp_secret TEXT NOT NULL DEFAULT '',
		totp_enabled INTEGER NOT NULL DEFAULT 0,
		active INTEGER NOT NULL DEFAULT 1,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS roles (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		permissions TEXT NOT NULL DEFAULT '[]',
		built_in INTEGER NOT NULL DEFAULT 0,
		template INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`,
	`CREATE TABLE IF NOT EXISTS user_roles (
		user_id INTEGER NOT NULL,
		role_id INTEGER NOT NULL,
		PRIMARY KEY (user_id, role_id),
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY(role_id) REFERENCES roles(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL,
		username TEXT NOT NULL,
		roles TEXT NOT NULL,
		csrf_token TEXT NOT NULL,
		ip TEXT NOT NULL DEFAULT '',
		user_agent TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_seen_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP NOT NULL,
		revoked INTEGER NOT NULL DEFAULT 0,
		revoked_at TIMESTAMP,
		revoked_by TEXT NOT NULL DEFAULT ''
	);`,
	`CREATE TABLE IF NOT EXISTS audit_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL,
		action TEXT NOT NULL,
		details TEXT,
		created_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS dashboard_layouts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL UNIQUE,
		layout_json TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS doc_folders (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		parent_id INTEGER,
		classification_level INTEGER NOT NULL DEFAULT 0,
		classification_tags TEXT NOT NULL DEFAULT '[]',
		inherit_acl INTEGER NOT NULL DEFAULT 1,
		inherit_classification INTEGER NOT NULL DEFAULT 1,
		created_by INTEGER,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY(parent_id) REFERENCES doc_folders(id) ON DELETE SET NULL
	);`,
	`CREATE TABLE IF NOT EXISTS folder_acl (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		folder_id INTEGER NOT NULL,
		subject_type TEXT NOT NULL,
		subject_id TEXT NOT NULL,
		permission TEXT NOT NULL,
		UNIQUE(folder_id, subject_type, subject_id, permission),
		FOREIGN KEY(folder_id) REFERENCES doc_folders(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS docs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		folder_id INTEGER,
		title TEXT NOT NULL,
		status TEXT NOT NULL,
		classification_level INTEGER NOT NULL,
		classification_tags TEXT NOT NULL DEFAULT '[]',
		reg_number TEXT NOT NULL UNIQUE,
		doc_type TEXT NOT NULL DEFAULT 'document',
		inherit_acl INTEGER NOT NULL DEFAULT 1,
		inherit_classification INTEGER NOT NULL DEFAULT 1,
		created_by INTEGER,
		current_version INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		deleted_at TIMESTAMP,
		FOREIGN KEY(folder_id) REFERENCES doc_folders(id) ON DELETE SET NULL
	);`,
	`CREATE TABLE IF NOT EXISTS doc_versions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		doc_id INTEGER NOT NULL,
		version INTEGER NOT NULL,
		author_id INTEGER,
		author_username TEXT,
		reason TEXT,
		path TEXT NOT NULL,
		format TEXT NOT NULL,
		size_bytes INTEGER NOT NULL,
		sha256_plain TEXT NOT NULL,
		sha256_cipher TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		UNIQUE(doc_id, version),
		FOREIGN KEY(doc_id) REFERENCES docs(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS doc_acl (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		doc_id INTEGER NOT NULL,
		subject_type TEXT NOT NULL,
		subject_id TEXT NOT NULL,
		permission TEXT NOT NULL,
		UNIQUE(doc_id, subject_type, subject_id, permission),
		FOREIGN KEY(doc_id) REFERENCES docs(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS doc_templates (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		format TEXT NOT NULL,
		content TEXT NOT NULL,
		variables TEXT NOT NULL DEFAULT '[]',
		classification_level INTEGER NOT NULL DEFAULT 0,
		classification_tags TEXT NOT NULL DEFAULT '[]',
		created_by INTEGER,
		created_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS report_meta (
		doc_id INTEGER PRIMARY KEY,
		period_from TIMESTAMP,
		period_to TIMESTAMP,
		report_status TEXT NOT NULL DEFAULT 'draft',
		template_id INTEGER,
		FOREIGN KEY(doc_id) REFERENCES docs(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS report_templates (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		template_markdown TEXT NOT NULL,
		created_by INTEGER,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS report_settings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		default_classification TEXT NOT NULL DEFAULT 'INTERNAL',
		default_template_id INTEGER,
		header_enabled INTEGER NOT NULL DEFAULT 1,
		header_logo_path TEXT NOT NULL DEFAULT '/gui/static/logo.png',
		header_title TEXT NOT NULL DEFAULT 'Berkut Solutions: Security Control Center',
		watermark_threshold TEXT NOT NULL DEFAULT '',
		updated_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS report_sections (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		report_id INTEGER NOT NULL,
		section_type TEXT NOT NULL,
		title TEXT NOT NULL DEFAULT '',
		config_json TEXT NOT NULL DEFAULT '{}',
		order_index INTEGER NOT NULL DEFAULT 0,
		is_enabled INTEGER NOT NULL DEFAULT 1,
		FOREIGN KEY(report_id) REFERENCES docs(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS report_charts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		report_id INTEGER NOT NULL,
		chart_type TEXT NOT NULL,
		title TEXT NOT NULL DEFAULT '',
		section_type TEXT NOT NULL DEFAULT '',
		config_json TEXT NOT NULL DEFAULT '{}',
		order_index INTEGER NOT NULL DEFAULT 0,
		is_enabled INTEGER NOT NULL DEFAULT 1,
		FOREIGN KEY(report_id) REFERENCES docs(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS report_snapshots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		report_id INTEGER NOT NULL,
		created_at TIMESTAMP NOT NULL,
		created_by INTEGER,
		reason TEXT NOT NULL DEFAULT '',
		snapshot_json TEXT NOT NULL,
		sha256 TEXT NOT NULL,
		FOREIGN KEY(report_id) REFERENCES docs(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS report_snapshot_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		snapshot_id INTEGER NOT NULL,
		entity_type TEXT NOT NULL,
		entity_id TEXT NOT NULL,
		entity_json TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		FOREIGN KEY(snapshot_id) REFERENCES report_snapshots(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS approvals (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		doc_id INTEGER NOT NULL,
		status TEXT NOT NULL,
		message TEXT,
		current_stage INTEGER NOT NULL DEFAULT 1,
		created_by INTEGER NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY(doc_id) REFERENCES docs(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS doc_export_approvals (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		doc_id INTEGER NOT NULL,
		requested_by INTEGER NOT NULL,
		approved_by INTEGER NOT NULL,
		reason TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		expires_at TIMESTAMP NOT NULL,
		consumed_at TIMESTAMP,
		FOREIGN KEY(doc_id) REFERENCES docs(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS approval_participants (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		approval_id INTEGER NOT NULL,
		user_id INTEGER NOT NULL,
		role TEXT NOT NULL,
		stage INTEGER NOT NULL DEFAULT 1,
		stage_name TEXT NOT NULL DEFAULT '',
		stage_message TEXT NOT NULL DEFAULT '',
		decision TEXT,
		comment TEXT,
		decided_at TIMESTAMP,
		UNIQUE(approval_id, user_id, role, stage),
		FOREIGN KEY(approval_id) REFERENCES approvals(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS approval_comments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		approval_id INTEGER NOT NULL,
		user_id INTEGER NOT NULL,
		comment TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		FOREIGN KEY(approval_id) REFERENCES approvals(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS entity_links (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		doc_id INTEGER NOT NULL,
		source_type TEXT NOT NULL DEFAULT 'doc',
		source_id TEXT NOT NULL DEFAULT '',
		target_type TEXT NOT NULL,
		target_id TEXT NOT NULL,
		relation_type TEXT NOT NULL DEFAULT 'related',
		created_at TIMESTAMP NOT NULL,
		FOREIGN KEY(doc_id) REFERENCES docs(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS doc_reg_counters (
		classification_level INTEGER NOT NULL,
		folder_id INTEGER,
		year INTEGER NOT NULL,
		seq INTEGER NOT NULL,
		PRIMARY KEY (classification_level, folder_id, year)
	);`,
	`CREATE TABLE IF NOT EXISTS incidents (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		reg_no TEXT NOT NULL UNIQUE,
		title TEXT NOT NULL,
		description TEXT,
		severity TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'draft',
		source TEXT NOT NULL DEFAULT '',
		source_ref_id INTEGER,
		closed_at TIMESTAMP,
		closed_by INTEGER,
		owner_user_id INTEGER NOT NULL,
		assignee_user_id INTEGER,
		classification_level INTEGER NOT NULL DEFAULT 1,
		classification_tags TEXT NOT NULL DEFAULT '[]',
		meta_json TEXT NOT NULL DEFAULT '{}',
		created_by INTEGER NOT NULL,
		updated_by INTEGER NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		version INTEGER NOT NULL DEFAULT 1,
		deleted_at TIMESTAMP
	);`,
	`CREATE TABLE IF NOT EXISTS incident_participants (
		incident_id INTEGER NOT NULL,
		user_id INTEGER NOT NULL,
		role TEXT NOT NULL,
		PRIMARY KEY (incident_id, user_id),
		FOREIGN KEY(incident_id) REFERENCES incidents(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS incident_stages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		incident_id INTEGER NOT NULL,
		title TEXT NOT NULL,
		position INTEGER NOT NULL,
		created_by INTEGER NOT NULL,
		updated_by INTEGER NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		status TEXT NOT NULL DEFAULT 'open',
		closed_at TIMESTAMP,
		closed_by INTEGER,
		is_default INTEGER NOT NULL DEFAULT 0,
		version INTEGER NOT NULL DEFAULT 1,
		FOREIGN KEY(incident_id) REFERENCES incidents(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS incident_stage_entries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		stage_id INTEGER NOT NULL UNIQUE,
		content TEXT NOT NULL,
		change_reason TEXT NOT NULL DEFAULT '',
		created_by INTEGER NOT NULL,
		updated_by INTEGER NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		version INTEGER NOT NULL DEFAULT 1,
		FOREIGN KEY(stage_id) REFERENCES incident_stages(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS incident_acl (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		incident_id INTEGER NOT NULL,
		subject_type TEXT NOT NULL,
		subject_id TEXT NOT NULL,
		permission TEXT NOT NULL,
		UNIQUE(incident_id, subject_type, subject_id, permission),
		FOREIGN KEY(incident_id) REFERENCES incidents(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS incident_reg_counters (
		year INTEGER NOT NULL,
		seq INTEGER NOT NULL,
		PRIMARY KEY (year)
	);`,
	`CREATE TABLE IF NOT EXISTS incident_links (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		incident_id INTEGER NOT NULL,
		entity_type TEXT NOT NULL,
		entity_id TEXT NOT NULL,
		title TEXT,
		comment TEXT NOT NULL DEFAULT '',
		unverified INTEGER NOT NULL DEFAULT 0,
		created_by INTEGER NOT NULL,
		created_at TIMESTAMP NOT NULL,
		UNIQUE(incident_id, entity_type, entity_id),
		FOREIGN KEY(incident_id) REFERENCES incidents(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS incident_attachments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		incident_id INTEGER NOT NULL,
		filename TEXT NOT NULL,
		content_type TEXT NOT NULL,
		size_bytes INTEGER NOT NULL,
		sha256_plain TEXT NOT NULL,
		sha256_cipher TEXT NOT NULL,
		classification_level INTEGER NOT NULL DEFAULT 1,
		classification_tags TEXT NOT NULL DEFAULT '[]',
		uploaded_by INTEGER NOT NULL,
		uploaded_at TIMESTAMP NOT NULL,
		deleted_at TIMESTAMP,
		FOREIGN KEY(incident_id) REFERENCES incidents(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS incident_timeline (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		incident_id INTEGER NOT NULL,
		event_type TEXT NOT NULL,
		message TEXT NOT NULL,
		meta_json TEXT NOT NULL DEFAULT '{}',
		created_by INTEGER NOT NULL,
		created_at TIMESTAMP NOT NULL,
		event_at TIMESTAMP,
		FOREIGN KEY(incident_id) REFERENCES incidents(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS incident_artifact_files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		incident_id INTEGER NOT NULL,
		artifact_id TEXT NOT NULL,
		filename TEXT NOT NULL,
		content_type TEXT NOT NULL,
		size_bytes INTEGER NOT NULL,
		sha256_plain TEXT NOT NULL,
		sha256_cipher TEXT NOT NULL,
		classification_level INTEGER NOT NULL DEFAULT 1,
		classification_tags TEXT NOT NULL DEFAULT '[]',
		uploaded_by INTEGER NOT NULL,
		uploaded_at TIMESTAMP NOT NULL,
		deleted_at TIMESTAMP,
		FOREIGN KEY(incident_id) REFERENCES incidents(id) ON DELETE CASCADE
	);`,
	`CREATE VIRTUAL TABLE IF NOT EXISTS docs_fts USING fts5(content, doc_id UNINDEXED, version_id UNINDEXED);`,
	`CREATE INDEX IF NOT EXISTS idx_docs_folder ON docs(folder_id);`,
	`CREATE INDEX IF NOT EXISTS idx_doc_versions_doc ON doc_versions(doc_id);`,
	`CREATE INDEX IF NOT EXISTS idx_approvals_doc ON approvals(doc_id);`,
	`CREATE INDEX IF NOT EXISTS idx_doc_export_approvals_doc ON doc_export_approvals(doc_id, requested_by, expires_at);`,
	`CREATE INDEX IF NOT EXISTS idx_entity_links_doc ON entity_links(doc_id);`,
	`CREATE INDEX IF NOT EXISTS idx_incidents_status ON incidents(status);`,
	`CREATE INDEX IF NOT EXISTS idx_incidents_severity ON incidents(severity);`,
	`CREATE INDEX IF NOT EXISTS idx_incidents_owner ON incidents(owner_user_id);`,
	`CREATE INDEX IF NOT EXISTS idx_incidents_assignee ON incidents(assignee_user_id);`,
	`CREATE INDEX IF NOT EXISTS idx_incidents_created ON incidents(created_at);`,
	`CREATE INDEX IF NOT EXISTS idx_incident_participants_user ON incident_participants(user_id);`,
	`CREATE INDEX IF NOT EXISTS idx_incident_stages_incident ON incident_stages(incident_id);`,
	`CREATE INDEX IF NOT EXISTS idx_incident_stage_entries_stage ON incident_stage_entries(stage_id);`,
	`CREATE INDEX IF NOT EXISTS idx_incident_acl_incident ON incident_acl(incident_id);`,
	`CREATE INDEX IF NOT EXISTS idx_incident_links_incident ON incident_links(incident_id);`,
	`CREATE INDEX IF NOT EXISTS idx_incident_attachments_incident ON incident_attachments(incident_id);`,
	`CREATE INDEX IF NOT EXISTS idx_incident_artifact_files_incident ON incident_artifact_files(incident_id);`,
	`CREATE INDEX IF NOT EXISTS idx_incident_artifact_files_artifact ON incident_artifact_files(artifact_id);`,
	`CREATE INDEX IF NOT EXISTS idx_incident_timeline_incident ON incident_timeline(incident_id);`,
	`CREATE INDEX IF NOT EXISTS idx_report_charts_report ON report_charts(report_id);`,
	`CREATE TABLE IF NOT EXISTS groups (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		clearance_level INTEGER NOT NULL DEFAULT 0,
		clearance_tags TEXT NOT NULL DEFAULT '[]',
		menu_permissions TEXT NOT NULL DEFAULT '[]',
		is_system INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`,
	`CREATE TABLE IF NOT EXISTS group_roles (
		group_id INTEGER NOT NULL,
		role_id INTEGER NOT NULL,
		PRIMARY KEY(group_id, role_id),
		FOREIGN KEY(group_id) REFERENCES groups(id) ON DELETE CASCADE,
		FOREIGN KEY(role_id) REFERENCES roles(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS user_groups (
		user_id INTEGER NOT NULL,
		group_id INTEGER NOT NULL,
		PRIMARY KEY(user_id, group_id),
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY(group_id) REFERENCES groups(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS password_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		password_hash TEXT NOT NULL,
		salt TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS login_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER,
		username TEXT NOT NULL,
		event TEXT NOT NULL,
		details TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE SET NULL
	);`,
	`CREATE TABLE IF NOT EXISTS controls (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		code TEXT NOT NULL UNIQUE,
		title TEXT NOT NULL,
		description_md TEXT NOT NULL DEFAULT '',
		control_type TEXT NOT NULL,
		domain TEXT NOT NULL,
		owner_user_id INTEGER,
		review_frequency TEXT NOT NULL,
		status TEXT NOT NULL,
		risk_level TEXT NOT NULL,
		tags_json TEXT NOT NULL DEFAULT '[]',
		created_by INTEGER NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		is_active INTEGER NOT NULL DEFAULT 1
	);`,
	`CREATE TABLE IF NOT EXISTS control_types (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		is_builtin INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS idx_control_types_name ON control_types(name);`,
	`CREATE TABLE IF NOT EXISTS control_checks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		control_id INTEGER NOT NULL,
		checked_at TIMESTAMP NOT NULL,
		checked_by INTEGER,
		result TEXT NOT NULL,
		notes_md TEXT NOT NULL DEFAULT '',
		evidence_links_json TEXT NOT NULL DEFAULT '[]',
		created_at TIMESTAMP NOT NULL,
		FOREIGN KEY(control_id) REFERENCES controls(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS control_comments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		control_id INTEGER NOT NULL,
		author_id INTEGER NOT NULL,
		content TEXT NOT NULL,
		attachments TEXT NOT NULL DEFAULT '[]',
		created_at TIMESTAMP NOT NULL,
		FOREIGN KEY(control_id) REFERENCES controls(id) ON DELETE CASCADE,
		FOREIGN KEY(author_id) REFERENCES users(id) ON DELETE CASCADE
	);`,
	`CREATE INDEX IF NOT EXISTS idx_control_comments_control ON control_comments(control_id);`,
	`CREATE TABLE IF NOT EXISTS control_violations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		control_id INTEGER NOT NULL,
		incident_id INTEGER,
		happened_at TIMESTAMP NOT NULL,
		severity TEXT NOT NULL,
		summary TEXT NOT NULL,
		impact_md TEXT NOT NULL DEFAULT '',
		created_by INTEGER NOT NULL,
		created_at TIMESTAMP NOT NULL,
		is_auto INTEGER NOT NULL DEFAULT 0,
		is_active INTEGER NOT NULL DEFAULT 1,
		FOREIGN KEY(control_id) REFERENCES controls(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS control_frameworks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		version TEXT NOT NULL DEFAULT '',
		is_active INTEGER NOT NULL DEFAULT 1
	);`,
	`CREATE TABLE IF NOT EXISTS control_framework_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		framework_id INTEGER NOT NULL,
		code TEXT NOT NULL,
		title TEXT NOT NULL,
		description_md TEXT NOT NULL DEFAULT '',
		UNIQUE(framework_id, code),
		FOREIGN KEY(framework_id) REFERENCES control_frameworks(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS control_framework_map (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		framework_item_id INTEGER NOT NULL,
		control_id INTEGER NOT NULL,
		UNIQUE(framework_item_id, control_id),
		FOREIGN KEY(framework_item_id) REFERENCES control_framework_items(id) ON DELETE CASCADE,
		FOREIGN KEY(control_id) REFERENCES controls(id) ON DELETE CASCADE
	);`,
	`CREATE INDEX IF NOT EXISTS idx_controls_code ON controls(code);`,
	`CREATE INDEX IF NOT EXISTS idx_controls_status ON controls(status);`,
	`CREATE INDEX IF NOT EXISTS idx_controls_risk ON controls(risk_level);`,
	`CREATE INDEX IF NOT EXISTS idx_controls_domain ON controls(domain);`,
	`CREATE INDEX IF NOT EXISTS idx_controls_owner ON controls(owner_user_id);`,
	`CREATE INDEX IF NOT EXISTS idx_control_checks_control ON control_checks(control_id);`,
	`CREATE INDEX IF NOT EXISTS idx_control_checks_checked ON control_checks(checked_at);`,
	`CREATE INDEX IF NOT EXISTS idx_control_violations_control ON control_violations(control_id);`,
	`CREATE INDEX IF NOT EXISTS idx_control_violations_incident ON control_violations(incident_id);`,
	`CREATE INDEX IF NOT EXISTS idx_control_framework_items_framework ON control_framework_items(framework_id);`,
	`CREATE INDEX IF NOT EXISTS idx_control_framework_map_item ON control_framework_map(framework_item_id);`,
	`CREATE INDEX IF NOT EXISTS idx_control_framework_map_control ON control_framework_map(control_id);`,
	`CREATE TABLE IF NOT EXISTS monitors (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		url TEXT,
		host TEXT,
		port INTEGER,
		method TEXT NOT NULL DEFAULT 'GET',
		request_body TEXT,
		request_body_type TEXT NOT NULL DEFAULT 'none',
		headers_json TEXT NOT NULL DEFAULT '{}',
		interval_sec INTEGER NOT NULL DEFAULT 60,
		timeout_sec INTEGER NOT NULL DEFAULT 5,
		retries INTEGER NOT NULL DEFAULT 0,
		retry_interval_sec INTEGER NOT NULL DEFAULT 5,
		allowed_status_json TEXT NOT NULL DEFAULT '["200-299"]',
		ignore_tls_errors INTEGER NOT NULL DEFAULT 0,
		notify_tls_expiring INTEGER NOT NULL DEFAULT 1,
		is_active INTEGER NOT NULL DEFAULT 1,
		is_paused INTEGER NOT NULL DEFAULT 0,
		tags_json TEXT NOT NULL DEFAULT '[]',
		group_id INTEGER,
		sla_target_pct REAL,
		auto_incident INTEGER NOT NULL DEFAULT 0,
		auto_task_on_down INTEGER NOT NULL DEFAULT 0,
		incident_severity TEXT NOT NULL DEFAULT 'low',
		incident_type_id TEXT NOT NULL DEFAULT '',
		created_by INTEGER,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS monitor_state (
		monitor_id INTEGER PRIMARY KEY,
		status TEXT NOT NULL,
		last_result_status TEXT NOT NULL DEFAULT '',
		maintenance_active INTEGER NOT NULL DEFAULT 0,
		last_checked_at TIMESTAMP,
		last_up_at TIMESTAMP,
		last_down_at TIMESTAMP,
		last_latency_ms INTEGER,
		last_status_code INTEGER,
		last_error TEXT,
		uptime_24h REAL NOT NULL DEFAULT 0,
		uptime_30d REAL NOT NULL DEFAULT 0,
		avg_latency_24h REAL NOT NULL DEFAULT 0,
		tls_days_left INTEGER,
		tls_not_after TIMESTAMP,
		FOREIGN KEY(monitor_id) REFERENCES monitors(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS monitor_metrics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		monitor_id INTEGER NOT NULL,
		ts TIMESTAMP NOT NULL,
		latency_ms INTEGER NOT NULL DEFAULT 0,
		ok INTEGER NOT NULL DEFAULT 0,
		status_code INTEGER,
		error TEXT,
		FOREIGN KEY(monitor_id) REFERENCES monitors(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS monitor_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		monitor_id INTEGER NOT NULL,
		ts TIMESTAMP NOT NULL,
		event_type TEXT NOT NULL,
		message TEXT NOT NULL,
		FOREIGN KEY(monitor_id) REFERENCES monitors(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS monitor_tls (
		monitor_id INTEGER PRIMARY KEY,
		checked_at TIMESTAMP NOT NULL,
		not_after TIMESTAMP NOT NULL,
		not_before TIMESTAMP NOT NULL,
		common_name TEXT NOT NULL DEFAULT '',
		issuer TEXT NOT NULL DEFAULT '',
		san_json TEXT NOT NULL DEFAULT '[]',
		fingerprint_sha256 TEXT NOT NULL DEFAULT '',
		last_error TEXT,
		FOREIGN KEY(monitor_id) REFERENCES monitors(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS monitor_maintenance (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		description_md TEXT NOT NULL DEFAULT '',
		monitor_id INTEGER,
		monitor_ids_json TEXT NOT NULL DEFAULT '[]',
		tags_json TEXT,
		starts_at TIMESTAMP NOT NULL,
		ends_at TIMESTAMP NOT NULL,
		timezone TEXT NOT NULL DEFAULT '',
		strategy TEXT NOT NULL DEFAULT 'single',
		strategy_json TEXT NOT NULL DEFAULT '{}',
		is_recurring INTEGER NOT NULL DEFAULT 0,
		rrule_text TEXT,
		created_by INTEGER,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		is_active INTEGER NOT NULL DEFAULT 1,
		stopped_at TIMESTAMP,
		stopped_by INTEGER,
		FOREIGN KEY(monitor_id) REFERENCES monitors(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS monitoring_settings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		retention_days INTEGER NOT NULL DEFAULT 30,
		max_concurrent_checks INTEGER NOT NULL DEFAULT 10,
		default_timeout_sec INTEGER NOT NULL DEFAULT 5,
		default_interval_sec INTEGER NOT NULL DEFAULT 60,
		engine_enabled INTEGER NOT NULL DEFAULT 1,
		allow_private_networks INTEGER NOT NULL DEFAULT 0,
		tls_refresh_hours INTEGER NOT NULL DEFAULT 24,
		tls_expiring_days INTEGER NOT NULL DEFAULT 30,
		notify_suppress_minutes INTEGER NOT NULL DEFAULT 5,
		notify_repeat_down_minutes INTEGER NOT NULL DEFAULT 30,
		notify_maintenance INTEGER NOT NULL DEFAULT 0,
		auto_task_on_down INTEGER NOT NULL DEFAULT 1,
		auto_tls_incident INTEGER NOT NULL DEFAULT 1,
		auto_tls_incident_days INTEGER NOT NULL DEFAULT 14,
		default_retries INTEGER NOT NULL DEFAULT 2,
		default_retry_interval_sec INTEGER NOT NULL DEFAULT 30,
		default_sla_target_pct REAL NOT NULL DEFAULT 90,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`,
	`CREATE TABLE IF NOT EXISTS notification_channels (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		type TEXT NOT NULL,
		name TEXT NOT NULL,
		telegram_bot_token BLOB NOT NULL,
		telegram_chat_id TEXT NOT NULL,
		telegram_thread_id INTEGER,
		template_text TEXT NOT NULL DEFAULT '',
		quiet_hours_enabled INTEGER NOT NULL DEFAULT 0,
		quiet_hours_start TEXT NOT NULL DEFAULT '',
		quiet_hours_end TEXT NOT NULL DEFAULT '',
		quiet_hours_tz TEXT NOT NULL DEFAULT '',
		silent INTEGER NOT NULL DEFAULT 0,
		protect_content INTEGER NOT NULL DEFAULT 0,
		is_default INTEGER NOT NULL DEFAULT 0,
		created_by INTEGER,
		created_at TIMESTAMP NOT NULL,
		is_active INTEGER NOT NULL DEFAULT 1
	);`,
	`CREATE TABLE IF NOT EXISTS monitor_notification_deliveries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		monitor_id INTEGER,
		notification_channel_id INTEGER NOT NULL,
		event_type TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT '',
		error_text TEXT NOT NULL DEFAULT '',
		body_preview TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		acknowledged_at TIMESTAMP,
		acknowledged_by INTEGER,
		FOREIGN KEY(monitor_id) REFERENCES monitors(id) ON DELETE SET NULL,
		FOREIGN KEY(notification_channel_id) REFERENCES notification_channels(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS monitor_notifications (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		monitor_id INTEGER NOT NULL,
		notification_channel_id INTEGER NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		FOREIGN KEY(monitor_id) REFERENCES monitors(id) ON DELETE CASCADE,
		FOREIGN KEY(notification_channel_id) REFERENCES notification_channels(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS monitor_notification_state (
		monitor_id INTEGER PRIMARY KEY,
		last_notified_at TIMESTAMP,
		last_down_notified_at TIMESTAMP,
		last_up_notified_at TIMESTAMP,
		last_tls_notified_at TIMESTAMP,
		last_maintenance_notified_at TIMESTAMP,
		down_started_at TIMESTAMP,
		down_sequence INTEGER NOT NULL DEFAULT 0,
		FOREIGN KEY(monitor_id) REFERENCES monitors(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE IF NOT EXISTS app_https_settings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		mode TEXT NOT NULL DEFAULT 'disabled',
		listen_port INTEGER NOT NULL DEFAULT 8080,
		trusted_proxies_json TEXT NOT NULL DEFAULT '[]',
		builtin_cert_path TEXT NOT NULL DEFAULT '',
		builtin_key_path TEXT NOT NULL DEFAULT '',
		external_proxy_hint TEXT NOT NULL DEFAULT '',
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`,
	`CREATE TABLE IF NOT EXISTS app_runtime_settings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		deployment_mode TEXT NOT NULL DEFAULT 'enterprise',
		update_checks_enabled INTEGER NOT NULL DEFAULT 0,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`,
	`CREATE INDEX IF NOT EXISTS idx_monitors_name ON monitors(name);`,
	`CREATE INDEX IF NOT EXISTS idx_monitor_metrics_monitor_ts ON monitor_metrics(monitor_id, ts);`,
	`CREATE INDEX IF NOT EXISTS idx_monitor_events_monitor_ts ON monitor_events(monitor_id, ts);`,
	`CREATE INDEX IF NOT EXISTS idx_monitor_tls_checked ON monitor_tls(checked_at);`,
	`CREATE INDEX IF NOT EXISTS idx_monitor_maintenance_window ON monitor_maintenance(starts_at, ends_at);`,
	`CREATE INDEX IF NOT EXISTS idx_notification_channels_default ON notification_channels(is_default, is_active);`,
	`CREATE INDEX IF NOT EXISTS idx_monitor_notification_deliveries_created ON monitor_notification_deliveries(created_at);`,
	`CREATE INDEX IF NOT EXISTS idx_monitor_notification_deliveries_status ON monitor_notification_deliveries(status, acknowledged_at);`,
	`CREATE INDEX IF NOT EXISTS idx_monitor_notifications_monitor ON monitor_notifications(monitor_id);`,
}

func ApplyMigrations(ctx context.Context, db *sql.DB, logger *utils.Logger) error {
	isPG, err := isPostgresDB(ctx, db)
	if err != nil {
		return err
	}
	if !isPG {
		if !isTestRuntime() {
			return fmt.Errorf("only postgres is supported outside go test runtime")
		}
		return applySQLiteTestMigrations(ctx, db, logger)
	}
	return applyGooseMigrations(ctx, db, logger)
}

func applySQLiteTestMigrations(ctx context.Context, db *sql.DB, logger *utils.Logger) error {
	if logger != nil {
		logger.Printf("applying sqlite test migrations")
	}
	for i, stmt := range migrations {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("sqlite migration #%d failed: %w", i+1, err)
		}
	}
	post := []func(context.Context, *sql.DB) error{
		ensureUserColumns,
		ensureRoleColumns,
		ensureGroupColumns,
		ensureSessionColumns,
		ensureTemplateColumns,
		ensureDocsColumns,
		ensureApprovalColumns,
		ensureApprovalParticipantsSchema,
		ensureMonitoringColumns,
		ensureIncidentColumns,
		ensureIncidentStageColumns,
		ensureIncidentLinkColumns,
		ensureIncidentTimelineColumns,
		ensureIncidentArtifactFiles,
		ensureTaskColumns,
		ensureTaskCommentColumns,
		ensureTaskBoardColumns,
		ensureTaskColumnColumns,
		ensureTaskSpaceBackfill,
		ensureTaskBoardSpaceGuards,
		ensureControlViolationColumns,
		ensureEntityLinksSchema,
		ensureControlTypes,
	}
	for _, fn := range post {
		if err := fn(ctx, db); err != nil {
			return err
		}
	}
	if logger != nil {
		logger.Printf("sqlite test migrations applied")
	}
	return nil
}

func ensureControlTypes(ctx context.Context, db *sql.DB) error {
	for _, name := range controls.ControlTypes {
		val := strings.TrimSpace(name)
		if val == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, `
			INSERT OR IGNORE INTO control_types(name, is_builtin, created_at)
			VALUES(?, 1, CURRENT_TIMESTAMP)`, val); err != nil {
			return fmt.Errorf("insert control type %s: %w", val, err)
		}
	}
	return nil
}

func ensureUserColumns(ctx context.Context, db *sql.DB) error {
	type col struct {
		Name string
		SQL  string
	}
	cols := []col{
		{Name: "email", SQL: "ALTER TABLE users ADD COLUMN email TEXT NOT NULL DEFAULT ''"},
		{Name: "full_name", SQL: "ALTER TABLE users ADD COLUMN full_name TEXT NOT NULL DEFAULT ''"},
		{Name: "department", SQL: "ALTER TABLE users ADD COLUMN department TEXT NOT NULL DEFAULT ''"},
		{Name: "position", SQL: "ALTER TABLE users ADD COLUMN position TEXT NOT NULL DEFAULT ''"},
		{Name: "clearance_level", SQL: "ALTER TABLE users ADD COLUMN clearance_level INTEGER NOT NULL DEFAULT 0"},
		{Name: "clearance_tags", SQL: "ALTER TABLE users ADD COLUMN clearance_tags TEXT NOT NULL DEFAULT '[]'"},
		{Name: "password_set", SQL: "ALTER TABLE users ADD COLUMN password_set INTEGER NOT NULL DEFAULT 1"},
		{Name: "disabled_at", SQL: "ALTER TABLE users ADD COLUMN disabled_at TIMESTAMP"},
		{Name: "require_password_change", SQL: "ALTER TABLE users ADD COLUMN require_password_change INTEGER NOT NULL DEFAULT 0"},
		{Name: "failed_attempts", SQL: "ALTER TABLE users ADD COLUMN failed_attempts INTEGER NOT NULL DEFAULT 0"},
		{Name: "locked_until", SQL: "ALTER TABLE users ADD COLUMN locked_until TIMESTAMP"},
		{Name: "lock_reason", SQL: "ALTER TABLE users ADD COLUMN lock_reason TEXT NOT NULL DEFAULT ''"},
		{Name: "lock_stage", SQL: "ALTER TABLE users ADD COLUMN lock_stage INTEGER NOT NULL DEFAULT 0"},
		{Name: "last_login_at", SQL: "ALTER TABLE users ADD COLUMN last_login_at TIMESTAMP"},
		{Name: "last_failed_at", SQL: "ALTER TABLE users ADD COLUMN last_failed_at TIMESTAMP"},
		{Name: "totp_secret", SQL: "ALTER TABLE users ADD COLUMN totp_secret TEXT NOT NULL DEFAULT ''"},
		{Name: "totp_enabled", SQL: "ALTER TABLE users ADD COLUMN totp_enabled INTEGER NOT NULL DEFAULT 0"},
		{Name: "password_changed_at", SQL: "ALTER TABLE users ADD COLUMN password_changed_at TIMESTAMP"},
	}
	for _, c := range cols {
		exists, err := columnExists(ctx, db, "users", c.Name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := db.ExecContext(ctx, c.SQL); err != nil {
			return fmt.Errorf("add column %s: %w", c.Name, err)
		}
	}
	return nil
}

func ensureTemplateColumns(ctx context.Context, db *sql.DB) error {
	type col struct {
		Name string
		SQL  string
	}
	cols := []col{
		{Name: "description", SQL: "ALTER TABLE doc_templates ADD COLUMN description TEXT NOT NULL DEFAULT ''"},
		{Name: "variables", SQL: "ALTER TABLE doc_templates ADD COLUMN variables TEXT NOT NULL DEFAULT '[]'"},
	}
	for _, c := range cols {
		exists, err := columnExists(ctx, db, "doc_templates", c.Name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := db.ExecContext(ctx, c.SQL); err != nil {
			return fmt.Errorf("add column %s: %w", c.Name, err)
		}
	}
	return nil
}

func ensureDocsColumns(ctx context.Context, db *sql.DB) error {
	exists, err := columnExists(ctx, db, "docs", "doc_type")
	if err != nil {
		return err
	}
	if !exists {
		if _, err := db.ExecContext(ctx, "ALTER TABLE docs ADD COLUMN doc_type TEXT NOT NULL DEFAULT 'document'"); err != nil {
			return fmt.Errorf("add column docs.doc_type: %w", err)
		}
	}
	if _, err := db.ExecContext(ctx, "UPDATE docs SET doc_type='document' WHERE doc_type IS NULL OR TRIM(doc_type)=''"); err != nil {
		return fmt.Errorf("normalize docs.doc_type: %w", err)
	}
	return nil
}

func ensureApprovalColumns(ctx context.Context, db *sql.DB) error {
	type col struct {
		Table string
		Name  string
		SQL   string
	}
	cols := []col{
		{Table: "approvals", Name: "current_stage", SQL: "ALTER TABLE approvals ADD COLUMN current_stage INTEGER NOT NULL DEFAULT 1"},
		{Table: "approval_participants", Name: "stage", SQL: "ALTER TABLE approval_participants ADD COLUMN stage INTEGER NOT NULL DEFAULT 1"},
		{Table: "approval_participants", Name: "stage_name", SQL: "ALTER TABLE approval_participants ADD COLUMN stage_name TEXT NOT NULL DEFAULT ''"},
		{Table: "approval_participants", Name: "stage_message", SQL: "ALTER TABLE approval_participants ADD COLUMN stage_message TEXT NOT NULL DEFAULT ''"},
	}
	for _, c := range cols {
		exists, err := columnExists(ctx, db, c.Table, c.Name)
		if err != nil {
			return err
		}
		if !exists {
			if _, err := db.ExecContext(ctx, c.SQL); err != nil {
				return err
			}
		}
	}
	return nil
}

func ensureApprovalParticipantsSchema(ctx context.Context, db *sql.DB) error {
	needRebuild, err := approvalParticipantsNeedsRebuild(ctx, db)
	if err != nil {
		return err
	}
	if !needRebuild {
		return nil
	}
	return rebuildApprovalParticipants(ctx, db)
}

func approvalParticipantsNeedsRebuild(ctx context.Context, db *sql.DB) (bool, error) {
	var sqlText sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name='approval_participants'`).Scan(&sqlText); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	ddl := strings.ToLower(sqlText.String)
	hasStageCol := strings.Contains(ddl, "stage_name") && strings.Contains(ddl, "stage_message")
	hasStageUnique := strings.Contains(ddl, "unique") && strings.Contains(ddl, "role, stage")
	return !(hasStageCol && hasStageUnique), nil
}

func rebuildApprovalParticipants(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS approval_participants_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			approval_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL,
			role TEXT NOT NULL,
			stage INTEGER NOT NULL DEFAULT 1,
			stage_name TEXT NOT NULL DEFAULT '',
			stage_message TEXT NOT NULL DEFAULT '',
			decision TEXT,
			comment TEXT,
			decided_at TIMESTAMP,
			UNIQUE(approval_id, user_id, role, stage),
			FOREIGN KEY(approval_id) REFERENCES approvals(id) ON DELETE CASCADE
		);`)
	if err != nil {
		tx.Rollback()
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO approval_participants_new (id, approval_id, user_id, role, stage, stage_name, stage_message, decision, comment, decided_at)
		SELECT id, approval_id, user_id, role, stage, COALESCE(stage_name, ''), COALESCE(stage_message, ''), decision, comment, decided_at FROM approval_participants;
	`)
	if err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE approval_participants`); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE approval_participants_new RENAME TO approval_participants`); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func columnExists(ctx context.Context, db *sql.DB, table, column string) (bool, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func ensureRoleColumns(ctx context.Context, db *sql.DB) error {
	type col struct {
		Name string
		SQL  string
	}
	cols := []col{
		{Name: "description", SQL: "ALTER TABLE roles ADD COLUMN description TEXT NOT NULL DEFAULT ''"},
		{Name: "permissions", SQL: "ALTER TABLE roles ADD COLUMN permissions TEXT NOT NULL DEFAULT '[]'"},
		{Name: "built_in", SQL: "ALTER TABLE roles ADD COLUMN built_in INTEGER NOT NULL DEFAULT 0"},
		{Name: "template", SQL: "ALTER TABLE roles ADD COLUMN template INTEGER NOT NULL DEFAULT 0"},
		{Name: "created_at", SQL: "ALTER TABLE roles ADD COLUMN created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP"},
		{Name: "updated_at", SQL: "ALTER TABLE roles ADD COLUMN updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP"},
	}
	for _, c := range cols {
		exists, err := columnExists(ctx, db, "roles", c.Name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := db.ExecContext(ctx, c.SQL); err != nil {
			return fmt.Errorf("add column roles.%s: %w", c.Name, err)
		}
	}
	return nil
}

func ensureSessionColumns(ctx context.Context, db *sql.DB) error {
	type col struct {
		Name string
		SQL  string
	}
	cols := []col{
		{Name: "ip", SQL: "ALTER TABLE sessions ADD COLUMN ip TEXT NOT NULL DEFAULT ''"},
		{Name: "user_agent", SQL: "ALTER TABLE sessions ADD COLUMN user_agent TEXT NOT NULL DEFAULT ''"},
		{Name: "created_at", SQL: "ALTER TABLE sessions ADD COLUMN created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP"},
		{Name: "last_seen_at", SQL: "ALTER TABLE sessions ADD COLUMN last_seen_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP"},
		{Name: "revoked", SQL: "ALTER TABLE sessions ADD COLUMN revoked INTEGER NOT NULL DEFAULT 0"},
		{Name: "revoked_at", SQL: "ALTER TABLE sessions ADD COLUMN revoked_at TIMESTAMP"},
		{Name: "revoked_by", SQL: "ALTER TABLE sessions ADD COLUMN revoked_by TEXT NOT NULL DEFAULT ''"},
	}
	for _, c := range cols {
		exists, err := columnExists(ctx, db, "sessions", c.Name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := db.ExecContext(ctx, c.SQL); err != nil {
			return fmt.Errorf("add column sessions.%s: %w", c.Name, err)
		}
	}
	return nil
}

func ensureMonitoringColumns(ctx context.Context, db *sql.DB) error {
	type col struct {
		Table string
		Name  string
		SQL   string
	}
	cols := []col{
		{Table: "monitors", Name: "sla_target_pct", SQL: "ALTER TABLE monitors ADD COLUMN sla_target_pct REAL"},
		{Table: "monitors", Name: "auto_incident", SQL: "ALTER TABLE monitors ADD COLUMN auto_incident INTEGER NOT NULL DEFAULT 0"},
		{Table: "monitors", Name: "auto_task_on_down", SQL: "ALTER TABLE monitors ADD COLUMN auto_task_on_down INTEGER NOT NULL DEFAULT 0"},
		{Table: "monitors", Name: "incident_severity", SQL: "ALTER TABLE monitors ADD COLUMN incident_severity TEXT NOT NULL DEFAULT 'low'"},
		{Table: "monitors", Name: "incident_type_id", SQL: "ALTER TABLE monitors ADD COLUMN incident_type_id TEXT NOT NULL DEFAULT ''"},
		{Table: "monitor_state", Name: "last_result_status", SQL: "ALTER TABLE monitor_state ADD COLUMN last_result_status TEXT NOT NULL DEFAULT ''"},
		{Table: "monitor_state", Name: "maintenance_active", SQL: "ALTER TABLE monitor_state ADD COLUMN maintenance_active INTEGER NOT NULL DEFAULT 0"},
		{Table: "monitor_state", Name: "tls_days_left", SQL: "ALTER TABLE monitor_state ADD COLUMN tls_days_left INTEGER"},
		{Table: "monitor_state", Name: "tls_not_after", SQL: "ALTER TABLE monitor_state ADD COLUMN tls_not_after TIMESTAMP"},
		{Table: "monitoring_settings", Name: "tls_refresh_hours", SQL: "ALTER TABLE monitoring_settings ADD COLUMN tls_refresh_hours INTEGER NOT NULL DEFAULT 24"},
		{Table: "monitoring_settings", Name: "tls_expiring_days", SQL: "ALTER TABLE monitoring_settings ADD COLUMN tls_expiring_days INTEGER NOT NULL DEFAULT 30"},
		{Table: "monitoring_settings", Name: "notify_suppress_minutes", SQL: "ALTER TABLE monitoring_settings ADD COLUMN notify_suppress_minutes INTEGER NOT NULL DEFAULT 5"},
		{Table: "monitoring_settings", Name: "notify_repeat_down_minutes", SQL: "ALTER TABLE monitoring_settings ADD COLUMN notify_repeat_down_minutes INTEGER NOT NULL DEFAULT 30"},
		{Table: "monitoring_settings", Name: "notify_maintenance", SQL: "ALTER TABLE monitoring_settings ADD COLUMN notify_maintenance INTEGER NOT NULL DEFAULT 0"},
		{Table: "monitoring_settings", Name: "auto_task_on_down", SQL: "ALTER TABLE monitoring_settings ADD COLUMN auto_task_on_down INTEGER NOT NULL DEFAULT 1"},
		{Table: "monitoring_settings", Name: "auto_tls_incident", SQL: "ALTER TABLE monitoring_settings ADD COLUMN auto_tls_incident INTEGER NOT NULL DEFAULT 1"},
		{Table: "monitoring_settings", Name: "auto_tls_incident_days", SQL: "ALTER TABLE monitoring_settings ADD COLUMN auto_tls_incident_days INTEGER NOT NULL DEFAULT 14"},
		{Table: "monitors", Name: "ignore_tls_errors", SQL: "ALTER TABLE monitors ADD COLUMN ignore_tls_errors INTEGER NOT NULL DEFAULT 0"},
		{Table: "monitors", Name: "notify_tls_expiring", SQL: "ALTER TABLE monitors ADD COLUMN notify_tls_expiring INTEGER NOT NULL DEFAULT 1"},
		{Table: "monitoring_settings", Name: "default_retries", SQL: "ALTER TABLE monitoring_settings ADD COLUMN default_retries INTEGER NOT NULL DEFAULT 2"},
		{Table: "monitoring_settings", Name: "default_retry_interval_sec", SQL: "ALTER TABLE monitoring_settings ADD COLUMN default_retry_interval_sec INTEGER NOT NULL DEFAULT 30"},
		{Table: "monitoring_settings", Name: "default_sla_target_pct", SQL: "ALTER TABLE monitoring_settings ADD COLUMN default_sla_target_pct REAL NOT NULL DEFAULT 90"},
	}
	for _, c := range cols {
		exists, err := columnExists(ctx, db, c.Table, c.Name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := db.ExecContext(ctx, c.SQL); err != nil {
			return fmt.Errorf("add column %s.%s: %w", c.Table, c.Name, err)
		}
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS monitor_tls (
		monitor_id INTEGER PRIMARY KEY,
		checked_at TIMESTAMP NOT NULL,
		not_after TIMESTAMP NOT NULL,
		not_before TIMESTAMP NOT NULL,
		common_name TEXT NOT NULL DEFAULT '',
		issuer TEXT NOT NULL DEFAULT '',
		san_json TEXT NOT NULL DEFAULT '[]',
		fingerprint_sha256 TEXT NOT NULL DEFAULT '',
		last_error TEXT,
		FOREIGN KEY(monitor_id) REFERENCES monitors(id) ON DELETE CASCADE
	);`); err != nil {
		return err
	}
	monitorTLSCols := []col{
		{Table: "monitor_tls", Name: "checked_at", SQL: "ALTER TABLE monitor_tls ADD COLUMN checked_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP"},
		{Table: "monitor_tls", Name: "not_after", SQL: "ALTER TABLE monitor_tls ADD COLUMN not_after TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP"},
		{Table: "monitor_tls", Name: "not_before", SQL: "ALTER TABLE monitor_tls ADD COLUMN not_before TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP"},
		{Table: "monitor_tls", Name: "common_name", SQL: "ALTER TABLE monitor_tls ADD COLUMN common_name TEXT NOT NULL DEFAULT ''"},
		{Table: "monitor_tls", Name: "issuer", SQL: "ALTER TABLE monitor_tls ADD COLUMN issuer TEXT NOT NULL DEFAULT ''"},
		{Table: "monitor_tls", Name: "san_json", SQL: "ALTER TABLE monitor_tls ADD COLUMN san_json TEXT NOT NULL DEFAULT '[]'"},
		{Table: "monitor_tls", Name: "fingerprint_sha256", SQL: "ALTER TABLE monitor_tls ADD COLUMN fingerprint_sha256 TEXT NOT NULL DEFAULT ''"},
		{Table: "monitor_tls", Name: "last_error", SQL: "ALTER TABLE monitor_tls ADD COLUMN last_error TEXT"},
	}
	for _, c := range monitorTLSCols {
		exists, err := columnExists(ctx, db, c.Table, c.Name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := db.ExecContext(ctx, c.SQL); err != nil {
			return fmt.Errorf("add column %s.%s: %w", c.Table, c.Name, err)
		}
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS monitor_maintenance (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		description_md TEXT NOT NULL DEFAULT '',
		monitor_id INTEGER,
		monitor_ids_json TEXT NOT NULL DEFAULT '[]',
		tags_json TEXT,
		starts_at TIMESTAMP NOT NULL,
		ends_at TIMESTAMP NOT NULL,
		timezone TEXT NOT NULL DEFAULT '',
		strategy TEXT NOT NULL DEFAULT 'single',
		strategy_json TEXT NOT NULL DEFAULT '{}',
		is_recurring INTEGER NOT NULL DEFAULT 0,
		rrule_text TEXT,
		created_by INTEGER,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		is_active INTEGER NOT NULL DEFAULT 1,
		stopped_at TIMESTAMP,
		stopped_by INTEGER,
		FOREIGN KEY(monitor_id) REFERENCES monitors(id) ON DELETE CASCADE
	);`); err != nil {
		return err
	}
	maintenanceCols := []col{
		{Table: "monitor_maintenance", Name: "description_md", SQL: "ALTER TABLE monitor_maintenance ADD COLUMN description_md TEXT NOT NULL DEFAULT ''"},
		{Table: "monitor_maintenance", Name: "monitor_ids_json", SQL: "ALTER TABLE monitor_maintenance ADD COLUMN monitor_ids_json TEXT NOT NULL DEFAULT '[]'"},
		{Table: "monitor_maintenance", Name: "strategy", SQL: "ALTER TABLE monitor_maintenance ADD COLUMN strategy TEXT NOT NULL DEFAULT 'single'"},
		{Table: "monitor_maintenance", Name: "strategy_json", SQL: "ALTER TABLE monitor_maintenance ADD COLUMN strategy_json TEXT NOT NULL DEFAULT '{}'"},
		{Table: "monitor_maintenance", Name: "stopped_at", SQL: "ALTER TABLE monitor_maintenance ADD COLUMN stopped_at TIMESTAMP"},
		{Table: "monitor_maintenance", Name: "stopped_by", SQL: "ALTER TABLE monitor_maintenance ADD COLUMN stopped_by INTEGER"},
	}
	for _, c := range maintenanceCols {
		exists, err := columnExists(ctx, db, c.Table, c.Name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := db.ExecContext(ctx, c.SQL); err != nil {
			return fmt.Errorf("add column %s.%s: %w", c.Table, c.Name, err)
		}
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS notification_channels (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		type TEXT NOT NULL,
		name TEXT NOT NULL,
		telegram_bot_token BLOB NOT NULL,
		telegram_chat_id TEXT NOT NULL,
		telegram_thread_id INTEGER,
		template_text TEXT NOT NULL DEFAULT '',
		quiet_hours_enabled INTEGER NOT NULL DEFAULT 0,
		quiet_hours_start TEXT NOT NULL DEFAULT '',
		quiet_hours_end TEXT NOT NULL DEFAULT '',
		quiet_hours_tz TEXT NOT NULL DEFAULT '',
		silent INTEGER NOT NULL DEFAULT 0,
		protect_content INTEGER NOT NULL DEFAULT 0,
		is_default INTEGER NOT NULL DEFAULT 0,
		created_by INTEGER,
		created_at TIMESTAMP NOT NULL,
		is_active INTEGER NOT NULL DEFAULT 1
	);`); err != nil {
		return err
	}
	notificationCols := []col{
		{Table: "notification_channels", Name: "template_text", SQL: "ALTER TABLE notification_channels ADD COLUMN template_text TEXT NOT NULL DEFAULT ''"},
		{Table: "notification_channels", Name: "quiet_hours_enabled", SQL: "ALTER TABLE notification_channels ADD COLUMN quiet_hours_enabled INTEGER NOT NULL DEFAULT 0"},
		{Table: "notification_channels", Name: "quiet_hours_start", SQL: "ALTER TABLE notification_channels ADD COLUMN quiet_hours_start TEXT NOT NULL DEFAULT ''"},
		{Table: "notification_channels", Name: "quiet_hours_end", SQL: "ALTER TABLE notification_channels ADD COLUMN quiet_hours_end TEXT NOT NULL DEFAULT ''"},
		{Table: "notification_channels", Name: "quiet_hours_tz", SQL: "ALTER TABLE notification_channels ADD COLUMN quiet_hours_tz TEXT NOT NULL DEFAULT ''"},
	}
	for _, c := range notificationCols {
		exists, err := columnExists(ctx, db, c.Table, c.Name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := db.ExecContext(ctx, c.SQL); err != nil {
			return fmt.Errorf("add column %s.%s: %w", c.Table, c.Name, err)
		}
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS monitor_notifications (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		monitor_id INTEGER NOT NULL,
		notification_channel_id INTEGER NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		FOREIGN KEY(monitor_id) REFERENCES monitors(id) ON DELETE CASCADE,
		FOREIGN KEY(notification_channel_id) REFERENCES notification_channels(id) ON DELETE CASCADE
	);`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS monitor_notification_state (
		monitor_id INTEGER PRIMARY KEY,
		last_notified_at TIMESTAMP,
		last_down_notified_at TIMESTAMP,
		last_up_notified_at TIMESTAMP,
		last_tls_notified_at TIMESTAMP,
		last_maintenance_notified_at TIMESTAMP,
		down_started_at TIMESTAMP,
		down_sequence INTEGER NOT NULL DEFAULT 0,
		FOREIGN KEY(monitor_id) REFERENCES monitors(id) ON DELETE CASCADE
	);`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS monitor_notification_deliveries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		monitor_id INTEGER,
		notification_channel_id INTEGER NOT NULL,
		event_type TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT '',
		error_text TEXT NOT NULL DEFAULT '',
		body_preview TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		acknowledged_at TIMESTAMP,
		acknowledged_by INTEGER,
		FOREIGN KEY(monitor_id) REFERENCES monitors(id) ON DELETE SET NULL,
		FOREIGN KEY(notification_channel_id) REFERENCES notification_channels(id) ON DELETE CASCADE
	);`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_monitor_tls_checked ON monitor_tls(checked_at);`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_monitor_maintenance_window ON monitor_maintenance(starts_at, ends_at);`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_notification_channels_default ON notification_channels(is_default, is_active);`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_monitor_notifications_monitor ON monitor_notifications(monitor_id);`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_monitor_notification_deliveries_created ON monitor_notification_deliveries(created_at);`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_monitor_notification_deliveries_status ON monitor_notification_deliveries(status, acknowledged_at);`); err != nil {
		return err
	}
	return nil
}

func ensureGroupColumns(ctx context.Context, db *sql.DB) error {
	type col struct {
		Name string
		SQL  string
	}
	cols := []col{
		{Name: "description", SQL: "ALTER TABLE groups ADD COLUMN description TEXT NOT NULL DEFAULT ''"},
		{Name: "clearance_level", SQL: "ALTER TABLE groups ADD COLUMN clearance_level INTEGER NOT NULL DEFAULT 0"},
		{Name: "clearance_tags", SQL: "ALTER TABLE groups ADD COLUMN clearance_tags TEXT NOT NULL DEFAULT '[]'"},
		{Name: "menu_permissions", SQL: "ALTER TABLE groups ADD COLUMN menu_permissions TEXT NOT NULL DEFAULT '[]'"},
		{Name: "created_at", SQL: "ALTER TABLE groups ADD COLUMN created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP"},
		{Name: "updated_at", SQL: "ALTER TABLE groups ADD COLUMN updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP"},
		{Name: "is_system", SQL: "ALTER TABLE groups ADD COLUMN is_system INTEGER NOT NULL DEFAULT 0"},
	}
	for _, c := range cols {
		exists, err := columnExists(ctx, db, "groups", c.Name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := db.ExecContext(ctx, c.SQL); err != nil {
			return fmt.Errorf("add column groups.%s: %w", c.Name, err)
		}
	}
	return nil
}

func ensureIncidentColumns(ctx context.Context, db *sql.DB) error {
	type col struct {
		Name string
		SQL  string
	}
	cols := []col{
		{Name: "classification_level", SQL: "ALTER TABLE incidents ADD COLUMN classification_level INTEGER NOT NULL DEFAULT 1"},
		{Name: "classification_tags", SQL: "ALTER TABLE incidents ADD COLUMN classification_tags TEXT NOT NULL DEFAULT '[]'"},
		{Name: "meta_json", SQL: "ALTER TABLE incidents ADD COLUMN meta_json TEXT NOT NULL DEFAULT '{}'"},
		{Name: "closed_at", SQL: "ALTER TABLE incidents ADD COLUMN closed_at TIMESTAMP"},
		{Name: "closed_by", SQL: "ALTER TABLE incidents ADD COLUMN closed_by INTEGER"},
		{Name: "source", SQL: "ALTER TABLE incidents ADD COLUMN source TEXT NOT NULL DEFAULT ''"},
		{Name: "source_ref_id", SQL: "ALTER TABLE incidents ADD COLUMN source_ref_id INTEGER"},
	}
	for _, c := range cols {
		exists, err := columnExists(ctx, db, "incidents", c.Name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := db.ExecContext(ctx, c.SQL); err != nil {
			return fmt.Errorf("add column incidents.%s: %w", c.Name, err)
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE incidents SET status='draft' WHERE status IS NULL OR TRIM(status)=''`); err != nil {
		return fmt.Errorf("normalize incident.status: %w", err)
	}
	return nil
}

func ensureIncidentStageColumns(ctx context.Context, db *sql.DB) error {
	type col struct {
		Name string
		SQL  string
	}
	cols := []col{
		{Name: "status", SQL: "ALTER TABLE incident_stages ADD COLUMN status TEXT NOT NULL DEFAULT 'open'"},
		{Name: "closed_at", SQL: "ALTER TABLE incident_stages ADD COLUMN closed_at TIMESTAMP"},
		{Name: "closed_by", SQL: "ALTER TABLE incident_stages ADD COLUMN closed_by INTEGER"},
	}
	for _, c := range cols {
		exists, err := columnExists(ctx, db, "incident_stages", c.Name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := db.ExecContext(ctx, c.SQL); err != nil {
			return fmt.Errorf("add column incident_stages.%s: %w", c.Name, err)
		}
	}
	return nil
}

func ensureIncidentLinkColumns(ctx context.Context, db *sql.DB) error {
	exists, err := columnExists(ctx, db, "incident_links", "comment")
	if err != nil {
		return err
	}
	if !exists {
		if _, err := db.ExecContext(ctx, "ALTER TABLE incident_links ADD COLUMN comment TEXT NOT NULL DEFAULT ''"); err != nil {
			return fmt.Errorf("add column incident_links.comment: %w", err)
		}
	}
	return nil
}

func ensureIncidentTimelineColumns(ctx context.Context, db *sql.DB) error {
	exists, err := columnExists(ctx, db, "incident_timeline", "event_at")
	if err != nil {
		return err
	}
	if !exists {
		if _, err := db.ExecContext(ctx, "ALTER TABLE incident_timeline ADD COLUMN event_at TIMESTAMP"); err != nil {
			return fmt.Errorf("add column incident_timeline.event_at: %w", err)
		}
		if _, err := db.ExecContext(ctx, "UPDATE incident_timeline SET event_at=created_at WHERE event_at IS NULL"); err != nil {
			return fmt.Errorf("backfill incident_timeline.event_at: %w", err)
		}
	}
	return nil
}

func ensureIncidentArtifactFiles(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS incident_artifact_files (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			incident_id INTEGER NOT NULL,
			artifact_id TEXT NOT NULL,
			filename TEXT NOT NULL,
			content_type TEXT NOT NULL,
			size_bytes INTEGER NOT NULL,
			sha256_plain TEXT NOT NULL,
			sha256_cipher TEXT NOT NULL,
			classification_level INTEGER NOT NULL DEFAULT 1,
			classification_tags TEXT NOT NULL DEFAULT '[]',
			uploaded_by INTEGER NOT NULL,
			uploaded_at TIMESTAMP NOT NULL,
			deleted_at TIMESTAMP,
			FOREIGN KEY(incident_id) REFERENCES incidents(id) ON DELETE CASCADE
		);`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_incident_artifact_files_incident ON incident_artifact_files(incident_id)`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_incident_artifact_files_artifact ON incident_artifact_files(artifact_id)`); err != nil {
		return err
	}
	return nil
}

func ensureTaskColumns(ctx context.Context, db *sql.DB) error {
	type col struct {
		Name string
		SQL  string
	}
	cols := []col{
		{Name: "template_id", SQL: "ALTER TABLE tasks ADD COLUMN template_id INTEGER"},
		{Name: "recurring_rule_id", SQL: "ALTER TABLE tasks ADD COLUMN recurring_rule_id INTEGER"},
		{Name: "checklist", SQL: "ALTER TABLE tasks ADD COLUMN checklist TEXT NOT NULL DEFAULT '[]'"},
		{Name: "result", SQL: "ALTER TABLE tasks ADD COLUMN result TEXT NOT NULL DEFAULT ''"},
		{Name: "external_link", SQL: "ALTER TABLE tasks ADD COLUMN external_link TEXT NOT NULL DEFAULT ''"},
		{Name: "business_customer", SQL: "ALTER TABLE tasks ADD COLUMN business_customer TEXT NOT NULL DEFAULT ''"},
		{Name: "size_estimate", SQL: "ALTER TABLE tasks ADD COLUMN size_estimate INTEGER"},
		{Name: "subcolumn_id", SQL: "ALTER TABLE tasks ADD COLUMN subcolumn_id INTEGER"},
	}
	for _, c := range cols {
		exists, err := columnExists(ctx, db, "tasks", c.Name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := db.ExecContext(ctx, c.SQL); err != nil {
			return fmt.Errorf("add column tasks.%s: %w", c.Name, err)
		}
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_tasks_template ON tasks(template_id)`); err != nil {
		return fmt.Errorf("create index tasks.template_id: %w", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_tasks_recurring_rule ON tasks(recurring_rule_id)`); err != nil {
		return fmt.Errorf("create index tasks.recurring_rule_id: %w", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_tasks_subcolumn ON tasks(subcolumn_id)`); err != nil {
		return fmt.Errorf("create index tasks.subcolumn_id: %w", err)
	}
	return nil
}

func ensureTaskCommentColumns(ctx context.Context, db *sql.DB) error {
	exists, err := columnExists(ctx, db, "task_comments", "attachments")
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := db.ExecContext(ctx, "ALTER TABLE task_comments ADD COLUMN attachments TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return fmt.Errorf("add column task_comments.attachments: %w", err)
	}
	return nil
}

func ensureTaskBoardColumns(ctx context.Context, db *sql.DB) error {
	type col struct {
		Name string
		SQL  string
	}
	cols := []col{
		{Name: "space_id", SQL: "ALTER TABLE task_boards ADD COLUMN space_id INTEGER NOT NULL DEFAULT 0"},
		{Name: "position", SQL: "ALTER TABLE task_boards ADD COLUMN position INTEGER NOT NULL DEFAULT 0"},
		{Name: "default_template_id", SQL: "ALTER TABLE task_boards ADD COLUMN default_template_id INTEGER"},
	}
	for _, c := range cols {
		exists, err := columnExists(ctx, db, "task_boards", c.Name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := db.ExecContext(ctx, c.SQL); err != nil {
			return fmt.Errorf("add column task_boards.%s: %w", c.Name, err)
		}
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_task_boards_space ON task_boards(space_id)`); err != nil {
		return fmt.Errorf("create index task_boards.space_id: %w", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_task_boards_position ON task_boards(space_id, position)`); err != nil {
		return fmt.Errorf("create index task_boards.position: %w", err)
	}
	return nil
}

func ensureTaskColumnColumns(ctx context.Context, db *sql.DB) error {
	type col struct {
		Name string
		SQL  string
	}
	cols := []col{
		{Name: "default_template_id", SQL: "ALTER TABLE task_columns ADD COLUMN default_template_id INTEGER"},
	}
	for _, c := range cols {
		exists, err := columnExists(ctx, db, "task_columns", c.Name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := db.ExecContext(ctx, c.SQL); err != nil {
			return fmt.Errorf("add column task_columns.%s: %w", c.Name, err)
		}
	}
	return nil
}

func ensureTaskSpaceBackfill(ctx context.Context, db *sql.DB) error {
	var boardCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM task_boards`).Scan(&boardCount); err != nil {
		return err
	}
	if boardCount == 0 {
		return nil
	}
	fallbackSpaceID, err := ensureDefaultTaskSpace(ctx, db)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, `
		UPDATE task_boards
		SET space_id=?
		WHERE space_id=0
	`, fallbackSpaceID)
	if err != nil {
		return err
	}
	if fixed, _ := res.RowsAffected(); fixed > 0 {
		logMigrationAudit(ctx, db, "migration.task.space.backfill", fmt.Sprintf("fixed_zero_space_id=%d fallback_space_id=%d", fixed, fallbackSpaceID))
	}
	res, err = db.ExecContext(ctx, `
		UPDATE task_boards
		SET space_id=?
		WHERE space_id>0
			AND space_id NOT IN (SELECT id FROM task_spaces)
	`, fallbackSpaceID)
	if err != nil {
		return err
	}
	if fixed, _ := res.RowsAffected(); fixed > 0 {
		logMigrationAudit(ctx, db, "migration.task.space.rebind_missing", fmt.Sprintf("fixed_missing_space=%d fallback_space_id=%d", fixed, fallbackSpaceID))
	}
	if _, err := db.ExecContext(ctx, `
		INSERT OR IGNORE INTO task_space_acl(space_id, subject_type, subject_id, permission)
		SELECT b.space_id, a.subject_type, a.subject_id, a.permission
		FROM task_board_acl a
		JOIN task_boards b ON b.id=a.board_id
		WHERE b.space_id>0
	`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE task_boards
		SET position=COALESCE(position, 0)
		WHERE position IS NULL
	`); err != nil {
		return err
	}
	return nil
}

func ensureDefaultTaskSpace(ctx context.Context, db *sql.DB) (int64, error) {
	var existingID int64
	err := db.QueryRowContext(ctx, `SELECT id FROM task_spaces WHERE is_active=1 ORDER BY created_at ASC, id ASC LIMIT 1`).Scan(&existingID)
	if err == nil && existingID > 0 {
		return existingID, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	now := utils.NowUTC()
	name := "Default space"
	res, err := db.ExecContext(ctx, `
		INSERT INTO task_spaces (organization_id, name, description, layout, created_by, created_at, updated_at, is_active)
		VALUES('', ?, '', 'row', NULL, ?, ?, 1)
	`, name, now, now)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	logMigrationAudit(ctx, db, "migration.task.space.create_default", fmt.Sprintf("space_id=%d", id))
	return id, nil
}

func ensureTaskBoardSpaceGuards(ctx context.Context, db *sql.DB) error {
	triggers := []string{
		`CREATE TRIGGER IF NOT EXISTS trg_task_boards_space_insert
		BEFORE INSERT ON task_boards
		FOR EACH ROW
		WHEN NEW.space_id IS NULL OR NEW.space_id<=0 OR (SELECT COUNT(1) FROM task_spaces WHERE id=NEW.space_id)=0
		BEGIN
			SELECT RAISE(ABORT, 'tasks.spaceRequired');
		END`,
		`CREATE TRIGGER IF NOT EXISTS trg_task_boards_space_update
		BEFORE UPDATE OF space_id ON task_boards
		FOR EACH ROW
		WHEN NEW.space_id IS NULL OR NEW.space_id<=0 OR (SELECT COUNT(1) FROM task_spaces WHERE id=NEW.space_id)=0
		BEGIN
			SELECT RAISE(ABORT, 'tasks.spaceRequired');
		END`,
	}
	for _, sqlStmt := range triggers {
		if _, err := db.ExecContext(ctx, sqlStmt); err != nil {
			return err
		}
	}
	return nil
}

func logMigrationAudit(ctx context.Context, db *sql.DB, action, details string) {
	_, _ = db.ExecContext(ctx, `
		INSERT INTO audit_log(username, action, details, created_at)
		VALUES('system', ?, ?, CURRENT_TIMESTAMP)
	`, action, details)
}

func ensureControlViolationColumns(ctx context.Context, db *sql.DB) error {
	type col struct {
		Name string
		SQL  string
	}
	cols := []col{
		{Name: "is_auto", SQL: "ALTER TABLE control_violations ADD COLUMN is_auto INTEGER NOT NULL DEFAULT 0"},
		{Name: "is_active", SQL: "ALTER TABLE control_violations ADD COLUMN is_active INTEGER NOT NULL DEFAULT 1"},
	}
	for _, c := range cols {
		exists, err := columnExists(ctx, db, "control_violations", c.Name)
		if err != nil {
			return err
		}
		if !exists {
			if _, err := db.ExecContext(ctx, c.SQL); err != nil {
				return fmt.Errorf("add column control_violations.%s: %w", c.Name, err)
			}
		}
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE control_violations
		SET is_active=1
		WHERE is_active IS NULL OR is_active=0
	`); err != nil {
		return fmt.Errorf("backfill control_violations.is_active: %w", err)
	}
	return nil
}
