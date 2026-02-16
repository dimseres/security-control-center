package backups

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	corebackups "berkut-scc/core/backups"
	"berkut-scc/core/rbac"
)

type routeMockService struct{}

func (m *routeMockService) ListArtifacts(ctx context.Context, filter corebackups.ListArtifactsFilter) ([]corebackups.BackupArtifact, error) {
	return []corebackups.BackupArtifact{}, nil
}
func (m *routeMockService) GetArtifact(ctx context.Context, id int64) (*corebackups.BackupArtifact, error) {
	status := corebackups.StatusSuccess
	return &corebackups.BackupArtifact{ID: id, Status: status}, nil
}
func (m *routeMockService) DownloadArtifact(ctx context.Context, id int64) (*corebackups.DownloadArtifact, error) {
	reader := bytes.NewReader([]byte("data"))
	return &corebackups.DownloadArtifact{
		ID:       id,
		Filename: "backup.bscc",
		Size:     4,
		ModTime:  time.Now().UTC(),
		Reader:   readSeekNopCloser{Reader: reader},
	}, nil
}
func (m *routeMockService) CreateBackup(ctx context.Context) (*corebackups.CreateBackupResult, error) {
	return &corebackups.CreateBackupResult{}, nil
}
func (m *routeMockService) CreateBackupWithOptions(ctx context.Context, opts corebackups.CreateBackupOptions) (*corebackups.CreateBackupResult, error) {
	return m.CreateBackup(ctx)
}
func (m *routeMockService) ImportBackup(ctx context.Context, req corebackups.ImportBackupRequest) (*corebackups.BackupArtifact, error) {
	status := corebackups.StatusSuccess
	return &corebackups.BackupArtifact{ID: 5, Status: status}, nil
}
func (m *routeMockService) RunAutoBackup(ctx context.Context) error { return nil }
func (m *routeMockService) DeleteBackup(ctx context.Context, id int64) error {
	return nil
}
func (m *routeMockService) StartRestore(ctx context.Context, artifactID int64, requestedBy string) (*corebackups.RestoreRun, error) {
	return &corebackups.RestoreRun{ID: 7, ArtifactID: artifactID}, nil
}
func (m *routeMockService) StartRestoreDryRun(ctx context.Context, artifactID int64, requestedBy string) (*corebackups.RestoreRun, error) {
	return &corebackups.RestoreRun{ID: 8, ArtifactID: artifactID, DryRun: true}, nil
}
func (m *routeMockService) GetRestoreRun(ctx context.Context, id int64) (*corebackups.RestoreRun, error) {
	return &corebackups.RestoreRun{ID: id, ArtifactID: 1}, nil
}
func (m *routeMockService) GetPlan(ctx context.Context) (*corebackups.BackupPlan, error) {
	return &corebackups.BackupPlan{}, nil
}
func (m *routeMockService) UpdatePlan(ctx context.Context, plan corebackups.BackupPlan, requestedBy string) (*corebackups.BackupPlan, error) {
	return &plan, nil
}
func (m *routeMockService) EnablePlan(ctx context.Context, requestedBy string) (*corebackups.BackupPlan, error) {
	return &corebackups.BackupPlan{Enabled: true}, nil
}
func (m *routeMockService) DisablePlan(ctx context.Context, requestedBy string) (*corebackups.BackupPlan, error) {
	return &corebackups.BackupPlan{Enabled: false}, nil
}
func (m *routeMockService) IsMaintenanceMode(ctx context.Context) bool { return false }
func (m *routeMockService) UploadMaxBytes() int64                      { return 8 * 1024 * 1024 }

func TestBackupsRoutesAuthAndPermissionGuards(t *testing.T) {
	h := NewHandler(&routeMockService{}, nil)
	router := RegisterRoutes(RouteDeps{
		WithSession: func(next http.HandlerFunc) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				if strings.TrimSpace(r.Header.Get("X-Auth")) == "" {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				next(w, r)
			}
		},
		RequirePermission: func(perm rbac.Permission) func(http.HandlerFunc) http.HandlerFunc {
			return func(next http.HandlerFunc) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("X-Perm") != string(perm) {
						http.Error(w, "forbidden", http.StatusForbidden)
						return
					}
					next(w, r)
				}
			}
		},
		Handler: h,
	})

	tests := []struct {
		method string
		path   string
		perm   string
		bodyFn func() (io.Reader, string)
	}{
		{method: http.MethodGet, path: "/backups", perm: string(corebackups.PermRead)},
		{method: http.MethodGet, path: "/backups/integrity", perm: string(corebackups.PermRead)},
		{method: http.MethodPost, path: "/backups/integrity/run", perm: string(corebackups.PermRestore)},
		{method: http.MethodGet, path: "/backups/1", perm: string(corebackups.PermRead)},
		{method: http.MethodPost, path: "/backups", perm: string(corebackups.PermCreate)},
		{method: http.MethodPost, path: "/backups/import", perm: string(corebackups.PermImport), bodyFn: importBody},
		{method: http.MethodGet, path: "/backups/1/download", perm: string(corebackups.PermDownload)},
		{method: http.MethodDelete, path: "/backups/1", perm: string(corebackups.PermDelete)},
		{method: http.MethodPost, path: "/backups/1/restore", perm: string(corebackups.PermRestore)},
		{method: http.MethodPost, path: "/backups/1/restore/dry-run", perm: string(corebackups.PermRestore)},
		{method: http.MethodGet, path: "/backups/restores/1", perm: string(corebackups.PermRead)},
		{method: http.MethodGet, path: "/backups/plan", perm: string(corebackups.PermRead)},
		{method: http.MethodPut, path: "/backups/plan", perm: string(corebackups.PermPlanUpdate), bodyFn: func() (io.Reader, string) {
			return strings.NewReader(`{"enabled":true}`), "application/json"
		}},
		{method: http.MethodPost, path: "/backups/plan/enable", perm: string(corebackups.PermPlanUpdate)},
		{method: http.MethodPost, path: "/backups/plan/disable", perm: string(corebackups.PermPlanUpdate)},
	}

	for i := range tests {
		tc := tests[i]
		t.Run(strconv.Itoa(i)+"_"+tc.method+"_"+strings.ReplaceAll(tc.path, "/", "_"), func(t *testing.T) {
			reqNoAuth := makeRequest(t, tc.method, tc.path, tc.bodyFn, "")
			rrNoAuth := httptest.NewRecorder()
			router.ServeHTTP(rrNoAuth, reqNoAuth)
			if rrNoAuth.Code != http.StatusUnauthorized {
				t.Fatalf("no auth: expected 401 got %d", rrNoAuth.Code)
			}

			reqNoPerm := makeRequest(t, tc.method, tc.path, tc.bodyFn, "1")
			rrNoPerm := httptest.NewRecorder()
			router.ServeHTTP(rrNoPerm, reqNoPerm)
			if rrNoPerm.Code != http.StatusForbidden {
				t.Fatalf("no perm: expected 403 got %d", rrNoPerm.Code)
			}

			reqOk := makeRequest(t, tc.method, tc.path, tc.bodyFn, "1")
			reqOk.Header.Set("X-Perm", tc.perm)
			rrOK := httptest.NewRecorder()
			router.ServeHTTP(rrOK, reqOk)
			if rrOK.Code == http.StatusUnauthorized || rrOK.Code == http.StatusForbidden {
				t.Fatalf("with perm: expected non-auth error status, got %d", rrOK.Code)
			}
		})
	}
}

func makeRequest(t *testing.T, method, path string, bodyFn func() (io.Reader, string), authHeader string) *http.Request {
	t.Helper()
	var body io.Reader
	contentType := ""
	if bodyFn != nil {
		body, contentType = bodyFn()
	}
	req := httptest.NewRequest(method, path, body)
	if strings.TrimSpace(authHeader) != "" {
		req.Header.Set("X-Auth", authHeader)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return req
}

func importBody() (io.Reader, string) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "upload.bscc")
	_, _ = part.Write([]byte("BSCC"))
	_ = writer.Close()
	return &buf, writer.FormDataContentType()
}

type readSeekNopCloser struct {
	Reader *bytes.Reader
}

func (r readSeekNopCloser) Read(p []byte) (int, error) { return r.Reader.Read(p) }
func (r readSeekNopCloser) Seek(o int64, w int) (int64, error) {
	return r.Reader.Seek(o, w)
}
func (r readSeekNopCloser) Close() error { return nil }
