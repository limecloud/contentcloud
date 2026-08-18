package agentadapter

import (
	"sort"
	"strings"

	"github.com/limecloud/contentcloud/internal/platform/fault"
)

type ClientID string

const (
	ClientCodex      ClientID = "codex"
	ClientClaudeCode ClientID = "claude-code"
	ClientWorkBuddy  ClientID = "workbuddy"
	ClientCursor     ClientID = "cursor"
	ClientHermes     ClientID = "hermes"
	ClientOpenClaw   ClientID = "openclaw"
)

type Capability string

const (
	CapabilityLocalAutomation     Capability = "local_automation"
	CapabilityWorkspaceRegister   Capability = "workspace_registration"
	CapabilityWorkspaceBootstrap  Capability = "workspace_bootstrap"
	CapabilityInteractiveHandoff  Capability = "interactive_handoff"
	CapabilityCreativeEnvironment Capability = "creative_environment"
)

type SupportStatus string

const (
	SupportAvailable SupportStatus = "available"
	SupportPlanned   SupportStatus = "planned"
)

type CapabilitySupport struct {
	ID     Capability    `json:"id"`
	Status SupportStatus `json:"status"`
}

type ClientDefinition struct {
	ID           ClientID            `json:"id"`
	DisplayName  string              `json:"display_name"`
	Capabilities []CapabilitySupport `json:"capabilities"`
	Aliases      []string            `json:"-"`
}

var capabilityOrder = []Capability{
	CapabilityLocalAutomation,
	CapabilityWorkspaceRegister,
	CapabilityWorkspaceBootstrap,
	CapabilityInteractiveHandoff,
	CapabilityCreativeEnvironment,
}

var clientDefinitions = []ClientDefinition{
	clientDefinition(ClientCodex, "Codex", nil, map[Capability]SupportStatus{
		CapabilityLocalAutomation: SupportAvailable, CapabilityWorkspaceRegister: SupportAvailable,
		CapabilityWorkspaceBootstrap: SupportAvailable, CapabilityInteractiveHandoff: SupportAvailable,
		CapabilityCreativeEnvironment: SupportAvailable,
	}),
	clientDefinition(ClientClaudeCode, "Claude Code", []string{"claude"}, map[Capability]SupportStatus{
		CapabilityLocalAutomation: SupportAvailable, CapabilityWorkspaceRegister: SupportAvailable,
	}),
	clientDefinition(ClientWorkBuddy, "WorkBuddy", nil, nil),
	clientDefinition(ClientCursor, "Cursor", nil, nil),
	clientDefinition(ClientHermes, "Hermes", nil, nil),
	clientDefinition(ClientOpenClaw, "OpenClaw", []string{"open-claw"}, nil),
}

func clientDefinition(id ClientID, displayName string, aliases []string, available map[Capability]SupportStatus) ClientDefinition {
	capabilities := make([]CapabilitySupport, 0, len(capabilityOrder))
	for _, capability := range capabilityOrder {
		status := SupportPlanned
		if configured, ok := available[capability]; ok {
			status = configured
		}
		capabilities = append(capabilities, CapabilitySupport{ID: capability, Status: status})
	}
	return ClientDefinition{ID: id, DisplayName: displayName, Capabilities: capabilities, Aliases: append([]string(nil), aliases...)}
}

// Clients returns a detached, stable catalog suitable for API responses.
func Clients() []ClientDefinition {
	clients := make([]ClientDefinition, len(clientDefinitions))
	for index, client := range clientDefinitions {
		clients[index] = cloneClient(client)
	}
	return clients
}

func Lookup(value string) (ClientDefinition, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	for _, client := range clientDefinitions {
		if normalized == string(client.ID) || containsAlias(client.Aliases, normalized) {
			return cloneClient(client), true
		}
	}
	return ClientDefinition{}, false
}

func RequireKnown(value string) (ClientDefinition, error) {
	client, ok := Lookup(value)
	if ok {
		return client, nil
	}
	err := fault.Invalid("AGENT_CLIENT_INVALID", "未知的智能体客户端")
	err.Details = map[string]any{"client": strings.TrimSpace(value), "known_clients": ClientIDs()}
	return ClientDefinition{}, err
}

func RequireCapability(value string, capability Capability) (ClientDefinition, error) {
	client, lookupErr := RequireKnown(value)
	if lookupErr != nil {
		return ClientDefinition{}, lookupErr
	}
	if client.CapabilityStatus(capability) == SupportAvailable {
		return client, nil
	}
	domainErr := fault.Policy("AGENT_CLIENT_CAPABILITY_UNAVAILABLE", client.DisplayName+" 尚未提供所需的 Content Work OS 能力", "选择已支持该能力的客户端，或等待对应适配器发布")
	domainErr.Details = map[string]any{"client": client.ID, "capability": capability, "status": client.CapabilityStatus(capability)}
	return ClientDefinition{}, domainErr
}

func (client ClientDefinition) CapabilityStatus(capability Capability) SupportStatus {
	for _, support := range client.Capabilities {
		if support.ID == capability {
			return support.Status
		}
	}
	return SupportPlanned
}

func ClientIDs() []string {
	values := make([]string, 0, len(clientDefinitions))
	for _, client := range clientDefinitions {
		values = append(values, string(client.ID))
	}
	sort.Strings(values)
	return values
}

func cloneClient(client ClientDefinition) ClientDefinition {
	client.Aliases = append([]string(nil), client.Aliases...)
	client.Capabilities = append([]CapabilitySupport(nil), client.Capabilities...)
	return client
}

func containsAlias(aliases []string, value string) bool {
	for _, alias := range aliases {
		if value == alias {
			return true
		}
	}
	return false
}
