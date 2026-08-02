package app_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestKnowledgePackPublishAndQuery(t *testing.T) {
	ctx := context.Background()
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(ctx, "knowledge@example.com", "long-enough-password", "Owner", "Knowledge")
	must(t, err)
	actor, _, err := service.SessionActor(ctx, session.ID)
	must(t, err)
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "Brand", ProductName: "Product", Channel: "short_video"}, "")
	must(t, err)
	weightRef := createAcceptedEvidence(t, ctx, service, actor, project.ID, "净重 50g", nil)
	weightSpans, err := service.Evidence(ctx, actor, weightRef.SourceRevisionID)
	must(t, err)
	object, err := service.CreateKnowledgeObject(ctx, actor, app.CreateKnowledgeObjectInput{ProjectID: project.ID, ID: "fact:weight", ObjectType: "FactAssertion", Layer: "product", Title: "净重", Statement: "净重 50g", EvidenceRefs: []string{weightSpans[0].ID}}, "")
	must(t, err)
	if object.Payload == nil {
		t.Fatal("knowledge object payload must normalize to an object")
	}
	object, decision, err := service.ReviewKnowledgeObject(ctx, actor, object.ID, app.ReviewKnowledgeObjectInput{ExpectedVersion: object.Version, ExpectedDigest: object.Digest, Decision: "approve", Reason: "规格证据已复核"}, "")
	must(t, err)
	if object.Status != "verified" || decision.ResultVersion != object.Version {
		t.Fatalf("unexpected knowledge decision: %#v %#v", object, decision)
	}
	priceRef := createAcceptedEvidence(t, ctx, service, actor, project.ID, "价格主张", nil)
	priceSpans, err := service.Evidence(ctx, actor, priceRef.SourceRevisionID)
	must(t, err)
	_, err = service.CreateKnowledgeObject(ctx, actor, app.CreateKnowledgeObjectInput{ProjectID: project.ID, ID: "claim:price", ObjectType: "Claim", Layer: "expression", Status: "candidate", Title: "价格主张", EvidenceRefs: []string{priceSpans[0].ID}}, "")
	must(t, err)
	pack, err := service.CreateKnowledgePack(ctx, actor, app.CreateKnowledgePackInput{ProjectID: project.ID, ID: "pack:product", Name: "产品知识包", Purpose: "content", ObjectRefs: []domain.KnowledgePackObjectRef{{ObjectID: object.ID, Version: object.Version}, {ObjectID: "claim:price", Version: 1}}}, "")
	must(t, err)
	published, snapshot, err := service.PublishKnowledgePack(ctx, actor, pack.ID, "")
	must(t, err)
	if published.Status != "published" || snapshot.PackID != pack.ID || len(snapshot.Objects) != 2 {
		t.Fatalf("unexpected publish result: %#v %#v", published, snapshot)
	}
	at := time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC)
	result, err := service.QueryKnowledge(ctx, actor, app.QueryKnowledgeInput{ProjectID: project.ID, SnapshotID: snapshot.ID, Channel: "short_video", At: at})
	must(t, err)
	if len(result.Eligible) != 1 || result.Eligible[0].ObjectID != object.ID || len(result.Blocked) != 1 {
		t.Fatalf("unexpected knowledge query result: %#v", result)
	}
	byPack, err := service.QueryKnowledge(ctx, actor, app.QueryKnowledgeInput{PackID: pack.ID, Channel: "short_video", At: at})
	must(t, err)
	if byPack.QueryDigest != result.QueryDigest {
		t.Fatalf("snapshot and pack query should resolve to same deterministic result: %#v %#v", result, byPack)
	}
}

func TestKnowledgePackRejectsCrossProjectObject(t *testing.T) {
	ctx := context.Background()
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(ctx, "knowledge-scope@example.com", "long-enough-password", "Owner", "Knowledge Scope")
	must(t, err)
	actor, _, err := service.SessionActor(ctx, session.ID)
	must(t, err)
	first, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "A", ProductName: "A"}, "")
	must(t, err)
	second, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "B", ProductName: "B"}, "")
	must(t, err)
	ref := createAcceptedEvidence(t, ctx, service, actor, first.ID, "跨项目证据", nil)
	spans, err := service.Evidence(ctx, actor, ref.SourceRevisionID)
	must(t, err)
	object, err := service.CreateKnowledgeObject(ctx, actor, app.CreateKnowledgeObjectInput{ProjectID: first.ID, ID: "fact:scope", ObjectType: "FactAssertion", Layer: "product", EvidenceRefs: []string{spans[0].ID}}, "")
	must(t, err)
	pack, err := service.CreateKnowledgePack(ctx, actor, app.CreateKnowledgePackInput{ProjectID: second.ID, ID: "pack:scope", Name: "越界包", Purpose: "test", ObjectRefs: []domain.KnowledgePackObjectRef{{ObjectID: object.ID, Version: 1}}}, "")
	must(t, err)
	if _, _, err := service.PublishKnowledgePack(ctx, actor, pack.ID, ""); err == nil {
		t.Fatal("cross-project object must not be publishable")
	}
}
