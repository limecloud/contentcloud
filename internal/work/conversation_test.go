package work

import (
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/stablehash"
)

func TestConversationBundleValidationRequiresExplicitRedactionAndDigest(t *testing.T) {
	now := time.Date(2026, 8, 2, 9, 0, 0, 0, time.UTC)
	value := ConversationImport{
		ID: "import-1", TenantID: "tenant-1", ProjectID: "project-1", TaskID: "task-1",
		ClientID: "codex", AdapterVersion: "0.1.0", AdapterID: "codex@0.1.0",
		Purpose: "task_handoff", RequestedScope: ConversationScopeSummary, AttachAs: ConversationAttachTaskInput,
		RetentionDays: 30, Status: ConversationImportAwaitingConfirmation, ExpiresAt: now.Add(time.Hour), CreatedBy: "user-1", CreatedAt: now,
	}
	content := []ConversationContent{{Kind: "summary", Text: "已完成资料核对。"}}
	digest, err := stablehash.Sum(content)
	if err != nil {
		t.Fatal(err)
	}
	bundle := ConversationBundle{
		SchemaVersion: ConversationBundleSchema, BundleID: "bundle-1", ImportID: value.ID,
		Client:  ConversationClient{ID: "codex", ClientVersion: "1.0.0", AdapterVersion: "0.1.0"},
		Source:  ConversationSource{Format: "codex.events-jsonl/v1", SessionRef: "hmac:opaque-session-reference"},
		Purpose: value.Purpose, Scope: ConversationScope{Mode: ConversationScopeSummary}, Target: ConversationTarget{TaskID: value.TaskID}, Content: content,
		Redaction: ConversationRedaction{Applied: true, PolicyDigest: "sha256:" + strings.Repeat("a", 64)},
		Consent:   ConversationConsent{ConfirmedAt: now}, ContentDigest: "sha256:" + digest, ExportedAt: now.Add(time.Minute),
	}
	if err := bundle.ValidateAgainst(value, now); err != nil {
		t.Fatalf("valid bundle rejected: %v", err)
	}
	bundle.Redaction.Applied = false
	if err := bundle.ValidateAgainst(value, now); err == nil || !strings.Contains(err.Error(), "脱敏") {
		t.Fatalf("bundle without redaction should be rejected: %v", err)
	}
}

func TestConversationBundleRejectsPrivateRuntimeData(t *testing.T) {
	now := time.Now().UTC()
	value := ConversationImport{ID: "import-1", TenantID: "tenant-1", ProjectID: "project-1", TaskID: "task-1", ClientID: "codex", AdapterVersion: "0.1.0", AdapterID: "codex@0.1.0", Purpose: "task_handoff", RequestedScope: ConversationScopeSummary, AttachAs: ConversationAttachTaskInput, RetentionDays: 30, Status: ConversationImportAwaitingConfirmation, ExpiresAt: now.Add(time.Hour), CreatedBy: "user-1", CreatedAt: now}
	content := []ConversationContent{{Kind: "summary", Text: "读取了 /Users/coso/private.txt"}}
	digest, _ := stablehash.Sum(content)
	bundle := ConversationBundle{SchemaVersion: ConversationBundleSchema, BundleID: "bundle-1", ImportID: value.ID, Client: ConversationClient{ID: "codex", ClientVersion: "1.0.0", AdapterVersion: "0.1.0"}, Source: ConversationSource{Format: "codex.events-jsonl/v1", SessionRef: "hmac:opaque-session-reference"}, Purpose: value.Purpose, Scope: ConversationScope{Mode: value.RequestedScope}, Target: ConversationTarget{TaskID: value.TaskID}, Content: content, Redaction: ConversationRedaction{Applied: true, PolicyDigest: "sha256:" + strings.Repeat("b", 64)}, Consent: ConversationConsent{ConfirmedAt: now}, ContentDigest: "sha256:" + digest, ExportedAt: now.Add(time.Minute)}
	if err := bundle.ValidateAgainst(value, now); err == nil || !strings.Contains(err.Error(), "本机路径") {
		t.Fatalf("private runtime data should be rejected: %v", err)
	}
}
