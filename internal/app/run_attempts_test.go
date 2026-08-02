package app_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
	"github.com/limecloud/contentcloud/internal/testsupport"
)

func TestRunAttemptLeaseHeartbeatExpiryAndStaleReport(t *testing.T) {
	ctx := context.Background()
	service, store, actor, deviceActor, device, run := setupKnowledgeRun(t, ctx, "attempt-lifecycle@example.com")
	caps := capabilities()
	caps[0].Digest = "sha256:test-knowledge-skill"

	first, err := service.Poll(ctx, deviceActor, device, caps)
	must(t, err)
	if first.Attempt.RunID != run.ID || first.Run.ActiveAttemptID != first.Attempt.ID {
		t.Fatalf("poll did not bind the active attempt: %#v", first)
	}
	if first.Attempt.CapabilityDigest != caps[0].Digest || first.Attempt.InputSchema != caps[0].InputSchema || first.Attempt.OutputSchema != caps[0].OutputSchema {
		t.Fatalf("attempt did not freeze capability identity: %#v", first.Attempt)
	}

	heartbeatRun, err := service.HeartbeatRun(ctx, deviceActor, device, run.ID, first.Attempt.ID, first.RunToken, domain.RunHeartbeat{Sequence: 1, Phase: "executing", Label: "working"}, "")
	must(t, err)
	if heartbeatRun.State != "running" || heartbeatRun.HeartbeatSequence != 1 {
		t.Fatalf("heartbeat did not advance run: %#v", heartbeatRun)
	}
	heartbeatAttempt, err := store.RunAttempt(ctx, actor.TenantID, first.Attempt.ID)
	must(t, err)
	if heartbeatAttempt.State != "running" || heartbeatAttempt.HeartbeatAt == nil || heartbeatAttempt.StartedAt == nil {
		t.Fatalf("heartbeat did not advance attempt: %#v", heartbeatAttempt)
	}
	progress, err := service.RunProgress(ctx, actor, run.ID, 0)
	if err != nil || len(progress) != 1 || progress[0].Phase != "executing" || progress[0].Cursor == 0 {
		t.Fatalf("heartbeat progress not persisted: events=%#v err=%v", progress, err)
	}
	if incremental, err := service.RunProgress(ctx, actor, run.ID, progress[0].Cursor); err != nil || len(incremental) != 0 {
		t.Fatalf("progress cursor was not respected: events=%#v err=%v", incremental, err)
	}

	must(t, store.ExpireRunAttempts(ctx, actor.TenantID, heartbeatAttempt.LeaseExpiresAt.Add(time.Second)))
	second, err := service.Poll(ctx, deviceActor, device, caps)
	must(t, err)
	if second.Attempt.ID == first.Attempt.ID || second.Run.AttemptCount != 2 {
		t.Fatalf("expired lease did not create a second attempt: %#v", second)
	}
	firstStored, err := store.RunAttempt(ctx, actor.TenantID, first.Attempt.ID)
	must(t, err)
	if firstStored.State != "expired" || firstStored.FailureClass != "lease_expired" {
		t.Fatalf("first attempt was not preserved as expired: %#v", firstStored)
	}

	_, err = service.ReportTask(ctx, deviceActor, device, run.ID, first.Attempt.ID, first.RunToken, json.RawMessage(`{}`), "")
	assertDomainCode(t, err, "RUN_ATTEMPT_STALE")
	attempts, err := service.RunAttempts(ctx, actor, run.ID)
	must(t, err)
	if len(attempts) != 2 || attempts[0].TokenHash != "" || attempts[1].TokenHash != "" {
		t.Fatalf("attempt history must be complete without token hashes: %#v", attempts)
	}
}

func TestRunAttemptCancellationWinsBeforeReport(t *testing.T) {
	ctx := context.Background()
	service, _, actor, deviceActor, device, run := setupKnowledgeRun(t, ctx, "attempt-cancel@example.com")
	lease, err := service.Poll(ctx, deviceActor, device, capabilities())
	must(t, err)
	_, err = service.CancelRun(ctx, actor, run.ID, "")
	must(t, err)

	_, err = service.ReportTask(ctx, deviceActor, device, run.ID, lease.Attempt.ID, lease.RunToken, json.RawMessage(`{}`), "")
	assertDomainCode(t, err, "RUN_CANCELED")
	stored, err := service.Run(ctx, actor, run.ID)
	must(t, err)
	if stored.State != "canceled" || stored.ActiveAttemptID != "" {
		t.Fatalf("cancel request did not win over report: %#v", stored)
	}
	attempts, err := service.RunAttempts(ctx, actor, run.ID)
	must(t, err)
	if len(attempts) != 1 || attempts[0].State != "canceled" {
		t.Fatalf("attempt cancellation was not preserved: %#v", attempts)
	}
	items, err := service.KnowledgeObjects(ctx, actor, run.ProjectID)
	must(t, err)
	if len(items) != 0 {
		t.Fatalf("canceled report wrote knowledge: %#v", items)
	}
}

func TestInvalidOutputExhaustsThreeRunAttempts(t *testing.T) {
	ctx := context.Background()
	service, _, actor, deviceActor, device, run := setupKnowledgeRun(t, ctx, "attempt-retry@example.com")
	for attemptNumber := 1; attemptNumber <= 3; attemptNumber++ {
		lease, err := service.Poll(ctx, deviceActor, device, capabilities())
		must(t, err)
		_, err = service.ReportTask(ctx, deviceActor, device, run.ID, lease.Attempt.ID, lease.RunToken, json.RawMessage(`{"broken":`), "")
		assertDomainCode(t, err, "CAPABILITY_OUTPUT_INVALID")
		stored, err := service.Run(ctx, actor, run.ID)
		must(t, err)
		wantState := "queued"
		if attemptNumber == 3 {
			wantState = "failed"
		}
		if stored.State != wantState || stored.AttemptCount != attemptNumber {
			t.Fatalf("attempt %d: got state=%s count=%d", attemptNumber, stored.State, stored.AttemptCount)
		}
	}
	attempts, err := service.RunAttempts(ctx, actor, run.ID)
	must(t, err)
	if len(attempts) != 3 {
		t.Fatalf("expected three immutable attempts, got %d", len(attempts))
	}
	for _, attempt := range attempts {
		if attempt.State != "failed" || attempt.FailureClass != "knowledge_json" {
			t.Fatalf("invalid output attempt not classified: %#v", attempt)
		}
	}
}

func TestCapabilityContractMismatchDoesNotLease(t *testing.T) {
	ctx := context.Background()
	service, _, actor, deviceActor, device, run := setupKnowledgeRun(t, ctx, "attempt-capability@example.com")
	wrong := capabilities()[0]
	wrong.Version = "9.0.0"
	if _, err := service.Poll(ctx, deviceActor, device, []domain.Capability{wrong}); err == nil {
		t.Fatal("mismatched capability version must not lease the run")
	}
	stored, err := service.Run(ctx, actor, run.ID)
	must(t, err)
	if stored.State != "queued" || stored.AttemptCount != 0 || stored.ActiveAttemptID != "" {
		t.Fatalf("capability mismatch mutated the run: %#v", stored)
	}
	attempts, err := service.RunAttempts(ctx, actor, run.ID)
	must(t, err)
	if len(attempts) != 0 {
		t.Fatalf("capability mismatch created attempts: %#v", attempts)
	}
}

func setupKnowledgeRun(t *testing.T, ctx context.Context, email string) (*app.Service, *memory.Store, app.Actor, app.Actor, domain.Device, domain.TaskRun) {
	t.Helper()
	store := memory.New()
	service := app.New(store, slog.Default())
	session, err := service.Register(ctx, email, "long-enough-password", "Owner", "Attempt Tenant")
	must(t, err)
	actor, _, err := service.SessionActor(ctx, session.ID)
	must(t, err)
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	must(t, err)
	ref := createAcceptedEvidence(t, ctx, service, actor, project.ID, "可信原文", nil)
	connect, err := service.CreateConnectSession(ctx, actor, project.ID, "")
	must(t, err)
	connected, err := testsupport.ConnectBootstrap(ctx, service, actor, connect, app.ConnectDeviceInput{Hostname: "local", Platform: "darwin", Arch: "arm64", Version: "test", Capabilities: capabilities()})
	must(t, err)
	deviceActor, device, err := service.DeviceActor(ctx, connected.DeviceToken)
	must(t, err)
	run, err := service.CreateKnowledgeExtractionRun(ctx, actor, app.CreateKnowledgeExtractionRunInput{ProjectID: project.ID, SourceRevisionIDs: []string{ref.SourceRevisionID}, IdempotencyKey: email, OutputCount: 1}, "")
	must(t, err)
	return service, store, actor, deviceActor, device, run
}
