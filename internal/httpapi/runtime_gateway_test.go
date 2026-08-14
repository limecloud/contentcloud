package httpapi_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/httpapi"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestRuntimeGatewayHTTPIsAttemptScopedAndRejectsForgedIdentity(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
	started, err := service.Runtime().Start(t.Context(), contentruntime.StartInput{
		TenantID: "tenant-gateway", ProjectID: "project-gateway", WorkTaskID: "gateway-task", BusinessType: "gateway.test",
		SOP: gatewaySOP(), BindingDigest: "sha256:" + strings.Repeat("a", 64), InputDigest: "sha256:" + strings.Repeat("b", 64),
		RuntimePolicyID: "runtime-policy/gateway", ContractMajor: 1, CreatedBy: "user-gateway", IdempotencyKey: "gateway-job",
	})
	if err != nil {
		t.Fatal(err)
	}
	handle, err := service.Runtime().PrepareRemoteDispatch(t.Context(), contentruntime.DispatchInput{
		TenantID: started.Job.TenantID, JobRunID: started.Job.ID, Owner: "device:gateway", HarnessKind: "fake", Role: "writer",
		ExecutionProfileID: "profile-gateway", AllowedTools: []string{contentruntime.ToolChildList}, MaxTokens: 1024, BudgetMinor: 10,
		RemainingDescendants: 1, LeaseFor: time.Minute,
	}, agentadapter.HarnessCapabilities{Kind: "fake", Events: true, Resume: true, StructuredOutput: true, MCPStdio: true, MaxParallelSessions: 1})
	if err != nil {
		t.Fatal(err)
	}
	handle, err = service.Runtime().ActivateDispatch(t.Context(), handle, agentadapter.AgentSessionRef{TenantID: started.Job.TenantID, HarnessKind: "fake", SessionID: "gateway-session"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	defer server.Close()
	endpoint := server.URL + "/api/v1/runtime/mcp/call"

	for _, authorization := range []string{"", "Bearer invalid", "Bearer dt_not-a-runtime-token"} {
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, strings.NewReader(`{"tool_name":"child.list","request_id":"invalid-auth","arguments":{}}`))
		request.Header.Set("Content-Type", "application/json")
		if authorization != "" {
			request.Header.Set("Authorization", authorization)
		}
		response, requestErr := http.DefaultClient.Do(request)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("authorization %q returned %d, want 401", authorization, response.StatusCode)
		}
	}

	forgedBody := `{"tool_name":"child.list","request_id":"forged","tenant_id":"tenant-other","attempt_id":"attempt-other","fence_token":"fence-other","arguments":{}}`
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, strings.NewReader(forgedBody))
	request.Header.Set("Authorization", "Bearer "+handle.GatewayToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("forged identity body returned %d, want decoder rejection", response.StatusCode)
	}

	validBody := `{"tool_name":"child.list","request_id":"valid-call","arguments":{}}`
	request, _ = http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, strings.NewReader(validBody))
	request.Header.Set("Authorization", "Bearer "+handle.GatewayToken)
	request.Header.Set("Content-Type", "application/json")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("valid Gateway call returned %d: %s", response.StatusCode, body)
	}
	var envelope struct {
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil || !envelope.OK {
		t.Fatalf("valid Gateway response = %#v err=%v", envelope, err)
	}

	if _, err := service.Runtime().FinalizeDispatch(t.Context(), handle, contentruntime.DispatchOutcome{State: domain.RuntimeAttemptSucceeded}); err != nil {
		t.Fatal(err)
	}
	request, _ = http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, strings.NewReader(validBody))
	request.Header.Set("Authorization", "Bearer "+handle.GatewayToken)
	request.Header.Set("Content-Type", "application/json")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("terminal Attempt token returned %d, want 401", response.StatusCode)
	}
}

func gatewaySOP() domain.SOPVersion {
	return domain.SOPVersion{ID: "gateway-sop-v1", TenantID: "tenant-gateway", SOPID: "gateway-sop", Version: 1, SchemaVersion: domain.SOPSchemaVersion, Name: "Gateway", Status: "published", DefaultExecutionMode: "agent", Stages: []domain.StageDefinition{{ID: "write", Name: "Write", Order: 10, OutputSchema: "contentcloud.gateway/1.0", ExecutionModes: []string{"agent"}}}}
}
