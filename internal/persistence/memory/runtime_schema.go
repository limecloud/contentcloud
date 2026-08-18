package memory

import (
	"context"
	"sort"
	"strconv"

	contentruntime "github.com/limecloud/contentcloud/internal/runtime"

	"github.com/limecloud/contentcloud/internal/platform/fault"
)

func runtimeSchemaKey(tenantID, schemaID string, revision int) string {
	return tenantID + ":" + schemaID + ":" + strconv.Itoa(revision)
}

func (s *Store) CreateRuntimeSchema(_ context.Context, schema contentruntime.RuntimeSchema) error {
	if err := schema.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := runtimeSchemaKey(schema.TenantID, schema.SchemaID, schema.Revision)
	if _, exists := s.runtimeSchemas[key]; exists {
		return fault.Conflict("RUNTIME_SCHEMA_EXISTS", "Runtime Schema 版本已存在")
	}
	s.runtimeSchemas[key] = schema
	return nil
}

func (s *Store) RuntimeSchema(_ context.Context, tenantID, schemaID string, revision int) (contentruntime.RuntimeSchema, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	schema, ok := s.runtimeSchemas[runtimeSchemaKey(tenantID, schemaID, revision)]
	if !ok {
		return contentruntime.RuntimeSchema{}, fault.NotFound("Runtime Schema")
	}
	return schema, nil
}

func (s *Store) RuntimeSchemas(_ context.Context, tenantID, schemaID string) ([]contentruntime.RuntimeSchema, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]contentruntime.RuntimeSchema, 0)
	for _, schema := range s.runtimeSchemas {
		if schema.TenantID == tenantID && (schemaID == "" || schema.SchemaID == schemaID) {
			result = append(result, schema)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].SchemaID == result[j].SchemaID {
			return result[i].Revision > result[j].Revision
		}
		return result[i].SchemaID < result[j].SchemaID
	})
	return result, nil
}

func (s *Store) PublishRuntimeSchema(_ context.Context, next contentruntime.RuntimeSchema, expectedVersion int) (contentruntime.RuntimeSchema, error) {
	return s.transitionRuntimeSchema(next, expectedVersion, "draft", "published")
}

func (s *Store) RetireRuntimeSchema(_ context.Context, next contentruntime.RuntimeSchema, expectedVersion int) (contentruntime.RuntimeSchema, error) {
	return s.transitionRuntimeSchema(next, expectedVersion, "published", "retired")
}

func (s *Store) transitionRuntimeSchema(next contentruntime.RuntimeSchema, expectedVersion int, expectedState, nextState string) (contentruntime.RuntimeSchema, error) {
	if err := next.Validate(); err != nil {
		return next, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := runtimeSchemaKey(next.TenantID, next.SchemaID, next.Revision)
	current, ok := s.runtimeSchemas[key]
	if !ok {
		return next, fault.NotFound("Runtime Schema")
	}
	if current.Version != expectedVersion || current.Status != expectedState || next.Version != expectedVersion+1 || next.Status != nextState {
		return current, fault.Conflict("RUNTIME_SCHEMA_VERSION_CONFLICT", "Runtime Schema 已被更新")
	}
	s.runtimeSchemas[key] = next
	return next, nil
}
