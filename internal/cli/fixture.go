package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
)

func GenerateFixtureKnowledge(contract domain.TaskContract, limit int) domain.KnowledgeExtractionPackage {
	if limit <= 0 {
		limit = 20
	}
	candidates := []domain.KnowledgeCandidate{}
	for _, source := range contract.Sources {
		for _, evidence := range source.Evidence {
			locator, _ := json.Marshal(evidence.Locator)
			kind := "fact"
			if source.SourceType == "visual_asset" || strings.Contains(source.SourceType, "visual") {
				kind = "visual_rule"
			}
			candidates = append(candidates, domain.KnowledgeCandidate{Kind: kind, Title: source.Name, Statement: evidence.Quote, Subject: contract.Project.ProductName, Predicate: source.SourceType, Value: domain.TypedValue{Type: "text", Text: evidence.Quote}, Scope: domain.KnowledgeScope{Regions: []string{}, Channels: []string{}, Audiences: []string{}, ProductVariants: []string{}}, RiskLevel: "medium", AllowedChannels: []string{}, Evidence: []domain.EvidenceRef{{SourceRevisionID: source.RevisionID, LocatorKind: evidence.LocatorKind, Locator: string(locator), Quote: evidence.Quote}}, ForbiddenExtensions: []string{}, DependsOnFactIDs: []string{}})
			if len(candidates) >= limit {
				return domain.KnowledgeExtractionPackage{SchemaVersion: "1.0", Candidates: candidates, Warnings: []string{}}
			}
		}
	}
	return domain.KnowledgeExtractionPackage{SchemaVersion: "1.0", Candidates: candidates, Warnings: []string{}}
}

func GenerateFixtureScript(contract domain.TaskContract) domain.ScriptPackage {
	brief := contract.Brief
	duration := brief.TargetDurationSeconds
	if duration <= 0 {
		duration = 30
	}
	bounds := []int{0, duration * 13 / 100, duration * 30 / 100, duration * 50 / 100, duration * 73 / 100, duration * 87 / 100, duration}
	knowledgeIDs := []string{}
	for _, item := range contract.Knowledge {
		if item.Status == "approved" {
			knowledgeIDs = append(knowledgeIDs, item.ID)
		}
	}
	if len(knowledgeIDs) == 0 {
		return domain.ScriptPackage{SchemaVersion: "1.1", Deliverability: "blocked", Title: contract.Project.ProductName + "营销视频", Channel: brief.Channel, TargetDurationSeconds: duration, AspectRatio: "9:16", CreativeStrategy: strategy(brief), ProductionBible: bible(contract), Narrative: []string{}, Shots: []domain.Shot{}, Citations: []domain.Citation{}, BlockedReasons: []domain.BlockReason{{Code: "approved_knowledge_missing", Message: "Task Contract 中没有可用的批准知识", NextAction: "由项目审核员批准事实、主张和视觉规则"}}, MissingInputs: []string{"approved knowledge"}}
	}
	roles := []string{"hook", "context", "product_solution", "proof", "payoff", "cta"}
	purposes := []string{"在前三秒建立安静需求时刻", "让观众认出自己的信息过载状态", "把产品作为日常切换动作引入", "用真实工艺与使用画面提供可信证明", "展示从忙乱到安定的可观察变化", "给出单一、低门槛的下一步"}
	visuals := []string{"夜色书房中，手机通知亮起后被手掌翻面扣下", "人物松开肩膀，清理桌面留出一块空白", "真实线香与香具由手放入画面，包装采用真实素材合成", "微距展示线香材质与点燃过程，烟线在侧光中可见", "人物坐回书桌，呼吸和翻书动作变慢", "真实产品包装与香事指南页面并列，保留字幕安全区"}
	shots := make([]domain.Shot, 0, 6)
	citations := []domain.Citation{}
	for i := range roles {
		refs := []string{knowledgeIDs[i%len(knowledgeIDs)]}
		shotID := fmt.Sprintf("SC01-SH%02d", i+1)
		planID := ""
		if roles[i] == "proof" && len(brief.VisualizationPlanIDs) > 0 {
			planID = brief.VisualizationPlanIDs[0]
		}
		shot := domain.Shot{ShotID: shotID, StartMS: bounds[i] * 1000, EndMS: bounds[i+1] * 1000, Role: roles[i], NarrativePurpose: purposes[i], Subject: map[bool]string{true: contract.Project.ProductName, false: "城市居家用户"}[i >= 2], VisualIntent: visuals[i], SubjectAction: actionFor(i), Composition: compositionFor(i), CameraMotion: cameraFor(i), FirstFrame: domain.FrameSpec{VisualState: "承接上一镜的稳定主体、道具和光线状态", PromptZH: visuals[i] + "的动作开始前静态状态"}, MotionSpec: actionFor(i) + "；运动方向连续，产品形态保持不变", EndFrame: domain.FrameSpec{VisualState: "动作完成并为下一镜保留明确空间方向", PromptZH: visuals[i] + "的动作完成后静态状态"}, Voiceover: voiceFor(i, brief), OnScreenText: textFor(i, brief), SoundIntent: soundFor(i), KnowledgeRefs: refs, ReferenceAssetIDs: []string{}, NegativeConstraints: []string{"包装文字不得由模型重绘", "不得出现医疗或保健暗示", "产品形态不得漂移"}, Continuity: domain.Continuity{IncomingState: "暖色书房侧光，主体位于画面右侧", OutgoingState: "暖色书房侧光保持，动作在画面中央结束", MovementAxis: "从右向左建立，再沿同轴承接", LightingLock: "窗侧暖主光，背景低照度", ProductLock: "产品包装、Logo 和可读文字使用真实素材合成"}, ProductTruthStrategy: "real_asset_composite", VisualizationPlanID: planID, AcceptanceCriteria: []string{"主体动作一眼可理解", "画面与口播不矛盾", "产品包装和文字保持真实"}, PlanB: "若生成模型无法稳定保持产品细节，改用环境空镜并在后期合成真实产品素材"}
		shots = append(shots, shot)
		citations = append(citations, domain.Citation{KnowledgeID: refs[0], ShotID: shotID, Usage: usageFor(i)})
	}
	return domain.ScriptPackage{SchemaVersion: "1.1", Deliverability: "review_ready", Title: contract.Project.ProductName + "｜把一天慢下来", Channel: brief.Channel, TargetDurationSeconds: duration, AspectRatio: "9:16", CreativeStrategy: strategy(brief), ProductionBible: bible(contract), Narrative: roles, Shots: shots, Citations: citations, BlockedReasons: []domain.BlockReason{}, MissingInputs: []string{}}
}

func strategy(b domain.BriefVersion) domain.CreativeStrategy {
	return domain.CreativeStrategy{Objective: b.Objective, Audience: b.Audience, DemandMoment: b.DemandMoment, PrimarySellingPoint: b.PrimarySellingPoint, SecondarySellingPoints: b.SecondarySellingPoints, CTA: b.CTA, Hypothesis: "具体需求时刻比抽象文化介绍更能促使目标用户继续观看", PrimaryTestVariable: b.PrimaryTestVariable, InvariantFields: []string{"audience", "primary_selling_point", "proof", "cta", "duration"}}
}
func bible(c domain.TaskContract) domain.ProductionBible {
	return domain.ProductionBible{Subjects: []domain.SubjectLock{{ID: "SUBJECT-PRODUCT", Name: c.Project.ProductName, IdentityAnchors: []string{"细长线香形制", "真实品牌包装素材", "香具比例固定"}, WardrobeAndProps: []string{"线香", "香插", "品牌提供的真实包装"}}, {ID: "SUBJECT-USER", Name: "居家用户", IdentityAnchors: []string{"自然生活状态", "手部动作克制", "不展示夸张情绪"}, WardrobeAndProps: []string{"素色居家服", "书与手机"}}}, SceneLock: "夜间书房或茶席，桌面真实使用痕迹，空间布局跨镜一致", VisualStyleLock: "克制的暖侧光与真实材质，不使用悬浮产品或塑料 CG 高光", AssetIDs: []string{}}
}
func actionFor(i int) string {
	return []string{"手掌把不断亮起的手机翻面扣下", "人物清理桌面并缓慢呼气", "手将线香和香具放到桌面中央", "手点燃线香，火头熄灭后稳定形成烟线", "人物翻开书页并停留在安静坐姿", "手指向真实香事指南，画面停在明确 CTA"}[i]
}
func compositionFor(i int) string {
	return []string{"手机极近景，右上保留字幕安全区", "肩背中景，桌面形成引导线", "俯拍中近景，产品处于视觉中心", "侧逆光微距，焦点锁在线香材质和烟线", "平视中景，人物与产品形成前后景", "产品与指南的双主体构图，下方保留 CTA 安全区"}[i]
}
func cameraFor(i int) string {
	return []string{"缓慢推进后在手机被扣下时停住", "轻微横移跟随手部清理动作", "稳定俯拍，不做旋转英雄镜头", "微距沿线香轴向缓慢移动", "固定机位，仅保留自然呼吸感", "缓慢拉远后稳定停住"}[i]
}
func soundFor(i int) string {
	return []string{"通知声戛然而止，保留室内底噪", "书页与木桌轻微摩擦声", "香具落桌的清脆触碰声", "火柴摩擦、短促燃烧和室内环境声", "翻书声和远处城市低频底噪", "声音收束，仅保留一句 CTA 口播"}[i]
}
func voiceFor(i int, b domain.BriefVersion) string {
	values := []string{"一天结束了，消息却还没有停。", "真正难得的，不是时间，是切换。", b.PrimarySellingPoint, "看得见的材料和动作，才让仪式落到日常。", "让一个具体动作，替你把节奏慢下来。", b.CTA}
	return strings.TrimSpace(values[i])
}
func textFor(i int, b domain.BriefVersion) string {
	if i == 0 {
		return "今天，停在哪一刻？"
	}
	if i == 5 {
		return b.CTA
	}
	return ""
}
func usageFor(i int) string {
	if i == 5 {
		return "on_screen_text"
	}
	if i >= 2 {
		return "visual_fact"
	}
	return "style_rule"
}
