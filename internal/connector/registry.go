package connector

import (
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/limecloud/contentcloud/internal/domain"
)

type Registry struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
}

func NewRegistry() *Registry { return &Registry{adapters: map[string]Adapter{}} }

func NewDefaultRegistry() *Registry {
	registry := NewRegistry()
	endpoint := strings.TrimSpace(os.Getenv("CONTENTCLOUD_CONNECTOR_ENDPOINT"))
	if endpoint == "" {
		return registry
	}
	adapter, err := NewHTTPAdapter(HTTPConfig{
		Endpoint: endpoint,
		Token:    os.Getenv("CONTENTCLOUD_CONNECTOR_API_KEY"),
	})
	if err == nil {
		registry.Register("remote-http", adapter)
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
		return nil, domain.Policy("CONNECTOR_ADAPTER_UNAVAILABLE", "Connector Adapter 注册表未配置", "配置 Connector 后重试")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	adapter, ok := r.adapters[strings.ToLower(strings.TrimSpace(id))]
	if !ok {
		return nil, domain.Policy("CONNECTOR_ADAPTER_UNAVAILABLE", "Connector Adapter 不可用", "检查绑定的 Connector ID 和服务端配置")
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
