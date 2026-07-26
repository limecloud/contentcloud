package localworkspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/ingest"
)

const localSourceSizeLimit = 100 << 20

var localSourceIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9:._-]{0,199}$`)

type SourceRegistry struct {
	SchemaVersion string        `json:"schema_version"`
	Sources       []LocalSource `json:"sources"`
}

type LocalSource struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	FilePath     string     `json:"file_path"`
	SHA256       string     `json:"sha256"`
	MIMEType     string     `json:"mime_type"`
	SourceKind   string     `json:"source_kind"`
	ByteSize     int64      `json:"byte_size"`
	StorageMode  string     `json:"storage_mode"`
	IngestStatus string     `json:"ingest_status"`
	EvidencePath string     `json:"evidence_path,omitempty"`
	RegisteredAt time.Time  `json:"registered_at"`
	IngestedAt   *time.Time `json:"ingested_at,omitempty"`
}

type LocalEvidenceBundle struct {
	SchemaVersion string          `json:"schema_version"`
	SourceID      string          `json:"source_id"`
	SourceSHA256  string          `json:"source_sha256"`
	MIMEType      string          `json:"mime_type"`
	ParserVersion string          `json:"parser_version"`
	Status        string          `json:"status"`
	ErrorCode     string          `json:"error_code,omitempty"`
	Evidence      []LocalEvidence `json:"evidence"`
	CreatedAt     time.Time       `json:"created_at"`
}

type LocalEvidence struct {
	ID            string         `json:"id"`
	SourceID      string         `json:"source_id"`
	LocatorKind   string         `json:"locator_kind"`
	Locator       map[string]any `json:"locator"`
	Quote         string         `json:"quote"`
	QuoteHash     string         `json:"quote_hash"`
	OCRConfidence *float64       `json:"ocr_confidence,omitempty"`
	ReviewStatus  string         `json:"review_status"`
}

type RegisterLocalSourceOptions struct {
	Root        string
	File        string
	ID          string
	Title       string
	SourceKind  string
	StorageMode string
	Now         time.Time
}

type SourceVerification struct {
	Valid    bool          `json:"valid"`
	Count    int           `json:"count"`
	Results  []SourceCheck `json:"results"`
	Warnings []string      `json:"warnings"`
}

type SourceCheck struct {
	ID           string `json:"id"`
	FilePath     string `json:"file_path"`
	Exists       bool   `json:"exists"`
	HashMatches  bool   `json:"hash_matches"`
	MIMEMatches  bool   `json:"mime_matches"`
	ActualSHA256 string `json:"actual_sha256,omitempty"`
	ActualMIME   string `json:"actual_mime,omitempty"`
}

func RegisterLocalSource(options RegisterLocalSourceOptions) (LocalSource, error) {
	root, err := FindRoot(options.Root)
	if err != nil {
		return LocalSource{}, err
	}
	absolute, err := filepath.Abs(options.File)
	if err != nil {
		return LocalSource{}, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return LocalSource{}, err
	}
	if info.IsDir() || info.Size() <= 0 || info.Size() > localSourceSizeLimit {
		return LocalSource{}, domain.Invalid("LOCAL_SOURCE_SIZE_INVALID", "本地来源必须是 1B 到 100MB 的文件")
	}
	body, err := os.ReadFile(absolute)
	if err != nil {
		return LocalSource{}, err
	}
	hash := digest(body)
	mimeType := ingest.DetectMIME(body)
	mode := strings.ToLower(strings.TrimSpace(options.StorageMode))
	if mode == "" {
		mode = "copy"
	}
	if mode != "copy" && mode != "reference" {
		return LocalSource{}, domain.Invalid("LOCAL_SOURCE_STORAGE_MODE_INVALID", "storage mode 只允许 copy 或 reference")
	}
	id := strings.TrimSpace(options.ID)
	if id == "" {
		id = defaultLocalSourceID(filepath.Base(absolute), hash)
	}
	if !localSourceIDPattern.MatchString(id) {
		return LocalSource{}, domain.Invalid("LOCAL_SOURCE_ID_INVALID", "来源 ID 只能包含字母、数字、冒号、点、下划线和连字符")
	}
	registry, err := loadSourceRegistry(root)
	if err != nil {
		return LocalSource{}, err
	}
	for _, existing := range registry.Sources {
		if existing.ID == id {
			if existing.SHA256 == hash {
				return existing, nil
			}
			return LocalSource{}, domain.Conflict("LOCAL_SOURCE_ID_CONFLICT", "相同来源 ID 已绑定不同内容；请使用新的稳定 ID")
		}
		if existing.SHA256 == hash {
			return LocalSource{}, domain.Conflict("LOCAL_SOURCE_DUPLICATE", "相同内容已登记为 "+existing.ID)
		}
	}
	storedPath := absolute
	if mode == "copy" {
		destination := filepath.Join(root, "raw", "inbox", hash[:12]+"-"+filepath.Base(absolute))
		if filepath.Clean(destination) != filepath.Clean(absolute) {
			if existing, readErr := os.ReadFile(destination); readErr == nil {
				if digest(existing) != hash {
					return LocalSource{}, domain.Conflict("LOCAL_SOURCE_COPY_CONFLICT", "raw/inbox 中目标文件内容不同")
				}
			} else if !errors.Is(readErr, os.ErrNotExist) {
				return LocalSource{}, readErr
			} else if err := writeNewFile(destination, body); err != nil {
				return LocalSource{}, err
			}
		}
		storedPath = destination
	}
	relative, err := filepath.Rel(root, storedPath)
	if err != nil {
		return LocalSource{}, err
	}
	now := localNow(options.Now)
	value := LocalSource{
		ID: id, Title: defaultLocalValue(options.Title, filepath.Base(absolute)), FilePath: filepath.ToSlash(relative), SHA256: hash, MIMEType: mimeType,
		SourceKind: defaultLocalValue(options.SourceKind, "customer_material"), ByteSize: info.Size(), StorageMode: mode, IngestStatus: "registered", RegisteredAt: now,
	}
	registry.Sources = append(registry.Sources, value)
	sort.Slice(registry.Sources, func(i, j int) bool { return registry.Sources[i].ID < registry.Sources[j].ID })
	if err := saveSourceRegistry(root, registry); err != nil {
		return LocalSource{}, err
	}
	return value, nil
}

func LocalSources(root string) ([]LocalSource, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return nil, err
	}
	registry, err := loadSourceRegistry(resolved)
	if err != nil {
		return nil, err
	}
	return append([]LocalSource(nil), registry.Sources...), nil
}

func LocalSourceByID(root, id string) (LocalSource, error) {
	values, err := LocalSources(root)
	if err != nil {
		return LocalSource{}, err
	}
	for _, value := range values {
		if value.ID == id {
			return value, nil
		}
	}
	return LocalSource{}, domain.NotFound("本地来源")
}

func IngestLocalSource(root, id string, now time.Time) (LocalEvidenceBundle, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return LocalEvidenceBundle{}, err
	}
	registry, err := loadSourceRegistry(resolved)
	if err != nil {
		return LocalEvidenceBundle{}, err
	}
	index := -1
	for i := range registry.Sources {
		if registry.Sources[i].ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return LocalEvidenceBundle{}, domain.NotFound("本地来源")
	}
	source := registry.Sources[index]
	absolute := resolveLocalSourcePath(resolved, source.FilePath)
	body, err := os.ReadFile(absolute)
	if err != nil {
		return LocalEvidenceBundle{}, err
	}
	if len(body) == 0 || len(body) > localSourceSizeLimit {
		return LocalEvidenceBundle{}, domain.Invalid("LOCAL_SOURCE_SIZE_INVALID", "本地来源必须是 1B 到 100MB 的文件")
	}
	if digest(body) != source.SHA256 {
		return LocalEvidenceBundle{}, domain.Conflict("LOCAL_SOURCE_HASH_MISMATCH", "来源文件已变化；必须登记为新的不可变来源")
	}
	detected := ingest.DetectMIME(body)
	if detected != source.MIMEType {
		return LocalEvidenceBundle{}, domain.Conflict("LOCAL_SOURCE_MIME_MISMATCH", "来源文件 MIME 与登记值不一致")
	}
	parsed := ingest.Parse(filepath.Base(absolute), detected, body)
	createdAt := localNow(now)
	evidence := make([]LocalEvidence, 0, len(parsed.Evidence))
	for position, span := range parsed.Evidence {
		quote := strings.TrimSpace(span.QuoteText)
		if quote == "" {
			continue
		}
		quoteSum := sha256.Sum256([]byte(quote))
		reviewStatus := "accepted"
		if parsed.Status != "ready" || (span.OCRConfidence != nil && *span.OCRConfidence < 0.85) {
			reviewStatus = "needs_review"
		}
		evidence = append(evidence, LocalEvidence{
			ID: localEvidenceID(source.ID, position+1), SourceID: source.ID, LocatorKind: span.LocatorKind, Locator: span.Locator, Quote: quote,
			QuoteHash: hex.EncodeToString(quoteSum[:]), OCRConfidence: span.OCRConfidence, ReviewStatus: reviewStatus,
		})
	}
	bundle := LocalEvidenceBundle{
		SchemaVersion: SchemaVersion, SourceID: source.ID, SourceSHA256: source.SHA256, MIMEType: detected, ParserVersion: ingest.ParserVersion,
		Status: parsed.Status, ErrorCode: parsed.ErrorCode, Evidence: evidence, CreatedAt: createdAt,
	}
	evidenceRelative := filepath.ToSlash(filepath.Join("knowledge", "evidence", localSafeName(source.ID)+".json"))
	if err := replaceJSON(filepath.Join(resolved, filepath.FromSlash(evidenceRelative)), bundle, 0o600); err != nil {
		return LocalEvidenceBundle{}, err
	}
	registry.Sources[index].IngestStatus = parsed.Status
	registry.Sources[index].EvidencePath = evidenceRelative
	registry.Sources[index].IngestedAt = &createdAt
	if err := saveSourceRegistry(resolved, registry); err != nil {
		return LocalEvidenceBundle{}, err
	}
	return bundle, nil
}

func VerifyLocalSources(root string) (SourceVerification, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return SourceVerification{}, err
	}
	registry, err := loadSourceRegistry(resolved)
	if err != nil {
		return SourceVerification{}, err
	}
	report := SourceVerification{Valid: true, Count: len(registry.Sources), Results: []SourceCheck{}, Warnings: []string{}}
	for _, source := range registry.Sources {
		check := SourceCheck{ID: source.ID, FilePath: source.FilePath}
		body, readErr := os.ReadFile(resolveLocalSourcePath(resolved, source.FilePath))
		if readErr == nil {
			check.Exists = true
			check.ActualSHA256 = digest(body)
			check.ActualMIME = ingest.DetectMIME(body)
			check.HashMatches = check.ActualSHA256 == source.SHA256
			check.MIMEMatches = check.ActualMIME == source.MIMEType
		}
		if !check.Exists || !check.HashMatches || !check.MIMEMatches {
			report.Valid = false
		}
		report.Results = append(report.Results, check)
	}
	return report, nil
}

func loadSourceRegistry(root string) (SourceRegistry, error) {
	var registry SourceRegistry
	path := filepath.Join(root, "raw", "source-registry.yaml")
	if err := readJSON(path, &registry); err != nil {
		return registry, domain.Invalid("LOCAL_SOURCE_REGISTRY_INVALID", "source-registry.yaml 必须使用模板提供的 JSON/YAML 子集格式")
	}
	if registry.SchemaVersion == "" {
		registry.SchemaVersion = SchemaVersion
	}
	if registry.SchemaVersion != SchemaVersion {
		return registry, domain.Conflict("LOCAL_SOURCE_REGISTRY_VERSION_UNSUPPORTED", "source registry schema version 不受支持")
	}
	if registry.Sources == nil {
		registry.Sources = []LocalSource{}
	}
	return registry, nil
}

func saveSourceRegistry(root string, registry SourceRegistry) error {
	registry.SchemaVersion = SchemaVersion
	return replaceJSON(filepath.Join(root, "raw", "source-registry.yaml"), registry, 0o600)
}

func resolveLocalSourcePath(root, stored string) string {
	if filepath.IsAbs(stored) {
		return filepath.Clean(stored)
	}
	return filepath.Clean(filepath.Join(root, filepath.FromSlash(stored)))
}

func defaultLocalSourceID(name, hash string) string {
	base := strings.TrimSuffix(strings.ToLower(name), strings.ToLower(filepath.Ext(name)))
	var builder strings.Builder
	lastDash := false
	for _, char := range base {
		valid := (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9')
		if valid {
			builder.WriteRune(char)
			lastDash = false
		} else if builder.Len() > 0 && !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(builder.String(), "-")
	if slug == "" {
		slug = "material"
	}
	return "source:" + slug + "-" + hash[:8]
}

func localEvidenceID(sourceID string, position int) string {
	return "evidence:" + localSafeName(strings.TrimPrefix(sourceID, "source:")) + ":" + fmtInt(position, 4)
}

func localSafeName(value string) string {
	var builder strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			builder.WriteRune(char)
		} else {
			builder.WriteByte('-')
		}
	}
	value = strings.Trim(builder.String(), "-.")
	if value == "" {
		return "item"
	}
	return value
}

func fmtInt(value, width int) string {
	result := strconv.Itoa(value)
	if len(result) >= width {
		return result
	}
	return strings.Repeat("0", width-len(result)) + result
}

func localNow(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC()
}

func defaultLocalValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
