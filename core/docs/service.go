package docs

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"berkut-scc/config"
	"berkut-scc/core/store"
	"berkut-scc/core/utils"
)

type ConverterStatus struct {
	Enabled          bool   `json:"enabled"`
	PandocAvailable  bool   `json:"pandoc_available"`
	SofficeAvailable bool   `json:"soffice_available"`
	Message          string `json:"message,omitempty"`
}

type Service struct {
	cfg        *config.AppConfig
	store      store.DocsStore
	users      store.UsersStore
	encryptor  *utils.Encryptor
	audits     store.AuditStore
	logger     *utils.Logger
	converters ConverterStatus
	tempDir    string
}

type SaveRequest struct {
	Doc      *store.Document
	Author   *store.User
	Format   string
	Content  []byte
	Reason   string
	IndexFTS bool
}

type ExportRequest struct {
	Doc      *store.Document
	Version  *store.DocVersion
	Format   string
	Username string
}

type ExportResult struct {
	Data             []byte
	Filename         string
	ContentType      string
	WatermarkApplied bool
}

type jsonExportPayload struct {
	DocID            int64  `json:"doc_id"`
	RegNumber        string `json:"reg_number"`
	Title            string `json:"title"`
	Version          int    `json:"version"`
	SourceFormat     string `json:"source_format"`
	WatermarkApplied bool   `json:"watermark_applied"`
	Content          string `json:"content,omitempty"`
	ContentBase64    string `json:"content_base64,omitempty"`
}

func NewService(cfg *config.AppConfig, ds store.DocsStore, us store.UsersStore, audits store.AuditStore, logger *utils.Logger) (*Service, error) {
	if cfg.Docs.StorageDir == "" {
		cfg.Docs.StorageDir = cfg.Docs.StoragePath
	}
	if cfg.Docs.StorageDir == "" {
		cfg.Docs.StorageDir = "data/docs"
	}
	cfg.Docs.StoragePath = cfg.Docs.StorageDir
	enc, err := utils.NewEncryptorFromString(cfg.Docs.EncryptionKey)
	if err != nil {
		return nil, err
	}
	s := &Service{
		cfg:       cfg,
		store:     ds,
		users:     us,
		encryptor: enc,
		audits:    audits,
		logger:    logger,
		tempDir:   cfg.Docs.Converters.TempDir,
	}
	if s.tempDir == "" {
		s.tempDir = os.TempDir()
	}
	if err := os.MkdirAll(s.tempDir, 0o700); err != nil && s.logger != nil {
		s.logger.Errorf("temp dir init failed: %v", err)
	}
	s.converters = s.checkConverters()
	return s, nil
}

func (s *Service) SaveVersion(ctx context.Context, req SaveRequest) (*store.DocVersion, error) {
	if req.Doc == nil || req.Author == nil {
		return nil, errors.New("missing doc or author")
	}
	nextVer := req.Doc.CurrentVersion + 1
	dir := filepath.Join(s.cfg.Docs.StorageDir, fmt.Sprintf("%d", req.Doc.ID))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	filePath := filepath.Join(dir, fmt.Sprintf("%d.enc", nextVer))
	blob, err := s.encryptor.EncryptToBlob(req.Content)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filePath, blob, 0o600); err != nil {
		return nil, err
	}
	v := &store.DocVersion{
		DocID:          req.Doc.ID,
		Version:        nextVer,
		AuthorID:       req.Author.ID,
		AuthorUsername: req.Author.Username,
		Reason:         req.Reason,
		Path:           filePath,
		Format:         req.Format,
		SizeBytes:      int64(len(req.Content)),
		SHA256Plain:    utils.Sha256Hex(req.Content),
		SHA256Cipher:   utils.Sha256Hex(blob),
		CreatedAt:      time.Now().UTC(),
	}
	if err := s.store.AddVersion(ctx, v); err != nil {
		return nil, err
	}
	req.Doc.CurrentVersion = nextVer
	if s.cfg.Docs.VersionLimit > 0 {
		versions, _ := s.store.ListVersions(ctx, req.Doc.ID)
		if len(versions) > s.cfg.Docs.VersionLimit {
			oldest := versions[len(versions)-1]
			if _, err := s.store.DeleteVersion(ctx, oldest.DocID, oldest.Version); err == nil {
				_ = os.Remove(oldest.Path)
			}
		}
	}
	if req.IndexFTS {
		if err := s.indexContent(ctx, req.Doc.ID, v.Version, req.Format, req.Content); err != nil && s.logger != nil {
			s.logger.Errorf("fts index failed: %v", err)
		}
	}
	return v, nil
}

func (s *Service) LoadContent(ctx context.Context, v *store.DocVersion) ([]byte, error) {
	blob, err := os.ReadFile(v.Path)
	if err != nil {
		return nil, err
	}
	plain, err := s.encryptor.DecryptBlob(blob)
	if err != nil {
		return nil, err
	}
	if utils.Sha256Hex(plain) != v.SHA256Plain {
		return nil, errors.New("integrity check failed")
	}
	if utils.Sha256Hex(blob) != v.SHA256Cipher {
		return nil, errors.New("ciphertext integrity failed")
	}
	return plain, nil
}

func (s *Service) Export(ctx context.Context, req ExportRequest) (*ExportResult, error) {
	content, err := s.LoadContent(ctx, req.Version)
	if err != nil {
		return nil, err
	}
	srcFormat := normalizeFormat(req.Version.Format)
	target := normalizeFormat(req.Format)
	if target == "" {
		target = srcFormat
	}
	needsWM := s.needsWatermark(req.Doc.ClassificationLevel)
	wmText := ""
	if needsWM {
		wmText = s.watermarkString(req.Doc, req.Username)
	}

	var out []byte
	switch target {
	case srcFormat:
		out = content
		if needsWM {
			out = s.applyWatermarkText(srcFormat, out, wmText)
		}
	case FormatMarkdown:
		md, err := s.ensureMarkdown(ctx, srcFormat, content)
		if err != nil {
			return nil, err
		}
		if needsWM {
			md = s.applyWatermarkMarkdown(md, wmText)
		}
		out = md
	case FormatTXT:
		md, err := s.ensureMarkdown(ctx, srcFormat, content)
		if err != nil {
			return nil, err
		}
		if needsWM {
			md = s.applyWatermarkMarkdown(md, wmText)
		}
		out = md
	case FormatDocx:
		if srcFormat == FormatDocx && (!s.converters.Enabled || !s.converters.PandocAvailable) {
			if needsWM {
				return nil, fmt.Errorf("converter required to apply watermark for DOCX")
			}
			out = content
			break
		}
		md, err := s.ensureMarkdown(ctx, srcFormat, content)
		if err != nil {
			return nil, err
		}
		if needsWM {
			md = s.applyWatermarkMarkdown(md, wmText)
		}
		out, err = s.convertFromMarkdown(ctx, FormatDocx, md)
		if err != nil {
			return nil, err
		}
	case FormatPDF:
		data, err := s.exportPDF(ctx, srcFormat, content, needsWM, wmText)
		if err != nil {
			return nil, err
		}
		out = data
	case FormatJSON:
		jsonData := jsonExportPayload{
			DocID:            req.Doc.ID,
			RegNumber:        req.Doc.RegNumber,
			Title:            req.Doc.Title,
			Version:          req.Version.Version,
			SourceFormat:     srcFormat,
			WatermarkApplied: needsWM,
		}
		// Keep text sources human-readable, binary sources lossless.
		switch srcFormat {
		case FormatMarkdown, FormatTXT:
			textContent := content
			if needsWM {
				textContent = s.applyWatermarkText(srcFormat, textContent, wmText)
			}
			jsonData.Content = string(textContent)
		default:
			binaryContent := content
			if needsWM {
				binaryContent = s.applyWatermarkText(srcFormat, binaryContent, wmText)
			}
			jsonData.ContentBase64 = base64.StdEncoding.EncodeToString(binaryContent)
		}
		out, err = json.MarshalIndent(jsonData, "", "  ")
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported export format %s", target)
	}
	filename := fmt.Sprintf("%s.%s", safeFileName(req.Doc.RegNumber), target)
	return &ExportResult{
		Data:             out,
		Filename:         filename,
		ContentType:      contentTypeFor(target),
		WatermarkApplied: needsWM,
	}, nil
}

func (s *Service) exportPDF(ctx context.Context, srcFormat string, content []byte, needsWM bool, wmText string) ([]byte, error) {
	switch srcFormat {
	case FormatMarkdown, FormatTXT:
		md := content
		if needsWM {
			md = s.applyWatermarkMarkdown(md, wmText)
		}
		return s.convertFromMarkdown(ctx, FormatPDF, md)
	case FormatDocx:
		if needsWM {
			md, err := s.convertToMarkdown(ctx, FormatDocx, content)
			if err != nil {
				return nil, err
			}
			md = s.applyWatermarkMarkdown(md, wmText)
			return s.convertFromMarkdown(ctx, FormatPDF, md)
		}
		return s.docxToPDF(ctx, content)
	case FormatPDF:
		if needsWM {
			return nil, errors.New("converter required to apply watermark for PDF")
		}
		return content, nil
	default:
		return nil, fmt.Errorf("pdf export unsupported for %s", srcFormat)
	}
}

func (s *Service) needsWatermark(level int) bool {
	thresholdName := s.cfg.Docs.Watermark.MinLevel
	if thresholdName == "" {
		thresholdName = s.cfg.Docs.WatermarkMinLevel
	}
	if thresholdName == "" {
		thresholdName = "CONFIDENTIAL"
	}
	th, _ := ParseLevel(thresholdName)
	return s.cfg.Docs.Watermark.Enabled && RequiresWatermark(ClassificationLevel(level), th)
}

func (s *Service) WatermarkFor(doc *store.Document, username string) (string, bool) {
	if doc == nil {
		return "", false
	}
	if !s.needsWatermark(doc.ClassificationLevel) {
		return "", false
	}
	return s.watermarkString(doc, username), true
}

func (s *Service) watermarkString(doc *store.Document, username string) string {
	tmpl := s.cfg.Docs.Watermark.TextTemplate
	if tmpl == "" {
		tmpl = "Berkut SCC • {classification} • {username} • {timestamp} • DocNo {reg_no}"
	}
	replacements := map[string]string{
		"{classification}": LevelName(ClassificationLevel(doc.ClassificationLevel)),
		"{username}":       username,
		"{timestamp}":      time.Now().UTC().Format(time.RFC3339),
		"{reg_no}":         doc.RegNumber,
	}
	out := tmpl
	for k, v := range replacements {
		out = strings.ReplaceAll(out, k, v)
	}
	return out
}

func (s *Service) applyWatermarkText(format string, content []byte, watermark string) []byte {
	if watermark == "" {
		return content
	}
	placement := strings.ToLower(strings.TrimSpace(s.cfg.Docs.Watermark.Placement))
	if placement == "" {
		placement = "header"
	}
	switch placement {
	case "footer":
		return append(content, []byte("\n\n"+watermark)...)
	case "both":
		return append([]byte(watermark+"\n\n"), append(content, []byte("\n\n"+watermark)...)...)
	default:
		return append([]byte(watermark+"\n\n"), content...)
	}
}

func (s *Service) applyWatermarkMarkdown(md []byte, watermark string) []byte {
	if watermark == "" {
		return md
	}
	block := fmt.Sprintf("> %s\n\n", watermark)
	return s.applyWatermarkText(FormatMarkdown, append([]byte(block), md...), "")
}

func (s *Service) ensureMarkdown(ctx context.Context, srcFormat string, content []byte) ([]byte, error) {
	switch srcFormat {
	case FormatMarkdown:
		return content, nil
	case FormatTXT:
		return content, nil
	case FormatDocx:
		if !s.converters.Enabled || !s.converters.PandocAvailable {
			return nil, errors.New("docx to markdown requires pandoc (converter disabled or missing)")
		}
		return s.convertToMarkdown(ctx, srcFormat, content)
	default:
		return nil, fmt.Errorf("cannot convert %s to markdown", srcFormat)
	}
}

func (s *Service) convertToMarkdown(ctx context.Context, srcFormat string, content []byte) ([]byte, error) {
	switch srcFormat {
	case FormatMarkdown, FormatTXT:
		return content, nil
	case FormatDocx:
		return s.runPandoc(ctx, srcFormat, "markdown", content)
	default:
		return nil, fmt.Errorf("unsupported source format %s", srcFormat)
	}
}

func (s *Service) convertFromMarkdown(ctx context.Context, target string, content []byte) ([]byte, error) {
	if !s.converters.Enabled || !s.converters.PandocAvailable {
		return nil, errors.New("pandoc converter disabled or missing")
	}
	return s.runPandoc(ctx, "markdown", target, content)
}

func (s *Service) docxToPDF(ctx context.Context, content []byte) ([]byte, error) {
	if !s.converters.Enabled || !s.converters.SofficeAvailable {
		return nil, errors.New("soffice converter disabled or missing")
	}
	tmpIn, err := os.CreateTemp(s.tempDir, "berkut-docx-*.docx")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpIn.Name())
	if _, err := tmpIn.Write(content); err != nil {
		return nil, err
	}
	tmpIn.Close()
	base := strings.TrimSuffix(tmpIn.Name(), filepath.Ext(tmpIn.Name()))
	tmpOut := base + ".pdf"
	args := []string{"--headless", "--convert-to", "pdf", "--outdir", filepath.Dir(tmpIn.Name()), tmpIn.Name()}
	if err := s.runCommand(ctx, s.cfg.Docs.Converters.SofficePath, args...); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(tmpOut)
	_ = os.Remove(tmpOut)
	return data, err
}

func (s *Service) runPandoc(ctx context.Context, from, to string, content []byte) ([]byte, error) {
	var cancel context.CancelFunc
	if s.cfg.Docs.Converters.TimeoutSec > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(s.cfg.Docs.Converters.TimeoutSec)*time.Second)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, s.cfg.Docs.Converters.PandocPath, "-f", from, "-t", to)
	cmd.Stdin = bytes.NewReader(content)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("conversion timeout exceeded")
	}
	if err != nil {
		return nil, fmt.Errorf("pandoc convert failed: %v: %s", err, string(out))
	}
	return out, nil
}

func (s *Service) runCommand(ctx context.Context, bin string, args ...string) error {
	var cancel context.CancelFunc
	if s.cfg.Docs.Converters.TimeoutSec > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(s.cfg.Docs.Converters.TimeoutSec)*time.Second)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("converter timeout")
		}
		return fmt.Errorf("%s failed: %v: %s", filepath.Base(bin), err, string(out))
	}
	return nil
}

func (s *Service) CopyLatestContent(ctx context.Context, doc *store.Document) ([]byte, *store.DocVersion, error) {
	v, err := s.store.GetVersion(ctx, doc.ID, doc.CurrentVersion)
	if err != nil {
		return nil, nil, err
	}
	if v == nil {
		return nil, nil, errors.New("no version found")
	}
	content, err := s.LoadContent(ctx, v)
	return content, v, err
}

func safeFileName(name string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_")
	return replacer.Replace(name)
}

func (s *Service) CheckACL(user *store.User, roles []string, doc *store.Document, docACL []store.ACLRule, folderACL []store.ACLRule, required string) bool {
	if user == nil || doc == nil {
		return false
	}
	isPrivileged := false
	for _, r := range roles {
		if r == "superadmin" || r == "admin" {
			isPrivileged = true
			break
		}
	}
	if !isPrivileged && !HasClearance(ClassificationLevel(user.ClearanceLevel), user.ClearanceTags, ClassificationLevel(doc.ClassificationLevel), doc.ClassificationTags) {
		return false
	}
	if len(docACL) == 0 && doc.InheritACL {
		docACL = append(docACL, folderACL...)
	}
	if doc.CreatedBy == user.ID {
		return true
	}
	for _, a := range docACL {
		if a.Permission != required {
			continue
		}
		switch strings.ToLower(a.SubjectType) {
		case "user":
			if a.SubjectID == user.Username || a.SubjectID == fmt.Sprintf("%d", user.ID) {
				return true
			}
		case "role":
			for _, r := range roles {
				if r == a.SubjectID {
					return true
				}
			}
		}
	}
	// allow owners/managers to perform privileged actions even if explicit rule is missing
	if required != "manage" {
		for _, a := range docACL {
			if a.Permission != "manage" {
				continue
			}
			switch strings.ToLower(a.SubjectType) {
			case "user":
				if a.SubjectID == user.Username || a.SubjectID == fmt.Sprintf("%d", user.ID) {
					return true
				}
			case "role":
				for _, r := range roles {
					if r == a.SubjectID {
						return true
					}
				}
			}
		}
	}
	return false
}

func (s *Service) Log(ctx context.Context, username, action, details string) {
	if s.audits != nil {
		_ = s.audits.Log(ctx, username, action, details)
	}
}

func (s *Service) StreamFile(ctx context.Context, w io.Writer, v *store.DocVersion) error {
	content, err := s.LoadContent(ctx, v)
	if err != nil {
		return err
	}
	_, err = w.Write(content)
	return err
}

func (s *Service) checkConverters() ConverterStatus {
	status := ConverterStatus{Enabled: s.cfg.Docs.Converters.Enabled}
	if !status.Enabled {
		status.Message = "converters disabled"
		return status
	}
	if path, err := exec.LookPath(s.cfg.Docs.Converters.PandocPath); err == nil {
		s.cfg.Docs.Converters.PandocPath = path
		status.PandocAvailable = true
	} else {
		status.Message = "pandoc missing"
	}
	if path, err := exec.LookPath(s.cfg.Docs.Converters.SofficePath); err == nil {
		s.cfg.Docs.Converters.SofficePath = path
		status.SofficeAvailable = true
	} else if status.Message == "" {
		status.Message = "soffice missing"
	}
	if !status.PandocAvailable && !status.SofficeAvailable {
		status.Enabled = false
		if s.logger != nil {
			s.logger.Errorf("converters enabled in config but binaries missing: %s", status.Message)
		}
	}
	return status
}

func (s *Service) ConvertersStatus() ConverterStatus {
	return s.converters
}

func (s *Service) ConvertMarkdown(ctx context.Context, target string, md []byte, watermark string) ([]byte, string, error) {
	t := normalizeFormat(target)
	if t == "" || t == FormatMarkdown {
		if watermark != "" {
			md = s.applyWatermarkMarkdown(md, watermark)
		}
		return md, contentTypeFor(FormatMarkdown), nil
	}
	if t == FormatTXT {
		if watermark != "" {
			md = s.applyWatermarkMarkdown(md, watermark)
		}
		return md, contentTypeFor(FormatTXT), nil
	}
	if t == FormatDocx {
		if watermark != "" {
			md = s.applyWatermarkMarkdown(md, watermark)
		}
		out, err := s.convertFromMarkdown(ctx, FormatDocx, md)
		return out, contentTypeFor(FormatDocx), err
	}
	if t == FormatPDF {
		if watermark != "" {
			md = s.applyWatermarkMarkdown(md, watermark)
		}
		out, err := s.convertFromMarkdown(ctx, FormatPDF, md)
		return out, contentTypeFor(FormatPDF), err
	}
	if t == FormatJSON {
		payload := jsonExportPayload{
			SourceFormat:     FormatMarkdown,
			WatermarkApplied: watermark != "",
			Content:          string(md),
		}
		if watermark != "" {
			payload.Content = string(s.applyWatermarkMarkdown(md, watermark))
		}
		out, err := json.MarshalIndent(payload, "", "  ")
		return out, contentTypeFor(FormatJSON), err
	}
	return nil, "", fmt.Errorf("unsupported export format %s", t)
}

func (s *Service) indexContent(ctx context.Context, docID int64, ver int, format string, content []byte) error {
	format = normalizeFormat(format)
	switch format {
	case FormatMarkdown, FormatTXT:
		return s.store.UpsertFTS(ctx, docID, ver, string(content))
	case FormatDocx:
		md, err := s.convertToMarkdown(ctx, FormatDocx, content)
		if err != nil {
			return err
		}
		return s.store.UpsertFTS(ctx, docID, ver, string(md))
	default:
		return errors.New("format not supported for indexing")
	}
}

func normalizeFormat(f string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(f), "."))
}

func contentTypeFor(format string) string {
	switch format {
	case FormatDocx:
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case FormatPDF:
		return "application/pdf"
	case FormatTXT:
		return "text/plain; charset=utf-8"
	case FormatJSON:
		return "application/json; charset=utf-8"
	default:
		return "text/markdown; charset=utf-8"
	}
}
