package backups

import (
	"net/http"

	corebackups "berkut-scc/core/backups"
	"berkut-scc/core/rbac"
	"github.com/go-chi/chi/v5"
)

type RouteDeps struct {
	WithSession       func(http.HandlerFunc) http.HandlerFunc
	RequirePermission func(rbac.Permission) func(http.HandlerFunc) http.HandlerFunc
	Handler           *Handler
}

func RegisterRoutes(deps RouteDeps) http.Handler {
	r := chi.NewRouter()
	h := deps.Handler
	withSession := deps.WithSession
	require := deps.RequirePermission

	r.MethodFunc(http.MethodGet, "/backups", withSession(require(corebackups.PermRead)(h.ListBackups)))
	r.MethodFunc(http.MethodGet, "/backups/integrity", withSession(require(corebackups.PermRead)(h.GetIntegrityStatus)))
	r.MethodFunc(http.MethodPost, "/backups/integrity/run", withSession(require(corebackups.PermRestore)(h.RunIntegrityTest)))
	r.MethodFunc(http.MethodGet, "/backups/plan", withSession(require(corebackups.PermRead)(h.GetPlan)))
	r.MethodFunc(http.MethodPut, "/backups/plan", withSession(require(corebackups.PermPlanUpdate)(h.UpdatePlan)))
	r.MethodFunc(http.MethodPost, "/backups/plan/enable", withSession(require(corebackups.PermPlanUpdate)(h.EnablePlan)))
	r.MethodFunc(http.MethodPost, "/backups/plan/disable", withSession(require(corebackups.PermPlanUpdate)(h.DisablePlan)))
	r.MethodFunc(http.MethodGet, "/backups/{id:[0-9]+}", withSession(require(corebackups.PermRead)(h.GetBackup)))
	r.MethodFunc(http.MethodPost, "/backups", withSession(require(corebackups.PermCreate)(h.CreateBackup)))
	r.MethodFunc(http.MethodPost, "/backups/import", withSession(require(corebackups.PermImport)(h.ImportBackup)))
	r.MethodFunc(http.MethodDelete, "/backups/{id:[0-9]+}", withSession(require(corebackups.PermDelete)(h.DeleteBackup)))
	r.MethodFunc(http.MethodGet, "/backups/{id:[0-9]+}/download", withSession(require(corebackups.PermDownload)(h.DownloadBackup)))
	r.MethodFunc(http.MethodPost, "/backups/{id:[0-9]+}/restore", withSession(require(corebackups.PermRestore)(h.StartRestore)))
	r.MethodFunc(http.MethodPost, "/backups/{id:[0-9]+}/restore/dry-run", withSession(require(corebackups.PermRestore)(h.DryRunRestore)))
	r.MethodFunc(http.MethodGet, "/backups/restores/{restore_id:[0-9]+}", withSession(require(corebackups.PermRead)(h.GetRestoreStatus)))
	return r
}
