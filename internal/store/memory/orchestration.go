package memory

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func cloneEnvironment(value domain.Environment) domain.Environment {
	value.NormalizeCollections()
	value.Capabilities = append([]domain.EnvironmentCapability{}, value.Capabilities...)
	return value
}

func cloneSOPVersion(value domain.SOPVersion) domain.SOPVersion {
	value.NormalizeCollections()
	value.ContentTypes = append([]string{}, value.ContentTypes...)
	value.Stages = append([]domain.StageDefinition{}, value.Stages...)
	value.Gates = append([]domain.GateDefinition{}, value.Gates...)
	for i := range value.Stages {
		value.Stages[i].OwnerRoles = append([]string{}, value.Stages[i].OwnerRoles...)
		value.Stages[i].InputRefs = append([]string{}, value.Stages[i].InputRefs...)
		value.Stages[i].RequiredCapabilities = append([]string{}, value.Stages[i].RequiredCapabilities...)
		value.Stages[i].ExecutionModes = append([]string{}, value.Stages[i].ExecutionModes...)
		value.Stages[i].Checks = append([]string{}, value.Stages[i].Checks...)
		value.Stages[i].GateIDs = append([]string{}, value.Stages[i].GateIDs...)
		value.Stages[i].AcceptedInputTypes = append([]domain.StageObjectRequirement{}, value.Stages[i].AcceptedInputTypes...)
		value.Stages[i].RequiredOutputTypes = append([]domain.StageObjectRequirement{}, value.Stages[i].RequiredOutputTypes...)
		value.Stages[i].OutputSchemaRefs = append([]string{}, value.Stages[i].OutputSchemaRefs...)
		value.Stages[i].RetryPolicy.RetryableErrorCode = append([]string{}, value.Stages[i].RetryPolicy.RetryableErrorCode...)
	}
	for i := range value.Gates {
		value.Gates[i].AssigneeRoles = append([]string{}, value.Gates[i].AssigneeRoles...)
		value.Gates[i].InputRefs = append([]string{}, value.Gates[i].InputRefs...)
		value.Gates[i].Checks = append([]string{}, value.Gates[i].Checks...)
	}
	return value
}

func cloneWorkTask(value domain.WorkTask) domain.WorkTask {
	value.InputRefs = append([]string(nil), value.InputRefs...)
	if value.RequestedOutput != nil {
		value.RequestedOutput = cloneTaskMap(value.RequestedOutput)
	}
	return value
}

func cloneConversationImport(value domain.ConversationImport) domain.ConversationImport {
	if value.Bundle == nil {
		return value
	}
	bundle := *value.Bundle
	bundle.NormalizeCollections()
	bundle.Content = append([]domain.ConversationContent{}, bundle.Content...)
	bundle.Redaction.RemovedTypes = append([]string{}, bundle.Redaction.RemovedTypes...)
	value.Bundle = &bundle
	return value
}

func cloneTaskMap(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func cloneGateEvaluation(value domain.GateEvaluation) domain.GateEvaluation {
	value.NormalizeCollections()
	value.InputRefs = append([]string{}, value.InputRefs...)
	value.Checks = cloneTaskMap(value.Checks)
	return value
}

func cloneTaskRevision(value domain.TaskRevision) domain.TaskRevision {
	value.NormalizeCollections()
	value.Content = append([]byte{}, value.Content...)
	value.KnowledgeSnapshotIDs = append([]string{}, value.KnowledgeSnapshotIDs...)
	value.EvidenceSummary = cloneTaskMap(value.EvidenceSummary)
	value.RightsSummary = cloneTaskMap(value.RightsSummary)
	return value
}

func cloneTaskDelivery(value domain.TaskDelivery) domain.TaskDelivery {
	value.NormalizeCollections()
	value.Manifest = append([]string{}, value.Manifest...)
	return value
}

func (s *Store) CreateEnvironment(_ context.Context, value domain.Environment) error {
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.environments[value.ID]; exists {
		return domain.Conflict("ENVIRONMENT_EXISTS", "环境已存在")
	}
	for _, existing := range s.environments {
		if existing.TenantID == value.TenantID && existing.Slug == value.Slug {
			return domain.Conflict("ENVIRONMENT_SLUG_EXISTS", "环境标识已存在")
		}
	}
	s.environments[value.ID] = cloneEnvironment(value)
	return nil
}

func (s *Store) Environments(_ context.Context, tenantID string) ([]domain.Environment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.Environment{}
	for _, value := range s.environments {
		if value.TenantID == tenantID {
			result = append(result, cloneEnvironment(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

func (s *Store) Environment(_ context.Context, tenantID, id string) (domain.Environment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.environments[id]
	if !ok || value.TenantID != tenantID {
		return value, domain.NotFound("环境")
	}
	return cloneEnvironment(value), nil
}

func (s *Store) SaveEnvironment(_ context.Context, value domain.Environment) error {
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.environments[value.ID]
	if !ok || current.TenantID != value.TenantID {
		return domain.NotFound("环境")
	}
	for id, existing := range s.environments {
		if id != value.ID && existing.TenantID == value.TenantID && existing.Slug == value.Slug {
			return domain.Conflict("ENVIRONMENT_SLUG_EXISTS", "环境标识已存在")
		}
	}
	s.environments[value.ID] = cloneEnvironment(value)
	return nil
}

func (s *Store) CreateSOP(_ context.Context, definition domain.SOPDefinition, version domain.SOPVersion) error {
	if err := version.Validate(); err != nil {
		return err
	}
	if definition.ID == "" || definition.TenantID == "" || definition.Name == "" || version.SOPID != definition.ID || version.TenantID != definition.TenantID {
		return domain.Invalid("SOP_INVALID", "SOP 定义和版本作用域不一致")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	definitionKey := sopDefinitionKey(definition.TenantID, definition.ID)
	if _, exists := s.sopDefinitions[definitionKey]; exists {
		return domain.Conflict("SOP_EXISTS", "SOP 已存在")
	}
	for _, existing := range s.sopVersions {
		if existing.TenantID == version.TenantID && existing.SOPID == version.SOPID && existing.Version == version.Version {
			return domain.Conflict("SOP_VERSION_EXISTS", "SOP 版本已存在")
		}
	}
	s.sopDefinitions[definitionKey] = definition
	s.sopVersions[sopVersionKey(version.TenantID, version.SOPID, version.Version)] = cloneSOPVersion(version)
	return nil
}

func (s *Store) SaveSOPDefinition(_ context.Context, value domain.SOPDefinition) error {
	if value.ID == "" || value.TenantID == "" || value.Name == "" {
		return domain.Invalid("SOP_INVALID", "SOP 定义缺少 ID、租户或名称")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	definitionKey := sopDefinitionKey(value.TenantID, value.ID)
	current, ok := s.sopDefinitions[definitionKey]
	if !ok || current.TenantID != value.TenantID {
		return domain.NotFound("流程规范")
	}
	if value.BuiltIn && value.TemplateKey != "" {
		for id, existing := range s.sopDefinitions {
			if id != definitionKey && existing.TenantID == value.TenantID && existing.BuiltIn && existing.TemplateKey == value.TemplateKey {
				return domain.Conflict("SOP_TEMPLATE_EXISTS", "同一内置 SOP 模板已存在")
			}
		}
	}
	value.NormalizeCollections()
	s.sopDefinitions[definitionKey] = value
	return nil
}

func (s *Store) CreateSOPVersion(_ context.Context, version domain.SOPVersion) error {
	if err := version.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	definition, ok := s.sopDefinitions[sopDefinitionKey(version.TenantID, version.SOPID)]
	if !ok || definition.TenantID != version.TenantID {
		return domain.NotFound("流程规范")
	}
	key := sopVersionKey(version.TenantID, version.SOPID, version.Version)
	if _, exists := s.sopVersions[key]; exists {
		return domain.Conflict("SOP_VERSION_EXISTS", "SOP 版本已存在")
	}
	s.sopVersions[key] = cloneSOPVersion(version)
	return nil
}

func sopVersionKey(tenantID, sopID string, version int) string {
	return fmt.Sprintf("%s:%s:%d", tenantID, sopID, version)
}

func sopDefinitionKey(tenantID, sopID string) string {
	return tenantID + ":" + sopID
}

func (s *Store) SOPs(_ context.Context, tenantID string) ([]domain.SOPSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.SOPSummary{}
	for _, definition := range s.sopDefinitions {
		if definition.TenantID != tenantID {
			continue
		}
		summary := domain.SOPSummary{Definition: definition, Versions: []domain.SOPVersion{}}
		for _, version := range s.sopVersions {
			if version.TenantID == tenantID && version.SOPID == definition.ID {
				summary.Versions = append(summary.Versions, cloneSOPVersion(version))
			}
		}
		sort.Slice(summary.Versions, func(i, j int) bool { return summary.Versions[i].Version > summary.Versions[j].Version })
		result = append(result, summary)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Definition.UpdatedAt.After(result[j].Definition.UpdatedAt) })
	return result, nil
}

func (s *Store) SOP(_ context.Context, tenantID, id string) (domain.SOPSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	definition, ok := s.sopDefinitions[sopDefinitionKey(tenantID, id)]
	if !ok || definition.TenantID != tenantID {
		return domain.SOPSummary{}, domain.NotFound("流程规范")
	}
	result := domain.SOPSummary{Definition: definition, Versions: []domain.SOPVersion{}}
	for _, version := range s.sopVersions {
		if version.TenantID == tenantID && version.SOPID == id {
			result.Versions = append(result.Versions, cloneSOPVersion(version))
		}
	}
	sort.Slice(result.Versions, func(i, j int) bool { return result.Versions[i].Version > result.Versions[j].Version })
	return result, nil
}

func (s *Store) SaveSOPVersion(_ context.Context, value domain.SOPVersion) error {
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := sopVersionKey(value.TenantID, value.SOPID, value.Version)
	current, ok := s.sopVersions[key]
	if !ok {
		return domain.NotFound("流程规范版本")
	}
	if current.Status != "draft" {
		return domain.Conflict("SOP_VERSION_IMMUTABLE", "已发布或已退休的 SOP 版本不可修改")
	}
	s.sopVersions[key] = cloneSOPVersion(value)
	return nil
}

func (s *Store) PublishSOPVersion(_ context.Context, tenantID, sopID string, version int, publishedBy string, publishedAt time.Time) (domain.SOPVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := sopVersionKey(tenantID, sopID, version)
	value, ok := s.sopVersions[key]
	if !ok {
		return value, domain.NotFound("流程规范版本")
	}
	if err := value.Validate(); err != nil {
		return value, err
	}
	digest, err := value.ContentDigest()
	if err != nil {
		return value, err
	}
	value.Digest = "sha256:" + digest
	value.Status = "published"
	value.PublishedBy = publishedBy
	value.PublishedAt = &publishedAt
	s.sopVersions[key] = cloneSOPVersion(value)
	definitionKey := sopDefinitionKey(tenantID, sopID)
	definition := s.sopDefinitions[definitionKey]
	definition.CurrentVersion = version
	definition.UpdatedAt = publishedAt
	s.sopDefinitions[definitionKey] = definition
	return cloneSOPVersion(value), nil
}

func (s *Store) RetireSOPVersion(_ context.Context, tenantID, sopID string, version int, retiredAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := sopVersionKey(tenantID, sopID, version)
	value, ok := s.sopVersions[key]
	if !ok {
		return domain.NotFound("流程规范版本")
	}
	if value.Status != "published" {
		return domain.Conflict("SOP_VERSION_STATE_INVALID", "只有已发布 SOP 版本可以退休")
	}
	definition, ok := s.sopDefinitions[sopDefinitionKey(tenantID, sopID)]
	if !ok {
		return domain.NotFound("流程规范")
	}
	if definition.CurrentVersion == version {
		return domain.Policy("SOP_CURRENT_VERSION_CANNOT_RETIRE", "当前生效版本不能直接退休", "先发布另一个版本或执行回滚")
	}
	value.Status = "retired"
	s.sopVersions[key] = cloneSOPVersion(value)
	definition.UpdatedAt = retiredAt
	s.sopDefinitions[sopDefinitionKey(tenantID, sopID)] = definition
	return nil
}

func (s *Store) SaveProjectSOPBinding(_ context.Context, value domain.ProjectSOPBinding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.projects[value.ProjectID]; !ok {
		return domain.NotFound("项目")
	}
	if _, ok := s.sopVersions[sopVersionKey(value.TenantID, value.SOPID, value.SOPVersion)]; !ok {
		return domain.NotFound("流程规范版本")
	}
	s.projectSOPBindings[value.ProjectID] = value
	return nil
}

func (s *Store) ProjectSOPBinding(_ context.Context, tenantID, projectID string) (domain.ProjectSOPBinding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.projectSOPBindings[projectID]
	if !ok || value.TenantID != tenantID {
		return value, domain.NotFound("项目 SOP")
	}
	return value, nil
}

func (s *Store) ProjectSOPBindings(_ context.Context, tenantID string) ([]domain.ProjectSOPBinding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.ProjectSOPBinding{}
	for _, value := range s.projectSOPBindings {
		if value.TenantID == tenantID {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].BoundAt.Before(result[j].BoundAt) })
	return result, nil
}

func (s *Store) CreateWorkTask(_ context.Context, value domain.WorkTask) error {
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.workTasks[value.ID]; exists {
		return domain.Conflict("TASK_EXISTS", "任务已存在")
	}
	if value.IdempotencyKey != "" {
		for _, existing := range s.workTasks {
			if existing.TenantID == value.TenantID && existing.IdempotencyKey == value.IdempotencyKey {
				return domain.Conflict("IDEMPOTENCY_REPLAY", "相同幂等键已经创建过任务")
			}
		}
	}
	s.workTasks[value.ID] = cloneWorkTask(value)
	return nil
}

func (s *Store) WorkTaskByIdempotencyKey(_ context.Context, tenantID, key string) (domain.WorkTask, error) {
	if key == "" {
		return domain.WorkTask{}, domain.NotFound("任务")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, value := range s.workTasks {
		if value.TenantID == tenantID && value.IdempotencyKey == key {
			return cloneWorkTask(value), nil
		}
	}
	return domain.WorkTask{}, domain.NotFound("任务")
}

func (s *Store) WorkTasks(_ context.Context, tenantID, projectID string) ([]domain.WorkTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.WorkTask{}
	for _, value := range s.workTasks {
		if value.TenantID == tenantID && (projectID == "" || value.ProjectID == projectID) {
			result = append(result, cloneWorkTask(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.After(result[j].UpdatedAt) })
	return result, nil
}

func (s *Store) CreateConversationImport(_ context.Context, value domain.ConversationImport) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.conversationImports[value.ID]; exists {
		return domain.Conflict("CONVERSATION_IMPORT_EXISTS", "对话导入请求已存在")
	}
	if value.IdempotencyKey != "" {
		for _, existing := range s.conversationImports {
			if existing.TenantID == value.TenantID && existing.IdempotencyKey == value.IdempotencyKey {
				return domain.Conflict("IDEMPOTENCY_REPLAY", "相同幂等键已经创建过对话导入请求")
			}
		}
	}
	s.conversationImports[value.ID] = cloneConversationImport(value)
	return nil
}

func (s *Store) ConversationImport(_ context.Context, tenantID, id string) (domain.ConversationImport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.conversationImports[id]
	if !ok || value.TenantID != tenantID {
		return value, domain.NotFound("对话导入请求")
	}
	return cloneConversationImport(value), nil
}

func (s *Store) ConversationImportByIdempotencyKey(_ context.Context, tenantID, key string) (domain.ConversationImport, error) {
	if key == "" {
		return domain.ConversationImport{}, domain.NotFound("对话导入请求")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, value := range s.conversationImports {
		if value.TenantID == tenantID && value.IdempotencyKey == key {
			return cloneConversationImport(value), nil
		}
	}
	return domain.ConversationImport{}, domain.NotFound("对话导入请求")
}

func (s *Store) ConversationImportsForTask(_ context.Context, tenantID, taskID string) ([]domain.ConversationImport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.ConversationImport{}
	for _, value := range s.conversationImports {
		if value.TenantID == tenantID && value.TaskID == taskID {
			result = append(result, cloneConversationImport(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

func (s *Store) SaveConversationImport(_ context.Context, value domain.ConversationImport) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.conversationImports[value.ID]
	if !ok || current.TenantID != value.TenantID {
		return domain.NotFound("对话导入请求")
	}
	if current.Status == domain.ConversationImportUploaded && value.Status != current.Status {
		return domain.Conflict("CONVERSATION_IMPORT_IMMUTABLE", "已上传 Bundle 的对话导入请求不可再次改变状态")
	}
	s.conversationImports[value.ID] = cloneConversationImport(value)
	return nil
}

func (s *Store) WorkTask(_ context.Context, tenantID, id string) (domain.WorkTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.workTasks[id]
	if !ok || value.TenantID != tenantID {
		return value, domain.NotFound("任务")
	}
	return cloneWorkTask(value), nil
}

func (s *Store) SaveWorkTask(_ context.Context, value domain.WorkTask) error {
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.workTasks[value.ID]
	if !ok || current.TenantID != value.TenantID {
		return domain.NotFound("任务")
	}
	s.workTasks[value.ID] = cloneWorkTask(value)
	return nil
}

func (s *Store) StageRuns(_ context.Context, tenantID, taskID string) ([]domain.StageRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.StageRun{}
	for _, value := range s.stageRuns {
		if value.TenantID == tenantID && value.TaskID == taskID {
			value.Outputs = []domain.TaskStageOutput{}
			for _, output := range s.stageOutputs {
				if output.StageRunID == value.ID {
					value.Outputs = append(value.Outputs, cloneStageOutput(output))
				}
			}
			sort.Slice(value.Outputs, func(i, j int) bool { return value.Outputs[i].CreatedAt.Before(value.Outputs[j].CreatedAt) })
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.Before(result[j].UpdatedAt) })
	return result, nil
}

func (s *Store) CreateStageRun(_ context.Context, value domain.StageRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.stageRuns[value.ID]; exists {
		return domain.Conflict("STAGE_RUN_EXISTS", "阶段执行记录已存在")
	}
	s.stageRuns[value.ID] = value
	return nil
}

func (s *Store) SaveStageRun(_ context.Context, value domain.StageRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.stageRuns[value.ID]; !exists {
		return domain.NotFound("阶段执行记录")
	}
	s.stageRuns[value.ID] = value
	return nil
}

func (s *Store) WorkTaskRuns(_ context.Context, tenantID, taskID string) ([]domain.TaskRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.TaskRun{}
	for _, value := range s.runs {
		if value.TenantID == tenantID && value.WorkTaskID == taskID {
			value.OutputRefs = append([]string{}, value.OutputRefs...)
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

func (s *Store) CreateGateEvaluation(_ context.Context, value domain.GateEvaluation) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.gateEvaluations[value.ID]; exists {
		return domain.Conflict("GATE_EVALUATION_EXISTS", "审核门评估已存在")
	}
	s.gateEvaluations[value.ID] = cloneGateEvaluation(value)
	return nil
}

func (s *Store) GateEvaluations(_ context.Context, tenantID, taskID string) ([]domain.GateEvaluation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.GateEvaluation{}
	for _, value := range s.gateEvaluations {
		if value.TenantID == tenantID && value.TaskID == taskID {
			result = append(result, cloneGateEvaluation(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

func (s *Store) GateEvaluation(_ context.Context, tenantID, id string) (domain.GateEvaluation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.gateEvaluations[id]
	if !ok || value.TenantID != tenantID {
		return value, domain.NotFound("审核门评估")
	}
	return cloneGateEvaluation(value), nil
}

func (s *Store) SaveGateEvaluation(_ context.Context, value domain.GateEvaluation) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.gateEvaluations[value.ID]
	if !ok || current.TenantID != value.TenantID {
		return domain.NotFound("审核门评估")
	}
	s.gateEvaluations[value.ID] = cloneGateEvaluation(value)
	return nil
}

func (s *Store) CreateTaskRevision(_ context.Context, value domain.TaskRevision) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.taskRevisions[value.ID]; exists {
		return domain.Conflict("TASK_REVISION_EXISTS", "任务版本已存在")
	}
	for _, existing := range s.taskRevisions {
		if existing.TenantID == value.TenantID && existing.TaskID == value.TaskID && existing.RevisionNo == value.RevisionNo {
			return domain.Conflict("TASK_REVISION_NUMBER_EXISTS", "任务版本编号已存在")
		}
	}
	s.taskRevisions[value.ID] = cloneTaskRevision(value)
	return nil
}

func (s *Store) TaskRevisions(_ context.Context, tenantID, taskID string) ([]domain.TaskRevision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.TaskRevision{}
	for _, value := range s.taskRevisions {
		if value.TenantID == tenantID && value.TaskID == taskID {
			result = append(result, cloneTaskRevision(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].RevisionNo < result[j].RevisionNo })
	return result, nil
}

func (s *Store) TaskRevision(_ context.Context, tenantID, id string) (domain.TaskRevision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.taskRevisions[id]
	if !ok || value.TenantID != tenantID {
		return value, domain.NotFound("任务版本")
	}
	return cloneTaskRevision(value), nil
}

func (s *Store) CreateTaskDelivery(_ context.Context, value domain.TaskDelivery) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.taskDeliveries[value.ID]; exists {
		return domain.Conflict("TASK_DELIVERY_EXISTS", "任务交付已存在")
	}
	s.taskDeliveries[value.ID] = cloneTaskDelivery(value)
	return nil
}

func (s *Store) TaskDeliveries(_ context.Context, tenantID, taskID string) ([]domain.TaskDelivery, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.TaskDelivery{}
	for _, value := range s.taskDeliveries {
		if value.TenantID == tenantID && value.TaskID == taskID {
			result = append(result, cloneTaskDelivery(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

func (s *Store) TaskDelivery(_ context.Context, tenantID, id string) (domain.TaskDelivery, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.taskDeliveries[id]
	if !ok || value.TenantID != tenantID {
		return value, domain.NotFound("任务交付")
	}
	return cloneTaskDelivery(value), nil
}

func (s *Store) SaveTaskDelivery(_ context.Context, value domain.TaskDelivery) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.taskDeliveries[value.ID]
	if !ok || current.TenantID != value.TenantID {
		return domain.NotFound("任务交付")
	}
	s.taskDeliveries[value.ID] = cloneTaskDelivery(value)
	return nil
}
