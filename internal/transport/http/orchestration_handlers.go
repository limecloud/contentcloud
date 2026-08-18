package httpapi

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/limecloud/contentcloud/internal/application"
	"github.com/limecloud/contentcloud/internal/work"
)

func (s *Server) adminWorkOS(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Work.AdminWorkOS(r.Context(), actor)
	s.dispatchResult(w, r, "admin.work_os.show", value, err)
}

func (s *Server) operationsExecutors(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Operations.OperationsExecutors(r.Context(), actor)
	s.dispatchResult(w, r, "operations.executors.list", value, err)
}

func (s *Server) operationsExecutor(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Operations.OperationsExecutor(r.Context(), actor, chi.URLParam(r, "executorID"))
	s.dispatchResult(w, r, "operations.executor.show", value, err)
}

func (s *Server) operationsSkills(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Operations.OperationsSkills(r.Context(), actor)
	s.dispatchResult(w, r, "operations.skills.list", value, err)
}

func (s *Server) operationsSkill(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Operations.OperationsSkill(r.Context(), actor, chi.URLParam(r, "skillID"), chi.URLParam(r, "skillVersion"))
	s.dispatchResult(w, r, "operations.skill.show", value, err)
}

func (s *Server) updateAdminEnvironment(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.SaveEnvironmentInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.Work.SaveEnvironment(r.Context(), actor, chi.URLParam(r, "id"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "admin.environment.update", value, err)
}

func (s *Server) createAdminEnvironment(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.SaveEnvironmentInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.Work.CreateEnvironment(r.Context(), actor, input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "admin.environment.create", value, err)
}

func (s *Server) createAdminSOP(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.CreateSOPInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.Work.CreateSOP(r.Context(), actor, input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "admin.sop.create", value, err)
}

func (s *Server) updateAdminSOPVersion(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	version, err := parsePathInt(r, "version")
	if err != nil {
		s.fail(w, r, "admin.sop_version.update", err)
		return
	}
	var input application.SaveSOPVersionInput
	if !s.decode(w, r, &input) {
		return
	}
	input.Version = version
	value, err := s.service.Work.SaveSOPVersion(r.Context(), actor, chi.URLParam(r, "sopID"), version, input, middleware.GetReqID(r.Context()))
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
	value, err := s.service.Work.CreateSOPDraft(r.Context(), actor, chi.URLParam(r, "sopID"), input.SourceVersion, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "admin.sop_version.create", value, err)
}

func (s *Server) publishAdminSOPVersion(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	version, err := parsePathInt(r, "version")
	if err != nil {
		s.fail(w, r, "admin.sop_version.publish", err)
		return
	}
	value, err := s.service.Work.PublishSOP(r.Context(), actor, chi.URLParam(r, "sopID"), version, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "admin.sop_version.publish", value, err)
}

func (s *Server) lintAdminSOPVersion(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	version, err := parsePathInt(r, "version")
	if err != nil {
		s.fail(w, r, "admin.sop_version.lint", err)
		return
	}
	value, err := s.service.Work.LintSOPVersion(r.Context(), actor, chi.URLParam(r, "sopID"), version)
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
	value, err := s.service.Work.SOPVersionDiff(r.Context(), actor, chi.URLParam(r, "sopID"), fromVersion, toVersion)
	s.dispatchResult(w, r, "admin.sop_version.diff", value, err)
}

func (s *Server) impactAdminSOPVersion(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	version, err := parsePathInt(r, "version")
	if err != nil {
		s.fail(w, r, "admin.sop_version.impact", err)
		return
	}
	value, err := s.service.Work.SOPVersionImpact(r.Context(), actor, chi.URLParam(r, "sopID"), version)
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
	value, err := s.service.Work.SOPVersionPreview(r.Context(), actor, chi.URLParam(r, "sopID"), version, environmentID)
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
	value, err := s.service.Work.RollbackSOPVersion(r.Context(), actor, chi.URLParam(r, "sopID"), input.TargetVersion, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "admin.sop_version.rollback", value, err)
}

func (s *Server) retireAdminSOPVersion(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	version, err := parsePathInt(r, "version")
	if err != nil {
		s.fail(w, r, "admin.sop_version.retire", err)
		return
	}
	err = s.service.Work.RetireSOPVersion(r.Context(), actor, chi.URLParam(r, "sopID"), version, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "admin.sop_version.retire", map[string]any{"sop_id": chi.URLParam(r, "sopID"), "version": version, "status": "retired"}, err)
}

func (s *Server) projectSOP(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	binding, version, err := s.service.Work.ProjectSOP(r.Context(), actor, chi.URLParam(r, "projectID"))
	s.dispatchResult(w, r, "project.sop.show", map[string]any{"binding": binding, "sop": version}, err)
}

func (s *Server) bindProjectSOP(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.BindProjectSOPInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.Work.BindProjectSOP(r.Context(), actor, chi.URLParam(r, "projectID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "project.sop.bind", value, err)
}

func (s *Server) workTasks(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Work.WorkTasks(r.Context(), actor, r.URL.Query().Get("project_id"))
	s.dispatchResult(w, r, "task.list", value, err)
}

func (s *Server) createWorkTask(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.CreateWorkTaskInput
	if !s.decode(w, r, &input) {
		return
	}
	if input.IdempotencyKey == "" {
		input.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	}
	value, err := s.service.Work.CreateWorkTask(r.Context(), actor, input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "task.create", value, err)
}

func (s *Server) workTask(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Work.WorkTask(r.Context(), actor, chi.URLParam(r, "taskID"))
	s.dispatchResult(w, r, "task.show", value, err)
}

func (s *Server) taskAction(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.TaskActionInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.Work.TaskAction(r.Context(), actor, chi.URLParam(r, "taskID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "task.action", value, err)
}

func (s *Server) reportStage(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.StageReportInput
	if !s.decode(w, r, &input) {
		return
	}
	input.StageID = defaultInput(input.StageID, chi.URLParam(r, "stageID"))
	value, err := s.service.Work.ReportStage(r.Context(), actor, chi.URLParam(r, "taskID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "task.stage.report", value, err)
}

func (s *Server) createMediaGenerationJob(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.CreateMediaGenerationJobInput
	if !s.decode(w, r, &input) {
		return
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		input.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	}
	value, err := s.service.Delivery.CreateMediaGenerationJob(r.Context(), actor, chi.URLParam(r, "taskID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "media.job.create", value, err)
}

func (s *Server) uploadStoryboardArtifact(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	const maxBytes = 25 * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(1<<20))
	if err := r.ParseMultipartForm(maxBytes + (1 << 20)); err != nil {
		s.fail(w, r, "storyboard.artifact.upload", fault.Invalid("STORYBOARD_MULTIPART_INVALID", "分镜素材上传表单无效或超过大小限制"))
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		s.fail(w, r, "storyboard.artifact.upload", fault.Invalid("STORYBOARD_FILE_REQUIRED", "必须上传一个分镜素材文件"))
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
	value, err := s.service.Delivery.UploadStoryboardArtifact(r.Context(), actor, chi.URLParam(r, "taskID"), application.UploadStoryboardArtifactInput{SnapshotID: r.FormValue("snapshot_id"), AssetID: r.FormValue("asset_id"), FileName: fileName, Body: body}, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "storyboard.artifact.upload", value, err)
}

func (s *Server) uploadSeedancePromptPackage(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	const maxBytes = 8 * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(1<<20))
	if err := r.ParseMultipartForm(maxBytes + (1 << 20)); err != nil {
		s.fail(w, r, "seedance.prompt_package.upload", fault.Invalid("SEEDANCE_MULTIPART_INVALID", "Seedance 提示包上传表单无效或超过大小限制"))
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		s.fail(w, r, "seedance.prompt_package.upload", fault.Invalid("SEEDANCE_PROMPT_PACKAGE_REQUIRED", "必须上传 Seedance 提示包 JSON 文件"))
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
	value, err := s.service.Delivery.UploadSeedancePromptPackage(r.Context(), actor, chi.URLParam(r, "taskID"), application.UploadSeedancePromptPackageInput{SnapshotID: r.FormValue("snapshot_id"), FileName: fileName, Body: body}, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "seedance.prompt_package.upload", value, err)
}

func (s *Server) createFinalRender(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.CreateFinalRenderInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.Delivery.CreateFinalRender(r.Context(), actor, chi.URLParam(r, "taskID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "media.final_render.create", value, err)
}

func (s *Server) approveMediaGenerationJob(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.MediaJobDecisionInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.Delivery.ApproveMediaGenerationJob(r.Context(), actor, chi.URLParam(r, "id"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "media.job.approve_cost", value, err)
}

func (s *Server) cancelMediaGenerationJob(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.MediaJobDecisionInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.Delivery.CancelMediaGenerationJob(r.Context(), actor, chi.URLParam(r, "id"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "media.job.cancel", value, err)
}

func (s *Server) reconcileMediaGenerationSubmit(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.MediaJobSubmitReconciliationInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.Delivery.ReconcileMediaGenerationSubmit(r.Context(), actor, chi.URLParam(r, "id"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "media.job.submit_reconcile", value, err)
}

func (s *Server) decideMediaReview(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.MediaReviewDecisionInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.Delivery.DecideMediaReview(r.Context(), actor, chi.URLParam(r, "id"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "media.review.decide", value, err)
}

func (s *Server) buildTaskDeliveryPackage(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.BuildTaskDeliveryPackageInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.Delivery.BuildTaskDeliveryPackage(r.Context(), actor, chi.URLParam(r, "taskID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "task.delivery_package.build", value, err)
}

func (s *Server) runsForTask(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Work.RunsForWorkTask(r.Context(), actor, chi.URLParam(r, "taskID"))
	s.dispatchResult(w, r, "task.runs", value, err)
}

func (s *Server) taskGates(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Work.WorkTaskGates(r.Context(), actor, chi.URLParam(r, "taskID"))
	s.dispatchResult(w, r, "task.gates", value, err)
}

func (s *Server) decideGate(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.GateDecisionInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.Work.DecideGate(r.Context(), actor, chi.URLParam(r, "taskID"), chi.URLParam(r, "gateID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "task.gate.decide", value, err)
}

func (s *Server) taskRevisions(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Work.WorkTaskRevisions(r.Context(), actor, chi.URLParam(r, "taskID"))
	s.dispatchResult(w, r, "task.revisions", value, err)
}

func (s *Server) createTaskRevision(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.CreateTaskRevisionInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.Work.CreateTaskRevision(r.Context(), actor, chi.URLParam(r, "taskID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "task.revision.create", value, err)
}

func (s *Server) taskDeliveries(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Work.WorkTaskDeliveries(r.Context(), actor, chi.URLParam(r, "taskID"))
	s.dispatchResult(w, r, "task.deliveries", value, err)
}

func (s *Server) taskConversationImports(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Work.TaskConversationImports(r.Context(), actor, chi.URLParam(r, "taskID"))
	s.dispatchResult(w, r, "conversation_import.list", value, err)
}

func (s *Server) createConversationImport(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.CreateConversationImportInput
	if !s.decode(w, r, &input) {
		return
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		input.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	}
	value, err := s.service.Work.CreateConversationImport(r.Context(), actor, chi.URLParam(r, "taskID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "conversation_import.create", value, err)
}

func (s *Server) conversationImport(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Work.ConversationImport(r.Context(), actor, chi.URLParam(r, "id"))
	s.dispatchResult(w, r, "conversation_import.show", value, err)
}

func (s *Server) submitConversationBundle(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input work.ConversationBundle
	if !s.decodeLimit(w, r, &input, 12<<20) {
		return
	}
	value, err := s.service.Work.SubmitConversationBundle(r.Context(), actor, chi.URLParam(r, "id"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "conversation_import.bundle", value, err)
}

func (s *Server) cancelConversationImport(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Work.CancelConversationImport(r.Context(), actor, chi.URLParam(r, "id"), middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "conversation_import.cancel", value, err)
}

func (s *Server) createTaskDelivery(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input application.CreateTaskDeliveryInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.Work.CreateTaskDelivery(r.Context(), actor, chi.URLParam(r, "taskID"), input, middleware.GetReqID(r.Context()))
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
		return 0, fault.Invalid("PATH_INVALID", name+" 必须是正整数")
	}
	return value, nil
}
