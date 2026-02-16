package routegroups

import (
	"berkut-scc/api/handlers"
	"github.com/go-chi/chi/v5"
)

func RegisterMonitoring(apiRouter chi.Router, g Guards, monitoring *handlers.MonitoringHandler) {
	apiRouter.Route("/monitoring", func(monitoringRouter chi.Router) {
		monitoringRouter.MethodFunc("GET", "/monitors", g.SessionPerm("monitoring.view", monitoring.ListMonitors))
		monitoringRouter.MethodFunc("POST", "/monitors", g.SessionPerm("monitoring.manage", monitoring.CreateMonitor))
		monitoringRouter.MethodFunc("GET", "/monitors/{id:[0-9]+}", g.SessionPerm("monitoring.view", monitoring.GetMonitor))
		monitoringRouter.MethodFunc("PUT", "/monitors/{id:[0-9]+}", g.SessionPerm("monitoring.manage", monitoring.UpdateMonitor))
		monitoringRouter.MethodFunc("DELETE", "/monitors/{id:[0-9]+}", g.SessionPerm("monitoring.manage", monitoring.DeleteMonitor))
		monitoringRouter.MethodFunc("POST", "/monitors/{id:[0-9]+}/pause", g.SessionPerm("monitoring.manage", monitoring.PauseMonitor))
		monitoringRouter.MethodFunc("POST", "/monitors/{id:[0-9]+}/resume", g.SessionPerm("monitoring.manage", monitoring.ResumeMonitor))
		monitoringRouter.MethodFunc("POST", "/monitors/{id:[0-9]+}/check-now", g.SessionPerm("monitoring.manage", monitoring.CheckNow))
		monitoringRouter.MethodFunc("POST", "/monitors/{id:[0-9]+}/push", g.SessionPerm("monitoring.manage", monitoring.PushMonitor))
		monitoringRouter.MethodFunc("POST", "/monitors/{id:[0-9]+}/clone", g.SessionPerm("monitoring.manage", monitoring.CloneMonitor))
		monitoringRouter.MethodFunc("PUT", "/monitors/{id:[0-9]+}/sla-policy", g.SessionPerm("monitoring.manage", monitoring.UpdateMonitorSLAPolicy))
		monitoringRouter.MethodFunc("GET", "/monitors/{id:[0-9]+}/state", g.SessionPerm("monitoring.view", monitoring.GetState))
		monitoringRouter.MethodFunc("GET", "/monitors/{id:[0-9]+}/metrics", g.SessionPerm("monitoring.view", monitoring.GetMetrics))
		monitoringRouter.MethodFunc("DELETE", "/monitors/{id:[0-9]+}/metrics", g.SessionPerm("monitoring.manage", monitoring.DeleteMonitorMetrics))
		monitoringRouter.MethodFunc("GET", "/monitors/{id:[0-9]+}/events", g.SessionPerm("monitoring.events.view", monitoring.GetEvents))
		monitoringRouter.MethodFunc("DELETE", "/monitors/{id:[0-9]+}/events", g.SessionPerm("monitoring.manage", monitoring.DeleteMonitorEvents))
		monitoringRouter.MethodFunc("GET", "/monitors/{id:[0-9]+}/tls", g.SessionPerm("monitoring.certs.view", monitoring.GetTLS))
		monitoringRouter.MethodFunc("GET", "/certs", g.SessionPerm("monitoring.certs.view", monitoring.ListCerts))
		monitoringRouter.MethodFunc("POST", "/certs/test-notification", g.SessionPerm("monitoring.certs.manage", monitoring.TestCertNotification))
		monitoringRouter.MethodFunc("GET", "/events", g.SessionPerm("monitoring.events.view", monitoring.EventsFeed))
		monitoringRouter.MethodFunc("GET", "/sla/overview", g.SessionPerm("monitoring.view", monitoring.ListSLAOverview))
		monitoringRouter.MethodFunc("GET", "/sla/history", g.SessionPerm("monitoring.view", monitoring.ListSLAHistory))
		monitoringRouter.MethodFunc("GET", "/maintenance", g.SessionPerm("monitoring.maintenance.view", monitoring.ListMaintenance))
		monitoringRouter.MethodFunc("POST", "/maintenance", g.SessionPerm("monitoring.maintenance.manage", monitoring.CreateMaintenance))
		monitoringRouter.MethodFunc("PUT", "/maintenance/{id:[0-9]+}", g.SessionPerm("monitoring.maintenance.manage", monitoring.UpdateMaintenance))
		monitoringRouter.MethodFunc("POST", "/maintenance/{id:[0-9]+}/stop", g.SessionPerm("monitoring.maintenance.manage", monitoring.StopMaintenance))
		monitoringRouter.MethodFunc("DELETE", "/maintenance/{id:[0-9]+}", g.SessionPerm("monitoring.maintenance.manage", monitoring.DeleteMaintenance))
		monitoringRouter.MethodFunc("GET", "/settings", g.SessionPerm("monitoring.settings.manage", monitoring.GetSettings))
		monitoringRouter.MethodFunc("PUT", "/settings", g.SessionPerm("monitoring.settings.manage", monitoring.UpdateSettings))
		monitoringRouter.MethodFunc("GET", "/notifications", g.SessionPerm("monitoring.notifications.view", monitoring.ListNotificationChannels))
		monitoringRouter.MethodFunc("POST", "/notifications", g.SessionPerm("monitoring.notifications.manage", monitoring.CreateNotificationChannel))
		monitoringRouter.MethodFunc("PUT", "/notifications/{id:[0-9]+}", g.SessionPerm("monitoring.notifications.manage", monitoring.UpdateNotificationChannel))
		monitoringRouter.MethodFunc("DELETE", "/notifications/{id:[0-9]+}", g.SessionPerm("monitoring.notifications.manage", monitoring.DeleteNotificationChannel))
		monitoringRouter.MethodFunc("POST", "/notifications/{id:[0-9]+}/test", g.SessionPerm("monitoring.notifications.manage", monitoring.TestNotificationChannel))
		monitoringRouter.MethodFunc("GET", "/notifications/deliveries", g.SessionPerm("monitoring.notifications.view", monitoring.ListNotificationDeliveries))
		monitoringRouter.MethodFunc("POST", "/notifications/deliveries/{id:[0-9]+}/ack", g.SessionPerm("monitoring.notifications.manage", monitoring.AcknowledgeNotificationDelivery))
		monitoringRouter.MethodFunc("GET", "/monitors/{id:[0-9]+}/notifications", g.SessionPerm("monitoring.notifications.view", monitoring.ListMonitorNotifications))
		monitoringRouter.MethodFunc("PUT", "/monitors/{id:[0-9]+}/notifications", g.SessionPerm("monitoring.notifications.manage", monitoring.UpdateMonitorNotifications))
	})
}

func RegisterTemplatesAndApprovals(apiRouter chi.Router, g Guards, docs *handlers.DocsHandler) {
	apiRouter.Route("/templates", func(templatesRouter chi.Router) {
		templatesRouter.MethodFunc("GET", "/", g.SessionPerm("templates.view", docs.ListTemplates))
		templatesRouter.MethodFunc("POST", "/", g.SessionPerm("templates.manage", docs.SaveTemplate))
		templatesRouter.MethodFunc("DELETE", "/{id}", g.SessionPerm("templates.manage", docs.DeleteTemplate))
	})

	apiRouter.Route("/approvals", func(approvalsRouter chi.Router) {
		approvalsRouter.MethodFunc("POST", "/cleanup", g.SessionPerm("docs.approval.view", docs.CleanupApprovals))
		approvalsRouter.MethodFunc("GET", "/", g.SessionPerm("docs.approval.view", docs.ListApprovals))
		approvalsRouter.MethodFunc("GET", "/{approval_id}", g.SessionPerm("docs.approval.view", docs.GetApproval))
		approvalsRouter.MethodFunc("POST", "/{approval_id}/decision", g.SessionPerm("docs.approval.approve", docs.ApprovalDecision))
		approvalsRouter.MethodFunc("GET", "/{approval_id}/comments", g.SessionPerm("docs.approval.view", docs.ListApprovalComments))
		approvalsRouter.MethodFunc("POST", "/{approval_id}/comments", g.SessionPerm("docs.approval.view", docs.AddApprovalComment))
	})
}

func RegisterLogsAndSettings(apiRouter chi.Router, g Guards, logs *handlers.LogsHandler, https *handlers.HTTPSSettingsHandler, runtime *handlers.RuntimeSettingsHandler, hardening *handlers.HardeningHandler) {
	apiRouter.Route("/logs", func(logsRouter chi.Router) {
		logsRouter.MethodFunc("GET", "/", g.SessionPerm("logs.view", logs.List))
		logsRouter.MethodFunc("GET", "/export", g.SessionPerm("logs.view", logs.Export))
	})

	apiRouter.MethodFunc("GET", "/settings/https", g.SessionPerm("settings.advanced", https.Get))
	apiRouter.MethodFunc("PUT", "/settings/https", g.SessionPerm("settings.advanced", https.Update))
	apiRouter.MethodFunc("GET", "/settings/runtime", g.SessionPerm("settings.advanced", runtime.Get))
	apiRouter.MethodFunc("PUT", "/settings/runtime", g.SessionPerm("settings.advanced", runtime.Update))
	apiRouter.MethodFunc("GET", "/settings/hardening", g.SessionPerm("settings.advanced", hardening.GetBaseline))
	apiRouter.MethodFunc("POST", "/settings/updates/check", g.SessionPerm("settings.advanced", runtime.CheckUpdates))
}
