package localworkspace

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	exportfmt "github.com/limecloud/contentcloud/internal/local/export"
	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/stablehash"
)

const CommerceContentSchema = "contentcloud.commerce-content/1.0"

// CommerceContentItem is the governed business artifact produced by the
// commerce workbench. WorkTask, Runtime, review, delivery and performance keep
// their shared models; only the business payload is commerce-specific.
type CommerceContentItem struct {
	ID             string            `json:"id"`
	Type           string            `json:"type"`
	Status         string            `json:"status"`
	SchemaVersion  string            `json:"schema_version"`
	Deliverability string            `json:"deliverability"`
	ProjectID      string            `json:"project_id"`
	ContentID      string            `json:"content_id"`
	ContentBatchID string            `json:"content_batch_id"`
	Title          string            `json:"title"`
	Channel        string            `json:"channel"`
	TargetAudience string            `json:"target_audience"`
	ProductFacts   map[string]string `json:"product_facts"`
	OfferPoints    []string          `json:"offer_points"`
	Variants       []CommerceVariant `json:"variants"`
	BlockedReasons []string          `json:"blocked_reasons"`
	MissingInputs  []string          `json:"missing_inputs"`
}

type CommerceVariant struct {
	ID      string `json:"id"`
	Format  string `json:"format"`
	Copy    string `json:"copy"`
	CTA     string `json:"cta"`
	Channel string `json:"channel,omitempty"`
}

func ValidateCommerceContentForSubmission(raw json.RawMessage, projectID string) (CommerceContentItem, error) {
	var item CommerceContentItem
	if err := strictUnmarshal(raw, &item); err != nil {
		return item, fault.Invalid("COMMERCE_CONTENT_JSON_INVALID", err.Error())
	}
	if item.SchemaVersion != CommerceContentSchema || item.Type != "commerce_content_item" || item.ID == "" || item.ContentID == "" || item.ContentBatchID == "" {
		return item, fault.Invalid("COMMERCE_CONTENT_IDENTITY_INVALID", "电商内容需要有效的 schema、对象标识和内容批次标识")
	}
	if item.ProjectID != projectID {
		return item, fault.Conflict("COMMERCE_CONTENT_PROJECT_MISMATCH", "电商内容不属于当前项目")
	}
	if item.Status != "candidate" || item.Deliverability != "review_ready" {
		return item, fault.Policy("COMMERCE_CONTENT_NOT_REVIEW_READY", "只有待确认且可评审的电商内容才能提交", "补齐阻断项后重新提交")
	}
	if strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.Channel) == "" || strings.TrimSpace(item.TargetAudience) == "" || len(item.ProductFacts) == 0 || len(item.OfferPoints) == 0 || len(item.Variants) == 0 {
		return item, fault.Invalid("COMMERCE_CONTENT_FIELDS_REQUIRED", "电商内容需要标题、渠道、目标人群、商品事实、购买理由和至少一个内容变体")
	}
	seen := map[string]bool{}
	for _, variant := range item.Variants {
		if variant.ID == "" || variant.Format == "" || strings.TrimSpace(variant.Copy) == "" || seen[variant.ID] {
			return item, fault.Invalid("COMMERCE_CONTENT_VARIANT_INVALID", "电商内容变体需要唯一标识、格式和正文")
		}
		seen[variant.ID] = true
	}
	if item.BlockedReasons == nil || item.MissingInputs == nil {
		return item, fault.Invalid("COMMERCE_CONTENT_COLLECTIONS_REQUIRED", "电商内容必须显式声明阻断项和缺失输入集合")
	}
	if len(item.BlockedReasons) > 0 || len(item.MissingInputs) > 0 {
		return item, fault.Policy("COMMERCE_CONTENT_BLOCKED", "仍有阻断项的电商内容不能进入批准流程", "解决阻断项后重新提交")
	}
	return item, nil
}

func renderCommerceContentDelivery(raw json.RawMessage) (RenderedContentDelivery, error) {
	var identity struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(raw, &identity); err != nil {
		return RenderedContentDelivery{}, fault.Invalid("COMMERCE_CONTENT_JSON_INVALID", err.Error())
	}
	item, err := ValidateCommerceContentForSubmission(raw, identity.ProjectID)
	if err != nil {
		return RenderedContentDelivery{}, err
	}
	jsonBody, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return RenderedContentDelivery{}, err
	}
	jsonBody = append(jsonBody, '\n')
	markdown := []byte(renderCommerceMarkdown(item))
	xlsx, err := renderCommerceXLSX(item)
	if err != nil {
		return RenderedContentDelivery{}, err
	}
	hash, err := stablehash.Sum(item)
	if err != nil {
		return RenderedContentDelivery{}, err
	}
	files := []RenderedContentFile{
		{Format: "json", Name: "commerce-content.json", MediaType: "application/json", Body: jsonBody, SHA256: digest(jsonBody)},
		{Format: "markdown", Name: "commerce-content.md", MediaType: "text/markdown", Body: markdown, SHA256: digest(markdown)},
		{Format: "xlsx", Name: "commerce-content.xlsx", MediaType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Body: xlsx, SHA256: digest(xlsx)},
	}
	return RenderedContentDelivery{ItemID: item.ID, SchemaID: CommerceContentSchema, ContentHash: "sha256:" + hash, Files: files}, nil
}

func renderCommerceMarkdown(item CommerceContentItem) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# %s\n\n- 渠道：%s\n- 目标人群：%s\n\n", item.Title, item.Channel, item.TargetAudience)
	out.WriteString("## 商品事实\n\n")
	keys := make([]string, 0, len(item.ProductFacts))
	for key := range item.ProductFacts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(&out, "- %s：%s\n", key, item.ProductFacts[key])
	}
	out.WriteString("\n## 购买理由\n\n")
	for _, point := range item.OfferPoints {
		fmt.Fprintf(&out, "- %s\n", point)
	}
	out.WriteString("\n## 内容变体\n\n")
	for _, variant := range item.Variants {
		fmt.Fprintf(&out, "### %s · %s\n\n%s\n\nCTA：%s\n\n", variant.ID, variant.Format, variant.Copy, variant.CTA)
	}
	return out.String()
}

func renderCommerceXLSX(item CommerceContentItem) ([]byte, error) {
	rows := [][]string{{"变体ID", "格式", "渠道", "正文", "CTA"}}
	for _, variant := range item.Variants {
		channel := variant.Channel
		if channel == "" {
			channel = item.Channel
		}
		rows = append(rows, []string{variant.ID, variant.Format, channel, variant.Copy, variant.CTA})
	}
	return exportfmt.XLSX("内容变体", rows)
}
