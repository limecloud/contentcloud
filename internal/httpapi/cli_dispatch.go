package httpapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Server) handleUserDispatch(w http.ResponseWriter, r *http.Request, req dispatchRequest) bool {
	actor, _, token, err := s.cliUserFromRequest(r)
	if err != nil {
		s.fail(w, r, req.Command, err)
		return true
	}
	requestID := middleware.GetReqID(r.Context())
	switch req.Command {
	case "auth.status":
		tenant, err := s.service.Tenant(r.Context(), actor)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return true
		}
		s.ok(w, r, req.Command, map[string]any{"authenticated": true, "tenant": tenant, "role": actor.Role})
	case "auth.logout":
		if err := s.service.LogoutCLI(r.Context(), token); err != nil {
			s.fail(w, r, req.Command, err)
			return true
		}
		s.ok(w, r, req.Command, map[string]any{"logged_out": true})
	case "tenant.list":
		value, err := s.service.Tenants(r.Context(), actor)
		s.dispatchResult(w, r, req.Command, value, err)
	case "tenant.switch":
		var in struct {
			TenantID string `json:"tenant_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.SwitchCLITenant(r.Context(), token, in.TenantID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "membership.list":
		value, err := s.service.Members(r.Context(), actor)
		s.dispatchResult(w, r, req.Command, value, err)
	case "membership.invite.list":
		value, err := s.service.MembershipInvites(r.Context(), actor)
		s.dispatchResult(w, r, req.Command, value, err)
	case "membership.invite.create":
		var in struct {
			Email string `json:"email"`
			Role  string `json:"role"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.CreateMembershipInvite(r.Context(), actor, in.Email, in.Role, requestID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "membership.invite.accept":
		var in struct {
			Token string `json:"token"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.AcceptMembershipInvite(r.Context(), actor, in.Token, requestID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "membership.invite.revoke":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.RevokeMembershipInvite(r.Context(), actor, in.ID, requestID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "membership.update":
		var in struct {
			UserID string `json:"user_id"`
			Role   string `json:"role"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.UpdateMembershipRole(r.Context(), actor, in.UserID, in.Role, requestID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "membership.revoke":
		var in struct {
			UserID string `json:"user_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.RevokeMembership(r.Context(), actor, in.UserID, requestID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "project.list":
		v, err := s.service.Projects(r.Context(), actor)
		s.dispatchResult(w, r, req.Command, v, err)
	case "project.create":
		var in app.CreateProjectInput
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.CreateProject(r.Context(), actor, in, requestID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "project.show":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Project(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "project.update":
		var in struct {
			ProjectID string `json:"project_id"`
			app.UpdateProjectInput
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.UpdateProject(r.Context(), actor, in.ProjectID, in.UpdateProjectInput, requestID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "project.archive", "project.restore":
		var in struct {
			ProjectID  string `json:"project_id"`
			RowVersion int    `json:"row_version"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		action := strings.TrimPrefix(req.Command, "project.")
		value, err := s.service.SetProjectLifecycle(r.Context(), actor, in.ProjectID, action, in.RowVersion, requestID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "project_template.list":
		value, err := s.service.ProjectTemplates(r.Context(), actor)
		s.dispatchResult(w, r, req.Command, value, err)
	case "project_template.create":
		var in app.CreateProjectTemplateInput
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.CreateProjectTemplate(r.Context(), actor, in, requestID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "device.connect_session.create":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.CreateConnectSession(r.Context(), actor, in.ProjectID, requestID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "device.connect_session.show":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.ConnectSession(r.Context(), actor, in.ID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "device.connect_session.cancel":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.CancelConnectSession(r.Context(), actor, in.ID, requestID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "device.list":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Devices(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "device.show":
		var in struct {
			DeviceID string `json:"device_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Device(r.Context(), actor, in.DeviceID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "device.attach", "device.detach":
		var in struct {
			DeviceID  string `json:"device_id"`
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		var v domain.Device
		var err error
		if req.Command == "device.attach" {
			v, err = s.service.AttachDevice(r.Context(), actor, in.DeviceID, in.ProjectID, requestID)
		} else {
			v, err = s.service.DetachDevice(r.Context(), actor, in.DeviceID, in.ProjectID, requestID)
		}
		s.dispatchResult(w, r, req.Command, v, err)
	case "device.revoke":
		var in struct {
			DeviceID string `json:"device_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.RevokeDevice(r.Context(), actor, in.DeviceID, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "source.list":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Sources(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "source.status":
		var in struct {
			RevisionID string `json:"revision_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.SourceRevision(r.Context(), actor, in.RevisionID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "source.revisions":
		var in struct {
			SourceID string `json:"source_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.SourceRevisions(r.Context(), actor, in.SourceID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "source.impact":
		var in struct {
			SourceID string `json:"source_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.SourceImpact(r.Context(), actor, in.SourceID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "source.upload":
		var in struct {
			ProjectID     string `json:"project_id"`
			Name          string `json:"name"`
			SourceType    string `json:"source_type"`
			FileName      string `json:"file_name"`
			MIME          string `json:"mime"`
			ContentBase64 string `json:"content_base64"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		data, err := base64.StdEncoding.DecodeString(in.ContentBase64)
		if err != nil {
			s.fail(w, r, req.Command, domain.Invalid("SOURCE_CONTENT_INVALID", "来源内容不是有效 base64"))
			return true
		}
		v, err := s.service.UploadSource(r.Context(), actor, in.ProjectID, in.Name, in.SourceType, in.FileName, in.MIME, data, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "source.revise":
		var in struct {
			SourceID      string `json:"source_id"`
			FileName      string `json:"file_name"`
			MIME          string `json:"mime"`
			ContentBase64 string `json:"content_base64"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		data, err := base64.StdEncoding.DecodeString(in.ContentBase64)
		if err != nil {
			s.fail(w, r, req.Command, domain.Invalid("SOURCE_CONTENT_INVALID", "来源内容不是有效 base64"))
			return true
		}
		v, err := s.service.UploadSourceRevision(r.Context(), actor, in.SourceID, in.FileName, in.MIME, data, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "evidence.review":
		var in struct {
			EvidenceID string `json:"evidence_id"`
			Decision   string `json:"decision"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.ReviewEvidence(r.Context(), actor, in.EvidenceID, in.Decision, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "asset.list":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Assets(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "asset.create":
		var in app.CreateAssetInput
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.CreateAsset(r.Context(), actor, in, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "rights.list":
		var in struct {
			AssetID string `json:"asset_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.RightsRecords(r.Context(), actor, in.AssetID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "rights.create":
		var in app.CreateRightsRecordInput
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.CreateRightsRecord(r.Context(), actor, in, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "rights.review":
		var in struct {
			ID       string `json:"id"`
			Decision string `json:"decision"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.ReviewRightsRecord(r.Context(), actor, in.ID, in.Decision, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "knowledge.list":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Knowledge(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "knowledge.show":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.KnowledgeItem(r.Context(), actor, in.ID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "knowledge.review":
		var in struct {
			ID       string `json:"id"`
			Decision string `json:"decision"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.ReviewKnowledge(r.Context(), actor, in.ID, in.Decision, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "knowledge.conflicts":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.KnowledgeConflicts(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "knowledge.decisions":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.DecisionRequests(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "knowledge.decision.resolve":
		var in struct {
			ID                  string `json:"id"`
			SelectedKnowledgeID string `json:"selected_knowledge_id"`
			Notes               string `json:"notes"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.ResolveDecisionRequest(r.Context(), actor, in.ID, in.SelectedKnowledgeID, in.Notes, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "knowledge.extract":
		var in app.CreateKnowledgeExtractionRunInput
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.CreateKnowledgeExtractionRun(r.Context(), actor, in, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "brief.list":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Briefs(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "brief.show":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Brief(r.Context(), actor, in.ID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "brief.create":
		var in app.CreateBriefInput
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.CreateBrief(r.Context(), actor, in, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "brief.review":
		var in struct {
			ID       string `json:"id"`
			Decision string `json:"decision"`
			Reason   string `json:"reason"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.ReviewBriefWithReason(r.Context(), actor, in.ID, in.Decision, in.Reason, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "run.create":
		var in struct {
			BriefID        string `json:"brief_id"`
			IdempotencyKey string `json:"idempotency_key"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.CreateScriptRun(r.Context(), actor, in.BriefID, in.IdempotencyKey, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "run.list":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Runs(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "run.show":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Run(r.Context(), actor, in.ID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "run.attempts":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.RunAttempts(r.Context(), actor, in.ID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "run.cancel":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.CancelRun(r.Context(), actor, in.ID, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "script.list":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Scripts(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "script.show":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Script(r.Context(), actor, in.ID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "script.change.create":
		var in struct {
			BaselineVersionID string `json:"baseline_script_version_id"`
			app.CreateScriptChangeRunInput
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.CreateScriptChangeRun(r.Context(), actor, in.BaselineVersionID, in.CreateScriptChangeRunInput, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "script.review":
		var in struct {
			ID string `json:"id"`
			app.ReviewScriptInput
			Reason string `json:"reason"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		if in.Conclusion == "" {
			in.Conclusion = in.Reason
		}
		v, err := s.service.ReviewScriptWithInput(r.Context(), actor, in.ID, in.ReviewScriptInput, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "review_cycle.list":
		var in struct {
			ScriptID string `json:"script_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.ReviewCycles(r.Context(), actor, in.ScriptID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "artifact.list":
		var in struct {
			ScriptID string `json:"script_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.ArtifactPresentations(r.Context(), actor, in.ScriptID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "artifact.presentation":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.ArtifactPresentation(r.Context(), actor, in.ID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "artifact.open":
		var in struct {
			ID       string `json:"id"`
			DeviceID string `json:"device_id"`
			DryRun   bool   `json:"dry_run"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.CreateArtifactOpenRequest(r.Context(), actor, in.ID, in.DeviceID, in.DryRun, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "artifact.open.status":
		var in struct {
			OpenRequestID string `json:"open_request_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.ArtifactOpenRequest(r.Context(), actor, in.OpenRequestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "artifact.export":
		var in struct {
			SnapshotID string `json:"snapshot_id"`
			ScriptID   string `json:"script_id"`
			Format     string `json:"format"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.ExportApprovedSnapshot(r.Context(), actor, in.SnapshotID, in.ScriptID, in.Format, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "delivery.create":
		var in struct {
			SnapshotID string `json:"snapshot_id"`
			ScriptID   string `json:"script_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.CreateDeliveryPackage(r.Context(), actor, in.SnapshotID, in.ScriptID, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "delivery.list":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.DeliveryPackages(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "delivery.show":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.DeliveryPackage(r.Context(), actor, in.ID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "artifact.download":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		artifact, data, err := s.service.ArtifactBytes(r.Context(), actor, in.ID)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return true
		}
		s.ok(w, r, req.Command, map[string]any{"artifact": artifact, "content_base64": base64.StdEncoding.EncodeToString(data)})
	case "review.create":
		var in struct {
			RevisionID    string `json:"revision_id"`
			ReviewerEmail string `json:"reviewer_email"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.CreateReviewGrant(r.Context(), actor, in.RevisionID, in.ReviewerEmail, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "review.list":
		var in struct {
			RevisionID string `json:"revision_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.ReviewGrants(r.Context(), actor, in.RevisionID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "review.status":
		var in struct {
			RevisionID string `json:"revision_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.SubmissionReviewStatus(r.Context(), actor, in.RevisionID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "review.revoke":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.RevokeReviewGrant(r.Context(), actor, in.ID, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "result.list":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.PerformanceObservations(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "result.batches":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.PerformanceImportBatches(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "result.batch-show":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.PerformanceImportDetails(r.Context(), actor, in.ID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "result.import":
		var in app.ImportPerformanceInput
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.ImportPerformanceObservations(r.Context(), actor, in, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "result.create":
		var in app.CreateObservationInput
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.ImportPerformanceObservations(r.Context(), actor, app.ImportPerformanceInput{ProjectID: in.ProjectID, SourceName: "manual-cli", SourceFormat: "manual", Observations: []app.CreateObservationInput{in}}, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "result.rate":
		var in app.CreateRatingDecisionInput
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.CreateRatingDecision(r.Context(), actor, in, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "result.ratings":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.RatingDecisions(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "lineage.show":
		var in struct {
			ProjectID string `json:"project_id"`
			app.LineageQuery
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.ProjectLineage(r.Context(), actor, in.ProjectID, in.LineageQuery)
		s.dispatchResult(w, r, req.Command, v, err)
	case "lineage.impact":
		var in struct {
			ProjectID string `json:"project_id"`
			app.LineageQuery
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.ProjectImpact(r.Context(), actor, in.ProjectID, in.LineageQuery)
		s.dispatchResult(w, r, req.Command, v, err)
	case "audit.list":
		var in struct {
			ProjectID string `json:"project_id"`
			Limit     int    `json:"limit"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Audit(r.Context(), actor, in.ProjectID, in.Limit)
		s.dispatchResult(w, r, req.Command, v, err)
	case "submission.list":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.Submissions(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "submission.show":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.SubmissionDetails(r.Context(), actor, in.ID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "submission.approve":
		var in struct {
			RevisionID string `json:"revision_id"`
			Reason     string `json:"reason"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.ApproveSubmission(r.Context(), actor, in.RevisionID, in.Reason, requestID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "submission.request_changes":
		var in struct {
			RevisionID  string `json:"revision_id"`
			Reason      string `json:"reason"`
			JSONPointer string `json:"json_pointer"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.RequestSubmissionChanges(r.Context(), actor, in.RevisionID, in.Reason, in.JSONPointer, requestID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "snapshot.list":
		var in struct {
			ProjectID      string `json:"project_id"`
			SubmissionType string `json:"submission_type"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.ApprovedSnapshots(r.Context(), actor, in.ProjectID, in.SubmissionType)
		s.dispatchResult(w, r, req.Command, value, err)
	case "snapshot.show":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.ApprovedSnapshot(r.Context(), actor, in.ID)
		s.dispatchResult(w, r, req.Command, value, err)
	default:
		return false
	}
	return true
}

func (s *Server) cliUserFromRequest(r *http.Request) (app.Actor, domain.User, string, error) {
	header := r.Header.Get("Authorization")
	token := strings.TrimPrefix(header, "Bearer ")
	actor, user, err := s.service.CLITokenActor(r.Context(), token)
	return actor, user, token, err
}

func decodeParams(w http.ResponseWriter, r *http.Request, s *Server, req dispatchRequest, out any) bool {
	if len(req.Params) == 0 {
		req.Params = []byte("{}")
	}
	if err := strictDecodeParams(req.Params, out); err != nil {
		s.fail(w, r, req.Command, domain.Invalid("INPUT_INVALID", "命令参数错误"))
		return false
	}
	return true
}

func strictDecodeParams(body []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return domain.Invalid("INPUT_INVALID", "命令参数只能包含一个 JSON 对象")
	}
	return nil
}

func (s *Server) dispatchResult(w http.ResponseWriter, r *http.Request, command string, value any, err error) {
	if err != nil {
		s.fail(w, r, command, err)
		return
	}
	s.ok(w, r, command, value)
}
