package appbootstrap

import (
	"database/sql"

	"berkut-scc/api"
	"berkut-scc/config"
	"berkut-scc/core/appmeta"
	"berkut-scc/core/backups"
	backupsstore "berkut-scc/core/backups/store"
	"berkut-scc/core/docs"
	"berkut-scc/core/incidents"
	"berkut-scc/core/monitoring"
	"berkut-scc/core/store"
	"berkut-scc/core/utils"
	"berkut-scc/tasks"
	taskstore "berkut-scc/tasks/store"
)

type runtimeComposition struct {
	serverDeps api.ServerDeps
	sessions   store.SessionStore
	workers    []api.BackgroundWorker
}

func composeRuntime(cfg *config.AppConfig, db *sql.DB, logger *utils.Logger) (*runtimeComposition, error) {
	users := store.NewUsersStore(db)
	sessions := store.NewSessionsStore(db)
	roles := store.NewRolesStore(db)
	groups := store.NewGroupsStore(db)
	audits := store.NewAuditStore(db)
	docsStore := store.NewDocsStore(db)
	reportsStore := store.NewReportsStore(db)
	incidentsStore := store.NewIncidentsStore(db)
	controlsStore := store.NewControlsStore(db)
	entityLinks := store.NewEntityLinksStore(db)
	monitoringStore := store.NewMonitoringStore(db)
	appHTTPSStore := store.NewAppHTTPSStore(db)
	appRuntimeStore := store.NewAppRuntimeStore(db)
	updateChecker := appmeta.NewUpdateChecker()
	dashboardStore := store.NewDashboardStore(db)
	backupsRepo := backupsstore.NewRepository(db)
	backupsSvc := backups.NewService(cfg, db, backupsRepo, audits, logger)
	backupsScheduler := backups.NewScheduler(cfg.Scheduler, backupsSvc)
	tasksStore := taskstore.NewStore(db)
	tasksSvc := tasks.NewService(tasksStore)

	docsSvc, err := docs.NewService(cfg, docsStore, users, audits, logger)
	if err != nil {
		return nil, err
	}
	incidentsSvc, err := incidents.NewService(cfg, audits)
	if err != nil {
		return nil, err
	}
	tasksScheduler := tasks.NewRecurringScheduler(cfg.Scheduler, tasksSvc.Store(), audits, logger)
	monitoringEngine := monitoring.NewEngineWithDeps(
		monitoringStore,
		incidentsStore,
		audits,
		cfg.Incidents.RegNoFormat,
		incidentsSvc.Encryptor(),
		monitoring.NewHTTPTelegramSender(),
		logger,
	)
	monitoringEngine.SetTaskStore(tasksStore)

	return &runtimeComposition{
		serverDeps: api.ServerDeps{
			Users:            users,
			Sessions:         sessions,
			Roles:            roles,
			Groups:           groups,
			Audits:           audits,
			DocsStore:        docsStore,
			ReportsStore:     reportsStore,
			IncidentsStore:   incidentsStore,
			ControlsStore:    controlsStore,
			EntityLinksStore: entityLinks,
			MonitoringStore:  monitoringStore,
			AppHTTPSStore:    appHTTPSStore,
			AppRuntimeStore:  appRuntimeStore,
			UpdateChecker:    updateChecker,
			DashboardStore:   dashboardStore,
			BackupsSvc:       backupsSvc,
			DocsSvc:          docsSvc,
			IncidentsSvc:     incidentsSvc,
			TasksStore:       tasksStore,
			TasksSvc:         tasksSvc,
			MonitoringEngine: monitoringEngine,
		},
		sessions: sessions,
		workers:  []api.BackgroundWorker{tasksScheduler, monitoringEngine, backupsScheduler},
	}, nil
}
