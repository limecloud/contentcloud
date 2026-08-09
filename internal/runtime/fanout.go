package runtime

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
)

type FanoutItemInput struct {
	ItemKey    string `json:"item_key"`
	ItemDigest string `json:"item_digest"`
}

type CreateFanoutSetInput struct {
	TenantID         string
	JobRunID         string
	MapNodeKey       string
	JoinNodeKey      string
	SourceCollection string
	SourceRevision   int
	SourceWatermark  int64
	Generation       int
	IdempotencyKey   string
	Reason           string
	JoinPolicy       domain.JoinPolicy
	NodeTemplate     domain.JobPlanNode
	Items            []FanoutItemInput
}

type FanoutSetResult struct {
	Plan    domain.JobPlanRevision `json:"plan"`
	Job     domain.JobRun          `json:"job"`
	Set     domain.FanoutSet       `json:"set"`
	Members []domain.FanoutMember  `json:"members"`
	Nodes   []domain.NodeRun       `json:"nodes"`
}

func (s *Service) FanoutSet(ctx context.Context, tenantID, id string) (domain.FanoutSet, error) {
	return s.repo.FanoutSet(ctx, tenantID, id)
}

func (s *Service) FanoutMembers(ctx context.Context, tenantID, setID string) ([]domain.FanoutMember, error) {
	return s.repo.FanoutMembers(ctx, tenantID, setID)
}

// CreateFanoutSet freezes the membership snapshot and appends deterministic
// child nodes in the same command transaction as the plan/job pointer.
func (s *Service) CreateFanoutSet(ctx context.Context, input CreateFanoutSetInput) (FanoutSetResult, error) {
	if s == nil || s.repo == nil {
		return FanoutSetResult{}, domain.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	if strings.TrimSpace(input.TenantID) == "" || strings.TrimSpace(input.JobRunID) == "" || strings.TrimSpace(input.MapNodeKey) == "" || strings.TrimSpace(input.JoinNodeKey) == "" || strings.TrimSpace(input.IdempotencyKey) == "" {
		return FanoutSetResult{}, domain.Invalid("FANOUT_INPUT_INVALID", "FanoutSet 缺少执行实例、映射节点、汇聚节点或幂等键")
	}
	if strings.TrimSpace(input.MapNodeKey) == strings.TrimSpace(input.JoinNodeKey) {
		return FanoutSetResult{}, domain.Invalid("FANOUT_NODE_INVALID", "映射节点和汇聚节点不能是同一个节点")
	}
	if input.Generation < 1 {
		input.Generation = 1
	}
	input.JoinPolicy = domain.NormalizeJoinPolicy(input.JoinPolicy)
	if err := input.JoinPolicy.Validate(); err != nil {
		return FanoutSetResult{}, err
	}
	items, membershipDigest, err := canonicalFanoutMembership(input)
	if err != nil {
		return FanoutSetResult{}, err
	}
	template := input.NodeTemplate
	if template.Kind == "" {
		template.Kind = "fanout_item"
	}
	if template.OutputSchema == "" {
		return FanoutSetResult{}, domain.Invalid("FANOUT_NODE_TEMPLATE_INVALID", "Fanout 子节点必须声明输出 Schema")
	}
	if template.RetryMaxAttempts < 1 {
		template.RetryMaxAttempts = 1
	}
	requestDigest, err := domain.CanonicalHash(struct {
		TenantID         string             `json:"tenant_id"`
		JobRunID         string             `json:"job_run_id"`
		MapNodeKey       string             `json:"map_node_key"`
		JoinNodeKey      string             `json:"join_node_key"`
		SourceCollection string             `json:"source_collection"`
		SourceRevision   int                `json:"source_revision"`
		SourceWatermark  int64              `json:"source_watermark"`
		Generation       int                `json:"generation"`
		JoinPolicy       domain.JoinPolicy  `json:"join_policy"`
		NodeTemplate     domain.JobPlanNode `json:"node_template"`
		Items            []FanoutItemInput  `json:"items"`
	}{input.TenantID, input.JobRunID, input.MapNodeKey, input.JoinNodeKey, input.SourceCollection, input.SourceRevision, input.SourceWatermark, input.Generation, input.JoinPolicy, template, items})
	if err != nil {
		return FanoutSetResult{}, err
	}
	if existing, err := s.repo.FanoutSetByIdempotencyKey(ctx, input.TenantID, input.JobRunID, input.IdempotencyKey); err == nil {
		if existing.RequestDigest != "sha256:"+requestDigest {
			return FanoutSetResult{}, domain.Conflict("FANOUT_IDEMPOTENCY_MISMATCH", "幂等键已用于不同的 Fanout 准入快照")
		}
		members, memberErr := s.repo.FanoutMembers(ctx, input.TenantID, existing.ID)
		if memberErr != nil {
			return FanoutSetResult{}, memberErr
		}
		job, jobErr := s.repo.JobRun(ctx, input.TenantID, input.JobRunID)
		if jobErr != nil {
			return FanoutSetResult{}, jobErr
		}
		plan, planErr := s.repo.Plan(ctx, input.TenantID, job.PlanRevisionID)
		if planErr != nil {
			return FanoutSetResult{}, planErr
		}
		nodes, nodeErr := s.repo.NodeRuns(ctx, input.TenantID, input.JobRunID)
		if nodeErr != nil {
			return FanoutSetResult{}, nodeErr
		}
		return FanoutSetResult{Plan: plan, Job: job, Set: existing, Members: members, Nodes: nodes}, nil
	} else if !domain.IsNotFound(err) {
		return FanoutSetResult{}, err
	}
	job, err := s.repo.JobRun(ctx, input.TenantID, input.JobRunID)
	if err != nil {
		return FanoutSetResult{}, err
	}
	plan, err := s.repo.Plan(ctx, input.TenantID, job.PlanRevisionID)
	if err != nil {
		return FanoutSetResult{}, err
	}
	if !planNodeExists(plan, input.MapNodeKey) || !planNodeExists(plan, input.JoinNodeKey) {
		return FanoutSetResult{}, domain.Invalid("FANOUT_NODE_NOT_FOUND", "FanoutSet 的映射或汇聚节点不在当前执行计划中")
	}
	if !planHasPath(plan, input.MapNodeKey, input.JoinNodeKey) {
		return FanoutSetResult{}, domain.Invalid("FANOUT_JOIN_NOT_DOWNSTREAM", "汇聚节点必须位于映射节点的下游")
	}
	existingNodes, err := s.repo.NodeRuns(ctx, input.TenantID, input.JobRunID)
	if err != nil {
		return FanoutSetResult{}, err
	}
	joinNodeFound := false
	for _, node := range existingNodes {
		if node.NodeKey != input.JoinNodeKey {
			continue
		}
		joinNodeFound = true
		switch node.State {
		case domain.NodePending, domain.NodeBlocked, domain.NodeSkipped, domain.NodeCancelled:
		default:
			return FanoutSetResult{}, domain.Conflict("FANOUT_JOIN_NODE_ACTIVE", "汇聚节点已经领取或执行，不能再绑定新的 FanoutSet")
		}
	}
	if !joinNodeFound {
		return FanoutSetResult{}, domain.NotFound("Fanout 汇聚节点")
	}
	now := s.now().UTC()
	setID := domain.NewID()
	patchNodes := make([]domain.JobPlanNode, 0, len(items))
	members := make([]domain.FanoutMember, 0, len(items))
	nodes := make([]domain.NodeRun, 0, len(items))
	for _, item := range items {
		memberKey, err := domain.DeterministicFanoutMemberKey(input.JobRunID, input.MapNodeKey, setID, item.ItemKey, input.Generation)
		if err != nil {
			return FanoutSetResult{}, err
		}
		node := template
		node.Key = memberKey
		if strings.TrimSpace(node.Name) == "" {
			node.Name = "Fanout " + item.ItemKey
		} else {
			node.Name = node.Name + " [" + item.ItemKey + "]"
		}
		node.DependsOn = append(append([]string{}, node.DependsOn...), input.MapNodeKey)
		patchNodes = append(patchNodes, node)
		nodeRun := domain.NodeRun{ID: domain.NewID(), TenantID: input.TenantID, JobRunID: input.JobRunID, NodeKey: memberKey, State: domain.NodePending, OutputRefs: []string{}, Version: 1, CreatedAt: now, UpdatedAt: now}
		nodes = append(nodes, nodeRun)
		members = append(members, domain.FanoutMember{ID: domain.NewID(), TenantID: input.TenantID, FanoutSetID: setID, MemberKey: memberKey, ItemKey: item.ItemKey, ItemDigest: item.ItemDigest, Generation: input.Generation, NodeRunID: nodeRun.ID, State: domain.FanoutMemberPending, OutputRefs: []string{}, Version: 1, CreatedAt: now, UpdatedAt: now})
	}
	patched, err := ApplyGraphPatch(plan, plan.GraphVersion, GraphPatch{ExpectedGraphVersion: plan.GraphVersion, IdempotencyKey: input.IdempotencyKey, Reason: input.Reason, AddNodes: patchNodes})
	if err != nil {
		return FanoutSetResult{}, err
	}
	set := domain.FanoutSet{ID: setID, TenantID: input.TenantID, JobRunID: input.JobRunID, MapNodeKey: input.MapNodeKey, JoinNodeKey: input.JoinNodeKey, SourceCollection: input.SourceCollection, SourceRevision: input.SourceRevision, SourceWatermark: input.SourceWatermark, Generation: input.Generation, IdempotencyKey: input.IdempotencyKey, MembershipDigest: "sha256:" + membershipDigest, RequestDigest: "sha256:" + requestDigest, MemberCount: len(members), JoinPolicy: input.JoinPolicy, Status: domain.FanoutClosed, Version: 1, ClosedAt: &now, CreatedAt: now, UpdatedAt: now}
	job.Version++
	job.PlanRevisionID, job.PlanDigest, job.UpdatedAt = patched.Plan.ID, patched.Plan.Digest, now
	commands, err := s.commands()
	if err != nil {
		return FanoutSetResult{}, err
	}
	updatedJob, err := commands.CreateFanoutSetCommand(ctx, job, job.Version-1, patched.Plan, set, members, nodes, domain.JobEvent{ID: domain.NewID(), TenantID: input.TenantID, JobRunID: input.JobRunID, Type: "fanout.created", ActorType: "runtime", ActorID: "runtime.fanout", IdempotencyKey: "fanout:" + input.IdempotencyKey, Payload: map[string]any{"fanout_set_id": set.ID, "member_count": len(members), "membership_digest": set.MembershipDigest}, OccurredAt: now})
	if err != nil {
		return FanoutSetResult{}, err
	}
	return FanoutSetResult{Plan: patched.Plan, Job: updatedJob, Set: set, Members: members, Nodes: nodes}, nil
}

func canonicalFanoutMembership(input CreateFanoutSetInput) ([]FanoutItemInput, string, error) {
	items := append([]FanoutItemInput{}, input.Items...)
	sort.Slice(items, func(i, j int) bool { return items[i].ItemKey < items[j].ItemKey })
	seenItems := map[string]bool{}
	for _, item := range items {
		if strings.TrimSpace(item.ItemKey) == "" || strings.TrimSpace(item.ItemDigest) == "" || seenItems[item.ItemKey] {
			return nil, "", domain.Invalid("FANOUT_ITEM_INVALID", "FanoutSet 成员必须有唯一项目键和摘要")
		}
		seenItems[item.ItemKey] = true
	}
	digest, err := domain.CanonicalHash(struct {
		SourceCollection string            `json:"source_collection"`
		SourceRevision   int               `json:"source_revision"`
		SourceWatermark  int64             `json:"source_watermark"`
		Generation       int               `json:"generation"`
		Items            []FanoutItemInput `json:"items"`
	}{input.SourceCollection, input.SourceRevision, input.SourceWatermark, input.Generation, items})
	return items, digest, err
}

func planNodeExists(plan domain.JobPlanRevision, key string) bool {
	for _, node := range plan.Nodes {
		if node.Key == key {
			return true
		}
	}
	return false
}

func planHasPath(plan domain.JobPlanRevision, from, to string) bool {
	adjacency := map[string][]string{}
	for _, edge := range plan.Edges {
		adjacency[edge.From] = append(adjacency[edge.From], edge.To)
	}
	seen := map[string]bool{}
	queue := []string{from}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == to {
			return true
		}
		if seen[current] {
			continue
		}
		seen[current] = true
		queue = append(queue, adjacency[current]...)
	}
	return false
}

func (s *Service) JoinFanoutSet(ctx context.Context, tenantID, setID, actorID string) (domain.FanoutSet, error) {
	set, err := s.repo.FanoutSet(ctx, tenantID, setID)
	if err != nil {
		return domain.FanoutSet{}, err
	}
	if set.Status == domain.FanoutSucceeded || set.Status == domain.FanoutFailed {
		return set, nil
	}
	members, err := s.repo.FanoutMembers(ctx, tenantID, setID)
	if err != nil {
		return domain.FanoutSet{}, err
	}
	membersChanged := false
	for index := range members {
		before := members[index]
		node, nodeErr := s.repo.NodeRun(ctx, tenantID, members[index].NodeRunID)
		if nodeErr != nil {
			return domain.FanoutSet{}, nodeErr
		}
		members[index] = fanoutMemberFromNode(members[index], node)
		if fanoutMemberChanged(before, members[index]) {
			membersChanged = true
		}
	}
	decision, err := domain.EvaluateJoin(set, members)
	if err != nil {
		return domain.FanoutSet{}, err
	}
	if decision.Status == "" && len(decision.CancelMemberKeys) == 0 && !membersChanged {
		return set, nil
	}
	now := s.now().UTC()
	next := set
	next.Version++
	next.UpdatedAt = now
	if decision.Terminal {
		next.Status = decision.Status
	}
	commands, err := s.commands()
	if err != nil {
		return domain.FanoutSet{}, err
	}
	eventType := "fanout.progressed"
	if decision.Terminal || len(decision.CancelMemberKeys) > 0 {
		eventType = "fanout.joined"
	}
	return commands.ApplyFanoutJoinCommand(ctx, next, set.Version, members, decision.CancelMemberKeys, domain.JobEvent{ID: domain.NewID(), TenantID: tenantID, JobRunID: set.JobRunID, Type: eventType, ActorType: "runtime", ActorID: strings.TrimSpace(actorID), IdempotencyKey: "fanout-join:" + set.ID + ":" + strconv.Itoa(next.Version), Payload: map[string]any{"fanout_set_id": set.ID, "status": decision.Status, "successful": decision.Successful, "required_success": decision.RequiredSuccess, "cancelled_member_keys": decision.CancelMemberKeys}, OccurredAt: now})
}

func fanoutMemberChanged(before, after domain.FanoutMember) bool {
	return before.State != after.State || before.OutputDigest != after.OutputDigest || before.ErrorCode != after.ErrorCode || strings.Join(before.OutputRefs, "\x00") != strings.Join(after.OutputRefs, "\x00")
}

func fanoutMemberFromNode(member domain.FanoutMember, node domain.NodeRun) domain.FanoutMember {
	next := member
	switch node.State {
	case domain.NodeRunning, domain.NodeLeased, domain.NodeWaitingExternal, domain.NodeWaitingHuman, domain.NodeWaitingResource, domain.NodeRetryableFailed, domain.NodeLeaseExpired:
		next.State = domain.FanoutMemberRunning
	case domain.NodeSucceeded:
		next.State = domain.FanoutMemberSucceeded
	case domain.NodeFailed, domain.NodeBlocked:
		next.State = domain.FanoutMemberFailed
	case domain.NodeCancelled:
		next.State = domain.FanoutMemberCancelled
	case domain.NodeSkipped:
		next.State = domain.FanoutMemberSkipped
	default:
		next.State = domain.FanoutMemberPending
	}
	next.OutputRefs = append([]string{}, node.OutputRefs...)
	next.OutputDigest = node.OutputDigest
	next.ErrorCode = node.ErrorCode
	if next.State != member.State || next.OutputDigest != member.OutputDigest || next.ErrorCode != member.ErrorCode || strings.Join(next.OutputRefs, "\x00") != strings.Join(member.OutputRefs, "\x00") {
		next.Version++
		next.UpdatedAt = node.UpdatedAt
	}
	return next
}
