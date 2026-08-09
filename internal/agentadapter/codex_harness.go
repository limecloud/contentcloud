package agentadapter

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

const maxCodexHarnessEventBytes = 1 << 20

type codexExecHarness struct {
	binary     string
	prefixArgs []string
	extraEnv   []string
	detect     func(context.Context) error

	mu       sync.Mutex
	sessions map[string]*codexExecSession
}

type codexExecSession struct {
	status AgentSessionStatus
	stream *harnessEventStream
	cancel context.CancelFunc
}

type codexJSONEvent struct {
	Type     string          `json:"type"`
	ThreadID string          `json:"thread_id,omitempty"`
	Usage    *codexUsage     `json:"usage,omitempty"`
	Item     json.RawMessage `json:"item,omitempty"`
}

type codexUsage struct {
	InputTokens           int64 `json:"input_tokens"`
	CachedInputTokens     int64 `json:"cached_input_tokens"`
	CacheWriteInputTokens int64 `json:"cache_write_input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
}

func newCodexExecHarness() AgentHarnessAdapter {
	harness := &codexExecHarness{binary: "codex", sessions: map[string]*codexExecSession{}}
	harness.detect = func(ctx context.Context) error {
		path, err := exec.LookPath(harness.binary)
		if err != nil {
			return err
		}
		cmd := exec.CommandContext(ctx, path, "exec", "resume", "--help")
		cmd.Env = agentEnvironment("codex")
		return cmd.Run()
	}
	return harness
}

func (h *codexExecHarness) Detect(ctx context.Context) (HarnessCapabilities, error) {
	if h == nil || h.detect == nil {
		return HarnessCapabilities{}, domain.Policy("CODEX_HARNESS_UNAVAILABLE", "Codex Harness 尚未配置", "检查 Runtime 的 Codex 执行适配器配置")
	}
	if err := h.detect(ctx); err != nil {
		return HarnessCapabilities{}, err
	}
	return HarnessCapabilities{
		Kind: "codex", Events: true, Resume: true, Fork: false,
		StructuredOutput: true, SandboxProfile: "workspace_write_auto_approval",
		MaxParallelSessions: 8, TranscriptExport: false,
	}, nil
}

func (h *codexExecHarness) Start(ctx context.Context, request StartAgentRequest) (AgentSessionRef, EventStream, error) {
	if strings.TrimSpace(request.NodeRunID) == "" || strings.TrimSpace(request.AttemptID) == "" {
		return AgentSessionRef{}, nil, domain.Invalid("AGENT_START_INVALID", "启动智能体需要 NodeRun 和 Attempt 引用")
	}
	dir, contract, _, skill, err := loadWorkspace(request.Workspace)
	if err != nil {
		return AgentSessionRef{}, nil, err
	}
	prompt := codexHarnessPrompt(agentPrompt(contract, skill), request.Prompt)
	return h.launch(ctx, request.TenantID, dir, prompt, "")
}

func (h *codexExecHarness) Resume(ctx context.Context, request ResumeAgentRequest) (EventStream, error) {
	if strings.TrimSpace(request.Session.SessionID) == "" || normalizeHarnessKind(request.Session.HarnessKind) != "codex" {
		return nil, domain.Invalid("AGENT_SESSION_INVALID", "Codex Resume 缺少有效会话引用")
	}
	if request.Session.TenantID != "" && request.TenantID != "" && request.Session.TenantID != request.TenantID {
		return nil, domain.Policy("AGENT_SESSION_TENANT_MISMATCH", "Codex 会话不属于当前租户", "使用当前 RuntimeAttempt 绑定的会话引用")
	}
	dir, contract, _, skill, err := loadWorkspace(request.Workspace)
	if err != nil {
		return nil, err
	}
	tenantID := request.TenantID
	if tenantID == "" {
		tenantID = request.Session.TenantID
	}
	prompt := codexHarnessPrompt(agentPrompt(contract, skill), request.Prompt)
	ref, stream, err := h.launch(ctx, tenantID, dir, prompt, request.Session.SessionID)
	if err != nil {
		return nil, err
	}
	if ref.SessionID != request.Session.SessionID {
		_ = stream.Close()
		return nil, domain.Conflict("CODEX_SESSION_MISMATCH", "Codex Resume 返回了不同的会话标识")
	}
	return stream, nil
}

func (h *codexExecHarness) Interrupt(_ context.Context, ref AgentSessionRef) error {
	h.mu.Lock()
	session, ok := h.sessions[ref.SessionID]
	if !ok || (ref.TenantID != "" && session.status.Session.TenantID != ref.TenantID) {
		h.mu.Unlock()
		return domain.NotFound("Codex 会话")
	}
	if session.status.State != "active" {
		h.mu.Unlock()
		return nil
	}
	now := time.Now().UTC()
	session.status.State = "interrupted"
	session.status.LastEventAt = now
	cancel := session.cancel
	stream := session.stream
	h.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	stream.emit(AgentEvent{Type: "session.interrupted", Session: ref, OccurredAt: now})
	return stream.Close()
}

func (h *codexExecHarness) Inspect(_ context.Context, ref AgentSessionRef) (AgentSessionStatus, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	session, ok := h.sessions[ref.SessionID]
	if !ok || (ref.TenantID != "" && session.status.Session.TenantID != ref.TenantID) {
		return AgentSessionStatus{}, domain.NotFound("Codex 会话")
	}
	return session.status, nil
}

func (h *codexExecHarness) launch(ctx context.Context, tenantID, dir, prompt, resumeSessionID string) (AgentSessionRef, EventStream, error) {
	resultPath, err := reserveCodexResultPath(dir)
	if err != nil {
		return AgentSessionRef{}, nil, err
	}
	cleanupResult := true
	defer func() {
		if cleanupResult {
			_ = os.Remove(resultPath)
		}
	}()

	runCtx, cancel := context.WithCancel(ctx)
	args := codexHarnessArguments(dir, resultPath, resumeSessionID)
	cmd := exec.CommandContext(runCtx, h.binary, append(append([]string(nil), h.prefixArgs...), args...)...)
	configureAgentProcess(cmd)
	cmd.Dir = dir
	cmd.Env = append(agentEnvironment("codex"), h.extraEnv...)
	cmd.Stdin = strings.NewReader(prompt)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return AgentSessionRef{}, nil, err
	}
	var stderr limitedBuffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return AgentSessionRef{}, nil, err
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64<<10), maxCodexHarnessEventBytes)
	first, err := scanCodexEvent(scanner)
	if err != nil || first.Type != "thread.started" || !validCodexSessionID(first.ThreadID) {
		cancel()
		_ = cmd.Wait()
		if err == nil {
			err = domain.Invalid("CODEX_EVENT_PROTOCOL_INVALID", "Codex JSONL 未以有效 thread.started 事件开始")
		}
		return AgentSessionRef{}, nil, err
	}
	if resumeSessionID != "" && first.ThreadID != resumeSessionID {
		cancel()
		_ = cmd.Wait()
		return AgentSessionRef{}, nil, domain.Conflict("CODEX_SESSION_MISMATCH", "Codex Resume 返回了不同的会话标识")
	}

	ref := AgentSessionRef{TenantID: tenantID, HarnessKind: "codex", SessionID: first.ThreadID}
	stream := newHarnessEventStream()
	session := &codexExecSession{status: AgentSessionStatus{Session: ref, State: "active", LastEventAt: time.Now().UTC()}, stream: stream, cancel: cancel}
	if err := h.registerSession(session); err != nil {
		cancel()
		_ = cmd.Wait()
		return AgentSessionRef{}, nil, err
	}
	eventType := "session.started"
	if resumeSessionID != "" {
		eventType = "session.resumed"
	}
	stream.emit(AgentEvent{Type: eventType, Session: ref, OccurredAt: session.status.LastEventAt})
	cleanupResult = false
	go h.consume(session, scanner, cmd, resultPath)
	return ref, stream, nil
}

func (h *codexExecHarness) registerSession(session *codexExecSession) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if existing := h.sessions[session.status.Session.SessionID]; existing != nil {
		if existing.status.Session.TenantID != session.status.Session.TenantID {
			return domain.Policy("AGENT_SESSION_TENANT_MISMATCH", "Codex 会话已经绑定其他租户", "使用当前 RuntimeAttempt 绑定的会话引用")
		}
		if existing.status.State == "active" {
			return domain.Conflict("CODEX_SESSION_ACTIVE", "Codex 会话已有活动执行")
		}
	}
	h.sessions[session.status.Session.SessionID] = session
	return nil
}

func (h *codexExecHarness) consume(session *codexExecSession, scanner *bufio.Scanner, cmd *exec.Cmd, resultPath string) {
	defer os.Remove(resultPath)
	cancel := session.cancel
	failed := false
	for scanner.Scan() {
		event, err := decodeCodexEvent(scanner.Bytes())
		if err != nil {
			h.failSession(session, "CODEX_EVENT_PROTOCOL_INVALID")
			cancel()
			failed = true
			break
		}
		if event.Type == "thread.started" {
			if event.ThreadID != session.status.Session.SessionID {
				h.failSession(session, "CODEX_SESSION_MISMATCH")
				cancel()
				failed = true
				break
			}
			continue
		}
		if event.Type == "turn.failed" || event.Type == "error" {
			h.failSession(session, codexFailureCode(event.Type))
			cancel()
			failed = true
			break
		}
		if projected, ok := projectCodexEvent(session.status.Session, event); ok {
			if !session.stream.emit(projected) {
				cancel()
				failed = true
				break
			}
			h.touchSession(session, projected.OccurredAt)
		}
	}
	scanErr := scanner.Err()
	waitErr := cmd.Wait()
	if failed {
		return
	}
	if scanErr != nil {
		h.failSession(session, "CODEX_EVENT_STREAM_INVALID")
		return
	}
	if waitErr != nil {
		h.failSession(session, "CODEX_PROCESS_FAILED")
		return
	}
	body, err := os.ReadFile(resultPath)
	if err != nil {
		h.failSession(session, "CODEX_RESULT_MISSING")
		return
	}
	result, err := decodeOutput(body)
	if err != nil {
		h.failSession(session, "CODEX_RESULT_INVALID")
		return
	}
	h.completeSession(session, result)
}

func (h *codexExecHarness) touchSession(session *codexExecSession, now time.Time) {
	h.mu.Lock()
	if session.status.State == "active" {
		session.status.LastEventAt = now
	}
	h.mu.Unlock()
}

func (h *codexExecHarness) failSession(session *codexExecSession, code string) {
	now := time.Now().UTC()
	h.mu.Lock()
	if session.status.State != "active" {
		h.mu.Unlock()
		return
	}
	session.status.State = "failed"
	session.status.ErrorCode = code
	session.status.LastEventAt = now
	session.cancel = nil
	h.mu.Unlock()
	session.stream.emit(AgentEvent{Type: "session.failed", Session: session.status.Session, ErrorCode: code, OccurredAt: now})
	_ = session.stream.Close()
}

func (h *codexExecHarness) completeSession(session *codexExecSession, result json.RawMessage) {
	now := time.Now().UTC()
	h.mu.Lock()
	if session.status.State != "active" {
		h.mu.Unlock()
		return
	}
	session.status.State = "completed"
	session.status.LastEventAt = now
	session.cancel = nil
	h.mu.Unlock()
	session.stream.emit(AgentEvent{Type: "result.completed", Session: session.status.Session, Data: result, OccurredAt: now})
	_ = session.stream.Close()
}

func reserveCodexResultPath(dir string) (string, error) {
	file, err := os.CreateTemp(dir, ".contentcloud-codex-result-*.json")
	if err != nil {
		return "", err
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	return path, nil
}

func codexHarnessArguments(dir, outputPath, resumeSessionID string) []string {
	args := []string{
		"exec", "--json", "--sandbox", "workspace-write", "--approve-for-me",
		"--skip-git-repo-check", "--output-schema", filepath.Join(dir, "output.schema.json"),
		"--output-last-message", outputPath, "--cd", dir,
	}
	if resumeSessionID != "" {
		args = append(args, "resume", resumeSessionID)
	}
	return args
}

func codexHarnessPrompt(base, supplemental string) string {
	supplemental = strings.TrimSpace(supplemental)
	if supplemental == "" {
		return base
	}
	return base + "\n\n<runtime_instruction>\n" + supplemental + "\n</runtime_instruction>"
}

func scanCodexEvent(scanner *bufio.Scanner) (codexJSONEvent, error) {
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return codexJSONEvent{}, err
		}
		return codexJSONEvent{}, io.ErrUnexpectedEOF
	}
	return decodeCodexEvent(scanner.Bytes())
}

func decodeCodexEvent(line []byte) (codexJSONEvent, error) {
	var event codexJSONEvent
	if len(line) == 0 || json.Unmarshal(line, &event) != nil || strings.TrimSpace(event.Type) == "" {
		return codexJSONEvent{}, domain.Invalid("CODEX_EVENT_PROTOCOL_INVALID", "Codex 返回了无效 JSONL 事件")
	}
	return event, nil
}

func projectCodexEvent(ref AgentSessionRef, event codexJSONEvent) (AgentEvent, bool) {
	now := time.Now().UTC()
	switch event.Type {
	case "turn.started":
		return AgentEvent{Type: event.Type, Session: ref, OccurredAt: now}, true
	case "turn.completed":
		body, _ := json.Marshal(struct {
			Usage *codexUsage `json:"usage,omitempty"`
		}{event.Usage})
		return AgentEvent{Type: event.Type, Session: ref, Data: body, OccurredAt: now}, true
	case "item.started", "item.updated", "item.completed":
		var item struct {
			Type   string `json:"type"`
			Status string `json:"status,omitempty"`
		}
		_ = json.Unmarshal(event.Item, &item)
		body, _ := json.Marshal(struct {
			ItemType string `json:"item_type,omitempty"`
			Status   string `json:"status,omitempty"`
		}{ItemType: item.Type, Status: item.Status})
		return AgentEvent{Type: event.Type, Session: ref, Data: body, OccurredAt: now}, true
	default:
		return AgentEvent{}, false
	}
}

func codexFailureCode(eventType string) string {
	if eventType == "turn.failed" {
		return "CODEX_TURN_FAILED"
	}
	return "CODEX_EVENT_STREAM_FAILED"
}

func validCodexSessionID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 1024 {
		return false
	}
	for _, char := range value {
		if char < 0x21 || char == 0x7f {
			return false
		}
	}
	return true
}

var _ AgentHarnessAdapter = (*codexExecHarness)(nil)
