package backups

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"berkut-scc/config"
	"berkut-scc/core/appmeta"
	backupcrypto "berkut-scc/core/backups/crypto"
	"berkut-scc/core/backups/format"
	"berkut-scc/core/backups/pgdump"
	"berkut-scc/core/backups/pgrestore"
	"berkut-scc/core/store"
	"berkut-scc/core/utils"
)

type Service struct {
	cfg       *config.AppConfig
	db        *sql.DB
	repo      Repository
	dumper    pgdump.Runner
	restorer  pgrestore.Runner
	audits    store.AuditStore
	logger    *utils.Logger
	limiter   chan struct{}
	opMu      sync.Mutex
	opName    string
	downloads map[int64]int
	restoreMu sync.RWMutex
	restores  map[int64]RestoreRun
}

func NewService(cfg *config.AppConfig, db *sql.DB, repo Repository, audits store.AuditStore, logger *utils.Logger) *Service {
	maxParallel := 1
	if cfg != nil && cfg.Backups.MaxParallel > 0 {
		maxParallel = cfg.Backups.MaxParallel
	}
	return &Service{
		cfg:       cfg,
		db:        db,
		repo:      repo,
		dumper:    pgdump.NewRunner(),
		restorer:  pgrestore.NewRunner(),
		audits:    audits,
		logger:    logger,
		limiter:   make(chan struct{}, maxParallel),
		downloads: make(map[int64]int),
		restores:  make(map[int64]RestoreRun),
	}
}

func (s *Service) ListArtifacts(ctx context.Context, filter ListArtifactsFilter) ([]BackupArtifact, error) {
	if s == nil || s.repo == nil {
		return []BackupArtifact{}, nil
	}
	return s.repo.ListArtifacts(ctx, filter)
}

func (s *Service) GetArtifact(ctx context.Context, id int64) (*BackupArtifact, error) {
	if s == nil || s.repo == nil {
		return nil, ErrNotFound
	}
	return s.repo.GetArtifact(ctx, id)
}

func (s *Service) CreateBackup(ctx context.Context) (*CreateBackupResult, error) {
	return s.CreateBackupWithOptions(ctx, CreateBackupOptions{})
}

func (s *Service) CreateBackupWithOptions(ctx context.Context, opts CreateBackupOptions) (*CreateBackupResult, error) {
	if s == nil || s.repo == nil || s.cfg == nil {
		return nil, NewDomainError(ErrorCodeStorageMissing, ErrorKeyStorageMissing)
	}
	if err := s.beginRun(ctx); err != nil {
		return nil, err
	}
	defer s.endRun()
	if err := s.acquire(ctx); err != nil {
		return nil, NewDomainError(ErrorCodeStorageMissing, ErrorKeyStorageMissing)
	}
	defer s.release()

	outDir := s.cfg.Backups.Path
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return nil, NewDomainError(ErrorCodeStorageMissing, ErrorKeyStorageMissing)
	}

	run := &BackupRun{Status: StatusQueued, MetaJSON: json.RawMessage("{}")}
	createdRun, err := s.repo.CreateRun(ctx, run)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "idx_backups_runs_single_active") {
			return nil, NewDomainError(ErrorCodeNotReady, ErrorKeyNotReady)
		}
		return nil, err
	}
	createdRun.Status = StatusRunning
	createdRun.MetaJSON = json.RawMessage(`{"event":"backups.create.started"}`)
	_ = s.repo.UpdateRun(ctx, createdRun)

	result, runErr := s.runBackupFlow(ctx, createdRun, outDir, normalizeCreateOptions(opts))
	if runErr != nil {
		return nil, runErr
	}
	return result, nil
}

func (s *Service) beginRun(ctx context.Context) error {
	if s == nil || s.repo == nil {
		return NewDomainError(ErrorCodeInternal, "common.serverError")
	}
	if err := s.beginPipeline("backup"); err != nil {
		return err
	}
	active, err := s.repo.HasRunningBackupRun(ctx)
	if err != nil {
		s.endPipeline("backup")
		return err
	}
	if active {
		s.endPipeline("backup")
		return NewDomainError(ErrorCodeConcurrent, ErrorKeyConcurrent)
	}
	restoreActive, err := s.repo.HasRunningRestoreRun(ctx)
	if err != nil {
		s.endPipeline("backup")
		return err
	}
	if restoreActive || s.IsMaintenanceMode(ctx) {
		s.endPipeline("backup")
		return NewDomainError(ErrorCodeConcurrent, ErrorKeyConcurrent)
	}
	return nil
}

func (s *Service) endRun() {
	s.endPipeline("backup")
}

func (s *Service) DeleteBackup(ctx context.Context, id int64) error {
	if s == nil || s.repo == nil || s.cfg == nil {
		return NewDomainError(ErrorCodeInternal, ErrorKeyInternal)
	}
	if id <= 0 {
		return NewDomainError(ErrorCodeInvalidRequest, ErrorKeyInvalidRequest)
	}
	if err := s.beginPipeline("delete"); err != nil {
		return err
	}
	defer s.endPipeline("delete")

	if s.IsMaintenanceMode(ctx) {
		return NewDomainError(ErrorCodeConcurrent, ErrorKeyConcurrent)
	}
	backupRunning, err := s.repo.HasRunningBackupRun(ctx)
	if err != nil {
		return err
	}
	if backupRunning {
		return NewDomainError(ErrorCodeConcurrent, ErrorKeyConcurrent)
	}
	restoreRunning, err := s.repo.HasRunningRestoreRun(ctx)
	if err != nil {
		return err
	}
	if restoreRunning {
		return NewDomainError(ErrorCodeConcurrent, ErrorKeyConcurrent)
	}
	if s.hasActiveDownload(id) {
		return NewDomainError(ErrorCodeFileBusy, ErrorKeyFileBusy)
	}

	artifact, err := s.repo.GetArtifact(ctx, id)
	if err != nil {
		if err == ErrNotFound {
			return NewDomainError(ErrorCodeNotFound, ErrorKeyNotFound)
		}
		return err
	}

	if artifact.StoragePath != nil && strings.TrimSpace(*artifact.StoragePath) != "" {
		cleanStorage, ok := s.cleanStoragePath(*artifact.StoragePath)
		if !ok {
			return NewDomainError(ErrorCodeInvalidRequest, ErrorKeyInvalidRequest)
		}
		if stat, statErr := os.Stat(cleanStorage); statErr == nil && !stat.IsDir() {
			if removeErr := os.Remove(cleanStorage); removeErr != nil && !os.IsNotExist(removeErr) {
				return NewDomainError(ErrorCodeFileBusy, ErrorKeyFileBusy)
			}
		}
	}
	if err := s.repo.DeleteArtifact(ctx, id); err != nil {
		if err == sql.ErrNoRows {
			return NewDomainError(ErrorCodeNotFound, ErrorKeyNotFound)
		}
		return err
	}
	return nil
}

func (s *Service) GetRestoreRun(ctx context.Context, id int64) (*RestoreRun, error) {
	if s == nil || s.repo == nil {
		return nil, ErrNotFound
	}
	run, err := s.repo.GetRestoreRun(ctx, id)
	if err == nil && run != nil {
		s.cacheRestoreRun(run)
		return run, nil
	}
	if err == ErrNotFound {
		if cached, ok := s.cachedRestoreRun(id); ok {
			return cached, nil
		}
	}
	return nil, err
}

func (s *Service) cacheRestoreRun(run *RestoreRun) {
	if s == nil || run == nil || run.ID <= 0 {
		return
	}
	s.restoreMu.Lock()
	defer s.restoreMu.Unlock()
	cp := *run
	cp.DryRun, cp.Steps = decodeRestoreMeta(cp.MetaJSON, cp.DryRun)
	s.restores[cp.ID] = cp
}

func (s *Service) cachedRestoreRun(id int64) (*RestoreRun, bool) {
	if s == nil || id <= 0 {
		return nil, false
	}
	s.restoreMu.RLock()
	defer s.restoreMu.RUnlock()
	run, ok := s.restores[id]
	if !ok {
		return nil, false
	}
	cp := run
	return &cp, true
}

func decodeRestoreMeta(raw json.RawMessage, currentDryRun bool) (bool, []RestoreStep) {
	if len(raw) == 0 {
		return currentDryRun, nil
	}
	type restoreMeta struct {
		Mode  string        `json:"mode"`
		Steps []RestoreStep `json:"steps"`
	}
	meta := restoreMeta{}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return currentDryRun, nil
	}
	return meta.Mode == "dry_run", meta.Steps
}

func (s *Service) runBackupFlow(ctx context.Context, run *BackupRun, outDir string, opts CreateBackupOptions) (*CreateBackupResult, error) {
	key := s.cfg.Backups.EncryptionKey
	cipher, err := backupcrypto.NewFileCipher(key, 1024*1024)
	if err != nil {
		return nil, s.failRun(ctx, run, NewDomainError(ErrorCodeInvalidEncKey, ErrorKeyInvalidEncKey), ErrorKeyInvalidEncKey)
	}

	tmpDir, err := os.MkdirTemp(outDir, "backup-tmp-*")
	if err != nil {
		return nil, s.failRun(ctx, run, NewDomainError(ErrorCodeStorageMissing, ErrorKeyStorageMissing), ErrorKeyStorageMissing)
	}
	defer os.RemoveAll(tmpDir)

	dumpPath := filepath.Join(tmpDir, "db.dump")
	if err := s.dumpDatabase(ctx, dumpPath); err != nil {
		return nil, s.failRun(ctx, run, NewDomainError(ErrorCodePGDumpFailed, ErrorKeyPGDumpFailed), ErrorKeyPGDumpFailed)
	}

	gooseVersion, err := s.repo.GetGooseVersion(ctx)
	if err != nil {
		gooseVersion = 0
	}
	now := backupNow()
	manifest := format.NewManifest(appmeta.AppVersion, gooseVersion, now.UTC())
	manifest.IncludesFiles = opts.IncludeFiles
	manifest.Notes = buildBackupNotes(opts)
	payloadPath := filepath.Join(tmpDir, "payload.tar")
	payloadInfo, err := format.BuildPayload(dumpPath, payloadPath, manifest)
	if err != nil {
		return nil, s.failRun(ctx, run, NewDomainError(ErrorCodeStorageMissing, ErrorKeyStorageMissing), ErrorKeyStorageMissing)
	}

	filename := buildBackupFilename(opts, now)
	finalPath := filepath.Join(outDir, filename)
	if err := cipher.EncryptFile(payloadPath, finalPath); err != nil {
		return nil, s.failRun(ctx, run, NewDomainError(ErrorCodeEncryptFailed, ErrorKeyEncryptFailed), ErrorKeyEncryptFailed)
	}
	finalChecksum, finalSize, err := fileSHA256(finalPath)
	if err != nil {
		return nil, s.failRun(ctx, run, NewDomainError(ErrorCodeStorageMissing, ErrorKeyStorageMissing), ErrorKeyStorageMissing)
	}

	meta, _ := json.Marshal(map[string]any{
		"format_version":   manifest.FormatVersion,
		"created_at":       manifest.CreatedAt,
		"app_version":      manifest.AppVersion,
		"db_engine":        manifest.DBEngine,
		"goose_db_version": manifest.GooseDBVersion,
		"includes_files":   manifest.IncludesFiles,
		"backup_scope":     normalizedBackupScope(opts.Scope),
		"backup_label":     strings.TrimSpace(opts.Label),
		"entity_counts":    s.snapshotEntityCounts(ctx, opts.Scope),
		"checksums":        payloadInfo.Checksums,
	})
	artifact := &BackupArtifact{
		Source:      ArtifactSourceLocal,
		Status:      StatusSuccess,
		SizeBytes:   &finalSize,
		Checksum:    &finalChecksum,
		Filename:    &filename,
		StoragePath: &finalPath,
		MetaJSON:    meta,
	}
	createdArtifact, err := s.repo.CreateArtifact(ctx, artifact)
	if err != nil {
		return nil, s.failRun(ctx, run, err, ErrorKeyStorageMissing)
	}
	if err := s.repo.AttachArtifactToRun(ctx, run.ID, createdArtifact.ID); err != nil {
		return nil, s.failRun(ctx, run, err, ErrorKeyStorageMissing)
	}
	run.ArtifactID = &createdArtifact.ID
	run.Status = StatusSuccess
	run.SizeBytes = artifact.SizeBytes
	run.Checksum = artifact.Checksum
	run.Filename = artifact.Filename
	run.StoragePath = artifact.StoragePath
	run.MetaJSON = meta
	if err := s.repo.UpdateRun(ctx, run); err != nil {
		return nil, err
	}
	return &CreateBackupResult{
		Run:      *run,
		Artifact: *createdArtifact,
	}, nil
}

func (s *Service) dumpDatabase(ctx context.Context, dumpPath string) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	err := s.dumper.Dump(timeoutCtx, pgdump.DumpOptions{
		BinaryPath: s.cfg.Backups.PGDumpBin,
		DBURL:      s.cfg.DBURL,
		OutputPath: dumpPath,
	})
	if err != nil && s.logger != nil {
		s.logger.Errorf("backup pg_dump failed: %v", err)
	}
	return err
}

func (s *Service) failRun(ctx context.Context, run *BackupRun, retErr error, i18nKey string) error {
	if run != nil {
		run.Status = StatusFailed
		code := i18nKey
		run.ErrorCode = &code
		message := "backup failed"
		run.ErrorMessage = &message
		run.MetaJSON = json.RawMessage(`{"event":"backups.create.failed"}`)
		_ = s.repo.UpdateRun(ctx, run)
	}
	return retErr
}

func (s *Service) acquire(ctx context.Context) error {
	select {
	case s.limiter <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) release() {
	select {
	case <-s.limiter:
	default:
	}
}

func (s *Service) UploadMaxBytes() int64 {
	if s == nil || s.cfg == nil || s.cfg.Backups.UploadMaxBytes <= 0 {
		return 512 * 1024 * 1024
	}
	return s.cfg.Backups.UploadMaxBytes
}

func (s *Service) cleanStoragePath(raw string) (string, bool) {
	basePath := ""
	if s != nil && s.cfg != nil {
		basePath = strings.TrimSpace(s.cfg.Backups.Path)
	}
	cleanStorage := filepath.Clean(strings.TrimSpace(raw))
	if cleanStorage == "." || cleanStorage == "" {
		return "", false
	}
	if basePath == "" {
		return cleanStorage, true
	}
	cleanBase := filepath.Clean(basePath)
	if !strings.HasPrefix(cleanStorage+string(os.PathSeparator), cleanBase+string(os.PathSeparator)) && cleanStorage != cleanBase {
		return "", false
	}
	return cleanStorage, true
}

func (s *Service) beginPipeline(name string) error {
	if s == nil {
		return NewDomainError(ErrorCodeInternal, ErrorKeyInternal)
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if s.opName != "" {
		return NewDomainError(ErrorCodeConcurrent, ErrorKeyConcurrent)
	}
	s.opName = strings.TrimSpace(name)
	return nil
}

func (s *Service) endPipeline(name string) {
	if s == nil {
		return
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if s.opName == strings.TrimSpace(name) {
		s.opName = ""
	}
}

func (s *Service) beginDownload(id int64) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if s.opName == "delete" || s.opName == "restore" {
		return NewDomainError(ErrorCodeConcurrent, ErrorKeyConcurrent)
	}
	s.downloads[id]++
	return nil
}

func (s *Service) endDownload(id int64) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	current := s.downloads[id]
	if current <= 1 {
		delete(s.downloads, id)
		return
	}
	s.downloads[id] = current - 1
}

func (s *Service) hasActiveDownload(id int64) bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.downloads[id] > 0
}

func (s *Service) activeOperation() string {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.opName
}

func (s *Service) activeDownloads() []int64 {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	out := make([]int64, 0, len(s.downloads))
	for k, v := range s.downloads {
		if v > 0 {
			out = append(out, k)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func fileSHA256(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	hasher := sha256.New()
	size, err := io.Copy(hasher, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hasher.Sum(nil)), size, nil
}

func normalizeCreateOptions(in CreateBackupOptions) CreateBackupOptions {
	out := in
	out.Label = strings.TrimSpace(out.Label)
	out.Scope = normalizedBackupScope(out.Scope)
	return out
}

func normalizedBackupScope(in []string) []string {
	allowed := map[string]struct{}{
		"docs":       {},
		"tasks":      {},
		"incidents":  {},
		"reports":    {},
		"monitoring": {},
		"controls":   {},
		"accounts":   {},
		"approvals":  {},
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		v := strings.ToLower(strings.TrimSpace(raw))
		if v == "" {
			continue
		}
		if v == "all" {
			return []string{"ALL"}
		}
		if _, ok := allowed[v]; !ok {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	if len(out) == 0 {
		return []string{"ALL"}
	}
	sort.Strings(out)
	return out
}

func buildBackupNotes(opts CreateBackupOptions) string {
	scope := normalizedBackupScope(opts.Scope)
	label := strings.TrimSpace(opts.Label)
	return "scope=" + strings.Join(scope, ",") + ";label=" + label
}

func buildBackupFilename(opts CreateBackupOptions, now time.Time) string {
	scope := strings.Join(normalizedBackupScope(opts.Scope), "-")
	scope = sanitizeFilenameToken(scope)
	if scope == "" {
		scope = "ALL"
	}
	label := sanitizeFilenameToken(strings.TrimSpace(opts.Label))
	ts := now.UTC().Format("2006-01-02_15-04-05")
	if label == "" {
		return "backup_" + scope + "_" + ts + ".bscc"
	}
	return "backup_" + scope + "_" + label + "_" + ts + ".bscc"
}

func sanitizeFilenameToken(in string) string {
	v := strings.TrimSpace(in)
	if v == "" {
		return ""
	}
	v = strings.ToUpper(v)
	replacer := strings.NewReplacer(
		" ", "_",
		"/", "_",
		"\\", "_",
		":", "_",
		";", "_",
		",", "_",
		"\"", "",
		"'", "",
		".", "_",
	)
	v = replacer.Replace(v)
	for strings.Contains(v, "__") {
		v = strings.ReplaceAll(v, "__", "_")
	}
	return strings.Trim(v, "_")
}
