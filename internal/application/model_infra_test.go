package application_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/application"
	modelprovider "github.com/limecloud/contentcloud/internal/integration/provider/model"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	"github.com/limecloud/contentcloud/internal/work"
)

type fakeModelProvider struct{}

func (fakeModelProvider) Detect(context.Context) (modelprovider.Capability, error) {
	return modelprovider.Capability{Provider: "fake", Model: "content-model", StructuredOutput: true}, nil
}

func (fakeModelProvider) Complete(_ context.Context, input modelprovider.CompletionRequest) (modelprovider.CompletionResult, error) {
	body := json.RawMessage(`{"title":"候选标题","body":"待审核正文"}`)
	return modelprovider.CompletionResult{Provider: "fake", Model: "content-model", Content: string(body), Structured: body, InputTokens: 12, OutputTokens: 8, TotalTokens: 20, RequestID: input.RequestID, ReceivedAt: time.Now().UTC()}, nil
}

func TestGenerateModelCandidateCreatesDraftAndReceiptAtomically(t *testing.T) {
	ctx := t.Context()
	store := memory.New()
	registry := modelprovider.NewRegistry()
	registry.Register("pi-backed-saas", fakeModelProvider{})
	service := application.New(application.DependenciesFrom(store), nil, application.WithModelProviderRegistry(registry))
	task := work.WorkTask{ID: "task-1", TenantID: "tenant-1", ProjectID: "project-1", SOPID: "sop-1", SOPVersion: 1, SOPDigest: "sha256:" + strings.Repeat("a", 64), Title: "公众号文章", ContentType: "wechat_article", InputRefs: []string{"source:1"}, RequestedOutput: map[string]any{}, Priority: "normal", RiskProfile: "low", Status: work.TaskStatusRunning, CreatedBy: "user-1", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := store.CreateWorkTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	actor := application.Actor{UserID: "user-1", TenantID: "tenant-1", Role: "editor", Type: "user"}
	result, err := service.Delivery.GenerateModelCandidate(ctx, actor, task.ID, application.GenerateModelCandidateInput{ProviderID: "pi-backed-saas", Messages: []modelprovider.Message{{Role: "user", Content: "生成公众号候选"}}, ResponseSchema: map[string]any{"type": "object"}, ContentType: "wechat_article", SchemaVersion: "contentcloud.article/1.0", RequestID: "request-1"}, "audit-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Revision.Status != reviewdomain.TaskRevisionDraft || result.Revision.SubmittedAt != nil {
		t.Fatalf("model output bypassed candidate boundary: %#v", result.Revision)
	}
	if result.Receipt.ProviderID != "pi-backed-saas" || result.Receipt.TotalTokens != 20 || !strings.HasPrefix(result.Receipt.RequestDigest, "sha256:") {
		t.Fatalf("generation receipt is incomplete: %#v", result.Receipt)
	}
	revisions, _ := store.TaskRevisions(ctx, actor.TenantID, task.ID)
	receipts, _ := store.ModelGenerationReceipts(ctx, actor.TenantID, task.ID)
	if len(revisions) != 1 || len(receipts) != 1 || receipts[0].TaskRevisionID != revisions[0].ID {
		t.Fatalf("candidate and receipt were not persisted together: revisions=%#v receipts=%#v", revisions, receipts)
	}
}
