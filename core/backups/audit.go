package backups

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"berkut-scc/core/store"
)

const (
	AuditListBackups       = "backups.list"
	AuditReadBackup        = "backups.read"
	AuditCreateBackup      = "backups.create"
	AuditCreateRequested   = "backups.create.requested"
	AuditCreateSuccess     = "backups.create.success"
	AuditCreateFailed      = "backups.create.failed"
	AuditPlanRead          = "backups.plan.read"
	AuditPlanUpdate        = "backups.plan.update"
	AuditPlanEnable        = "backups.plan.enable"
	AuditPlanDisable       = "backups.plan.disable"
	AuditDeleteBackup      = "backups.delete"
	AuditDownloadBackup    = "backups.download"
	AuditDownloadRequested = "backups.download.requested"
	AuditDownloadSuccess   = "backups.download.success"
	AuditDownloadFailed    = "backups.download.failed"
	AuditStartRestore      = "backups.restore.start"
	AuditRestoreRequested  = "backups.restore.requested"
	AuditRestoreDryRun     = "backups.restore.dry_run.requested"
	AuditRestoreDryRunAuto = "backups.restore.dry_run.auto"
	AuditRestoreSuccess    = "backups.restore.success"
	AuditRestoreFailed     = "backups.restore.failed"
	AuditMaintenanceEnter  = "backups.maintenance.enter"
	AuditMaintenanceExit   = "backups.maintenance.exit"
	AuditReadRestoreStatus = "backups.restore.read"
	AuditAutoStarted       = "backups.auto.started"
	AuditAutoSuccess       = "backups.auto.success"
	AuditAutoFailed        = "backups.auto.failed"
	AuditRetentionDeleted  = "backups.retention.deleted"
	AuditImportRequested   = "backups.import.requested"
	AuditImportSuccess     = "backups.import.success"
	AuditImportFailed      = "backups.import.failed"
)

func Log(audits store.AuditStore, ctx context.Context, username, action, result, details string) {
	if audits == nil {
		return
	}
	payload := "result=" + result
	if details != "" {
		payload = payload + " " + details
	}
	_ = audits.Log(ctx, username, action, payload)
}

func AuditActionForRequest(r *http.Request) string {
	segments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case r.Method == http.MethodGet && len(segments) == 2 && segments[0] == "api" && segments[1] == "backups":
		return AuditListBackups
	case r.Method == http.MethodGet && len(segments) == 5 && segments[0] == "api" && segments[1] == "backups" && segments[2] == "restores":
		return AuditReadRestoreStatus
	case r.Method == http.MethodPost && r.URL.Path == "/api/backups":
		return AuditCreateBackup
	case r.Method == http.MethodPost && r.URL.Path == "/api/backups/import":
		return AuditImportRequested
	case r.Method == http.MethodGet && r.URL.Path == "/api/backups/plan":
		return AuditPlanRead
	case r.Method == http.MethodPut && r.URL.Path == "/api/backups/plan":
		return AuditPlanUpdate
	case r.Method == http.MethodPost && r.URL.Path == "/api/backups/plan/enable":
		return AuditPlanEnable
	case r.Method == http.MethodPost && r.URL.Path == "/api/backups/plan/disable":
		return AuditPlanDisable
	case r.Method == http.MethodDelete && len(segments) == 3 && segments[0] == "api" && segments[1] == "backups":
		return AuditDeleteBackup
	case r.Method == http.MethodGet && len(segments) == 4 && segments[0] == "api" && segments[1] == "backups" && segments[3] == "download":
		return AuditDownloadBackup
	case r.Method == http.MethodPost && len(segments) == 4 && segments[0] == "api" && segments[1] == "backups" && segments[3] == "restore":
		return AuditStartRestore
	case r.Method == http.MethodPost && len(segments) == 5 && segments[0] == "api" && segments[1] == "backups" && segments[3] == "restore" && segments[4] == "dry-run":
		return AuditRestoreDryRun
	case r.Method == http.MethodGet && len(segments) == 3 && segments[0] == "api" && segments[1] == "backups":
		return AuditReadBackup
	default:
		return "backups.unknown"
	}
}

func NotImplementedDetails(id int64) string {
	if id <= 0 {
		return ""
	}
	return fmt.Sprintf("id=%d", id)
}
