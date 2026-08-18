package application

import (
	"context"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	agentadapter "github.com/limecloud/contentcloud/internal/integration/agent"
)

type AgentHarnessCapability struct {
	Kind         string                           `json:"kind"`
	Available    bool                             `json:"available"`
	Capabilities agentadapter.HarnessCapabilities `json:"capabilities"`
	ErrorCode    string                           `json:"error_code,omitempty"`
}

func (s *CatalogService) AgentHarnessCapabilities(ctx context.Context, actor Actor) ([]AgentHarnessCapability, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return nil, err
	}
	result := make([]AgentHarnessCapability, 0, len(s.runtimeHarnesses.IDs()))
	for _, kind := range s.runtimeHarnesses.IDs() {
		_, capabilities, err := s.runtimeHarnesses.Resolve(ctx, kind)
		entry := AgentHarnessCapability{Kind: kind, Available: err == nil, Capabilities: capabilities}
		if err != nil {
			entry.ErrorCode = "AGENT_HARNESS_UNAVAILABLE"
			if value, ok := err.(*fault.Error); ok && value.Code != "" {
				entry.ErrorCode = value.Code
			}
		}
		result = append(result, entry)
	}
	return result, nil
}
