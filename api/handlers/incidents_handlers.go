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
	"sort"
	"strconv"
	"strings"
	"time"

	"berkut-scc/config"
	"berkut-scc/core/auth"
	"berkut-scc/core/docs"
	"berkut-scc/core/incidents"
	"berkut-scc/core/rbac"
	"berkut-scc/core/store"
	"berkut-scc/core/utils"
)

type IncidentsHandler struct {
	cfg       *config.AppConfig
	store     store.IncidentsStore
	links     store.EntityLinksStore
	controls  store.ControlsStore
	users     store.UsersStore
	docsStore store.DocsStore
	policy    *rbac.Policy
	svc       *incidents.Service
	docsSvc   *docs.Service
	audits    store.AuditStore
	logger    *utils.Logger
}

func NewIncidentsHandler(cfg *config.AppConfig, is store.IncidentsStore, links store.EntityLinksStore, controls store.ControlsStore, us store.UsersStore, ds store.DocsStore, policy *rbac.Policy, svc *incidents.Service, docsSvc *docs.Service, audits store.AuditStore, logger *utils.Logger) *IncidentsHandler {
	return &IncidentsHandler{cfg: cfg, store: is, links: links, controls: controls, users: us, docsStore: ds, policy: policy, svc: svc, docsSvc: docsSvc, audits: audits, logger: logger}
}

var validIncidentSeverity = map[string]struct{}{
	"low":      {},
	"medium":   {},
	"high":     {},
	"critical": {},
}

var validIncidentStatus = map[string]struct{}{
	"draft":        {},
	"open":         {},
	"in_progress":  {},
	"contained":    {},
	"resolved":     {},
	"waiting":      {},
	"waiting_info": {},
	"approval":     {},
	"closed":       {},
}

var validDecisionOutcomes = map[string]struct{}{
	"approved": {},
	"rejected": {},
	"blocked":  {},
	"deferred": {},
	"monitor":  {},
}

const completionStageType = "closure"

func (h *IncidentsHandler) List(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	canManage := h.policy.Allowed(roles, "incidents.manage")
	filter := store.IncidentFilter{
		Search:   r.URL.Query().Get("q"),
		Status:   strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status"))),
		Severity: strings.ToLower(strings.TrimSpace(r.URL.Query().Get("severity"))),
		Limit:    parseIntDefault(r.URL.Query().Get("limit"), 0),
		Offset:   parseIntDefault(r.URL.Query().Get("offset"), 0),
	}
	if q := strings.TrimSpace(r.URL.Query().Get("status_in")); q != "" {
		parts := strings.Split(q, ",")
		for _, part := range parts {
			clean := strings.ToLower(strings.TrimSpace(part))
			if clean != "" {
				filter.StatusIn = append(filter.StatusIn, clean)
			}
		}
	}
	if r.URL.Query().Get("mine") == "1" || strings.ToLower(r.URL.Query().Get("mine")) == "true" {
		filter.MineUserID = user.ID
	}
	if r.URL.Query().Get("assigned_to_me") == "1" || strings.ToLower(r.URL.Query().Get("assigned_to_me")) == "true" {
		filter.AssignedUserID = user.ID
	}
	if r.URL.Query().Get("created_by_me") == "1" || strings.ToLower(r.URL.Query().Get("created_by_me")) == "true" {
		filter.CreatedByUserID = user.ID
	}
	if canManage && (r.URL.Query().Get("include_deleted") == "1" || strings.ToLower(r.URL.Query().Get("include_deleted")) == "true") {
		filter.IncludeDeleted = true
	}
	items, err := h.store.ListIncidents(r.Context(), filter)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	userMap := map[int64]*store.User{}
	resolveUser := func(id int64) *store.User {
		if id == 0 {
			return nil
		}
		if u, ok := userMap[id]; ok {
			return u
		}
		u, _, _ := h.users.Get(r.Context(), id)
		userMap[id] = u
		return u
	}
	var result []incidentDTO
	for _, inc := range items {
		acl, _ := h.store.GetIncidentACL(r.Context(), inc.ID)
		if inc.DeletedAt != nil {
			if !canManage || !h.svc.CheckACL(user, roles, acl, "manage") {
				continue
			}
		} else if !h.svc.CheckACL(user, roles, acl, "view") {
			continue
		}
		if !h.canViewByClassification(eff, inc.ClassificationLevel, inc.ClassificationTags) {
			continue
		}
		owner := resolveUser(inc.OwnerUserID)
		var assignee *store.User
		if inc.AssigneeUserID != nil {
			assignee = resolveUser(*inc.AssigneeUserID)
		}
		result = append(result, incidentDTO{
			Incident:     inc,
			OwnerName:    displayName(owner),
			AssigneeName: displayName(assignee),
			CaseSLA:      buildIncidentCaseSLA(inc),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

func (h *IncidentsHandler) ListIncidentsLite(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !h.policy.Allowed(roles, "incidents.view") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	limit := parseIntDefault(r.URL.Query().Get("limit"), 100)
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	filter := store.IncidentFilter{
		Search: r.URL.Query().Get("q"),
		Limit:  limit,
	}
	items, err := h.store.ListIncidents(r.Context(), filter)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	var result []map[string]any
	for _, inc := range items {
		acl, _ := h.store.GetIncidentACL(r.Context(), inc.ID)
		if !h.svc.CheckACL(user, roles, acl, "view") {
			continue
		}
		if !h.canViewByClassification(eff, inc.ClassificationLevel, inc.ClassificationTags) {
			continue
		}
		result = append(result, map[string]any{
			"id":     inc.ID,
			"reg_no": inc.RegNo,
			"title":  inc.Title,
			"status": inc.Status,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

func (h *IncidentsHandler) ListDocsLite(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !h.policy.Allowed(roles, "docs.view") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	limit := parseIntDefault(r.URL.Query().Get("limit"), 100)
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	filter := store.DocumentFilter{
		IncludeDeleted: false,
		Limit:          limit,
	}
	filter.DocType = "document"
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		filter.Search = q
	}
	docTypeFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("type")))
	docsList, err := h.docsStore.ListDocuments(r.Context(), filter)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	var items []map[string]any
	for _, doc := range docsList {
		docACL, _ := h.docsStore.GetDocACL(r.Context(), doc.ID)
		var folderACL []store.ACLRule
		if doc.FolderID != nil {
			folderACL, _ = h.docsStore.GetFolderACL(r.Context(), *doc.FolderID)
		}
		if !h.docsSvc.CheckACL(user, roles, &doc, docACL, folderACL, "view") {
			continue
		}
		if !h.canViewByClassification(eff, doc.ClassificationLevel, doc.ClassificationTags) {
			continue
		}
		kind := documentTypeFromTags(doc.ClassificationTags)
		if docTypeFilter != "" && docTypeFilter != strings.ToLower(kind) {
			continue
		}
		items = append(items, map[string]any{
			"id":      doc.ID,
			"title":   doc.Title,
			"type":    kind,
			"reg_no":  doc.RegNumber,
			"version": doc.CurrentVersion,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *IncidentsHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, _, _, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var payload struct {
		Title          string             `json:"title"`
		Description    string             `json:"description"`
		Severity       string             `json:"severity"`
		Status         string             `json:"status"`
		OwnerUserID    *int64             `json:"owner_user_id"`
		Owner          string             `json:"owner"`
		AssigneeUserID *int64             `json:"assignee_user_id"`
		Assignee       string             `json:"assignee"`
		Participants   []string           `json:"participants"`
		Meta           store.IncidentMeta `json:"meta"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	title := strings.TrimSpace(payload.Title)
	if title == "" {
		http.Error(w, "incidents.titleRequired", http.StatusBadRequest)
		return
	}
	severity := strings.ToLower(strings.TrimSpace(payload.Severity))
	if severity == "" {
		severity = "medium"
	}
	if !isValidSeverity(severity) {
		http.Error(w, "incidents.severityInvalid", http.StatusBadRequest)
		return
	}
	status := strings.ToLower(strings.TrimSpace(payload.Status))
	if status == "" {
		status = "draft"
	}
	if !isValidStatus(status) {
		http.Error(w, "incidents.statusInvalid", http.StatusBadRequest)
		return
	}
	ownerUser := user
	if payload.OwnerUserID != nil {
		ownerUser, err = h.lookupUserByID(r.Context(), *payload.OwnerUserID)
		if err != nil || ownerUser == nil {
			http.Error(w, "incidents.userNotFound", http.StatusBadRequest)
			return
		}
	} else if strings.TrimSpace(payload.Owner) != "" {
		ownerUser, err = h.lookupUserByToken(r.Context(), payload.Owner)
		if err != nil || ownerUser == nil {
			http.Error(w, "incidents.userNotFound", http.StatusBadRequest)
			return
		}
	}
	var assigneeUser *store.User
	if payload.AssigneeUserID != nil {
		assigneeUser, err = h.lookupUserByID(r.Context(), *payload.AssigneeUserID)
		if err != nil {
			http.Error(w, "incidents.userNotFound", http.StatusBadRequest)
			return
		}
	} else if strings.TrimSpace(payload.Assignee) != "" {
		assigneeUser, err = h.lookupUserByToken(r.Context(), payload.Assignee)
		if err != nil {
			http.Error(w, "incidents.userNotFound", http.StatusBadRequest)
			return
		}
	}
	participants, err := h.resolveParticipants(r.Context(), payload.Participants)
	if err != nil {
		http.Error(w, "incidents.userNotFound", http.StatusBadRequest)
		return
	}
	incident := &store.Incident{
		Title:               title,
		Description:         strings.TrimSpace(payload.Description),
		Severity:            severity,
		Status:              status,
		OwnerUserID:         ownerUser.ID,
		ClassificationLevel: int(docs.ClassificationInternal),
		ClassificationTags:  nil,
		CreatedBy:           user.ID,
		UpdatedBy:           user.ID,
		Version:             1,
		Meta:                store.NormalizeIncidentMeta(payload.Meta),
	}
	if assigneeUser != nil {
		incident.AssigneeUserID = &assigneeUser.ID
	}
	if _, err := h.store.CreateIncident(r.Context(), incident, participants, nil, h.cfg.Incidents.RegNoFormat); err != nil {
		http.Error(w, "incidents.regNoFailed", http.StatusInternalServerError)
		return
	}
	created, err := h.store.GetIncident(r.Context(), incident.ID)
	if err != nil || created == nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "incident.create", created.RegNo)
	h.addTimeline(r.Context(), created.ID, "incident.create", "incident created", user.ID)
	writeJSON(w, http.StatusCreated, incidentDTO{
		Incident:     *created,
		OwnerName:    displayName(ownerUser),
		AssigneeName: displayName(assigneeUser),
		CaseSLA:      buildIncidentCaseSLA(*created),
	})
}

func (h *IncidentsHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, _ := strconv.ParseInt(pathParams(r)["id"], 10, 64)
	incident, err := h.store.GetIncident(r.Context(), id)
	if err != nil || incident == nil {
		http.Error(w, "incidents.notFound", http.StatusNotFound)
		return
	}
	acl, _ := h.store.GetIncidentACL(r.Context(), incident.ID)
	canManage := h.policy.Allowed(roles, "incidents.manage")
	if incident.DeletedAt != nil {
		if !canManage || !h.svc.CheckACL(user, roles, acl, "manage") {
			http.Error(w, "incidents.notFound", http.StatusNotFound)
			return
		}
	} else if !h.svc.CheckACL(user, roles, acl, "view") {
		http.Error(w, "incidents.notFound", http.StatusNotFound)
		return
	}
	if !h.canViewByClassification(eff, incident.ClassificationLevel, incident.ClassificationTags) {
		http.Error(w, "incidents.notFound", http.StatusNotFound)
		return
	}
	parts, _ := h.store.ListIncidentParticipants(r.Context(), incident.ID)
	h.populateParticipantNames(r.Context(), parts)
	owner, _, _ := h.users.Get(r.Context(), incident.OwnerUserID)
	var assignee *store.User
	if incident.AssigneeUserID != nil {
		assignee, _, _ = h.users.Get(r.Context(), *incident.AssigneeUserID)
	}
	h.svc.Log(r.Context(), user.Username, "incident.view", incident.RegNo)
	writeJSON(w, http.StatusOK, map[string]any{
		"incident": incidentDTO{
			Incident:     *incident,
			OwnerName:    displayName(owner),
			AssigneeName: displayName(assignee),
			CaseSLA:      buildIncidentCaseSLA(*incident),
		},
		"participants": parts,
	})
}

func (h *IncidentsHandler) Update(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "edit")
	if !ok {
		return
	}
	var payload struct {
		Title               *string             `json:"title"`
		Description         *string             `json:"description"`
		Severity            *string             `json:"severity"`
		Status              *string             `json:"status"`
		ClassificationLevel *string             `json:"classification_level"`
		ClassificationTags  []string            `json:"classification_tags"`
		OwnerUserID         *int64              `json:"owner_user_id"`
		Owner               *string             `json:"owner"`
		AssigneeUserID      *int64              `json:"assignee_user_id"`
		Assignee            *string             `json:"assignee"`
		Participants        []string            `json:"participants"`
		Meta                *store.IncidentMeta `json:"meta"`
		Version             int                 `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	expectedVersion := payload.Version
	if expectedVersion == 0 {
		if hdr := strings.TrimSpace(r.Header.Get("If-Match")); hdr != "" {
			if v, err := strconv.Atoi(strings.Trim(hdr, "\"")); err == nil {
				expectedVersion = v
			}
		}
	}
	if expectedVersion == 0 {
		http.Error(w, "incidents.versionRequired", http.StatusBadRequest)
		return
	}
	updated := *incident
	if strings.ToLower(incident.Status) == "closed" {
		http.Error(w, "incidents.closedReadOnly", http.StatusConflict)
		return
	}
	if payload.Title != nil {
		updated.Title = strings.TrimSpace(*payload.Title)
	}
	if payload.Description != nil {
		updated.Description = strings.TrimSpace(*payload.Description)
	}
	if payload.Severity != nil {
		sev := strings.ToLower(strings.TrimSpace(*payload.Severity))
		if !isValidSeverity(sev) {
			http.Error(w, "incidents.severityInvalid", http.StatusBadRequest)
			return
		}
		updated.Severity = sev
	}
	if payload.Status != nil {
		st := strings.ToLower(strings.TrimSpace(*payload.Status))
		if st != "" {
			if !isValidStatus(st) {
				http.Error(w, "incidents.statusInvalid", http.StatusBadRequest)
				return
			}
			if st == "closed" {
				http.Error(w, "incidents.closeUseAction", http.StatusBadRequest)
				return
			}
			updated.Status = st
		}
	}
	if payload.ClassificationLevel != nil || payload.ClassificationTags != nil {
		if !h.canEditClassification(user, roles) {
			http.Error(w, "incidents.forbidden", http.StatusForbidden)
			return
		}
		level := updated.ClassificationLevel
		if payload.ClassificationLevel != nil {
			parsed, err := parseIncidentClassificationLevel(*payload.ClassificationLevel)
			if err != nil {
				http.Error(w, "invalid level", http.StatusBadRequest)
				return
			}
			level = parsed
		}
		tags := updated.ClassificationTags
		if payload.ClassificationTags != nil {
			tags = docs.NormalizeTags(payload.ClassificationTags)
		}
		if !h.canViewByClassification(eff, level, tags) {
			http.Error(w, "incidents.forbidden", http.StatusForbidden)
			return
		}
		updated.ClassificationLevel = level
		updated.ClassificationTags = tags
	}
	if payload.Meta != nil {
		updated.Meta = store.NormalizeIncidentMeta(*payload.Meta)
	}
	var ownerUser *store.User
	if payload.OwnerUserID != nil {
		ownerUser, err = h.lookupUserByID(r.Context(), *payload.OwnerUserID)
		if err != nil || ownerUser == nil {
			http.Error(w, "incidents.userNotFound", http.StatusBadRequest)
			return
		}
		updated.OwnerUserID = ownerUser.ID
	} else if payload.Owner != nil {
		if strings.TrimSpace(*payload.Owner) != "" {
			ownerUser, err = h.lookupUserByToken(r.Context(), *payload.Owner)
			if err != nil || ownerUser == nil {
				http.Error(w, "incidents.userNotFound", http.StatusBadRequest)
				return
			}
			updated.OwnerUserID = ownerUser.ID
		}
	}
	var assigneeUser *store.User
	if payload.AssigneeUserID != nil {
		if *payload.AssigneeUserID == 0 {
			updated.AssigneeUserID = nil
		} else {
			assigneeUser, err = h.lookupUserByID(r.Context(), *payload.AssigneeUserID)
			if err != nil {
				http.Error(w, "incidents.userNotFound", http.StatusBadRequest)
				return
			}
			updated.AssigneeUserID = &assigneeUser.ID
		}
	} else if payload.Assignee != nil {
		if strings.TrimSpace(*payload.Assignee) == "" {
			updated.AssigneeUserID = nil
		} else {
			assigneeUser, err = h.lookupUserByToken(r.Context(), *payload.Assignee)
			if err != nil {
				http.Error(w, "incidents.userNotFound", http.StatusBadRequest)
				return
			}
			updated.AssigneeUserID = &assigneeUser.ID
		}
	}
	updated.UpdatedBy = user.ID
	if err := h.store.UpdateIncident(r.Context(), &updated, expectedVersion); err != nil {
		if errors.Is(err, store.ErrConflict) {
			http.Error(w, "incidents.conflictVersion", http.StatusConflict)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	statusChanged := payload.Status != nil && incident.Status != updated.Status
	severityChanged := payload.Severity != nil && incident.Severity != updated.Severity
	assigneeChanged := (payload.AssigneeUserID != nil || payload.Assignee != nil) && !sameAssignee(incident.AssigneeUserID, updated.AssigneeUserID)
	ownerChanged := (payload.OwnerUserID != nil || payload.Owner != nil) && incident.OwnerUserID != updated.OwnerUserID
	classChanged := (payload.ClassificationLevel != nil || payload.ClassificationTags != nil) && (incident.ClassificationLevel != updated.ClassificationLevel || !sameTags(incident.ClassificationTags, updated.ClassificationTags))
	if statusChanged {
		h.addTimeline(r.Context(), incident.ID, "status.change", fmt.Sprintf("%s -> %s", incident.Status, updated.Status), user.ID)
	}
	if severityChanged {
		h.addTimeline(r.Context(), incident.ID, "severity.change", fmt.Sprintf("%s -> %s", incident.Severity, updated.Severity), user.ID)
	}
	if assigneeChanged {
		h.addTimeline(r.Context(), incident.ID, "assignee.change", "assignee updated", user.ID)
	}
	if classChanged {
		h.addTimeline(r.Context(), incident.ID, "classification.change", "classification updated", user.ID)
	}
	if ownerChanged {
		h.addTimeline(r.Context(), incident.ID, "owner.change", "owner updated", user.ID)
	}
	if payload.Participants != nil {
		participants, err := h.resolveParticipants(r.Context(), payload.Participants)
		if err != nil {
			http.Error(w, "incidents.userNotFound", http.StatusBadRequest)
			return
		}
		if err := h.store.SetIncidentParticipants(r.Context(), incident.ID, participants); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		h.ensureParticipantACL(r.Context(), incident.ID, participants)
		h.svc.Log(r.Context(), user.Username, "incident.participants.update", incident.RegNo)
	}
	if payload.AssigneeUserID != nil || payload.Assignee != nil {
		h.ensureAssigneeACL(r.Context(), incident.ID, updated.AssigneeUserID)
		h.svc.Log(r.Context(), user.Username, "incident.assignee.change", incident.RegNo)
	}
	if ownerChanged {
		h.svc.Log(r.Context(), user.Username, "incident.owner.change", incident.RegNo)
	}
	if statusChanged {
		h.svc.Log(r.Context(), user.Username, "incident.status.change", incident.RegNo)
	}
	if severityChanged {
		h.svc.Log(r.Context(), user.Username, "incident.severity.change", incident.RegNo)
	}
	if classChanged {
		h.svc.Log(r.Context(), user.Username, "incident.classification.change", incident.RegNo)
	}
	h.svc.Log(r.Context(), user.Username, "incident.update", incident.RegNo)
	var owner *store.User
	if ownerUser != nil {
		owner = ownerUser
	} else {
		owner, _, _ = h.users.Get(r.Context(), updated.OwnerUserID)
	}
	if updated.AssigneeUserID != nil {
		assigneeUser, _ = h.lookupUserByID(r.Context(), *updated.AssigneeUserID)
	}
	writeJSON(w, http.StatusOK, incidentDTO{
		Incident:     updated,
		OwnerName:    displayName(owner),
		AssigneeName: displayName(assigneeUser),
		CaseSLA:      buildIncidentCaseSLA(updated),
	})
}

func (h *IncidentsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, _ := strconv.ParseInt(pathParams(r)["id"], 10, 64)
	incident, err := h.store.GetIncident(r.Context(), id)
	if err != nil || incident == nil {
		http.Error(w, "incidents.notFound", http.StatusNotFound)
		return
	}
	acl, _ := h.store.GetIncidentACL(r.Context(), incident.ID)
	if !h.svc.CheckACL(user, roles, acl, "manage") {
		http.Error(w, "incidents.notFound", http.StatusNotFound)
		return
	}
	if !h.canViewByClassification(eff, incident.ClassificationLevel, incident.ClassificationTags) {
		http.Error(w, "incidents.notFound", http.StatusNotFound)
		return
	}
	if strings.ToLower(incident.Status) == "closed" {
		http.Error(w, "incidents.closedReadOnly", http.StatusConflict)
		return
	}
	if incident.DeletedAt != nil {
		http.Error(w, "incidents.deleted", http.StatusBadRequest)
		return
	}
	if err := h.store.SoftDeleteIncident(r.Context(), incident.ID, user.ID); err != nil {
		if errors.Is(err, store.ErrConflict) {
			http.Error(w, "incidents.conflictVersion", http.StatusConflict)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "incident.delete", incident.RegNo)
	h.addTimeline(r.Context(), incident.ID, "incident.delete", "incident deleted", user.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *IncidentsHandler) Restore(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, _ := strconv.ParseInt(pathParams(r)["id"], 10, 64)
	incident, err := h.store.GetIncident(r.Context(), id)
	if err != nil || incident == nil {
		http.Error(w, "incidents.notFound", http.StatusNotFound)
		return
	}
	acl, _ := h.store.GetIncidentACL(r.Context(), incident.ID)
	if !h.svc.CheckACL(user, roles, acl, "manage") {
		http.Error(w, "incidents.notFound", http.StatusNotFound)
		return
	}
	if !h.canViewByClassification(eff, incident.ClassificationLevel, incident.ClassificationTags) {
		http.Error(w, "incidents.notFound", http.StatusNotFound)
		return
	}
	if err := h.store.RestoreIncident(r.Context(), incident.ID, user.ID); err != nil {
		if errors.Is(err, store.ErrConflict) {
			http.Error(w, "incidents.conflictVersion", http.StatusConflict)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "incident.restore", incident.RegNo)
	h.addTimeline(r.Context(), incident.ID, "incident.restore", "incident restored", user.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *IncidentsHandler) CloseIncident(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "edit")
	if !ok {
		return
	}
	if strings.ToLower(incident.Status) == "closed" {
		http.Error(w, "incidents.closedReadOnly", http.StatusConflict)
		return
	}
	stages, err := h.store.ListIncidentStages(r.Context(), incident.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	var completionStage *store.IncidentStage
	var completionPayload stageContentPayload
	for idx := range stages {
		st := stages[idx]
		if st.IsDefault {
			continue
		}
		entry, err := h.store.GetStageEntry(r.Context(), st.ID)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		payload := parseStageContentPayload(entry)
		stageType := strings.ToLower(strings.TrimSpace(payload.StageType))
		if stageType == completionStageType {
			completionStage = &st
			completionPayload = payload
			break
		}
	}
	if completionStage == nil {
		http.Error(w, "incidents.completionStageMissing", http.StatusBadRequest)
		return
	}
	if strings.ToLower(completionStage.Status) != "done" {
		http.Error(w, "incidents.completionStageNotDone", http.StatusBadRequest)
		return
	}
	if !hasDecisionEntry(completionPayload) {
		http.Error(w, "incidents.completionStageEmpty", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(incident.Meta.Postmortem) == "" && (incident.Severity == "high" || incident.Severity == "critical") {
		http.Error(w, "incidents.postmortemRequired", http.StatusBadRequest)
		return
	}
	if outcome := extractDecisionOutcome(completionPayload); outcome != "" {
		updated := *incident
		updated.Meta = store.NormalizeIncidentMeta(updated.Meta)
		updated.Meta.ClosureOutcome = outcome
		updated.UpdatedBy = user.ID
		if err := h.store.UpdateIncident(r.Context(), &updated, incident.Version); err != nil {
			if errors.Is(err, store.ErrConflict) {
				http.Error(w, "incidents.conflictVersion", http.StatusConflict)
				return
			}
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		incident = &updated
	}
	updated, err := h.store.CloseIncident(r.Context(), incident.ID, user.ID)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			http.Error(w, "incidents.closedReadOnly", http.StatusConflict)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "incidents.closed", updated.RegNo)
	h.addTimeline(r.Context(), incident.ID, "incident.closed", "incident closed", user.ID)
	writeJSON(w, http.StatusOK, map[string]any{"incident": updated})
}

func (h *IncidentsHandler) SavePostmortem(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "edit")
	if !ok {
		return
	}
	var payload struct {
		Postmortem string `json:"postmortem"`
		Version    int    `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	updated := *incident
	updated.Meta = store.NormalizeIncidentMeta(updated.Meta)
	updated.Meta.Postmortem = strings.TrimSpace(payload.Postmortem)
	updated.UpdatedBy = user.ID
	expected := payload.Version
	if expected <= 0 {
		expected = incident.Version
	}
	if err := h.store.UpdateIncident(r.Context(), &updated, expected); err != nil {
		if errors.Is(err, store.ErrConflict) {
			http.Error(w, "incidents.conflictVersion", http.StatusConflict)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "incident.postmortem.update", incident.RegNo)
	h.addTimeline(r.Context(), incident.ID, "incident.postmortem.update", "postmortem updated", user.ID)
	writeJSON(w, http.StatusOK, map[string]any{"incident": updated})
}

func (h *IncidentsHandler) ListStages(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "view")
	if !ok {
		return
	}
	stages, err := h.store.ListIncidentStages(r.Context(), incident.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": stages})
}

func (h *IncidentsHandler) GetStage(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "view")
	if !ok {
		return
	}
	stageID, _ := strconv.ParseInt(pathParams(r)["stage_id"], 10, 64)
	stage, err := h.store.GetIncidentStage(r.Context(), stageID)
	if err != nil || stage == nil || stage.IncidentID != incident.ID {
		http.Error(w, "incidents.notFound", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, stage)
}

func (h *IncidentsHandler) AddStage(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "edit")
	if !ok {
		return
	}
	var payload struct {
		Title    string `json:"title"`
		Position int    `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	title := strings.TrimSpace(payload.Title)
	if title == "" {
		http.Error(w, "incidents.stageTitleRequired", http.StatusBadRequest)
		return
	}
	position := payload.Position
	if position <= 0 {
		pos, err := h.store.NextStagePosition(r.Context(), incident.ID)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		position = pos
	}
	stage := &store.IncidentStage{
		IncidentID: incident.ID,
		Title:      title,
		Position:   position,
		CreatedBy:  user.ID,
		UpdatedBy:  user.ID,
		IsDefault:  false,
		Version:    1,
	}
	if _, err := h.store.CreateIncidentStage(r.Context(), stage); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	entry := &store.IncidentStageEntry{
		StageID:   stage.ID,
		Content:   "",
		CreatedBy: user.ID,
		UpdatedBy: user.ID,
		Version:   1,
	}
	if _, err := h.store.CreateStageEntry(r.Context(), entry); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "incident.stage.add", fmt.Sprintf("%s|%d", incident.RegNo, stage.ID))
	h.addTimeline(r.Context(), incident.ID, "stage.add", fmt.Sprintf("stage added: %s", stage.Title), user.ID)
	writeJSON(w, http.StatusCreated, map[string]any{
		"stage": stage,
		"entry": entry,
	})
}

func (h *IncidentsHandler) UpdateStage(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "edit")
	if !ok {
		return
	}
	stageID, _ := strconv.ParseInt(pathParams(r)["stage_id"], 10, 64)
	stage, err := h.store.GetIncidentStage(r.Context(), stageID)
	if err != nil || stage == nil || stage.IncidentID != incident.ID {
		http.Error(w, "incidents.notFound", http.StatusNotFound)
		return
	}
	if strings.ToLower(stage.Status) == "done" {
		http.Error(w, "incidents.stageCompletedReadOnly", http.StatusConflict)
		return
	}
	var payload struct {
		Title    *string `json:"title"`
		Position *int    `json:"position"`
		Version  int     `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	expectedVersion := payload.Version
	if expectedVersion == 0 {
		http.Error(w, "incidents.versionRequired", http.StatusBadRequest)
		return
	}
	updated := *stage
	if payload.Title != nil {
		updated.Title = strings.TrimSpace(*payload.Title)
	}
	if payload.Position != nil && *payload.Position > 0 {
		updated.Position = *payload.Position
	}
	updated.UpdatedBy = user.ID
	if err := h.store.UpdateIncidentStage(r.Context(), &updated, expectedVersion); err != nil {
		if errors.Is(err, store.ErrConflict) {
			http.Error(w, "incidents.conflictVersion", http.StatusConflict)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if payload.Title != nil && stage.Title != updated.Title {
		h.svc.Log(r.Context(), user.Username, "incident.stage.rename", fmt.Sprintf("%s|%d", incident.RegNo, stage.ID))
		h.addTimeline(r.Context(), incident.ID, "stage.rename", fmt.Sprintf("stage renamed: %s", updated.Title), user.ID)
	}
	if payload.Position != nil && stage.Position != updated.Position {
		h.svc.Log(r.Context(), user.Username, "incident.stage.reorder", fmt.Sprintf("%s|%d", incident.RegNo, stage.ID))
		h.addTimeline(r.Context(), incident.ID, "stage.reorder", fmt.Sprintf("stage reordered: %s", updated.Title), user.ID)
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *IncidentsHandler) DeleteStage(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "edit")
	if !ok {
		return
	}
	stageID, _ := strconv.ParseInt(pathParams(r)["stage_id"], 10, 64)
	stage, err := h.store.GetIncidentStage(r.Context(), stageID)
	if err != nil || stage == nil || stage.IncidentID != incident.ID {
		http.Error(w, "incidents.notFound", http.StatusNotFound)
		return
	}
	if stage.IsDefault {
		http.Error(w, "incidents.cannotDeleteOverview", http.StatusBadRequest)
		return
	}
	if strings.ToLower(stage.Status) == "done" {
		http.Error(w, "incidents.stageCompletedReadOnly", http.StatusConflict)
		return
	}
	if err := h.store.DeleteIncidentStage(r.Context(), stage.ID); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "incident.stage.delete", fmt.Sprintf("%s|%d", incident.RegNo, stage.ID))
	h.addTimeline(r.Context(), incident.ID, "stage.delete", fmt.Sprintf("stage deleted: %s", stage.Title), user.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *IncidentsHandler) CompleteStage(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "edit")
	if !ok {
		return
	}
	stageID, _ := strconv.ParseInt(pathParams(r)["stage_id"], 10, 64)
	stage, err := h.store.GetIncidentStage(r.Context(), stageID)
	if err != nil || stage == nil || stage.IncidentID != incident.ID {
		http.Error(w, "incidents.notFound", http.StatusNotFound)
		return
	}
	if strings.ToLower(stage.Status) == "done" {
		http.Error(w, "incidents.stageCompletedReadOnly", http.StatusConflict)
		return
	}
	updated, err := h.store.CompleteIncidentStage(r.Context(), stage.ID, user.ID)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			http.Error(w, "incidents.stageCompletedReadOnly", http.StatusConflict)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "incidents.stage_completed", fmt.Sprintf("%s|%d", incident.RegNo, stage.ID))
	h.addTimeline(r.Context(), incident.ID, "stage.completed", fmt.Sprintf("stage completed: %s", stage.Title), user.ID)
	writeJSON(w, http.StatusOK, updated)
}

func (h *IncidentsHandler) GetStageContent(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "view")
	if !ok {
		return
	}
	stageID, _ := strconv.ParseInt(pathParams(r)["stage_id"], 10, 64)
	stage, err := h.store.GetIncidentStage(r.Context(), stageID)
	if err != nil || stage == nil || stage.IncidentID != incident.ID {
		http.Error(w, "incidents.notFound", http.StatusNotFound)
		return
	}
	entry, err := h.store.GetStageEntry(r.Context(), stage.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if entry == nil {
		entry = &store.IncidentStageEntry{StageID: stage.ID, Content: "", Version: 1}
	}
	writeJSON(w, http.StatusOK, entry)
}

func (h *IncidentsHandler) UpdateStageContent(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "edit")
	if !ok {
		return
	}
	stageID, _ := strconv.ParseInt(pathParams(r)["stage_id"], 10, 64)
	stage, err := h.store.GetIncidentStage(r.Context(), stageID)
	if err != nil || stage == nil || stage.IncidentID != incident.ID {
		http.Error(w, "incidents.notFound", http.StatusNotFound)
		return
	}
	if strings.ToLower(stage.Status) == "done" {
		http.Error(w, "incidents.stageCompletedReadOnly", http.StatusConflict)
		return
	}
	var payload struct {
		Content      string `json:"content"`
		ChangeReason string `json:"change_reason"`
		Version      int    `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if payload.Version == 0 {
		http.Error(w, "incidents.versionRequired", http.StatusBadRequest)
		return
	}
	entry := &store.IncidentStageEntry{
		StageID:      stage.ID,
		Content:      payload.Content,
		ChangeReason: strings.TrimSpace(payload.ChangeReason),
		UpdatedBy:    user.ID,
	}
	if err := h.store.UpdateStageEntry(r.Context(), entry, payload.Version); err != nil {
		if errors.Is(err, store.ErrConflict) {
			http.Error(w, "incidents.conflictVersion", http.StatusConflict)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "incident.stage.content.update", fmt.Sprintf("%s|%d", incident.RegNo, stage.ID))
	h.addTimeline(r.Context(), incident.ID, "stage.content.update", fmt.Sprintf("stage content updated: %s", stage.Title), user.ID)
	writeJSON(w, http.StatusOK, entry)
}

func (h *IncidentsHandler) GetACL(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "manage")
	if !ok {
		return
	}
	acl, err := h.store.GetIncidentACL(r.Context(), incident.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": acl})
}

func (h *IncidentsHandler) UpdateACL(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "manage")
	if !ok {
		return
	}
	var payload struct {
		Items []store.ACLRule `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	for i := range payload.Items {
		payload.Items[i].SubjectType = strings.ToLower(strings.TrimSpace(payload.Items[i].SubjectType))
		payload.Items[i].SubjectID = strings.TrimSpace(payload.Items[i].SubjectID)
		payload.Items[i].Permission = strings.ToLower(strings.TrimSpace(payload.Items[i].Permission))
		if payload.Items[i].SubjectType == "" || payload.Items[i].SubjectID == "" || !isValidIncidentPerm(payload.Items[i].Permission) {
			http.Error(w, "incidents.aclInvalid", http.StatusBadRequest)
			return
		}
	}
	if err := h.store.SetIncidentACL(r.Context(), incident.ID, payload.Items); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "incident.acl.update", incident.RegNo)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *IncidentsHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	items, err := h.store.ListIncidents(r.Context(), store.IncidentFilter{IncludeDeleted: false})
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	userMap := map[int64]*store.User{}
	resolveUser := func(id int64) *store.User {
		if id == 0 {
			return nil
		}
		if u, ok := userMap[id]; ok {
			return u
		}
		u, _, _ := h.users.Get(r.Context(), id)
		userMap[id] = u
		return u
	}
	var visible []store.Incident
	for _, inc := range items {
		acl, _ := h.store.GetIncidentACL(r.Context(), inc.ID)
		if !h.svc.CheckACL(user, roles, acl, "view") {
			continue
		}
		if !h.canViewByClassification(eff, inc.ClassificationLevel, inc.ClassificationTags) {
			continue
		}
		visible = append(visible, inc)
	}
	sortByUpdated := func(a, b store.Incident) bool {
		return a.UpdatedAt.After(b.UpdatedAt)
	}
	sortIncidents := func(list []store.Incident) {
		sort.Slice(list, func(i, j int) bool { return sortByUpdated(list[i], list[j]) })
	}
	toDTO := func(list []store.Incident) []incidentDTO {
		res := make([]incidentDTO, 0, len(list))
		for _, inc := range list {
			owner := resolveUser(inc.OwnerUserID)
			var assignee *store.User
			if inc.AssigneeUserID != nil {
				assignee = resolveUser(*inc.AssigneeUserID)
			}
			res = append(res, incidentDTO{
				Incident:     inc,
				OwnerName:    displayName(owner),
				AssigneeName: displayName(assignee),
				CaseSLA:      buildIncidentCaseSLA(inc),
			})
		}
		return res
	}
	metrics := map[string]int{"open": 0, "in_progress": 0, "closed": 0, "critical": 0}
	for _, inc := range visible {
		switch inc.Status {
		case "open":
			metrics["open"]++
		case "in_progress":
			metrics["in_progress"]++
		case "closed":
			metrics["closed"]++
		}
		if inc.Severity == "critical" {
			metrics["critical"]++
		}
	}
	var mine []store.Incident
	var attention []store.Incident
	for _, inc := range visible {
		if inc.OwnerUserID == user.ID || (inc.AssigneeUserID != nil && *inc.AssigneeUserID == user.ID) {
			mine = append(mine, inc)
		}
		if (inc.Severity == "critical" || inc.Severity == "high") && inc.Status != "closed" {
			attention = append(attention, inc)
		}
	}
	sortIncidents(mine)
	sortIncidents(attention)
	sortIncidents(visible)
	writeJSON(w, http.StatusOK, map[string]any{
		"metrics":   metrics,
		"mine":      toDTO(sliceLimit(mine, 10)),
		"attention": toDTO(sliceLimit(attention, 10)),
		"recent":    toDTO(sliceLimit(visible, 10)),
	})
}

func (h *IncidentsHandler) ListLinks(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "view")
	if !ok {
		return
	}
	links, err := h.store.ListIncidentLinks(r.Context(), incident.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": links})
}

func (h *IncidentsHandler) ListControlLinks(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "view")
	if !ok {
		return
	}
	links, err := h.links.ListByTarget(r.Context(), "incident", strconv.FormatInt(incident.ID, 10))
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

func (h *IncidentsHandler) AddLink(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "edit")
	if !ok {
		return
	}
	var payload struct {
		TargetType string `json:"target_type"`
		TargetID   string `json:"target_id"`
		Comment    string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	targetType := strings.ToLower(strings.TrimSpace(payload.TargetType))
	targetID := strings.TrimSpace(payload.TargetID)
	comment := strings.TrimSpace(payload.Comment)
	if targetType == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	allowedTypes := map[string]bool{"doc": true, "incident": true, "report": true, "other": true}
	if !allowedTypes[targetType] {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	link := &store.IncidentLink{
		IncidentID: incident.ID,
		EntityType: targetType,
		EntityID:   targetID,
		Comment:    comment,
		CreatedBy:  user.ID,
		Unverified: true,
	}
	switch targetType {
	case "doc", "report":
		docID, err := strconv.ParseInt(targetID, 10, 64)
		if err != nil || docID == 0 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		doc, err := h.docsStore.GetDocument(r.Context(), docID)
		if err != nil || doc == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		docACL, _ := h.docsStore.GetDocACL(r.Context(), doc.ID)
		var folderACL []store.ACLRule
		if doc.FolderID != nil {
			folderACL, _ = h.docsStore.GetFolderACL(r.Context(), *doc.FolderID)
		}
		if !h.docsSvc.CheckACL(user, roles, doc, docACL, folderACL, "view") || !h.canViewByClassification(eff, doc.ClassificationLevel, doc.ClassificationTags) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		kind := strings.ToLower(documentTypeFromTags(doc.ClassificationTags))
		if targetType == "report" && kind != "report" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		link.Title = fmt.Sprintf("%s (%s)", doc.Title, doc.RegNumber)
		link.EntityID = fmt.Sprintf("%d", doc.ID)
		link.Unverified = false
	case "incident":
		refID, err := strconv.ParseInt(targetID, 10, 64)
		if err != nil || refID == 0 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		ref, err := h.store.GetIncident(r.Context(), refID)
		if err != nil || ref == nil || ref.DeletedAt != nil {
			http.Error(w, "incidents.notFound", http.StatusNotFound)
			return
		}
		refACL, _ := h.store.GetIncidentACL(r.Context(), ref.ID)
		if !h.svc.CheckACL(user, roles, refACL, "view") || !h.canViewByClassification(eff, ref.ClassificationLevel, ref.ClassificationTags) {
			http.Error(w, "incidents.notFound", http.StatusNotFound)
			return
		}
		link.Title = fmt.Sprintf("%s ? %s", ref.RegNo, ref.Title)
		link.EntityID = fmt.Sprintf("%d", ref.ID)
		link.Unverified = false
	case "other":
		if comment == "" {
			http.Error(w, "incidents.links.commentRequired", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(targetID) == "" {
			link.EntityID = fmt.Sprintf("other-%d-%d", incident.ID, time.Now().UTC().UnixNano())
		}
		link.Title = comment
	default:
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if _, err := h.store.AddIncidentLink(r.Context(), link); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			http.Error(w, "conflict", http.StatusConflict)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "incident.link.add", fmt.Sprintf("%s|%s|%s", incident.RegNo, link.EntityType, link.EntityID))
	h.addTimeline(r.Context(), incident.ID, "link.add", fmt.Sprintf("%s:%s", link.EntityType, link.EntityID), user.ID)
	writeJSON(w, http.StatusCreated, link)
}

func (h *IncidentsHandler) DeleteLink(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "edit")
	if !ok {
		return
	}
	linkID, _ := strconv.ParseInt(pathParams(r)["link_id"], 10, 64)
	if linkID == 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	links, err := h.store.ListIncidentLinks(r.Context(), incident.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	var target *store.IncidentLink
	for i := range links {
		if links[i].ID == linkID {
			target = &links[i]
			break
		}
	}
	if target == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err := h.store.DeleteIncidentLink(r.Context(), linkID); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "incident.link.remove", fmt.Sprintf("%s|%s|%s", incident.RegNo, target.EntityType, target.EntityID))
	h.addTimeline(r.Context(), incident.ID, "link.remove", fmt.Sprintf("%s:%s", target.EntityType, target.EntityID), user.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *IncidentsHandler) ListAttachments(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "view")
	if !ok {
		return
	}
	items, err := h.store.ListIncidentAttachments(r.Context(), incident.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	userMap := map[int64]*store.User{}
	resolveUser := func(id int64) *store.User {
		if id == 0 {
			return nil
		}
		if u, ok := userMap[id]; ok {
			return u
		}
		u, _, _ := h.users.Get(r.Context(), id)
		userMap[id] = u
		return u
	}
	var out []attachmentDTO
	for _, att := range items {
		if !h.canViewByClassification(eff, att.ClassificationLevel, att.ClassificationTags) {
			continue
		}
		uploadedBy := resolveUser(att.UploadedBy)
		out = append(out, attachmentDTO{
			IncidentAttachment: att,
			UploadedByName:     displayName(uploadedBy),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *IncidentsHandler) UploadAttachment(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "edit")
	if !ok {
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
	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	classLevel := incident.ClassificationLevel
	classTags := incident.ClassificationTags
	if raw := strings.TrimSpace(r.FormValue("classification_level")); raw != "" {
		parsed, err := parseIncidentClassificationLevel(raw)
		if err != nil {
			http.Error(w, "invalid level", http.StatusBadRequest)
			return
		}
		classLevel = parsed
	}
	if classLevel < incident.ClassificationLevel {
		classLevel = incident.ClassificationLevel
	}
	if tags := r.MultipartForm.Value["classification_tags"]; len(tags) > 0 {
		classTags = docs.NormalizeTags(tags)
	}
	if !h.canViewByClassification(eff, classLevel, classTags) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	enc := h.svc.Encryptor()
	if enc == nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	blob, err := enc.EncryptToBlob(data)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	att := &store.IncidentAttachment{
		IncidentID:          incident.ID,
		Filename:            header.Filename,
		ContentType:         header.Header.Get("Content-Type"),
		SizeBytes:           int64(len(data)),
		SHA256Plain:         utils.Sha256Hex(data),
		SHA256Cipher:        utils.Sha256Hex(blob),
		ClassificationLevel: classLevel,
		ClassificationTags:  classTags,
		UploadedBy:          user.ID,
	}
	if _, err := h.store.AddIncidentAttachment(r.Context(), att); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	path := h.svc.AttachmentPath(incident.ID, att.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		_ = h.store.SoftDeleteIncidentAttachment(r.Context(), att.ID)
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		_ = h.store.SoftDeleteIncidentAttachment(r.Context(), att.ID)
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "incident.attachment.upload", fmt.Sprintf("%s|%d", incident.RegNo, att.ID))
	h.addTimeline(r.Context(), incident.ID, "attachment.upload", att.Filename, user.ID)
	writeJSON(w, http.StatusCreated, att)
}

func (h *IncidentsHandler) DownloadAttachment(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "view")
	if !ok {
		return
	}
	attID, _ := strconv.ParseInt(pathParams(r)["att_id"], 10, 64)
	if attID == 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	att, err := h.store.GetIncidentAttachment(r.Context(), incident.ID, attID)
	if err != nil || att == nil || att.DeletedAt != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !h.canViewByClassification(eff, att.ClassificationLevel, att.ClassificationTags) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	blob, err := os.ReadFile(h.svc.AttachmentPath(incident.ID, att.ID))
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if utils.Sha256Hex(blob) != att.SHA256Cipher {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	plain, err := h.svc.Encryptor().DecryptBlob(blob)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if utils.Sha256Hex(plain) != att.SHA256Plain {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "incident.attachment.download", fmt.Sprintf("%s|%d", incident.RegNo, att.ID))
	h.addTimeline(r.Context(), incident.ID, "attachment.download", att.Filename, user.ID)
	w.Header().Set("Content-Disposition", attachmentDisposition(safeFileName(att.Filename)))
	w.Header().Set("Content-Type", att.ContentType)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(plain)
}

func (h *IncidentsHandler) DeleteAttachment(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "edit")
	if !ok {
		return
	}
	attID, _ := strconv.ParseInt(pathParams(r)["att_id"], 10, 64)
	if attID == 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	att, err := h.store.GetIncidentAttachment(r.Context(), incident.ID, attID)
	if err != nil || att == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err := h.store.SoftDeleteIncidentAttachment(r.Context(), attID); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "incident.attachment.delete", fmt.Sprintf("%s|%d", incident.RegNo, attID))
	h.addTimeline(r.Context(), incident.ID, "attachment.delete", att.Filename, user.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *IncidentsHandler) ListArtifactFiles(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "view")
	if !ok {
		return
	}
	artifactID := strings.TrimSpace(pathParams(r)["artifact_id"])
	if artifactID == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	files, err := h.store.ListIncidentArtifactFiles(r.Context(), incident.ID, artifactID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	userMap := map[int64]*store.User{}
	resolveUser := func(id int64) *store.User {
		if id == 0 {
			return nil
		}
		if u, ok := userMap[id]; ok {
			return u
		}
		u, _, _ := h.users.Get(r.Context(), id)
		userMap[id] = u
		return u
	}
	type artifactFileDTO struct {
		store.IncidentArtifactFile
		UploadedByName string `json:"uploaded_by_name,omitempty"`
	}
	var out []artifactFileDTO
	for _, f := range files {
		if !h.canViewByClassification(eff, f.ClassificationLevel, f.ClassificationTags) {
			continue
		}
		out = append(out, artifactFileDTO{
			IncidentArtifactFile: f,
			UploadedByName:       displayName(resolveUser(f.UploadedBy)),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *IncidentsHandler) UploadArtifactFile(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "edit")
	if !ok {
		return
	}
	artifactID := strings.TrimSpace(pathParams(r)["artifact_id"])
	if artifactID == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
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
	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	classLevel := incident.ClassificationLevel
	classTags := incident.ClassificationTags
	if raw := strings.TrimSpace(r.FormValue("classification_level")); raw != "" {
		parsed, err := parseIncidentClassificationLevel(raw)
		if err != nil {
			http.Error(w, "invalid level", http.StatusBadRequest)
			return
		}
		classLevel = parsed
	}
	if classLevel < incident.ClassificationLevel {
		classLevel = incident.ClassificationLevel
	}
	if tags := r.MultipartForm.Value["classification_tags"]; len(tags) > 0 {
		classTags = docs.NormalizeTags(tags)
	}
	if !h.canViewByClassification(eff, classLevel, classTags) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	enc := h.svc.Encryptor()
	if enc == nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	blob, err := enc.EncryptToBlob(data)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	record := &store.IncidentArtifactFile{
		IncidentID:          incident.ID,
		ArtifactID:          artifactID,
		Filename:            header.Filename,
		ContentType:         header.Header.Get("Content-Type"),
		SizeBytes:           int64(len(data)),
		SHA256Plain:         utils.Sha256Hex(data),
		SHA256Cipher:        utils.Sha256Hex(blob),
		ClassificationLevel: classLevel,
		ClassificationTags:  classTags,
		UploadedBy:          user.ID,
	}
	if _, err := h.store.AddIncidentArtifactFile(r.Context(), record); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	path := h.svc.ArtifactFilePath(incident.ID, artifactID, record.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		_ = h.store.SoftDeleteIncidentArtifactFile(r.Context(), record.ID)
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		_ = h.store.SoftDeleteIncidentArtifactFile(r.Context(), record.ID)
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "incident.artifact.upload", fmt.Sprintf("%s|%s|%d", incident.RegNo, artifactID, record.ID))
	h.addTimeline(r.Context(), incident.ID, "artifact.file.upload", fmt.Sprintf("%s:%s", artifactID, record.Filename), user.ID)
	writeJSON(w, http.StatusCreated, record)
}

func (h *IncidentsHandler) DownloadArtifactFile(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "view")
	if !ok {
		return
	}
	artifactID := strings.TrimSpace(pathParams(r)["artifact_id"])
	fileID, _ := strconv.ParseInt(pathParams(r)["file_id"], 10, 64)
	if artifactID == "" || fileID == 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	fileRec, err := h.store.GetIncidentArtifactFile(r.Context(), incident.ID, fileID)
	if err != nil || fileRec == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !h.canViewByClassification(eff, fileRec.ClassificationLevel, fileRec.ClassificationTags) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if strings.TrimSpace(fileRec.ArtifactID) != artifactID {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	path := h.svc.ArtifactFilePath(incident.ID, artifactID, fileID)
	data, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	enc := h.svc.Encryptor()
	if enc == nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	plain, err := enc.DecryptBlob(data)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "incident.artifact.download", fmt.Sprintf("%s|%s|%d", incident.RegNo, artifactID, fileID))
	h.addTimeline(r.Context(), incident.ID, "artifact.file.download", fmt.Sprintf("%s:%s", artifactID, fileRec.Filename), user.ID)
	w.Header().Set("Content-Disposition", attachmentDisposition(fileRec.Filename))
	w.Header().Set("Content-Type", fileRec.ContentType)
	_, _ = w.Write(plain)
}

func (h *IncidentsHandler) DeleteArtifactFile(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "edit")
	if !ok {
		return
	}
	artifactID := strings.TrimSpace(pathParams(r)["artifact_id"])
	fileID, _ := strconv.ParseInt(pathParams(r)["file_id"], 10, 64)
	if artifactID == "" || fileID == 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	fileRec, err := h.store.GetIncidentArtifactFile(r.Context(), incident.ID, fileID)
	if err != nil || fileRec == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if strings.TrimSpace(fileRec.ArtifactID) != artifactID {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err := h.store.SoftDeleteIncidentArtifactFile(r.Context(), fileID); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "incident.artifact.delete", fmt.Sprintf("%s|%s|%d", incident.RegNo, artifactID, fileID))
	h.addTimeline(r.Context(), incident.ID, "artifact.file.delete", fmt.Sprintf("%s:%s", artifactID, fileRec.Filename), user.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *IncidentsHandler) ListTimeline(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "view")
	if !ok {
		return
	}
	eventType := strings.TrimSpace(r.URL.Query().Get("event_type"))
	items, err := h.store.ListIncidentTimeline(r.Context(), incident.ID, 200, eventType)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *IncidentsHandler) AddTimeline(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "edit")
	if !ok {
		return
	}
	var payload struct {
		Message   string `json:"message"`
		EventType string `json:"event_type"`
		EventAt   string `json:"event_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	msg := strings.TrimSpace(payload.Message)
	if msg == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	eventType := strings.TrimSpace(payload.EventType)
	if eventType == "" {
		eventType = "note"
	}
	var eventAt time.Time
	if strings.TrimSpace(payload.EventAt) != "" {
		eventAt, err = parseISOTime(payload.EventAt)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
	}
	if _, err := h.store.AddIncidentTimeline(r.Context(), &store.IncidentTimelineEvent{
		IncidentID: incident.ID,
		EventType:  eventType,
		Message:    msg,
		CreatedBy:  user.ID,
		EventAt:    eventAt,
	}); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	h.svc.Log(r.Context(), user.Username, "incident.note.add", incident.RegNo)
	writeJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
}

func (h *IncidentsHandler) ListActivity(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "view")
	if !ok {
		return
	}
	limit := parseIntDefault(r.URL.Query().Get("limit"), 200)
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	events, err := h.store.ListIncidentTimeline(r.Context(), incident.ID, limit, "")
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	userCache := map[int64]*store.User{}
	resolveUser := func(id int64) *store.User {
		if id == 0 {
			return nil
		}
		if u, ok := userCache[id]; ok {
			return u
		}
		u, _, _ := h.users.Get(r.Context(), id)
		userCache[id] = u
		return u
	}
	type activityItem struct {
		store.IncidentTimelineEvent
		CreatedByName string `json:"created_by_name,omitempty"`
	}
	var items []activityItem
	for _, ev := range events {
		if ev.EventAt.IsZero() {
			ev.EventAt = ev.CreatedAt
		}
		items = append(items, activityItem{
			IncidentTimelineEvent: ev,
			CreatedByName:         displayName(resolveUser(ev.CreatedBy)),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *IncidentsHandler) Export(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "view")
	if !ok {
		return
	}
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = docs.FormatMarkdown
	}
	if format != docs.FormatMarkdown && format != docs.FormatPDF && format != docs.FormatDocx {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	md, err := h.buildIncidentReportMarkdown(r.Context(), incident, eff)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	wm := ""
	if h.needsIncidentWatermark(incident) {
		wm = h.incidentWatermarkString(incident, user.Username)
	}
	data, contentType, err := h.docsSvc.ConvertMarkdown(r.Context(), format, md, wm)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	filename := fmt.Sprintf("%s.%s", safeFileName(incident.RegNo), format)
	h.svc.Log(r.Context(), user.Username, "incident.export", fmt.Sprintf("%s|%s", incident.RegNo, format))
	h.addTimeline(r.Context(), incident.ID, "incident.export", fmt.Sprintf("export %s", format), user.ID)
	w.Header().Set("Content-Disposition", attachmentDisposition(filename))
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *IncidentsHandler) CreateReportDoc(w http.ResponseWriter, r *http.Request) {
	user, roles, eff, err := h.currentUser(r)
	if err != nil || user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	incident, ok := h.getIncidentWithACL(w, r, user, roles, eff, "edit")
	if !ok {
		return
	}
	if !h.policy.Allowed(roles, "docs.create") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var payload struct {
		FolderID *int64 `json:"folder_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&payload)
	if !h.canViewByClassification(eff, incident.ClassificationLevel, incident.ClassificationTags) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var folderACL []store.ACLRule
	if payload.FolderID != nil {
		folderACL, _ = h.docsStore.GetFolderACL(r.Context(), *payload.FolderID)
		docProbe := &store.Document{
			FolderID:              payload.FolderID,
			Title:                 "",
			Status:                docs.StatusDraft,
			ClassificationLevel:   incident.ClassificationLevel,
			ClassificationTags:    incident.ClassificationTags,
			InheritACL:            true,
			InheritClassification: true,
			CreatedBy:             user.ID,
		}
		if !h.docsSvc.CheckACL(user, roles, docProbe, nil, folderACL, "edit") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	}
	title := fmt.Sprintf("Отчёт по инциденту %s", incident.RegNo)
	doc := &store.Document{
		FolderID:              payload.FolderID,
		Title:                 title,
		Status:                docs.StatusDraft,
		ClassificationLevel:   incident.ClassificationLevel,
		ClassificationTags:    docs.NormalizeTags(incident.ClassificationTags),
		InheritACL:            true,
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
	if _, err := h.docsStore.CreateDocument(r.Context(), doc, acl, h.cfg.Docs.RegTemplate, h.cfg.Docs.PerFolderSequence); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	md, err := h.buildIncidentReportMarkdown(r.Context(), incident, eff)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if _, err := h.docsSvc.SaveVersion(r.Context(), docs.SaveRequest{
		Doc:      doc,
		Author:   user,
		Format:   docs.FormatMarkdown,
		Content:  md,
		Reason:   "incident report",
		IndexFTS: true,
	}); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	link := &store.IncidentLink{
		IncidentID: incident.ID,
		EntityType: "doc",
		EntityID:   fmt.Sprintf("%d", doc.ID),
		Title:      doc.Title,
		Unverified: false,
		CreatedBy:  user.ID,
	}
	_, _ = h.store.AddIncidentLink(r.Context(), link)
	h.svc.Log(r.Context(), user.Username, "incident.report.create", fmt.Sprintf("%s|%d", incident.RegNo, doc.ID))
	h.addTimeline(r.Context(), incident.ID, "report.doc.create", fmt.Sprintf("report doc %d", doc.ID), user.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"doc_id": doc.ID})
}

func (h *IncidentsHandler) currentUser(r *http.Request) (*store.User, []string, store.EffectiveAccess, error) {
	val := r.Context().Value(auth.SessionContextKey)
	if val == nil {
		return nil, nil, store.EffectiveAccess{}, errors.New("no session")
	}
	sess := val.(*store.SessionRecord)
	u, roles, err := h.users.FindByUsername(r.Context(), sess.Username)
	if err != nil || u == nil {
		return u, roles, store.EffectiveAccess{}, err
	}
	groups, _ := h.users.UserGroups(r.Context(), u.ID)
	eff := auth.CalculateEffectiveAccess(u, roles, groups, h.policy)
	return u, eff.Roles, eff, err
}

func (h *IncidentsHandler) getIncidentWithACL(w http.ResponseWriter, r *http.Request, user *store.User, roles []string, eff store.EffectiveAccess, required string) (*store.Incident, bool) {
	id, _ := strconv.ParseInt(pathParams(r)["id"], 10, 64)
	incident, err := h.store.GetIncident(r.Context(), id)
	if err != nil || incident == nil {
		http.Error(w, "incidents.notFound", http.StatusNotFound)
		return nil, false
	}
	canManage := h.policy.Allowed(roles, "incidents.manage")
	acl, _ := h.store.GetIncidentACL(r.Context(), incident.ID)
	if incident.DeletedAt != nil {
		if !canManage || !h.svc.CheckACL(user, roles, acl, "manage") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return nil, false
		}
	} else if !canManage && !h.svc.CheckACL(user, roles, acl, required) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil, false
	}
	if !h.canViewByClassification(eff, incident.ClassificationLevel, incident.ClassificationTags) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil, false
	}
	if strings.ToLower(required) != "view" && strings.ToLower(incident.Status) == "closed" {
		http.Error(w, "incidents.closedReadOnly", http.StatusConflict)
		return nil, false
	}
	return incident, true
}

func (h *IncidentsHandler) lookupUserByToken(ctx context.Context, token string) (*store.User, error) {
	t := strings.TrimSpace(token)
	if t == "" {
		return nil, errors.New("empty token")
	}
	if id, err := strconv.ParseInt(t, 10, 64); err == nil {
		return h.lookupUserByID(ctx, id)
	}
	u, _, err := h.users.FindByUsername(ctx, strings.ToLower(t))
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, errors.New("not found")
	}
	return u, nil
}

func (h *IncidentsHandler) lookupUserByID(ctx context.Context, id int64) (*store.User, error) {
	u, _, err := h.users.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, errors.New("not found")
	}
	return u, nil
}

func (h *IncidentsHandler) resolveParticipants(ctx context.Context, items []string) ([]store.IncidentParticipant, error) {
	seen := map[int64]struct{}{}
	var out []store.IncidentParticipant
	for _, raw := range items {
		token := strings.TrimSpace(raw)
		if token == "" {
			continue
		}
		u, err := h.lookupUserByToken(ctx, token)
		if err != nil || u == nil {
			return nil, errors.New("participant not found")
		}
		if _, ok := seen[u.ID]; ok {
			continue
		}
		seen[u.ID] = struct{}{}
		out = append(out, store.IncidentParticipant{UserID: u.ID, Role: "member", Username: u.Username, FullName: u.FullName})
	}
	return out, nil
}

func (h *IncidentsHandler) ensureAssigneeACL(ctx context.Context, incidentID int64, assigneeID *int64) {
	if assigneeID == nil {
		return
	}
	u, _, err := h.users.Get(ctx, *assigneeID)
	if err != nil || u == nil {
		return
	}
	acl, err := h.store.GetIncidentACL(ctx, incidentID)
	if err != nil {
		return
	}
	for _, rule := range acl {
		if strings.ToLower(rule.SubjectType) == "user" && rule.SubjectID == u.Username && rule.Permission == "edit" {
			return
		}
	}
	acl = append(acl, store.ACLRule{SubjectType: "user", SubjectID: u.Username, Permission: "edit"})
	_ = h.store.SetIncidentACL(ctx, incidentID, acl)
}

func (h *IncidentsHandler) ensureParticipantACL(ctx context.Context, incidentID int64, participants []store.IncidentParticipant) {
	if len(participants) == 0 {
		return
	}
	acl, err := h.store.GetIncidentACL(ctx, incidentID)
	if err != nil {
		return
	}
	seen := map[string]struct{}{}
	for _, rule := range acl {
		if strings.ToLower(rule.SubjectType) == "user" && rule.Permission == "view" {
			seen[rule.SubjectID] = struct{}{}
		}
	}
	changed := false
	for _, p := range participants {
		if p.Username == "" {
			continue
		}
		if _, ok := seen[p.Username]; ok {
			continue
		}
		acl = append(acl, store.ACLRule{SubjectType: "user", SubjectID: p.Username, Permission: "view"})
		seen[p.Username] = struct{}{}
		changed = true
	}
	if changed {
		_ = h.store.SetIncidentACL(ctx, incidentID, acl)
	}
}

func (h *IncidentsHandler) populateParticipantNames(ctx context.Context, parts []store.IncidentParticipant) {
	for i := range parts {
		u, _, err := h.users.Get(ctx, parts[i].UserID)
		if err != nil || u == nil {
			continue
		}
		parts[i].Username = u.Username
		parts[i].FullName = u.FullName
	}
}

func (h *IncidentsHandler) canViewByClassification(eff store.EffectiveAccess, level int, tags []string) bool {
	if h.cfg != nil && h.cfg.Security.TagsSubsetEnforced {
		return docs.HasClearance(docs.ClassificationLevel(eff.ClearanceLevel), eff.ClearanceTags, docs.ClassificationLevel(level), tags)
	}
	return eff.ClearanceLevel >= level
}

func (h *IncidentsHandler) canEditClassification(user *store.User, roles []string) bool {
	if h.policy.Allowed(roles, "incidents.manage") {
		return true
	}
	for _, r := range roles {
		if r == "security_officer" {
			return true
		}
	}
	return false
}

func (h *IncidentsHandler) needsIncidentWatermark(incident *store.Incident) bool {
	if incident == nil {
		return false
	}
	if incident.Severity == "high" || incident.Severity == "critical" {
		return true
	}
	return incident.ClassificationLevel >= int(docs.ClassificationConfidential)
}

func (h *IncidentsHandler) incidentWatermarkString(incident *store.Incident, username string) string {
	if incident == nil {
		return ""
	}
	tmpl := h.cfg.Docs.Watermark.TextTemplate
	if tmpl == "" {
		tmpl = "Berkut SCC ¢?÷ {classification} ¢?÷ {username} ¢?÷ {timestamp} ¢?÷ DocNo {reg_no}"
	}
	replacements := map[string]string{
		"{classification}": docs.LevelName(docs.ClassificationLevel(incident.ClassificationLevel)),
		"{username}":       username,
		"{timestamp}":      time.Now().UTC().Format(time.RFC3339),
		"{reg_no}":         incident.RegNo,
	}
	out := tmpl
	for k, v := range replacements {
		out = strings.ReplaceAll(out, k, v)
	}
	return out
}

func (h *IncidentsHandler) addTimeline(ctx context.Context, incidentID int64, eventType, message string, userID int64, eventAt ...time.Time) {
	if strings.TrimSpace(eventType) == "" || incidentID == 0 {
		return
	}
	ev := &store.IncidentTimelineEvent{
		IncidentID: incidentID,
		EventType:  eventType,
		Message:    message,
		CreatedBy:  userID,
	}
	if len(eventAt) > 0 && !eventAt[0].IsZero() {
		ev.EventAt = eventAt[0].UTC()
	}
	_, _ = h.store.AddIncidentTimeline(ctx, ev)
}

func parseISOTime(val string) (time.Time, error) {
	clean := strings.TrimSpace(val)
	if clean == "" {
		return time.Time{}, errors.New("empty time")
	}
	layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"}
	var lastErr error
	for _, layout := range layouts {
		if ts, err := time.Parse(layout, clean); err == nil {
			return ts.UTC(), nil
		} else {
			lastErr = err
		}
	}
	return time.Time{}, lastErr
}

func documentTypeFromTags(tags []string) string {
	for _, t := range docs.NormalizeTags(tags) {
		if strings.ToUpper(strings.TrimSpace(t)) == "REPORT" {
			return "report"
		}
	}
	return "document"
}

func parseIncidentClassificationLevel(val string) (int, error) {
	raw := strings.TrimSpace(val)
	if raw == "" {
		return 0, errors.New("empty")
	}
	if n, err := strconv.Atoi(raw); err == nil {
		return n, nil
	}
	lvl, err := docs.ParseLevel(raw)
	if err != nil {
		return 0, err
	}
	return int(lvl), nil
}

func sameAssignee(a, b *int64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func sameTags(a, b []string) bool {
	na := docs.NormalizeTags(a)
	nb := docs.NormalizeTags(b)
	if len(na) != len(nb) {
		return false
	}
	sort.Strings(na)
	sort.Strings(nb)
	for i := range na {
		if na[i] != nb[i] {
			return false
		}
	}
	return true
}

func sliceLimit[T any](items []T, limit int) []T {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}

func safeFileName(name string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_")
	return replacer.Replace(name)
}

func (h *IncidentsHandler) buildIncidentReportMarkdown(ctx context.Context, incident *store.Incident, eff store.EffectiveAccess) ([]byte, error) {
	if incident == nil {
		return nil, errors.New("missing incident")
	}
	owner, _, _ := h.users.Get(ctx, incident.OwnerUserID)
	var assignee *store.User
	if incident.AssigneeUserID != nil {
		assignee, _, _ = h.users.Get(ctx, *incident.AssigneeUserID)
	}
	stages, _ := h.store.ListIncidentStages(ctx, incident.ID)
	links, _ := h.store.ListIncidentLinks(ctx, incident.ID)
	atts, _ := h.store.ListIncidentAttachments(ctx, incident.ID)
	var visibleAtts []store.IncidentAttachment
	for _, att := range atts {
		if h.canViewByClassification(eff, att.ClassificationLevel, att.ClassificationTags) {
			visibleAtts = append(visibleAtts, att)
		}
	}
	tlLimit := h.cfg.Incidents.TimelineExportLimit
	if tlLimit <= 0 {
		tlLimit = 50
	}
	timeline, _ := h.store.ListIncidentTimeline(ctx, incident.ID, tlLimit, "")
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Incident Report %s\n\n", incident.RegNo))
	b.WriteString("## Metadata\n")
	b.WriteString(fmt.Sprintf("- Status: %s\n", incident.Status))
	b.WriteString(fmt.Sprintf("- Severity: %s\n", incident.Severity))
	b.WriteString(fmt.Sprintf("- Owner: %s\n", displayName(owner)))
	b.WriteString(fmt.Sprintf("- Assignee: %s\n", displayName(assignee)))
	b.WriteString(fmt.Sprintf("- Created: %s\n", incident.CreatedAt.UTC().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- Updated: %s\n", incident.UpdatedAt.UTC().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- Classification: %s\n", docs.LevelName(docs.ClassificationLevel(incident.ClassificationLevel))))
	if len(incident.ClassificationTags) > 0 {
		b.WriteString(fmt.Sprintf("- Tags: %s\n", strings.Join(docs.NormalizeTags(incident.ClassificationTags), ", ")))
	}
	if strings.TrimSpace(incident.Description) != "" {
		b.WriteString("\n## Summary\n")
		b.WriteString(incident.Description)
		b.WriteString("\n")
	}
	b.WriteString("\n## Stages\n")
	if len(stages) == 0 {
		b.WriteString("_No stages._\n")
	} else {
		for _, st := range stages {
			entry, _ := h.store.GetStageEntry(ctx, st.ID)
			title := st.Title
			if st.IsDefault {
				title = "Overview"
			}
			b.WriteString(fmt.Sprintf("### %s\n", title))
			if entry != nil && strings.TrimSpace(entry.Content) != "" {
				b.WriteString(entry.Content)
				b.WriteString("\n\n")
			} else {
				b.WriteString("_No content._\n\n")
			}
		}
	}
	b.WriteString("## Links\n")
	if len(links) == 0 {
		b.WriteString("_No links._\n")
	} else {
		for _, l := range links {
			label := l.Title
			if strings.TrimSpace(label) == "" {
				label = l.EntityID
			}
			status := ""
			if l.Unverified {
				status = " (unverified)"
			}
			b.WriteString(fmt.Sprintf("- %s:%s%s\n", l.EntityType, label, status))
		}
	}
	b.WriteString("\n## Attachments\n")
	if len(visibleAtts) == 0 {
		b.WriteString("_No attachments._\n")
	} else {
		for _, a := range visibleAtts {
			b.WriteString(fmt.Sprintf("- %s (%d bytes)\n", a.Filename, a.SizeBytes))
		}
	}
	b.WriteString("\n## Timeline\n")
	if len(timeline) == 0 {
		b.WriteString("_No events._\n")
	} else {
		for _, ev := range timeline {
			ts := ev.CreatedAt
			if !ev.EventAt.IsZero() {
				ts = ev.EventAt
			}
			b.WriteString(fmt.Sprintf("- %s [%s] %s\n", ts.UTC().Format(time.RFC3339), ev.EventType, ev.Message))
		}
	}
	return []byte(b.String()), nil
}

func displayName(u *store.User) string {
	if u == nil {
		return ""
	}
	if strings.TrimSpace(u.FullName) != "" {
		return u.FullName
	}
	return u.Username
}

type incidentDTO struct {
	store.Incident
	OwnerName    string          `json:"owner_name"`
	AssigneeName string          `json:"assignee_name,omitempty"`
	CaseSLA      incidentCaseSLA `json:"case_sla"`
}

type incidentCaseSLA struct {
	FirstResponseDueAt string `json:"first_response_due_at,omitempty"`
	FirstResponseLate  bool   `json:"first_response_late"`
	ResolveDueAt       string `json:"resolve_due_at,omitempty"`
	ResolveLate        bool   `json:"resolve_late"`
}

type stageContentBlock struct {
	Type  string            `json:"type"`
	Items []json.RawMessage `json:"items"`
}

type stageContentPayload struct {
	StageType string              `json:"stageType"`
	Type      string              `json:"type"`
	Blocks    []stageContentBlock `json:"blocks"`
}

type attachmentDTO struct {
	store.IncidentAttachment
	UploadedByName string `json:"uploaded_by_name,omitempty"`
}

func isValidSeverity(val string) bool {
	_, ok := validIncidentSeverity[strings.ToLower(val)]
	return ok
}

func isValidStatus(val string) bool {
	_, ok := validIncidentStatus[strings.ToLower(val)]
	return ok
}

func buildIncidentCaseSLA(inc store.Incident) incidentCaseSLA {
	now := time.Now().UTC()
	result := incidentCaseSLA{}
	parse := func(raw string) *time.Time {
		val := strings.TrimSpace(raw)
		if val == "" {
			return nil
		}
		for _, layout := range []string{time.RFC3339, "2006-01-02 15:04", "2006-01-02T15:04"} {
			if parsed, err := time.Parse(layout, val); err == nil {
				utc := parsed.UTC()
				return &utc
			}
		}
		return nil
	}
	if due := parse(inc.Meta.FirstResponseDeadline); due != nil {
		result.FirstResponseDueAt = due.Format(time.RFC3339)
		if strings.ToLower(strings.TrimSpace(inc.Status)) != "closed" {
			result.FirstResponseLate = now.After(*due)
		}
	}
	if due := parse(inc.Meta.ResolveDeadline); due != nil {
		result.ResolveDueAt = due.Format(time.RFC3339)
		if strings.ToLower(strings.TrimSpace(inc.Status)) != "closed" {
			result.ResolveLate = now.After(*due)
		}
	}
	return result
}

func parseStageContentPayload(entry *store.IncidentStageEntry) stageContentPayload {
	var payload stageContentPayload
	if entry == nil || strings.TrimSpace(entry.Content) == "" {
		return payload
	}
	if err := json.Unmarshal([]byte(entry.Content), &payload); err != nil {
		return payload
	}
	if payload.StageType == "" {
		payload.StageType = payload.Type
	}
	payload.StageType = strings.ToLower(strings.TrimSpace(payload.StageType))
	if len(payload.Blocks) > 0 {
		blocks := make([]stageContentBlock, 0, len(payload.Blocks))
		for _, b := range payload.Blocks {
			blocks = append(blocks, stageContentBlock{
				Type:  strings.ToLower(strings.TrimSpace(b.Type)),
				Items: b.Items,
			})
		}
		payload.Blocks = blocks
	}
	return payload
}

func hasDecisionEntry(payload stageContentPayload) bool {
	for _, b := range payload.Blocks {
		if strings.ToLower(b.Type) != "decisions" {
			continue
		}
		if len(b.Items) > 0 {
			return true
		}
	}
	return false
}

type decisionOutcomeItem struct {
	Outcome  string `json:"outcome"`
	Title    string `json:"title"`
	Decision string `json:"decision"`
	Selected string `json:"selected"`
}

func normalizeDecisionOutcome(raw string) string {
	val := strings.ToLower(strings.TrimSpace(raw))
	if val == "" {
		return ""
	}
	if _, ok := validDecisionOutcomes[val]; ok {
		return val
	}
	switch val {
	case "разрешено":
		return "approved"
	case "отклонено":
		return "rejected"
	case "блокировка", "заблокировано":
		return "blocked"
	case "отложено":
		return "deferred"
	case "под наблюдением", "monitoring":
		return "monitor"
	default:
		return ""
	}
}

func extractDecisionOutcome(payload stageContentPayload) string {
	outcome := ""
	for _, b := range payload.Blocks {
		if strings.ToLower(b.Type) != "decisions" {
			continue
		}
		for _, raw := range b.Items {
			var item decisionOutcomeItem
			if err := json.Unmarshal(raw, &item); err != nil {
				continue
			}
			candidate := normalizeDecisionOutcome(item.Outcome)
			if candidate == "" {
				candidate = normalizeDecisionOutcome(item.Title)
			}
			if candidate == "" {
				candidate = normalizeDecisionOutcome(item.Decision)
			}
			if candidate == "" {
				candidate = normalizeDecisionOutcome(item.Selected)
			}
			if candidate == "" {
				continue
			}
			outcome = candidate
		}
	}
	return outcome
}

func isValidIncidentPerm(val string) bool {
	switch strings.ToLower(val) {
	case "view", "edit", "manage":
		return true
	default:
		return false
	}
}
