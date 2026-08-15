package localworkspace

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/ingest"
	"gopkg.in/yaml.v3"
)

const (
	WorkspaceViewSchema           = "contentcloud.workspace-view/1.0"
	workspaceViewMaxBytes   int64 = 2 * 1024 * 1024
	workspaceMediaMaxBytes  int64 = 512 * 1024 * 1024
	workspaceMIMESniffBytes       = 512
)

var workspaceViewRoots = map[string]bool{
	"10-context": true, "20-sources": true, "30-knowledge": true,
	"40-work": true, "50-production": true, "60-delivery": true,
	"70-results": true, "90-archive": true,
}

type WorkspaceViewOptions struct {
	Root                    string
	View                    string
	Ref                     string
	RunID                   string
	ExpectedContextRevision uint64
	ExpectedDigest          string
	Now                     time.Time
}

type WorkspaceView struct {
	SchemaVersion   string                 `json:"schema_version"`
	WorkspaceID     string                 `json:"workspace_id"`
	ProjectID       string                 `json:"project_id"`
	RunID           string                 `json:"run_id,omitempty"`
	ContextRevision uint64                 `json:"context_revision,omitempty"`
	ObservedDigest  string                 `json:"observed_digest,omitempty"`
	View            WorkspaceViewBody      `json:"view"`
	Resources       []WorkspaceResourceRef `json:"resources"`
	Offline         bool                   `json:"offline"`
}

type WorkspaceViewBody struct {
	Kind      string                `json:"kind"`
	Title     string                `json:"title"`
	Summary   string                `json:"summary"`
	Ref       string                `json:"ref,omitempty"`
	MIMEType  string                `json:"mime_type,omitempty"`
	ByteSize  int64                 `json:"byte_size,omitempty"`
	Truncated bool                  `json:"truncated"`
	Data      any                   `json:"data,omitempty"`
	Text      string                `json:"text,omitempty"`
	Checks    []WorkspaceViewCheck  `json:"checks"`
	Actions   []WorkspaceViewAction `json:"actions"`
}

type WorkspaceViewCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type WorkspaceViewAction struct {
	ID                   string         `json:"id"`
	Title                string         `json:"title"`
	Tool                 string         `json:"tool"`
	Arguments            map[string]any `json:"arguments"`
	RequiresClaim        bool           `json:"requires_claim"`
	RequiresConfirmation bool           `json:"requires_confirmation"`
}

type WorkspaceDirectoryEntry struct {
	Ref      string `json:"ref"`
	Kind     string `json:"kind"`
	ByteSize int64  `json:"byte_size,omitempty"`
	MIMEType string `json:"mime_type,omitempty"`
}

type WorkspaceResourceRef struct {
	URI      string `json:"uri"`
	Name     string `json:"name"`
	MIMEType string `json:"mime_type"`
	Digest   string `json:"digest"`
	ByteSize int64  `json:"byte_size"`
}

type WorkspaceResource struct {
	URI      string
	MIMEType string
	Text     string
	Blob     []byte
}

type ReadSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}

type WorkspaceResourceStream struct {
	URI      string
	Ref      string
	MIMEType string
	Digest   string
	ByteSize int64
	Reader   ReadSeekCloser
}

func BuildWorkspaceView(options WorkspaceViewOptions) (WorkspaceView, error) {
	root, err := FindRoot(options.Root)
	if err != nil {
		return WorkspaceView{}, err
	}
	status, err := LoadStatus(root)
	if err != nil {
		return WorkspaceView{}, err
	}
	kind := strings.TrimSpace(options.View)
	if kind == "" {
		kind = "file"
	}
	if !validWorkspaceViewKind(kind) {
		return WorkspaceView{}, domain.Invalid("WORKSPACE_VIEW_KIND_INVALID", "view 必须是 workspace_summary、file、run、handoff、content_item、render、diff 或 delivery")
	}
	result := WorkspaceView{
		SchemaVersion: WorkspaceViewSchema,
		WorkspaceID:   status.Binding.WorkspaceID,
		ProjectID:     status.Binding.ProjectID,
		Resources:     []WorkspaceResourceRef{},
		Offline:       true,
	}
	if strings.TrimSpace(options.RunID) != "" && kind != "run" {
		run, runErr := loadLocalRun(root, strings.TrimSpace(options.RunID))
		if runErr != nil {
			return WorkspaceView{}, runErr
		}
		if err := verifyWorkspaceViewRevision(run.ContextRevision, options.ExpectedContextRevision); err != nil {
			return WorkspaceView{}, err
		}
		result.RunID = run.RunID
		result.ContextRevision = run.ContextRevision
	}
	if kind == "workspace_summary" {
		context, contextErr := ConversationContext(root, root, localNow(options.Now))
		if contextErr != nil {
			return WorkspaceView{}, contextErr
		}
		context.Root = ""
		result.View = WorkspaceViewBody{Kind: kind, Title: "Content Work OS 本地工作区", Summary: fmt.Sprintf("%d 个活动 Run，%d 个待接手 Handoff", len(context.ActiveRuns), len(context.ReadyHandoffs)), Data: context, Checks: []WorkspaceViewCheck{{Name: "workspace_binding", Status: "passed"}}, Actions: []WorkspaceViewAction{}}
		return result, nil
	}
	if kind == "run" && strings.TrimSpace(options.Ref) == "" {
		runID := strings.TrimSpace(options.RunID)
		if runID == "" {
			return WorkspaceView{}, domain.Invalid("WORKSPACE_VIEW_RUN_REQUIRED", "run 视图需要 run_id 或 ref")
		}
		run, runErr := ShowLocalRun(root, runID)
		if runErr != nil {
			return WorkspaceView{}, runErr
		}
		if err := verifyWorkspaceViewRevision(run.ContextRevision, options.ExpectedContextRevision); err != nil {
			return WorkspaceView{}, err
		}
		body, _ := json.Marshal(run)
		result.RunID = run.RunID
		result.ContextRevision = run.ContextRevision
		result.ObservedDigest = workspaceDigest(body)
		result.View = WorkspaceViewBody{Kind: kind, Title: run.RunID, Summary: run.Intent + " / " + run.Stage + " / " + run.Status, Data: run, Checks: []WorkspaceViewCheck{{Name: "context_revision", Status: "passed"}}, Actions: []WorkspaceViewAction{}}
		return result, nil
	}
	if kind == "handoff" && strings.TrimSpace(options.Ref) == "" {
		return WorkspaceView{}, domain.Invalid("WORKSPACE_VIEW_REF_REQUIRED", "handoff 视图需要 40-work/handoffs 下的 ref")
	}
	if directory, directoryErr := readWorkspaceViewDirectory(root, options.Ref); directoryErr == nil {
		result.View = WorkspaceViewBody{
			Kind: kind, Title: filepath.Base(strings.TrimSuffix(filepath.ToSlash(options.Ref), "/")),
			Summary: fmt.Sprintf("%d 个允许展示的直接子项", len(directory)), Ref: filepath.ToSlash(options.Ref), Data: directory,
			Checks: []WorkspaceViewCheck{{Name: "workspace_path", Status: "passed"}}, Actions: []WorkspaceViewAction{},
		}
		return result, nil
	} else if !workspaceViewErrorCode(directoryErr, "WORKSPACE_VIEW_NOT_DIRECTORY") {
		return WorkspaceView{}, directoryErr
	}
	file, err := readWorkspaceViewFile(root, options.Ref)
	if err != nil {
		return WorkspaceView{}, err
	}
	if err := verifyWorkspaceViewDigest(file.Digest, options.ExpectedDigest); err != nil {
		return WorkspaceView{}, err
	}
	var data any
	text := ""
	if strings.Contains(file.MIMEType, "json") {
		if err := json.Unmarshal(file.Body, &data); err != nil {
			return WorkspaceView{}, domain.Invalid("WORKSPACE_VIEW_DOCUMENT_INVALID", "JSON 文件无法解析为类型化视图")
		}
	} else if strings.HasSuffix(file.Ref, ".yaml") || strings.HasSuffix(file.Ref, ".yml") {
		var value any
		if err := yaml.Unmarshal(file.Body, &value); err != nil {
			return WorkspaceView{}, domain.Invalid("WORKSPACE_VIEW_DOCUMENT_INVALID", "YAML 文件无法解析为类型化视图")
		}
		body, marshalErr := json.Marshal(value)
		if marshalErr != nil || json.Unmarshal(body, &data) != nil {
			return WorkspaceView{}, domain.Invalid("WORKSPACE_VIEW_DOCUMENT_INVALID", "YAML 文件无法转换为安全结构化内容")
		}
	} else if strings.HasPrefix(file.MIMEType, "text/") {
		text = string(file.Body)
	}
	result.ObservedDigest = file.Digest
	result.View = WorkspaceViewBody{
		Kind: kind, Title: filepath.Base(file.Ref), Summary: workspaceViewSummary(kind, file), Ref: file.Ref,
		MIMEType: file.MIMEType, ByteSize: file.Size, Truncated: false, Data: data, Text: text,
		Checks:  []WorkspaceViewCheck{{Name: "workspace_path", Status: "passed"}, {Name: "content_digest", Status: "passed"}},
		Actions: []WorkspaceViewAction{},
	}
	result.Resources = append(result.Resources, WorkspaceResourceRef{URI: WorkspaceFileResourceURI(file.Ref, file.Digest), Name: filepath.Base(file.Ref), MIMEType: file.MIMEType, Digest: file.Digest, ByteSize: file.Size})
	return result, nil
}

func ReadWorkspaceResource(root, uri string) (WorkspaceResource, error) {
	stream, err := OpenWorkspaceResource(root, uri)
	if err != nil {
		return WorkspaceResource{}, err
	}
	defer stream.Reader.Close()
	if stream.ByteSize > workspaceViewMaxBytes {
		policy := domain.Policy("MCP_RESOURCE_TOO_LARGE", "资源超过 MCP 内联读取大小上限", "通过本地 Workbench 使用 Range 流式读取该媒体资源")
		policy.Details = map[string]any{"byte_size": stream.ByteSize, "max_bytes": workspaceViewMaxBytes}
		return WorkspaceResource{}, policy
	}
	body, err := io.ReadAll(io.LimitReader(stream.Reader, workspaceViewMaxBytes+1))
	if err != nil {
		return WorkspaceResource{}, err
	}
	return resourceFromBody(uri, stream.MIMEType, body), nil
}

func OpenWorkspaceResource(root, uri string) (WorkspaceResourceStream, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return WorkspaceResourceStream{}, err
	}
	ref, expected, err := parseWorkspaceFileResourceURI(uri)
	if err != nil {
		return WorkspaceResourceStream{}, err
	}
	metadata, err := readWorkspaceViewFile(resolved, ref)
	if err != nil {
		return WorkspaceResourceStream{}, err
	}
	if err := verifyWorkspaceViewDigest(metadata.Digest, expected); err != nil {
		return WorkspaceResourceStream{}, err
	}
	reader, err := os.Open(metadata.Path)
	if err != nil {
		return WorkspaceResourceStream{}, err
	}
	info, err := reader.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != metadata.Size {
		reader.Close()
		return WorkspaceResourceStream{}, domain.Conflict("WORKSPACE_VIEW_STALE", "资源在打开过程中发生变化")
	}
	// Hash the descriptor's already-open file descriptor. A path-only size
	// check is insufficient when a file is atomically replaced with content of
	// the same length between descriptor creation and resource serving.
	hash := sha256.New()
	if _, err := io.Copy(hash, reader); err != nil {
		reader.Close()
		return WorkspaceResourceStream{}, err
	}
	actual := "sha256:" + hex.EncodeToString(hash.Sum(nil))
	if actual != metadata.Digest {
		reader.Close()
		return WorkspaceResourceStream{}, domain.Conflict("WORKSPACE_VIEW_STALE", "资源在打开过程中发生变化")
	}
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		reader.Close()
		return WorkspaceResourceStream{}, err
	}
	return WorkspaceResourceStream{URI: uri, Ref: metadata.Ref, MIMEType: metadata.MIMEType, Digest: metadata.Digest, ByteSize: metadata.Size, Reader: reader}, nil
}

func WorkspaceFileResourceURI(ref, digest string) string {
	parts := strings.Split(filepath.ToSlash(ref), "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}
	return "contentcloud://workspace/files/" + strings.Join(parts, "/") + "?digest=" + url.QueryEscape(strings.TrimPrefix(digest, "sha256:"))
}

func parseWorkspaceFileResourceURI(uri string) (string, string, error) {
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "contentcloud" || parsed.Host != "workspace" {
		return "", "", domain.Invalid("MCP_RESOURCE_URI_INVALID", "本地工作台资源 URI 无效")
	}
	parts := strings.Split(strings.TrimPrefix(parsed.EscapedPath(), "/"), "/")
	if len(parts) < 2 || parts[0] != "files" || parsed.Query().Get("digest") == "" {
		return "", "", domain.Invalid("MCP_RESOURCE_URI_INVALID", "本地文件资源 URI 必须包含 files 路径和 digest")
	}
	decoded := make([]string, 0, len(parts)-1)
	for _, part := range parts[1:] {
		value, decodeErr := url.PathUnescape(part)
		if decodeErr != nil || value == "" || value == "." || value == ".." || strings.ContainsAny(value, "/\\") {
			return "", "", domain.Invalid("MCP_RESOURCE_URI_INVALID", "本地文件资源路径无效")
		}
		decoded = append(decoded, value)
	}
	return strings.Join(decoded, "/"), parsed.Query().Get("digest"), nil
}

type workspaceViewFile struct {
	Ref      string
	Path     string
	Body     []byte
	Digest   string
	MIMEType string
	Size     int64
}

func readWorkspaceViewFile(root, raw string) (workspaceViewFile, error) {
	ref, err := normalizeWorkspaceViewRef(raw)
	if err != nil {
		return workspaceViewFile{}, err
	}
	path, err := ResolveWorkspaceFile(root, ref)
	if err != nil {
		return workspaceViewFile{}, err
	}
	reader, err := os.Open(path)
	if err != nil {
		return workspaceViewFile{}, err
	}
	defer reader.Close()
	info, err := reader.Stat()
	if err != nil {
		return workspaceViewFile{}, err
	}
	if !info.Mode().IsRegular() {
		return workspaceViewFile{}, domain.Policy("WORKSPACE_VIEW_FILE_TYPE_DENIED", "本地展示只允许普通文件", "选择普通 Workspace 文件")
	}
	if info.Size() > workspaceMediaMaxBytes {
		return workspaceViewFile{}, workspaceFileTooLarge(info.Size(), workspaceMediaMaxBytes)
	}
	sample := make([]byte, minInt64(info.Size(), workspaceMIMESniffBytes))
	if _, err := io.ReadFull(reader, sample); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return workspaceViewFile{}, err
	}
	mediaType := detectWorkspaceMIME(ref, sample)
	if mediaType == "application/octet-stream" {
		return workspaceViewFile{}, domain.Policy("WORKSPACE_VIEW_MIME_UNSUPPORTED", "文件类型不能安全地通过本地工作台展示", "使用受支持的文本、JSON、YAML、图片、音频、视频或 PDF 文件")
	}
	if info.Size() > workspaceViewMaxBytes && !streamableWorkspaceMIME(mediaType) {
		return workspaceViewFile{}, workspaceFileTooLarge(info.Size(), workspaceViewMaxBytes)
	}
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return workspaceViewFile{}, err
	}
	var body []byte
	hash := sha256.New()
	if info.Size() <= workspaceViewMaxBytes {
		body, err = io.ReadAll(io.TeeReader(io.LimitReader(reader, workspaceViewMaxBytes+1), hash))
	} else {
		_, err = io.Copy(hash, reader)
	}
	if err != nil {
		return workspaceViewFile{}, err
	}
	return workspaceViewFile{Ref: ref, Path: path, Body: body, Digest: "sha256:" + hex.EncodeToString(hash.Sum(nil)), MIMEType: mediaType, Size: info.Size()}, nil
}

func readWorkspaceViewDirectory(root, raw string) ([]WorkspaceDirectoryEntry, error) {
	ref, err := normalizeWorkspaceViewRef(raw)
	if err != nil {
		return nil, err
	}
	path, err := ResolveWorkspaceFile(root, ref)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return nil, domain.Invalid("WORKSPACE_VIEW_NOT_DIRECTORY", "ref 不是可浏览目录")
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	if len(entries) > 200 {
		return nil, domain.Policy("WORKSPACE_VIEW_DIRECTORY_TOO_LARGE", "目录直接子项超过展示上限", "选择更具体的子目录")
	}
	result := make([]WorkspaceDirectoryEntry, 0, len(entries))
	for _, entry := range entries {
		if deniedWorkspaceViewSegment(entry.Name()) {
			continue
		}
		entryInfo, infoErr := entry.Info()
		if infoErr != nil || entryInfo.Mode()&os.ModeSymlink != 0 || (!entryInfo.IsDir() && !entryInfo.Mode().IsRegular()) {
			continue
		}
		childRef := filepath.ToSlash(filepath.Join(ref, entry.Name()))
		item := WorkspaceDirectoryEntry{Ref: childRef, Kind: "file", ByteSize: entryInfo.Size()}
		if entryInfo.IsDir() {
			item.Kind = "directory"
		} else if sample, readErr := readFileSample(filepath.Join(path, entry.Name())); readErr == nil {
			item.MIMEType = detectWorkspaceMIME(childRef, sample)
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind != result[j].Kind {
			return result[i].Kind == "directory"
		}
		return result[i].Ref < result[j].Ref
	})
	return result, nil
}

func normalizeWorkspaceViewRef(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "\\") || strings.ContainsRune(raw, '\x00') || (len(raw) >= 2 && raw[1] == ':') {
		return "", domain.Invalid("WORKSPACE_VIEW_PATH_INVALID", "ref 必须使用正斜杠的 Workspace-relative 路径")
	}
	ref := filepath.ToSlash(filepath.Clean(filepath.FromSlash(raw)))
	if ref == "." || filepath.IsAbs(ref) || ref == ".." || strings.HasPrefix(ref, "../") || strings.Contains(raw, "//") {
		return "", domain.Invalid("WORKSPACE_VIEW_PATH_INVALID", "ref 必须是允许目录下的 Workspace-relative 路径")
	}
	parts := strings.Split(ref, "/")
	if !workspaceViewRoots[parts[0]] {
		return "", domain.Policy("WORKSPACE_VIEW_PATH_DENIED", "ref 不在本地展示 allowlist 中", "只读取 context、sources、knowledge、work、production、delivery、results 或 archive")
	}
	for _, part := range parts[1:] {
		if deniedWorkspaceViewSegment(part) {
			return "", domain.Policy("WORKSPACE_VIEW_PATH_DENIED", "ref 包含默认拒绝展示的敏感路径段", "不要通过本地工作台读取凭据、Token、日志、transcript 或隐藏文件")
		}
	}
	return ref, nil
}

func deniedWorkspaceViewSegment(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if strings.HasPrefix(value, ".") || strings.HasPrefix(value, ".env") {
		return true
	}
	switch value {
	case "credential", "credentials", "token", "tokens", "log", "logs", "transcript", "transcripts":
		return true
	default:
		return false
	}
}

func readFileSample(path string) ([]byte, error) {
	reader, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(io.LimitReader(reader, workspaceMIMESniffBytes))
}

func detectWorkspaceMIME(ref string, body []byte) string {
	extension := strings.ToLower(filepath.Ext(ref))
	if extension == ".md" || extension == ".markdown" {
		return "text/markdown"
	}
	if extension == ".yaml" || extension == ".yml" {
		return "application/yaml"
	}
	if extension == ".csv" {
		return "text/csv"
	}
	detected := ingest.DetectMIME(body)
	if detected == "text/html" {
		return "text/plain"
	}
	if detected != "text/plain" && detected != "application/octet-stream" {
		return detected
	}
	if value := mime.TypeByExtension(extension); value != "" {
		value = strings.Split(value, ";")[0]
		if strings.HasPrefix(value, "text/") || value == "application/json" || streamableWorkspaceMIME(value) {
			return value
		}
	}
	if utf8.Valid(body) && !bytes.ContainsRune(body, '\x00') {
		return "text/plain"
	}
	return detected
}

func streamableWorkspaceMIME(mediaType string) bool {
	return strings.HasPrefix(mediaType, "image/") || strings.HasPrefix(mediaType, "audio/") || strings.HasPrefix(mediaType, "video/") || mediaType == "application/pdf"
}

func workspaceFileTooLarge(size, limit int64) error {
	policy := domain.Policy("WORKSPACE_VIEW_FILE_TOO_LARGE", "文件超过本地展示大小上限", "选择受支持且大小受控的 Workspace 文件")
	policy.Details = map[string]any{"byte_size": size, "max_bytes": limit}
	return policy
}

func verifyWorkspaceViewDigest(actual, expected string) error {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return nil
	}
	if !strings.HasPrefix(expected, "sha256:") {
		expected = "sha256:" + expected
	}
	if actual == expected {
		return nil
	}
	conflict := domain.Conflict("WORKSPACE_VIEW_STALE", "本地文件摘要已变化，旧视图不能继续使用")
	conflict.Details = map[string]any{"expected_digest": expected, "actual_digest": actual}
	return conflict
}

func verifyWorkspaceViewRevision(actual, expected uint64) error {
	if expected == 0 || expected == actual {
		return nil
	}
	conflict := domain.Conflict("WORKSPACE_VIEW_STALE", "LocalRun context revision 已变化，旧视图不能继续使用")
	conflict.Details = map[string]any{"expected_context_revision": expected, "actual_context_revision": actual}
	return conflict
}

func workspaceViewErrorCode(err error, code string) bool {
	var domainError *domain.Error
	return errors.As(err, &domainError) && domainError.Code == code
}

func validWorkspaceViewKind(kind string) bool {
	switch kind {
	case "workspace_summary", "file", "run", "handoff", "content_item", "render", "diff", "delivery":
		return true
	default:
		return false
	}
}

func workspaceViewSummary(kind string, file workspaceViewFile) string {
	return fmt.Sprintf("%s 视图，%s，%d bytes，摘要 %s", kind, file.MIMEType, file.Size, file.Digest)
}

func workspaceDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func resourceFromBody(uri, mediaType string, body []byte) WorkspaceResource {
	if strings.HasPrefix(mediaType, "text/") || mediaType == "application/json" || mediaType == "application/yaml" {
		return WorkspaceResource{URI: uri, MIMEType: mediaType, Text: string(body)}
	}
	return WorkspaceResource{URI: uri, MIMEType: mediaType, Blob: body}
}

func minInt64(left int64, right int) int {
	if left < int64(right) {
		return int(left)
	}
	return right
}
