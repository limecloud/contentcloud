package environment

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"
)

const PreparationPlanSchemaVersion = "1.0"

type PreparationAction struct {
	Plugin      PluginRef        `json:"plugin"`
	Reason      string           `json:"reason"`
	Permissions []string         `json:"permissions"`
	DataFlow    RegistryDataFlow `json:"data_flow"`
	Cost        RegistryCost     `json:"cost"`
}

type PreparationPlan struct {
	SchemaVersion        string              `json:"schema_version"`
	PreparationID        string              `json:"preparation_id"`
	ExecutionPlanID      string              `json:"execution_plan_id"`
	ProjectID            string              `json:"project_id"`
	EnvironmentDigest    string              `json:"environment_digest"`
	State                string              `json:"state"`
	Actions              []PreparationAction `json:"actions"`
	RequiresConfirmation bool                `json:"requires_confirmation"`
	RequiresNewChat      bool                `json:"requires_new_chat"`
}

func BuildPreparationPlan(projectID string, execution LocalExecutionPlan, verifiedRegistry VerifiedRegistry) (PreparationPlan, error) {
	if projectID == "" || execution.PlanID == "" || execution.EnvironmentDigest == "" || (execution.State != "ready" && execution.State != "environment_prepare") {
		return PreparationPlan{}, fault.Invalid("ENVIRONMENT_PREPARATION_INPUT_INVALID", "环境准备需要有效的本地执行计划和项目")
	}
	registry := verifiedRegistry.raw()
	plan := PreparationPlan{
		SchemaVersion: PreparationPlanSchemaVersion, ExecutionPlanID: execution.PlanID, ProjectID: projectID,
		EnvironmentDigest: execution.EnvironmentDigest, State: "noop", Actions: []PreparationAction{},
	}
	for _, preparation := range execution.Preparation {
		entry, err := registry.Exact(preparation.Plugin.ID, preparation.Plugin.Version, preparation.Plugin.Digest)
		if err != nil {
			return PreparationPlan{}, err
		}
		if _, err := AssessRegistryEntry(entry, PurposeNewInstall); err != nil {
			return PreparationPlan{}, err
		}
		plan.Actions = append(plan.Actions, PreparationAction{
			Plugin: preparation.Plugin, Reason: preparation.Reason, Permissions: sortedCopy(entry.Permissions), DataFlow: entry.DataFlow, Cost: entry.Cost,
		})
	}
	sort.Slice(plan.Actions, func(i, j int) bool { return plan.Actions[i].Plugin.ID < plan.Actions[j].Plugin.ID })
	if len(plan.Actions) > 0 {
		plan.State = "ready"
		plan.RequiresConfirmation = true
		plan.RequiresNewChat = true
		for _, action := range plan.Actions {
			if action.Reason != "not_installed" {
				plan.State = "repair_required"
			}
		}
	}
	body, err := json.Marshal(plan)
	if err != nil {
		return PreparationPlan{}, err
	}
	sum := sha256.Sum256(body)
	plan.PreparationID = "epp_" + hex.EncodeToString(sum[:])
	return plan, nil
}

// PreparedLock derives the exact lock that may be committed after every plan action is installed.
func PreparedLock(manifest Manifest, current EnvironmentLock, plan PreparationPlan, verifiedRegistry VerifiedRegistry, now time.Time) (EnvironmentLock, error) {
	if plan.SchemaVersion != PreparationPlanSchemaVersion || plan.State != "ready" || !plan.RequiresConfirmation || !plan.RequiresNewChat || len(plan.Actions) == 0 {
		return EnvironmentLock{}, fault.Invalid("ENVIRONMENT_PREPARATION_PLAN_INVALID", "环境准备计划不是可执行的已确认计划")
	}
	if plan.ProjectID != manifest.ProjectID || plan.EnvironmentDigest != manifest.Digest {
		return EnvironmentLock{}, fault.Conflict("ENVIRONMENT_PREPARATION_ENVIRONMENT_MISMATCH", "环境准备计划与当前环境清单不一致")
	}
	if err := ValidateLock(manifest, current); err != nil {
		return EnvironmentLock{}, err
	}
	registry := verifiedRegistry.raw()
	allowed := make(map[string]PluginRef, len(manifest.Distribution.Plugins))
	for _, plugin := range manifest.Distribution.Plugins {
		allowed[plugin.ID] = plugin
	}
	locked, err := lockedPlugins(current.Plugins)
	if err != nil {
		return EnvironmentLock{}, err
	}
	next := current
	next.Plugins = append([]LockedPlugin(nil), current.Plugins...)
	seen := map[string]struct{}{}
	for _, action := range plan.Actions {
		if action.Reason != "not_installed" || action.Plugin.Scope != "task" {
			return EnvironmentLock{}, fault.Policy("ENVIRONMENT_PREPARATION_REPAIR_REQUIRED", "只允许安装尚未安装的任务级能力包", "版本或摘要变化必须通过独立修复流程处理")
		}
		if _, duplicated := seen[action.Plugin.ID]; duplicated {
			return EnvironmentLock{}, fault.Conflict("ENVIRONMENT_PREPARATION_ACTION_DUPLICATED", "环境准备计划包含重复能力包")
		}
		seen[action.Plugin.ID] = struct{}{}
		plugin, exists := allowed[action.Plugin.ID]
		if !exists || !reflect.DeepEqual(plugin, action.Plugin) {
			return EnvironmentLock{}, fault.Policy("ENVIRONMENT_PREPARATION_PLUGIN_DENIED", "环境准备能力包不在当前环境清单的准确允许列表中", "重新生成环境准备计划")
		}
		entry, exactErr := registry.Exact(plugin.ID, plugin.Version, plugin.Digest)
		if exactErr != nil {
			return EnvironmentLock{}, exactErr
		}
		if _, assessErr := AssessRegistryEntry(entry, PurposeNewInstall); assessErr != nil {
			return EnvironmentLock{}, assessErr
		}
		if installed, exists := locked[plugin.ID]; exists {
			if installed.Installed {
				return EnvironmentLock{}, fault.Conflict("ENVIRONMENT_PREPARATION_ALREADY_INSTALLED", "环境准备能力包已记录为安装，当前计划已经失效")
			}
			replaced := false
			for index := range next.Plugins {
				if next.Plugins[index].ID == plugin.ID {
					next.Plugins[index] = LockedPlugin{ID: plugin.ID, Kind: plugin.Kind, Version: plugin.Version, Digest: plugin.Digest, Installed: true}
					replaced = true
					break
				}
			}
			if !replaced {
				return EnvironmentLock{}, fault.Conflict("ENVIRONMENT_PREPARATION_LOCK_INVALID", "environment.lock 中的能力包索引与实际内容不一致")
			}
		} else {
			next.Plugins = append(next.Plugins, LockedPlugin{ID: plugin.ID, Kind: plugin.Kind, Version: plugin.Version, Digest: plugin.Digest, Installed: true})
		}
	}
	sort.Slice(next.Plugins, func(i, j int) bool { return next.Plugins[i].ID < next.Plugins[j].ID })
	if now.IsZero() {
		now = time.Now().UTC()
	}
	next.VerifiedAt = now.UTC()
	if err := ValidateLock(manifest, next); err != nil {
		return EnvironmentLock{}, err
	}
	return next, nil
}
