package workbench

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/stablehash"
)

// Entry is an immutable, server-approved workbench plugin version. The
// manifest describes the customer surface; the template reference remains the
// owner of SOP, Gate, Runtime and artifact semantics.
type Entry struct {
	Manifest        Manifest `json:"manifest"`
	Digest          string   `json:"digest"`
	Status          string   `json:"status"`
	LifecycleReason string   `json:"lifecycle_reason,omitempty"`
	TemplateAliases []string `json:"template_aliases,omitempty"`
	TenantIDs       []string `json:"tenant_ids,omitempty"`
}

// Registry is an in-memory value object at the domain boundary. The
// application may merge it with the platform-scoped persistent Registry
// without changing the Customer BFF contract.
type Registry struct {
	entries []Entry
}

// Entries returns immutable copies for control-plane projections. Callers can
// inspect or persist these values without gaining access to registry state.
func (r *Registry) Entries() []Entry {
	if r == nil {
		return nil
	}
	entries := make([]Entry, 0, len(r.entries))
	for _, entry := range r.entries {
		entries = append(entries, cloneEntry(entry))
	}
	return entries
}

func NewRegistry(entries []Entry) (*Registry, error) {
	validated := make([]Entry, 0, len(entries))
	seen := map[string]struct{}{}
	for _, entry := range entries {
		entry.LifecycleReason = strings.TrimSpace(entry.LifecycleReason)
		if entry.Status != "revoked" {
			entry.LifecycleReason = ""
		}
		if err := validateEntry(entry); err != nil {
			return nil, err
		}
		key := entry.Manifest.ID + "@" + entry.Manifest.Version
		if _, exists := seen[key]; exists {
			return nil, fault.Conflict("WORKBENCH_PLUGIN_DUPLICATED", "业务工作台插件的 ID 和版本重复")
		}
		seen[key] = struct{}{}
		digest, err := manifestDigest(entry.Manifest)
		if err != nil {
			return nil, fault.Invalid("WORKBENCH_PLUGIN_DIGEST_INVALID", "业务工作台插件摘要计算失败")
		}
		if entry.Digest == "" {
			entry.Digest = digest
		} else if entry.Digest != digest {
			return nil, fault.Conflict("WORKBENCH_PLUGIN_DIGEST_MISMATCH", "业务工作台插件摘要与声明内容不一致")
		}
		entry.TemplateAliases = uniqueStrings(entry.TemplateAliases)
		entry.TenantIDs = uniqueStrings(entry.TenantIDs)
		validated = append(validated, entry)
	}
	sort.SliceStable(validated, func(i, j int) bool {
		return validated[i].Manifest.ID+"@"+validated[i].Manifest.Version < validated[j].Manifest.ID+"@"+validated[j].Manifest.Version
	})
	return &Registry{entries: validated}, nil
}

// DefaultRegistry contains the platform-owned first-party experiences. An
// external package must pass through NewRegistry before it can be enabled.
func DefaultRegistry() *Registry {
	registry, err := NewRegistry([]Entry{
		{
			Status: "published",
			Manifest: Manifest{
				Schema: ManifestSchema, ID: "contentcloud-workbench-marketing-video", Version: "1.0.0", Name: "视频生产工作台",
				ContentTypes: []string{"marketing_video"}, Experience: ExperienceRef{TemplateID: "ip_persona_marketing_video"},
				UI: UIManifest{Renderer: "approved", Layout: "stage-canvas-context", Density: "comfortable", Theme: "signal-blue",
					Navigation: []NavigationItem{{ID: "overview", Label: "视频首页", Icon: "video"}, {ID: "tasks", Label: "视频任务", Icon: "list-checks"}, {ID: "assets", Label: "视频素材", Icon: "image"}, {ID: "deliveries", Label: "视频交付", Icon: "package-check"}},
					Stages:     []Stage{{ID: "objective", Label: "目标与资料", Outcome: "确定视频目标", PrimaryAction: "save_brief"}, {ID: "script", Label: "剧本", Outcome: "确认营销剧本", PrimaryAction: "submit_script"}, {ID: "storyboard", Label: "分镜", Outcome: "确认镜头和画面素材", PrimaryAction: "approve_storyboard"}, {ID: "candidates", Label: "候选视频", Outcome: "选择一个候选版本", PrimaryAction: "select_candidate"}, {ID: "approval", Label: "确认成片", Outcome: "确认最终成片", PrimaryAction: "approve_final"}, {ID: "delivery", Label: "交付", Outcome: "生成可追溯交付包", PrimaryAction: "create_delivery"}},
				},
			},
			TemplateAliases: []string{"builtin-sop-marketing-video", "builtin_sop_marketing_video", "marketing_video_production"},
		},
		{
			Status: "published",
			Manifest: Manifest{
				Schema: ManifestSchema, ID: "contentcloud-workbench-article", Version: "1.0.0", Name: "文章创作工作台",
				ContentTypes: []string{"article", "wechat_article", "article_content"}, Experience: ExperienceRef{TemplateID: "article_content"},
				UI: UIManifest{Renderer: "approved", Layout: "article-editor", Density: "comfortable", Theme: "editorial-green",
					Navigation: []NavigationItem{{ID: "overview", Label: "文章首页", Icon: "pen-line"}, {ID: "tasks", Label: "文章任务", Icon: "list-checks"}, {ID: "library", Label: "资料与引用", Icon: "book-open"}, {ID: "deliveries", Label: "文章交付", Icon: "file-check"}},
					Stages:     []Stage{{ID: "topic", Label: "选题", Outcome: "确定文章主题", PrimaryAction: "save_brief"}, {ID: "sources", Label: "资料与引用", Outcome: "固定可追溯引用", PrimaryAction: "attach_sources"}, {ID: "draft", Label: "文章草稿", Outcome: "完成文章草稿", PrimaryAction: "save_draft"}, {ID: "proofing", Label: "校对与确认", Outcome: "确认文章版本", PrimaryAction: "approve_article"}, {ID: "delivery", Label: "交付", Outcome: "生成发布交付包", PrimaryAction: "create_delivery"}},
				},
			},
			TemplateAliases: []string{"article_collaboration"},
		},
		{
			Status: "published",
			Manifest: Manifest{
				Schema: ManifestSchema, ID: "contentcloud-workbench-commerce", Version: "1.0.0", Name: "电商内容工作台",
				ContentTypes: []string{"commerce", "ecommerce", "douyin_commerce", "product_content"}, Experience: ExperienceRef{TemplateID: "commerce_content"},
				UI: UIManifest{Renderer: "approved", Layout: "product-variants", Density: "dense", Theme: "production-coral",
					Navigation: []NavigationItem{{ID: "overview", Label: "商品首页", Icon: "shopping-bag"}, {ID: "tasks", Label: "商品任务", Icon: "list-checks"}, {ID: "catalog", Label: "商品资料", Icon: "tags"}, {ID: "deliveries", Label: "渠道交付", Icon: "package-check"}},
					Stages:     []Stage{{ID: "product", Label: "商品信息", Outcome: "固定商品事实", PrimaryAction: "save_product"}, {ID: "offer", Label: "卖点策略", Outcome: "确认购买理由", PrimaryAction: "approve_offer"}, {ID: "variants", Label: "内容变体", Outcome: "生成渠道版本", PrimaryAction: "create_variants"}, {ID: "preview", Label: "渠道预览", Outcome: "通过渠道规格检查", PrimaryAction: "validate_channel"}, {ID: "delivery", Label: "交付", Outcome: "生成渠道交付包", PrimaryAction: "create_delivery"}},
				},
			},
		},
		{
			Status: "published",
			Manifest: Manifest{
				Schema: ManifestSchema, ID: "contentcloud-workbench-serialized-novel", Version: "1.0.0", Name: "连载小说工作台",
				ContentTypes: []string{"serialized_novel"}, Experience: ExperienceRef{TemplateID: "serialized-novel"},
				UI: UIManifest{Renderer: "approved", Layout: "novel-editor", Density: "comfortable", Theme: "review-rose",
					Navigation: []NavigationItem{{ID: "overview", Label: "小说首页", Icon: "book-open"}, {ID: "tasks", Label: "章节任务", Icon: "list-checks"}, {ID: "library", Label: "Canon 与资料", Icon: "archive"}, {ID: "deliveries", Label: "章节交付", Icon: "file-check"}},
					Stages:     []Stage{{ID: "canon", Label: "世界观 Canon", Outcome: "固定角色、规则与时间线", PrimaryAction: "save_brief"}, {ID: "outline", Label: "卷章规划", Outcome: "确定章节目标与引用", PrimaryAction: "save_draft"}, {ID: "chapter", Label: "章节候选", Outcome: "完成可审阅章节", PrimaryAction: "save_draft"}, {ID: "continuity", Label: "连续性校验", Outcome: "通过角色、时间线和伏笔检查", PrimaryAction: "validate_channel"}, {ID: "edit", Label: "编辑与确认", Outcome: "确认章节版本", PrimaryAction: "approve_chapter"}, {ID: "delivery", Label: "章节交付", Outcome: "生成可追溯章节交付包", PrimaryAction: "create_delivery"}},
				},
			},
			TemplateAliases: []string{"serialized_novel", "novel_production"},
		},
	})
	if err != nil {
		panic(err)
	}
	return registry
}

func (r *Registry) Resolve(contentType, templateRef, tenantID string) (Entry, error) {
	if r == nil {
		return Entry{}, fault.NotFound("业务工作台插件")
	}
	contentType = normalizeContentType(contentType)
	templateRef = strings.TrimSpace(templateRef)
	tenantID = strings.TrimSpace(tenantID)
	candidates := make([]Entry, 0, len(r.entries))
	for _, entry := range r.entries {
		if entry.Status == "published" && containsContentType(entry.Manifest.ContentTypes, contentType) && entry.matchesTemplate(templateRef) && entry.enabledForTenant(tenantID) {
			candidates = append(candidates, entry)
		}
	}
	if len(candidates) == 0 {
		return Entry{}, fault.NotFound("已发布的业务工作台插件")
	}
	// A tenant-scoped release must override the platform default. Within the
	// same scope, prefer an exact template binding and then the highest
	// semantic version. The final ID tie-breaker keeps resolution deterministic.
	sort.SliceStable(candidates, func(i, j int) bool {
		if scoped := len(candidates[i].TenantIDs) > 0; scoped != (len(candidates[j].TenantIDs) > 0) {
			return scoped
		}
		iExact := strings.TrimSpace(templateRef) != "" && candidates[i].Manifest.Experience.TemplateID == strings.TrimSpace(templateRef)
		jExact := strings.TrimSpace(templateRef) != "" && candidates[j].Manifest.Experience.TemplateID == strings.TrimSpace(templateRef)
		if iExact != jExact {
			return iExact
		}
		if compareVersions(candidates[i].Manifest.Version, candidates[j].Manifest.Version) != 0 {
			return compareVersions(candidates[i].Manifest.Version, candidates[j].Manifest.Version) > 0
		}
		return candidates[i].Manifest.ID < candidates[j].Manifest.ID
	})
	return cloneEntry(candidates[0]), nil
}

// ResolveVersion reads an immutable workbench version already pinned by a
// WorkTask. Tenant assignment and normal retirement are intentionally ignored
// here: retiring a version can stop new tasks, but must not rewrite an existing
// task's customer surface. Emergency revocation still fails closed. The digest
// prevents an ID/version pair from being reused with different declarations.
func (r *Registry) ResolveVersion(id, version, digest string) (Entry, error) {
	if r == nil {
		return Entry{}, fault.NotFound("业务工作台插件版本")
	}
	id = strings.TrimSpace(id)
	version = strings.TrimSpace(version)
	digest = strings.TrimSpace(digest)
	if !validIdentifier(id) || !versionPattern.MatchString(version) || !stablehash.Matches(digest) {
		return Entry{}, fault.Invalid("WORKBENCH_PLUGIN_REF_INVALID", "任务固定的业务工作台引用无效")
	}
	for _, entry := range r.entries {
		if entry.Manifest.ID != id || entry.Manifest.Version != version {
			continue
		}
		if entry.Status == "revoked" {
			return Entry{}, fault.Policy("WORKBENCH_PLUGIN_REVOKED", "任务固定的业务工作台版本已被安全撤销", "联系平台运营人员检查该工作台版本")
		}
		if entry.Status == "draft" {
			return Entry{}, fault.Policy("WORKBENCH_PLUGIN_NOT_PUBLISHED", "任务固定的业务工作台版本尚未发布", "联系平台运营人员检查任务的工作台绑定")
		}
		if entry.Digest != digest {
			return Entry{}, fault.Conflict("WORKBENCH_PLUGIN_DIGEST_MISMATCH", "任务固定的业务工作台摘要与 Registry 不一致")
		}
		return cloneEntry(entry), nil
	}
	return Entry{}, fault.NotFound("业务工作台插件版本")
}

func compareVersions(left, right string) int {
	leftParts := strings.Split(left, ".")
	rightParts := strings.Split(right, ".")
	for index := 0; index < 3; index++ {
		leftValue, _ := strconv.Atoi(leftParts[index])
		rightValue, _ := strconv.Atoi(rightParts[index])
		if leftValue != rightValue {
			if leftValue > rightValue {
				return 1
			}
			return -1
		}
	}
	return 0
}

func normalizeContentType(value string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "-", "_"))
}

func containsContentType(values []string, wanted string) bool {
	for _, value := range values {
		if normalizeContentType(value) == wanted {
			return true
		}
	}
	return false
}

// ScopesConflict reports whether two published entries compete for the same
// resolution scope. A tenant override may coexist with the platform default;
// two defaults or two tenant scopes with overlap may not.
func ScopesConflict(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return len(left) == 0 && len(right) == 0
	}
	for _, leftTenant := range left {
		for _, rightTenant := range right {
			if strings.TrimSpace(leftTenant) == strings.TrimSpace(rightTenant) {
				return true
			}
		}
	}
	return false
}

// SameTenantScope compares tenant assignments as sets. Registry lifecycle
// guards use it to keep emergency revocations immutable without depending on
// request ordering or duplicate values.
func SameTenantScope(left, right []string) bool {
	left = uniqueStrings(left)
	right = uniqueStrings(right)
	if len(left) != len(right) {
		return false
	}
	wanted := make(map[string]struct{}, len(left))
	for _, tenantID := range left {
		wanted[tenantID] = struct{}{}
	}
	for _, tenantID := range right {
		if _, exists := wanted[tenantID]; !exists {
			return false
		}
	}
	return true
}

// SharesContentType compares the normalized content types used by registry
// resolution. It keeps publication validation consistent across application,
// memory, and PostgreSQL repositories.
func SharesContentType(left, right []string) bool {
	for _, leftType := range left {
		if containsContentType(right, normalizeContentType(leftType)) {
			return true
		}
	}
	return false
}

func (entry Entry) matchesTemplate(templateRef string) bool {
	if templateRef == "" || entry.Manifest.Experience.TemplateID == templateRef {
		return true
	}
	return containsString(entry.TemplateAliases, templateRef)
}

func (entry Entry) enabledForTenant(tenantID string) bool {
	if len(entry.TenantIDs) == 0 {
		return true
	}
	return containsString(entry.TenantIDs, tenantID)
}

func validateEntry(entry Entry) error {
	if err := validateManifest(entry.Manifest); err != nil {
		return err
	}
	if entry.Status != "published" && entry.Status != "draft" && entry.Status != "retired" && entry.Status != "revoked" {
		return fault.Invalid("WORKBENCH_PLUGIN_STATUS_INVALID", "业务工作台插件状态无效")
	}
	if entry.Status == "revoked" && entry.LifecycleReason == "" {
		return fault.Invalid("WORKBENCH_PLUGIN_REVOCATION_REASON_REQUIRED", "安全撤销业务工作台版本时必须填写原因")
	}
	if entry.Digest != "" && !stablehash.Matches(entry.Digest) {
		return fault.Invalid("WORKBENCH_PLUGIN_DIGEST_INVALID", "业务工作台插件摘要必须是 SHA-256")
	}
	return nil
}

func manifestDigest(manifest Manifest) (string, error) {
	digest, err := stablehash.Sum(manifest)
	if err != nil {
		return "", err
	}
	return "sha256:" + digest, nil
}

func cloneEntry(entry Entry) Entry {
	entry.Manifest.ContentTypes = append([]string(nil), entry.Manifest.ContentTypes...)
	entry.Manifest.UI.Navigation = append([]NavigationItem(nil), entry.Manifest.UI.Navigation...)
	entry.Manifest.UI.Stages = append([]Stage(nil), entry.Manifest.UI.Stages...)
	entry.Manifest.UI.Panels = append([]Panel(nil), entry.Manifest.UI.Panels...)
	for index := range entry.Manifest.UI.Panels {
		entry.Manifest.UI.Panels[index].StageIDs = append([]string(nil), entry.Manifest.UI.Panels[index].StageIDs...)
	}
	entry.TemplateAliases = append([]string(nil), entry.TemplateAliases...)
	entry.TenantIDs = append([]string(nil), entry.TenantIDs...)
	return entry
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func (e Entry) String() string {
	return fmt.Sprintf("%s@%s", e.Manifest.ID, e.Manifest.Version)
}
