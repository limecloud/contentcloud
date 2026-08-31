package desktopapi

import (
	"sort"
	"strings"
	"time"

	localsync "github.com/limecloud/contentcloud/internal/local/sync"
)

type experiencePatternDefinition struct {
	id     string
	kind   string
	title  string
	detail string
	status string
}

var experiencePatternDefinitions = map[string]experiencePatternDefinition{
	"project.observed": {
		id: "workspace-observation", kind: "observation", title: "工作区摘要已记录",
		detail: "本地工作区产生了新的可校验摘要，可作为后续经验的起点。", status: "recorded",
	},
	"workspace.publish.synced": {
		id: "publish-confirmed", kind: "success", title: "发布路径已验证",
		detail: "内容摘要已获得云端 Revision 确认，说明当前同步路径可复用。", status: "validated",
	},
	"workspace.publish.failed": {
		id: "publish-failure", kind: "failure", title: "同步失败需要复盘",
		detail: "发布命令未完成，需要检查错误码、网络状态和基础 Revision。", status: "needs_review",
	},
	"workspace.publish.conflict": {
		id: "publish-conflict", kind: "failure", title: "检测到 Revision 冲突",
		detail: "本地摘要与云端基线不一致，必须先完成冲突处理再继续发布。", status: "needs_review",
	},
	"workspace.publish.auth_required": {
		id: "publish-auth-required", kind: "failure", title: "同步授权已失效",
		detail: "云端要求重新授权设备，当前经验不能直接重放写入动作。", status: "needs_review",
	},
	"workspace.publish.retrying": {
		id: "publish-recovery", kind: "recovery", title: "失败命令正在恢复",
		detail: "系统已安排自动重试，后续结果将作为新的证据追加。", status: "improving",
	},
	"workspace.publish.requeued": {
		id: "publish-requeued", kind: "recovery", title: "失败命令已重新排队",
		detail: "人工或系统触发了可追踪的重试，保留原失败记录。", status: "improving",
	},
	"cloud.resync.required": {
		id: "cloud-resync", kind: "failure", title: "云端需要重新同步",
		detail: "事件游标或云端状态需要重新建立，暂缓继续演化。", status: "needs_review",
	},
}

func buildExperienceProjection(events []localsync.ProjectEvent, now time.Time) ExperienceProjection {
	projection := ExperienceProjection{
		SchemaVersion: ExperienceSchemaVersion,
		Patterns:      []ExperiencePattern{},
	}
	patterns := map[string]*ExperiencePattern{}
	for _, event := range events {
		projection.EventCount++
		if projection.LastUpdated == nil || event.CreatedAt.After(*projection.LastUpdated) {
			at := event.CreatedAt.UTC()
			projection.LastUpdated = &at
		}
		definition, ok := experiencePatternDefinitions[event.Type]
		if !ok {
			continue
		}
		switch definition.kind {
		case "success":
			projection.SuccessCount++
		case "failure":
			projection.FailureCount++
		case "recovery":
			projection.RecoveryCount++
		}
		pattern := patterns[definition.id]
		if pattern == nil {
			pattern = &ExperiencePattern{ID: definition.id, Kind: definition.kind, Title: definition.title, Detail: definition.detail, Status: definition.status, Evidence: []ExperienceEvidence{}}
			patterns[definition.id] = pattern
		}
		pattern.EvidenceCount++
		if len(pattern.Evidence) < 3 {
			pattern.Evidence = append(pattern.Evidence, ExperienceEvidence{EventID: event.ID, Cursor: event.Cursor, EventType: event.Type, CreatedAt: event.CreatedAt.UTC()})
		}
	}
	for _, pattern := range patterns {
		projection.Patterns = append(projection.Patterns, *pattern)
	}
	sort.Slice(projection.Patterns, func(i, j int) bool {
		left, right := projection.Patterns[i], projection.Patterns[j]
		if left.Kind != right.Kind {
			return experienceKindRank(left.Kind) < experienceKindRank(right.Kind)
		}
		return left.ID < right.ID
	})
	projection.PatternCount = len(projection.Patterns)
	if projection.LastUpdated == nil && !now.IsZero() {
		at := now.UTC()
		projection.LastUpdated = &at
	}
	return projection
}

func experienceKindRank(kind string) int {
	switch strings.TrimSpace(kind) {
	case "failure":
		return 0
	case "recovery":
		return 1
	case "success":
		return 2
	default:
		return 3
	}
}
