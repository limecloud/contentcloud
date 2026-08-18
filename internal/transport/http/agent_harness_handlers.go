package httpapi

import "net/http"

func (s *Server) agentHarnesses(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth(r)
	value, err := s.service.Catalog.AgentHarnessCapabilities(r.Context(), actor)
	s.dispatchResult(w, r, "agent.harness.list", value, err)
}
