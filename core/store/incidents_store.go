package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var ErrConflict = errors.New("conflict")

type Incident struct {
	ID                  int64        `json:"id"`
	RegNo               string       `json:"reg_no"`
	Title               string       `json:"title"`
	Description         string       `json:"description"`
	Severity            string       `json:"severity"`
	Status              string       `json:"status"`
	Source              string       `json:"source,omitempty"`
	SourceRefID         *int64       `json:"source_ref_id,omitempty"`
	ClosedAt            *time.Time   `json:"closed_at,omitempty"`
	ClosedBy            *int64       `json:"closed_by,omitempty"`
	OwnerUserID         int64        `json:"owner_user_id"`
	AssigneeUserID      *int64       `json:"assignee_user_id,omitempty"`
	ClassificationLevel int          `json:"classification_level"`
	ClassificationTags  []string     `json:"classification_tags,omitempty"`
	CreatedBy           int64        `json:"created_by"`
	UpdatedBy           int64        `json:"updated_by"`
	CreatedAt           time.Time    `json:"created_at"`
	UpdatedAt           time.Time    `json:"updated_at"`
	Version             int          `json:"version"`
	DeletedAt           *time.Time   `json:"deleted_at,omitempty"`
	Meta                IncidentMeta `json:"meta,omitempty"`
}

type IncidentParticipant struct {
	IncidentID int64  `json:"incident_id"`
	UserID     int64  `json:"user_id"`
	Role       string `json:"role"`
	Username   string `json:"username,omitempty"`
	FullName   string `json:"full_name,omitempty"`
}

type IncidentMeta struct {
	IncidentType          string   `json:"incident_type,omitempty"`
	DetectionSource       string   `json:"detection_source,omitempty"`
	SLAResponse           string   `json:"sla_response,omitempty"`
	FirstResponseDeadline string   `json:"first_response_deadline,omitempty"`
	ResolveDeadline       string   `json:"resolve_deadline,omitempty"`
	WhatHappened          string   `json:"what_happened,omitempty"`
	DetectedAt            string   `json:"detected_at,omitempty"`
	AffectedSystems       string   `json:"affected_systems,omitempty"`
	Risk                  string   `json:"risk,omitempty"`
	ActionsTaken          string   `json:"actions_taken,omitempty"`
	Assets                string   `json:"assets,omitempty"`
	Tags                  []string `json:"tags,omitempty"`
	ClosureOutcome        string   `json:"closure_outcome,omitempty"`
	Postmortem            string   `json:"postmortem,omitempty"`
}

type IncidentStage struct {
	ID         int64      `json:"id"`
	IncidentID int64      `json:"incident_id"`
	Title      string     `json:"title"`
	Position   int        `json:"position"`
	CreatedBy  int64      `json:"created_by"`
	UpdatedBy  int64      `json:"updated_by"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	Status     string     `json:"status"`
	ClosedAt   *time.Time `json:"closed_at,omitempty"`
	ClosedBy   *int64     `json:"closed_by,omitempty"`
	IsDefault  bool       `json:"is_default"`
	Version    int        `json:"version"`
}

type IncidentStageEntry struct {
	ID           int64     `json:"id"`
	StageID      int64     `json:"stage_id"`
	Content      string    `json:"content"`
	ChangeReason string    `json:"change_reason"`
	CreatedBy    int64     `json:"created_by"`
	UpdatedBy    int64     `json:"updated_by"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Version      int       `json:"version"`
}

type IncidentLink struct {
	ID         int64     `json:"id"`
	IncidentID int64     `json:"incident_id"`
	EntityType string    `json:"entity_type"`
	EntityID   string    `json:"entity_id"`
	Title      string    `json:"title,omitempty"`
	Comment    string    `json:"comment,omitempty"`
	Unverified bool      `json:"unverified"`
	CreatedBy  int64     `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
}

type IncidentAttachment struct {
	ID                  int64      `json:"id"`
	IncidentID          int64      `json:"incident_id"`
	Filename            string     `json:"filename"`
	ContentType         string     `json:"content_type"`
	SizeBytes           int64      `json:"size_bytes"`
	SHA256Plain         string     `json:"sha256_plain"`
	SHA256Cipher        string     `json:"sha256_cipher"`
	ClassificationLevel int        `json:"classification_level"`
	ClassificationTags  []string   `json:"classification_tags,omitempty"`
	UploadedBy          int64      `json:"uploaded_by"`
	UploadedAt          time.Time  `json:"uploaded_at"`
	DeletedAt           *time.Time `json:"deleted_at,omitempty"`
}

type IncidentArtifactFile struct {
	ID                  int64      `json:"id"`
	IncidentID          int64      `json:"incident_id"`
	ArtifactID          string     `json:"artifact_id"`
	Filename            string     `json:"filename"`
	ContentType         string     `json:"content_type"`
	SizeBytes           int64      `json:"size_bytes"`
	SHA256Plain         string     `json:"sha256_plain"`
	SHA256Cipher        string     `json:"sha256_cipher"`
	ClassificationLevel int        `json:"classification_level"`
	ClassificationTags  []string   `json:"classification_tags,omitempty"`
	UploadedBy          int64      `json:"uploaded_by"`
	UploadedAt          time.Time  `json:"uploaded_at"`
	DeletedAt           *time.Time `json:"deleted_at,omitempty"`
}

type IncidentTimelineEvent struct {
	ID         int64     `json:"id"`
	IncidentID int64     `json:"incident_id"`
	EventType  string    `json:"event_type"`
	Message    string    `json:"message"`
	MetaJSON   string    `json:"meta_json"`
	CreatedBy  int64     `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
	EventAt    time.Time `json:"event_at"`
}

type IncidentFilter struct {
	Search          string
	Status          string
	StatusIn        []string
	Severity        string
	MineUserID      int64
	AssignedUserID  int64
	CreatedByUserID int64
	IncludeDeleted  bool
	Limit           int
	Offset          int
}

type IncidentsStore interface {
	CreateIncident(ctx context.Context, incident *Incident, participants []IncidentParticipant, acl []ACLRule, regFormat string) (int64, error)
	UpdateIncident(ctx context.Context, incident *Incident, expectedVersion int) error
	CloseIncident(ctx context.Context, incidentID int64, userID int64) (*Incident, error)
	SoftDeleteIncident(ctx context.Context, id int64, updatedBy int64) error
	RestoreIncident(ctx context.Context, id int64, updatedBy int64) error
	GetIncident(ctx context.Context, id int64) (*Incident, error)
	GetIncidentByRegNo(ctx context.Context, regNo string) (*Incident, error)
	ListIncidents(ctx context.Context, filter IncidentFilter) ([]Incident, error)

	SetIncidentACL(ctx context.Context, incidentID int64, acl []ACLRule) error
	GetIncidentACL(ctx context.Context, incidentID int64) ([]ACLRule, error)

	SetIncidentParticipants(ctx context.Context, incidentID int64, participants []IncidentParticipant) error
	ListIncidentParticipants(ctx context.Context, incidentID int64) ([]IncidentParticipant, error)

	CreateIncidentStage(ctx context.Context, stage *IncidentStage) (int64, error)
	UpdateIncidentStage(ctx context.Context, stage *IncidentStage, expectedVersion int) error
	CompleteIncidentStage(ctx context.Context, stageID int64, userID int64) (*IncidentStage, error)
	DeleteIncidentStage(ctx context.Context, stageID int64) error
	GetIncidentStage(ctx context.Context, stageID int64) (*IncidentStage, error)
	ListIncidentStages(ctx context.Context, incidentID int64) ([]IncidentStage, error)

	CreateStageEntry(ctx context.Context, entry *IncidentStageEntry) (int64, error)
	UpdateStageEntry(ctx context.Context, entry *IncidentStageEntry, expectedVersion int) error
	GetStageEntry(ctx context.Context, stageID int64) (*IncidentStageEntry, error)
	NextStagePosition(ctx context.Context, incidentID int64) (int, error)

	ListIncidentLinks(ctx context.Context, incidentID int64) ([]IncidentLink, error)
	AddIncidentLink(ctx context.Context, link *IncidentLink) (int64, error)
	DeleteIncidentLink(ctx context.Context, linkID int64) error

	ListIncidentAttachments(ctx context.Context, incidentID int64) ([]IncidentAttachment, error)
	AddIncidentAttachment(ctx context.Context, att *IncidentAttachment) (int64, error)
	GetIncidentAttachment(ctx context.Context, incidentID, attachmentID int64) (*IncidentAttachment, error)
	SoftDeleteIncidentAttachment(ctx context.Context, attachmentID int64) error
	ListIncidentArtifactFiles(ctx context.Context, incidentID int64, artifactID string) ([]IncidentArtifactFile, error)
	AddIncidentArtifactFile(ctx context.Context, file *IncidentArtifactFile) (int64, error)
	GetIncidentArtifactFile(ctx context.Context, incidentID, fileID int64) (*IncidentArtifactFile, error)
	SoftDeleteIncidentArtifactFile(ctx context.Context, fileID int64) error

	ListIncidentTimeline(ctx context.Context, incidentID int64, limit int, eventType string) ([]IncidentTimelineEvent, error)
	AddIncidentTimeline(ctx context.Context, ev *IncidentTimelineEvent) (int64, error)
	FindOpenIncidentBySource(ctx context.Context, source string, refID int64) (*Incident, error)
}

type incidentsStore struct {
	db *sql.DB
}

func NewIncidentsStore(db *sql.DB) IncidentsStore {
	return &incidentsStore{db: db}
}

func (s *incidentsStore) CreateIncident(ctx context.Context, incident *Incident, participants []IncidentParticipant, acl []ACLRule, regFormat string) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(incident.RegNo) == "" {
		seq, err := s.nextIncidentSeqTx(ctx, tx, time.Now().UTC().Year())
		if err != nil {
			tx.Rollback()
			return 0, err
		}
		incident.RegNo = buildIncidentRegNo(regFormat, time.Now().UTC().Year(), seq)
	}
	if incident.Version <= 0 {
		incident.Version = 1
	}
	if incident.ClassificationLevel == 0 {
		incident.ClassificationLevel = 1
	}
	incident.Meta = NormalizeIncidentMeta(incident.Meta)
	if strings.TrimSpace(incident.Status) == "" {
		incident.Status = "draft"
	}
	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx, `
		INSERT INTO incidents(reg_no, title, description, severity, status, source, source_ref_id, closed_at, closed_by, owner_user_id, assignee_user_id, classification_level, classification_tags, meta_json, created_by, updated_by, created_at, updated_at, version, deleted_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		incident.RegNo, incident.Title, incident.Description, incident.Severity, incident.Status, strings.TrimSpace(incident.Source), nullableID(incident.SourceRefID), nullableTime(incident.ClosedAt), nullableID(incident.ClosedBy), incident.OwnerUserID, nullableID(incident.AssigneeUserID), incident.ClassificationLevel, tagsToJSON(normalizeTags(incident.ClassificationTags)), metaToJSON(incident.Meta), incident.CreatedBy, incident.UpdatedBy, now, now, incident.Version, nil)
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	incidentID, _ := res.LastInsertId()
	incident.ID = incidentID
	acl = ensureIncidentACLDefaults(incident, participants, acl)
	if len(acl) > 0 {
		for _, a := range acl {
			if _, err := tx.ExecContext(ctx, `INSERT INTO incident_acl(incident_id, subject_type, subject_id, permission) VALUES(?,?,?,?)`,
				incidentID, strings.ToLower(a.SubjectType), a.SubjectID, strings.ToLower(a.Permission)); err != nil {
				tx.Rollback()
				return 0, err
			}
		}
	}
	if len(participants) > 0 {
		for _, p := range participants {
			if _, err := tx.ExecContext(ctx, `INSERT INTO incident_participants(incident_id, user_id, role) VALUES(?,?,?)`,
				incidentID, p.UserID, strings.ToLower(strings.TrimSpace(p.Role))); err != nil {
				tx.Rollback()
				return 0, err
			}
		}
	}
	stageRes, err := tx.ExecContext(ctx, `
		INSERT INTO incident_stages(incident_id, title, position, created_by, updated_by, created_at, updated_at, status, closed_at, closed_by, is_default, version)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		incidentID, "Overview", 1, incident.CreatedBy, incident.UpdatedBy, now, now, "open", nil, nil, 1, 1)
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	stageID, _ := stageRes.LastInsertId()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO incident_stage_entries(stage_id, content, change_reason, created_by, updated_by, created_at, updated_at, version)
		VALUES(?,?,?,?,?,?,?,?)`, stageID, "", "", incident.CreatedBy, incident.UpdatedBy, now, now, 1); err != nil {
		tx.Rollback()
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return incidentID, nil
}

func (s *incidentsStore) UpdateIncident(ctx context.Context, incident *Incident, expectedVersion int) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE incidents SET title=?, description=?, severity=?, status=?, owner_user_id=?, assignee_user_id=?, classification_level=?, classification_tags=?, meta_json=?, updated_by=?, updated_at=?, version=version+1
		WHERE id=? AND version=?`,
		incident.Title, incident.Description, incident.Severity, incident.Status, incident.OwnerUserID, nullableID(incident.AssigneeUserID), incident.ClassificationLevel, tagsToJSON(normalizeTags(incident.ClassificationTags)), metaToJSON(NormalizeIncidentMeta(incident.Meta)), incident.UpdatedBy, now, incident.ID, expectedVersion)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrConflict
	}
	incident.Version = expectedVersion + 1
	incident.UpdatedAt = now
	return nil
}

func (s *incidentsStore) CloseIncident(ctx context.Context, incidentID int64, userID int64) (*Incident, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE incidents SET status='closed', closed_at=?, closed_by=?, updated_at=?, updated_by=?, version=version+1
		WHERE id=? AND deleted_at IS NULL AND status!='closed'`,
		now, userID, now, userID, incidentID)
	if err != nil {
		return nil, err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return nil, ErrConflict
	}
	return s.GetIncident(ctx, incidentID)
}

func (s *incidentsStore) SoftDeleteIncident(ctx context.Context, id int64, updatedBy int64) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE incidents SET deleted_at=?, updated_at=?, updated_by=?, version=version+1 WHERE id=? AND deleted_at IS NULL`,
		now, now, updatedBy, id)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrConflict
	}
	return nil
}

func (s *incidentsStore) RestoreIncident(ctx context.Context, id int64, updatedBy int64) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE incidents SET deleted_at=NULL, updated_at=?, updated_by=?, version=version+1 WHERE id=? AND deleted_at IS NOT NULL`,
		now, updatedBy, id)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrConflict
	}
	return nil
}

func (s *incidentsStore) GetIncident(ctx context.Context, id int64) (*Incident, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, reg_no, title, description, severity, status, source, source_ref_id, closed_at, closed_by, owner_user_id, assignee_user_id, classification_level, classification_tags, meta_json, created_by, updated_by, created_at, updated_at, version, deleted_at
		FROM incidents WHERE id=?`, id)
	return s.scanIncident(row)
}

func (s *incidentsStore) GetIncidentByRegNo(ctx context.Context, regNo string) (*Incident, error) {
	if strings.TrimSpace(regNo) == "" {
		return nil, nil
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, reg_no, title, description, severity, status, source, source_ref_id, closed_at, closed_by, owner_user_id, assignee_user_id, classification_level, classification_tags, meta_json, created_by, updated_by, created_at, updated_at, version, deleted_at
		FROM incidents WHERE reg_no=?`, regNo)
	return s.scanIncident(row)
}

func (s *incidentsStore) FindOpenIncidentBySource(ctx context.Context, source string, refID int64) (*Incident, error) {
	src := strings.ToLower(strings.TrimSpace(source))
	if src == "" || refID <= 0 {
		return nil, nil
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, reg_no, title, description, severity, status, source, source_ref_id, closed_at, closed_by, owner_user_id, assignee_user_id, classification_level, classification_tags, meta_json, created_by, updated_by, created_at, updated_at, version, deleted_at
		FROM incidents
		WHERE deleted_at IS NULL AND status!='closed' AND LOWER(source)=? AND source_ref_id=?
		ORDER BY created_at DESC LIMIT 1`, src, refID)
	return s.scanIncident(row)
}

func (s *incidentsStore) ListIncidents(ctx context.Context, filter IncidentFilter) ([]Incident, error) {
	var clauses []string
	var args []any
	if !filter.IncludeDeleted {
		clauses = append(clauses, "deleted_at IS NULL")
	}
	if len(filter.StatusIn) > 0 {
		var in []string
		for _, raw := range filter.StatusIn {
			if strings.TrimSpace(raw) != "" {
				in = append(in, strings.TrimSpace(raw))
			}
		}
		if len(in) > 0 {
			placeholders := strings.Repeat("?,", len(in))
			placeholders = strings.TrimRight(placeholders, ",")
			clauses = append(clauses, fmt.Sprintf("status IN (%s)", placeholders))
			for _, val := range in {
				args = append(args, val)
			}
		}
	} else if filter.Status != "" {
		clauses = append(clauses, "status=?")
		args = append(args, filter.Status)
	}
	if filter.Severity != "" {
		clauses = append(clauses, "severity=?")
		args = append(args, filter.Severity)
	}
	if filter.Search != "" {
		clauses = append(clauses, "(title LIKE ? OR description LIKE ? OR reg_no LIKE ?)")
		q := "%" + filter.Search + "%"
		args = append(args, q, q, q)
	}
	if filter.MineUserID > 0 {
		clauses = append(clauses, "(owner_user_id=? OR assignee_user_id=?)")
		args = append(args, filter.MineUserID, filter.MineUserID)
	}
	if filter.AssignedUserID > 0 {
		clauses = append(clauses, "assignee_user_id=?")
		args = append(args, filter.AssignedUserID)
	}
	if filter.CreatedByUserID > 0 {
		clauses = append(clauses, "created_by=?")
		args = append(args, filter.CreatedByUserID)
	}
	query := `SELECT id, reg_no, title, description, severity, status, source, source_ref_id, closed_at, closed_by, owner_user_id, assignee_user_id, classification_level, classification_tags, meta_json, created_by, updated_by, created_at, updated_at, version, deleted_at FROM incidents`
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY updated_at DESC"
	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
		if filter.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", filter.Offset)
		}
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []Incident
	for rows.Next() {
		incident, err := s.scanIncidentRow(rows)
		if err != nil {
			return nil, err
		}
		res = append(res, incident)
	}
	return res, rows.Err()
}

func (s *incidentsStore) SetIncidentACL(ctx context.Context, incidentID int64, acl []ACLRule) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM incident_acl WHERE incident_id=?`, incidentID); err != nil {
		tx.Rollback()
		return err
	}
	for _, a := range acl {
		if _, err := tx.ExecContext(ctx, `INSERT INTO incident_acl(incident_id, subject_type, subject_id, permission) VALUES(?,?,?,?)`,
			incidentID, strings.ToLower(a.SubjectType), a.SubjectID, strings.ToLower(a.Permission)); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *incidentsStore) GetIncidentACL(ctx context.Context, incidentID int64) ([]ACLRule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT subject_type, subject_id, permission FROM incident_acl WHERE incident_id=?`, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []ACLRule
	for rows.Next() {
		var a ACLRule
		if err := rows.Scan(&a.SubjectType, &a.SubjectID, &a.Permission); err != nil {
			return nil, err
		}
		res = append(res, a)
	}
	return res, rows.Err()
}

func (s *incidentsStore) SetIncidentParticipants(ctx context.Context, incidentID int64, participants []IncidentParticipant) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM incident_participants WHERE incident_id=?`, incidentID); err != nil {
		tx.Rollback()
		return err
	}
	for _, p := range participants {
		if _, err := tx.ExecContext(ctx, `INSERT INTO incident_participants(incident_id, user_id, role) VALUES(?,?,?)`,
			incidentID, p.UserID, strings.ToLower(strings.TrimSpace(p.Role))); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *incidentsStore) ListIncidentParticipants(ctx context.Context, incidentID int64) ([]IncidentParticipant, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT incident_id, user_id, role FROM incident_participants WHERE incident_id=? ORDER BY user_id ASC`, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []IncidentParticipant
	for rows.Next() {
		var p IncidentParticipant
		if err := rows.Scan(&p.IncidentID, &p.UserID, &p.Role); err != nil {
			return nil, err
		}
		res = append(res, p)
	}
	return res, rows.Err()
}

func (s *incidentsStore) CreateIncidentStage(ctx context.Context, stage *IncidentStage) (int64, error) {
	if stage.Version <= 0 {
		stage.Version = 1
	}
	if strings.TrimSpace(stage.Status) == "" {
		stage.Status = "open"
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO incident_stages(incident_id, title, position, created_by, updated_by, created_at, updated_at, status, closed_at, closed_by, is_default, version)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		stage.IncidentID, stage.Title, stage.Position, stage.CreatedBy, stage.UpdatedBy, now, now, stage.Status, nullableTime(stage.ClosedAt), nullableID(stage.ClosedBy), boolToInt(stage.IsDefault), stage.Version)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	stage.ID = id
	stage.CreatedAt = now
	stage.UpdatedAt = now
	stage.ClosedAt = nil
	stage.ClosedBy = nil
	return id, nil
}

func (s *incidentsStore) UpdateIncidentStage(ctx context.Context, stage *IncidentStage, expectedVersion int) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE incident_stages SET title=?, position=?, updated_by=?, updated_at=?, version=version+1 WHERE id=? AND version=?`,
		stage.Title, stage.Position, stage.UpdatedBy, now, stage.ID, expectedVersion)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrConflict
	}
	stage.Version = expectedVersion + 1
	stage.UpdatedAt = now
	return nil
}

func (s *incidentsStore) CompleteIncidentStage(ctx context.Context, stageID int64, userID int64) (*IncidentStage, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE incident_stages SET status='done', closed_at=?, closed_by=?, updated_at=?, updated_by=?, version=version+1
		WHERE id=? AND status!='done'`,
		now, userID, now, userID, stageID)
	if err != nil {
		return nil, err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return nil, ErrConflict
	}
	return s.GetIncidentStage(ctx, stageID)
}

func (s *incidentsStore) DeleteIncidentStage(ctx context.Context, stageID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM incident_stages WHERE id=?`, stageID)
	return err
}

func (s *incidentsStore) GetIncidentStage(ctx context.Context, stageID int64) (*IncidentStage, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, incident_id, title, position, created_by, updated_by, created_at, updated_at, status, closed_at, closed_by, is_default, version
		FROM incident_stages WHERE id=?`, stageID)
	return s.scanIncidentStage(row)
}

func (s *incidentsStore) ListIncidentStages(ctx context.Context, incidentID int64) ([]IncidentStage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, incident_id, title, position, created_by, updated_by, created_at, updated_at, status, closed_at, closed_by, is_default, version
		FROM incident_stages WHERE incident_id=? ORDER BY position ASC, id ASC`, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []IncidentStage
	for rows.Next() {
		var st IncidentStage
		var closedAt sql.NullTime
		var closedBy sql.NullInt64
		var defInt int
		if err := rows.Scan(&st.ID, &st.IncidentID, &st.Title, &st.Position, &st.CreatedBy, &st.UpdatedBy, &st.CreatedAt, &st.UpdatedAt, &st.Status, &closedAt, &closedBy, &defInt, &st.Version); err != nil {
			return nil, err
		}
		if strings.TrimSpace(st.Status) == "" {
			st.Status = "open"
		}
		if closedAt.Valid {
			st.ClosedAt = &closedAt.Time
		}
		if closedBy.Valid {
			st.ClosedBy = &closedBy.Int64
		}
		st.IsDefault = defInt == 1
		res = append(res, st)
	}
	return res, rows.Err()
}

func (s *incidentsStore) CreateStageEntry(ctx context.Context, entry *IncidentStageEntry) (int64, error) {
	if entry.Version <= 0 {
		entry.Version = 1
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO incident_stage_entries(stage_id, content, change_reason, created_by, updated_by, created_at, updated_at, version)
		VALUES(?,?,?,?,?,?,?,?)`, entry.StageID, entry.Content, entry.ChangeReason, entry.CreatedBy, entry.UpdatedBy, now, now, entry.Version)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	entry.ID = id
	entry.CreatedAt = now
	entry.UpdatedAt = now
	return id, nil
}

func (s *incidentsStore) UpdateStageEntry(ctx context.Context, entry *IncidentStageEntry, expectedVersion int) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE incident_stage_entries SET content=?, change_reason=?, updated_by=?, updated_at=?, version=version+1 WHERE stage_id=? AND version=?`,
		entry.Content, entry.ChangeReason, entry.UpdatedBy, now, entry.StageID, expectedVersion)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrConflict
	}
	entry.Version = expectedVersion + 1
	entry.UpdatedAt = now
	return nil
}

func (s *incidentsStore) GetStageEntry(ctx context.Context, stageID int64) (*IncidentStageEntry, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, stage_id, content, change_reason, created_by, updated_by, created_at, updated_at, version
		FROM incident_stage_entries WHERE stage_id=?`, stageID)
	var e IncidentStageEntry
	if err := row.Scan(&e.ID, &e.StageID, &e.Content, &e.ChangeReason, &e.CreatedBy, &e.UpdatedBy, &e.CreatedAt, &e.UpdatedAt, &e.Version); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &e, nil
}

func (s *incidentsStore) NextStagePosition(ctx context.Context, incidentID int64) (int, error) {
	row := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), 0) FROM incident_stages WHERE incident_id=?`, incidentID)
	var max int
	if err := row.Scan(&max); err != nil {
		return 0, err
	}
	return max + 1, nil
}

func NormalizeIncidentMeta(meta IncidentMeta) IncidentMeta {
	meta.IncidentType = strings.TrimSpace(meta.IncidentType)
	meta.DetectionSource = strings.TrimSpace(meta.DetectionSource)
	meta.SLAResponse = strings.TrimSpace(meta.SLAResponse)
	meta.FirstResponseDeadline = strings.TrimSpace(meta.FirstResponseDeadline)
	meta.ResolveDeadline = strings.TrimSpace(meta.ResolveDeadline)
	meta.WhatHappened = strings.TrimSpace(meta.WhatHappened)
	meta.DetectedAt = strings.TrimSpace(meta.DetectedAt)
	meta.AffectedSystems = strings.TrimSpace(meta.AffectedSystems)
	meta.Risk = strings.TrimSpace(meta.Risk)
	meta.ActionsTaken = strings.TrimSpace(meta.ActionsTaken)
	meta.Assets = strings.TrimSpace(meta.Assets)
	meta.Tags = normalizeTags(meta.Tags)
	meta.ClosureOutcome = strings.TrimSpace(meta.ClosureOutcome)
	meta.Postmortem = strings.TrimSpace(meta.Postmortem)
	return meta
}

func metaToJSON(meta IncidentMeta) string {
	norm := NormalizeIncidentMeta(meta)
	b, err := json.Marshal(norm)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func parseIncidentMeta(raw string) IncidentMeta {
	if strings.TrimSpace(raw) == "" {
		return IncidentMeta{}
	}
	var meta IncidentMeta
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return IncidentMeta{}
	}
	return NormalizeIncidentMeta(meta)
}

func (s *incidentsStore) scanIncident(row *sql.Row) (*Incident, error) {
	var inc Incident
	var assignee sql.NullInt64
	var deleted sql.NullTime
	var closedAt sql.NullTime
	var closedBy sql.NullInt64
	var sourceRef sql.NullInt64
	var tagsRaw string
	var metaRaw string
	if err := row.Scan(&inc.ID, &inc.RegNo, &inc.Title, &inc.Description, &inc.Severity, &inc.Status, &inc.Source, &sourceRef, &closedAt, &closedBy, &inc.OwnerUserID, &assignee, &inc.ClassificationLevel, &tagsRaw, &metaRaw, &inc.CreatedBy, &inc.UpdatedBy, &inc.CreatedAt, &inc.UpdatedAt, &inc.Version, &deleted); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if strings.TrimSpace(inc.Status) == "" {
		inc.Status = "draft"
	}
	if closedAt.Valid {
		inc.ClosedAt = &closedAt.Time
	}
	if closedBy.Valid {
		inc.ClosedBy = &closedBy.Int64
	}
	if assignee.Valid {
		inc.AssigneeUserID = &assignee.Int64
	}
	if sourceRef.Valid {
		inc.SourceRefID = &sourceRef.Int64
	}
	if deleted.Valid {
		inc.DeletedAt = &deleted.Time
	}
	_ = json.Unmarshal([]byte(tagsRaw), &inc.ClassificationTags)
	inc.Meta = parseIncidentMeta(metaRaw)
	return &inc, nil
}

func (s *incidentsStore) scanIncidentRow(rows *sql.Rows) (Incident, error) {
	var inc Incident
	var assignee sql.NullInt64
	var deleted sql.NullTime
	var closedAt sql.NullTime
	var closedBy sql.NullInt64
	var sourceRef sql.NullInt64
	var tagsRaw string
	var metaRaw string
	if err := rows.Scan(&inc.ID, &inc.RegNo, &inc.Title, &inc.Description, &inc.Severity, &inc.Status, &inc.Source, &sourceRef, &closedAt, &closedBy, &inc.OwnerUserID, &assignee, &inc.ClassificationLevel, &tagsRaw, &metaRaw, &inc.CreatedBy, &inc.UpdatedBy, &inc.CreatedAt, &inc.UpdatedAt, &inc.Version, &deleted); err != nil {
		return inc, err
	}
	if strings.TrimSpace(inc.Status) == "" {
		inc.Status = "draft"
	}
	if closedAt.Valid {
		inc.ClosedAt = &closedAt.Time
	}
	if closedBy.Valid {
		inc.ClosedBy = &closedBy.Int64
	}
	if assignee.Valid {
		inc.AssigneeUserID = &assignee.Int64
	}
	if sourceRef.Valid {
		inc.SourceRefID = &sourceRef.Int64
	}
	if deleted.Valid {
		inc.DeletedAt = &deleted.Time
	}
	_ = json.Unmarshal([]byte(tagsRaw), &inc.ClassificationTags)
	inc.Meta = parseIncidentMeta(metaRaw)
	return inc, nil
}

func (s *incidentsStore) scanIncidentStage(row *sql.Row) (*IncidentStage, error) {
	var st IncidentStage
	var closedAt sql.NullTime
	var closedBy sql.NullInt64
	var defInt int
	if err := row.Scan(&st.ID, &st.IncidentID, &st.Title, &st.Position, &st.CreatedBy, &st.UpdatedBy, &st.CreatedAt, &st.UpdatedAt, &st.Status, &closedAt, &closedBy, &defInt, &st.Version); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if strings.TrimSpace(st.Status) == "" {
		st.Status = "open"
	}
	if closedAt.Valid {
		st.ClosedAt = &closedAt.Time
	}
	if closedBy.Valid {
		st.ClosedBy = &closedBy.Int64
	}
	st.IsDefault = defInt == 1
	return &st, nil
}

func (s *incidentsStore) nextIncidentSeqTx(ctx context.Context, tx *sql.Tx, year int) (int64, error) {
	var seq int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO incident_reg_counters(year, seq)
		VALUES(?,1)
		ON CONFLICT (year)
		DO UPDATE SET seq = incident_reg_counters.seq + 1
		RETURNING seq
	`, year).Scan(&seq); err != nil {
		return 0, err
	}
	return seq, nil
}

var seqToken = regexp.MustCompile(`\{seq(?::(\d+))?\}`)

func buildIncidentRegNo(format string, year int, seq int64) string {
	if strings.TrimSpace(format) == "" {
		format = "INC-{year}-{seq:05}"
	}
	out := strings.ReplaceAll(format, "{year}", fmt.Sprintf("%d", year))
	out = seqToken.ReplaceAllStringFunc(out, func(token string) string {
		m := seqToken.FindStringSubmatch(token)
		if len(m) == 2 && m[1] != "" {
			width := 0
			_, _ = fmt.Sscanf(m[1], "%d", &width)
			if width > 0 {
				return fmt.Sprintf("%0*d", width, seq)
			}
		}
		return fmt.Sprintf("%d", seq)
	})
	return out
}

func ensureIncidentACLDefaults(incident *Incident, participants []IncidentParticipant, acl []ACLRule) []ACLRule {
	existing := map[string]struct{}{}
	for _, rule := range acl {
		key := strings.ToLower(rule.SubjectType) + "|" + rule.SubjectID + "|" + strings.ToLower(rule.Permission)
		existing[key] = struct{}{}
	}
	add := func(subjectID, perm string) {
		if subjectID == "" || perm == "" {
			return
		}
		key := "user|" + subjectID + "|" + strings.ToLower(perm)
		if _, ok := existing[key]; ok {
			return
		}
		existing[key] = struct{}{}
		acl = append(acl, ACLRule{SubjectType: "user", SubjectID: subjectID, Permission: strings.ToLower(perm)})
	}
	if incident != nil {
		if incident.OwnerUserID != 0 {
			add(fmt.Sprintf("%d", incident.OwnerUserID), "manage")
		}
		if incident.CreatedBy != 0 && incident.CreatedBy != incident.OwnerUserID {
			add(fmt.Sprintf("%d", incident.CreatedBy), "manage")
		}
		if incident.AssigneeUserID != nil {
			add(fmt.Sprintf("%d", *incident.AssigneeUserID), "edit")
		}
	}
	for _, p := range participants {
		if p.UserID != 0 {
			add(fmt.Sprintf("%d", p.UserID), "view")
		}
	}
	return acl
}

func (s *incidentsStore) ListIncidentLinks(ctx context.Context, incidentID int64) ([]IncidentLink, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, incident_id, entity_type, entity_id, COALESCE(title, ''), COALESCE(comment, ''), unverified, created_by, created_at
		FROM incident_links WHERE incident_id=? ORDER BY created_at DESC, id DESC`, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []IncidentLink
	for rows.Next() {
		var l IncidentLink
		var unverified int
		if err := rows.Scan(&l.ID, &l.IncidentID, &l.EntityType, &l.EntityID, &l.Title, &l.Comment, &unverified, &l.CreatedBy, &l.CreatedAt); err != nil {
			return nil, err
		}
		l.Unverified = unverified == 1
		res = append(res, l)
	}
	return res, rows.Err()
}

func (s *incidentsStore) AddIncidentLink(ctx context.Context, link *IncidentLink) (int64, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO incident_links(incident_id, entity_type, entity_id, title, comment, unverified, created_by, created_at)
		VALUES(?,?,?,?,?,?,?,?)`,
		link.IncidentID, strings.ToLower(strings.TrimSpace(link.EntityType)), strings.TrimSpace(link.EntityID), strings.TrimSpace(link.Title), strings.TrimSpace(link.Comment), boolToInt(link.Unverified), link.CreatedBy, now)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	link.ID = id
	link.CreatedAt = now
	return id, nil
}

func (s *incidentsStore) DeleteIncidentLink(ctx context.Context, linkID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM incident_links WHERE id=?`, linkID)
	return err
}

func (s *incidentsStore) ListIncidentAttachments(ctx context.Context, incidentID int64) ([]IncidentAttachment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, incident_id, filename, content_type, size_bytes, sha256_plain, sha256_cipher, classification_level, classification_tags, uploaded_by, uploaded_at, deleted_at
		FROM incident_attachments WHERE incident_id=? AND deleted_at IS NULL ORDER BY uploaded_at DESC, id DESC`, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []IncidentAttachment
	for rows.Next() {
		var a IncidentAttachment
		var tagsRaw string
		var deleted sql.NullTime
		if err := rows.Scan(&a.ID, &a.IncidentID, &a.Filename, &a.ContentType, &a.SizeBytes, &a.SHA256Plain, &a.SHA256Cipher, &a.ClassificationLevel, &tagsRaw, &a.UploadedBy, &a.UploadedAt, &deleted); err != nil {
			return nil, err
		}
		if deleted.Valid {
			a.DeletedAt = &deleted.Time
		}
		_ = json.Unmarshal([]byte(tagsRaw), &a.ClassificationTags)
		res = append(res, a)
	}
	return res, rows.Err()
}

func (s *incidentsStore) AddIncidentAttachment(ctx context.Context, att *IncidentAttachment) (int64, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO incident_attachments(incident_id, filename, content_type, size_bytes, sha256_plain, sha256_cipher, classification_level, classification_tags, uploaded_by, uploaded_at, deleted_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,NULL)`,
		att.IncidentID, att.Filename, att.ContentType, att.SizeBytes, att.SHA256Plain, att.SHA256Cipher, att.ClassificationLevel, tagsToJSON(normalizeTags(att.ClassificationTags)), att.UploadedBy, now)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	att.ID = id
	att.UploadedAt = now
	return id, nil
}

func (s *incidentsStore) GetIncidentAttachment(ctx context.Context, incidentID, attachmentID int64) (*IncidentAttachment, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, incident_id, filename, content_type, size_bytes, sha256_plain, sha256_cipher, classification_level, classification_tags, uploaded_by, uploaded_at, deleted_at
		FROM incident_attachments WHERE id=? AND incident_id=?`, attachmentID, incidentID)
	var att IncidentAttachment
	var tagsRaw string
	var deleted sql.NullTime
	if err := row.Scan(&att.ID, &att.IncidentID, &att.Filename, &att.ContentType, &att.SizeBytes, &att.SHA256Plain, &att.SHA256Cipher, &att.ClassificationLevel, &tagsRaw, &att.UploadedBy, &att.UploadedAt, &deleted); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if deleted.Valid {
		att.DeletedAt = &deleted.Time
	}
	_ = json.Unmarshal([]byte(tagsRaw), &att.ClassificationTags)
	return &att, nil
}

func (s *incidentsStore) SoftDeleteIncidentAttachment(ctx context.Context, attachmentID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE incident_attachments SET deleted_at=? WHERE id=? AND deleted_at IS NULL`, time.Now().UTC(), attachmentID)
	return err
}

func (s *incidentsStore) ListIncidentArtifactFiles(ctx context.Context, incidentID int64, artifactID string) ([]IncidentArtifactFile, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, incident_id, artifact_id, filename, content_type, size_bytes, sha256_plain, sha256_cipher, classification_level, classification_tags, uploaded_by, uploaded_at, deleted_at
		FROM incident_artifact_files
		WHERE incident_id=? AND artifact_id=? AND deleted_at IS NULL
		ORDER BY uploaded_at DESC, id DESC`, incidentID, strings.TrimSpace(artifactID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []IncidentArtifactFile
	for rows.Next() {
		var f IncidentArtifactFile
		var tagsRaw string
		var deleted sql.NullTime
		if err := rows.Scan(&f.ID, &f.IncidentID, &f.ArtifactID, &f.Filename, &f.ContentType, &f.SizeBytes, &f.SHA256Plain, &f.SHA256Cipher, &f.ClassificationLevel, &tagsRaw, &f.UploadedBy, &f.UploadedAt, &deleted); err != nil {
			return nil, err
		}
		if deleted.Valid {
			f.DeletedAt = &deleted.Time
		}
		_ = json.Unmarshal([]byte(tagsRaw), &f.ClassificationTags)
		res = append(res, f)
	}
	return res, rows.Err()
}

func (s *incidentsStore) AddIncidentArtifactFile(ctx context.Context, file *IncidentArtifactFile) (int64, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO incident_artifact_files(incident_id, artifact_id, filename, content_type, size_bytes, sha256_plain, sha256_cipher, classification_level, classification_tags, uploaded_by, uploaded_at, deleted_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,NULL)`,
		file.IncidentID, strings.TrimSpace(file.ArtifactID), file.Filename, file.ContentType, file.SizeBytes, file.SHA256Plain, file.SHA256Cipher, file.ClassificationLevel, tagsToJSON(normalizeTags(file.ClassificationTags)), file.UploadedBy, now)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	file.ID = id
	file.UploadedAt = now
	return id, nil
}

func (s *incidentsStore) GetIncidentArtifactFile(ctx context.Context, incidentID, fileID int64) (*IncidentArtifactFile, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, incident_id, artifact_id, filename, content_type, size_bytes, sha256_plain, sha256_cipher, classification_level, classification_tags, uploaded_by, uploaded_at, deleted_at
		FROM incident_artifact_files WHERE id=? AND incident_id=?`, fileID, incidentID)
	var f IncidentArtifactFile
	var tagsRaw string
	var deleted sql.NullTime
	if err := row.Scan(&f.ID, &f.IncidentID, &f.ArtifactID, &f.Filename, &f.ContentType, &f.SizeBytes, &f.SHA256Plain, &f.SHA256Cipher, &f.ClassificationLevel, &tagsRaw, &f.UploadedBy, &f.UploadedAt, &deleted); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if deleted.Valid {
		f.DeletedAt = &deleted.Time
	}
	_ = json.Unmarshal([]byte(tagsRaw), &f.ClassificationTags)
	return &f, nil
}

func (s *incidentsStore) SoftDeleteIncidentArtifactFile(ctx context.Context, fileID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE incident_artifact_files SET deleted_at=? WHERE id=? AND deleted_at IS NULL`, time.Now().UTC(), fileID)
	return err
}

func (s *incidentsStore) ListIncidentTimeline(ctx context.Context, incidentID int64, limit int, eventType string) ([]IncidentTimelineEvent, error) {
	query := `
		SELECT id, incident_id, event_type, message, meta_json, created_by, created_at, event_at
		FROM incident_timeline WHERE incident_id=?`
	args := []any{incidentID}
	if strings.TrimSpace(eventType) != "" {
		query += " AND event_type=?"
		args = append(args, eventType)
	}
	query += " ORDER BY COALESCE(event_at, created_at) DESC, id DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []IncidentTimelineEvent
	for rows.Next() {
		var ev IncidentTimelineEvent
		var eventAt sql.NullTime
		if err := rows.Scan(&ev.ID, &ev.IncidentID, &ev.EventType, &ev.Message, &ev.MetaJSON, &ev.CreatedBy, &ev.CreatedAt, &eventAt); err != nil {
			return nil, err
		}
		if eventAt.Valid {
			ev.EventAt = eventAt.Time
		} else {
			ev.EventAt = ev.CreatedAt
		}
		res = append(res, ev)
	}
	return res, rows.Err()
}

func (s *incidentsStore) AddIncidentTimeline(ctx context.Context, ev *IncidentTimelineEvent) (int64, error) {
	now := time.Now().UTC()
	if ev.EventAt.IsZero() {
		ev.EventAt = now
	} else {
		ev.EventAt = ev.EventAt.UTC()
	}
	if strings.TrimSpace(ev.MetaJSON) == "" {
		ev.MetaJSON = "{}"
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO incident_timeline(incident_id, event_type, message, meta_json, created_by, created_at, event_at)
		VALUES(?,?,?,?,?,?,?)`,
		ev.IncidentID, strings.TrimSpace(ev.EventType), strings.TrimSpace(ev.Message), ev.MetaJSON, ev.CreatedBy, now, ev.EventAt)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	ev.ID = id
	ev.CreatedAt = now
	return id, nil
}
