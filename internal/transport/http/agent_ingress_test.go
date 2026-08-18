package httpapi_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/application"
	agentadapter "github.com/limecloud/contentcloud/internal/integration/agent"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
	httpapi "github.com/limecloud/contentcloud/internal/transport/http"
)

func TestAgentCallbackIngressAuthenticatesAndCompletesAgentSaaSAttempt(t *testing.T) {
	store := memory.New()
	service := application.New(application.DependenciesFrom(store), nil)
	started, err := service.Runtime.Runtime().Start(t.Context(), contentruntime.StartInput{TenantID: "tenant-1", ProjectID: "project-1", WorkTaskID: "agent-ingress-task", BusinessType: "content.produce", SOP: ingressSOP(), BindingDigest: "sha256:" + repeatHex("b"), InputDigest: "sha256:" + repeatHex("c"), RuntimePolicyID: "runtime.test/1", ContractMajor: 1, CreatedBy: "test", IdempotencyKey: "agent-ingress-job"})
	if err != nil {
		t.Fatal(err)
	}
	handle, err := service.Runtime.Runtime().PrepareRemoteDispatch(t.Context(), contentruntime.DispatchInput{TenantID: "tenant-1", JobRunID: started.Job.ID, Owner: "worker-1", HarnessKind: "agent-saas", Role: "writer", ExecutionProfileID: "profile-agent-saas", MaxTokens: 4096, BudgetMinor: 100, RemainingDescendants: 1, LeaseFor: time.Minute}, agentadapter.HarnessCapabilities{Kind: "agent-saas", Events: true, Resume: true, MCPHTTP: true, StructuredOutput: true, SandboxProfile: "remote", MaxParallelSessions: 8})
	if err != nil {
		t.Fatal(err)
	}
	handle, err = service.Runtime.Runtime().ActivateDispatch(t.Context(), handle, agentadapter.AgentSessionRef{TenantID: "tenant-1", HarnessKind: "agent-saas", SessionID: "remote-session-1"})
	if err != nil {
		t.Fatal(err)
	}

	secret := []byte("agent-callback-secret")
	server := httptest.NewServer(httpapi.New(service, nil, false, "", httpapi.WithAgentCallbackSecret("tenant-1", "agent-saas", secret)).Handler())
	defer server.Close()
	endpoint := server.URL + "/api/v1/agent-harnesses/agent-saas/tenants/tenant-1/callbacks"
	body, err := json.Marshal(map[string]any{"message_id": "result-1", "attempt_id": handle.Attempt.ID, "session_id": "remote-session-1", "event_type": "result.completed", "data": map[string]any{"output_refs": []string{"artifact:article-1"}, "output_digest": "sha256:article", "safe_summary": map[string]any{"kind": "article"}, "used_cost_minor": 10}, "occurred_at": time.Now().UTC().Format(time.RFC3339Nano)})
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(signedProviderRequest(t, endpoint, body, secret))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("agent callback status=%d body=%s", response.StatusCode, data)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Applied  bool                          `json:"applied"`
			Replayed bool                          `json:"replayed"`
			Attempt  contentruntime.RuntimeAttempt `json:"attempt"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil || !envelope.OK || !envelope.Data.Applied || envelope.Data.Replayed || envelope.Data.Attempt.State != contentruntime.RuntimeAttemptSucceeded {
		t.Fatalf("unexpected agent callback response: %#v err=%v", envelope, err)
	}
	replay, err := http.DefaultClient.Do(signedProviderRequest(t, endpoint, body, secret))
	if err != nil {
		t.Fatal(err)
	}
	defer replay.Body.Close()
	if replay.StatusCode != http.StatusOK {
		t.Fatalf("agent callback replay status=%d", replay.StatusCode)
	}
	var replayEnvelope struct {
		Data struct {
			Applied  bool `json:"applied"`
			Replayed bool `json:"replayed"`
		} `json:"data"`
	}
	if err := json.NewDecoder(replay.Body).Decode(&replayEnvelope); err != nil || replayEnvelope.Data.Applied || !replayEnvelope.Data.Replayed {
		t.Fatalf("agent callback replay was not idempotent: %#v err=%v", replayEnvelope, err)
	}
	bad, err := http.DefaultClient.Do(signedProviderRequest(t, endpoint, body, []byte("wrong")))
	if err != nil {
		t.Fatal(err)
	}
	defer bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad agent signature status=%d", bad.StatusCode)
	}
}

func repeatHex(value string) string {
	result := ""
	for len(result) < 64 {
		result += value
	}
	return result[:64]
}
