package httpapi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Server) runtimeGatewayCall(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if !strings.HasPrefix(token, "rtg_") {
		s.fail(w, r, "runtime.gateway.call", domain.E("authentication", "runtime_gateway", "RUNTIME_GATEWAY_TOKEN_INVALID", "Runtime Gateway 凭据无效", 3))
		return
	}
	var input app.RuntimeGatewayCallInput
	if !s.decodeLimit(w, r, &input, 128<<10) {
		return
	}
	value, err := s.service.CallRuntimeMCPWithGatewayToken(r.Context(), token, input)
	if err != nil {
		s.fail(w, r, "runtime.gateway.call", err)
		return
	}
	s.write(w, http.StatusOK, envelope{OK: true, Command: "runtime.gateway.call", RequestID: middleware.GetReqID(r.Context()), Data: value, Meta: map[string]any{}})
}
