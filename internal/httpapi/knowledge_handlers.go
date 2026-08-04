package httpapi

import (
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Server) sources(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Sources(r.Context(), actor, chi.URLParam(r, "projectID"))
	s.dispatchResult(w, r, "source.list", value, err)
}

func (s *Server) createSource(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.CreateSourceInput
	if !s.decode(w, r, &input) {
		return
	}
	input.ProjectID = chi.URLParam(r, "projectID")
	value, err := s.service.CreateSource(r.Context(), actor, input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "source.create", value, err)
}

func (s *Server) uploadSource(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	data, fileName, err := multipartSourceFile(w, r)
	if err != nil {
		s.fail(w, r, "source.upload", err)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	sourceType := strings.TrimSpace(r.FormValue("source_type"))
	value, err := s.service.UploadSource(r.Context(), actor, chi.URLParam(r, "projectID"), name, sourceType, fileName, r.FormValue("file_type"), data, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "source.upload", value, err)
}

func (s *Server) sourceRevisions(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.SourceRevisions(r.Context(), actor, chi.URLParam(r, "id"))
	s.dispatchResult(w, r, "source.revisions", value, err)
}

func (s *Server) uploadSourceRevision(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	data, fileName, err := multipartSourceFile(w, r)
	if err != nil {
		s.fail(w, r, "source.revise", err)
		return
	}
	value, err := s.service.UploadSourceRevision(r.Context(), actor, chi.URLParam(r, "sourceID"), fileName, r.FormValue("file_type"), data, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "source.revise", value, err)
}

func (s *Server) sourceRevision(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.SourceRevision(r.Context(), actor, chi.URLParam(r, "id"))
	s.dispatchResult(w, r, "source_revision.show", value, err)
}

func (s *Server) sourceImpact(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.SourceImpact(r.Context(), actor, chi.URLParam(r, "id"))
	s.dispatchResult(w, r, "source.impact", value, err)
}

func (s *Server) evidence(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Evidence(r.Context(), actor, chi.URLParam(r, "id"))
	s.dispatchResult(w, r, "evidence.list", value, err)
}

func (s *Server) reviewEvidence(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input struct {
		Decision string `json:"decision"`
	}
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.ReviewEvidence(r.Context(), actor, chi.URLParam(r, "id"), input.Decision, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "evidence.review", value, err)
}

const maxSourceUploadBytes = 100 * 1024 * 1024

func multipartSourceFile(w http.ResponseWriter, r *http.Request) ([]byte, string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSourceUploadBytes+1<<20)
	if err := r.ParseMultipartForm(maxSourceUploadBytes + 1<<20); err != nil {
		return nil, "", domain.Invalid("SOURCE_MULTIPART_INVALID", "来源上传表单无效或超过大小限制")
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return nil, "", domain.Invalid("SOURCE_FILE_REQUIRED", "必须上传一个来源文件")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxSourceUploadBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 || len(data) > maxSourceUploadBytes {
		return nil, "", domain.Invalid("SOURCE_SIZE_INVALID", "单文件大小必须在 1B 到 100MB 之间")
	}
	if header == nil || strings.TrimSpace(header.Filename) == "" {
		return nil, "", domain.Invalid("SOURCE_FILE_REQUIRED", "来源文件名不能为空")
	}
	fileType := strings.TrimSpace(r.FormValue("file_type"))
	if fileType == "" {
		fileType = strings.TrimSpace(header.Header.Get("Content-Type"))
	}
	if fileType == "" || fileType == "application/octet-stream" {
		fileType = mime.TypeByExtension(filepath.Ext(header.Filename))
	}
	if fileType == "" {
		return nil, "", domain.Invalid("SOURCE_MIME_REQUIRED", "来源文件必须声明 MIME 类型")
	}
	// Keep the normalized MIME in the form value consumed by the handlers.
	r.Form.Set("file_type", fileType)
	return data, header.Filename, nil
}

func (s *Server) knowledgeObjects(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	projectID, ok := s.knowledgeParam(w, r, "projectID", "knowledge_object.list")
	if !ok {
		return
	}
	value, err := s.service.KnowledgeObjects(r.Context(), actor, projectID)
	s.dispatchResult(w, r, "knowledge_object.list", value, err)
}

func (s *Server) createKnowledgeObject(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.CreateKnowledgeObjectInput
	if !s.decode(w, r, &input) {
		return
	}
	projectID, ok := s.knowledgeParam(w, r, "projectID", "knowledge_object.create")
	if !ok {
		return
	}
	input.ProjectID = projectID
	value, err := s.service.CreateKnowledgeObject(r.Context(), actor, input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "knowledge_object.create", value, err)
}

func (s *Server) transitionKnowledgeObject(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.ReviewKnowledgeObjectInput
	if !s.decode(w, r, &input) {
		return
	}
	id, ok := s.knowledgeParam(w, r, "id", "knowledge_object.transition")
	if !ok {
		return
	}
	object, decision, err := s.service.ReviewKnowledgeObject(r.Context(), actor, id, input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "knowledge_object.transition", map[string]any{"object": object, "decision": decision}, err)
}

func (s *Server) knowledgeDecisions(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	id, ok := s.knowledgeParam(w, r, "id", "knowledge_decision.list")
	if !ok {
		return
	}
	value, err := s.service.KnowledgeDecisions(r.Context(), actor, id)
	s.dispatchResult(w, r, "knowledge_decision.list", value, err)
}

func (s *Server) knowledgePacks(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	projectID, ok := s.knowledgeParam(w, r, "projectID", "knowledge_pack.list")
	if !ok {
		return
	}
	value, err := s.service.KnowledgePacks(r.Context(), actor, projectID)
	s.dispatchResult(w, r, "knowledge_pack.list", value, err)
}

func (s *Server) createKnowledgePack(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.CreateKnowledgePackInput
	if !s.decode(w, r, &input) {
		return
	}
	projectID, ok := s.knowledgeParam(w, r, "projectID", "knowledge_pack.create")
	if !ok {
		return
	}
	input.ProjectID = projectID
	value, err := s.service.CreateKnowledgePack(r.Context(), actor, input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "knowledge_pack.create", value, err)
}

func (s *Server) publishKnowledgePack(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	id, ok := s.knowledgeParam(w, r, "id", "knowledge_pack.publish")
	if !ok {
		return
	}
	pack, snapshot, err := s.service.PublishKnowledgePack(r.Context(), actor, id, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "knowledge_pack.publish", map[string]any{"pack": pack, "snapshot": snapshot}, err)
}

func (s *Server) knowledgeSnapshot(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	id, ok := s.knowledgeParam(w, r, "id", "knowledge_snapshot.show")
	if !ok {
		return
	}
	value, err := s.service.KnowledgeSnapshot(r.Context(), actor, id)
	s.dispatchResult(w, r, "knowledge_snapshot.show", value, err)
}

func (s *Server) knowledgeSnapshots(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	projectID, ok := s.knowledgeParam(w, r, "projectID", "knowledge_snapshot.list")
	if !ok {
		return
	}
	packID, ok := s.knowledgeParam(w, r, "packID", "knowledge_snapshot.list")
	if !ok {
		return
	}
	value, err := s.service.KnowledgeSnapshots(r.Context(), actor, projectID, packID)
	s.dispatchResult(w, r, "knowledge_snapshot.list", value, err)
}

func (s *Server) knowledgeParam(w http.ResponseWriter, r *http.Request, name, action string) (string, bool) {
	value, err := url.PathUnescape(chi.URLParam(r, name))
	if err != nil {
		s.fail(w, r, action, domain.Invalid("URL_PARAM_INVALID", "请求路径参数编码无效"))
		return "", false
	}
	return value, true
}

func (s *Server) queryKnowledge(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.QueryKnowledgeInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.QueryKnowledge(r.Context(), actor, input)
	s.dispatchResult(w, r, "knowledge.query", value, err)
}
