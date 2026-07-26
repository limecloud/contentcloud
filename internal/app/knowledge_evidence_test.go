package app_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestKnowledgeEvidenceTrustBoundary(t *testing.T) {
	ctx := context.Background()
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(ctx, "evidence@example.com", "long-enough-password", "Reviewer", "Evidence Tenant")
	must(t, err)
	actor, _, err := service.SessionActor(ctx, session.ID)
	must(t, err)
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "Brand A", ProductName: "Product A"}, "")
	must(t, err)
	otherProject, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "Brand B", ProductName: "Product B"}, "")
	must(t, err)

	t.Run("rejects fictional revision", func(t *testing.T) {
		_, err := createKnowledgeWithRef(ctx, service, actor, project.ID, domain.EvidenceRef{SourceRevisionID: "missing", LocatorKind: "paragraph", Locator: `{"paragraph":1}`, Quote: "missing"})
		assertDomainCode(t, err, "RESOURCE_NOT_FOUND")
	})

	t.Run("rejects evidence from another project", func(t *testing.T) {
		ref := createAcceptedEvidence(t, ctx, service, actor, otherProject.ID, "other project", nil)
		_, err := createKnowledgeWithRef(ctx, service, actor, project.ID, ref)
		assertDomainCode(t, err, "EVIDENCE_PROJECT_MISMATCH")
	})

	t.Run("rejects source that is not ready", func(t *testing.T) {
		revision, err := service.UploadSource(ctx, actor, project.ID, "Pending", "brand_manual", "pending.txt", "text/plain", []byte("pending source"), "")
		must(t, err)
		_, err = createKnowledgeWithRef(ctx, service, actor, project.ID, domain.EvidenceRef{SourceRevisionID: revision.ID, LocatorKind: "paragraph", Locator: `{"paragraph":1}`, Quote: "pending source"})
		assertDomainCode(t, err, "EVIDENCE_SOURCE_NOT_READY")
	})

	t.Run("rejects modified quote", func(t *testing.T) {
		ref := createAcceptedEvidence(t, ctx, service, actor, project.ID, "source wording", nil)
		ref.Quote = "rewritten wording"
		_, err := createKnowledgeWithRef(ctx, service, actor, project.ID, ref)
		assertDomainCode(t, err, "EVIDENCE_NOT_ACCEPTED")
	})

	t.Run("rejects low confidence OCR span", func(t *testing.T) {
		confidence := 0.4
		ref := createAcceptedEvidence(t, ctx, service, actor, project.ID, "uncertain OCR", &confidence)
		_, err := createKnowledgeWithRef(ctx, service, actor, project.ID, ref)
		assertDomainCode(t, err, "EVIDENCE_NOT_ACCEPTED")
	})

	t.Run("revalidates evidence on approval", func(t *testing.T) {
		ref := createAcceptedEvidence(t, ctx, service, actor, project.ID, "approval evidence", nil)
		knowledge, err := createKnowledgeWithRef(ctx, service, actor, project.ID, ref)
		must(t, err)
		worker := actor
		worker.Type = "worker"
		_, err = service.CompleteSource(ctx, worker, ref.SourceRevisionID, app.CompleteSourceInput{DetectedMIME: "text/plain", Status: "needs_review", ParserVersion: "test/v2"}, "")
		must(t, err)
		_, err = service.ReviewKnowledge(ctx, actor, knowledge.ID, "approve", "")
		assertDomainCode(t, err, "EVIDENCE_SOURCE_NOT_READY")
	})

	t.Run("requires evidence for approved methodology", func(t *testing.T) {
		knowledge, err := service.CreateKnowledge(ctx, actor, app.CreateKnowledgeInput{ProjectID: project.ID, Kind: "methodology", Title: "Method", Statement: "Use the approved local skill"}, "")
		must(t, err)
		_, err = service.ReviewKnowledge(ctx, actor, knowledge.ID, "approve", "")
		assertDomainCode(t, err, "EVIDENCE_REQUIRED")
	})
}

func TestSourceRevisionEvidenceReviewAndImpactPropagation(t *testing.T) {
	ctx := context.Background()
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(ctx, "revision@example.com", "long-enough-password", "Reviewer", "Revision Tenant")
	must(t, err)
	actor, _, err := service.SessionActor(ctx, session.ID)
	must(t, err)
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	must(t, err)

	confidence := 0.4
	first, err := service.UploadSource(ctx, actor, project.ID, "Product manual", "brand_manual", "manual-v1.txt", "text/plain", []byte("original claim"), "")
	must(t, err)
	worker := actor
	worker.Type = "worker"
	_, err = service.CompleteSource(ctx, worker, first.ID, app.CompleteSourceInput{
		DetectedMIME:  "text/plain",
		Status:        "ready",
		ParserVersion: "test/v1",
		Evidence:      []app.CreateEvidenceInput{{LocatorKind: "paragraph", Locator: map[string]any{"paragraph": 1}, QuoteText: "original claim", OCRConfidence: &confidence}},
	}, "")
	must(t, err)
	spans, err := service.Evidence(ctx, actor, first.ID)
	must(t, err)
	if len(spans) != 1 || spans[0].ReviewStatus != "needs_review" {
		t.Fatalf("expected one evidence span needing review, got %#v", spans)
	}
	span, err := service.ReviewEvidence(ctx, actor, spans[0].ID, "accept", "")
	must(t, err)
	if span.ReviewStatus != "accepted" || span.ReviewedAt == nil || span.ReviewedBy != actor.UserID {
		t.Fatalf("human evidence decision was not persisted: %#v", span)
	}

	knowledge, err := service.CreateKnowledge(ctx, actor, app.CreateKnowledgeInput{
		ProjectID: project.ID,
		Kind:      "fact",
		Title:     "Grounded product claim",
		Statement: "Original claim",
		Evidence:  []domain.EvidenceRef{{SourceRevisionID: first.ID, LocatorKind: "paragraph", Locator: `{"paragraph":1}`, Quote: "original claim"}},
	}, "")
	must(t, err)
	knowledge, err = service.ReviewKnowledge(ctx, actor, knowledge.ID, "approve", "")
	must(t, err)
	if knowledge.Status != "approved" {
		t.Fatalf("expected approved knowledge, got %s", knowledge.Status)
	}

	second, err := service.UploadSourceRevision(ctx, actor, first.SourceID, "manual-v2.txt", "text/plain", []byte("updated claim"), "")
	must(t, err)
	if second.SupersedesID != first.ID {
		t.Fatalf("expected revision %s to supersede %s, got %s", second.ID, first.ID, second.SupersedesID)
	}
	revisions, err := service.SourceRevisions(ctx, actor, first.SourceID)
	must(t, err)
	if len(revisions) != 2 || revisions[0].ID != second.ID {
		t.Fatalf("unexpected revision chain: %#v", revisions)
	}
	knowledge, err = service.KnowledgeItem(ctx, actor, knowledge.ID)
	must(t, err)
	if knowledge.Status != "review_required" {
		t.Fatalf("source change must invalidate approved knowledge, got %s", knowledge.Status)
	}
	impact, err := service.SourceImpact(ctx, actor, first.SourceID)
	must(t, err)
	if len(impact) != 1 || impact[0].ObjectID != knowledge.ID || impact[0].CurrentStatus != "review_required" {
		t.Fatalf("unexpected source impact: %#v", impact)
	}

	span, err = service.ReviewEvidence(ctx, actor, span.ID, "reject", "")
	must(t, err)
	if span.ReviewStatus != "rejected" {
		t.Fatalf("expected rejected evidence, got %#v", span)
	}
}

func TestTypedKnowledgeConflictRequiresExplicitDecision(t *testing.T) {
	ctx := context.Background()
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(ctx, "conflict@example.com", "long-enough-password", "Reviewer", "Conflict Tenant")
	must(t, err)
	actor, _, err := service.SessionActor(ctx, session.ID)
	must(t, err)
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "Brand", ProductName: "Incense"}, "")
	must(t, err)
	ref := createAcceptedEvidence(t, ctx, service, actor, project.ID, "净含量以包装标示为准", nil)

	ten := 10.0
	first, err := service.CreateKnowledge(ctx, actor, app.CreateKnowledgeInput{
		ProjectID: project.ID, Kind: "fact", Title: "净含量", Statement: "净含量 10 克", Subject: "产品", Predicate: "净含量",
		Value: domain.TypedValue{Type: "number", Number: &ten, Unit: "g"}, Scope: domain.KnowledgeScope{Channels: []string{"douyin"}}, Evidence: []domain.EvidenceRef{ref},
	}, "")
	must(t, err)
	first, err = service.ReviewKnowledge(ctx, actor, first.ID, "approve", "")
	must(t, err)

	twelve := 12.0
	second, err := service.CreateKnowledge(ctx, actor, app.CreateKnowledgeInput{
		ProjectID: project.ID, Kind: "fact", Title: "新版净含量", Statement: "净含量 12 克", Subject: "产品", Predicate: "净含量",
		Value: domain.TypedValue{Type: "number", Number: &twelve, Unit: "g"}, Scope: domain.KnowledgeScope{Channels: []string{"douyin"}}, Evidence: []domain.EvidenceRef{ref},
	}, "")
	must(t, err)
	if second.Status != "conflicted" {
		t.Fatalf("conflicting value must remain explicit, got %s", second.Status)
	}
	first, err = service.KnowledgeItem(ctx, actor, first.ID)
	must(t, err)
	if first.Status != "review_required" {
		t.Fatalf("previously approved value must be invalidated, got %s", first.Status)
	}
	conflicts, err := service.KnowledgeConflicts(ctx, actor, project.ID)
	must(t, err)
	requests, err := service.DecisionRequests(ctx, actor, project.ID)
	must(t, err)
	if len(conflicts) != 1 || len(requests) != 1 || requests[0].ConflictID != conflicts[0].ID {
		t.Fatalf("expected one conflict and decision request: %#v %#v", conflicts, requests)
	}
	_, err = service.ReviewKnowledge(ctx, actor, second.ID, "approve", "")
	assertDomainCode(t, err, "KNOWLEDGE_CONFLICT_OPEN")
	request, err := service.ResolveDecisionRequest(ctx, actor, requests[0].ID, second.ID, "采用新版包装", "")
	must(t, err)
	if request.Status != "resolved" || request.SelectedKnowledgeID != second.ID {
		t.Fatalf("unexpected decision resolution: %#v", request)
	}
	second, err = service.ReviewKnowledge(ctx, actor, second.ID, "approve", "")
	must(t, err)
	if second.Status != "approved" || second.ApprovedBy != actor.UserID || second.ApprovedAt == nil {
		t.Fatalf("selected knowledge was not audibly approved: %#v", second)
	}
}

func TestKnowledgeValidityBlocksPrematureApproval(t *testing.T) {
	ctx := context.Background()
	service := app.New(memory.New(), slog.Default())
	session, _ := service.Register(ctx, "validity@example.com", "long-enough-password", "Reviewer", "Validity Tenant")
	actor, _, _ := service.SessionActor(ctx, session.ID)
	project, _ := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	ref := createAcceptedEvidence(t, ctx, service, actor, project.ID, "future fact", nil)
	future := time.Now().UTC().Add(24 * time.Hour)
	item, err := service.CreateKnowledge(ctx, actor, app.CreateKnowledgeInput{ProjectID: project.ID, Kind: "fact", Title: "Future", Statement: "future fact", ValidFrom: &future, Evidence: []domain.EvidenceRef{ref}}, "")
	must(t, err)
	_, err = service.ReviewKnowledge(ctx, actor, item.ID, "approve", "")
	assertDomainCode(t, err, "KNOWLEDGE_NOT_EFFECTIVE")
}

func createAcceptedEvidence(t *testing.T, ctx context.Context, service *app.Service, actor app.Actor, projectID, quote string, confidence *float64) domain.EvidenceRef {
	t.Helper()
	fileName := "evidence-" + domain.NewID() + ".txt"
	revision, err := service.UploadSource(ctx, actor, projectID, "Evidence", "brand_manual", fileName, "text/plain", []byte(quote), "")
	must(t, err)
	worker := actor
	worker.Type = "worker"
	_, err = service.CompleteSource(ctx, worker, revision.ID, app.CompleteSourceInput{DetectedMIME: "text/plain", Status: "ready", ParserVersion: "test/v1", Evidence: []app.CreateEvidenceInput{{LocatorKind: "paragraph", Locator: map[string]any{"paragraph": 1}, QuoteText: quote, OCRConfidence: confidence}}}, "")
	must(t, err)
	return domain.EvidenceRef{SourceRevisionID: revision.ID, LocatorKind: "paragraph", Locator: `{"paragraph":1}`, Quote: quote}
}

func createKnowledgeWithRef(ctx context.Context, service *app.Service, actor app.Actor, projectID string, ref domain.EvidenceRef) (domain.KnowledgeItem, error) {
	return service.CreateKnowledge(ctx, actor, app.CreateKnowledgeInput{ProjectID: projectID, Kind: "fact", Title: "Grounded fact", Statement: "Grounded statement", Evidence: []domain.EvidenceRef{ref}}, "")
}

func assertDomainCode(t *testing.T, err error, code string) {
	t.Helper()
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("expected domain error %s, got %v", code, err)
	}
	if domainErr.Code != code {
		t.Fatalf("expected error code %s, got %s", code, domainErr.Code)
	}
}
