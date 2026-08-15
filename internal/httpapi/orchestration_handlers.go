package httpapi

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Server) adminWorkOS(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.AdminWorkOS(r.Context(), actor)
	s.dispatchResult(w, r, "admin.work_os.show", value, err)
}

func (s *Server) operationsExecutors(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.OperationsExecutors(r.Context(), actor)
	s.dispatchResult(w, r, "operations.executors.list", value, err)
}

func (s *Server) operationsExecutor(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.OperationsExecutor(r.Context(), actor, chi.URLParam(r, "executorID"))
	s.dispatchResult(w, r, "operations.executor.show", value, err)
}

func (s *Server) operationsSkills(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.OperationsSkills(r.Context(), actor)
	s.dispatchResult(w, r, "operations.skills.list", value, err)
}

func (s *Server) operationsSkill(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.OperationsSkill(r.Context(), actor, chi.URLParam(r, "skillID"), chi.URLParam(r, "skillVersion"))
	s.dispatchResult(w, r, "operations.skill.show", value, err)
}

func (s *Server) updateAdminEnvironment(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.SaveEnvironmentInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.SaveEnvironment(r.Context(), actor, chi.URLParam(r, "id"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "admin.environment.update", value, err)
}

func (s *Server) createAdminEnvironment(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.SaveEnvironmentInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.CreateEnvironment(r.Context(), actor, input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "admin.environment.create", value, err)
}

func (s *Server) createAdminSOP(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.CreateSOPInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.CreateSOP(r.Context(), actor, input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "admin.sop.create", value, err)
}

func (s *Server) updateAdminSOPVersion(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	version, err := parsePathInt(r, "version")
	if err != nil {
		s.fail(w, r, "admin.sop_version.update", err)
		return
	}
	var input app.SaveSOPVersionInput
	if !s.decode(w, r, &input) {
		return
	}
	input.Version = version
	value, err := s.service.SaveSOPVersion(r.Context(), actor, chi.URLParam(r, "sopID"), version, input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "admin.sop_version.update", value, err)
}

func (s *Server) createAdminSOPDraft(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input struct {
		SourceVersion int `json:"source_version"`
	}
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.CreateSOPDraft(r.Context(), actor, chi.URLParam(r, "sopID"), input.SourceVersion, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "admin.sop_version.create", value, err)
}

func (s *Server) publishAdminSOPVersion(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	version, err := parsePathInt(r, "version")
	if err != nil {
		s.fail(w, r, "admin.sop_version.publish", err)
		return
	}
	value, err := s.service.PublishSOP(r.Context(), actor, chi.URLParam(r, "sopID"), version, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "admin.sop_version.publish", value, err)
}

func (s *Server) lintAdminSOPVersion(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	version, err := parsePathInt(r, "version")
	if err != nil {
		s.fail(w, r, "admin.sop_version.lint", err)
		return
	}
	value, err := s.service.LintSOPVersion(r.Context(), actor, chi.URLParam(r, "sopID"), version)
	s.dispatchResult(w, r, "admin.sop_version.lint", value, err)
}

func (s *Server) diffAdminSOPVersions(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	fromVersion, err := parsePathInt(r, "fromVersion")
	if err != nil {
		s.fail(w, r, "admin.sop_version.diff", err)
		return
	}
	toVersion, err := parsePathInt(r, "toVersion")
	if err != nil {
		s.fail(w, r, "admin.sop_version.diff", err)
		return
	}
	value, err := s.service.SOPVersionDiff(r.Context(), actor, chi.URLParam(r, "sopID"), fromVersion, toVersion)
	s.dispatchResult(w, r, "admin.sop_version.diff", value, err)
}

func (s *Server) impactAdminSOPVersion(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	version, err := parsePathInt(r, "version")
	if err != nil {
		s.fail(w, r, "admin.sop_version.impact", err)
		return
	}
	value, err := s.service.SOPVersionImpact(r.Context(), actor, chi.URLParam(r, "sopID"), version)
	s.dispatchResult(w, r, "admin.sop_version.impact", value, err)
}

func (s *Server) previewAdminSOPVersion(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	version, err := parsePathInt(r, "version")
	if err != nil {
		s.fail(w, r, "admin.sop_version.preview", err)
		return
	}
	environmentID := strings.TrimSpace(r.URL.Query().Get("environment_id"))
	value, err := s.service.SOPVersionPreview(r.Context(), actor, chi.URLParam(r, "sopID"), version, environmentID)
	s.dispatchResult(w, r, "admin.sop_version.preview", value, err)
}

func (s *Server) rollbackAdminSOPVersion(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input struct {
		TargetVersion int `json:"target_version"`
	}
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.RollbackSOPVersion(r.Context(), actor, chi.URLParam(r, "sopID"), input.TargetVersion, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "admin.sop_version.rollback", value, err)
}

func (s *Server) retireAdminSOPVersion(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	version, err := parsePathInt(r, "version")
	if err != nil {
		s.fail(w, r, "admin.sop_version.retire", err)
		return
	}
	err = s.service.RetireSOPVersion(r.Context(), actor, chi.URLParam(r, "sopID"), version, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "admin.sop_version.retire", map[string]any{"sop_id": chi.URLParam(r, "sopID"), "version": version, "status": "retired"}, err)
}

func (s *Server) projectSOP(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	binding, version, err := s.service.ProjectSOP(r.Context(), actor, chi.URLParam(r, "projectID"))
	s.dispatchResult(w, r, "project.sop.show", map[string]any{"binding": binding, "sop": version}, err)
}

func (s *Server) bindProjectSOP(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.BindProjectSOPInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.BindProjectSOP(r.Context(), actor, chi.URLParam(r, "projectID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "project.sop.bind", value, err)
}

func (s *Server) workTasks(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.WorkTasks(r.Context(), actor, r.URL.Query().Get("project_id"))
	s.dispatchResult(w, r, "task.list", value, err)
}

func (s *Server) createWorkTask(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.CreateWorkTaskInput
	if !s.decode(w, r, &input) {
		return
	}
	if input.IdempotencyKey == "" {
		input.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	}
	value, err := s.service.CreateWorkTask(r.Context(), actor, input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "task.create", value, err)
}

func (s *Server) workTask(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.WorkTask(r.Context(), actor, chi.URLParam(r, "taskID"))
	s.dispatchResult(w, r, "task.show", value, err)
}

func (s *Server) taskAction(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.TaskActionInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.TaskAction(r.Context(), actor, chi.URLParam(r, "taskID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "task.action", value, err)
}

func (s *Server) reportStage(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.StageReportInput
	if !s.decode(w, r, &input) {
		return
	}
	input.StageID = defaultInput(input.StageID, chi.URLParam(r, "stageID"))
	value, err := s.service.ReportStage(r.Context(), actor, chi.URLParam(r, "taskID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "task.stage.report", value, err)
}

func (s *Server) createMediaGenerationJob(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.CreateMediaGenerationJobInput
	if !s.decode(w, r, &input) {
		return
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		input.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	}
	value, err := s.service.CreateMediaGenerationJob(r.Context(), actor, chi.URLParam(r, "taskID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "media.job.create", value, err)
}

func (s *Server) uploadStoryboardArtifact(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	const maxBytes = 25 * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(1<<20))
	if err := r.ParseMultipartForm(maxBytes + (1 << 20)); err != nil {
		s.fail(w, r, "storyboard.artifact.upload", domain.Invalid("STORYBOARD_MULTIPART_INVALID", "分镜素材上传表单无效或超过大小限制"))
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		s.fail(w, r, "storyboard.artifact.upload", domain.Invalid("STORYBOARD_FILE_REQUIRED", "必须上传一个分镜素材文件"))
		return
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		s.fail(w, r, "storyboard.artifact.upload", err)
		return
	}
	fileName := ""
	if header != nil {
		fileName = header.Filename
	}
	value, err := s.service.UploadStoryboardArtifact(r.Context(), actor, chi.URLParam(r, "taskID"), app.UploadStoryboardArtifactInput{SnapshotID: r.FormValue("snapshot_id"), AssetID: r.FormValue("asset_id"), FileName: fileName, Body: body}, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "storyboard.artifact.upload", value, err)
}

func (s *Server) uploadSeedancePromptPackage(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	const maxBytes = 8 * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(1<<20))
	if err := r.ParseMultipartForm(maxBytes + (1 << 20)); err != nil {
		s.fail(w, r, "seedance.prompt_package.upload", domain.Invalid("SEEDANCE_MULTIPART_INVALID", "Seedance 提示包上传表单无效或超过大小限制"))
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		s.fail(w, r, "seedance.prompt_package.upload", domain.Invalid("SEEDANCE_PROMPT_PACKAGE_REQUIRED", "必须上传 Seedance 提示包 JSON 文件"))
		return
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		s.fail(w, r, "seedance.prompt_package.upload", err)
		return
	}
	fileName := ""
	if header != nil {
		fileName = header.Filename
	}
	value, err := s.service.UploadSeedancePromptPackage(r.Context(), actor, chi.URLParam(r, "taskID"), app.UploadSeedancePromptPackageInput{SnapshotID: r.FormValue("snapshot_id"), FileName: fileName, Body: body}, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "seedance.prompt_package.upload", value, err)
}

func (s *Server) createFinalRender(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.CreateFinalRenderInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.CreateFinalRender(r.Context(), actor, chi.URLParam(r, "taskID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "media.final_render.create", value, err)
}

func (s *Server) approveMediaGenerationJob(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.MediaJobDecisionInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.ApproveMediaGenerationJob(r.Context(), actor, chi.URLParam(r, "id"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "media.job.approve_cost", value, err)
}

func (s *Server) cancelMediaGenerationJob(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.MediaJobDecisionInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.CancelMediaGenerationJob(r.Context(), actor, chi.URLParam(r, "id"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "media.job.cancel", value, err)
}

func (s *Server) reconcileMediaGenerationSubmit(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.MediaJobSubmitReconciliationInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.ReconcileMediaGenerationSubmit(r.Context(), actor, chi.URLParam(r, "id"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "media.job.submit_reconcile", value, err)
}

func (s *Server) decideMediaReview(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.MediaReviewDecisionInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.DecideMediaReview(r.Context(), actor, chi.URLParam(r, "id"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "media.review.decide", value, err)
}

func (s *Server) buildTaskDeliveryPackage(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.BuildTaskDeliveryPackageInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.BuildTaskDeliveryPackage(r.Context(), actor, chi.URLParam(r, "taskID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "task.delivery_package.build", value, err)
}

func (s *Server) runsForTask(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.RunsForWorkTask(r.Context(), actor, chi.URLParam(r, "taskID"))
	s.dispatchResult(w, r, "task.runs", value, err)
}

func (s *Server) taskGates(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.WorkTaskGates(r.Context(), actor, chi.URLParam(r, "taskID"))
	s.dispatchResult(w, r, "task.gates", value, err)
}

func (s *Server) decideGate(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.GateDecisionInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.DecideGate(r.Context(), actor, chi.URLParam(r, "taskID"), chi.URLParam(r, "gateID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "task.gate.decide", value, err)
}

func (s *Server) taskRevisions(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.WorkTaskRevisions(r.Context(), actor, chi.URLParam(r, "taskID"))
	s.dispatchResult(w, r, "task.revisions", value, err)
}

func (s *Server) createTaskRevision(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.CreateTaskRevisionInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.CreateTaskRevision(r.Context(), actor, chi.URLParam(r, "taskID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "task.revision.create", value, err)
}

func (s *Server) taskDeliveries(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.WorkTaskDeliveries(r.Context(), actor, chi.URLParam(r, "taskID"))
	s.dispatchResult(w, r, "task.deliveries", value, err)
}

func (s *Server) taskConversationImports(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.TaskConversationImports(r.Context(), actor, chi.URLParam(r, "taskID"))
	s.dispatchResult(w, r, "conversation_import.list", value, err)
}

func (s *Server) createConversationImport(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.CreateConversationImportInput
	if !s.decode(w, r, &input) {
		return
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		input.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	}
	value, err := s.service.CreateConversationImport(r.Context(), actor, chi.URLParam(r, "taskID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "conversation_import.create", value, err)
}

func (s *Server) conversationImport(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.ConversationImport(r.Context(), actor, chi.URLParam(r, "id"))
	s.dispatchResult(w, r, "conversation_import.show", value, err)
}

func (s *Server) submitConversationBundle(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input domain.ConversationBundle
	if !s.decodeLimit(w, r, &input, 12<<20) {
		return
	}
	value, err := s.service.SubmitConversationBundle(r.Context(), actor, chi.URLParam(r, "id"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "conversation_import.bundle", value, err)
}

func (s *Server) cancelConversationImport(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.CancelConversationImport(r.Context(), actor, chi.URLParam(r, "id"), middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "conversation_import.cancel", value, err)
}

func (s *Server) createTaskDelivery(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.CreateTaskDeliveryInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.CreateTaskDelivery(r.Context(), actor, chi.URLParam(r, "taskID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "task.delivery.create", value, err)
}

func defaultInput(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func parsePathInt(r *http.Request, name string) (int, error) {
	value, err := strconv.Atoi(chi.URLParam(r, name))
	if err != nil || value < 1 {
		return 0, domain.Invalid("PATH_INVALID", name+" 必须是正整数")
	}
	return value, nil
}
