package api

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRoutegroupsRequireSessionGuards(t *testing.T) {
	root := projectRoot(t)
	dir := filepath.Join(root, "api", "routegroups")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read routegroups dir: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		lines := readLines(t, path)
		for i, line := range lines {
			if !strings.Contains(line, ".MethodFunc(") {
				continue
			}
			if strings.Contains(line, "g.SessionPerm(") || strings.Contains(line, "g.SessionAnyPerm(") {
				continue
			}
			t.Fatalf("unguarded routegroup handler in %s:%d -> %s", path, i+1, strings.TrimSpace(line))
		}
	}
}

func TestTasksRoutesRequireSessionAndPermission(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "tasks", "http", "router.go")
	lines := readLines(t, path)
	found := 0
	for i, line := range lines {
		if !(strings.Contains(line, "r.Get(") || strings.Contains(line, "r.Post(") || strings.Contains(line, "r.Put(") || strings.Contains(line, "r.Delete(")) {
			continue
		}
		found++
		if strings.Contains(line, "withSession(require(") {
			continue
		}
		t.Fatalf("unguarded tasks route in %s:%d -> %s", path, i+1, strings.TrimSpace(line))
	}
	if found == 0 {
		t.Fatalf("no tasks routes found in %s", path)
	}
}

func TestBackupsRoutesRequireSessionAndPermission(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "api", "backups", "router.go")
	lines := readLines(t, path)
	found := 0
	for i, line := range lines {
		if !strings.Contains(line, "r.MethodFunc(") {
			continue
		}
		found++
		if strings.Contains(line, "withSession(require(") {
			continue
		}
		t.Fatalf("unguarded backups route in %s:%d -> %s", path, i+1, strings.TrimSpace(line))
	}
	if found == 0 {
		t.Fatalf("no backups routes found in %s", path)
	}
}

func TestCoreAPIRoutesHaveSessionGuards(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "api", "routes_shell_core.go")
	lines := readLines(t, path)
	for i, line := range lines {
		switch {
		case strings.Contains(line, "apiRouter.MethodFunc("):
			if strings.Contains(line, "\"/auth/login\"") {
				continue
			}
			if strings.Contains(line, "s.withSession(") {
				continue
			}
			t.Fatalf("core api route missing session guard in %s:%d -> %s", path, i+1, strings.TrimSpace(line))
		case strings.Contains(line, "pageRouter.MethodFunc("):
			if strings.Contains(line, "s.withSession(") {
				continue
			}
			t.Fatalf("page route missing session guard in %s:%d -> %s", path, i+1, strings.TrimSpace(line))
		}
	}
}

func TestShellTabRoutesHaveSessionGuard(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "api", "routes_shell_tabs.go")
	lines := readLines(t, path)
	found := 0
	for i, line := range lines {
		if !strings.Contains(line, "s.router.MethodFunc(") {
			continue
		}
		found++
		if strings.Contains(line, "appShell") || strings.Contains(line, "s.withSession(s.requirePermission(\"app.view\")(") {
			continue
		}
		t.Fatalf("shell tab route missing session/app.view guard in %s:%d -> %s", path, i+1, strings.TrimSpace(line))
	}
	if found == 0 {
		t.Fatalf("no shell tab routes found in %s", path)
	}
}

func projectRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime caller unavailable")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), ".."))
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return lines
}
