package runtime

import (
	"context"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

type RuntimeSchemaInput struct {
	TenantID        string
	SchemaID        string
	Revision        int
	Compatibility   string
	Definition      map[string]any
	RetentionPolicy string
	CreatedBy       string
}

func (s *Service) CreateRuntimeSchema(ctx context.Context, input RuntimeSchemaInput) (domain.RuntimeSchema, error) {
	repo, err := s.schemaRepository()
	if err != nil {
		return domain.RuntimeSchema{}, err
	}
	if strings.TrimSpace(input.TenantID) == "" || strings.TrimSpace(input.SchemaID) == "" || input.Revision < 1 || input.Definition == nil || strings.TrimSpace(input.CreatedBy) == "" {
		return domain.RuntimeSchema{}, domain.Invalid("RUNTIME_SCHEMA_INPUT_INVALID", "Runtime Schema 缺少租户、标识、版本、定义或创建者")
	}
	if rootType, _ := input.Definition["type"].(string); rootType != "object" {
		return domain.RuntimeSchema{}, domain.Invalid("RUNTIME_SCHEMA_DEFINITION_INVALID", "Runtime Schema 首版只接受根类型为 object 的 JSON Schema")
	}
	compatibility := strings.TrimSpace(input.Compatibility)
	if compatibility == "" {
		compatibility = "backward"
	}
	retention := normalizeSchemaRetention(input.RetentionPolicy)
	if retention == "" {
		return domain.RuntimeSchema{}, domain.Invalid("RUNTIME_SCHEMA_RETENTION_INVALID", "Runtime Schema 保留策略无效")
	}
	digest, err := domain.CanonicalHash(struct {
		SchemaID      string         `json:"schema_id"`
		Revision      int            `json:"revision"`
		Compatibility string         `json:"compatibility"`
		Definition    map[string]any `json:"definition"`
	}{input.SchemaID, input.Revision, compatibility, input.Definition})
	if err != nil {
		return domain.RuntimeSchema{}, err
	}
	now := s.now().UTC()
	schema := domain.RuntimeSchema{TenantID: input.TenantID, SchemaID: input.SchemaID, Revision: input.Revision, Status: "draft", Compatibility: compatibility, Definition: input.Definition, Digest: "sha256:" + digest, RetentionPolicy: retention, CreatedBy: input.CreatedBy, CreatedAt: now, Version: 1}
	if err := schema.Validate(); err != nil {
		return domain.RuntimeSchema{}, err
	}
	if err := repo.CreateRuntimeSchema(ctx, schema); err != nil {
		return domain.RuntimeSchema{}, err
	}
	return schema, nil
}

func (s *Service) PublishRuntimeSchema(ctx context.Context, tenantID, schemaID string, revision, expectedVersion int) (domain.RuntimeSchema, error) {
	repo, err := s.schemaRepository()
	if err != nil {
		return domain.RuntimeSchema{}, err
	}
	schema, err := repo.RuntimeSchema(ctx, tenantID, schemaID, revision)
	if err != nil {
		return domain.RuntimeSchema{}, err
	}
	if schema.Status != "draft" {
		return domain.RuntimeSchema{}, domain.Conflict("RUNTIME_SCHEMA_NOT_DRAFT", "只有 draft Runtime Schema 可以发布")
	}
	if revision > 1 {
		previous, previousErr := repo.RuntimeSchema(ctx, tenantID, schemaID, revision-1)
		if previousErr != nil || previous.Status != "published" {
			return domain.RuntimeSchema{}, domain.Conflict("RUNTIME_SCHEMA_PREVIOUS_NOT_PUBLISHED", "新 Schema 版本必须建立在已发布的前一版本上")
		}
		if (schema.Compatibility == "backward" || schema.Compatibility == "full") && !schemaDefinitionsBackwardCompatible(previous.Definition, schema.Definition) {
			return domain.RuntimeSchema{}, domain.Conflict("RUNTIME_SCHEMA_COMPATIBILITY_FAILED", "Runtime Schema 未通过 backward 兼容检查")
		}
		if schema.Compatibility == "full" && !schemaDefinitionsBackwardCompatible(schema.Definition, previous.Definition) {
			return domain.RuntimeSchema{}, domain.Conflict("RUNTIME_SCHEMA_COMPATIBILITY_FAILED", "Runtime Schema 未通过 full 兼容检查")
		}
	}
	now := s.now().UTC()
	schema.Status = "published"
	schema.PublishedAt = &now
	schema.Version++
	if err := schema.Validate(); err != nil {
		return domain.RuntimeSchema{}, err
	}
	return repo.PublishRuntimeSchema(ctx, schema, expectedVersion)
}

func (s *Service) RetireRuntimeSchema(ctx context.Context, tenantID, schemaID string, revision, expectedVersion int) (domain.RuntimeSchema, error) {
	repo, err := s.schemaRepository()
	if err != nil {
		return domain.RuntimeSchema{}, err
	}
	schema, err := repo.RuntimeSchema(ctx, tenantID, schemaID, revision)
	if err != nil {
		return domain.RuntimeSchema{}, err
	}
	if schema.Status != "published" {
		return domain.RuntimeSchema{}, domain.Conflict("RUNTIME_SCHEMA_NOT_PUBLISHED", "只有 published Runtime Schema 可以退役")
	}
	now := s.now().UTC()
	schema.Status = "retired"
	schema.RetiredAt = &now
	schema.RetainUntil = schemaRetentionDeadline(now, schema.RetentionPolicy)
	schema.Version++
	if err := schema.Validate(); err != nil {
		return domain.RuntimeSchema{}, err
	}
	return repo.RetireRuntimeSchema(ctx, schema, expectedVersion)
}

func (s *Service) RuntimeSchema(ctx context.Context, tenantID, schemaID string, revision int) (domain.RuntimeSchema, error) {
	repo, err := s.schemaRepository()
	if err != nil {
		return domain.RuntimeSchema{}, err
	}
	return repo.RuntimeSchema(ctx, tenantID, schemaID, revision)
}

func (s *Service) RuntimeSchemas(ctx context.Context, tenantID, schemaID string) ([]domain.RuntimeSchema, error) {
	repo, err := s.schemaRepository()
	if err != nil {
		return nil, err
	}
	return repo.RuntimeSchemas(ctx, tenantID, schemaID)
}

func (s *Service) schemaRepository() (Repository, error) {
	if s == nil || s.repo == nil {
		return nil, domain.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	return s.repo, nil
}

func normalizeSchemaRetention(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "job":
		return "job"
	case "30d", "90d", "forever":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

// The first registry cut only validates object shape and required fields. A
// future typed JSON Schema compiler can make this stricter without changing
// the publish/retire protocol.
func schemaDefinitionsBackwardCompatible(previous, next map[string]any) bool {
	previousType, _ := previous["type"].(string)
	nextType, _ := next["type"].(string)
	if previousType == "" || nextType == "" || previousType != nextType {
		return false
	}
	previousProperties := schemaObject(previous["properties"])
	nextProperties := schemaObject(next["properties"])
	for key, previousValue := range previousProperties {
		nextValue, exists := nextProperties[key]
		if !exists {
			continue
		}
		previousProperty := schemaObject(previousValue)
		nextProperty := schemaObject(nextValue)
		previousPropertyType, _ := previousProperty["type"].(string)
		nextPropertyType, _ := nextProperty["type"].(string)
		if previousPropertyType != "" && nextPropertyType != "" && previousPropertyType != nextPropertyType {
			return false
		}
	}
	previousRequired := schemaStringSet(previous["required"])
	for key := range schemaStringSet(next["required"]) {
		if _, existed := previousRequired[key]; !existed {
			return false
		}
	}
	return true
}

func schemaObject(value any) map[string]any {
	object, _ := value.(map[string]any)
	if object == nil {
		return map[string]any{}
	}
	return object
}

func schemaStringSet(value any) map[string]struct{} {
	result := map[string]struct{}{}
	switch values := value.(type) {
	case []string:
		for _, item := range values {
			if item = strings.TrimSpace(item); item != "" {
				result[item] = struct{}{}
			}
		}
	case []any:
		for _, value := range values {
			if item, ok := value.(string); ok {
				if item = strings.TrimSpace(item); item != "" {
					result[item] = struct{}{}
				}
			}
		}
	}
	return result
}

func schemaRetentionDeadline(retiredAt time.Time, policy string) *time.Time {
	var duration time.Duration
	switch policy {
	case "30d":
		duration = 30 * 24 * time.Hour
	case "90d":
		duration = 90 * 24 * time.Hour
	default:
		return nil
	}
	deadline := retiredAt.Add(duration)
	return &deadline
}
