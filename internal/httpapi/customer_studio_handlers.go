package httpapi

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/app"
)

type studioExecutionClient struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Available   bool   `json:"available"`
}

type studioExecutionClientCatalog struct {
	Clients []studioExecutionClient `json:"clients"`
}

func (s *Server) customerStudioBootstrap(w http.ResponseWriter, r *http.Request) {
	actor, user := auth(r)
	value, err := s.service.CustomerStudioBootstrap(r.Context(), actor, user)
	s.dispatchResult(w, r, "studio.bootstrap", value, err)
}

func (s *Server) customerStudioExecutionClients(w http.ResponseWriter, r *http.Request) {
	clients := agentadapter.Clients()
	result := studioExecutionClientCatalog{Clients: make([]studioExecutionClient, 0, len(clients))}
	for _, client := range clients {
		result.Clients = append(result.Clients, studioExecutionClient{
			ID: string(client.ID), DisplayName: client.DisplayName,
			Available: client.CapabilityStatus(agentadapter.CapabilityWorkspaceBootstrap) == agentadapter.SupportAvailable,
		})
	}
	s.dispatchResult(w, r, "studio.execution_client.list", result, nil)
}

func (s *Server) createCustomerStudioConnectSession(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.CreateCustomerStudioConnectSession(r.Context(), actor, chi.URLParam(r, "projectID"), middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "studio.execution_client.connect", value, err)
}

func (s *Server) customerStudioConnectSession(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.CustomerStudioConnectSession(r.Context(), actor, chi.URLParam(r, "sessionID"))
	s.dispatchResult(w, r, "studio.execution_client.connection", value, err)
}

func (s *Server) approveCustomerStudioConnectSession(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.ApproveCustomerStudioConnectSession(r.Context(), actor, chi.URLParam(r, "sessionID"), middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "studio.execution_client.approve", value, err)
}

func (s *Server) denyCustomerStudioConnectSession(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.DenyCustomerStudioConnectSession(r.Context(), actor, chi.URLParam(r, "sessionID"), middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "studio.execution_client.deny", value, err)
}

func (s *Server) cancelCustomerStudioConnectSession(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.CancelCustomerStudioConnectSession(r.Context(), actor, chi.URLParam(r, "sessionID"), middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "studio.execution_client.cancel", value, err)
}

func (s *Server) customerStudioTasks(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.CustomerStudioTasks(r.Context(), actor)
	s.dispatchResult(w, r, "studio.task.list", value, err)
}

func (s *Server) customerStudioTask(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.CustomerStudioTask(r.Context(), actor, chi.URLParam(r, "taskID"))
	s.dispatchResult(w, r, "studio.task.show", value, err)
}

func (s *Server) createCustomerStudioTask(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.StudioCreateTaskInput
	if !s.decode(w, r, &input) {
		return
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		input.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	}
	value, err := s.service.CreateCustomerStudioTask(r.Context(), actor, input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "studio.task.create", value, err)
}

func (s *Server) customerStudioTaskAction(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.TaskActionInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.CustomerStudioTaskAction(r.Context(), actor, chi.URLParam(r, "taskID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "studio.task.action", value, err)
}

func (s *Server) addCustomerStudioInspiration(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.StudioAddInspirationInput
	if !s.decode(w, r, &input) {
		return
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		input.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	}
	value, err := s.service.AddCustomerStudioInspiration(r.Context(), actor, chi.URLParam(r, "taskID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "studio.inspiration.add", value, err)
}

func (s *Server) decideCustomerStudioTask(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.GateDecisionInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.DecideCustomerStudioTask(r.Context(), actor, chi.URLParam(r, "taskID"), chi.URLParam(r, "decisionID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "studio.decision.create", value, err)
}

func (s *Server) attachCustomerStudioAssets(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.StudioAttachAssetsInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.AttachCustomerStudioAssets(r.Context(), actor, chi.URLParam(r, "taskID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "studio.task_assets.attach", value, err)
}

func (s *Server) attachCustomerStudioMaterials(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.StudioAttachMaterialsInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.AttachCustomerWorkspaceMaterials(r.Context(), actor, chi.URLParam(r, "taskID"), input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "studio.task_materials.attach", value, err)
}

func (s *Server) createCustomerStudioFolder(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	var input app.CreateWorkspaceFolderInput
	if !s.decode(w, r, &input) {
		return
	}
	value, err := s.service.CreateCustomerWorkspaceFolder(r.Context(), actor, input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "studio.workspace_folder.create", value, err)
}

func (s *Server) uploadCustomerStudioMaterial(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	data, fileName, err := multipartSourceFile(w, r)
	if err != nil {
		s.fail(w, r, "studio.workspace_material.upload", err)
		return
	}
	value, err := s.service.UploadCustomerWorkspaceMaterial(r.Context(), actor, strings.TrimSpace(r.FormValue("project_id")), strings.TrimSpace(r.FormValue("folder_ref")), strings.TrimSpace(r.FormValue("title")), fileName, r.FormValue("file_type"), data, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "studio.workspace_material.upload", value, err)
}

func (s *Server) downloadCustomerStudioMaterial(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	material, data, err := s.service.CustomerWorkspaceMaterialBytes(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "studio.workspace_material.download", err)
		return
	}
	w.Header().Set("Content-Type", material.MIMEType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", strings.ReplaceAll(filepath.Base(material.FileName), "\"", "")))
	w.Header().Set("Content-Length", strconv.FormatInt(material.ByteSize, 10))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) customerStudioAssets(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.CustomerStudioAssets(r.Context(), actor, strings.TrimSpace(r.URL.Query().Get("project_id")))
	s.dispatchResult(w, r, "studio.asset.list", value, err)
}

func (s *Server) customerStudioCreativeResult(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.CustomerStudioCreativeResult(r.Context(), actor, strings.TrimSpace(r.URL.Query().Get("task_id")), chi.URLParam(r, "resultID"))
	s.dispatchResult(w, r, "studio.asset.show", value, err)
}

func (s *Server) customerStudioDeliveries(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.CustomerStudioDeliveries(r.Context(), actor)
	s.dispatchResult(w, r, "studio.delivery.list", value, err)
}
