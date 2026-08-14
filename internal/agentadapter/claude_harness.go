package agentadapter

import (
	"bufio"
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

const maxClaudeHarnessEventBytes = 1 << 20

type claudeStreamHarness struct {
	binary           string
	prefixArgs       []string
	extraEnv         []string
	detect           func(context.Context) (string, error)
	handshakeTimeout time.Duration
	mu               sync.Mutex
	sessions         map[string]*claudeStreamSession
}

type claudeStreamSession struct {
	status AgentSessionStatus
	stream *harnessEventStream
	cancel context.CancelFunc
}

type claudeJSONEvent struct {
	Type      string          `json:"type"`
	Subtype   string          `json:"subtype,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     string          `json:"error,omitempty"`
}

func newClaudeStreamHarness() AgentHarnessAdapter {
	harness := &claudeStreamHarness{binary: "claude", sessions: map[string]*claudeStreamSession{}}
	harness.detect = func(ctx context.Context) (string, error) {
		path, err := exec.LookPath(harness.binary)
		if err != nil {
			return "", err
		}
		output, err := exec.CommandContext(ctx, path, "--help").CombinedOutput()
		if err != nil {
			return "", err
		}
		help := string(output)
		for _, required := range []string{"--output-format", "stream-json", "--session-id", "--resume", "--mcp-config", "--strict-mcp-config"} {
			if !strings.Contains(help, required) {
				return "", domain.Policy("CLAUDE_CAPABILITY_UNAVAILABLE", "Claude CLI 缺少 Runtime 所需的结构化流或会话恢复能力", "升级 Claude CLI 或切换到支持 stream/resume 的 Harness")
			}
		}
		authOutput, authErr := exec.CommandContext(ctx, path, "auth", "status", "--json").CombinedOutput()
		var authStatus struct {
			LoggedIn bool `json:"loggedIn"`
		}
		if authErr != nil || json.Unmarshal(authOutput, &authStatus) != nil || !authStatus.LoggedIn {
			return "", domain.Policy("CLAUDE_AUTH_REQUIRED", "Claude Code 尚未完成认证", "在本机完成 Claude Code 登录后等待 Daemon 重探测或重启 Daemon")
		}
		version, versionErr := exec.CommandContext(ctx, path, "--version").CombinedOutput()
		if versionErr != nil {
			return "", versionErr
		}
		return strings.TrimSpace(string(version)), nil
	}
	return harness
}

func (h *claudeStreamHarness) Detect(ctx context.Context) (HarnessCapabilities, error) {
	if h == nil || h.detect == nil {
		return HarnessCapabilities{}, domain.Policy("CLAUDE_HARNESS_UNAVAILABLE", "Claude Harness 尚未配置", "检查 Runtime 的 Claude 执行适配器配置")
	}
	version, err := h.detect(ctx)
	if err != nil {
		return HarnessCapabilities{}, err
	}
	return HarnessCapabilities{Kind: "claude", Version: version, Events: true, Resume: true, Fork: false, MCPStdio: true, StructuredOutput: true, SandboxProfile: "workspace_write_bypass", MaxParallelSessions: 4, TranscriptExport: false}, nil
}

func (h *claudeStreamHarness) Start(ctx context.Context, request StartAgentRequest) (AgentSessionRef, EventStream, error) {
	if strings.TrimSpace(request.NodeRunID) == "" || strings.TrimSpace(request.AttemptID) == "" {
		return AgentSessionRef{}, nil, domain.Invalid("AGENT_START_INVALID", "启动智能体需要 NodeRun 和 Attempt 引用")
	}
	dir, contract, schema, skill, err := loadWorkspace(request.Workspace)
	if err != nil {
		return AgentSessionRef{}, nil, err
	}
	return h.launch(ctx, request.TenantID, dir, schema, claudeHarnessPrompt(agentPrompt(contract, skill), request.Prompt), "", request.RuntimeGateway)
}

func (h *claudeStreamHarness) Resume(ctx context.Context, request ResumeAgentRequest) (EventStream, error) {
	if normalizeHarnessKind(request.Session.HarnessKind) != "claude" || strings.TrimSpace(request.Session.SessionID) == "" {
		return nil, domain.Invalid("AGENT_SESSION_INVALID", "Claude Resume 缺少有效会话引用")
	}
	if request.Session.TenantID != "" && request.TenantID != "" && request.Session.TenantID != request.TenantID {
		return nil, domain.Policy("AGENT_SESSION_TENANT_MISMATCH", "Claude 会话不属于当前租户", "使用当前 RuntimeAttempt 绑定的会话引用")
	}
	dir, contract, schema, skill, err := loadWorkspace(request.Workspace)
	if err != nil {
		return nil, err
	}
	tenantID := request.TenantID
	if tenantID == "" {
		tenantID = request.Session.TenantID
	}
	_, stream, err := h.launch(ctx, tenantID, dir, schema, claudeHarnessPrompt(agentPrompt(contract, skill), request.Prompt), request.Session.SessionID, request.RuntimeGateway)
	return stream, err
}

func (h *claudeStreamHarness) Interrupt(_ context.Context, ref AgentSessionRef) error {
	h.mu.Lock()
	session, ok := h.sessions[ref.SessionID]
	if !ok || (ref.TenantID != "" && session.status.Session.TenantID != ref.TenantID) {
		h.mu.Unlock()
		return domain.NotFound("Claude 会话")
	}
	if session.status.State != "active" {
		h.mu.Unlock()
		return nil
	}
	session.status.State = "interrupted"
	session.status.LastEventAt = time.Now().UTC()
	cancel, stream := session.cancel, session.stream
	h.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	stream.emit(AgentEvent{Type: "session.interrupted", Session: ref, OccurredAt: time.Now().UTC()})
	return stream.Close()
}

func (h *claudeStreamHarness) Inspect(_ context.Context, ref AgentSessionRef) (AgentSessionStatus, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	session, ok := h.sessions[ref.SessionID]
	if !ok || (ref.TenantID != "" && session.status.Session.TenantID != ref.TenantID) {
		return AgentSessionStatus{}, domain.NotFound("Claude 会话")
	}
	return session.status, nil
}

func (h *claudeStreamHarness) launch(ctx context.Context, tenantID, dir string, schema []byte, prompt, resumeID string, gateway RuntimeGatewayConfig) (AgentSessionRef, EventStream, error) {
	runCtx, cancel := context.WithCancel(ctx)
	args := []string{"--print", "--output-format", "stream-json", "--input-format", "text", "--permission-mode", "bypassPermissions", "--json-schema", string(schema)}
	gatewayEnv, err := runtimeGatewayEnvironment(gateway)
	if err != nil {
		cancel()
		return AgentSessionRef{}, nil, err
	}
	if len(gatewayEnv) > 0 {
		executable, err := contentcloudExecutable()
		if err != nil {
			cancel()
			return AgentSessionRef{}, nil, err
		}
		config, err := json.Marshal(map[string]any{"mcpServers": map[string]any{"contentcloud-runtime": map[string]any{"type": "stdio", "command": executable, "args": []string{"mcp", "runtime-serve"}}}})
		if err != nil {
			cancel()
			return AgentSessionRef{}, nil, err
		}
		args = append(args, "--mcp-config", string(config), "--strict-mcp-config")
	}
	if resumeID != "" {
		args = append(args, "--resume", resumeID)
	} else {
		args = append(args, "--session-id", domain.NewID())
	}
	args = append(args, prompt)
	cmd := exec.CommandContext(runCtx, h.binary, append(append([]string(nil), h.prefixArgs...), args...)...)
	configureAgentProcess(cmd)
	cmd.Dir = dir
	cmd.Env = append(append(agentEnvironment("claude"), gatewayEnv...), h.extraEnv...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return AgentSessionRef{}, nil, err
	}
	var stderr limitedBuffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return AgentSessionRef{}, nil, classifyProcessError("claude", err, stderr.String())
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64<<10), maxClaudeHarnessEventBytes)
	type handshakeResult struct {
		event claudeJSONEvent
		err   error
	}
	handshake := make(chan handshakeResult, 1)
	go func() {
		first, scanErr := scanClaudeEvent(scanner)
		handshake <- handshakeResult{event: first, err: scanErr}
	}()
	var first claudeJSONEvent
	select {
	case result := <-handshake:
		first, err = result.event, result.err
	case <-time.After(resolvedHarnessHandshakeTimeout(h.handshakeTimeout)):
		cancel()
		_ = cmd.Wait()
		<-handshake
		return AgentSessionRef{}, nil, domain.Conflict("CLAUDE_HANDSHAKE_TIMEOUT", "Claude CLI 启动后未在期限内返回首个结构化事件")
	}
	if err != nil {
		cancel()
		waitErr := cmd.Wait()
		if waitErr != nil || processFailureCode("claude", err, stderr.String()) != "CLAUDE_PROCESS_FAILED" {
			if waitErr == nil {
				waitErr = err
			}
			return AgentSessionRef{}, nil, classifyProcessError("claude", waitErr, stderr.String())
		}
		return AgentSessionRef{}, nil, domain.Invalid("CLAUDE_EVENT_PROTOCOL_INVALID", "Claude CLI 未返回有效的首个结构化事件")
	}
	if first.SessionID == "" {
		cancel()
		_ = cmd.Wait()
		return AgentSessionRef{}, nil, domain.Invalid("CLAUDE_SESSION_ID_MISSING", "Claude CLI 结构化事件缺少 session_id")
	}
	if resumeID != "" && first.SessionID != resumeID {
		cancel()
		_ = cmd.Wait()
		return AgentSessionRef{}, nil, domain.Conflict("CLAUDE_SESSION_MISMATCH", "Claude Resume 返回了不同的会话标识")
	}
	ref := AgentSessionRef{TenantID: tenantID, HarnessKind: "claude", SessionID: first.SessionID}
	stream := newHarnessEventStream()
	session := &claudeStreamSession{status: AgentSessionStatus{Session: ref, State: "active", LastEventAt: time.Now().UTC()}, stream: stream, cancel: cancel}
	h.mu.Lock()
	if existing := h.sessions[ref.SessionID]; existing != nil && existing.status.State == "active" {
		h.mu.Unlock()
		cancel()
		_ = cmd.Wait()
		return AgentSessionRef{}, nil, domain.Conflict("CLAUDE_SESSION_ACTIVE", "Claude 会话已有活动执行")
	}
	h.sessions[ref.SessionID] = session
	h.mu.Unlock()
	eventType := "session.started"
	if resumeID != "" {
		eventType = "session.resumed"
	}
	stream.emit(AgentEvent{Type: eventType, Session: ref, OccurredAt: session.status.LastEventAt})
	go h.consume(session, scanner, cmd, &stderr, first)
	return ref, stream, nil
}

func (h *claudeStreamHarness) consume(session *claudeStreamSession, scanner *bufio.Scanner, cmd *exec.Cmd, stderr *limitedBuffer, first claudeJSONEvent) {
	cancel := session.cancel
	finishTerminal := func(projected AgentEvent) {
		if cancel != nil {
			cancel()
		}
		_ = cmd.Wait()
		h.mu.Lock()
		if projected.Type == "session.failed" {
			session.status.State = "failed"
			if session.status.ErrorCode == "" {
				session.status.ErrorCode = "CLAUDE_PROCESS_FAILED"
			}
		} else {
			session.status.State = "completed"
		}
		session.status.LastEventAt = projected.OccurredAt
		session.cancel = nil
		h.mu.Unlock()
		_ = session.stream.Close()
	}
	if projected, terminal := projectClaudeEvent(session.status.Session, first); projected.Type != "" {
		session.stream.emit(projected)
		if terminal {
			finishTerminal(projected)
			return
		}
	}
	for scanner.Scan() {
		event, err := scanClaudeEventBytes(scanner.Bytes())
		if err != nil {
			if cancel != nil {
				cancel()
			}
			_ = cmd.Wait()
			h.failClaudeSession(session, "CLAUDE_EVENT_PROTOCOL_INVALID")
			return
		}
		projected, terminal := projectClaudeEvent(session.status.Session, event)
		if projected.Type != "" {
			session.stream.emit(projected)
			h.touchClaudeSession(session, projected.OccurredAt)
		}
		if terminal {
			finishTerminal(projected)
			return
		}
	}
	if scanner.Err() != nil {
		if cancel != nil {
			cancel()
		}
		_ = cmd.Wait()
		h.failClaudeSession(session, "CLAUDE_EVENT_STREAM_INVALID")
		return
	}
	if err := cmd.Wait(); err != nil {
		h.failClaudeSession(session, processFailureCode("claude", err, stderr.String()))
		return
	}
	h.failClaudeSession(session, "CLAUDE_RESULT_MISSING")
}

func (h *claudeStreamHarness) touchClaudeSession(session *claudeStreamSession, now time.Time) {
	h.mu.Lock()
	if session.status.State == "active" {
		session.status.LastEventAt = now
	}
	h.mu.Unlock()
}

func (h *claudeStreamHarness) failClaudeSession(session *claudeStreamSession, code string) {
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

func scanClaudeEvent(scanner *bufio.Scanner) (claudeJSONEvent, error) {
	if !scanner.Scan() {
		return claudeJSONEvent{}, scanner.Err()
	}
	return scanClaudeEventBytes(scanner.Bytes())
}

func scanClaudeEventBytes(body []byte) (claudeJSONEvent, error) {
	var event claudeJSONEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return claudeJSONEvent{}, err
	}
	if strings.TrimSpace(event.Type) == "" {
		return claudeJSONEvent{}, domain.Invalid("CLAUDE_EVENT_PROTOCOL_INVALID", "Claude 事件缺少 type")
	}
	return event, nil
}

func projectClaudeEvent(session AgentSessionRef, event claudeJSONEvent) (AgentEvent, bool) {
	now := time.Now().UTC()
	switch strings.ToLower(strings.TrimSpace(event.Type)) {
	case "system":
		return AgentEvent{Type: "session.progress", Session: session, Data: json.RawMessage(`{"provider":"claude","event_type":"system"}`), OccurredAt: now}, false
	case "assistant", "user", "content_block_delta", "content_block_start", "content_block_stop":
		return AgentEvent{Type: "session.progress", Session: session, Data: json.RawMessage(`{"provider":"claude","event_type":"message"}`), OccurredAt: now}, false
	case "result":
		if !event.IsError && (event.Subtype == "" || strings.EqualFold(event.Subtype, "success")) && event.Error == "" {
			result, err := claudeResultPayload(event.Result)
			if err != nil {
				return AgentEvent{Type: "session.failed", Session: session, ErrorCode: "CLAUDE_RESULT_INVALID", OccurredAt: now}, true
			}
			return AgentEvent{Type: "result.completed", Session: session, Data: result, OccurredAt: now}, true
		}
		code := structuredFailureCode("claude", event.Error+string(event.Result))
		if code == "" {
			code = "CLAUDE_RESULT_FAILED"
		}
		return AgentEvent{Type: "session.failed", Session: session, ErrorCode: code, OccurredAt: now}, true
	default:
		return AgentEvent{Type: "session.progress", Session: session, Data: json.RawMessage(`{"provider":"claude","event_type":"unknown"}`), OccurredAt: now}, false
	}
}

func claudeResultPayload(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, domain.Invalid("CLAUDE_RESULT_INVALID", "Claude 结果为空")
	}
	var text string
	if raw[0] == '"' {
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil, err
		}
		return decodeOutput([]byte(text))
	}
	return decodeOutput(raw)
}

func claudeHarnessPrompt(systemPrompt, requestPrompt string) string {
	if strings.TrimSpace(requestPrompt) == "" {
		return systemPrompt
	}
	return systemPrompt + "\n\n<runtime_request>\n" + requestPrompt + "\n</runtime_request>"
}
