package agentadapter

import (
	"context"
	"strings"
	"sync"

	"github.com/limecloud/contentcloud/internal/domain"
)

type harnessRegistryEntry struct {
	adapter AgentHarnessAdapter
	once    sync.Once
	caps    HarnessCapabilities
	err     error
}

// HarnessRegistry caches capability detection and owns active adapters for one
// worker process. Durable session identity comes from the host and is fixed on
// RuntimeAttempt, so resumable adapters must also work after process restart.
type HarnessRegistry struct {
	mu      sync.RWMutex
	entries map[string]*harnessRegistryEntry
}

func NewHarnessRegistry() *HarnessRegistry {
	return &HarnessRegistry{entries: map[string]*harnessRegistryEntry{}}
}

func NewDefaultHarnessRegistry() *HarnessRegistry {
	registry := NewHarnessRegistry()
	registry.mustRegister("fake", NewFakeHarness())
	registry.mustRegister("codex", newCodexExecHarness())
	registry.mustRegister("claude", newClaudeStreamHarness())
	return registry
}

func (r *HarnessRegistry) Register(kind string, adapter AgentHarnessAdapter) error {
	if r == nil || adapter == nil {
		return domain.Invalid("AGENT_HARNESS_REGISTRY_INVALID", "HarnessRegistry 缺少适配器")
	}
	normalized := normalizeHarnessKind(kind)
	if normalized == "" {
		return domain.Invalid("AGENT_HARNESS_INVALID", "智能体执行适配器类型不能为空")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.entries[normalized]; exists {
		return domain.Conflict("AGENT_HARNESS_EXISTS", "智能体执行适配器已经注册")
	}
	r.entries[normalized] = &harnessRegistryEntry{adapter: adapter}
	return nil
}

func (r *HarnessRegistry) Resolve(ctx context.Context, kind string) (AgentHarnessAdapter, HarnessCapabilities, error) {
	if r == nil {
		return nil, HarnessCapabilities{}, domain.Policy("AGENT_HARNESS_REGISTRY_UNAVAILABLE", "智能体执行适配器注册表尚未配置", "联系平台运营人员检查 Runtime 配置")
	}
	normalized := normalizeHarnessKind(kind)
	r.mu.RLock()
	entry := r.entries[normalized]
	r.mu.RUnlock()
	if entry == nil {
		return nil, HarnessCapabilities{}, domain.Invalid("AGENT_HARNESS_INVALID", "未知的智能体执行适配器")
	}
	entry.once.Do(func() {
		entry.caps, entry.err = entry.adapter.Detect(ctx)
		if entry.err == nil {
			entry.caps.Kind = normalizeHarnessKind(entry.caps.Kind)
			if entry.caps.Kind != normalized {
				entry.err = domain.Invalid("AGENT_HARNESS_KIND_MISMATCH", "智能体执行适配器能力声明与注册类型不一致")
			}
		}
	})
	if entry.err != nil {
		return nil, HarnessCapabilities{}, entry.err
	}
	return entry.adapter, entry.caps, nil
}

func (r *HarnessRegistry) mustRegister(kind string, adapter AgentHarnessAdapter) {
	if err := r.Register(kind, adapter); err != nil {
		panic(err)
	}
}

func normalizeHarnessKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "fake", "fakeharness":
		return "fake"
	case "codex":
		return "codex"
	case "claude", "claude-code":
		return "claude"
	default:
		return strings.ToLower(strings.TrimSpace(kind))
	}
}
