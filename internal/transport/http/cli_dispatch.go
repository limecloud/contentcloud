package httpapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/limecloud/contentcloud/internal/application"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
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
		tenant, err := s.service.Identity.Tenant(r.Context(), actor)
		if err != nil {
			s.fail(w, r, req.Command, err)
			return true
		}
		s.ok(w, r, req.Command, map[string]any{"authenticated": true, "tenant": tenant, "role": actor.Role})
	case "auth.logout":
		if err := s.service.Identity.LogoutCLI(r.Context(), token); err != nil {
			s.fail(w, r, req.Command, err)
			return true
		}
		s.ok(w, r, req.Command, map[string]any{"logged_out": true})
	case "tenant.list":
		value, err := s.service.Identity.Tenants(r.Context(), actor)
		s.dispatchResult(w, r, req.Command, value, err)
	case "tenant.switch":
		var in struct {
			TenantID string `json:"tenant_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.Identity.SwitchCLITenant(r.Context(), token, in.TenantID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "membership.list":
		value, err := s.service.Identity.Members(r.Context(), actor)
		s.dispatchResult(w, r, req.Command, value, err)
	case "membership.invite.list":
		value, err := s.service.Identity.MembershipInvites(r.Context(), actor)
		s.dispatchResult(w, r, req.Command, value, err)
	case "membership.invite.create":
		var in struct {
			Email string `json:"email"`
			Role  string `json:"role"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.Identity.CreateMembershipInvite(r.Context(), actor, in.Email, in.Role, requestID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "membership.invite.accept":
		var in struct {
			Token string `json:"token"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.Identity.AcceptMembershipInvite(r.Context(), actor, in.Token, requestID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "membership.invite.revoke":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.Identity.RevokeMembershipInvite(r.Context(), actor, in.ID, requestID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "membership.update":
		var in struct {
			UserID string `json:"user_id"`
			Role   string `json:"role"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.Identity.UpdateMembershipRole(r.Context(), actor, in.UserID, in.Role, requestID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "membership.revoke":
		var in struct {
			UserID string `json:"user_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.Identity.RevokeMembership(r.Context(), actor, in.UserID, requestID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "project.list":
		v, err := s.service.Workspace.Projects(r.Context(), actor)
		s.dispatchResult(w, r, req.Command, v, err)
	case "project.create":
		var in application.CreateProjectInput
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.Workspace.CreateProject(r.Context(), actor, in, requestID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "project.show":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Workspace.Project(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "project.update":
		var in struct {
			ProjectID string `json:"project_id"`
			application.UpdateProjectInput
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.Identity.UpdateProject(r.Context(), actor, in.ProjectID, in.UpdateProjectInput, requestID)
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
		value, err := s.service.Identity.SetProjectLifecycle(r.Context(), actor, in.ProjectID, action, in.RowVersion, requestID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "project_template.list":
		value, err := s.service.Identity.ProjectTemplates(r.Context(), actor)
		s.dispatchResult(w, r, req.Command, value, err)
	case "project_template.create":
		var in application.CreateProjectTemplateInput
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.Identity.CreateProjectTemplate(r.Context(), actor, in, requestID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "device.connect_session.create":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.Workspace.CreateConnectSession(r.Context(), actor, in.ProjectID, requestID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "device.connect_session.show":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.Workspace.ConnectSession(r.Context(), actor, in.ID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "device.connect_session.cancel":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.Identity.CancelConnectSession(r.Context(), actor, in.ID, requestID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "device.list":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Workspace.Devices(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "device.show":
		var in struct {
			DeviceID string `json:"device_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Workspace.Device(r.Context(), actor, in.DeviceID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "device.attach", "device.detach":
		var in struct {
			DeviceID  string `json:"device_id"`
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		var v workspacedomain.Device
		var err error
		if req.Command == "device.attach" {
			v, err = s.service.Workspace.AttachDevice(r.Context(), actor, in.DeviceID, in.ProjectID, requestID)
		} else {
			v, err = s.service.Workspace.DetachDevice(r.Context(), actor, in.DeviceID, in.ProjectID, requestID)
		}
		s.dispatchResult(w, r, req.Command, v, err)
	case "device.revoke":
		var in struct {
			DeviceID string `json:"device_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Workspace.RevokeDevice(r.Context(), actor, in.DeviceID, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "device.credential.rotate":
		var in struct {
			DeviceID string `json:"device_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Workspace.RotateDeviceCredential(r.Context(), actor, in.DeviceID, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "source.list":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Source.Sources(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "source.search":
		var in application.SearchSourcesInput
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Source.SearchSources(r.Context(), actor, in, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "source.fetch":
		var in application.FetchSourceInput
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Source.FetchSource(r.Context(), actor, in, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "source.status":
		var in struct {
			RevisionID string `json:"revision_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Source.SourceRevision(r.Context(), actor, in.RevisionID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "source.revisions":
		var in struct {
			SourceID string `json:"source_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Source.SourceRevisions(r.Context(), actor, in.SourceID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "source.impact":
		var in struct {
			SourceID string `json:"source_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Source.SourceImpact(r.Context(), actor, in.SourceID)
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
			s.fail(w, r, req.Command, fault.Invalid("SOURCE_CONTENT_INVALID", "来源内容不是有效 base64"))
			return true
		}
		v, err := s.service.Source.UploadSource(r.Context(), actor, in.ProjectID, in.Name, in.SourceType, in.FileName, in.MIME, data, requestID)
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
			s.fail(w, r, req.Command, fault.Invalid("SOURCE_CONTENT_INVALID", "来源内容不是有效 base64"))
			return true
		}
		v, err := s.service.Source.UploadSourceRevision(r.Context(), actor, in.SourceID, in.FileName, in.MIME, data, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "evidence.review":
		var in struct {
			EvidenceID string `json:"evidence_id"`
			Decision   string `json:"decision"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Source.ReviewEvidence(r.Context(), actor, in.EvidenceID, in.Decision, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "asset.list":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Review.Assets(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "asset.create":
		var in application.CreateAssetInput
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Review.CreateAsset(r.Context(), actor, in, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "rights.list":
		var in struct {
			AssetID string `json:"asset_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Review.RightsRecords(r.Context(), actor, in.AssetID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "rights.create":
		var in application.CreateRightsRecordInput
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Review.CreateRightsRecord(r.Context(), actor, in, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "rights.review":
		var in struct {
			ID       string `json:"id"`
			Decision string `json:"decision"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Review.ReviewRightsRecord(r.Context(), actor, in.ID, in.Decision, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "knowledge.list":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Source.KnowledgeObjects(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "knowledge.show":
		var in struct {
			ID      string `json:"id"`
			Version int    `json:"version"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Source.KnowledgeObject(r.Context(), actor, in.ID, in.Version)
		s.dispatchResult(w, r, req.Command, v, err)
	case "knowledge.review":
		var in struct {
			ID              string `json:"id"`
			ExpectedVersion int    `json:"expected_version"`
			ExpectedDigest  string `json:"expected_digest"`
			Decision        string `json:"decision"`
			Reason          string `json:"reason"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		object, decision, err := s.service.Source.ReviewKnowledgeObject(r.Context(), actor, in.ID, application.ReviewKnowledgeObjectInput{ExpectedVersion: in.ExpectedVersion, ExpectedDigest: in.ExpectedDigest, Decision: in.Decision, Reason: in.Reason}, requestID)
		s.dispatchResult(w, r, req.Command, map[string]any{"object": object, "decision": decision}, err)
	case "knowledge.extract":
		var in application.CreateKnowledgeExtractionRunInput
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Source.CreateKnowledgeExtractionRun(r.Context(), actor, in, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "run.list":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Runtime.Runs(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "run.show":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Runtime.Run(r.Context(), actor, in.ID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "run.events":
		var in struct {
			ID    string `json:"id"`
			After int64  `json:"after"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Runtime.RunProgress(r.Context(), actor, in.ID, in.After)
		s.dispatchResult(w, r, req.Command, v, err)
	case "run.cancel":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Runtime.CancelRun(r.Context(), actor, in.ID, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "artifact.export":
		var in struct {
			SnapshotID    string `json:"snapshot_id"`
			ContentItemID string `json:"content_item_id"`
			Format        string `json:"format"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Review.ExportApprovedSnapshot(r.Context(), actor, in.SnapshotID, in.ContentItemID, in.Format, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "delivery.create":
		var in struct {
			SnapshotID    string `json:"snapshot_id"`
			ContentItemID string `json:"content_item_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Review.CreateDeliveryPackage(r.Context(), actor, in.SnapshotID, in.ContentItemID, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "delivery.list":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Review.DeliveryPackages(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "delivery.show":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Review.DeliveryPackage(r.Context(), actor, in.ID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "channel.adapter.list":
		s.ok(w, r, req.Command, s.service.Delivery.ChannelAdapterIDs())
	case "channel.binding.create":
		var in application.CreateChannelBindingInput
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Delivery.CreateChannelBinding(r.Context(), actor, in, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "channel.binding.list":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Delivery.ChannelBindings(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "channel.publication.prepare":
		var in application.PrepareChannelPublicationInput
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Delivery.PrepareChannelPublication(r.Context(), actor, in, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "channel.publication.submit", "channel.publication.inspect":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		if req.Command == "channel.publication.submit" {
			v, err := s.service.Delivery.SubmitChannelPublication(r.Context(), actor, in.ID, requestID)
			s.dispatchResult(w, r, req.Command, v, err)
		} else {
			v, err := s.service.Delivery.InspectChannelPublication(r.Context(), actor, in.ID, requestID)
			s.dispatchResult(w, r, req.Command, v, err)
		}
	case "channel.publication.receipt":
		var in struct {
			ID string `json:"id"`
			application.RecordManualChannelReceiptInput
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Delivery.RecordManualChannelReceipt(r.Context(), actor, in.ID, in.RecordManualChannelReceiptInput, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "channel.publication.withdraw":
		var in struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Delivery.WithdrawChannelPublication(r.Context(), actor, in.ID, in.Reason, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "channel.publication.list":
		var in struct {
			TaskID string `json:"task_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Delivery.ChannelPublications(r.Context(), actor, in.TaskID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "channel.publication.reconcile":
		var in struct {
			Limit int `json:"limit"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Delivery.ReconcileChannelPublications(r.Context(), actor, in.Limit, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "channel.performance.import":
		var in struct {
			ID string `json:"id"`
			application.ImportChannelPerformanceInput
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Delivery.ImportChannelPerformance(r.Context(), actor, in.ID, in.ImportChannelPerformanceInput, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "model.provider.list":
		s.ok(w, r, req.Command, s.service.Delivery.ModelProviderIDs())
	case "model.candidate.generate":
		var in struct {
			TaskID string `json:"task_id"`
			application.GenerateModelCandidateInput
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Delivery.GenerateModelCandidate(r.Context(), actor, in.TaskID, in.GenerateModelCandidateInput, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "model.receipt.list":
		var in struct {
			TaskID string `json:"task_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Delivery.ModelGenerationReceipts(r.Context(), actor, in.TaskID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "connector.adapter.list":
		s.ok(w, r, req.Command, s.service.Source.ConnectorAdapterIDs())
	case "connector.binding.create":
		var in application.CreateConnectorBindingInput
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Source.CreateConnectorBinding(r.Context(), actor, in, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "connector.binding.list":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Source.ConnectorBindings(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "connector.sync":
		var in struct {
			BindingID string `json:"binding_id"`
			application.SyncConnectorInput
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Source.SyncConnector(r.Context(), actor, in.BindingID, in.SyncConnectorInput, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "connector.receipt.list":
		var in struct {
			BindingID string `json:"binding_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Source.ConnectorReceipts(r.Context(), actor, in.BindingID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "content_profile.list":
		s.ok(w, r, req.Command, s.service.Catalog.ContentProfiles())
	case "content_profile.install":
		var in struct {
			ProfileID string `json:"profile_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Catalog.InstallContentProfile(r.Context(), actor, in.ProfileID, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "artifact.download":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		artifact, data, err := s.service.Review.ArtifactBytes(r.Context(), actor, in.ID)
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
		v, err := s.service.Review.CreateReviewGrant(r.Context(), actor, in.RevisionID, in.ReviewerEmail, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "review.list":
		var in struct {
			RevisionID string `json:"revision_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Review.ReviewGrants(r.Context(), actor, in.RevisionID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "review.status":
		var in struct {
			RevisionID string `json:"revision_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Review.SubmissionReviewStatus(r.Context(), actor, in.RevisionID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "review.revoke":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Review.RevokeReviewGrant(r.Context(), actor, in.ID, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "result.list":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Performance.PerformanceObservations(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "result.batches":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Performance.PerformanceImportBatches(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "result.batch-show":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Performance.PerformanceImportDetails(r.Context(), actor, in.ID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "result.import":
		var in application.ImportPerformanceInput
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Performance.ImportPerformanceObservations(r.Context(), actor, in, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "result.create":
		var in application.CreateObservationInput
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Performance.ImportPerformanceObservations(r.Context(), actor, application.ImportPerformanceInput{ProjectID: in.ProjectID, SourceName: "manual-cli", SourceFormat: "manual", Observations: []application.CreateObservationInput{in}}, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "result.rate":
		var in application.CreateRatingDecisionInput
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Performance.CreateRatingDecision(r.Context(), actor, in, requestID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "result.ratings":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Performance.RatingDecisions(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, v, err)
	case "lineage.show":
		var in struct {
			ProjectID string `json:"project_id"`
			application.LineageQuery
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Operations.ProjectLineage(r.Context(), actor, in.ProjectID, in.LineageQuery)
		s.dispatchResult(w, r, req.Command, v, err)
	case "lineage.impact":
		var in struct {
			ProjectID string `json:"project_id"`
			application.LineageQuery
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Operations.ProjectImpact(r.Context(), actor, in.ProjectID, in.LineageQuery)
		s.dispatchResult(w, r, req.Command, v, err)
	case "audit.list":
		var in struct {
			ProjectID string `json:"project_id"`
			Limit     int    `json:"limit"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		v, err := s.service.Operations.Audit(r.Context(), actor, in.ProjectID, in.Limit)
		s.dispatchResult(w, r, req.Command, v, err)
	case "submission.list":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.Review.Submissions(r.Context(), actor, in.ProjectID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "submission.show":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.Review.SubmissionDetails(r.Context(), actor, in.ID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "submission.approve":
		var in struct {
			RevisionID string `json:"revision_id"`
			Reason     string `json:"reason"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.Review.ApproveSubmission(r.Context(), actor, in.RevisionID, in.Reason, requestID)
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
		value, err := s.service.Review.RequestSubmissionChanges(r.Context(), actor, in.RevisionID, in.Reason, in.JSONPointer, requestID)
		s.dispatchResult(w, r, req.Command, value, err)
	case "snapshot.list":
		var in struct {
			ProjectID      string `json:"project_id"`
			SubmissionType string `json:"submission_type"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.Review.ApprovedSnapshots(r.Context(), actor, in.ProjectID, in.SubmissionType)
		s.dispatchResult(w, r, req.Command, value, err)
	case "snapshot.show":
		var in struct {
			ID string `json:"id"`
		}
		if !decodeParams(w, r, s, req, &in) {
			return true
		}
		value, err := s.service.Review.ApprovedSnapshot(r.Context(), actor, in.ID)
		s.dispatchResult(w, r, req.Command, value, err)
	default:
		return false
	}
	return true
}

func (s *Server) cliUserFromRequest(r *http.Request) (application.Actor, identitydomain.User, string, error) {
	header := r.Header.Get("Authorization")
	token := strings.TrimPrefix(header, "Bearer ")
	actor, user, err := s.service.Identity.CLITokenActor(r.Context(), token)
	return actor, user, token, err
}

func decodeParams(w http.ResponseWriter, r *http.Request, s *Server, req dispatchRequest, out any) bool {
	if len(req.Params) == 0 {
		req.Params = []byte("{}")
	}
	if err := strictDecodeParams(req.Params, out); err != nil {
		s.fail(w, r, req.Command, fault.Invalid("INPUT_INVALID", "命令参数错误"))
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
		return fault.Invalid("INPUT_INVALID", "命令参数只能包含一个 JSON 对象")
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
