package modelprovider

import (
	"context"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/limecloud/contentcloud/internal/platform/fault"
)

type Provider interface {
	Detect(context.Context) (Capability, error)
	Complete(context.Context, CompletionRequest) (CompletionResult, error)
}

type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

func NewRegistry() *Registry { return &Registry{providers: map[string]Provider{}} }

func NewDefaultRegistry() *Registry {
	registry := NewRegistry()
	for _, candidate := range []struct {
		id, endpoint, model, key string
		factory                  func(Config) (*Adapter, error)
	}{
		{"vllm", os.Getenv("CONTENTCLOUD_VLLM_ENDPOINT"), os.Getenv("CONTENTCLOUD_VLLM_MODEL"), os.Getenv("CONTENTCLOUD_VLLM_API_KEY"), NewVLLM},
		{"sglang", os.Getenv("CONTENTCLOUD_SGLANG_ENDPOINT"), os.Getenv("CONTENTCLOUD_SGLANG_MODEL"), os.Getenv("CONTENTCLOUD_SGLANG_API_KEY"), NewSGLang},
	} {
		if strings.TrimSpace(candidate.endpoint) == "" || strings.TrimSpace(candidate.model) == "" {
			continue
		}
		if provider, err := candidate.factory(Config{Endpoint: candidate.endpoint, Model: candidate.model, APIKey: candidate.key}); err == nil {
			registry.Register(candidate.id, provider)
		}
	}
	return registry
}

func (r *Registry) Register(id string, provider Provider) {
	if r == nil || provider == nil {
		return
	}
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[id] = provider
}

func (r *Registry) Resolve(id string) (Provider, error) {
	if r == nil {
		return nil, fault.Policy("MODEL_PROVIDER_UNAVAILABLE", "模型 Provider 注册表未配置", "配置模型 Provider 后重试")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	provider, ok := r.providers[strings.ToLower(strings.TrimSpace(id))]
	if !ok {
		return nil, fault.Policy("MODEL_PROVIDER_UNAVAILABLE", "模型 Provider 不可用", "配置 vLLM、SGLang 或注入兼容 Provider")
	}
	return provider, nil
}

func (r *Registry) IDs() []string {
	if r == nil {
		return []string{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.providers))
	for id := range r.providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
