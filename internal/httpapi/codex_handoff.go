package httpapi

import (
	"net/http"
	"strings"

	"github.com/limecloud/contentcloud/internal/agentadapter"
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
	projectID, err := s.projectHandoffContext(r)
	if err != nil {
		err = legacyCodexHandoffError(err)
		s.dispatchResult(w, r, "codex.handoff.project", codexHandoff{}, err)
		return
	}
	value := newProjectCodexHandoff(projectID)
	s.dispatchResult(w, r, "codex.handoff.project", value, nil)
}

func (s *Server) reviewFeedbackCodexHandoff(w http.ResponseWriter, r *http.Request) {
	projectID, revisionID, digest, err := s.reviewFeedbackHandoffContext(r)
	if err != nil {
		err = legacyCodexHandoffError(err)
		s.dispatchResult(w, r, "codex.handoff.review-feedback", codexHandoff{}, err)
		return
	}
	value := newReviewFeedbackCodexHandoff(projectID, revisionID, digest)
	s.dispatchResult(w, r, "codex.handoff.review-feedback", value, nil)
}

func newProjectCodexHandoff(projectID string) codexHandoff {
	value, _ := newProjectAgentHandoff(string(agentadapter.ClientCodex), projectID)
	return legacyCodexHandoff(value)
}

func newReviewFeedbackCodexHandoff(projectID, revisionID, digest string) codexHandoff {
	value, _ := newReviewFeedbackAgentHandoff(string(agentadapter.ClientCodex), projectID, revisionID, digest)
	return legacyCodexHandoff(value)
}

func legacyCodexHandoff(value agentadapter.Handoff) codexHandoff {
	return codexHandoff{
		SchemaVersion: codexHandoffSchemaVersion,
		Kind:          value.Kind, ProjectID: value.ProjectID,
		Target:   codexHandoffTarget{Kind: value.Target.Kind, ID: value.Target.ID, Digest: value.Target.Digest},
		PluginID: value.Integration.ID, PluginVersion: value.Integration.Version,
		RequiresNewChat: value.RequiresNewSession, RequiresWorkspaceSelection: value.RequiresWorkspaceSelection,
		LaunchURL: value.Launch.URL, Prompt: value.Prompt, Steps: value.Steps, FallbackURL: value.FallbackURL,
	}
}

func legacyCodexHandoffError(err error) error {
	domainError, ok := err.(*domain.Error)
	if !ok {
		return err
	}
	copy := *domainError
	switch copy.Code {
	case "AGENT_HANDOFF_WORKSPACE_REQUIRED":
		copy.Code = "CODEX_HANDOFF_WORKSPACE_REQUIRED"
	case "AGENT_HANDOFF_FEEDBACK_REQUIRED":
		copy.Code = "CODEX_HANDOFF_FEEDBACK_REQUIRED"
	}
	return &copy
}

func codexHandoffDigest(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if !strings.HasPrefix(value, "sha256:") {
		value = "sha256:" + value
	}
	return value
}
