package httpapi

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/codexplugin"
	"github.com/limecloud/contentcloud/internal/domain"
)

const codexHandoffSchemaVersion = "contentcloud.codex-handoff/1.0"

type codexHandoffTarget struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Digest string `json:"digest,omitempty"`
}

type codexHandoff struct {
	SchemaVersion              string             `json:"schema_version"`
	Kind                       string             `json:"kind"`
	ProjectID                  string             `json:"project_id"`
	Target                     codexHandoffTarget `json:"target"`
	PluginID                   string             `json:"plugin_id"`
	PluginVersion              string             `json:"plugin_version"`
	RequiresNewChat            bool               `json:"requires_new_chat"`
	RequiresWorkspaceSelection bool               `json:"requires_workspace_selection"`
	LaunchURL                  string             `json:"launch_url"`
	Prompt                     string             `json:"prompt"`
	Steps                      []string           `json:"steps"`
	FallbackURL                string             `json:"fallback_url"`
}

func (s *Server) projectCodexHandoff(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	projectID := chi.URLParam(r, "projectID")
	project, err := s.service.Project(r.Context(), actor, projectID)
	if err == nil && project.ConnectedDevices == 0 {
		err = domain.Conflict("CODEX_HANDOFF_WORKSPACE_REQUIRED", "项目尚未连接本地 Workspace，不能生成 Codex 恢复入口")
	}
	if err != nil {
		s.dispatchResult(w, r, "codex.handoff.project", codexHandoff{}, err)
		return
	}
	value := newProjectCodexHandoff(project.ID)
	s.dispatchResult(w, r, "codex.handoff.project", value, nil)
}

func (s *Server) reviewFeedbackCodexHandoff(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	projectID := chi.URLParam(r, "projectID")
	project, err := s.service.Project(r.Context(), actor, projectID)
	var view app.SubmissionRevisionView
	if err == nil {
		view, err = s.service.ProjectSubmissionRevision(r.Context(), actor, projectID, chi.URLParam(r, "id"))
	}
	if err == nil && project.ConnectedDevices == 0 {
		err = domain.Conflict("CODEX_HANDOFF_WORKSPACE_REQUIRED", "项目尚未连接本地 Workspace，不能生成 Codex 恢复入口")
	}
	if err == nil && len(view.Comments) == 0 && view.Submission.Status != "changes_requested" {
		err = domain.Conflict("CODEX_HANDOFF_FEEDBACK_REQUIRED", "该 SubmissionRevision 尚无可恢复的审核反馈")
	}
	if err != nil {
		s.dispatchResult(w, r, "codex.handoff.review-feedback", codexHandoff{}, err)
		return
	}
	value := newReviewFeedbackCodexHandoff(project.ID, view.Revision.ID, codexHandoffDigest(view.Revision.ContentHash))
	s.dispatchResult(w, r, "codex.handoff.review-feedback", value, nil)
}

func newProjectCodexHandoff(projectID string) codexHandoff {
	spec := codexplugin.DefaultSpec(codexGuideVersion)
	prompt := fmt.Sprintf("[@ContentCloud Video Production](plugin://%s) 在当前已选择的本机 Workspace 中继续 ContentCloud 项目 %s。先调用 workspace_context，并验证返回的 project_id 必须等于 %s；如果未选择 Workspace 或 project_id 不匹配，立即停止，不要扫描其他目录。不要从旧对话历史重建状态，也不要自动执行 pull、claim、publish 或任何本地写入；先报告当前状态和下一步。", spec.PluginID, projectID, projectID)
	return codexHandoff{
		SchemaVersion:              codexHandoffSchemaVersion,
		Kind:                       "project",
		ProjectID:                  projectID,
		Target:                     codexHandoffTarget{Kind: "project", ID: projectID},
		PluginID:                   spec.PluginID,
		PluginVersion:              spec.PluginVersion,
		RequiresNewChat:            true,
		RequiresWorkspaceSelection: true,
		LaunchURL:                  codexPromptDeepLink(prompt),
		Prompt:                     prompt,
		Steps: []string{
			"在 Codex Desktop 中选择已连接该项目的本机 Workspace。",
			"打开新对话并先调用 workspace_context。",
			"核对 project_id 后，再由用户决定是否执行下一步。",
		},
		FallbackURL: "/codex",
	}
}

func newReviewFeedbackCodexHandoff(projectID, revisionID, digest string) codexHandoff {
	spec := codexplugin.DefaultSpec(codexGuideVersion)
	prompt := fmt.Sprintf("[@ContentCloud Video Production](plugin://%s) 在当前已选择的本机 Workspace 中处理 ContentCloud 项目 %s 的审核反馈，目标 SubmissionRevision 为 %s，完整 digest 为 %s。先调用 workspace_context，并验证返回的 project_id 必须等于 %s；如果未选择 Workspace 或 project_id 不匹配，立即停止，不要扫描其他目录。随后只调用 review_feedback_list 读取云端反馈，并核对目标 Revision 与 digest；未经用户明确要求，不要 pull、claim、修改文件或开始新的修订 Run。", spec.PluginID, projectID, revisionID, digest, projectID)
	return codexHandoff{
		SchemaVersion:              codexHandoffSchemaVersion,
		Kind:                       "review_feedback",
		ProjectID:                  projectID,
		Target:                     codexHandoffTarget{Kind: "submission_revision", ID: revisionID, Digest: digest},
		PluginID:                   spec.PluginID,
		PluginVersion:              spec.PluginVersion,
		RequiresNewChat:            true,
		RequiresWorkspaceSelection: true,
		LaunchURL:                  codexPromptDeepLink(prompt),
		Prompt:                     prompt,
		Steps: []string{
			"在 Codex Desktop 中选择已连接该项目的本机 Workspace。",
			"先调用 workspace_context 并核对 project_id。",
			"只读取反馈摘要；pull、claim 和本地修订均等待用户明确要求。",
		},
		FallbackURL: "/codex",
	}
}

func codexPromptDeepLink(prompt string) string {
	query := url.Values{}
	query.Set("prompt", prompt)
	return (&url.URL{Scheme: "codex", Host: "new", RawQuery: query.Encode()}).String()
}

func codexHandoffDigest(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if !strings.HasPrefix(value, "sha256:") {
		value = "sha256:" + value
	}
	return value
}
