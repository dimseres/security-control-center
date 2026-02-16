package backups

import (
	"context"
	"database/sql"
)

func (s *Service) snapshotEntityCounts(ctx context.Context, scope []string) map[string]int64 {
	out := map[string]int64{}
	if s == nil || s.db == nil {
		return out
	}
	if scopeIncludes(scope, "docs") {
		s.addCount(ctx, out, "docs.documents", "SELECT COUNT(*) FROM docs")
		s.addCount(ctx, out, "docs.folders", "SELECT COUNT(*) FROM doc_folders")
	}
	if scopeIncludes(scope, "incidents") {
		s.addCount(ctx, out, "incidents.incidents", "SELECT COUNT(*) FROM incidents")
	}
	if scopeIncludes(scope, "reports") {
		s.addCount(ctx, out, "reports.reports", "SELECT COUNT(*) FROM report_meta")
	}
	if scopeIncludes(scope, "monitoring") {
		s.addCount(ctx, out, "monitoring.monitors", "SELECT COUNT(*) FROM monitors")
	}
	if scopeIncludes(scope, "tasks") {
		s.addCount(ctx, out, "tasks.tasks", "SELECT COUNT(*) FROM tasks")
		s.addCount(ctx, out, "tasks.boards", "SELECT COUNT(*) FROM task_boards WHERE is_active=1")
		s.addCount(ctx, out, "tasks.spaces", "SELECT COUNT(*) FROM task_spaces WHERE is_active=1")
	}
	if scopeIncludes(scope, "controls") {
		s.addCount(ctx, out, "controls.controls", "SELECT COUNT(*) FROM controls")
	}
	if scopeIncludes(scope, "accounts") {
		s.addCount(ctx, out, "accounts.users", "SELECT COUNT(*) FROM users")
		s.addCount(ctx, out, "accounts.groups", "SELECT COUNT(*) FROM groups")
	}
	if scopeIncludes(scope, "approvals") {
		s.addCount(ctx, out, "approvals.approvals", "SELECT COUNT(*) FROM approvals")
	}
	return out
}

func (s *Service) addCount(ctx context.Context, out map[string]int64, key, query string) {
	if out == nil || key == "" || query == "" {
		return
	}
	if n, err := s.queryCount(ctx, query); err == nil {
		out[key] = n
	}
}

func (s *Service) queryCount(ctx context.Context, query string) (int64, error) {
	if s == nil || s.db == nil {
		return 0, sql.ErrConnDone
	}
	var n int64
	if err := s.db.QueryRowContext(ctx, query).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func scopeIncludes(scope []string, item string) bool {
	normalized := normalizedBackupScope(scope)
	if len(normalized) == 1 && normalized[0] == "ALL" {
		return true
	}
	for _, v := range normalized {
		if v == item {
			return true
		}
	}
	return false
}
