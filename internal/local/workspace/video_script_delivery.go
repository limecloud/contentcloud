package localworkspace

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	exportfmt "github.com/limecloud/contentcloud/internal/local/export"
	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/stablehash"
)

// VideoScriptSchema is the compatibility payload used by the original short
// video workbench. It is now carried by the shared content_batch submission.
const VideoScriptSchema = "contentcloud.video_script/1.0"

// NormalizeVideoScriptForSubmission adds the platform identity fields that
// older clients did not have to provide. The business body remains otherwise
// unchanged so the compatibility DTO does not rewrite user content.
func NormalizeVideoScriptForSubmission(raw json.RawMessage, projectID, taskID string) (json.RawMessage, string, error) {
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return nil, "", fault.Invalid("VIDEO_SCRIPT_JSON_INVALID", "视频脚本正文必须是 JSON 对象")
	}
	if err := validateVideoScriptShape(value, projectID); err != nil {
		return nil, "", err
	}
	objectID := strings.TrimSpace(fmt.Sprint(value["id"]))
	if objectID == "" || objectID == "<nil>" {
		objectID = "video-script:" + taskID
	}
	value["id"] = objectID
	if strings.TrimSpace(fmt.Sprint(value["type"])) == "" || fmt.Sprint(value["type"]) == "<nil>" {
		value["type"] = "video_script"
	}
	value["schema_version"] = VideoScriptSchema
	if strings.TrimSpace(fmt.Sprint(value["status"])) == "" || fmt.Sprint(value["status"]) == "<nil>" {
		value["status"] = "candidate"
	}
	if strings.TrimSpace(fmt.Sprint(value["deliverability"])) == "" || fmt.Sprint(value["deliverability"]) == "<nil>" {
		value["deliverability"] = "review_ready"
	}
	if strings.TrimSpace(fmt.Sprint(value["project_id"])) == "" || fmt.Sprint(value["project_id"]) == "<nil>" {
		value["project_id"] = projectID
	}
	value["content_id"] = defaultMapString(value, "content_id", objectID)
	value["content_batch_id"] = defaultMapString(value, "content_batch_id", "content-batch:"+taskID)
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, "", err
	}
	return canonical, objectID, nil
}

func ValidateVideoScriptForSubmission(raw json.RawMessage, projectID string) error {
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return fault.Invalid("VIDEO_SCRIPT_JSON_INVALID", "视频脚本正文必须是 JSON 对象")
	}
	return validateVideoScriptShape(value, projectID)
}

func validateVideoScriptShape(value map[string]any, projectID string) error {
	schema := strings.TrimSpace(fmt.Sprint(value["schema_version"]))
	if schema != "" && schema != "<nil>" && schema != VideoScriptSchema && schema != "contentcloud.content_batch/3.0" {
		return fault.Invalid("VIDEO_SCRIPT_SCHEMA_INVALID", "视频脚本必须使用 contentcloud.video_script/1.0")
	}
	if value["title"] == nil && value["name"] == nil {
		return fault.Invalid("VIDEO_SCRIPT_TITLE_REQUIRED", "视频脚本需要标题或名称")
	}
	if strings.TrimSpace(fmt.Sprint(value["title"])) == "" && strings.TrimSpace(fmt.Sprint(value["name"])) == "" {
		return fault.Invalid("VIDEO_SCRIPT_TITLE_REQUIRED", "视频脚本需要标题或名称")
	}
	if value["scenes"] == nil && value["items"] == nil {
		return fault.Invalid("VIDEO_SCRIPT_SCENES_REQUIRED", "视频脚本需要场景列表（scenes）或内容项列表（items）")
	}
	if project := strings.TrimSpace(fmt.Sprint(value["project_id"])); project != "" && project != "<nil>" && project != projectID {
		return fault.Conflict("VIDEO_SCRIPT_PROJECT_MISMATCH", "视频脚本不属于当前项目")
	}
	return nil
}

func renderVideoScriptContentDelivery(raw json.RawMessage) (RenderedContentDelivery, error) {
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return RenderedContentDelivery{}, fault.Invalid("VIDEO_SCRIPT_JSON_INVALID", "批准快照中的视频脚本不是有效 JSON")
	}
	if err := validateVideoScriptShape(value, strings.TrimSpace(fmt.Sprint(value["project_id"]))); err != nil {
		return RenderedContentDelivery{}, err
	}
	if strings.EqualFold(strings.TrimSpace(fmt.Sprint(value["deliverability"])), "blocked") {
		return RenderedContentDelivery{}, fault.Policy("APPROVED_VIDEO_SCRIPT_BLOCKED", "只有可交付的视频脚本才能生成正式交付包", "修订并批准视频脚本")
	}
	objectID := strings.TrimSpace(fmt.Sprint(value["id"]))
	if objectID == "" || objectID == "<nil>" {
		return RenderedContentDelivery{}, fault.Invalid("VIDEO_SCRIPT_ID_REQUIRED", "批准快照中的视频脚本缺少对象标识")
	}
	jsonBody, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return RenderedContentDelivery{}, err
	}
	jsonBody = append(jsonBody, '\n')
	markdown := []byte(renderVideoScriptMarkdown(value))
	xlsx, err := renderVideoScriptXLSX(value)
	if err != nil {
		return RenderedContentDelivery{}, err
	}
	hash, err := stablehash.Sum(value)
	if err != nil {
		return RenderedContentDelivery{}, err
	}
	files := []RenderedContentFile{
		{Format: "json", Name: "video-script.json", MediaType: "application/json", Body: jsonBody, SHA256: digest(jsonBody)},
		{Format: "markdown", Name: "video-script.md", MediaType: "text/markdown", Body: markdown, SHA256: digest(markdown)},
		{Format: "xlsx", Name: "video-script.xlsx", MediaType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Body: xlsx, SHA256: digest(xlsx)},
	}
	return RenderedContentDelivery{ItemID: objectID, SchemaID: VideoScriptSchema, ContentHash: "sha256:" + hash, Files: files}, nil
}

func renderVideoScriptMarkdown(value map[string]any) string {
	title := strings.TrimSpace(fmt.Sprint(value["title"]))
	if title == "" || title == "<nil>" {
		title = strings.TrimSpace(fmt.Sprint(value["name"]))
	}
	var out strings.Builder
	fmt.Fprintf(&out, "# %s\n\n", title)
	for index, scene := range videoScriptEntries(value) {
		out.WriteString("## 场景 " + strconv.Itoa(index+1) + "\n\n")
		if object, ok := scene.(map[string]any); ok {
			keys := make([]string, 0, len(object))
			for key := range object {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				fmt.Fprintf(&out, "- %s：%v\n", key, object[key])
			}
		} else {
			fmt.Fprintf(&out, "%v\n", scene)
		}
		out.WriteString("\n")
	}
	return out.String()
}

func renderVideoScriptXLSX(value map[string]any) ([]byte, error) {
	rows := [][]string{{"序号", "场景内容"}}
	for index, scene := range videoScriptEntries(value) {
		body, err := json.Marshal(scene)
		if err != nil {
			return nil, err
		}
		rows = append(rows, []string{strconv.Itoa(index + 1), string(body)})
	}
	return exportfmt.XLSX("视频脚本", rows)
}

func videoScriptEntries(value map[string]any) []any {
	for _, key := range []string{"scenes", "items"} {
		if values, ok := value[key].([]any); ok {
			return values
		}
	}
	return []any{}
}

func defaultMapString(value map[string]any, key, fallback string) string {
	if current := strings.TrimSpace(fmt.Sprint(value[key])); current != "" && current != "<nil>" {
		return current
	}
	return fallback
}
