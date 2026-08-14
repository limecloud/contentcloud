package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	runtimepkg "github.com/limecloud/contentcloud/internal/runtime"
)

type runtimeMCPClient struct {
	URL          string
	Token        string
	AllowedTools map[string]struct{}
	HTTP         *http.Client
}

func newRuntimeMCPClient() (*runtimeMCPClient, error) {
	url := strings.TrimSpace(os.Getenv("CONTENTCLOUD_RUNTIME_GATEWAY_URL"))
	token := strings.TrimSpace(os.Getenv("CONTENTCLOUD_RUNTIME_GATEWAY_TOKEN"))
	if url == "" || !strings.HasPrefix(token, "rtg_") {
		return nil, &runtimeMCPError{code: -32001, message: "Runtime Gateway 配置缺失或已失效"}
	}
	rawTools := strings.TrimSpace(os.Getenv("CONTENTCLOUD_RUNTIME_GATEWAY_TOOLS"))
	if rawTools == "" {
		return nil, &runtimeMCPError{code: -32001, message: "Runtime Gateway 工具授权配置缺失或无效"}
	}
	allowed := map[string]struct{}{}
	var names []string
	if err := json.Unmarshal([]byte(rawTools), &names); err != nil || names == nil {
		return nil, &runtimeMCPError{code: -32001, message: "Runtime Gateway 工具授权配置缺失或无效"}
	}
	for _, name := range names {
		if strings.TrimSpace(name) != "" {
			allowed[strings.TrimSpace(name)] = struct{}{}
		}
	}
	return &runtimeMCPClient{URL: url, Token: token, AllowedTools: allowed, HTTP: &http.Client{}}, nil
}

func (r *Root) serveRuntimeMCP(ctx context.Context, input io.Reader) error {
	client, err := newRuntimeMCPClient()
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64<<10), 2<<20)
	encoder := json.NewEncoder(r.stdout)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		var request mcpRequest
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			if err := encoder.Encode(mcpResponse{JSONRPC: "2.0", Error: &mcpError{Code: -32700, Message: "JSON-RPC 请求无效"}}); err != nil {
				return err
			}
			continue
		}
		if request.Method == "notifications/initialized" {
			continue
		}
		response := client.handle(ctx, request)
		if err := encoder.Encode(response); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (c *runtimeMCPClient) handle(ctx context.Context, request mcpRequest) mcpResponse {
	response := mcpResponse{JSONRPC: "2.0", ID: request.ID}
	switch request.Method {
	case "initialize":
		response.Result = map[string]any{"protocolVersion": requestedMCPProtocolVersion(request.Params), "capabilities": map[string]any{"tools": map[string]any{"listChanged": false}}, "serverInfo": map[string]string{"name": "contentcloud-runtime", "version": Version}}
	case "ping":
		response.Result = map[string]any{}
	case "tools/list":
		response.Result = map[string]any{"tools": c.tools()}
	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil || strings.TrimSpace(params.Name) == "" {
			response.Result = mcpToolError(&runtimeMCPError{code: -32602, message: "Runtime MCP 工具参数无效"})
			return response
		}
		value, err := c.call(ctx, params.Name, params.Arguments)
		if err != nil {
			response.Result = mcpToolError(err)
		} else {
			response.Result = map[string]any{"content": []map[string]string{{"type": "text", "text": value}}, "isError": false}
		}
	default:
		response.Error = &mcpError{Code: -32601, Message: "未找到对应方法"}
	}
	return response
}

func (c *runtimeMCPClient) tools() []map[string]any {
	definitions := map[string]map[string]any{
		runtimepkg.ToolStateGet:      {"description": "读取当前 Attempt 已授权的状态记录", "inputSchema": runtimeStateSchema()},
		runtimepkg.ToolStateQuery:    {"description": "分页查询当前 Attempt 已授权的状态记录", "inputSchema": runtimeStateSchema()},
		runtimepkg.ToolStateMutate:   {"description": "按当前 Attempt 的 CAS 约束写入状态记录", "inputSchema": runtimeStateMutateSchema()},
		runtimepkg.ToolChildList:     {"description": "列出当前 Attempt 的动态子执行", "inputSchema": map[string]any{"type": "object", "additionalProperties": false}},
		runtimepkg.ToolEffectPrepare: {"description": "为外部副作用创建幂等 Effect", "inputSchema": map[string]any{"type": "object", "additionalProperties": true}},
		runtimepkg.ToolEffectStatus:  {"description": "读取当前 Attempt 已授权的 Effect 状态", "inputSchema": map[string]any{"type": "object", "additionalProperties": true}},
	}
	result := make([]map[string]any, 0, len(definitions))
	for _, name := range []string{runtimepkg.ToolStateGet, runtimepkg.ToolStateQuery, runtimepkg.ToolStateMutate, runtimepkg.ToolChildList, runtimepkg.ToolEffectPrepare, runtimepkg.ToolEffectStatus} {
		if _, ok := c.AllowedTools[name]; !ok {
			continue
		}
		entry := map[string]any{"name": name}
		for key, value := range definitions[name] {
			entry[key] = value
		}
		result = append(result, entry)
	}
	return result
}

func runtimeStateSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"collection": map[string]any{"type": "string"}, "key": map[string]any{"type": "string"}, "after_key": map[string]any{"type": "string"}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 100}}, "required": []string{"collection"}, "additionalProperties": true}
}

func runtimeStateMutateSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"collection": map[string]any{"type": "string"}, "key": map[string]any{"type": "string"}, "value": map[string]any{}, "expected_version": map[string]any{"type": "integer", "minimum": 0}}, "required": []string{"collection", "key", "value"}, "additionalProperties": false}
}

func (c *runtimeMCPClient) call(ctx context.Context, name string, arguments map[string]any) (string, error) {
	if _, ok := c.AllowedTools[name]; !ok {
		return "", &runtimeMCPError{code: -32003, message: "当前 Attempt 未授权该 Runtime MCP 工具"}
	}
	body, err := json.Marshal(map[string]any{"tool_name": name, "request_id": domain.NewID(), "arguments": arguments})
	if err != nil {
		return "", err
	}
	for attempt := 0; attempt < 5; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.Token)
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return "", err
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		resp.Body.Close()
		if readErr != nil {
			return "", readErr
		}
		var envelope struct {
			OK    bool            `json:"ok"`
			Data  json.RawMessage `json:"data"`
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil {
			return "", err
		}
		if envelope.OK {
			return string(envelope.Data), nil
		}
		if resp.StatusCode == http.StatusConflict && envelope.Error.Code == "MCP_GATEWAY_NOT_ACTIVE" && attempt < 4 {
			delay := time.Duration(attempt+1) * 50 * time.Millisecond
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return "", ctx.Err()
			case <-timer.C:
			}
			continue
		}
		return "", &runtimeMCPError{code: resp.StatusCode, message: string(data)}
	}
	return "", &runtimeMCPError{code: http.StatusConflict, message: "Runtime MCP Gateway 尚未激活"}
}

type runtimeMCPError struct {
	code    int
	message string
}

func (e *runtimeMCPError) Error() string { return e.message }
