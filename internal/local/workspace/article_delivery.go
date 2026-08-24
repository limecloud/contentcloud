package localworkspace

import (
	"encoding/json"
	"strconv"

	exportfmt "github.com/limecloud/contentcloud/internal/local/export"
	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/stablehash"
)

func renderArticleContentDelivery(raw json.RawMessage) (RenderedContentDelivery, error) {
	var identity struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(raw, &identity); err != nil {
		return RenderedContentDelivery{}, fault.Invalid("ARTICLE_ITEM_JSON_INVALID", err.Error())
	}
	item, err := ValidateArticleItemForSubmission(raw, identity.ProjectID)
	if err != nil {
		return RenderedContentDelivery{}, err
	}
	if item.Deliverability != "review_ready" {
		return RenderedContentDelivery{}, fault.Policy("APPROVED_ARTICLE_ITEM_BLOCKED", "只有处于 review_ready 状态的文章对象才能生成正式交付包", "修订并重新批准文章对象")
	}
	jsonBody, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return RenderedContentDelivery{}, err
	}
	jsonBody = append(jsonBody, '\n')
	markdown := []byte(renderArticleMarkdown(item))
	rows := [][]string{{"区块ID", "类型", "层级", "正文", "素材引用", "权利引用"}}
	for _, block := range item.Blocks {
		rows = append(rows, []string{block.ID, block.Type, strconv.Itoa(block.Level), block.Text, block.AssetRef, block.RightsRef})
	}
	xlsx, err := exportfmt.XLSX("文章区块", rows)
	if err != nil {
		return RenderedContentDelivery{}, err
	}
	hash, err := stablehash.Sum(item)
	if err != nil {
		return RenderedContentDelivery{}, err
	}
	files := []RenderedContentFile{
		{Format: "json", Name: "article.json", MediaType: "application/json", Body: jsonBody, SHA256: digest(jsonBody)},
		{Format: "markdown", Name: "article.md", MediaType: "text/markdown", Body: markdown, SHA256: digest(markdown)},
		{Format: "xlsx", Name: "article.xlsx", MediaType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Body: xlsx, SHA256: digest(xlsx)},
	}
	return RenderedContentDelivery{ItemID: item.ID, SchemaID: ArticleSchema, ContentHash: "sha256:" + hash, Files: files}, nil
}
