package httpapi

import (
	"net/http"
	"strings"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/go-chi/chi/v5"

	"github.com/limecloud/contentcloud/internal/application"
	agentadapter "github.com/limecloud/contentcloud/internal/integration/agent"
)

const agentClientCatalogSchemaVersion = "contentcloud.agent-client-catalog/1.0"

type agentClientCatalog struct {
	SchemaVersion string                          `json:"schema_version"`
	Clients       []agentadapter.ClientDefinition `json:"clients"`
}

func (s *Server) agentClients(w http.ResponseWriter, r *http.Request) {
	s.dispatchResult(w, r, "agent.clients", agentClientCatalog{SchemaVersion: agentClientCatalogSchemaVersion, Clients: agentadapter.Clients()}, nil)
}

func (s *Server) projectAgentHandoff(w http.ResponseWriter, r *http.Request) {
	clientID, err := requestedAgentClient(r)
	var value agentadapter.Handoff
	if err == nil {
		var projectID string
		projectID, err = s.projectHandoffContext(r)
		if err == nil {
			value, err = newProjectAgentHandoff(clientID, projectID)
		}
	}
	s.dispatchResult(w, r, "agent.handoff.project", value, err)
}

func (s *Server) reviewFeedbackAgentHandoff(w http.ResponseWriter, r *http.Request) {
	clientID, err := requestedAgentClient(r)
	var value agentadapter.Handoff
	if err == nil {
		var projectID, revisionID, digest string
		projectID, revisionID, digest, err = s.reviewFeedbackHandoffContext(r)
		if err == nil {
			value, err = newReviewFeedbackAgentHandoff(clientID, projectID, revisionID, digest)
		}
	}
	s.dispatchResult(w, r, "agent.handoff.review-feedback", value, err)
}

func requestedAgentClient(r *http.Request) (string, error) {
	query := r.URL.Query()
	if len(query) != 1 || len(query["client"]) != 1 || strings.TrimSpace(query.Get("client")) == "" {
		return "", fault.Invalid("AGENT_CLIENT_REQUIRED", "生成恢复入口需要唯一的 client 参数")
	}
	return query.Get("client"), nil
}

func (s *Server) projectHandoffContext(r *http.Request) (string, error) {
	actor, _ := auth(r)
	projectID := chi.URLParam(r, "projectID")
	project, err := s.service.Workspace.Project(r.Context(), actor, projectID)
	if err == nil && project.ConnectedDevices == 0 {
		err = fault.Conflict("AGENT_HANDOFF_WORKSPACE_REQUIRED", "项目尚未连接本地工作区，不能生成智能体恢复入口")
	}
	return project.ID, err
}

func (s *Server) reviewFeedbackHandoffContext(r *http.Request) (string, string, string, error) {
	actor, _ := auth(r)
	projectID := chi.URLParam(r, "projectID")
	project, err := s.service.Workspace.Project(r.Context(), actor, projectID)
	var view application.SubmissionRevisionView
	if err == nil {
		view, err = s.service.Review.ProjectSubmissionRevision(r.Context(), actor, projectID, chi.URLParam(r, "id"))
	}
	if err == nil && project.ConnectedDevices == 0 {
		err = fault.Conflict("AGENT_HANDOFF_WORKSPACE_REQUIRED", "项目尚未连接本地工作区，不能生成智能体恢复入口")
	}
	if err == nil && len(view.Comments) == 0 && view.Submission.Status != "changes_requested" {
		err = fault.Conflict("AGENT_HANDOFF_FEEDBACK_REQUIRED", "该提交版本尚无可恢复的审核反馈")
	}
	return project.ID, view.Revision.ID, handoffDigest(view.Revision.ContentHash), err
}

func handoffDigest(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if !strings.HasPrefix(value, "sha256:") {
		value = "sha256:" + value
	}
	return value
}

func newProjectAgentHandoff(clientID, projectID string) (agentadapter.Handoff, error) {
	adapter, err := agentadapter.SelectHandoff(clientID, codexGuideVersion)
	if err != nil {
		return agentadapter.Handoff{}, err
	}
	return adapter.Build(agentadapter.HandoffRequest{Kind: "project", ProjectID: projectID, Target: agentadapter.HandoffTarget{Kind: "project", ID: projectID}})
}

func newReviewFeedbackAgentHandoff(clientID, projectID, revisionID, digest string) (agentadapter.Handoff, error) {
	adapter, err := agentadapter.SelectHandoff(clientID, codexGuideVersion)
	if err != nil {
		return agentadapter.Handoff{}, err
	}
	return adapter.Build(agentadapter.HandoffRequest{Kind: "review_feedback", ProjectID: projectID, Target: agentadapter.HandoffTarget{Kind: "submission_revision", ID: revisionID, Digest: digest}})
}
