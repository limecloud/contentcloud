package httpapi

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Server) revokeDevice(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.RevokeDevice(r.Context(), actor, chi.URLParam(r, "id"), middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "device.revoke", err)
		return
	}
	s.ok(w, r, "device.revoke", v)
}

func (s *Server) device(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Device(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "device.show", err)
		return
	}
	s.ok(w, r, "device.show", v)
}

func (s *Server) attachDevice(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.AttachDevice(r.Context(), actor, chi.URLParam(r, "id"), chi.URLParam(r, "projectID"), middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "device.attach", err)
		return
	}
	s.ok(w, r, "device.attach", v)
}

func (s *Server) detachDevice(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.DetachDevice(r.Context(), actor, chi.URLParam(r, "id"), chi.URLParam(r, "projectID"), middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "device.detach", err)
		return
	}
	s.ok(w, r, "device.detach", v)
}

func (s *Server) approveDeviceAuth(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in struct {
		UserCode string `json:"user_code"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	v, err := s.service.ApproveUserDeviceLogin(r.Context(), actor, in.UserCode)
	if err != nil {
		s.fail(w, r, "auth.device.approve", err)
		return
	}
	s.ok(w, r, "auth.device.approve", v)
}

func (s *Server) sources(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Sources(r.Context(), actor, chi.URLParam(r, "projectID"))
	if err != nil {
		s.fail(w, r, "source.list", err)
		return
	}
	s.ok(w, r, "source.list", v)
}

func (s *Server) uploadSource(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	r.Body = http.MaxBytesReader(w, r.Body, 101*1024*1024)
	if err := r.ParseMultipartForm(100 * 1024 * 1024); err != nil {
		s.fail(w, r, "source.upload", domain.Invalid("SOURCE_UPLOAD_INVALID", "上传表单无效或文件超过 100MB"))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		s.fail(w, r, "source.upload", domain.Invalid("SOURCE_FILE_REQUIRED", "请选择要上传的文件"))
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 100*1024*1024+1))
	if err != nil || len(data) > 100*1024*1024 {
		s.fail(w, r, "source.upload", domain.Invalid("SOURCE_SIZE_INVALID", "文件读取失败或超过 100MB"))
		return
	}
	mediaType, _, _ := mime.ParseMediaType(header.Header.Get("Content-Type"))
	v, err := s.service.UploadSource(r.Context(), actor, chi.URLParam(r, "projectID"), r.FormValue("name"), r.FormValue("source_type"), header.Filename, mediaType, data, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "source.upload", err)
		return
	}
	s.ok(w, r, "source.upload", v)
}

func (s *Server) sourceRevisions(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.SourceRevisions(r.Context(), actor, chi.URLParam(r, "sourceID"))
	if err != nil {
		s.fail(w, r, "source.revisions", err)
		return
	}
	s.ok(w, r, "source.revisions", v)
}

func (s *Server) uploadSourceRevision(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	r.Body = http.MaxBytesReader(w, r.Body, 101*1024*1024)
	if err := r.ParseMultipartForm(100 * 1024 * 1024); err != nil {
		s.fail(w, r, "source.revise", domain.Invalid("SOURCE_UPLOAD_INVALID", "上传表单无效或文件超过 100MB"))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		s.fail(w, r, "source.revise", domain.Invalid("SOURCE_FILE_REQUIRED", "请选择要上传的文件"))
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 100*1024*1024+1))
	if err != nil || len(data) > 100*1024*1024 {
		s.fail(w, r, "source.revise", domain.Invalid("SOURCE_SIZE_INVALID", "文件读取失败或超过 100MB"))
		return
	}
	mediaType, _, _ := mime.ParseMediaType(header.Header.Get("Content-Type"))
	v, err := s.service.UploadSourceRevision(r.Context(), actor, chi.URLParam(r, "sourceID"), header.Filename, mediaType, data, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "source.revise", err)
		return
	}
	s.ok(w, r, "source.revise", v)
}

func (s *Server) sourceImpact(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.SourceImpact(r.Context(), actor, chi.URLParam(r, "sourceID"))
	if err != nil {
		s.fail(w, r, "source.impact", err)
		return
	}
	s.ok(w, r, "source.impact", v)
}

func (s *Server) evidence(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Evidence(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "evidence.list", err)
		return
	}
	s.ok(w, r, "evidence.list", v)
}

func (s *Server) reviewEvidence(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in struct {
		Decision string `json:"decision"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	v, err := s.service.ReviewEvidence(r.Context(), actor, chi.URLParam(r, "id"), in.Decision, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "evidence.review", err)
		return
	}
	s.ok(w, r, "evidence.review", v)
}

func (s *Server) assets(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Assets(r.Context(), actor, chi.URLParam(r, "projectID"))
	s.dispatchResult(w, r, "asset.list", v, err)
}

func (s *Server) createAsset(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in app.CreateAssetInput
	if !s.decode(w, r, &in) {
		return
	}
	in.ProjectID = chi.URLParam(r, "projectID")
	v, err := s.service.CreateAsset(r.Context(), actor, in, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "asset.create", v, err)
}

func (s *Server) rightsRecords(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.RightsRecords(r.Context(), actor, chi.URLParam(r, "id"))
	s.dispatchResult(w, r, "rights.list", v, err)
}

func (s *Server) createRightsRecord(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in app.CreateRightsRecordInput
	if !s.decode(w, r, &in) {
		return
	}
	in.AssetID = chi.URLParam(r, "id")
	v, err := s.service.CreateRightsRecord(r.Context(), actor, in, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "rights.create", v, err)
}

func (s *Server) reviewRightsRecord(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in struct {
		Decision string `json:"decision"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	v, err := s.service.ReviewRightsRecord(r.Context(), actor, chi.URLParam(r, "id"), in.Decision, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "rights.review", v, err)
}

func (s *Server) knowledgeConflicts(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.KnowledgeConflicts(r.Context(), actor, chi.URLParam(r, "projectID"))
	s.dispatchResult(w, r, "knowledge.conflicts", v, err)
}

func (s *Server) decisionRequests(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.DecisionRequests(r.Context(), actor, chi.URLParam(r, "projectID"))
	s.dispatchResult(w, r, "knowledge.decisions", v, err)
}

func (s *Server) resolveDecisionRequest(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in struct {
		SelectedKnowledgeID string `json:"selected_knowledge_id"`
		Notes               string `json:"notes"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	v, err := s.service.ResolveDecisionRequest(r.Context(), actor, chi.URLParam(r, "id"), in.SelectedKnowledgeID, in.Notes, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "knowledge.decision.resolve", v, err)
}

func (s *Server) sourceRevision(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.SourceRevision(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "source.status", err)
		return
	}
	s.ok(w, r, "source.status", v)
}

func (s *Server) benchmarks(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Benchmarks(r.Context(), actor, chi.URLParam(r, "projectID"))
	if err != nil {
		s.fail(w, r, "benchmark.list", err)
		return
	}
	s.ok(w, r, "benchmark.list", v)
}

func (s *Server) createBenchmark(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in app.CreateBenchmarkInput
	if !s.decode(w, r, &in) {
		return
	}
	in.ProjectID = chi.URLParam(r, "projectID")
	v, err := s.service.CreateBenchmark(r.Context(), actor, in, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "benchmark.create", err)
		return
	}
	s.ok(w, r, "benchmark.create", v)
}

func (s *Server) frameworks(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.Frameworks(r.Context(), actor, chi.URLParam(r, "projectID"))
	if err != nil {
		s.fail(w, r, "framework.list", err)
		return
	}
	s.ok(w, r, "framework.list", v)
}

func (s *Server) createFramework(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in app.CreateFrameworkInput
	if !s.decode(w, r, &in) {
		return
	}
	in.BenchmarkID = chi.URLParam(r, "id")
	v, err := s.service.CreateFramework(r.Context(), actor, in, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "framework.create", err)
		return
	}
	s.ok(w, r, "framework.create", v)
}

func (s *Server) shotPatterns(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.ShotPatterns(r.Context(), actor, chi.URLParam(r, "projectID"))
	if err != nil {
		s.fail(w, r, "shot_pattern.list", err)
		return
	}
	s.ok(w, r, "shot_pattern.list", v)
}

func (s *Server) createShotPattern(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in app.CreateShotPatternInput
	if !s.decode(w, r, &in) {
		return
	}
	in.FrameworkID = chi.URLParam(r, "id")
	v, err := s.service.CreateShotPattern(r.Context(), actor, in, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "shot_pattern.create", err)
		return
	}
	s.ok(w, r, "shot_pattern.create", v)
}

func (s *Server) sellingPoints(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.SellingPoints(r.Context(), actor, chi.URLParam(r, "projectID"))
	if err != nil {
		s.fail(w, r, "selling_point.list", err)
		return
	}
	s.ok(w, r, "selling_point.list", v)
}

func (s *Server) createSellingPoint(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in app.CreateSellingPointInput
	if !s.decode(w, r, &in) {
		return
	}
	in.ProjectID = chi.URLParam(r, "projectID")
	v, err := s.service.CreateSellingPoint(r.Context(), actor, in, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "selling_point.create", err)
		return
	}
	s.ok(w, r, "selling_point.create", v)
}

func (s *Server) visualizationPlans(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.VisualizationPlans(r.Context(), actor, chi.URLParam(r, "projectID"))
	if err != nil {
		s.fail(w, r, "visualization_plan.list", err)
		return
	}
	s.ok(w, r, "visualization_plan.list", v)
}

func (s *Server) createVisualizationPlan(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in app.CreateVisualizationPlanInput
	if !s.decode(w, r, &in) {
		return
	}
	in.SellingPointID = chi.URLParam(r, "id")
	v, err := s.service.CreateVisualizationPlan(r.Context(), actor, in, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "visualization_plan.create", err)
		return
	}
	s.ok(w, r, "visualization_plan.create", v)
}

func (s *Server) reviewVisualizationPlan(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in struct {
		Decision string `json:"decision"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	v, err := s.service.ReviewVisualizationPlan(r.Context(), actor, chi.URLParam(r, "id"), in.Decision, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "visualization_plan.review", err)
		return
	}
	s.ok(w, r, "visualization_plan.review", v)
}

func (s *Server) cancelRun(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.CancelRun(r.Context(), actor, chi.URLParam(r, "id"), middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "run.cancel", err)
		return
	}
	s.ok(w, r, "run.cancel", v)
}

func (s *Server) reviewComments(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.ReviewComments(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "review_comment.list", err)
		return
	}
	s.ok(w, r, "review_comment.list", v)
}

func (s *Server) createReviewComment(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in app.CreateReviewCommentInput
	if !s.decode(w, r, &in) {
		return
	}
	in.SubjectID = chi.URLParam(r, "id")
	v, err := s.service.CreateReviewComment(r.Context(), actor, in, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "review_comment.create", err)
		return
	}
	s.ok(w, r, "review_comment.create", v)
}

func (s *Server) reviewCycles(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.ReviewCycles(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "review_cycle.list", err)
		return
	}
	s.ok(w, r, "review_cycle.list", v)
}

func (s *Server) resolveReviewComment(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.ResolveReviewComment(r.Context(), actor, chi.URLParam(r, "id"), middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "review_comment.resolve", err)
		return
	}
	s.ok(w, r, "review_comment.resolve", v)
}

func (s *Server) createReviewGrant(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in struct {
		ReviewerEmail string `json:"reviewer_email"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	v, err := s.service.CreateReviewGrant(r.Context(), actor, chi.URLParam(r, "id"), in.ReviewerEmail, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "review_grant.create", err)
		return
	}
	s.ok(w, r, "review_grant.create", v)
}

func (s *Server) reviewGrants(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.ReviewGrants(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "review_grant.list", err)
		return
	}
	s.ok(w, r, "review_grant.list", v)
}

func (s *Server) legacyReviewGrants(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.LegacyReviewGrants(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "review_grant.list", err)
		return
	}
	s.ok(w, r, "review_grant.list", v)
}

func (s *Server) revokeReviewGrant(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.RevokeReviewGrant(r.Context(), actor, chi.URLParam(r, "id"), middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "review_grant.revoke", err)
		return
	}
	s.ok(w, r, "review_grant.revoke", v)
}

func (s *Server) artifacts(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.ArtifactPresentations(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "artifact.list", err)
		return
	}
	s.ok(w, r, "artifact.list", v)
}

func (s *Server) artifactPresentation(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.ArtifactPresentation(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "artifact.presentation", err)
		return
	}
	s.ok(w, r, "artifact.presentation", value)
}

func (s *Server) createArtifactOpenRequest(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input struct {
		DeviceID string `json:"device_id"`
		DryRun   bool   `json:"dry_run"`
	}
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.CreateArtifactOpenRequest(r.Context(), actor, chi.URLParam(r, "id"), input.DeviceID, input.DryRun, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "artifact.open", err)
		return
	}
	s.ok(w, r, "artifact.open", value)
}

func (s *Server) artifactOpenRequest(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.ArtifactOpenRequest(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "artifact.open.status", err)
		return
	}
	s.ok(w, r, "artifact.open.status", value)
}

func (s *Server) exportScript(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in struct {
		Format string `json:"format"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	v, err := s.service.ExportScript(r.Context(), actor, chi.URLParam(r, "id"), in.Format, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "artifact.export", err)
		return
	}
	s.ok(w, r, "artifact.export", v)
}

func (s *Server) exportApprovedSnapshot(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in struct {
		ScriptID string `json:"script_id"`
		Format   string `json:"format"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	value, err := s.service.ExportApprovedSnapshot(r.Context(), actor, chi.URLParam(r, "id"), in.ScriptID, in.Format, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "artifact.export", err)
		return
	}
	s.ok(w, r, "artifact.export", value)
}

func (s *Server) approvedSnapshotArtifacts(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.ApprovedSnapshotArtifacts(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "artifact.list", err)
		return
	}
	s.ok(w, r, "artifact.list", value)
}

func (s *Server) createDeliveryPackage(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in struct {
		ScriptID string `json:"script_id"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	value, err := s.service.CreateDeliveryPackage(r.Context(), actor, chi.URLParam(r, "id"), in.ScriptID, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "delivery.create", err)
		return
	}
	s.ok(w, r, "delivery.create", value)
}

func (s *Server) deliveryPackages(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.DeliveryPackages(r.Context(), actor, chi.URLParam(r, "projectID"))
	if err != nil {
		s.fail(w, r, "delivery.list", err)
		return
	}
	s.ok(w, r, "delivery.list", value)
}

func (s *Server) deliveryPackage(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.DeliveryPackage(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "delivery.show", err)
		return
	}
	s.ok(w, r, "delivery.show", value)
}

func (s *Server) downloadArtifact(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	artifact, data, err := s.service.ArtifactBytes(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "artifact.download", err)
		return
	}
	w.Header().Set("Content-Type", artifact.MediaType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", strings.ReplaceAll(filepath.Base(artifact.FileName), "\"", "")))
	w.Header().Set("X-Content-SHA256", artifact.SHA256)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) performanceObservations(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.PerformanceObservations(r.Context(), actor, chi.URLParam(r, "projectID"))
	if err != nil {
		s.fail(w, r, "result.list", err)
		return
	}
	s.ok(w, r, "result.list", v)
}

func (s *Server) createPerformanceObservation(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in app.CreateObservationInput
	if !s.decode(w, r, &in) {
		return
	}
	in.ProjectID = chi.URLParam(r, "projectID")
	v, err := s.service.ImportPerformanceObservations(r.Context(), actor, app.ImportPerformanceInput{ProjectID: in.ProjectID, SourceName: "manual-web", SourceFormat: "manual", Observations: []app.CreateObservationInput{in}}, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "result.create", err)
		return
	}
	s.ok(w, r, "result.create", v)
}

func (s *Server) performanceImportBatches(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.PerformanceImportBatches(r.Context(), actor, chi.URLParam(r, "projectID"))
	if err != nil {
		s.fail(w, r, "result.batches", err)
		return
	}
	s.ok(w, r, "result.batches", v)
}

func (s *Server) createPerformanceImport(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in app.ImportPerformanceInput
	if !s.decode(w, r, &in) {
		return
	}
	in.ProjectID = chi.URLParam(r, "projectID")
	for index := range in.Observations {
		in.Observations[index].ProjectID = in.ProjectID
	}
	v, err := s.service.ImportPerformanceObservations(r.Context(), actor, in, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "result.import", err)
		return
	}
	s.ok(w, r, "result.import", v)
}

func (s *Server) performanceImportDetails(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.PerformanceImportDetails(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "result.batch-show", err)
		return
	}
	s.ok(w, r, "result.batch-show", v)
}

func (s *Server) ratingDecisions(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	v, err := s.service.RatingDecisions(r.Context(), actor, chi.URLParam(r, "projectID"))
	if err != nil {
		s.fail(w, r, "result.ratings", err)
		return
	}
	s.ok(w, r, "result.ratings", v)
}

func (s *Server) createRatingDecision(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var in app.CreateRatingDecisionInput
	if !s.decode(w, r, &in) {
		return
	}
	in.ProjectID = chi.URLParam(r, "projectID")
	v, err := s.service.CreateRatingDecision(r.Context(), actor, in, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "result.rate", err)
		return
	}
	s.ok(w, r, "result.rate", v)
}

func (s *Server) publicReviewProjection(w http.ResponseWriter, r *http.Request) {
	v, err := s.service.ReviewProjection(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		s.fail(w, r, "review.projection", err)
		return
	}
	s.ok(w, r, "review.projection", v)
}

func (s *Server) publicReviewVerify(w http.ResponseWriter, r *http.Request) {
	var in struct {
		OTP string `json:"otp"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	v, err := s.service.VerifyReviewGrant(r.Context(), chi.URLParam(r, "token"), in.OTP)
	if err != nil {
		s.fail(w, r, "review.verify", err)
		return
	}
	s.ok(w, r, "review.verify", v)
}

func (s *Server) publicReviewDecision(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
		ShotID   string `json:"shot_id"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	v, err := s.service.DecideReviewGrant(r.Context(), chi.URLParam(r, "token"), in.Decision, in.Reason, in.ShotID, middleware.GetReqID(r.Context()))
	if err != nil {
		s.fail(w, r, "review.decision", err)
		return
	}
	s.ok(w, r, "review.decision", v)
}
