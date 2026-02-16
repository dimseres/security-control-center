package routegroups

import (
	"berkut-scc/api/handlers"
	"github.com/go-chi/chi/v5"
)

func RegisterDocs(apiRouter chi.Router, g Guards, docs *handlers.DocsHandler, incidents *handlers.IncidentsHandler) {
	apiRouter.Route("/docs/folders", func(foldersRouter chi.Router) {
		foldersRouter.MethodFunc("GET", "/", g.SessionPerm("folders.view", docs.ListFolders))
		foldersRouter.MethodFunc("POST", "/", g.SessionPerm("folders.manage", docs.CreateFolder))
		foldersRouter.MethodFunc("PUT", "/{id}", g.SessionPerm("folders.manage", docs.UpdateFolder))
		foldersRouter.MethodFunc("DELETE", "/{id}", g.SessionPerm("folders.manage", docs.DeleteFolder))
	})

	apiRouter.Route("/docs", func(docsRouter chi.Router) {
		docsRouter.MethodFunc("GET", "/", g.SessionPerm("docs.view", docs.List))
		docsRouter.MethodFunc("POST", "/", g.SessionPerm("docs.create", docs.Create))
		docsRouter.MethodFunc("GET", "/{id:[0-9]+}", g.SessionPerm("docs.view", docs.Get))
		docsRouter.MethodFunc("DELETE", "/{id:[0-9]+}", g.SessionPerm("docs.delete", docs.DeleteDocument))
		docsRouter.MethodFunc("GET", "/{id:[0-9]+}/content", g.SessionPerm("docs.view", docs.GetContent))
		docsRouter.MethodFunc("PUT", "/{id:[0-9]+}/content", g.SessionPerm("docs.edit", docs.UpdateContent))
		docsRouter.MethodFunc("GET", "/{id:[0-9]+}/versions", g.SessionPerm("docs.versions.view", docs.ListVersions))
		docsRouter.MethodFunc("GET", "/{id:[0-9]+}/versions/{ver:[0-9]+}/content", g.SessionPerm("docs.versions.view", docs.GetVersionContent))
		docsRouter.MethodFunc("POST", "/{id:[0-9]+}/versions/{ver:[0-9]+}/restore", g.SessionPerm("docs.versions.restore", docs.RestoreVersion))
		docsRouter.MethodFunc("GET", "/{id:[0-9]+}/export", g.SessionPerm("docs.export", docs.Export))
		docsRouter.MethodFunc("POST", "/{id:[0-9]+}/export-approve", g.SessionPerm("docs.export", docs.ApproveExport))
		docsRouter.MethodFunc("POST", "/{id:[0-9]+}/security-events", g.SessionPerm("docs.view", docs.LogSecurityEvent))
		docsRouter.MethodFunc("GET", "/{id:[0-9]+}/acl", g.SessionPerm("docs.manage", docs.GetACL))
		docsRouter.MethodFunc("PUT", "/{id:[0-9]+}/acl", g.SessionPerm("docs.manage", docs.UpdateACL))
		docsRouter.MethodFunc("POST", "/{id:[0-9]+}/classification", g.SessionPerm("docs.classification.set", docs.UpdateClassification))
		docsRouter.MethodFunc("POST", "/{id:[0-9]+}/approval/start", g.SessionPerm("docs.approval.start", docs.StartApproval))
		docsRouter.MethodFunc("GET", "/{id:[0-9]+}/links", g.SessionPerm("docs.view", docs.ListLinks))
		docsRouter.MethodFunc("POST", "/{id:[0-9]+}/links", g.SessionPerm("docs.edit", docs.AddLink))
		docsRouter.MethodFunc("DELETE", "/{id:[0-9]+}/links/{link_id:[0-9]+}", g.SessionPerm("docs.edit", docs.DeleteLink))
		docsRouter.MethodFunc("GET", "/{id:[0-9]+}/control-links", g.SessionPerm("docs.view", docs.ListControlLinks))
		docsRouter.MethodFunc("POST", "/{id:[0-9]+}/convert", g.SessionPerm("docs.edit", docs.ConvertToMarkdown))
		docsRouter.MethodFunc("GET", "/{id:[0-9]+}/office/config", g.SessionPerm("docs.view", docs.OnlyOfficeConfig))
		docsRouter.MethodFunc("GET", "/{id:[0-9]+}/office/file", g.SessionPerm("docs.view", docs.OnlyOfficeFile))
		docsRouter.MethodFunc("POST", "/{id:[0-9]+}/office/callback", g.SessionPerm("docs.edit", docs.OnlyOfficeCallback))
		docsRouter.MethodFunc("POST", "/{id:[0-9]+}/office/forcesave", g.SessionPerm("docs.edit", docs.OnlyOfficeForceSave))
		docsRouter.MethodFunc("POST", "/upload", g.SessionPerm("docs.upload", docs.Upload))
		docsRouter.MethodFunc("POST", "/import/commit", g.SessionPerm("docs.upload", docs.ImportCommit))
		docsRouter.MethodFunc("GET", "/list", g.SessionPerm("docs.view", incidents.ListDocsLite))
	})
}

func RegisterReports(apiRouter chi.Router, g Guards, reports *handlers.ReportsHandler) {
	apiRouter.Route("/reports", func(reportsRouter chi.Router) {
		reportsRouter.MethodFunc("GET", "/", g.SessionPerm("reports.view", reports.List))
		reportsRouter.MethodFunc("GET", "/export", g.SessionPerm("reports.export", reports.ExportBundle))
		reportsRouter.MethodFunc("GET", "/audit-package", g.SessionPerm("reports.export", reports.ExportAuditPackage))
		reportsRouter.MethodFunc("POST", "/", g.SessionPerm("reports.create", reports.Create))
		reportsRouter.MethodFunc("GET", "/{id:[0-9]+}", g.SessionPerm("reports.view", reports.Get))
		reportsRouter.MethodFunc("PUT", "/{id:[0-9]+}", g.SessionPerm("reports.edit", reports.UpdateMeta))
		reportsRouter.MethodFunc("DELETE", "/{id:[0-9]+}", g.SessionPerm("reports.delete", reports.Delete))
		reportsRouter.MethodFunc("GET", "/{id:[0-9]+}/content", g.SessionPerm("reports.view", reports.GetContent))
		reportsRouter.MethodFunc("PUT", "/{id:[0-9]+}/content", g.SessionPerm("reports.edit", reports.UpdateContent))
		reportsRouter.MethodFunc("GET", "/{id:[0-9]+}/export", g.SessionPerm("reports.export", reports.Export))
		reportsRouter.MethodFunc("GET", "/{id:[0-9]+}/sections", g.SessionPerm("reports.view", reports.ListSections))
		reportsRouter.MethodFunc("PUT", "/{id:[0-9]+}/sections", g.SessionPerm("reports.edit", reports.UpdateSections))
		reportsRouter.MethodFunc("GET", "/{id:[0-9]+}/charts", g.SessionPerm("reports.view", reports.ListCharts))
		reportsRouter.MethodFunc("PUT", "/{id:[0-9]+}/charts", g.SessionPerm("reports.edit", reports.UpdateCharts))
		reportsRouter.MethodFunc("GET", "/{id:[0-9]+}/charts/{chart_id:[0-9]+}/render", g.SessionPerm("reports.view", reports.RenderChart))
		reportsRouter.MethodFunc("POST", "/{id:[0-9]+}/build", g.SessionPerm("reports.edit", reports.Build))
		reportsRouter.MethodFunc("GET", "/{id:[0-9]+}/snapshots", g.SessionPerm("reports.view", reports.ListSnapshots))
		reportsRouter.MethodFunc("GET", "/{id:[0-9]+}/snapshots/{snapshot_id:[0-9]+}", g.SessionPerm("reports.view", reports.GetSnapshot))
		reportsRouter.MethodFunc("GET", "/templates", g.SessionPerm("reports.templates.view", reports.ListTemplates))
		reportsRouter.MethodFunc("POST", "/templates", g.SessionPerm("reports.templates.manage", reports.SaveTemplate))
		reportsRouter.MethodFunc("PUT", "/templates/{id:[0-9]+}", g.SessionPerm("reports.templates.manage", reports.SaveTemplate))
		reportsRouter.MethodFunc("DELETE", "/templates/{id:[0-9]+}", g.SessionPerm("reports.templates.manage", reports.DeleteTemplate))
		reportsRouter.MethodFunc("POST", "/from-template", g.SessionPerm("reports.create", reports.CreateFromTemplate))
		reportsRouter.MethodFunc("POST", "/from-incident", g.SessionPerm("reports.create", reports.CreateFromIncident))
		reportsRouter.MethodFunc("GET", "/settings", g.SessionPerm("reports.templates.manage", reports.GetSettings))
		reportsRouter.MethodFunc("PUT", "/settings", g.SessionPerm("reports.templates.manage", reports.UpdateSettings))
	})
}
