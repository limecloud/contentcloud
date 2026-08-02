package app_test

import (
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestConversationImportRequestAndBundleAreScopedAndIdempotent(t *testing.T) {
	ctx := t.Context()
	service := app.New(memory.New(), nil)
	session, err := service.Register(ctx, "conversation-owner@example.com", "long-enough-password", "流程负责人", "对话租户")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "品牌", ProductName: "产品"}, "")
	if err != nil {
		t.Fatal(err)
	}
	task, err := service.CreateWorkTask(ctx, actor, app.CreateWorkTaskInput{ProjectID: project.ID, Title: "整理本地资料", ContentType: domain.ContentTypeVideoScript, InputRefs: []string{"brief:1"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	input := app.CreateConversationImportInput{ClientID: "codex", NodeID: "node-1", Purpose: "task_handoff", RequestedScope: domain.ConversationScopeSummary, AttachAs: domain.ConversationAttachTaskInput, IdempotencyKey: "conversation-import-1"}
	created, err := service.CreateConversationImport(ctx, actor, task.Task.ID, input, "request-1")
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != domain.ConversationImportAwaitingConfirmation || created.Bundle != nil || created.AdapterID != "codex@0.1.0" {
		t.Fatalf("unexpected import request: %#v", created)
	}
	replayed, err := service.CreateConversationImport(ctx, actor, task.Task.ID, input, "request-2")
	if err != nil || replayed.ID != created.ID {
		t.Fatalf("idempotent create did not return original request: value=%#v err=%v", replayed, err)
	}
	content := []domain.ConversationContent{{Kind: "summary", Text: "资料核对完成，缺一条权利证明。"}}
	digest, err := domain.CanonicalHash(content)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	bundle := domain.ConversationBundle{SchemaVersion: domain.ConversationBundleSchema, BundleID: "bundle-1", ImportID: created.ID, Client: domain.ConversationClient{ID: "codex", ClientVersion: "1.0.0", AdapterVersion: "0.1.0", NodeID: "node-1"}, Source: domain.ConversationSource{Format: "codex.events-jsonl/v1", SessionRef: "hmac:opaque-session-reference"}, Purpose: "task_handoff", Scope: domain.ConversationScope{Mode: domain.ConversationScopeSummary}, Target: domain.ConversationTarget{TaskID: task.Task.ID}, Content: content, Redaction: domain.ConversationRedaction{Applied: true, PolicyDigest: "sha256:" + strings.Repeat("a", 64), RemovedTypes: []string{"local_path"}}, Consent: domain.ConversationConsent{ConfirmedAt: now}, ContentDigest: "sha256:" + digest, ExportedAt: now.Add(time.Second)}
	uploaded, err := service.SubmitConversationBundle(ctx, actor, created.ID, bundle, "request-3")
	if err != nil {
		t.Fatal(err)
	}
	if uploaded.Status != domain.ConversationImportUploaded || uploaded.Bundle == nil || uploaded.Bundle.BundleID != "bundle-1" {
		t.Fatalf("unexpected uploaded import: %#v", uploaded)
	}
	imports, err := service.TaskConversationImports(ctx, actor, task.Task.ID)
	if err != nil || len(imports) != 1 || imports[0].Bundle == nil {
		t.Fatalf("task import list missing bundle: values=%#v err=%v", imports, err)
	}
	if _, err := service.CancelConversationImport(ctx, actor, created.ID, "request-4"); err == nil {
		t.Fatal("uploaded import should not be cancellable")
	}
	if revisions, err := service.WorkTaskRevisions(ctx, actor, task.Task.ID); err != nil || len(revisions) != 0 {
		t.Fatalf("conversation import must not create revisions: %#v err=%v", revisions, err)
	}
}
