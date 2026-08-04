package app_test

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"testing"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestKnowledgeObjectRequiresDecisionAndPreservesHistory(t *testing.T) {
	ctx := context.Background()
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(ctx, "knowledge-governance@example.com", "long-enough-password", "Reviewer", "Knowledge Tenant")
	must(t, err)
	actor, _, err := service.SessionActor(ctx, session.ID)
	must(t, err)
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	must(t, err)
	ref := createAcceptedEvidence(t, ctx, service, actor, project.ID, "净重 10 克", nil)
	spans, err := service.Evidence(ctx, actor, ref.SourceRevisionID)
	must(t, err)

	object, err := service.CreateKnowledgeObject(ctx, actor, app.CreateKnowledgeObjectInput{
		ProjectID: project.ID, ID: "fact:weight", ObjectType: "FactAssertion", Layer: "product", Title: "净重", Statement: "净重 10 克", EvidenceRefs: []string{spans[0].ID},
	}, "")
	must(t, err)
	if object.Version != 1 || object.Status != "candidate" {
		t.Fatalf("unexpected candidate: %#v", object)
	}
	candidates, err := service.KnowledgeObjects(ctx, actor, project.ID)
	must(t, err)
	if len(candidates) != 1 || !reflect.DeepEqual(candidates[0].AllowedActions, []string{"approve", "reject"}) {
		t.Fatalf("candidate review actions are incomplete: %#v", candidates)
	}
	approved, decision, err := service.ReviewKnowledgeObject(ctx, actor, object.ID, app.ReviewKnowledgeObjectInput{ExpectedVersion: object.Version, ExpectedDigest: object.Digest, Decision: "approve", Reason: "已核对来源"}, "")
	must(t, err)
	if approved.Version != 2 || approved.Status != "verified" || decision.ResultVersion != 2 {
		t.Fatalf("unexpected decision result: object=%#v decision=%#v", approved, decision)
	}
	if _, _, err := service.ReviewKnowledgeObject(ctx, actor, object.ID, app.ReviewKnowledgeObjectInput{ExpectedVersion: object.Version, ExpectedDigest: object.Digest, Decision: "approve", Reason: "重放"}, ""); err == nil {
		t.Fatal("stale object decision must be rejected")
	}
	objects, err := service.KnowledgeObjects(ctx, actor, project.ID)
	must(t, err)
	if len(objects) != 2 {
		t.Fatalf("knowledge history must remain immutable: %#v", objects)
	}
	for _, item := range objects {
		if len(item.AllowedActions) != 0 {
			t.Fatalf("governed and historical versions must be read-only: %#v", objects)
		}
	}
}

func createAcceptedEvidence(t *testing.T, ctx context.Context, service *app.Service, actor app.Actor, projectID, quote string, confidence *float64) domain.EvidenceRef {
	t.Helper()
	revision, err := service.UploadSource(ctx, actor, projectID, "Evidence", "brand_manual", "evidence-"+domain.NewID()+".txt", "text/plain", []byte(quote), "")
	must(t, err)
	worker := actor
	worker.Type = "worker"
	_, err = service.CompleteSource(ctx, worker, revision.ID, app.CompleteSourceInput{DetectedMIME: "text/plain", Status: "ready", ParserVersion: "test", Evidence: []app.CreateEvidenceInput{{LocatorKind: "paragraph", Locator: map[string]any{"paragraph": 1}, QuoteText: quote, OCRConfidence: confidence}}}, "")
	must(t, err)
	return domain.EvidenceRef{SourceRevisionID: revision.ID, LocatorKind: "paragraph", Locator: `{"paragraph":1}`, Quote: quote}
}

func assertDomainCode(t *testing.T, err error, code string) {
	t.Helper()
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != code {
		t.Fatalf("expected domain error %s, got %v", code, err)
	}
}

func TestKnowledgeObjectEvidenceMustBeAcceptedAndProjectScoped(t *testing.T) {
	ctx := context.Background()
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(ctx, "knowledge-evidence@example.com", "long-enough-password", "Reviewer", "Evidence Tenant")
	must(t, err)
	actor, _, err := service.SessionActor(ctx, session.ID)
	must(t, err)
	first, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "First", ProductName: "Product"}, "")
	must(t, err)
	second, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "Second", ProductName: "Product"}, "")
	must(t, err)
	ref := createAcceptedEvidence(t, ctx, service, actor, first.ID, "第一项目证据", nil)
	spans, err := service.Evidence(ctx, actor, ref.SourceRevisionID)
	must(t, err)
	_, err = service.CreateKnowledgeObject(ctx, actor, app.CreateKnowledgeObjectInput{ProjectID: second.ID, ID: "fact:cross-project", ObjectType: "FactAssertion", Layer: "product", EvidenceRefs: []string{spans[0].ID}}, "")
	assertDomainCode(t, err, "KNOWLEDGE_EVIDENCE_PROJECT_MISMATCH")
	pendingRevision, err := service.UploadSource(ctx, actor, first.ID, "低置信度来源", "brand_manual", "pending.txt", "text/plain", []byte("待复核证据"), "")
	must(t, err)
	confidence := 0.5
	worker := actor
	worker.Type = "worker"
	_, err = service.CompleteSource(ctx, worker, pendingRevision.ID, app.CompleteSourceInput{DetectedMIME: "text/plain", Status: "ready", ParserVersion: "test", Evidence: []app.CreateEvidenceInput{{LocatorKind: "paragraph", Locator: map[string]any{"paragraph": 1}, QuoteText: "待复核证据", OCRConfidence: &confidence}}}, "")
	must(t, err)
	pendingSpans, err := service.Evidence(ctx, actor, pendingRevision.ID)
	must(t, err)
	pendingObject, err := service.CreateKnowledgeObject(ctx, actor, app.CreateKnowledgeObjectInput{ProjectID: first.ID, ID: "fact:pending", ObjectType: "FactAssertion", Layer: "product", EvidenceRefs: []string{pendingSpans[0].ID}}, "")
	must(t, err)
	_, _, err = service.ReviewKnowledgeObject(ctx, actor, pendingObject.ID, app.ReviewKnowledgeObjectInput{ExpectedVersion: pendingObject.Version, ExpectedDigest: pendingObject.Digest, Decision: "approve", Reason: "尝试批准未验收证据"}, "")
	assertDomainCode(t, err, "KNOWLEDGE_EVIDENCE_NOT_ACCEPTED")
	_, err = service.CreateKnowledgeObject(ctx, actor, app.CreateKnowledgeObjectInput{ProjectID: first.ID, ID: "fact:missing", ObjectType: "FactAssertion", Layer: "product", EvidenceRefs: []string{"missing:evidence"}}, "")
	assertDomainCode(t, err, "KNOWLEDGE_EVIDENCE_NOT_FOUND")
}
