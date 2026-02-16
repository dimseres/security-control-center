package tests

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"berkut-scc/config"
	"berkut-scc/core/docs"
	"berkut-scc/core/store"
	"berkut-scc/core/utils"
)

func setupDocs(t *testing.T) (context.Context, *config.AppConfig, *store.User, store.DocsStore, store.UsersStore, *docs.Service, func()) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.AppConfig{
		DBPath: filepath.Join(dir, "test.db"),
		Docs: config.DocsConfig{
			StoragePath:       filepath.Join(dir, "docs"),
			EncryptionKey:     "12345678901234567890123456789012",
			RegTemplate:       "{level}.{year}.{seq}",
			VersionLimit:      2,
			PerFolderSequence: false,
			Watermark: config.WatermarkConfig{
				Enabled:  true,
				MinLevel: "CONFIDENTIAL",
			},
			Converters: config.ConvertersConfig{Enabled: false},
		},
	}
	logger := utils.NewLogger()
	db, err := store.NewDB(cfg, logger)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.ApplyMigrations(context.Background(), db, logger); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ds := store.NewDocsStore(db)
	us := store.NewUsersStore(db)
	audits := store.NewAuditStore(db)
	svc, err := docs.NewService(cfg, ds, us, audits, logger)
	if err != nil {
		t.Fatalf("svc: %v", err)
	}
	u := &store.User{
		Username:       "user1",
		FullName:       "User One",
		Department:     "Sec",
		Position:       "Analyst",
		ClearanceLevel: int(docs.ClassificationTopSecret),
		ClearanceTags:  docs.TagList,
		PasswordHash:   "hash",
		Salt:           "salt",
		PasswordSet:    true,
		Active:         true,
	}
	uid, err := us.Create(context.Background(), u, []string{"doc_admin"})
	if err != nil {
		t.Fatalf("user create: %v", err)
	}
	u.ID = uid
	cleanup := func() {
		db.Close()
	}
	return context.Background(), cfg, u, ds, us, svc, cleanup
}

func createDoc(t *testing.T, ctx context.Context, ds store.DocsStore, u *store.User, cfg *config.AppConfig) *store.Document {
	doc := &store.Document{
		Title:               "Test",
		Status:              docs.StatusDraft,
		ClassificationLevel: int(docs.ClassificationInternal),
		ClassificationTags:  []string{},
		InheritACL:          true,
		CreatedBy:           u.ID,
	}
	acl := []store.ACLRule{{SubjectType: "user", SubjectID: u.Username, Permission: "view"}, {SubjectType: "user", SubjectID: u.Username, Permission: "edit"}, {SubjectType: "user", SubjectID: u.Username, Permission: "export"}, {SubjectType: "user", SubjectID: u.Username, Permission: "approve"}}
	if _, err := ds.CreateDocument(ctx, doc, acl, cfg.Docs.RegTemplate, cfg.Docs.PerFolderSequence); err != nil {
		t.Fatalf("create doc: %v", err)
	}
	return doc
}

func TestEncryptionRoundtrip(t *testing.T) {
	ctx, cfg, user, ds, _, svc, cleanup := setupDocs(t)
	defer cleanup()
	doc := createDoc(t, ctx, ds, user, cfg)
	content := []byte("# hello")
	ver, err := svc.SaveVersion(ctx, docs.SaveRequest{Doc: doc, Author: user, Format: docs.FormatMarkdown, Content: content, Reason: "test", IndexFTS: true})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := svc.LoadContent(ctx, ver)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("mismatch content")
	}
	// wrong key should fail
	badCfg := *cfg
	badCfg.Docs.EncryptionKey = "00000000000000000000000000000000"
	badSvc, _ := docs.NewService(&badCfg, ds, nil, nil, utils.NewLogger())
	if _, err := badSvc.LoadContent(ctx, ver); err == nil {
		t.Fatalf("expected decrypt failure with wrong key")
	}
}

func TestClearanceAndACL(t *testing.T) {
	ctx, cfg, user, ds, _, svc, cleanup := setupDocs(t)
	defer cleanup()
	doc := createDoc(t, ctx, ds, user, cfg)
	doc.ClassificationLevel = int(docs.ClassificationConfidential)
	doc.ClassificationTags = []string{"PERSONAL_DATA"}
	ds.UpdateDocument(ctx, doc)
	low := &store.User{ID: 99, Username: "low", ClearanceLevel: int(docs.ClassificationInternal), ClearanceTags: []string{}}
	docACL, _ := ds.GetDocACL(ctx, doc.ID)
	if svc.CheckACL(low, []string{}, doc, docACL, nil, "view") {
		t.Fatalf("expected deny for low clearance")
	}
	if !svc.CheckACL(user, []string{"doc_admin"}, doc, docACL, nil, "view") {
		t.Fatalf("expected allow for creator")
	}
}

func TestVersionLimit(t *testing.T) {
	ctx, cfg, user, ds, _, svc, cleanup := setupDocs(t)
	defer cleanup()
	cfg.Docs.VersionLimit = 2
	doc := createDoc(t, ctx, ds, user, cfg)
	for i := 0; i < 3; i++ {
		if _, err := svc.SaveVersion(ctx, docs.SaveRequest{Doc: doc, Author: user, Format: docs.FormatMarkdown, Content: []byte("v" + string(rune('A'+i))), Reason: "r", IndexFTS: true}); err != nil {
			t.Fatalf("save v%d: %v", i, err)
		}
	}
	vers, err := ds.ListVersions(ctx, doc.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(vers) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(vers))
	}
}

func TestRegNumberUniqueness(t *testing.T) {
	ctx, cfg, user, ds, _, _, cleanup := setupDocs(t)
	defer cleanup()
	doc1 := createDoc(t, ctx, ds, user, cfg)
	doc2 := createDoc(t, ctx, ds, user, cfg)
	if doc1.RegNumber == doc2.RegNumber {
		t.Fatalf("reg numbers must differ")
	}
}

func TestApprovalFlow(t *testing.T) {
	ctx, cfg, user, ds, us, svc, cleanup := setupDocs(t)
	defer cleanup()
	doc := createDoc(t, ctx, ds, user, cfg)
	ap := &store.Approval{
		DocID:        doc.ID,
		Status:       docs.StatusReview,
		CurrentStage: 1,
		Message:      "check",
		CreatedBy:    user.ID,
		CreatedAt:    utils.NowUTC(),
		UpdatedAt:    utils.NowUTC(),
	}
	approver := &store.User{Username: "approver", ClearanceLevel: int(docs.ClassificationTopSecret), ClearanceTags: docs.TagList, PasswordHash: "h", Salt: "s", PasswordSet: true, Active: true}
	aid, _ := us.Create(ctx, approver, []string{"doc_reviewer"})
	approver.ID = aid
	approvalID, err := ds.CreateApproval(ctx, ap, []store.ApprovalParticipant{{UserID: user.ID, Role: "initiator", Stage: 1}, {UserID: approver.ID, Role: "approver", Stage: 1}})
	if err != nil {
		t.Fatalf("create approval: %v", err)
	}
	if err := ds.SaveApprovalDecision(ctx, approvalID, approver.ID, "approve", "ok", 1); err != nil {
		t.Fatalf("decision: %v", err)
	}
	if err := ds.UpdateApprovalStatus(ctx, approvalID, docs.StatusApproved, 1); err != nil {
		t.Fatalf("status: %v", err)
	}
	apLoaded, _, err := ds.GetApproval(ctx, approvalID)
	if err != nil || apLoaded == nil {
		t.Fatalf("get approval: %v", err)
	}
	if apLoaded.Status != docs.StatusApproved {
		t.Fatalf("expected approved, got %s", apLoaded.Status)
	}
	_ = svc
}

func TestFTSSearch(t *testing.T) {
	ctx, cfg, user, ds, _, svc, cleanup := setupDocs(t)
	defer cleanup()
	doc := createDoc(t, ctx, ds, user, cfg)
	content := []byte("sensitive needle content")
	if _, err := svc.SaveVersion(ctx, docs.SaveRequest{Doc: doc, Author: user, Format: docs.FormatMarkdown, Content: content, Reason: "fts", IndexFTS: true}); err != nil {
		t.Fatalf("save: %v", err)
	}
	hits, err := ds.SearchFTS(ctx, "needle", nil)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	found := false
	for _, h := range hits {
		if h.DocID == doc.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("fts did not return doc")
	}
}

func TestWatermarkAppliedOnExport(t *testing.T) {
	ctx, cfg, user, ds, _, svc, cleanup := setupDocs(t)
	defer cleanup()
	doc := createDoc(t, ctx, ds, user, cfg)
	doc.ClassificationLevel = int(docs.ClassificationConfidential)
	content := []byte("# sample")
	ver, err := svc.SaveVersion(ctx, docs.SaveRequest{Doc: doc, Author: user, Format: docs.FormatMarkdown, Content: content, Reason: "wm", IndexFTS: true})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	res, err := svc.Export(ctx, docs.ExportRequest{Doc: doc, Version: ver, Format: docs.FormatTXT, Username: user.Username})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !strings.Contains(string(res.Data), "Berkut SCC") {
		t.Fatalf("expected watermark text in export")
	}
}

func TestConvertersHealthcheck(t *testing.T) {
	_, cfg, _, ds, us, _, cleanup := setupDocs(t)
	defer cleanup()
	cfg.Docs.Converters.Enabled = true
	cfg.Docs.Converters.PandocPath = "missing-pandoc-binary"
	cfg.Docs.Converters.SofficePath = "missing-soffice-binary"
	svc, err := docs.NewService(cfg, ds, us, nil, utils.NewLogger())
	if err != nil {
		t.Fatalf("svc init: %v", err)
	}
	status := svc.ConvertersStatus()
	if status.Enabled {
		t.Fatalf("converters should be disabled when binaries missing")
	}
	if status.PandocAvailable || status.SofficeAvailable {
		t.Fatalf("unexpected converter availability flags")
	}
}

func TestListDocumentFilters(t *testing.T) {
	ctx, cfg, user, ds, _, svc, cleanup := setupDocs(t)
	defer cleanup()
	docLow := createDoc(t, ctx, ds, user, cfg)
	docHigh := createDoc(t, ctx, ds, user, cfg)
	docHigh.ClassificationLevel = int(docs.ClassificationSecret)
	if err := ds.UpdateDocument(ctx, docHigh); err != nil {
		t.Fatalf("update doc: %v", err)
	}
	if _, err := svc.SaveVersion(ctx, docs.SaveRequest{Doc: docLow, Author: user, Format: docs.FormatMarkdown, Content: []byte("low"), Reason: "r", IndexFTS: true}); err != nil {
		t.Fatalf("save low: %v", err)
	}
	if _, err := svc.SaveVersion(ctx, docs.SaveRequest{Doc: docHigh, Author: user, Format: docs.FormatMarkdown, Content: []byte("high secret"), Reason: "r", IndexFTS: true}); err != nil {
		t.Fatalf("save high: %v", err)
	}
	filtered, err := ds.ListDocuments(ctx, store.DocumentFilter{MinLevel: int(docs.ClassificationSecret)})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != docHigh.ID {
		t.Fatalf("expected only high classification doc, got %+v", filtered)
	}
}
