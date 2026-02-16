package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Folder struct {
	ID                    int64     `json:"id"`
	Name                  string    `json:"name"`
	ParentID              *int64    `json:"parent_id,omitempty"`
	ClassificationLevel   int       `json:"classification_level"`
	ClassificationTags    []string  `json:"classification_tags"`
	InheritACL            bool      `json:"inherit_acl"`
	InheritClassification bool      `json:"inherit_classification"`
	CreatedBy             int64     `json:"created_by"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type Document struct {
	ID                    int64      `json:"id"`
	FolderID              *int64     `json:"folder_id,omitempty"`
	Title                 string     `json:"title"`
	Status                string     `json:"status"`
	ClassificationLevel   int        `json:"classification_level"`
	ClassificationTags    []string   `json:"classification_tags"`
	RegNumber             string     `json:"reg_number"`
	DocType               string     `json:"doc_type"`
	InheritACL            bool       `json:"inherit_acl"`
	InheritClassification bool       `json:"inherit_classification"`
	CreatedBy             int64      `json:"created_by"`
	CurrentVersion        int        `json:"current_version"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	DeletedAt             *time.Time `json:"deleted_at,omitempty"`
}

type DocVersion struct {
	ID             int64     `json:"id"`
	DocID          int64     `json:"doc_id"`
	Version        int       `json:"version"`
	AuthorID       int64     `json:"author_id"`
	AuthorUsername string    `json:"author_username"`
	Reason         string    `json:"reason"`
	Path           string    `json:"path"`
	Format         string    `json:"format"`
	SizeBytes      int64     `json:"size_bytes"`
	SHA256Plain    string    `json:"sha256_plain"`
	SHA256Cipher   string    `json:"sha256_cipher"`
	CreatedAt      time.Time `json:"created_at"`
}

type ACLRule struct {
	SubjectType string `json:"subject_type"` // role or user
	SubjectID   string `json:"subject_id"`
	Permission  string `json:"permission"`
}

type Approval struct {
	ID           int64     `json:"id"`
	DocID        int64     `json:"doc_id"`
	Status       string    `json:"status"`
	Message      string    `json:"message"`
	CurrentStage int       `json:"current_stage"`
	CreatedBy    int64     `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ApprovalParticipant struct {
	ApprovalID   int64      `json:"approval_id"`
	UserID       int64      `json:"user_id"`
	Role         string     `json:"role"`
	Stage        int        `json:"stage"`
	StageName    string     `json:"stage_name"`
	StageMessage string     `json:"stage_message"`
	Decision     *string    `json:"decision,omitempty"`
	Comment      *string    `json:"comment,omitempty"`
	DecidedAt    *time.Time `json:"decided_at,omitempty"`
}

type ApprovalComment struct {
	ID         int64     `json:"id"`
	ApprovalID int64     `json:"approval_id"`
	UserID     int64     `json:"user_id"`
	Comment    string    `json:"comment"`
	CreatedAt  time.Time `json:"created_at"`
}

type TemplateVariable struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	Default string `json:"default"`
}

type DocTemplate struct {
	ID                  int64              `json:"id"`
	Name                string             `json:"name"`
	Description         string             `json:"description"`
	Format              string             `json:"format"`
	Content             string             `json:"content"`
	Variables           []TemplateVariable `json:"variables"`
	ClassificationLevel int                `json:"classification_level"`
	ClassificationTags  []string           `json:"classification_tags"`
	CreatedBy           int64              `json:"created_by"`
	CreatedAt           time.Time          `json:"created_at"`
}

type DocExportApproval struct {
	ID          int64      `json:"id"`
	DocID       int64      `json:"doc_id"`
	RequestedBy int64      `json:"requested_by"`
	ApprovedBy  int64      `json:"approved_by"`
	Reason      string     `json:"reason"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
	ConsumedAt  *time.Time `json:"consumed_at,omitempty"`
}

type DocumentFilter struct {
	FolderID       *int64
	Status         string
	StatusIn       []string
	Search         string
	MineUserID     int64
	IncludeDeleted bool
	Tags           []string
	MinLevel       int
	DocType        string
	Limit          int
	Offset         int
	Sort           string
}

type ApprovalFilter struct {
	UserID int64
	Status string
}

type SearchHit struct {
	DocID   int64
	Version int
	Snippet string
}

type DocsStore interface {
	CreateFolder(ctx context.Context, f *Folder) (int64, error)
	UpdateFolder(ctx context.Context, f *Folder) error
	DeleteFolder(ctx context.Context, id int64) error
	GetFolder(ctx context.Context, id int64) (*Folder, error)
	ListFolders(ctx context.Context) ([]Folder, error)
	SetFolderACL(ctx context.Context, folderID int64, acl []ACLRule) error
	GetFolderACL(ctx context.Context, folderID int64) ([]ACLRule, error)

	CreateDocument(ctx context.Context, doc *Document, acl []ACLRule, regTemplate string, perFolderSeq bool) (int64, error)
	UpdateDocument(ctx context.Context, doc *Document) error
	SoftDeleteDocument(ctx context.Context, id int64) error
	GetDocument(ctx context.Context, id int64) (*Document, error)
	FindDocumentByTitle(ctx context.Context, title string, folderID *int64, docType string) (*Document, error)
	ListDocuments(ctx context.Context, filter DocumentFilter) ([]Document, error)
	SetDocACL(ctx context.Context, docID int64, acl []ACLRule) error
	GetDocACL(ctx context.Context, docID int64) ([]ACLRule, error)

	AddVersion(ctx context.Context, v *DocVersion) error
	ListVersions(ctx context.Context, docID int64) ([]DocVersion, error)
	GetVersion(ctx context.Context, docID int64, version int) (*DocVersion, error)
	DeleteVersion(ctx context.Context, docID int64, version int) (*DocVersion, error)

	UpsertFTS(ctx context.Context, docID int64, version int, content string) error
	DeleteFTSForVersion(ctx context.Context, docID int64, version int) error
	SearchFTS(ctx context.Context, query string, docIDs []int64) ([]SearchHit, error)

	CreateApproval(ctx context.Context, ap *Approval, participants []ApprovalParticipant) (int64, error)
	GetApproval(ctx context.Context, id int64) (*Approval, []ApprovalParticipant, error)
	ListApprovals(ctx context.Context, filter ApprovalFilter) ([]Approval, error)
	ListApprovalsByDocIDs(ctx context.Context, docIDs []int64) ([]Approval, error)
	SaveApprovalDecision(ctx context.Context, approvalID int64, userID int64, decision, comment string, stage int) error
	SaveApprovalComment(ctx context.Context, c *ApprovalComment) error
	ListApprovalComments(ctx context.Context, approvalID int64) ([]ApprovalComment, error)
	UpdateApprovalStatus(ctx context.Context, approvalID int64, status string, currentStage int) error
	CleanupApprovals(ctx context.Context, includeActive bool) (int64, error)

	ListLinks(ctx context.Context, docID int64) ([]map[string]string, error)
	AddLink(ctx context.Context, docID int64, targetType, targetID string) error
	DeleteLink(ctx context.Context, linkID int64) error

	ListTemplates(ctx context.Context) ([]DocTemplate, error)
	GetTemplate(ctx context.Context, id int64) (*DocTemplate, error)
	SaveTemplate(ctx context.Context, tpl *DocTemplate) error
	DeleteTemplate(ctx context.Context, id int64) error
	GetActiveApproval(ctx context.Context, docID int64) (*Approval, []ApprovalParticipant, error)
	CreateDocExportApproval(ctx context.Context, item *DocExportApproval) (int64, error)
	ConsumeDocExportApproval(ctx context.Context, docID, requestedBy int64) (*DocExportApproval, error)
}

type docsStore struct {
	db *sql.DB
}

func NewDocsStore(db *sql.DB) DocsStore {
	return &docsStore{db: db}
}

func (s *docsStore) CreateFolder(ctx context.Context, f *Folder) (int64, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO doc_folders(name, parent_id, classification_level, classification_tags, inherit_acl, inherit_classification, created_by, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?,?)`,
		f.Name, nullableID(f.ParentID), f.ClassificationLevel, tagsToJSON(normalizeTags(f.ClassificationTags)), boolToInt(f.InheritACL), boolToInt(f.InheritClassification), f.CreatedBy, now, now)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func (s *docsStore) UpdateFolder(ctx context.Context, f *Folder) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE doc_folders SET name=?, parent_id=?, classification_level=?, classification_tags=?, inherit_acl=?, inherit_classification=?, updated_at=?
		WHERE id=?`,
		f.Name, nullableID(f.ParentID), f.ClassificationLevel, tagsToJSON(normalizeTags(f.ClassificationTags)), boolToInt(f.InheritACL), boolToInt(f.InheritClassification), time.Now().UTC(), f.ID)
	return err
}

func (s *docsStore) DeleteFolder(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM doc_folders WHERE id=?`, id)
	return err
}

func (s *docsStore) GetFolder(ctx context.Context, id int64) (*Folder, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, parent_id, classification_level, classification_tags, inherit_acl, inherit_classification, created_by, created_at, updated_at
		FROM doc_folders WHERE id=?`, id)
	return s.scanFolder(row)
}

func (s *docsStore) ListFolders(ctx context.Context) ([]Folder, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, parent_id, classification_level, classification_tags, inherit_acl, inherit_classification, created_by, created_at, updated_at
		FROM doc_folders ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []Folder
	for rows.Next() {
		var f Folder
		var parent sql.NullInt64
		var tagsRaw string
		if err := rows.Scan(&f.ID, &f.Name, &parent, &f.ClassificationLevel, &tagsRaw, &f.InheritACL, &f.InheritClassification, &f.CreatedBy, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		if parent.Valid {
			f.ParentID = &parent.Int64
		}
		if tagsRaw != "" {
			_ = json.Unmarshal([]byte(tagsRaw), &f.ClassificationTags)
		}
		res = append(res, f)
	}
	return res, rows.Err()
}

func (s *docsStore) SetFolderACL(ctx context.Context, folderID int64, acl []ACLRule) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM folder_acl WHERE folder_id=?`, folderID); err != nil {
		tx.Rollback()
		return err
	}
	for _, a := range acl {
		if _, err := tx.ExecContext(ctx, `INSERT INTO folder_acl(folder_id, subject_type, subject_id, permission) VALUES(?,?,?,?)`, folderID, strings.ToLower(a.SubjectType), a.SubjectID, a.Permission); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *docsStore) GetFolderACL(ctx context.Context, folderID int64) ([]ACLRule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT subject_type, subject_id, permission FROM folder_acl WHERE folder_id=?`, folderID)
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

func (s *docsStore) CreateDocument(ctx context.Context, doc *Document, acl []ACLRule, regTemplate string, perFolderSeq bool) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(doc.RegNumber) == "" {
		seq, err := s.nextSeqTx(ctx, tx, doc.ClassificationLevel, doc.FolderID, time.Now().UTC().Year(), perFolderSeq)
		if err != nil {
			tx.Rollback()
			return 0, err
		}
		doc.RegNumber = buildRegNumber(regTemplate, doc.ClassificationLevel, seq)
	}
	now := time.Now().UTC()
	if strings.TrimSpace(doc.DocType) == "" {
		doc.DocType = "document"
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO docs(folder_id, title, status, classification_level, classification_tags, reg_number, doc_type, inherit_acl, inherit_classification, created_by, current_version, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		nullableID(doc.FolderID), doc.Title, doc.Status, doc.ClassificationLevel, tagsToJSON(normalizeTags(doc.ClassificationTags)), doc.RegNumber, doc.DocType, boolToInt(doc.InheritACL), boolToInt(doc.InheritClassification), doc.CreatedBy, doc.CurrentVersion, now, now)
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	docID, _ := res.LastInsertId()
	if len(acl) > 0 {
		for _, a := range acl {
			if _, err := tx.ExecContext(ctx, `INSERT INTO doc_acl(doc_id, subject_type, subject_id, permission) VALUES(?,?,?,?)`, docID, strings.ToLower(a.SubjectType), a.SubjectID, a.Permission); err != nil {
				tx.Rollback()
				return 0, err
			}
		}
	} else if doc.InheritACL && doc.FolderID != nil {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO doc_acl(doc_id, subject_type, subject_id, permission)
			SELECT ?, subject_type, subject_id, permission FROM folder_acl WHERE folder_id=?`, docID, *doc.FolderID); err != nil {
			tx.Rollback()
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	doc.ID = docID
	doc.CreatedAt = now
	doc.UpdatedAt = now
	return docID, nil
}

func (s *docsStore) nextSeqTx(ctx context.Context, tx *sql.Tx, level int, folderID *int64, year int, perFolder bool) (int64, error) {
	folderKey := int64(0)
	if folderID != nil && perFolder {
		folderKey = *folderID
	}
	var seq int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO doc_reg_counters(classification_level, folder_id, year, seq)
		VALUES(?,?,?,1)
		ON CONFLICT (classification_level, folder_id, year)
		DO UPDATE SET seq = doc_reg_counters.seq + 1
		RETURNING seq
	`, level, folderKey, year).Scan(&seq); err != nil {
		return 0, err
	}
	return seq, nil
}

func buildRegNumber(tmpl string, level int, seq int64) string {
	if strings.TrimSpace(tmpl) == "" {
		tmpl = "{level_code}.{year}.{seq}"
	}
	code := levelCode(level)
	year := time.Now().UTC().Year()
	out := strings.ReplaceAll(tmpl, "{level_code}", code)
	out = strings.ReplaceAll(out, "{level}", code)
	out = strings.ReplaceAll(out, "{year}", fmt.Sprintf("%d", year))
	out = strings.ReplaceAll(out, "{seq}", fmt.Sprintf("%05d", seq))
	return out
}

func (s *docsStore) UpdateDocument(ctx context.Context, doc *Document) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE docs SET title=?, status=?, classification_level=?, classification_tags=?, doc_type=?, inherit_acl=?, inherit_classification=?, updated_at=?, current_version=?
		WHERE id=?`,
		doc.Title, doc.Status, doc.ClassificationLevel, tagsToJSON(normalizeTags(doc.ClassificationTags)), doc.DocType, boolToInt(doc.InheritACL), boolToInt(doc.InheritClassification), time.Now().UTC(), doc.CurrentVersion, doc.ID)
	return err
}

func (s *docsStore) SoftDeleteDocument(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE docs SET deleted_at=?, status=?, updated_at=? WHERE id=?`, time.Now().UTC(), "deleted", time.Now().UTC(), id)
	return err
}

func (s *docsStore) GetDocument(ctx context.Context, id int64) (*Document, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, folder_id, title, status, classification_level, classification_tags, reg_number, doc_type, inherit_acl, inherit_classification, created_by, current_version, created_at, updated_at, deleted_at
		FROM docs WHERE id=?`, id)
	return s.scanDocument(row)
}

func (s *docsStore) FindDocumentByTitle(ctx context.Context, title string, folderID *int64, docType string) (*Document, error) {
	if strings.TrimSpace(docType) == "" {
		docType = "document"
	}
	query := `
		SELECT id, folder_id, title, status, classification_level, classification_tags, reg_number, doc_type, inherit_acl, inherit_classification, created_by, current_version, created_at, updated_at, deleted_at
		FROM docs WHERE deleted_at IS NULL AND doc_type=? AND LOWER(title)=LOWER(?)`
	args := []any{docType, title}
	if folderID == nil {
		query += " AND folder_id IS NULL"
	} else {
		query += " AND folder_id=?"
		args = append(args, *folderID)
	}
	row := s.db.QueryRowContext(ctx, query, args...)
	return s.scanDocument(row)
}

func (s *docsStore) ListDocuments(ctx context.Context, filter DocumentFilter) ([]Document, error) {
	clauses := []string{}
	args := []any{}
	if !filter.IncludeDeleted {
		clauses = append(clauses, "deleted_at IS NULL")
	}
	if filter.FolderID != nil {
		clauses = append(clauses, "folder_id=?")
		args = append(args, *filter.FolderID)
	}
	if filter.Status != "" {
		clauses = append(clauses, "status=?")
		args = append(args, filter.Status)
	} else if len(filter.StatusIn) > 0 {
		clauses = append(clauses, "status IN ("+placeholders(len(filter.StatusIn))+")")
		for _, st := range filter.StatusIn {
			args = append(args, st)
		}
	}
	if filter.MineUserID > 0 {
		clauses = append(clauses, "created_by=?")
		args = append(args, filter.MineUserID)
	}
	if filter.MinLevel > 0 {
		clauses = append(clauses, "classification_level>=?")
		args = append(args, filter.MinLevel)
	}
	if strings.TrimSpace(filter.DocType) != "" {
		clauses = append(clauses, "doc_type=?")
		args = append(args, filter.DocType)
	}
	for _, t := range filter.Tags {
		if strings.TrimSpace(t) == "" {
			continue
		}
		clauses = append(clauses, "classification_tags LIKE ?")
		args = append(args, "%"+strings.ToUpper(strings.TrimSpace(t))+"%")
	}
	baseQuery := `SELECT id, folder_id, title, status, classification_level, classification_tags, reg_number, doc_type, inherit_acl, inherit_classification, created_by, current_version, created_at, updated_at, deleted_at FROM docs`
	where := ""
	if len(clauses) > 0 {
		where = " WHERE " + strings.Join(clauses, " AND ")
	}
	order := " ORDER BY updated_at DESC"
	switch strings.ToLower(filter.Sort) {
	case "title":
		order = " ORDER BY title ASC"
	case "updated_asc":
		order = " ORDER BY updated_at ASC"
	}
	limitOffset := ""
	if filter.Limit > 0 {
		limitOffset = fmt.Sprintf(" LIMIT %d", filter.Limit)
		if filter.Offset > 0 {
			limitOffset += fmt.Sprintf(" OFFSET %d", filter.Offset)
		}
	}
	query := baseQuery + where + order + limitOffset
	if filter.Search != "" {
		matchClauses := strings.Join(clauses, " AND ")
		if matchClauses == "" {
			matchClauses = "1=1"
		}
		query = baseQuery + " INNER JOIN docs_fts ON docs_fts.doc_id=docs.id WHERE (" + matchClauses + ") AND docs_fts MATCH ? " + order + limitOffset
		args = append(args, filter.Search)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []Document
	for rows.Next() {
		doc, err := s.scanDocumentRow(rows)
		if err != nil {
			return nil, err
		}
		res = append(res, doc)
	}
	return res, rows.Err()
}

func (s *docsStore) SetDocACL(ctx context.Context, docID int64, acl []ACLRule) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM doc_acl WHERE doc_id=?`, docID); err != nil {
		tx.Rollback()
		return err
	}
	for _, a := range acl {
		if _, err := tx.ExecContext(ctx, `INSERT INTO doc_acl(doc_id, subject_type, subject_id, permission) VALUES(?,?,?,?)`, docID, strings.ToLower(a.SubjectType), a.SubjectID, a.Permission); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *docsStore) GetDocACL(ctx context.Context, docID int64) ([]ACLRule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT subject_type, subject_id, permission FROM doc_acl WHERE doc_id=?`, docID)
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

func (s *docsStore) AddVersion(ctx context.Context, v *DocVersion) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO doc_versions(doc_id, version, author_id, author_username, reason, path, format, size_bytes, sha256_plain, sha256_cipher, created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		v.DocID, v.Version, v.AuthorID, v.AuthorUsername, v.Reason, v.Path, v.Format, v.SizeBytes, v.SHA256Plain, v.SHA256Cipher, v.CreatedAt)
	if err != nil {
		tx.Rollback()
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE docs SET current_version=?, updated_at=? WHERE id=?`, v.Version, time.Now().UTC(), v.DocID)
	if err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *docsStore) ListVersions(ctx context.Context, docID int64) ([]DocVersion, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, doc_id, version, author_id, author_username, reason, path, format, size_bytes, sha256_plain, sha256_cipher, created_at
		FROM doc_versions WHERE doc_id=? ORDER BY version DESC`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []DocVersion
	for rows.Next() {
		var v DocVersion
		if err := rows.Scan(&v.ID, &v.DocID, &v.Version, &v.AuthorID, &v.AuthorUsername, &v.Reason, &v.Path, &v.Format, &v.SizeBytes, &v.SHA256Plain, &v.SHA256Cipher, &v.CreatedAt); err != nil {
			return nil, err
		}
		res = append(res, v)
	}
	return res, rows.Err()
}

func (s *docsStore) GetVersion(ctx context.Context, docID int64, version int) (*DocVersion, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, doc_id, version, author_id, author_username, reason, path, format, size_bytes, sha256_plain, sha256_cipher, created_at
		FROM doc_versions WHERE doc_id=? AND version=?`, docID, version)
	var v DocVersion
	if err := row.Scan(&v.ID, &v.DocID, &v.Version, &v.AuthorID, &v.AuthorUsername, &v.Reason, &v.Path, &v.Format, &v.SizeBytes, &v.SHA256Plain, &v.SHA256Cipher, &v.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &v, nil
}

func (s *docsStore) DeleteVersion(ctx context.Context, docID int64, version int) (*DocVersion, error) {
	v, err := s.GetVersion(ctx, docID, version)
	if err != nil || v == nil {
		return v, err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM doc_versions WHERE doc_id=? AND version=?`, docID, version)
	if err != nil {
		return nil, err
	}
	_ = s.DeleteFTSForVersion(ctx, docID, version)
	return v, nil
}

func (s *docsStore) UpsertFTS(ctx context.Context, docID int64, version int, content string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM docs_fts WHERE doc_id=? AND version_id=?`, docID, version); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO docs_fts(content, doc_id, version_id) VALUES(?,?,?)`, content, docID, version)
	return err
}

func (s *docsStore) DeleteFTSForVersion(ctx context.Context, docID int64, version int) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM docs_fts WHERE doc_id=? AND version_id=?`, docID, version)
	return err
}

func (s *docsStore) SearchFTS(ctx context.Context, query string, docIDs []int64) ([]SearchHit, error) {
	var args []any
	args = append(args, query)
	where := "docs_fts MATCH ?"
	if len(docIDs) > 0 {
		where += " AND doc_id IN (" + placeholders(len(docIDs)) + ")"
		for _, id := range docIDs {
			args = append(args, id)
		}
	}
	rows, err := s.db.QueryContext(ctx, `SELECT doc_id, version_id FROM docs_fts WHERE `+where+` LIMIT 50`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.DocID, &h.Version); err != nil {
			return nil, err
		}
		res = append(res, h)
	}
	return res, rows.Err()
}

func (s *docsStore) CreateApproval(ctx context.Context, ap *Approval, participants []ApprovalParticipant) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	stage := ap.CurrentStage
	if stage == 0 {
		stage = 1
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO approvals(doc_id, status, message, current_stage, created_by, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?)`,
		ap.DocID, ap.Status, ap.Message, stage, ap.CreatedBy, ap.CreatedAt, ap.UpdatedAt)
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	approvalID, _ := res.LastInsertId()
	for _, p := range participants {
		if p.Stage == 0 {
			p.Stage = 1
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO approval_participants(approval_id, user_id, role, stage, stage_name, stage_message, decision, comment, decided_at) VALUES(?,?,?,?,?,?,?,?,?)`,
			approvalID, p.UserID, p.Role, p.Stage, p.StageName, p.StageMessage, p.Decision, p.Comment, p.DecidedAt); err != nil {
			tx.Rollback()
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return approvalID, nil
}

func (s *docsStore) GetApproval(ctx context.Context, id int64) (*Approval, []ApprovalParticipant, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, doc_id, status, message, current_stage, created_by, created_at, updated_at FROM approvals WHERE id=?`, id)
	var a Approval
	if err := row.Scan(&a.ID, &a.DocID, &a.Status, &a.Message, &a.CurrentStage, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	partsRows, err := s.db.QueryContext(ctx, `SELECT approval_id, user_id, role, stage, stage_name, stage_message, decision, comment, decided_at FROM approval_participants WHERE approval_id=? ORDER BY stage ASC, role ASC`, id)
	if err != nil {
		return &a, nil, err
	}
	defer partsRows.Close()
	var parts []ApprovalParticipant
	for partsRows.Next() {
		var p ApprovalParticipant
		var decision, comment sql.NullString
		var decidedAt sql.NullTime
		var stageName, stageMsg sql.NullString
		if err := partsRows.Scan(&p.ApprovalID, &p.UserID, &p.Role, &p.Stage, &stageName, &stageMsg, &decision, &comment, &decidedAt); err != nil {
			return &a, nil, err
		}
		if stageName.Valid {
			p.StageName = stageName.String
		}
		if stageMsg.Valid {
			p.StageMessage = stageMsg.String
		}
		if decision.Valid {
			val := decision.String
			p.Decision = &val
		}
		if comment.Valid {
			val := comment.String
			p.Comment = &val
		}
		if decidedAt.Valid {
			p.DecidedAt = &decidedAt.Time
		}
		parts = append(parts, p)
	}
	return &a, parts, partsRows.Err()
}

func (s *docsStore) ListApprovals(ctx context.Context, filter ApprovalFilter) ([]Approval, error) {
	query := `SELECT DISTINCT a.id, a.doc_id, a.status, a.message, a.current_stage, a.created_by, a.created_at, a.updated_at FROM approvals a`
	var clauses []string
	var args []any
	if filter.UserID > 0 {
		query += ` JOIN approval_participants ap ON ap.approval_id=a.id`
		clauses = append(clauses, "ap.user_id=?")
		args = append(args, filter.UserID)
	}
	if filter.Status != "" {
		clauses = append(clauses, "a.status=?")
		args = append(args, filter.Status)
	}
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY a.updated_at DESC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []Approval
	for rows.Next() {
		var a Approval
		if err := rows.Scan(&a.ID, &a.DocID, &a.Status, &a.Message, &a.CurrentStage, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		res = append(res, a)
	}
	return res, rows.Err()
}

func (s *docsStore) ListApprovalsByDocIDs(ctx context.Context, docIDs []int64) ([]Approval, error) {
	if len(docIDs) == 0 {
		return nil, nil
	}
	args := make([]any, 0, len(docIDs))
	for _, id := range docIDs {
		args = append(args, id)
	}
	query := `SELECT id, doc_id, status, message, current_stage, created_by, created_at, updated_at FROM approvals WHERE doc_id IN (` + placeholders(len(docIDs)) + `) ORDER BY updated_at DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []Approval
	for rows.Next() {
		var a Approval
		if err := rows.Scan(&a.ID, &a.DocID, &a.Status, &a.Message, &a.CurrentStage, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		res = append(res, a)
	}
	return res, rows.Err()
}

func (s *docsStore) SaveApprovalDecision(ctx context.Context, approvalID int64, userID int64, decision, comment string, stage int) error {
	query := `
		UPDATE approval_participants SET decision=?, comment=?, decided_at=? WHERE approval_id=? AND user_id=? AND role='approver'`
	args := []any{decision, comment, time.Now().UTC(), approvalID, userID}
	if stage > 0 {
		query += " AND stage=?"
		args = append(args, stage)
	}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *docsStore) UpdateApprovalStatus(ctx context.Context, approvalID int64, status string, currentStage int) error {
	if currentStage <= 0 {
		_, err := s.db.ExecContext(ctx, `UPDATE approvals SET status=?, updated_at=? WHERE id=?`, status, time.Now().UTC(), approvalID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE approvals SET status=?, current_stage=?, updated_at=? WHERE id=?`, status, currentStage, time.Now().UTC(), approvalID)
	return err
}

func (s *docsStore) CleanupApprovals(ctx context.Context, includeActive bool) (int64, error) {
	query := "DELETE FROM approvals"
	args := []any{}
	if !includeActive {
		query += " WHERE status <> ?"
		args = append(args, "review")
	}
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	affected, _ := res.RowsAffected()
	return affected, nil
}

func (s *docsStore) SaveApprovalComment(ctx context.Context, c *ApprovalComment) error {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO approval_comments(approval_id, user_id, comment, created_at) VALUES(?,?,?,?)`,
		c.ApprovalID, c.UserID, c.Comment, c.CreatedAt)
	if err != nil {
		return err
	}
	c.ID, _ = res.LastInsertId()
	return nil
}

func (s *docsStore) ListApprovalComments(ctx context.Context, approvalID int64) ([]ApprovalComment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, approval_id, user_id, comment, created_at FROM approval_comments WHERE approval_id=? ORDER BY created_at ASC`, approvalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ApprovalComment
	for rows.Next() {
		var c ApprovalComment
		if err := rows.Scan(&c.ID, &c.ApprovalID, &c.UserID, &c.Comment, &c.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, rows.Err()
}

func (s *docsStore) ListLinks(ctx context.Context, docID int64) ([]map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, target_type, target_id FROM entity_links WHERE doc_id=?`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []map[string]string
	for rows.Next() {
		var id int64
		var ttype, tid string
		if err := rows.Scan(&id, &ttype, &tid); err != nil {
			return nil, err
		}
		res = append(res, map[string]string{
			"id":          fmt.Sprintf("%d", id),
			"target_type": ttype,
			"target_id":   tid,
		})
	}
	return res, rows.Err()
}

func (s *docsStore) AddLink(ctx context.Context, docID int64, targetType, targetID string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO entity_links(doc_id, target_type, target_id, created_at) VALUES(?,?,?,?)`, docID, targetType, targetID, time.Now().UTC())
	return err
}

func (s *docsStore) DeleteLink(ctx context.Context, linkID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM entity_links WHERE id=?`, linkID)
	return err
}

func (s *docsStore) ListTemplates(ctx context.Context) ([]DocTemplate, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, description, format, content, variables, classification_level, classification_tags, created_by, created_at
		FROM doc_templates ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []DocTemplate
	for rows.Next() {
		tpl, err := s.scanTemplateRow(rows)
		if err != nil {
			return nil, err
		}
		res = append(res, tpl)
	}
	return res, rows.Err()
}

func (s *docsStore) GetTemplate(ctx context.Context, id int64) (*DocTemplate, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, format, content, variables, classification_level, classification_tags, created_by, created_at
		FROM doc_templates WHERE id=?`, id)
	tpl, err := s.scanTemplate(row)
	if err != nil {
		return nil, err
	}
	return tpl, nil
}

func (s *docsStore) SaveTemplate(ctx context.Context, tpl *DocTemplate) error {
	varsJSON, _ := json.Marshal(tpl.Variables)
	now := time.Now().UTC()
	if tpl.ID == 0 {
		res, err := s.db.ExecContext(ctx, `
			INSERT INTO doc_templates(name, description, format, content, variables, classification_level, classification_tags, created_by, created_at)
			VALUES(?,?,?,?,?,?,?,?,?)`,
			tpl.Name, tpl.Description, tpl.Format, tpl.Content, string(varsJSON), tpl.ClassificationLevel, tagsToJSON(normalizeTags(tpl.ClassificationTags)), tpl.CreatedBy, now)
		if err != nil {
			return err
		}
		tpl.CreatedAt = now
		tpl.ID, _ = res.LastInsertId()
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE doc_templates SET name=?, description=?, format=?, content=?, variables=?, classification_level=?, classification_tags=? WHERE id=?`,
		tpl.Name, tpl.Description, tpl.Format, tpl.Content, string(varsJSON), tpl.ClassificationLevel, tagsToJSON(normalizeTags(tpl.ClassificationTags)), tpl.ID)
	return err
}

func (s *docsStore) DeleteTemplate(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM doc_templates WHERE id=?`, id)
	return err
}

func (s *docsStore) GetActiveApproval(ctx context.Context, docID int64) (*Approval, []ApprovalParticipant, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, doc_id, status, message, current_stage, created_by, created_at, updated_at FROM approvals WHERE doc_id=? AND status=? ORDER BY updated_at DESC LIMIT 1`, docID, "review")
	var a Approval
	if err := row.Scan(&a.ID, &a.DocID, &a.Status, &a.Message, &a.CurrentStage, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	partsRows, err := s.db.QueryContext(ctx, `SELECT approval_id, user_id, role, stage, stage_name, stage_message, decision, comment, decided_at FROM approval_participants WHERE approval_id=? ORDER BY stage ASC, role ASC`, a.ID)
	if err != nil {
		return &a, nil, err
	}
	defer partsRows.Close()
	var parts []ApprovalParticipant
	for partsRows.Next() {
		var p ApprovalParticipant
		var decision, comment sql.NullString
		var decidedAt sql.NullTime
		var stageName, stageMsg sql.NullString
		if err := partsRows.Scan(&p.ApprovalID, &p.UserID, &p.Role, &p.Stage, &stageName, &stageMsg, &decision, &comment, &decidedAt); err != nil {
			return &a, nil, err
		}
		if stageName.Valid {
			p.StageName = stageName.String
		}
		if stageMsg.Valid {
			p.StageMessage = stageMsg.String
		}
		if decision.Valid {
			val := decision.String
			p.Decision = &val
		}
		if comment.Valid {
			val := comment.String
			p.Comment = &val
		}
		if decidedAt.Valid {
			p.DecidedAt = &decidedAt.Time
		}
		parts = append(parts, p)
	}
	return &a, parts, partsRows.Err()
}

func (s *docsStore) scanFolder(row *sql.Row) (*Folder, error) {
	var f Folder
	var parent sql.NullInt64
	var tagsRaw string
	if err := row.Scan(&f.ID, &f.Name, &parent, &f.ClassificationLevel, &tagsRaw, &f.InheritACL, &f.InheritClassification, &f.CreatedBy, &f.CreatedAt, &f.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if parent.Valid {
		f.ParentID = &parent.Int64
	}
	if tagsRaw != "" {
		_ = json.Unmarshal([]byte(tagsRaw), &f.ClassificationTags)
	}
	return &f, nil
}

func (s *docsStore) scanDocument(row *sql.Row) (*Document, error) {
	var tagsRaw string
	var folder sql.NullInt64
	var deleted sql.NullTime
	var d Document
	if err := row.Scan(&d.ID, &folder, &d.Title, &d.Status, &d.ClassificationLevel, &tagsRaw, &d.RegNumber, &d.DocType, &d.InheritACL, &d.InheritClassification, &d.CreatedBy, &d.CurrentVersion, &d.CreatedAt, &d.UpdatedAt, &deleted); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if folder.Valid {
		d.FolderID = &folder.Int64
	}
	if deleted.Valid {
		d.DeletedAt = &deleted.Time
	}
	if tagsRaw != "" {
		_ = json.Unmarshal([]byte(tagsRaw), &d.ClassificationTags)
	}
	return &d, nil
}

func (s *docsStore) scanDocumentRow(rows *sql.Rows) (Document, error) {
	var d Document
	var folder sql.NullInt64
	var tagsRaw string
	var deleted sql.NullTime
	if err := rows.Scan(&d.ID, &folder, &d.Title, &d.Status, &d.ClassificationLevel, &tagsRaw, &d.RegNumber, &d.DocType, &d.InheritACL, &d.InheritClassification, &d.CreatedBy, &d.CurrentVersion, &d.CreatedAt, &d.UpdatedAt, &deleted); err != nil {
		return d, err
	}
	if folder.Valid {
		d.FolderID = &folder.Int64
	}
	if deleted.Valid {
		d.DeletedAt = &deleted.Time
	}
	if tagsRaw != "" {
		_ = json.Unmarshal([]byte(tagsRaw), &d.ClassificationTags)
	}
	return d, nil
}

func (s *docsStore) scanTemplate(row interface {
	Scan(dest ...any) error
}) (*DocTemplate, error) {
	var tpl DocTemplate
	var tagsRaw, varsRaw string
	if err := row.Scan(&tpl.ID, &tpl.Name, &tpl.Description, &tpl.Format, &tpl.Content, &varsRaw, &tpl.ClassificationLevel, &tagsRaw, &tpl.CreatedBy, &tpl.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if tagsRaw != "" {
		_ = json.Unmarshal([]byte(tagsRaw), &tpl.ClassificationTags)
	}
	if varsRaw != "" {
		_ = json.Unmarshal([]byte(varsRaw), &tpl.Variables)
	}
	return &tpl, nil
}

func (s *docsStore) scanTemplateRow(rows *sql.Rows) (DocTemplate, error) {
	var tpl DocTemplate
	var tagsRaw, varsRaw string
	if err := rows.Scan(&tpl.ID, &tpl.Name, &tpl.Description, &tpl.Format, &tpl.Content, &varsRaw, &tpl.ClassificationLevel, &tagsRaw, &tpl.CreatedBy, &tpl.CreatedAt); err != nil {
		return tpl, err
	}
	if tagsRaw != "" {
		_ = json.Unmarshal([]byte(tagsRaw), &tpl.ClassificationTags)
	}
	if varsRaw != "" {
		_ = json.Unmarshal([]byte(varsRaw), &tpl.Variables)
	}
	return tpl, nil
}

func nullableID(id *int64) any {
	if id == nil {
		return nil
	}
	return *id
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	sb := strings.Builder{}
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("?")
	}
	return sb.String()
}

func normalizeTags(tags []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, t := range tags {
		tt := strings.ToUpper(strings.TrimSpace(t))
		if tt == "" {
			continue
		}
		if _, ok := seen[tt]; ok {
			continue
		}
		out = append(out, tt)
		seen[tt] = struct{}{}
	}
	return out
}

var classificationCodes = map[int]string{
	0: "PUB",
	1: "INT",
	2: "CONF",
	3: "DSP",
	4: "SEC",
	5: "TOP",
	6: "SI",
}

func levelCode(level int) string {
	if c, ok := classificationCodes[level]; ok {
		return c
	}
	return "UNK"
}
