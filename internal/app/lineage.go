package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

type LineageQuery struct {
	FocusType string `json:"focus_type,omitempty"`
	FocusID   string `json:"focus_id,omitempty"`
	Direction string `json:"direction,omitempty"`
}

type lineageBuilder struct {
	nodes map[string]domain.LineageNode
	edges []domain.LineageEdge
}

func newLineageBuilder() *lineageBuilder {
	return &lineageBuilder{nodes: map[string]domain.LineageNode{}, edges: []domain.LineageEdge{}}
}

func lineageKey(objectType, id string) string {
	return objectType + ":" + id
}

func (b *lineageBuilder) node(objectType, id, label, status, stage string, createdAt time.Time, metadata map[string]any) {
	if id == "" {
		return
	}
	b.nodes[lineageKey(objectType, id)] = domain.LineageNode{Key: lineageKey(objectType, id), Type: objectType, ID: id, Label: label, Status: status, Stage: stage, CreatedAt: createdAt, Metadata: metadata}
}

func (b *lineageBuilder) edge(fromType, fromID, toType, toID, relation, reason string) {
	if fromID == "" || toID == "" {
		return
	}
	b.edges = append(b.edges, domain.LineageEdge{From: lineageKey(fromType, fromID), To: lineageKey(toType, toID), Relation: relation, Reason: reason})
}

func (s *Service) ProjectLineage(ctx context.Context, actor Actor, projectID string, query LineageQuery) (domain.LineageGraph, error) {
	if _, err := s.store.Project(ctx, actor.TenantID, projectID); err != nil {
		return domain.LineageGraph{}, err
	}
	query.FocusType = strings.TrimSpace(query.FocusType)
	query.FocusID = strings.TrimSpace(query.FocusID)
	query.Direction = strings.TrimSpace(query.Direction)
	if (query.FocusType == "") != (query.FocusID == "") {
		return domain.LineageGraph{}, domain.Invalid("LINEAGE_FOCUS_INVALID", "追踪对象类型（focus_type）和标识（focus_id）必须同时提供")
	}
	if query.Direction == "" {
		query.Direction = "both"
	}
	if query.Direction != "both" && query.Direction != "upstream" && query.Direction != "downstream" {
		return domain.LineageGraph{}, domain.Invalid("LINEAGE_DIRECTION_INVALID", "追踪方向（direction）必须是“上游（upstream）”“下游（downstream）”或“双向（both）”")
	}

	builder, err := s.buildProjectLineage(ctx, actor.TenantID, projectID)
	if err != nil {
		return domain.LineageGraph{}, err
	}
	return builder.graph(projectID, query, s.now().UTC())
}

func (s *Service) ProjectImpact(ctx context.Context, actor Actor, projectID string, query LineageQuery) (domain.ImpactAnalysis, error) {
	query.Direction = "downstream"
	graph, err := s.ProjectLineage(ctx, actor, projectID, query)
	if err != nil {
		return domain.ImpactAnalysis{}, err
	}
	analysis := domain.ImpactAnalysis{ProjectID: projectID, Items: []domain.ImpactItem{}, GeneratedAt: graph.GeneratedAt}
	if graph.FocusKey == "" {
		for _, node := range graph.Nodes {
			severity, action, include := lineageAdvisory(node)
			if !include {
				continue
			}
			analysis.Items = append(analysis.Items, domain.ImpactItem{Node: node, Severity: severity, Reason: "对象当前状态需要人工处理", CurrentStatus: node.Status, RecommendedAction: action})
		}
		return analysis, nil
	}

	nodeByKey := make(map[string]domain.LineageNode, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodeByKey[node.Key] = node
		if node.Key == graph.FocusKey {
			focus := node
			analysis.Focus = &focus
		}
	}
	depth, reasons := downstreamDepth(graph.FocusKey, graph.Edges)
	for _, node := range graph.Nodes {
		if node.Key == graph.FocusKey {
			continue
		}
		itemDepth, ok := depth[node.Key]
		if !ok {
			continue
		}
		severity, action, needsAttention := lineageAdvisory(node)
		if !needsAttention {
			severity = "review"
			action = lineageReviewAction(node.Type)
		}
		analysis.Items = append(analysis.Items, domain.ImpactItem{Node: node, Depth: itemDepth, Severity: severity, Reason: reasons[node.Key], CurrentStatus: node.Status, RecommendedAction: action})
	}
	sort.Slice(analysis.Items, func(i, j int) bool {
		if analysis.Items[i].Depth != analysis.Items[j].Depth {
			return analysis.Items[i].Depth < analysis.Items[j].Depth
		}
		return analysis.Items[i].Node.Key < analysis.Items[j].Node.Key
	})
	return analysis, nil
}

func (s *Service) buildProjectLineage(ctx context.Context, tenantID, projectID string) (*lineageBuilder, error) {
	b := newLineageBuilder()
	sources, err := s.store.Sources(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	revisions := []domain.SourceRevision{}
	for _, source := range sources {
		b.node("source", source.ID, source.Name, source.Status, "sources", source.CreatedAt, map[string]any{"source_type": source.SourceType, "revision_count": source.RevisionCount})
		items, loadErr := s.store.SourceRevisions(ctx, tenantID, source.ID)
		if loadErr != nil {
			return nil, loadErr
		}
		revisions = append(revisions, items...)
	}
	for _, revision := range revisions {
		b.node("source_revision", revision.ID, revision.FileName, revision.ProcessingStatus, "sources", revision.CreatedAt, map[string]any{"sha256": revision.SHA256, "parser_version": revision.ParserVersion})
		b.edge("source", revision.SourceID, "source_revision", revision.ID, "has_revision", "来源包含不可变修订")
		b.edge("source_revision", revision.SupersedesID, "source_revision", revision.ID, "supersedes", "新修订替代上一修订")
	}

	assets, err := s.store.Assets(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	for _, asset := range assets {
		b.node("asset", asset.ID, asset.Name, asset.Status, "knowledge", asset.CreatedAt, map[string]any{"asset_type": asset.AssetType, "usage_mode": asset.UsageMode})
		b.edge("source_revision", asset.SourceRevisionID, "asset", asset.ID, "registered_as", "来源修订登记为素材")
	}
	rights, err := s.store.RightsRecords(ctx, tenantID, "")
	if err != nil {
		return nil, err
	}
	for _, record := range rights {
		if record.ProjectID != projectID {
			continue
		}
		b.node("rights_record", record.ID, record.RightsHolder, record.Status, "knowledge", record.CreatedAt, map[string]any{"rights_type": record.RightsType})
		b.edge("asset", record.AssetID, "rights_record", record.ID, "governed_by", "素材使用受权利记录约束")
		b.edge("source_revision", record.ProofSourceRevisionID, "rights_record", record.ID, "proves", "来源修订提供权利证明")
	}

	knowledge, err := s.store.KnowledgeObjects(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	for _, object := range knowledge {
		b.node("knowledge_object", object.ID, firstNonEmpty(object.Title, object.Statement), object.Status, "knowledge", object.CreatedAt, map[string]any{"object_type": object.ObjectType, "layer": object.Layer})
		if origin, _ := object.Payload["origin_run_id"].(string); origin != "" {
			b.edge("task_run", origin, "knowledge_object", object.ID, "produces", "Automation Run 产出知识候选")
		}
		for _, evidenceID := range object.EvidenceRefs {
			span, spanErr := s.store.EvidenceSpan(ctx, tenantID, evidenceID)
			if spanErr == nil {
				b.edge("source_revision", span.RevisionID, "knowledge_object", object.ID, "supports", "证据支持知识对象")
			}
		}
		switch dependencies := object.Payload["depends_on_fact_ids"].(type) {
		case []string:
			for _, dependencyID := range dependencies {
				b.edge("knowledge_object", dependencyID, "knowledge_object", object.ID, "depends_on", "知识对象依赖另一事实")
			}
		case []any:
			for _, dependency := range dependencies {
				if dependencyID, ok := dependency.(string); ok {
					b.edge("knowledge_object", dependencyID, "knowledge_object", object.ID, "depends_on", "知识对象依赖另一事实")
				}
			}
		}
	}

	runs, err := s.taskRunsForProject(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		b.node("task_run", run.ID, runLabel(run), run.State, "automation", run.CreatedAt, map[string]any{"task_type": run.TaskType, "capability_id": run.CapabilityID, "error_code": run.ErrorCode})
		if snapshot, loadErr := s.store.Snapshot(ctx, tenantID, run.InputSnapshotID); loadErr == nil {
			for _, source := range snapshot.Sources {
				b.edge("source_revision", source.RevisionID, "task_run", run.ID, "frozen_into", "来源版本已固定到自动化执行契约")
			}
		}
	}
	approvedSnapshots, err := s.store.ApprovedSnapshots(ctx, tenantID, projectID, "")
	if err != nil {
		return nil, err
	}
	for _, snapshot := range approvedSnapshots {
		b.node("approved_snapshot", snapshot.ID, fmt.Sprintf("%s · 已批准快照", snapshot.SubmissionType), "approved", "approval", snapshot.CreatedAt, map[string]any{"content_hash": snapshot.ContentHash})
		b.edge("submission_revision", snapshot.SubmissionRevisionID, "approved_snapshot", snapshot.ID, "approved_as", "客户批准后，将不可变内容版本固化为快照")
	}

	for _, snapshot := range approvedSnapshots {
		artifacts, loadErr := s.store.ArtifactsByApprovedSnapshot(ctx, tenantID, snapshot.ID)
		if loadErr != nil {
			return nil, loadErr
		}
		for _, artifact := range artifacts {
			b.node("artifact", artifact.ID, artifact.FileName, artifact.Kind, "delivery", artifact.CreatedAt, map[string]any{"kind": artifact.Kind, "media_type": artifact.MediaType, "schema_id": artifact.SchemaID})
			b.edge("approved_snapshot", artifact.ApprovedSnapshotID, "artifact", artifact.ID, "exported_as", "批准快照确定性导出为交付工件")
		}
	}
	deliveryPackages, err := s.store.DeliveryPackages(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	for _, delivery := range deliveryPackages {
		b.node("delivery_package", delivery.ID, "交付包 · "+delivery.ContentItemID, delivery.Status, "delivery", delivery.CreatedAt, map[string]any{"content_item_id": delivery.ContentItemID, "artifact_count": len(delivery.Manifest)})
		for _, snapshotID := range delivery.ApprovedSnapshotIDs {
			b.edge("approved_snapshot", snapshotID, "delivery_package", delivery.ID, "delivered_as", "客户批准快照组成正式交付包")
		}
		for _, artifact := range delivery.Manifest {
			b.edge("delivery_package", delivery.ID, "artifact", artifact.ID, "contains", "交付包包含确定性格式文件")
		}
	}

	batches, err := s.store.PerformanceImportBatches(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	for _, batch := range batches {
		b.node("performance_import_batch", batch.ID, batch.SourceName, batch.Status, "results", batch.CreatedAt, map[string]any{"row_count": batch.RowCount, "currency": batch.Currency})
	}
	observations, err := s.store.PerformanceObservations(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	for _, observation := range observations {
		b.node("performance_observation", observation.ID, observationLabel(observation), observation.SampleStatus, "results", observation.CreatedAt, map[string]any{"platform": observation.Platform, "window_hours": observation.WindowHours, "roi": observation.ROI})
		b.edge("performance_import_batch", observation.ImportBatchID, "performance_observation", observation.ID, "imports", "导入批次包含效果观察")
		b.edge("approved_snapshot", observation.ApprovedSnapshotID, "performance_observation", observation.ID, "measured_by", "效果数据度量已批准快照")
	}
	ratings, err := s.store.RatingDecisions(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	for _, rating := range ratings {
		b.node("rating_decision", rating.ID, ratingLabel(rating), rating.Rating, "learning", rating.CreatedAt, map[string]any{"subject_type": rating.SubjectType, "subject_id": rating.SubjectID, "next_action": rating.NextAction})
		b.edge(rating.SubjectType, rating.SubjectID, "rating_decision", rating.ID, "rated_by", "人工决策评价业务对象")
		for _, observationID := range rating.ObservationIDs {
			b.edge("performance_observation", observationID, "rating_decision", rating.ID, "evidence_for", "效果观察作为人工评级证据")
		}
	}
	return b, nil
}

func (b *lineageBuilder) graph(projectID string, query LineageQuery, generatedAt time.Time) (domain.LineageGraph, error) {
	focusKey := ""
	selected := map[string]bool{}
	if query.FocusID != "" {
		focusKey = lineageKey(query.FocusType, query.FocusID)
		if _, ok := b.nodes[focusKey]; !ok {
			return domain.LineageGraph{}, domain.NotFound("追踪对象")
		}
		selected = connectedKeys(focusKey, b.edges, query.Direction)
	} else {
		for key := range b.nodes {
			selected[key] = true
		}
	}
	nodes := make([]domain.LineageNode, 0, len(selected))
	stageCount := map[string]int{}
	for key := range selected {
		if node, ok := b.nodes[key]; ok {
			nodes = append(nodes, node)
			stageCount[node.Stage]++
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		if lineageStageOrder(nodes[i].Stage) != lineageStageOrder(nodes[j].Stage) {
			return lineageStageOrder(nodes[i].Stage) < lineageStageOrder(nodes[j].Stage)
		}
		if !nodes[i].CreatedAt.Equal(nodes[j].CreatedAt) {
			return nodes[i].CreatedAt.Before(nodes[j].CreatedAt)
		}
		return nodes[i].Key < nodes[j].Key
	})
	edges := make([]domain.LineageEdge, 0, len(b.edges))
	seen := map[string]bool{}
	for _, edge := range b.edges {
		if _, fromExists := b.nodes[edge.From]; !fromExists || !selected[edge.From] {
			continue
		}
		if _, toExists := b.nodes[edge.To]; !toExists || !selected[edge.To] {
			continue
		}
		key := edge.From + "\x00" + edge.To + "\x00" + edge.Relation
		if seen[key] {
			continue
		}
		seen[key] = true
		edges = append(edges, edge)
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		if edges[i].To != edges[j].To {
			return edges[i].To < edges[j].To
		}
		return edges[i].Relation < edges[j].Relation
	})
	return domain.LineageGraph{ProjectID: projectID, FocusKey: focusKey, Direction: query.Direction, Nodes: nodes, Edges: edges, StageCount: stageCount, GeneratedAt: generatedAt}, nil
}

func connectedKeys(focus string, edges []domain.LineageEdge, direction string) map[string]bool {
	selected := map[string]bool{focus: true}
	queue := []string{focus}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, edge := range edges {
			next := ""
			if direction != "upstream" && edge.From == current {
				next = edge.To
			}
			if direction != "downstream" && edge.To == current {
				next = edge.From
			}
			if next != "" && !selected[next] {
				selected[next] = true
				queue = append(queue, next)
			}
		}
	}
	return selected
}

func downstreamDepth(focus string, edges []domain.LineageEdge) (map[string]int, map[string]string) {
	depth := map[string]int{focus: 0}
	reasons := map[string]string{}
	queue := []string{focus}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, edge := range edges {
			if edge.From != current {
				continue
			}
			if _, exists := depth[edge.To]; exists {
				continue
			}
			depth[edge.To] = depth[current] + 1
			reasons[edge.To] = edge.Reason
			queue = append(queue, edge.To)
		}
	}
	return depth, reasons
}

func lineageAdvisory(node domain.LineageNode) (severity, action string, include bool) {
	status := strings.ToLower(node.Status)
	switch status {
	case "failed", "blocked", "rejected", "conflicted", "discarded", "expired", "revoked", "invalid":
		return "blocked", lineageReviewAction(node.Type), true
	case "needs_review", "review_required", "revision_requested", "candidate", "draft", "pending", "insufficient_sample", "repairable":
		return "attention", lineageReviewAction(node.Type), true
	default:
		return "", "", false
	}
}

func lineageReviewAction(objectType string) string {
	switch objectType {
	case "source", "source_revision":
		return "复核来源修订并重新解析证据"
	case "asset", "rights_record":
		return "复核素材权利范围和有效期"
	case "knowledge_item":
		return "重新审核知识项及其证据"
	case "submission_revision":
		return "创建新的提交内容版本并重新提交审核"
	case "approved_snapshot":
		return "基于新的批准决策创建 ApprovedSnapshot"
	case "task_run":
		return "检查本地客户端状态后重试任务"
	case "artifact":
		return "基于有效 ApprovedSnapshot 重新导出"
	case "performance_observation":
		return "补足样本窗口后重新导入结果"
	case "rating_decision":
		return "结合新观察创建追加式评级决策"
	default:
		return "由项目负责人复核并确认后续动作"
	}
}

func lineageStageOrder(stage string) int {
	order := map[string]int{"sources": 0, "knowledge": 1, "automation": 2, "submission": 3, "approval": 4, "delivery": 5, "results": 6, "learning": 7}
	if value, ok := order[stage]; ok {
		return value
	}
	return 99
}

func runLabel(run domain.TaskRun) string { return run.TaskType }

func observationLabel(observation domain.PerformanceObservation) string {
	return fmt.Sprintf("%s · %dh · %s", observation.Platform, observation.WindowHours, observation.AccountAlias)
}

func ratingLabel(rating domain.RatingDecision) string {
	return fmt.Sprintf("%s · %s", rating.SubjectType, rating.Rating)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "未命名对象"
}
