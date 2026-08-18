package httpapi

import (
	"net/http"
	"strings"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/limecloud/contentcloud/internal/application"
)

func (s *Server) channelCallback(w http.ResponseWriter, r *http.Request) {
	adapterID, tenantID, body, digest, ok := s.authenticateChannelIngress(w, r)
	if !ok {
		return
	}
	var input application.ReceiveChannelCallbackInput
	if !decodeProviderIngressBody(body, &input) {
		s.fail(w, r, "channel.callback", fault.Invalid("CHANNEL_CALLBACK_INVALID", "渠道 Callback JSON 无效"))
		return
	}
	input.PayloadDigest = digest
	value, err := s.service.Delivery.ReceiveChannelCallback(r.Context(), tenantID, adapterID, input, middleware.GetReqID(r.Context()))
	s.dispatchResult(w, r, "channel.callback", value, err)
}

func (s *Server) authenticateChannelIngress(w http.ResponseWriter, r *http.Request) (string, string, []byte, string, bool) {
	adapterID := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "adapterID")))
	tenantID := strings.TrimSpace(chi.URLParam(r, "tenantID"))
	if adapterID == "" || tenantID == "" {
		s.fail(w, r, "channel.ingress.auth", fault.Invalid("CHANNEL_INGRESS_SCOPE_INVALID", "渠道 ingress 缺少 Adapter 或租户作用域"))
		return "", "", nil, "", false
	}
	secret := s.channelCallbackSecrets[tenantID+":"+adapterID]
	if len(secret) == 0 {
		s.fail(w, r, "channel.ingress.auth", fault.Policy("CHANNEL_INGRESS_SECRET_UNAVAILABLE", "渠道 ingress 未配置回调密钥", "在服务端密钥配置中登记租户和 Adapter 的 HMAC 密钥"))
		return "", "", nil, "", false
	}
	body, digest, authErr := verifySignedIngress(w, r, secret, "CHANNEL_INGRESS", "渠道")
	if authErr != nil {
		s.fail(w, r, "channel.ingress.auth", authErr)
		return "", "", nil, "", false
	}
	return adapterID, tenantID, body, digest, true
}
