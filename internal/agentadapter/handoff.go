package agentadapter

import (
	"fmt"
	"net/url"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/integration/pluginbuiltin"
)

const HandoffSchemaVersion = "contentcloud.agent-handoff/1.0"

type HandoffTarget struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Digest string `json:"digest,omitempty"`
}

type HandoffIntegration struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Version string `json:"version"`
}

type HandoffLaunch struct {
	Mode string `json:"mode"`
	URL  string `json:"url"`
}

type Handoff struct {
	SchemaVersion              string             `json:"schema_version"`
	Client                     ClientDefinition   `json:"client"`
	Kind                       string             `json:"kind"`
	ProjectID                  string             `json:"project_id"`
	Target                     HandoffTarget      `json:"target"`
	Integration                HandoffIntegration `json:"integration"`
	RequiresNewSession         bool               `json:"requires_new_session"`
	RequiresWorkspaceSelection bool               `json:"requires_workspace_selection"`
	Launch                     HandoffLaunch      `json:"launch"`
	Prompt                     string             `json:"prompt"`
	Steps                      []string           `json:"steps"`
	FallbackURL                string             `json:"fallback_url"`
}

type HandoffRequest struct {
	Kind      string
	ProjectID string
	Target    HandoffTarget
	Version   string
}

type HandoffAdapter interface {
	Build(HandoffRequest) (Handoff, error)
}

func SelectHandoff(clientID, version string) (HandoffAdapter, error) {
	client, err := RequireCapability(clientID, CapabilityInteractiveHandoff)
	if err != nil {
		return nil, err
	}
	factory, ok := handoffFactories[client.ID]
	if !ok {
		return nil, domain.Policy("AGENT_HANDOFF_NOT_IMPLEMENTED", client.DisplayName+" 的恢复入口尚未实现", "选择已实现恢复入口的客户端")
	}
	return factory(version), nil
}

var handoffFactories = map[ClientID]func(string) HandoffAdapter{
	ClientCodex: func(version string) HandoffAdapter { return codexAgentHandoffAdapter{version: version} },
}

type codexAgentHandoffAdapter struct {
	version string
}

func (adapter codexAgentHandoffAdapter) Build(request HandoffRequest) (Handoff, error) {
	client, _ := Lookup(string(ClientCodex))
	pluginID := pluginbuiltin.VideoProduction
	pluginVersion := adapter.version
	if pluginVersion == "" {
		pluginVersion = pluginbuiltin.VideoProductionVersion
	}
	prompt, steps, err := codexAgentHandoffContent(pluginID, pluginVersion, request)
	if err != nil {
		return Handoff{}, err
	}
	query := url.Values{}
	query.Set("prompt", prompt)
	launchURL := (&url.URL{Scheme: "codex", Host: "new", RawQuery: query.Encode()}).String()
	return Handoff{
		SchemaVersion: HandoffSchemaVersion, Client: client, Kind: request.Kind, ProjectID: request.ProjectID, Target: request.Target,
		Integration:        HandoffIntegration{Kind: "plugin", ID: pluginID, Version: pluginVersion},
		RequiresNewSession: true, RequiresWorkspaceSelection: true,
		Launch: HandoffLaunch{Mode: "deep_link", URL: launchURL}, Prompt: prompt, Steps: steps, FallbackURL: "/codex",
	}, nil
}

func codexAgentHandoffContent(pluginID, pluginVersion string, request HandoffRequest) (string, []string, error) {
	if pluginID == "" || pluginVersion == "" {
		return "", nil, domain.Invalid("AGENT_HANDOFF_PLUGIN_INVALID", "Codex 交接缺少标准插件身份")
	}
	switch request.Kind {
	case "project":
		prompt := fmt.Sprintf("[@ContentCloud Video Production](plugin://%s) 在当前已选择的本机工作区中继续 Content Work OS 项目 %s。标准插件版本为 %s。先调用 workspace_context，并验证返回的 project_id 必须等于 %s；如果未选择工作区或 project_id 不匹配，立即停止，不要扫描其他目录。不要从旧对话历史重建状态，也不要自动执行 pull、claim、publish 或任何本地写入；先报告当前状态和下一步。", pluginID, request.ProjectID, pluginVersion, request.ProjectID)
		return prompt, []string{
			"在 Codex Desktop 中选择已连接该项目的本机 Workspace。",
			"打开新对话并先调用 workspace_context。",
			"核对 project_id 后，再由用户决定是否执行下一步。",
		}, nil
	case "review_feedback":
		prompt := fmt.Sprintf("[@ContentCloud Video Production](plugin://%s) 在当前已选择的本机工作区中处理 Content Work OS 项目 %s 的审核反馈，标准插件版本为 %s，目标提交修订版本为 %s，完整摘要为 %s。先调用 workspace_context，并验证返回的 project_id 必须等于 %s；如果未选择工作区或 project_id 不匹配，立即停止，不要扫描其他目录。随后只调用 review_feedback_list 读取云端反馈，并核对目标修订版本与摘要；未经用户明确要求，不要 pull、claim、修改文件或开始新的修订运行。", pluginID, request.ProjectID, pluginVersion, request.Target.ID, request.Target.Digest, request.ProjectID)
		return prompt, []string{
			"在 Codex Desktop 中选择已连接该项目的本机 Workspace。",
			"先调用 workspace_context 并核对 project_id。",
			"只读取反馈摘要；pull、claim 和本地修订均等待用户明确要求。",
		}, nil
	default:
		return "", nil, domain.Invalid("AGENT_HANDOFF_KIND_INVALID", "不支持该智能体恢复入口类型")
	}
}
