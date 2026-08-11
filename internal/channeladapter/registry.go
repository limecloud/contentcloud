package channeladapter

import (
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/limecloud/contentcloud/internal/domain"
)

const ManualAdapterID = "manual"

type Registry struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
}

func NewRegistry() *Registry {
	return &Registry{adapters: map[string]Adapter{}}
}

func NewDefaultRegistry() *Registry {
	registry := NewRegistry()
	registry.Register(ManualAdapterID, ManualAdapter{})
	if endpoint := strings.TrimSpace(os.Getenv("CONTENTCLOUD_CHANNEL_ENDPOINT")); endpoint != "" {
		if adapter, err := NewHTTP(HTTPConfig{Endpoint: endpoint, Token: os.Getenv("CONTENTCLOUD_CHANNEL_TOKEN")}); err == nil {
			registry.Register("remote-http", adapter)
		}
	}
	return registry
}

func (r *Registry) Register(id string, adapter Adapter) {
	if r == nil || adapter == nil {
		return
	}
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[id] = adapter
}

func (r *Registry) Resolve(id string) (Adapter, error) {
	if r == nil {
		return nil, domain.Policy("CHANNEL_ADAPTER_UNAVAILABLE", "渠道适配器注册表未配置", "配置渠道适配器后重试")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	adapter, ok := r.adapters[strings.ToLower(strings.TrimSpace(id))]
	if !ok {
		return nil, domain.Policy("CHANNEL_ADAPTER_UNAVAILABLE", "渠道适配器不可用", "检查渠道绑定和服务端适配器配置")
	}
	return adapter, nil
}

func (r *Registry) IDs() []string {
	if r == nil {
		return []string{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.adapters))
	for id := range r.adapters {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
