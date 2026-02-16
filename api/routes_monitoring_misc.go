package api

import (
	"net/http"

	"berkut-scc/api/routegroups"
	"berkut-scc/core/rbac"
	taskhttp "berkut-scc/tasks/http"
	"github.com/go-chi/chi/v5"
)

func (s *Server) registerMonitoringRoutes(apiRouter chi.Router, h routeHandlers) {
	routegroups.RegisterMonitoring(apiRouter, routegroups.Guards{
		WithSession:       s.withSession,
		RequirePermission: func(p string) func(http.HandlerFunc) http.HandlerFunc { return s.requirePermission(rbac.Permission(p)) },
	}, h.monitoring)
}

func (s *Server) registerTasksRoutes(apiRouter chi.Router) {
	taskHandler := taskhttp.NewHandler(s.cfg, s.tasksSvc, s.users, s.docsStore, s.docsSvc, s.incidentsStore, s.incidentsSvc, s.controlsStore, s.entityLinksStore, s.policy, s.audits)
	tasksRouter := taskhttp.RegisterRoutes(taskhttp.RouteDeps{
		WithSession:       s.withSession,
		RequirePermission: s.requirePermission,
		Handler:           taskHandler,
	})
	apiRouter.Handle("/tasks", http.StripPrefix("/api", tasksRouter))
	apiRouter.Handle("/tasks/*", http.StripPrefix("/api", tasksRouter))
}

func (s *Server) registerTemplatesAndApprovalsRoutes(apiRouter chi.Router, h routeHandlers) {
	routegroups.RegisterTemplatesAndApprovals(apiRouter, routegroups.Guards{
		WithSession:       s.withSession,
		RequirePermission: func(p string) func(http.HandlerFunc) http.HandlerFunc { return s.requirePermission(rbac.Permission(p)) },
	}, h.docs)
}

func (s *Server) registerLogsAndSettingsRoutes(apiRouter chi.Router, h routeHandlers) {
	routegroups.RegisterLogsAndSettings(apiRouter, routegroups.Guards{
		WithSession:       s.withSession,
		RequirePermission: func(p string) func(http.HandlerFunc) http.HandlerFunc { return s.requirePermission(rbac.Permission(p)) },
	}, h.logs, h.https, h.runtime, h.hardening)
}
