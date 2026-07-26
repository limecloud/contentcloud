package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}
type Envelope struct {
	OK        bool            `json:"ok"`
	Command   string          `json:"command"`
	RequestID string          `json:"request_id"`
	Data      json.RawMessage `json:"data"`
	Meta      map[string]any  `json:"meta"`
	Error     *domain.Error   `json:"error"`
}

func New(baseURL, token string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, HTTP: &http.Client{Timeout: 30 * time.Second}}
}
func (c *Client) Dispatch(ctx context.Context, command string, params any, out any) error {
	payload, err := json.Marshal(map[string]any{"command": command, "params": params})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/v1/cli/dispatch", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return &domain.Error{Type: "network", Subtype: "request", Code: "NETWORK_ERROR", Message: err.Error(), Retryable: true, Hint: "检查 server URL 和网络连接", ExitCode: 5}
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return &domain.Error{Type: "not_found", Subtype: "empty_queue", Code: "NO_TASK", Message: "当前没有可领取任务", ExitCode: 0}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	var env Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("invalid server response (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if !env.OK {
		if env.Error == nil {
			return fmt.Errorf("server returned error")
		}
		return env.Error
	}
	if out != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("decode %s result: %w", command, err)
		}
	}
	return nil
}
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health returned %d", resp.StatusCode)
	}
	return nil
}
