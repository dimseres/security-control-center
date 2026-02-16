package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"berkut-scc/config"
	"berkut-scc/core/auth"
	"berkut-scc/core/docs"
	"berkut-scc/core/rbac"
	"berkut-scc/core/store"
	"berkut-scc/core/utils"
)

type DocsHandler struct {
	cfg         *config.AppConfig
	store       store.DocsStore
	links       store.EntityLinksStore
	controls    store.ControlsStore
	users       store.UsersStore
	policy      *rbac.Policy
	svc         *docs.Service
	audits      store.AuditStore
	logger      *utils.Logger
	uploads     map[string]uploadItem
	officeSaves map[string]onlyOfficePendingSave
	mu          sync.Mutex
}

type uploadItem struct {
	ID         string
	Path       string
	Name       string
	Size       int64
	Format     string
	Uploader   string
	UploadedAt time.Time
}

func NewDocsHandler(cfg *config.AppConfig, ds store.DocsStore, links store.EntityLinksStore, controls store.ControlsStore, us store.UsersStore, policy *rbac.Policy, svc *docs.Service, audits store.AuditStore, logger *utils.Logger) *DocsHandler {
	return &DocsHandler{
		cfg:         cfg,
		store:       ds,
		links:       links,
		controls:    controls,
		users:       us,
		policy:      policy,
		svc:         svc,
		audits:      audits,
		logger:      logger,
		uploads:     map[string]uploadItem{},
		officeSaves: map[string]onlyOfficePendingSave{},
	}
}

func (h *DocsHandler) List(w http.ResponseWriter, r *http.Request) {
	user, roles, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var folderID *int64
	if v := r.URL.Query().Get("folder_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			folderID = &id
		}
	}
	limit := parseIntDefault(r.URL.Query().Get("limit"), 0)
	offset := parseIntDefault(r.URL.Query().Get("offset"), 0)
	minLevel := parseIntDefault(r.URL.Query().Get("min_level"), -1)
	var tags []string
	if rawTags := r.URL.Query().Get("tags"); rawTags != "" {
		for _, t := range strings.Split(rawTags, ",") {
			if strings.TrimSpace(t) != "" {
				tags = append(tags, strings.TrimSpace(t))
			}
		}
	}
	filter := store.DocumentFilter{
		FolderID: folderID,
		Status:   r.URL.Query().Get("status"),
		Search:   r.URL.Query().Get("q"),
		Limit:    limit,
		Offset:   offset,
		Sort:     r.URL.Query().Get("sort"),
		Tags:     tags,
	}
	filter.DocType = "document"
	if minLevel >= 0 {
		filter.MinLevel = minLevel
	}
	if r.URL.Query().Get("mine") == "1" {
		filter.MineUserID = user.ID
	}
	if q := r.URL.Query().Get("status_in"); q != "" {
		filter.StatusIn = strings.Split(q, ",")
	}
	docsList, err := h.store.ListDocuments(r.Context(), filter)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	type docListItem struct {
		store.Document
		Format string `json:"format"`
	}
	var result []docListItem
	for _, d := range docsList {
		docACL, _ := h.store.GetDocACL(r.Context(), d.ID)
		var folderACL []store.ACLRule
		if d.FolderID != nil {
			folderACL, _ = h.store.GetFolderACL(r.Context(), *d.FolderID)
		}
		if h.svc.CheckACL(user, roles, &d, docACL, folderACL, "view") {
			if d.Status == docs.StatusReview {
				ap, parts, _ := h.store.GetActiveApproval(r.Context(), d.ID)
				if ap != nil && !isApprovalParticipant(parts, user.ID) && !hasRole(roles, "doc_admin") && !hasRole(roles, "admin") {
					continue
				}
			}
			format := docs.FormatMarkdown
			if d.CurrentVersion > 0 {
				if ver, err := h.store.GetVersion(r.Context(), d.ID, d.CurrentVersion); err == nil && ver != nil && ver.Format != "" {
					format = ver.Format
				}
			}
			result = append(result, docListItem{Document: d, Format: format})
		}
	}
	h.svc.Log(r.Context(), user.Username, "doc.list", "")
	writeJSON(w, http.StatusOK, map[string]any{
		"items":      result,
		"converters": h.svc.ConvertersStatus(),
	})
}

func (h *DocsHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, roles, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var payload struct {
		Title               string   `json:"title"`
		FolderID            *int64   `json:"folder_id"`
		ClassificationLevel string   `json:"classification_level"`
		ClassificationTags  []string `json:"classification_tags"`
		InheritACL          bool     `json:"inherit_acl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	level, err := docs.ParseLevel(payload.ClassificationLevel)
	if err != nil {
		http.Error(w, "invalid level", http.StatusBadRequest)
		return
	}
	if !hasRole(roles, "superadmin") && !hasRole(roles, "admin") && !docs.HasClearance(docs.ClassificationLevel(user.ClearanceLevel), user.ClearanceTags, level, payload.ClassificationTags) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	doc := &store.Document{
		FolderID:              payload.FolderID,
		Title:                 payload.Title,
		Status:                docs.StatusDraft,
		ClassificationLevel:   int(level),
		ClassificationTags:    docs.NormalizeTags(payload.ClassificationTags),
		DocType:               "document",
		InheritACL:            payload.InheritACL,
		InheritClassification: true,
		CreatedBy:             user.ID,
		CurrentVersion:        0,
	}
	acl := []store.ACLRule{
		{SubjectType: "user", SubjectID: user.Username, Permission: "view"},
		{SubjectType: "user", SubjectID: user.Username, Permission: "edit"},
		{SubjectType: "user", SubjectID: user.Username, Permission: "manage"},
		{SubjectType: "user", SubjectID: user.Username, Permission: "export"},
	}
	docID, err := h.store.CreateDocument(r.Context(), doc, acl, h.cfg.Docs.RegTemplate, h.cfg.Docs.PerFolderSequence)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	doc.ID = docID
	// initial empty version to anchor encryption chain
	_, _ = h.svc.SaveVersion(r.Context(), docs.SaveRequest{
		Doc:      doc,
		Author:   user,
		Format:   docs.FormatMarkdown,
		Content:  []byte{},
		Reason:   "initial",
		IndexFTS: true,
	})
	h.svc.Log(r.Context(), user.Username, "doc.create", doc.RegNumber)
	writeJSON(w, http.StatusOK, doc)
	_ = roles
}

func (h *DocsHandler) Upload(w http.ResponseWriter, r *http.Request) {
	user, _, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err := parseMultipartFormLimited(w, r, 25<<20); err != nil {
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	defer file.Close()
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(header.Filename)), ".")
	if format == "" {
		format = docs.FormatMarkdown
	}
	if !isAllowedUpload(format) {
		http.Error(w, "unsupported format", http.StatusBadRequest)
		return
	}
	uploadID, _ := utils.RandString(12)
	tmpDir := h.cfg.Docs.Converters.TempDir
	if tmpDir == "" {
		tmpDir = os.TempDir()
	}
	if err := os.MkdirAll(tmpDir, 0o700); err != nil {
		if h.logger != nil {
			h.logger.Errorf("docs upload mkdir temp failed (%s): %v", tmpDir, err)
		}
		tmpDir = os.TempDir()
		if err := os.MkdirAll(tmpDir, 0o700); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
	}
	tmpFile, err := os.CreateTemp(tmpDir, fmt.Sprintf("upload-*.%s", format))
	if err != nil {
		if h.logger != nil {
			h.logger.Errorf("docs upload create temp failed (%s): %v", tmpDir, err)
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	tmpPath := tmpFile.Name()
	size, copyErr := io.Copy(tmpFile, file)
	closeErr := tmpFile.Close()
	if copyErr != nil || closeErr != nil {
		if h.logger != nil {
			h.logger.Errorf("docs upload write failed: copy=%v close=%v", copyErr, closeErr)
		}
		_ = os.Remove(tmpPath)
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	item := uploadItem{
		ID:         uploadID,
		Path:       tmpPath,
		Name:       header.Filename,
		Size:       size,
		Format:     format,
		Uploader:   user.Username,
		UploadedAt: time.Now().UTC(),
	}
	h.storeUpload(item)
	writeJSON(w, http.StatusOK, map[string]any{
		"upload_id": uploadID,
		"name":      header.Filename,
		"format":    format,
		"size":      size,
	})
}

func (h *DocsHandler) ImportCommit(w http.ResponseWriter, r *http.Request) {
	user, roles, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var payload struct {
		UploadID            string   `json:"upload_id"`
		Title               string   `json:"title"`
		RegNumber           string   `json:"reg_number"`
		FolderID            *int64   `json:"folder_id"`
		ClassificationLevel string   `json:"classification_level"`
		ClassificationTags  []string `json:"classification_tags"`
		ACLRoles            []string `json:"acl_roles"`
		ACLUsers            []int64  `json:"acl_users"`
		InheritACL          bool     `json:"inherit_acl"`
		Owner               *int64   `json:"owner"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.UploadID == "" || payload.Title == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	payload.Title = strings.TrimSpace(payload.Title)
	if payload.Title == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	item, ok := h.takeUpload(payload.UploadID)
	if !ok {
		http.Error(w, "upload not found or expired", http.StatusBadRequest)
		return
	}
	defer os.Remove(item.Path)
	data, err := os.ReadFile(item.Path)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	level, err := docs.ParseLevel(payload.ClassificationLevel)
	if err != nil {
		http.Error(w, "invalid level", http.StatusBadRequest)
		return
	}
	if !hasRole(roles, "superadmin") && !hasRole(roles, "admin") && !docs.HasClearance(docs.ClassificationLevel(user.ClearanceLevel), user.ClearanceTags, level, payload.ClassificationTags) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	existingDoc, err := h.store.FindDocumentByTitle(r.Context(), payload.Title, payload.FolderID, "document")
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if existingDoc != nil {
		docACL, _ := h.store.GetDocACL(r.Context(), existingDoc.ID)
		var folderACL []store.ACLRule
		if existingDoc.FolderID != nil {
			folderACL, _ = h.store.GetFolderACL(r.Context(), *existingDoc.FolderID)
		}
		if !h.svc.CheckACL(user, roles, existingDoc, docACL, folderACL, "edit") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		existingDoc.Title = payload.Title
		existingDoc.FolderID = payload.FolderID
		existingDoc.ClassificationLevel = int(level)
		existingDoc.ClassificationTags = docs.NormalizeTags(payload.ClassificationTags)
		existingDoc.InheritACL = payload.InheritACL
		existingDoc.InheritClassification = true
		existingDoc.Status = docs.StatusDraft
		if err := h.store.UpdateDocument(r.Context(), existingDoc); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		v, err := h.svc.SaveVersion(r.Context(), docs.SaveRequest{
			Doc:      existingDoc,
			Author:   user,
			Format:   item.Format,
			Content:  data,
			Reason:   "import",
			IndexFTS: true,
		})
		if err != nil {
			http.Error(w, "import failed", http.StatusInternalServerError)
			return
		}
		existingDoc.CurrentVersion = v.Version
		h.svc.Log(r.Context(), user.Username, "doc.import", existingDoc.RegNumber)
		writeJSON(w, http.StatusOK, map[string]any{
			"id":           existingDoc.ID,
			"reg_number":   existingDoc.RegNumber,
			"version":      v.Version,
			"format":       item.Format,
			"can_convert":  h.svc.ConvertersStatus().Enabled,
			"class_level":  existingDoc.ClassificationLevel,
			"class_tags":   existingDoc.ClassificationTags,
			"storage_path": "",
		})
		return
	}
	createdBy := user.ID
	if payload.Owner != nil && *payload.Owner > 0 {
		createdBy = *payload.Owner
	}
	doc := &store.Document{
		FolderID:              payload.FolderID,
		Title:                 payload.Title,
		Status:                docs.StatusDraft,
		ClassificationLevel:   int(level),
		ClassificationTags:    docs.NormalizeTags(payload.ClassificationTags),
		DocType:               "document",
		InheritACL:            payload.InheritACL,
		InheritClassification: true,
		CreatedBy:             createdBy,
		CurrentVersion:        0,
		RegNumber:             payload.RegNumber,
	}
	acl := buildBaseACL(user)
	if len(payload.ACLRoles) > 0 || len(payload.ACLUsers) > 0 {
		acl = append(acl, buildACL(payload.ACLRoles, payload.ACLUsers)...)
	}
	docID, err := h.store.CreateDocument(r.Context(), doc, acl, h.cfg.Docs.RegTemplate, h.cfg.Docs.PerFolderSequence)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	doc.ID = docID
	v, err := h.svc.SaveVersion(r.Context(), docs.SaveRequest{
		Doc:      doc,
		Author:   user,
		Format:   item.Format,
		Content:  data,
		Reason:   "import",
		IndexFTS: true,
	})
	if err != nil {
		http.Error(w, "import failed", http.StatusInternalServerError)
		return
	}
	doc.CurrentVersion = v.Version
	h.svc.Log(r.Context(), user.Username, "doc.import", doc.RegNumber)
	writeJSON(w, http.StatusOK, map[string]any{
		"id":           doc.ID,
		"reg_number":   doc.RegNumber,
		"version":      v.Version,
		"format":       item.Format,
		"can_convert":  h.svc.ConvertersStatus().Enabled,
		"class_level":  doc.ClassificationLevel,
		"class_tags":   doc.ClassificationTags,
		"storage_path": "",
	})
}
func (h *DocsHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, roles, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, _ := strconv.ParseInt(pathParams(r)["id"], 10, 64)
	doc, err := h.store.GetDocument(r.Context(), id)
	if err != nil || doc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !h.isDocument(doc) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	docACL, _ := h.store.GetDocACL(r.Context(), doc.ID)
	var folderACL []store.ACLRule
	if doc.FolderID != nil {
		folderACL, _ = h.store.GetFolderACL(r.Context(), *doc.FolderID)
	}
	if !h.svc.CheckACL(user, roles, doc, docACL, folderACL, "view") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if doc.Status == docs.StatusReview {
		ap, parts, _ := h.store.GetActiveApproval(r.Context(), doc.ID)
		if ap != nil && !isApprovalParticipant(parts, user.ID) && !hasRole(roles, "doc_admin") && !hasRole(roles, "admin") && !hasRole(roles, "security_officer") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
	}
	h.svc.Log(r.Context(), user.Username, "doc.view", doc.RegNumber)
	writeJSON(w, http.StatusOK, doc)
}

func (h *DocsHandler) GetContent(w http.ResponseWriter, r *http.Request) {
	user, roles, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, _ := strconv.ParseInt(pathParams(r)["id"], 10, 64)
	doc, err := h.store.GetDocument(r.Context(), id)
	if err != nil || doc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !h.isDocument(doc) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	docACL, _ := h.store.GetDocACL(r.Context(), doc.ID)
	var folderACL []store.ACLRule
	if doc.FolderID != nil {
		folderACL, _ = h.store.GetFolderACL(r.Context(), *doc.FolderID)
	}
	if !h.svc.CheckACL(user, roles, doc, docACL, folderACL, "view") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if doc.Status == docs.StatusReview {
		ap, parts, _ := h.store.GetActiveApproval(r.Context(), doc.ID)
		if ap != nil && !isApprovalParticipant(parts, user.ID) && !hasRole(roles, "doc_admin") && !hasRole(roles, "admin") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
	}
	ver, err := h.store.GetVersion(r.Context(), doc.ID, doc.CurrentVersion)
	if err != nil || ver == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	content, err := h.svc.LoadContent(r.Context(), ver)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if strings.TrimSpace(r.URL.Query().Get("audit")) != "0" {
		h.svc.Log(r.Context(), user.Username, "doc.view", doc.RegNumber)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"format":  ver.Format,
		"version": ver.Version,
		"content": string(content),
	})
}

func (h *DocsHandler) UpdateContent(w http.ResponseWriter, r *http.Request) {
	user, roles, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, _ := strconv.ParseInt(pathParams(r)["id"], 10, 64)
	doc, err := h.store.GetDocument(r.Context(), id)
	if err != nil || doc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !h.isDocument(doc) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	docACL, _ := h.store.GetDocACL(r.Context(), doc.ID)
	var folderACL []store.ACLRule
	if doc.FolderID != nil {
		folderACL, _ = h.store.GetFolderACL(r.Context(), *doc.FolderID)
	}
	if !h.svc.CheckACL(user, roles, doc, docACL, folderACL, "edit") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var payload struct {
		Content string `json:"content"`
		Format  string `json:"format"`
		Reason  string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Reason == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if payload.Format == "" {
		payload.Format = docs.FormatMarkdown
	}
	v, err := h.svc.SaveVersion(r.Context(), docs.SaveRequest{
		Doc:      doc,
		Author:   user,
		Format:   payload.Format,
		Content:  []byte(payload.Content),
		Reason:   payload.Reason,
		IndexFTS: true,
	})
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	doc.CurrentVersion = v.Version
	doc.Status = docs.StatusDraft
	_ = h.store.UpdateDocument(r.Context(), doc)
	h.svc.Log(r.Context(), user.Username, "doc.edit", doc.RegNumber)
	writeJSON(w, http.StatusOK, map[string]any{"version": v.Version})
}

func (h *DocsHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	user, roles, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, _ := strconv.ParseInt(pathParams(r)["id"], 10, 64)
	doc, err := h.store.GetDocument(r.Context(), id)
	if err != nil || doc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !h.isDocument(doc) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	docACL, _ := h.store.GetDocACL(r.Context(), doc.ID)
	var folderACL []store.ACLRule
	if doc.FolderID != nil {
		folderACL, _ = h.store.GetFolderACL(r.Context(), *doc.FolderID)
	}
	if !h.svc.CheckACL(user, roles, doc, docACL, folderACL, "view") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	vers, err := h.store.ListVersions(r.Context(), doc.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "doc.versions.view", doc.RegNumber)
	writeJSON(w, http.StatusOK, map[string]any{"versions": vers})
}

func (h *DocsHandler) GetVersionContent(w http.ResponseWriter, r *http.Request) {
	user, roles, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	params := pathParams(r)
	id, _ := strconv.ParseInt(params["id"], 10, 64)
	verNum, _ := strconv.Atoi(params["ver"])
	doc, err := h.store.GetDocument(r.Context(), id)
	if err != nil || doc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !h.isDocument(doc) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	docACL, _ := h.store.GetDocACL(r.Context(), doc.ID)
	var folderACL []store.ACLRule
	if doc.FolderID != nil {
		folderACL, _ = h.store.GetFolderACL(r.Context(), *doc.FolderID)
	}
	if !h.svc.CheckACL(user, roles, doc, docACL, folderACL, "view") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if doc.Status == docs.StatusReview {
		ap, parts, _ := h.store.GetActiveApproval(r.Context(), doc.ID)
		if ap != nil && !isApprovalParticipant(parts, user.ID) && !hasRole(roles, "doc_admin") && !hasRole(roles, "admin") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
	}
	ver, err := h.store.GetVersion(r.Context(), doc.ID, verNum)
	if err != nil || ver == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	content, err := h.svc.LoadContent(r.Context(), ver)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": ver.Version, "format": ver.Format, "content": string(content)})
}

func (h *DocsHandler) RestoreVersion(w http.ResponseWriter, r *http.Request) {
	user, roles, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	params := pathParams(r)
	id, _ := strconv.ParseInt(params["id"], 10, 64)
	verNum, _ := strconv.Atoi(params["ver"])
	doc, err := h.store.GetDocument(r.Context(), id)
	if err != nil || doc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !h.isDocument(doc) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	docACL, _ := h.store.GetDocACL(r.Context(), doc.ID)
	var folderACL []store.ACLRule
	if doc.FolderID != nil {
		folderACL, _ = h.store.GetFolderACL(r.Context(), *doc.FolderID)
	}
	if !h.svc.CheckACL(user, roles, doc, docACL, folderACL, "edit") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if doc.Status == docs.StatusReview {
		ap, parts, _ := h.store.GetActiveApproval(r.Context(), doc.ID)
		if ap != nil && !isApprovalParticipant(parts, user.ID) && !hasRole(roles, "doc_admin") && !hasRole(roles, "admin") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
	}
	ver, err := h.store.GetVersion(r.Context(), doc.ID, verNum)
	if err != nil || ver == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	content, err := h.svc.LoadContent(r.Context(), ver)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	newVer, err := h.svc.SaveVersion(r.Context(), docs.SaveRequest{
		Doc:      doc,
		Author:   user,
		Format:   ver.Format,
		Content:  content,
		Reason:   "restore",
		IndexFTS: true,
	})
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	doc.CurrentVersion = newVer.Version
	_ = h.store.UpdateDocument(r.Context(), doc)
	h.svc.Log(r.Context(), user.Username, "doc.restore", doc.RegNumber)
	writeJSON(w, http.StatusOK, map[string]any{"version": newVer.Version})
}

func (h *DocsHandler) Export(w http.ResponseWriter, r *http.Request) {
	user, roles, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	format := r.URL.Query().Get("format")
	if format == "" {
		format = docs.FormatMarkdown
	}
	id, _ := strconv.ParseInt(pathParams(r)["id"], 10, 64)
	doc, err := h.store.GetDocument(r.Context(), id)
	if err != nil || doc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !h.isDocument(doc) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	docACL, _ := h.store.GetDocACL(r.Context(), doc.ID)
	var folderACL []store.ACLRule
	if doc.FolderID != nil {
		folderACL, _ = h.store.GetFolderACL(r.Context(), *doc.FolderID)
	}
	if !h.svc.CheckACL(user, roles, doc, docACL, folderACL, "export") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if h.requiresDualExportApproval(doc) {
		approval, err := h.store.ConsumeDocExportApproval(r.Context(), doc.ID, user.ID)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if approval == nil {
			h.svc.Log(r.Context(), user.Username, "doc.export.blocked_policy", fmt.Sprintf("%s|approval_required", doc.RegNumber))
			http.Error(w, "docs.export.approvalRequired", http.StatusForbidden)
			return
		}
		h.svc.Log(r.Context(), user.Username, "doc.export.approval.used", fmt.Sprintf("%s|approved_by=%d", doc.RegNumber, approval.ApprovedBy))
	}
	ver, err := h.store.GetVersion(r.Context(), doc.ID, doc.CurrentVersion)
	if err != nil || ver == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	res, err := h.svc.Export(r.Context(), docs.ExportRequest{
		Doc:      doc,
		Version:  ver,
		Format:   format,
		Username: user.Username,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.svc.Log(r.Context(), user.Username, "doc.export", fmt.Sprintf("%s|%s", doc.RegNumber, strings.ToLower(format)))
	disposition := "attachment"
	if inline := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("inline"))); inline == "1" || inline == "true" || inline == "yes" {
		disposition = "inline"
	}
	w.Header().Set("Content-Disposition", disposition+"; filename=\""+res.Filename+"\"")
	w.Header().Set("Content-Type", res.ContentType)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(res.Data)
}

func (h *DocsHandler) StartApproval(w http.ResponseWriter, r *http.Request) {
	user, roles, err := h.currentUser(r)
	if err != nil {
		if h.logger != nil {
			h.logger.Errorf("approval.start: no session: %v", err)
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, _ := strconv.ParseInt(pathParams(r)["id"], 10, 64)
	doc, err := h.store.GetDocument(r.Context(), id)
	if err != nil || doc == nil {
		if h.logger != nil {
			h.logger.Errorf("approval.start: doc not found id=%d err=%v", id, err)
		}
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !strings.EqualFold(doc.DocType, "report") && doc.Status != docs.StatusDraft && doc.Status != docs.StatusReturned {
		if h.logger != nil {
			h.logger.Errorf("approval.start: invalid status doc_id=%d status=%s user=%s", doc.ID, doc.Status, user.Username)
		}
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}
	docACL, _ := h.store.GetDocACL(r.Context(), doc.ID)
	var folderACL []store.ACLRule
	if doc.FolderID != nil {
		folderACL, _ = h.store.GetFolderACL(r.Context(), *doc.FolderID)
	}
	if !h.svc.CheckACL(user, roles, doc, docACL, folderACL, "approve") {
		if h.logger != nil {
			h.logger.Errorf("approval.start: acl denied doc_id=%d user=%s roles=%v status=%s doc_acl=%d folder_acl=%d", doc.ID, user.Username, roles, doc.Status, len(docACL), len(folderACL))
		}
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var payload struct {
		Approvers []int64 `json:"approvers"`
		Observers []int64 `json:"observers"`
		Message   string  `json:"message"`
		Stages    []struct {
			Approvers []int64 `json:"approvers"`
			Observers []int64 `json:"observers"`
			Name      string  `json:"name"`
			Message   string  `json:"message"`
		} `json:"stages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		if h.logger != nil {
			h.logger.Errorf("approval.start: bad payload doc_id=%d user=%s err=%v", doc.ID, user.Username, err)
		}
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	type stageData struct {
		Approvers []int64
		Observers []int64
		Name      string
		Message   string
	}
	stages := []stageData{}
	if len(payload.Stages) > 0 {
		for _, st := range payload.Stages {
			stages = append(stages, stageData{
				Approvers: st.Approvers,
				Observers: st.Observers,
				Name:      strings.TrimSpace(st.Name),
				Message:   strings.TrimSpace(st.Message),
			})
		}
	} else {
		stages = append(stages, stageData{
			Approvers: payload.Approvers,
			Observers: payload.Observers,
			Name:      "",
			Message:   strings.TrimSpace(payload.Message),
		})
	}
	if len(payload.Stages) > 0 && len(payload.Observers) > 0 && len(stages) > 0 {
		stages[0].Observers = append(stages[0].Observers, payload.Observers...)
	}
	hasApprover := false
	participants := []store.ApprovalParticipant{}
	for idx, st := range stages {
		stageNum := idx + 1
		stageName := strings.TrimSpace(st.Name)
		if stageName == "" {
			stageName = fmt.Sprintf("Этап %d", stageNum)
		}
		stageMsg := strings.TrimSpace(st.Message)
		for _, id := range st.Approvers {
			participants = append(participants, store.ApprovalParticipant{UserID: id, Role: "approver", Stage: stageNum, StageName: stageName, StageMessage: stageMsg})
			hasApprover = true
		}
		for _, id := range st.Observers {
			if id == user.ID {
				continue
			}
			participants = append(participants, store.ApprovalParticipant{UserID: id, Role: "observer", Stage: stageNum, StageName: stageName, StageMessage: stageMsg})
		}
	}
	if !hasApprover {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	ap := &store.Approval{
		DocID:        doc.ID,
		Status:       docs.StatusReview,
		CurrentStage: 1,
		Message:      payload.Message,
		CreatedBy:    user.ID,
		CreatedAt:    utils.NowUTC(),
		UpdatedAt:    utils.NowUTC(),
	}
	approvalID, err := h.store.CreateApproval(r.Context(), ap, participants)
	if err != nil {
		if h.logger != nil {
			h.logger.Errorf("approval.start: create failed doc_id=%d user=%s err=%v", doc.ID, user.Username, err)
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	doc.Status = docs.StatusReview
	_ = h.store.UpdateDocument(r.Context(), doc)
	h.svc.Log(r.Context(), user.Username, "approval.start", doc.RegNumber)
	writeJSON(w, http.StatusOK, map[string]any{"approval_id": approvalID})
}

func (h *DocsHandler) ListApprovals(w http.ResponseWriter, r *http.Request) {
	user, _, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	filter := store.ApprovalFilter{UserID: user.ID, Status: r.URL.Query().Get("status")}
	items, err := h.store.ListApprovals(r.Context(), filter)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *DocsHandler) GetApproval(w http.ResponseWriter, r *http.Request) {
	user, _, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, _ := strconv.ParseInt(pathParams(r)["approval_id"], 10, 64)
	ap, parts, err := h.store.GetApproval(r.Context(), id)
	if err != nil || ap == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	allowed := false
	for _, p := range parts {
		if p.UserID == user.ID {
			allowed = true
			break
		}
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	h.svc.Log(r.Context(), user.Username, "approval.view", fmt.Sprintf("%d", ap.ID))
	writeJSON(w, http.StatusOK, map[string]any{"approval": ap, "participants": parts})
}

func (h *DocsHandler) ApprovalDecision(w http.ResponseWriter, r *http.Request) {
	user, _, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, _ := strconv.ParseInt(pathParams(r)["approval_id"], 10, 64)
	ap, parts, err := h.store.GetApproval(r.Context(), id)
	if err != nil || ap == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if ap.Status != docs.StatusReview {
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}
	var payload struct {
		Decision string `json:"decision"`
		Comment  string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Comment == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	currentStage := ap.CurrentStage
	if currentStage == 0 {
		currentStage = 1
	}
	allowed := false
	userStage := 0
	for _, p := range parts {
		if p.UserID == user.ID && p.Role == "approver" {
			if p.Stage == currentStage {
				allowed = true
				userStage = currentStage
				break
			}
			if !allowed {
				allowed = true
				userStage = p.Stage
			}
		}
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	decision := strings.ToLower(payload.Decision)
	if decision != "approve" && decision != "reject" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if userStage > 0 && userStage != currentStage {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := h.store.SaveApprovalDecision(r.Context(), ap.ID, user.ID, decision, payload.Comment, currentStage); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	// update local participants with current decision to evaluate next steps
	now := utils.NowUTC()
	for i := range parts {
		if parts[i].UserID == user.ID && parts[i].Role == "approver" {
			parts[i].Decision = &decision
			parts[i].DecidedAt = &now
		}
	}
	// update status
	newStatus := ap.Status
	nextStage := currentStage
	if decision == "reject" {
		newStatus = docs.StatusReturned
	} else {
		stageApproved := true
		stageRejected := false
		maxStage := currentStage
		for _, p := range parts {
			if p.Role == "approver" {
				if p.Stage > maxStage {
					maxStage = p.Stage
				}
				if p.Stage != currentStage {
					continue
				}
				if p.Decision == nil || *p.Decision == "" {
					stageApproved = false
				} else if *p.Decision == "reject" {
					stageRejected = true
				}
			}
		}
		if stageRejected {
			newStatus = docs.StatusReturned
		} else if stageApproved {
			if currentStage < maxStage {
				nextStage = currentStage + 1
				newStatus = docs.StatusReview
			} else {
				newStatus = docs.StatusApproved
			}
		}
	}
	ap.Status = newStatus
	ap.CurrentStage = nextStage
	ap.UpdatedAt = utils.NowUTC()
	doc, _ := h.store.GetDocument(r.Context(), ap.DocID)
	if doc != nil {
		doc.Status = newStatus
		_ = h.store.UpdateDocument(r.Context(), doc)
	}
	_ = h.store.UpdateApprovalStatus(r.Context(), ap.ID, newStatus, nextStage)
	action := "approval.approve"
	if decision == "reject" {
		action = "approval.reject"
	}
	h.svc.Log(r.Context(), user.Username, action, fmt.Sprintf("%d", ap.ID))
	writeJSON(w, http.StatusOK, map[string]any{"status": newStatus})
}

func (h *DocsHandler) ListApprovalComments(w http.ResponseWriter, r *http.Request) {
	user, _, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, _ := strconv.ParseInt(pathParams(r)["approval_id"], 10, 64)
	ap, parts, err := h.store.GetApproval(r.Context(), id)
	if err != nil || ap == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !isApprovalParticipant(parts, user.ID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	comments, err := h.store.ListApprovalComments(r.Context(), id)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	usersCache := map[int64]string{}
	var enriched []map[string]any
	for _, c := range comments {
		username := usersCache[c.UserID]
		if username == "" {
			if u, _, err := h.users.Get(r.Context(), c.UserID); err == nil && u != nil {
				username = u.Username
				if u.FullName != "" {
					username = u.FullName
				}
				usersCache[c.UserID] = username
			}
		}
		enriched = append(enriched, map[string]any{
			"id":          c.ID,
			"approval_id": c.ApprovalID,
			"user_id":     c.UserID,
			"author":      username,
			"comment":     c.Comment,
			"created_at":  c.CreatedAt,
		})
	}
	h.svc.Log(r.Context(), user.Username, "approval.comments.view", strconv.FormatInt(id, 10))
	writeJSON(w, http.StatusOK, map[string]any{"comments": enriched})
}

func (h *DocsHandler) AddApprovalComment(w http.ResponseWriter, r *http.Request) {
	user, _, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, _ := strconv.ParseInt(pathParams(r)["approval_id"], 10, 64)
	ap, parts, err := h.store.GetApproval(r.Context(), id)
	if err != nil || ap == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !isApprovalParticipant(parts, user.ID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var payload struct {
		Comment string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || strings.TrimSpace(payload.Comment) == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	c := &store.ApprovalComment{
		ApprovalID: id,
		UserID:     user.ID,
		Comment:    strings.TrimSpace(payload.Comment),
		CreatedAt:  utils.NowUTC(),
	}
	if err := h.store.SaveApprovalComment(r.Context(), c); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "approval.comment", strconv.FormatInt(id, 10))
	writeJSON(w, http.StatusOK, map[string]any{
		"id":          c.ID,
		"approval_id": c.ApprovalID,
		"user_id":     c.UserID,
		"comment":     c.Comment,
		"created_at":  c.CreatedAt,
	})
}

func (h *DocsHandler) CleanupApprovals(w http.ResponseWriter, r *http.Request) {
	user, roles, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !hasRole(roles, "superadmin") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var payload struct {
		All bool `json:"all"`
	}
	_ = json.NewDecoder(r.Body).Decode(&payload)
	removed, err := h.store.CleanupApprovals(r.Context(), payload.All)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if h.svc != nil {
		h.svc.Log(r.Context(), user.Username, "approval.cleanup", fmt.Sprintf("all=%t removed=%d", payload.All, removed))
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": removed})
}

func (h *DocsHandler) ListFolders(w http.ResponseWriter, r *http.Request) {
	user, roles, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	folders, err := h.store.ListFolders(r.Context())
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	var visible []store.Folder
	for _, f := range folders {
		if h.folderAllowsAccess(r.Context(), user, roles, &f, "view") {
			visible = append(visible, f)
		}
	}
	h.svc.Log(r.Context(), user.Username, "folder.list", "")
	writeJSON(w, http.StatusOK, map[string]any{"folders": visible})
}

func (h *DocsHandler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	user, roles, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var payload struct {
		Name                  string   `json:"name"`
		ParentID              *int64   `json:"parent_id"`
		ClassificationLevel   string   `json:"classification_level"`
		ClassificationTags    []string `json:"classification_tags"`
		InheritACL            bool     `json:"inherit_acl"`
		InheritClassification bool     `json:"inherit_classification"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	level, err := docs.ParseLevel(payload.ClassificationLevel)
	if err != nil {
		http.Error(w, "invalid level", http.StatusBadRequest)
		return
	}
	if !hasRole(roles, "superadmin") && !docs.HasClearance(docs.ClassificationLevel(user.ClearanceLevel), user.ClearanceTags, level, payload.ClassificationTags) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	f := &store.Folder{
		Name:                  payload.Name,
		ParentID:              payload.ParentID,
		ClassificationLevel:   int(level),
		ClassificationTags:    docs.NormalizeTags(payload.ClassificationTags),
		InheritACL:            payload.InheritACL,
		InheritClassification: payload.InheritClassification,
		CreatedBy:             user.ID,
	}
	id, err := h.store.CreateFolder(r.Context(), f)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	f.ID = id
	h.svc.Log(r.Context(), user.Username, "folder.create", f.Name)
	writeJSON(w, http.StatusOK, f)
}

func (h *DocsHandler) UpdateFolder(w http.ResponseWriter, r *http.Request) {
	user, _, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, _ := strconv.ParseInt(pathParams(r)["id"], 10, 64)
	f, err := h.store.GetFolder(r.Context(), id)
	if err != nil || f == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var payload struct {
		Name                  string   `json:"name"`
		ParentID              *int64   `json:"parent_id"`
		ClassificationLevel   string   `json:"classification_level"`
		ClassificationTags    []string `json:"classification_tags"`
		InheritACL            bool     `json:"inherit_acl"`
		InheritClassification bool     `json:"inherit_classification"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	level, err := docs.ParseLevel(payload.ClassificationLevel)
	if err != nil {
		http.Error(w, "invalid level", http.StatusBadRequest)
		return
	}
	if !docs.HasClearance(docs.ClassificationLevel(user.ClearanceLevel), user.ClearanceTags, level, payload.ClassificationTags) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	f.Name = payload.Name
	f.ParentID = payload.ParentID
	f.ClassificationLevel = int(level)
	f.ClassificationTags = docs.NormalizeTags(payload.ClassificationTags)
	f.InheritACL = payload.InheritACL
	f.InheritClassification = payload.InheritClassification
	if err := h.store.UpdateFolder(r.Context(), f); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "folder.update", f.Name)
	writeJSON(w, http.StatusOK, f)
}

func (h *DocsHandler) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	user, _, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, _ := strconv.ParseInt(pathParams(r)["id"], 10, 64)
	if err := h.store.DeleteFolder(r.Context(), id); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "folder.delete", strconv.FormatInt(id, 10))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *DocsHandler) GetACL(w http.ResponseWriter, r *http.Request) {
	doc, user, ok := h.loadDocForAccess(w, r, "manage")
	if !ok {
		return
	}
	acl, err := h.store.GetDocACL(r.Context(), doc.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "doc.acl.view", strconv.FormatInt(doc.ID, 10))
	writeJSON(w, http.StatusOK, map[string]any{"acl": acl})
}

func (h *DocsHandler) UpdateACL(w http.ResponseWriter, r *http.Request) {
	doc, user, ok := h.loadDocForAccess(w, r, "manage")
	if !ok {
		return
	}
	var payload struct {
		ACL []store.ACLRule `json:"acl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := h.store.SetDocACL(r.Context(), doc.ID, payload.ACL); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "doc.acl.update", strconv.FormatInt(doc.ID, 10))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *DocsHandler) UpdateClassification(w http.ResponseWriter, r *http.Request) {
	user, _, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, _ := strconv.ParseInt(pathParams(r)["id"], 10, 64)
	doc, err := h.store.GetDocument(r.Context(), id)
	if err != nil || doc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !h.isDocument(doc) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var payload struct {
		ClassificationLevel string   `json:"classification_level"`
		ClassificationTags  []string `json:"classification_tags"`
		Inherit             bool     `json:"inherit_classification"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	level, err := docs.ParseLevel(payload.ClassificationLevel)
	if err != nil {
		http.Error(w, "invalid level", http.StatusBadRequest)
		return
	}
	if !docs.HasClearance(docs.ClassificationLevel(user.ClearanceLevel), user.ClearanceTags, level, payload.ClassificationTags) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	doc.ClassificationLevel = int(level)
	doc.ClassificationTags = docs.NormalizeTags(payload.ClassificationTags)
	doc.InheritClassification = payload.Inherit
	if err := h.store.UpdateDocument(r.Context(), doc); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "doc.classification.change", doc.RegNumber)
	writeJSON(w, http.StatusOK, doc)
}

func (h *DocsHandler) ListLinks(w http.ResponseWriter, r *http.Request) {
	doc, _, ok := h.loadDocForAccess(w, r, "view")
	if !ok {
		return
	}
	items, err := h.store.ListLinks(r.Context(), doc.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"links": items})
}

func (h *DocsHandler) ListControlLinks(w http.ResponseWriter, r *http.Request) {
	doc, _, ok := h.loadDocForAccess(w, r, "view")
	if !ok {
		return
	}
	links, err := h.links.ListByTarget(r.Context(), "doc", strconv.FormatInt(doc.ID, 10))
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	type controlLink struct {
		ID           int64  `json:"id"`
		ControlID    int64  `json:"control_id"`
		Code         string `json:"code"`
		Title        string `json:"title"`
		RelationType string `json:"relation_type"`
	}
	var items []controlLink
	for _, l := range links {
		if l.SourceType != "control" {
			continue
		}
		ctrlID, err := strconv.ParseInt(l.SourceID, 10, 64)
		if err != nil || ctrlID == 0 {
			continue
		}
		ctrl, err := h.controls.GetControl(r.Context(), ctrlID)
		if err != nil || ctrl == nil {
			continue
		}
		items = append(items, controlLink{
			ID:           l.ID,
			ControlID:    ctrl.ID,
			Code:         ctrl.Code,
			Title:        ctrl.Title,
			RelationType: l.RelationType,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *DocsHandler) AddLink(w http.ResponseWriter, r *http.Request) {
	doc, _, ok := h.loadDocForAccess(w, r, "edit")
	if !ok {
		return
	}
	var payload struct {
		TargetType string `json:"target_type"`
		TargetID   string `json:"target_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if payload.TargetType == "" || payload.TargetID == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := h.store.AddLink(r.Context(), doc.ID, payload.TargetType, payload.TargetID); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *DocsHandler) DeleteLink(w http.ResponseWriter, r *http.Request) {
	doc, _, ok := h.loadDocForAccess(w, r, "edit")
	if !ok {
		return
	}
	linkID, _ := strconv.ParseInt(pathParams(r)["link_id"], 10, 64)
	items, err := h.store.ListLinks(r.Context(), doc.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	found := false
	for _, item := range items {
		if item["id"] == fmt.Sprintf("%d", linkID) {
			found = true
			break
		}
	}
	if !found {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err := h.store.DeleteLink(r.Context(), linkID); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *DocsHandler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	user, _, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	templates, err := h.store.ListTemplates(r.Context())
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "template.list", fmt.Sprintf("%d", len(templates)))
	writeJSON(w, http.StatusOK, map[string]any{"templates": templates, "converters": h.svc.ConvertersStatus()})
}

func (h *DocsHandler) SaveTemplate(w http.ResponseWriter, r *http.Request) {
	user, _, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var payload struct {
		ID                  int64                    `json:"id"`
		Name                string                   `json:"name"`
		Description         string                   `json:"description"`
		Format              string                   `json:"format"`
		Content             string                   `json:"content"`
		Variables           []store.TemplateVariable `json:"variables"`
		ClassificationLevel string                   `json:"classification_level"`
		ClassificationTags  []string                 `json:"classification_tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || strings.TrimSpace(payload.Name) == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	level, err := docs.ParseLevel(payload.ClassificationLevel)
	if err != nil {
		http.Error(w, "invalid level", http.StatusBadRequest)
		return
	}
	if !docs.HasClearance(docs.ClassificationLevel(user.ClearanceLevel), user.ClearanceTags, level, payload.ClassificationTags) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	tpl := &store.DocTemplate{
		ID:                  payload.ID,
		Name:                payload.Name,
		Description:         payload.Description,
		Format:              strings.ToLower(payload.Format),
		Content:             payload.Content,
		Variables:           payload.Variables,
		ClassificationLevel: int(level),
		ClassificationTags:  docs.NormalizeTags(payload.ClassificationTags),
		CreatedBy:           user.ID,
	}
	if tpl.Format == "" {
		tpl.Format = docs.FormatMarkdown
	}
	if tpl.ID != 0 {
		existing, _ := h.store.GetTemplate(r.Context(), tpl.ID)
		if existing != nil {
			tpl.CreatedBy = existing.CreatedBy
			tpl.CreatedAt = existing.CreatedAt
		}
	}
	if err := h.store.SaveTemplate(r.Context(), tpl); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "template.save", tpl.Name)
	writeJSON(w, http.StatusOK, map[string]any{"template": tpl})
}

func (h *DocsHandler) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	user, _, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, _ := strconv.ParseInt(pathParams(r)["id"], 10, 64)
	if err := h.store.DeleteTemplate(r.Context(), id); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "template.delete", fmt.Sprintf("%d", id))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func isApprovalParticipant(parts []store.ApprovalParticipant, userID int64) bool {
	for _, p := range parts {
		if p.UserID == userID {
			return true
		}
	}
	return false
}

func hasRole(roles []string, role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

func parseIntDefault(val string, def int) int {
	if val == "" {
		return def
	}
	if n, err := strconv.Atoi(val); err == nil {
		return n
	}
	return def
}

func buildBaseACL(user *store.User) []store.ACLRule {
	if user == nil {
		return nil
	}
	return []store.ACLRule{
		{SubjectType: "user", SubjectID: user.Username, Permission: "view"},
		{SubjectType: "user", SubjectID: user.Username, Permission: "edit"},
		{SubjectType: "user", SubjectID: user.Username, Permission: "manage"},
		{SubjectType: "user", SubjectID: user.Username, Permission: "export"},
	}
}

func buildACL(roleIDs []string, userIDs []int64) []store.ACLRule {
	var acl []store.ACLRule
	for _, r := range roleIDs {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		for _, p := range []string{"view", "edit"} {
			acl = append(acl, store.ACLRule{SubjectType: "role", SubjectID: r, Permission: p})
		}
	}
	for _, uid := range userIDs {
		if uid == 0 {
			continue
		}
		idStr := fmt.Sprintf("%d", uid)
		for _, p := range []string{"view", "edit"} {
			acl = append(acl, store.ACLRule{SubjectType: "user", SubjectID: idStr, Permission: p})
		}
	}
	return acl
}

func isAllowedUpload(format string) bool {
	switch strings.ToLower(format) {
	case docs.FormatMarkdown, docs.FormatDocx, docs.FormatPDF, docs.FormatTXT:
		return true
	default:
		return false
	}
}

func (h *DocsHandler) storeUpload(item uploadItem) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.uploads[item.ID] = item
}

func (h *DocsHandler) takeUpload(id string) (uploadItem, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	item, ok := h.uploads[id]
	if ok {
		delete(h.uploads, id)
	}
	return item, ok
}

func (h *DocsHandler) ConvertToMarkdown(w http.ResponseWriter, r *http.Request) {
	user, roles, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, _ := strconv.ParseInt(pathParams(r)["id"], 10, 64)
	doc, err := h.store.GetDocument(r.Context(), id)
	if err != nil || doc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !h.isDocument(doc) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	docACL, _ := h.store.GetDocACL(r.Context(), doc.ID)
	var folderACL []store.ACLRule
	if doc.FolderID != nil {
		folderACL, _ = h.store.GetFolderACL(r.Context(), *doc.FolderID)
	}
	if !h.svc.CheckACL(user, roles, doc, docACL, folderACL, "view") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if doc.Status == docs.StatusReview {
		ap, parts, _ := h.store.GetActiveApproval(r.Context(), doc.ID)
		if ap != nil && !isApprovalParticipant(parts, user.ID) && !hasRole(roles, "doc_admin") && !hasRole(roles, "admin") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
	}
	ver, err := h.store.GetVersion(r.Context(), doc.ID, doc.CurrentVersion)
	if err != nil || ver == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	res, err := h.svc.Export(r.Context(), docs.ExportRequest{Doc: doc, Version: ver, Format: docs.FormatMarkdown, Username: user.Username})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"content": string(res.Data)})
}

func (h *DocsHandler) currentUser(r *http.Request) (*store.User, []string, error) {
	val := r.Context().Value(auth.SessionContextKey)
	if val == nil {
		return nil, nil, errors.New("no session")
	}
	sess := val.(*store.SessionRecord)
	u, roles, err := h.users.FindByUsername(r.Context(), sess.Username)
	if err != nil || u == nil {
		return u, roles, err
	}
	groups, _ := h.users.UserGroups(r.Context(), u.ID)
	eff := auth.CalculateEffectiveAccess(u, roles, groups, h.policy)
	u.ClearanceLevel = eff.ClearanceLevel
	u.ClearanceTags = eff.ClearanceTags
	return u, eff.Roles, err
}

func (h *DocsHandler) isDocument(doc *store.Document) bool {
	if doc == nil {
		return false
	}
	return doc.DocType == "" || doc.DocType == "document"
}

func (h *DocsHandler) loadDocForAccess(w http.ResponseWriter, r *http.Request, required string) (*store.Document, *store.User, bool) {
	user, roles, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil, nil, false
	}
	id, _ := strconv.ParseInt(pathParams(r)["id"], 10, 64)
	doc, err := h.store.GetDocument(r.Context(), id)
	if err != nil || doc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return nil, nil, false
	}
	if !h.isDocument(doc) {
		http.Error(w, "not found", http.StatusNotFound)
		return nil, nil, false
	}
	docACL, _ := h.store.GetDocACL(r.Context(), doc.ID)
	var folderACL []store.ACLRule
	if doc.FolderID != nil {
		folderACL, _ = h.store.GetFolderACL(r.Context(), *doc.FolderID)
	}
	if !h.svc.CheckACL(user, roles, doc, docACL, folderACL, required) {
		http.Error(w, "not found", http.StatusNotFound)
		return nil, nil, false
	}
	if doc.Status == docs.StatusReview {
		ap, parts, _ := h.store.GetActiveApproval(r.Context(), doc.ID)
		if ap != nil && !isApprovalParticipant(parts, user.ID) && !hasRole(roles, "doc_admin") && !hasRole(roles, "admin") && !hasRole(roles, "security_officer") {
			http.Error(w, "not found", http.StatusNotFound)
			return nil, nil, false
		}
	}
	return doc, user, true
}

func (h *DocsHandler) folderAllowsAccess(ctx context.Context, user *store.User, roles []string, f *store.Folder, required string) bool {
	if user == nil || f == nil {
		return false
	}
	if !docs.HasClearance(docs.ClassificationLevel(user.ClearanceLevel), user.ClearanceTags, docs.ClassificationLevel(f.ClassificationLevel), f.ClassificationTags) {
		return false
	}
	folderACL, _ := h.store.GetFolderACL(ctx, f.ID)
	if len(folderACL) == 0 {
		return true
	}
	probe := &store.Document{
		ClassificationLevel: f.ClassificationLevel,
		ClassificationTags:  f.ClassificationTags,
		InheritACL:          true,
	}
	return h.svc.CheckACL(user, roles, probe, nil, folderACL, required)
}

func (h *DocsHandler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	user, roles, err := h.currentUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, _ := strconv.ParseInt(pathParams(r)["id"], 10, 64)
	doc, err := h.store.GetDocument(r.Context(), id)
	if err != nil || doc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !h.isDocument(doc) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	docACL, _ := h.store.GetDocACL(r.Context(), doc.ID)
	var folderACL []store.ACLRule
	if doc.FolderID != nil {
		folderACL, _ = h.store.GetFolderACL(r.Context(), *doc.FolderID)
	}
	if !h.svc.CheckACL(user, roles, doc, docACL, folderACL, "manage") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err := h.store.SoftDeleteDocument(r.Context(), id); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	dir := filepath.Join(h.cfg.Docs.StorageDir, strconv.FormatInt(id, 10))
	_ = os.RemoveAll(dir)
	h.svc.Log(r.Context(), user.Username, "doc.delete", doc.RegNumber)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
