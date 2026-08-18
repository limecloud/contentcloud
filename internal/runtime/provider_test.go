package runtime_test

import (
	"testing"
	"time"

	. "github.com/limecloud/contentcloud/internal/runtime"

	"github.com/limecloud/contentcloud/internal/persistence/memory"
)

func setupProviderRuntime(t *testing.T) (*Service, *memory.Store, StartResult, ExternalEffect) {
	t.Helper()
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	repo := memory.New()
	service := New(repo, func() time.Time { return now })
	started, err := service.Start(t.Context(), testStartInput("provider-task", "provider-job"))
	if err != nil {
		t.Fatal(err)
	}
	effect, err := service.RegisterEffect(t.Context(), ExternalEffect{
		TenantID: "tenant-1", JobRunID: started.Job.ID, NodeRunID: started.Nodes[0].ID,
		Kind: "media.generate", IdempotencyKey: "provider-effect-1", RequestDigest: "sha256:request",
		CostMinor: 120, Currency: "CNY", SafeSummary: map[string]any{"provider_id": "provider-a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	effect, err = service.ReconcileEffect(t.Context(), effect.TenantID, effect.ID, EffectSubmitted, "external-1", "", "", effect.Version)
	if err != nil {
		t.Fatal(err)
	}
	effect, err = service.ReconcileEffect(t.Context(), effect.TenantID, effect.ID, EffectAcknowledged, "external-1", "", "", effect.Version)
	if err != nil {
		t.Fatal(err)
	}
	return service, repo, started, effect
}

func TestProviderCallbackInboxIdempotencyAndTerminalDuplicate(t *testing.T) {
	service, repo, started, effect := setupProviderRuntime(t)
	input := ProviderCallbackInput{
		TenantID: "tenant-1", JobRunID: started.Job.ID, ProviderID: "provider-a", MessageID: "message-1",
		ExternalID: "external-1", ProviderState: "completed", CostMinor: 120, Currency: "CNY",
		SafePayload: map[string]any{"status": "completed"},
	}
	message, terminal, err := service.ReceiveProviderCallback(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if message.State != ProviderInboxApplied || message.EffectID != effect.ID || terminal.State != EffectSucceeded {
		t.Fatalf("provider callback was not atomically applied: message=%#v effect=%#v", message, terminal)
	}
	events, err := repo.JobEvents(t.Context(), "tenant-1", started.Job.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	beforeDuplicate := len(events)
	replayed, replayEffect, err := service.ReceiveProviderCallback(t.Context(), input)
	if err != nil || replayed.ID != message.ID || replayEffect.Version != terminal.Version {
		t.Fatalf("same provider message was not replayed idempotently: message=%#v effect=%#v err=%v", replayed, replayEffect, err)
	}
	events, _ = repo.JobEvents(t.Context(), "tenant-1", started.Job.ID, 0)
	if len(events) != beforeDuplicate {
		t.Fatalf("idempotent provider replay appended a second event: before=%d after=%d", beforeDuplicate, len(events))
	}

	terminalDuplicate := input
	terminalDuplicate.MessageID = "message-2"
	secondMessage, secondEffect, err := service.ReceiveProviderCallback(t.Context(), terminalDuplicate)
	if err != nil || secondMessage.State != ProviderInboxApplied || secondEffect.Version != terminal.Version {
		t.Fatalf("new terminal duplicate callback must be recorded without advancing Effect: message=%#v effect=%#v err=%v", secondMessage, secondEffect, err)
	}
	conflict := input
	conflict.ProviderState = "failed"
	if _, _, err := service.ReceiveProviderCallback(t.Context(), conflict); err == nil {
		t.Fatal("same provider message id with a different digest must conflict")
	}
}

func TestUnknownProviderEffectRequiresExplicitReconciliation(t *testing.T) {
	service, repo, started, effect := setupProviderRuntime(t)
	unknown, err := service.ReconcileEffect(t.Context(), effect.TenantID, effect.ID, EffectUnknown, "external-1", "", "PROVIDER_TIMEOUT", effect.Version)
	if err != nil {
		t.Fatal(err)
	}
	message, observed, err := service.ReceiveProviderCallback(t.Context(), ProviderCallbackInput{
		TenantID: "tenant-1", JobRunID: started.Job.ID, ProviderID: "provider-a", MessageID: "message-unknown",
		ExternalID: "external-1", ProviderState: "completed", CostMinor: 120, Currency: "CNY", SafePayload: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if message.State != ProviderInboxApplied || observed.State != EffectReconciling || observed.Version != unknown.Version+1 {
		t.Fatalf("unknown effect was blindly completed: message=%#v effect=%#v", message, observed)
	}
	reconciliations, err := repo.ProviderReconciliations(t.Context(), "tenant-1", observed.ID)
	if err != nil || len(reconciliations) != 1 || reconciliations[0].Status != ProviderReconPending {
		t.Fatalf("provider reconciliation was not created: %#v err=%v", reconciliations, err)
	}
	resolved, finalEffect, err := service.ResolveProviderReconciliation(t.Context(), "tenant-1", reconciliations[0].ID, EffectSucceeded, "operator-1")
	if err != nil || resolved.Status != ProviderReconMatched || finalEffect.State != EffectSucceeded {
		t.Fatalf("provider reconciliation did not converge explicitly: recon=%#v effect=%#v err=%v", resolved, finalEffect, err)
	}
}

func TestProviderBillsMatchDisputeAndRemainPendingWithoutEffect(t *testing.T) {
	service, repo, started, _ := setupProviderRuntime(t)
	matched, err := service.RecordProviderBill(t.Context(), ProviderBillInput{TenantID: "tenant-1", ProviderID: "provider-a", BillID: "bill-1", ExternalID: "external-1", AmountMinor: 120, Currency: "CNY"})
	if err != nil || matched.Status != ProviderBillMatched || matched.EffectID == "" {
		t.Fatalf("matching bill was not linked: %#v err=%v", matched, err)
	}
	disputed, err := service.RecordProviderBill(t.Context(), ProviderBillInput{TenantID: "tenant-1", ProviderID: "provider-a", BillID: "bill-2", ExternalID: "external-1", AmountMinor: 150, Currency: "CNY"})
	if err != nil || disputed.Status != ProviderBillDisputed {
		t.Fatalf("cost mismatch was not disputed: %#v err=%v", disputed, err)
	}
	unmatched, err := service.RecordProviderBill(t.Context(), ProviderBillInput{TenantID: "tenant-1", JobRunID: started.Job.ID, ProviderID: "provider-a", BillID: "bill-3", ExternalID: "external-missing", AmountMinor: 50, Currency: "CNY"})
	if err != nil || unmatched.Status != ProviderBillUnmatched || unmatched.EffectID != "" {
		t.Fatalf("unmatched bill did not remain pending: %#v err=%v", unmatched, err)
	}
	reconciliations, err := repo.ProviderReconciliations(t.Context(), "tenant-1", "")
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]bool{}
	for _, reconciliation := range reconciliations {
		statuses[reconciliation.Status] = true
	}
	if !statuses[ProviderReconMatched] || !statuses[ProviderReconCostMismatch] || !statuses[ProviderReconPending] {
		t.Fatalf("bill reconciliation facts are incomplete: %#v", reconciliations)
	}
}
