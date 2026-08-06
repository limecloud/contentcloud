package runtime

import (
	"sort"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
)

// GraphPatch is the only shape a supervisor may submit to extend an execution
// graph. It has no delete or update operation for already published nodes.
type GraphPatch struct {
	ExpectedGraphVersion  int                  `json:"expected_graph_version"`
	IdempotencyKey        string               `json:"idempotency_key"`
	Reason                string               `json:"reason"`
	AddNodes              []domain.JobPlanNode `json:"add_nodes"`
	AddEdges              []domain.JobPlanEdge `json:"add_edges"`
	CancelPendingNodeKeys []string             `json:"cancel_pending_node_keys"`
}

type GraphPatchResult struct {
	Plan                  domain.JobPlanRevision `json:"plan"`
	GraphVersion          int                    `json:"graph_version"`
	CancelPendingNodeKeys []string               `json:"cancel_pending_node_keys"`
}

// ApplyGraphPatch validates and materializes one immutable plan revision. The
// caller persists the returned plan and graph version in one transaction; this
// function itself has no storage side effects.
func ApplyGraphPatch(plan domain.JobPlanRevision, currentGraphVersion int, patch GraphPatch) (GraphPatchResult, error) {
	if currentGraphVersion < 1 || patch.ExpectedGraphVersion != currentGraphVersion {
		return GraphPatchResult{}, domain.Conflict("GRAPH_VERSION_CONFLICT", "执行图版本已经变化，请重新读取后再提交")
	}
	if strings.TrimSpace(patch.IdempotencyKey) == "" || strings.TrimSpace(patch.Reason) == "" {
		return GraphPatchResult{}, domain.Invalid("GRAPH_PATCH_INVALID", "执行图变更需要幂等键和原因")
	}
	plan.NormalizeCollections()
	existing := map[string]domain.JobPlanNode{}
	for _, node := range plan.Nodes {
		existing[node.Key] = node
	}
	added := map[string]domain.JobPlanNode{}
	for _, node := range patch.AddNodes {
		if strings.TrimSpace(node.Key) == "" || strings.TrimSpace(node.Name) == "" || strings.TrimSpace(node.OutputSchema) == "" || node.Kind == "" {
			return GraphPatchResult{}, domain.Invalid("GRAPH_PATCH_NODE_INVALID", "新增节点必须有唯一 Key、类型、名称和输出 Schema")
		}
		if _, ok := existing[node.Key]; ok {
			return GraphPatchResult{}, domain.Conflict("GRAPH_PATCH_NODE_EXISTS", "GraphPatch 不能修改或重复添加已有节点")
		}
		if _, ok := added[node.Key]; ok {
			return GraphPatchResult{}, domain.Conflict("GRAPH_PATCH_NODE_EXISTS", "GraphPatch 包含重复节点")
		}
		node.DependsOn = normalizeKeys(node.DependsOn)
		added[node.Key] = node
	}
	if plan.Limits.MaxDynamicDescendants > 0 && len(added) > plan.Limits.MaxDynamicDescendants {
		return GraphPatchResult{}, domain.Invalid("GRAPH_PATCH_DESCENDANT_LIMIT", "GraphPatch 超过动态后代数量上限")
	}

	known := map[string]bool{}
	for key := range existing {
		known[key] = true
	}
	for key := range added {
		known[key] = true
	}
	edges := append([]domain.JobPlanEdge{}, plan.Edges...)
	seenEdges := map[string]bool{}
	for _, edge := range edges {
		seenEdges[edge.From+"\x00"+edge.To] = true
	}
	for _, edge := range patch.AddEdges {
		if _, downstreamAdded := added[edge.To]; edge.From == edge.To || !known[edge.From] || !known[edge.To] || !downstreamAdded {
			return GraphPatchResult{}, domain.Invalid("GRAPH_PATCH_EDGE_INVALID", "新增边必须引用已知节点，且下游必须是新增节点")
		}
		key := edge.From + "\x00" + edge.To
		if seenEdges[key] {
			return GraphPatchResult{}, domain.Conflict("GRAPH_PATCH_EDGE_EXISTS", "GraphPatch 包含重复边")
		}
		seenEdges[key] = true
		edges = append(edges, edge)
		for key, node := range added {
			if key == edge.To && !contains(node.DependsOn, edge.From) {
				node.DependsOn = append(node.DependsOn, edge.From)
				node.DependsOn = normalizeKeys(node.DependsOn)
				added[key] = node
			}
		}
	}
	for key, node := range added {
		for _, dependency := range node.DependsOn {
			if !known[dependency] {
				return GraphPatchResult{}, domain.Invalid("GRAPH_PATCH_DEPENDENCY_INVALID", "新增节点依赖了不存在的节点: "+dependency)
			}
			depEdge := dependency + "\x00" + key
			if !seenEdges[depEdge] {
				seenEdges[depEdge] = true
				edges = append(edges, domain.JobPlanEdge{From: dependency, To: key})
			}
		}
	}

	nodes := append([]domain.JobPlanNode{}, plan.Nodes...)
	for _, node := range added {
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Key < nodes[j].Key })
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From == edges[j].From {
			return edges[i].To < edges[j].To
		}
		return edges[i].From < edges[j].From
	})
	candidate := plan
	candidate.ID = domain.NewID()
	candidate.Nodes = nodes
	candidate.Edges = edges
	candidate.CompiledBy = "runtime.graph_patch"
	// Validate shape before replacing the digest with the immutable patch digest.
	candidate.Digest = plan.Digest
	if err := candidate.Validate(); err != nil {
		return GraphPatchResult{}, err
	}
	depth, err := graphDepth(candidate.Nodes)
	if err != nil {
		return GraphPatchResult{}, err
	}
	if candidate.Limits.MaxDepth > 0 && depth > candidate.Limits.MaxDepth {
		return GraphPatchResult{}, domain.Invalid("GRAPH_PATCH_DEPTH_LIMIT", "GraphPatch 超过执行图深度上限")
	}
	digest, err := domain.CanonicalHash(struct {
		BaseDigest string
		Reason     string
		Key        string
		Nodes      []domain.JobPlanNode
		Edges      []domain.JobPlanEdge
	}{plan.Digest, strings.TrimSpace(patch.Reason), strings.TrimSpace(patch.IdempotencyKey), nodes, edges})
	if err != nil {
		return GraphPatchResult{}, err
	}
	candidate.Digest = "sha256:" + digest
	cancel := normalizeKeys(patch.CancelPendingNodeKeys)
	for _, key := range cancel {
		if _, ok := existing[key]; !ok {
			return GraphPatchResult{}, domain.Invalid("GRAPH_PATCH_CANCEL_INVALID", "只能标记已有节点取消: "+key)
		}
	}
	return GraphPatchResult{Plan: candidate, GraphVersion: currentGraphVersion + 1, CancelPendingNodeKeys: cancel}, nil
}

func graphDepth(nodes []domain.JobPlanNode) (int, error) {
	byKey := map[string]domain.JobPlanNode{}
	for _, node := range nodes {
		byKey[node.Key] = node
	}
	depth := map[string]int{}
	visiting := map[string]bool{}
	var visit func(string) (int, error)
	visit = func(key string) (int, error) {
		if value, ok := depth[key]; ok {
			return value, nil
		}
		if visiting[key] {
			return 0, domain.Invalid("GRAPH_PATCH_CYCLE", "执行图不能包含环")
		}
		node, ok := byKey[key]
		if !ok {
			return 0, domain.Invalid("GRAPH_PATCH_DEPENDENCY_INVALID", "执行图依赖节点不存在")
		}
		visiting[key] = true
		value := 1
		for _, dependency := range node.DependsOn {
			parent, err := visit(dependency)
			if err != nil {
				return 0, err
			}
			if parent+1 > value {
				value = parent + 1
			}
		}
		delete(visiting, key)
		depth[key] = value
		return value, nil
	}
	max := 0
	for key := range byKey {
		value, err := visit(key)
		if err != nil {
			return 0, err
		}
		if value > max {
			max = value
		}
	}
	return max, nil
}

func normalizeKeys(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
