package runtime

import (
	"sort"
	"time"

	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	"github.com/limecloud/contentcloud/internal/platform/stablehash"
)

type Compiler struct {
	Limits RuntimeLimits
}

func NewCompiler(limits RuntimeLimits) Compiler {
	if limits.MaxNodes == 0 {
		limits = DefaultRuntimeLimits()
	}
	return Compiler{Limits: limits}
}

// CompileSOP converts the existing published SOP shape into an immutable DAG.
// It is deterministic: the same SOP digest always produces the same plan body.
func (c Compiler) CompileSOP(sop catalogdomain.SOPVersion, tenantID, compiledBy string, now time.Time) (JobPlanRevision, error) {
	if sop.Status != "published" {
		return JobPlanRevision{}, fault.Policy("JOB_PLAN_SOP_NOT_PUBLISHED", "只有已发布流程规范才能编译执行计划", "先发布流程规范版本")
	}
	if err := sop.Validate(); err != nil {
		return JobPlanRevision{}, err
	}
	limits := c.Limits
	if limits.MaxNodes == 0 {
		limits = DefaultRuntimeLimits()
	}
	stages := append([]catalogdomain.StageDefinition(nil), sop.Stages...)
	sort.SliceStable(stages, func(i, j int) bool { return stages[i].Order < stages[j].Order })
	stageNode := map[string]string{}
	stageExit := map[string]string{}
	for _, stage := range stages {
		stageNode[stage.ID] = "stage:" + stage.ID
		stageExit[stage.ID] = stageNode[stage.ID]
		if len(stage.GateIDs) > 0 {
			// A stage with several gates exits through the last gate in the
			// published order. The gate nodes themselves are still visible.
			stageExit[stage.ID] = "gate:" + stage.GateIDs[len(stage.GateIDs)-1]
		}
	}
	nodes := make([]JobPlanNode, 0, len(stages)+len(sop.Gates))
	edges := []JobPlanEdge{}
	steps := make([]JobPlanCustomerStep, 0, len(stages))
	for _, stage := range stages {
		depends := []string{}
		for _, ref := range stage.InputRefs {
			if key, ok := stageExit[ref]; ok {
				depends = append(depends, key)
			}
		}
		sort.Strings(depends)
		node := JobPlanNode{Key: stageNode[stage.ID], Kind: "stage", StageID: stage.ID, Name: stage.Name, DependsOn: depends, InputRefs: append([]string{}, stage.InputRefs...), OutputSchema: stage.OutputSchema, RequiredCapabilities: append([]string{}, stage.RequiredCapabilities...), ExecutionModes: append([]string{}, stage.ExecutionModes...), CustomerStepID: stage.ID, SideEffectClass: "business_candidate", RetryMaxAttempts: stage.RetryMaxAttempts}
		if node.RetryMaxAttempts < 1 {
			node.RetryMaxAttempts = limits.MaxAttemptsPerNode
		}
		nodes = append(nodes, node)
		for _, dep := range depends {
			edges = append(edges, JobPlanEdge{From: dep, To: node.Key})
		}
		steps = append(steps, JobPlanCustomerStep{ID: stage.ID, Title: stage.Name, NodeKeys: []string{node.Key}})
		for _, gateID := range stage.GateIDs {
			gate, found := findGate(sop.Gates, gateID)
			if !found {
				return JobPlanRevision{}, fault.Invalid("JOB_PLAN_GATE_NOT_FOUND", "Stage 引用了不存在的 Gate: "+gateID)
			}
			gateKey := "gate:" + gate.ID
			gateNode := JobPlanNode{Key: gateKey, Kind: "gate", GateID: gate.ID, Name: gate.Name, DependsOn: []string{node.Key}, InputRefs: append([]string{}, gate.InputRefs...), OutputSchema: "contentcloud.gate-decision/1.0", CustomerStepID: stage.ID, SideEffectClass: "human_decision", RetryMaxAttempts: 1}
			nodes = append(nodes, gateNode)
			edges = append(edges, JobPlanEdge{From: node.Key, To: gateKey})
			steps[len(steps)-1].NodeKeys = append(steps[len(steps)-1].NodeKeys, gateKey)
		}
	}
	plan := JobPlanRevision{ID: idgen.New(), TenantID: tenantID, GraphVersion: 1, SOPID: sop.SOPID, SOPVersion: sop.Version, SOPDigest: sop.Digest, SchemaVersion: JobPlanSchema, Nodes: nodes, Edges: edges, CustomerSteps: steps, Limits: limits, CompiledAt: now.UTC(), CompiledBy: compiledBy}
	bodyHash, err := stablehash.Sum(struct {
		Schema, SOPID string
		Version       int
		Digest        string
		Nodes         []JobPlanNode
		Edges         []JobPlanEdge
		Steps         []JobPlanCustomerStep
		Limits        RuntimeLimits
	}{JobPlanSchema, sop.SOPID, sop.Version, sop.Digest, nodes, edges, steps, limits})
	if err != nil {
		return JobPlanRevision{}, err
	}
	plan.Digest = "sha256:" + bodyHash
	if err := plan.Validate(); err != nil {
		return JobPlanRevision{}, err
	}
	return plan, nil
}

func findGate(gates []catalogdomain.GateDefinition, id string) (catalogdomain.GateDefinition, bool) {
	for _, gate := range gates {
		if gate.ID == id {
			return gate, true
		}
	}
	return catalogdomain.GateDefinition{}, false
}
