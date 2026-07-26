package app_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestKnowledgeExtractionRunsLocallyAndImportsGroundedCandidates(t *testing.T) {
	ctx := context.Background()
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(ctx, "extract@example.com", "long-enough-password", "Owner", "Extract Tenant")
	must(t, err)
	actor, _, err := service.SessionActor(ctx, session.ID)
	must(t, err)
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "Brand", ProductName: "Incense", Channel: "douyin"}, "")
	must(t, err)
	ref := createAcceptedEvidence(t, ctx, service, actor, project.ID, "每盒净含量为 10 克。", nil)
	connect, err := service.CreateConnectSession(ctx, actor, project.ID, "")
	must(t, err)
	connected, err := service.ConnectDevice(ctx, app.ConnectDeviceInput{ConnectKey: connect.PlaintextConnectKey, Hostname: "local", Platform: "darwin", Arch: "arm64", Version: "test", Capabilities: capabilities()})
	must(t, err)
	deviceActor, device, err := service.DeviceActor(ctx, connected.DeviceToken)
	must(t, err)

	run, err := service.CreateKnowledgeExtractionRun(ctx, actor, app.CreateKnowledgeExtractionRunInput{ProjectID: project.ID, SourceRevisionIDs: []string{ref.SourceRevisionID}, IdempotencyKey: "extract-1", OutputCount: 5}, "")
	must(t, err)
	if _, err := service.Poll(ctx, deviceActor, device, capabilities()[1:]); err == nil {
		t.Fatal("script-only capability must not lease a knowledge extraction run")
	}
	lease, err := service.Poll(ctx, deviceActor, device, capabilities())
	must(t, err)
	if lease.Run.ID != run.ID || len(lease.Contract.Sources) != 1 || len(lease.Contract.Sources[0].Evidence) != 1 {
		t.Fatalf("unexpected knowledge contract %#v", lease.Contract)
	}
	value := 10.0
	pkg := domain.KnowledgeExtractionPackage{SchemaVersion: "1.0", Candidates: []domain.KnowledgeCandidate{{
		Kind: "fact", Title: "净含量", Statement: "每盒净含量为 10 克。", Subject: "Incense", Predicate: "净含量",
		Value: domain.TypedValue{Type: "number", Number: &value, Unit: "g"}, Scope: domain.KnowledgeScope{Regions: []string{}, Channels: []string{}, Audiences: []string{}, ProductVariants: []string{}},
		RiskLevel: "low", AllowedChannels: []string{}, Evidence: []domain.EvidenceRef{ref}, ForbiddenExtensions: []string{}, DependsOnFactIDs: []string{},
	}}, Warnings: []string{}}
	body, _ := json.Marshal(pkg)
	reported, err := service.ReportTask(ctx, deviceActor, device, run.ID, lease.Attempt.ID, lease.RunToken, body, "")
	must(t, err)
	result, ok := reported.(domain.KnowledgeExtractionResult)
	if !ok || len(result.Items) != 1 || result.Items[0].OriginRunID != run.ID || result.Items[0].Status != "needs_review" {
		t.Fatalf("unexpected extraction result %#v", reported)
	}
	replayed, err := service.ReportTask(ctx, deviceActor, device, run.ID, lease.Attempt.ID, lease.RunToken, body, "")
	must(t, err)
	if len(replayed.(domain.KnowledgeExtractionResult).Items) != 1 {
		t.Fatal("idempotent report duplicated knowledge candidates")
	}
}

func TestKnowledgeExtractionRejectsEvidenceOutsideFrozenContract(t *testing.T) {
	ctx := context.Background()
	service := app.New(memory.New(), slog.Default())
	session, _ := service.Register(ctx, "extract-invalid@example.com", "long-enough-password", "Owner", "Extract Invalid Tenant")
	actor, _, _ := service.SessionActor(ctx, session.ID)
	project, _ := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	ref := createAcceptedEvidence(t, ctx, service, actor, project.ID, "可信原文", nil)
	connect, _ := service.CreateConnectSession(ctx, actor, project.ID, "")
	connected, _ := service.ConnectDevice(ctx, app.ConnectDeviceInput{ConnectKey: connect.PlaintextConnectKey, Hostname: "local", Platform: "darwin", Arch: "arm64", Version: "test", Capabilities: capabilities()})
	deviceActor, device, _ := service.DeviceActor(ctx, connected.DeviceToken)
	run, err := service.CreateKnowledgeExtractionRun(ctx, actor, app.CreateKnowledgeExtractionRunInput{ProjectID: project.ID, SourceRevisionIDs: []string{ref.SourceRevisionID}, IdempotencyKey: "extract-invalid", OutputCount: 1}, "")
	must(t, err)
	lease, err := service.Poll(ctx, deviceActor, device, capabilities())
	must(t, err)
	ref.Locator = `{"paragraph":999}`
	pkg := domain.KnowledgeExtractionPackage{SchemaVersion: "1.0", Candidates: []domain.KnowledgeCandidate{{Kind: "fact", Title: "Fact", Statement: "可信原文", Subject: "Product", Predicate: "fact", Value: domain.TypedValue{Type: "text", Text: "可信原文"}, Scope: domain.KnowledgeScope{}, RiskLevel: "low", Evidence: []domain.EvidenceRef{ref}}}, Warnings: []string{}}
	body, _ := json.Marshal(pkg)
	if _, err := service.ReportTask(ctx, deviceActor, device, run.ID, lease.Attempt.ID, lease.RunToken, body, ""); err == nil {
		t.Fatal("fabricated locator must reject the complete extraction report")
	}
	items, err := service.Knowledge(ctx, actor, project.ID)
	must(t, err)
	if len(items) != 0 {
		t.Fatalf("rejected package must not partially import candidates: %#v", items)
	}
}
