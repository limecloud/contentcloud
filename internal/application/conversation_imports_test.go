package application_test

import (
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/application"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	"github.com/limecloud/contentcloud/internal/platform/stablehash"
	"github.com/limecloud/contentcloud/internal/work"
)

func TestConversationImportRequestAndBundleAreScopedAndIdempotent(t *testing.T) {
	ctx := t.Context()
	service := application.New(application.DependenciesFrom(memory.New()), nil)
	session, err := service.Identity.Register(ctx, "conversation-owner@example.com", "long-enough-password", "流程负责人", "对话租户")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.Workspace.CreateProject(ctx, actor, application.CreateProjectInput{BrandName: "品牌", ProductName: "产品", ContentType: identitydomain.ContentTypeVideoScript}, "")
	if err != nil {
		t.Fatal(err)
	}
	task, err := service.Work.CreateWorkTask(ctx, actor, application.CreateWorkTaskInput{ProjectID: project.ID, Title: "整理本地资料", ContentType: identitydomain.ContentTypeVideoScript, InputRefs: []string{"brief:1"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	input := application.CreateConversationImportInput{ClientID: "codex", NodeID: "node-1", Purpose: "task_handoff", RequestedScope: work.ConversationScopeSummary, AttachAs: work.ConversationAttachTaskInput, IdempotencyKey: "conversation-import-1"}
	created, err := service.Work.CreateConversationImport(ctx, actor, task.Task.ID, input, "request-1")
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != work.ConversationImportAwaitingConfirmation || created.Bundle != nil || created.AdapterID != "codex@0.1.0" {
		t.Fatalf("unexpected import request: %#v", created)
	}
	replayed, err := service.Work.CreateConversationImport(ctx, actor, task.Task.ID, input, "request-2")
	if err != nil || replayed.ID != created.ID {
		t.Fatalf("idempotent create did not return original request: value=%#v err=%v", replayed, err)
	}
	content := []work.ConversationContent{{Kind: "summary", Text: "资料核对完成，缺一条权利证明。"}}
	digest, err := stablehash.Sum(content)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	bundle := work.ConversationBundle{SchemaVersion: work.ConversationBundleSchema, BundleID: "bundle-1", ImportID: created.ID, Client: work.ConversationClient{ID: "codex", ClientVersion: "1.0.0", AdapterVersion: "0.1.0", NodeID: "node-1"}, Source: work.ConversationSource{Format: "codex.events-jsonl/v1", SessionRef: "hmac:opaque-session-reference"}, Purpose: "task_handoff", Scope: work.ConversationScope{Mode: work.ConversationScopeSummary}, Target: work.ConversationTarget{TaskID: task.Task.ID}, Content: content, Redaction: work.ConversationRedaction{Applied: true, PolicyDigest: "sha256:" + strings.Repeat("a", 64), RemovedTypes: []string{"local_path"}}, Consent: work.ConversationConsent{ConfirmedAt: now}, ContentDigest: "sha256:" + digest, ExportedAt: now.Add(time.Second)}
	uploaded, err := service.Work.SubmitConversationBundle(ctx, actor, created.ID, bundle, "request-3")
	if err != nil {
		t.Fatal(err)
	}
	if uploaded.Status != work.ConversationImportUploaded || uploaded.Bundle == nil || uploaded.Bundle.BundleID != "bundle-1" {
		t.Fatalf("unexpected uploaded import: %#v", uploaded)
	}
	imports, err := service.Work.TaskConversationImports(ctx, actor, task.Task.ID)
	if err != nil || len(imports) != 1 || imports[0].Bundle == nil {
		t.Fatalf("task import list missing bundle: values=%#v err=%v", imports, err)
	}
	if _, err := service.Work.CancelConversationImport(ctx, actor, created.ID, "request-4"); err == nil {
		t.Fatal("uploaded import should not be cancellable")
	}
	if revisions, err := service.Work.WorkTaskRevisions(ctx, actor, task.Task.ID); err != nil || len(revisions) != 0 {
		t.Fatalf("conversation import must not create revisions: %#v err=%v", revisions, err)
	}
}
