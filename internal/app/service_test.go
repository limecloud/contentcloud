package app_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/cli"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestEndToEndScriptFlow(t *testing.T) {
	ctx := context.Background()
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(ctx, "owner@example.com", "long-enough-password", "Owner", "Agency")
	must(t, err)
	actor, _, err := service.SessionActor(ctx, session.ID)
	must(t, err)
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "Brand", ProductName: "Incense", Channel: "douyin", OwnerName: "Owner", ReviewerName: "Reviewer"}, "req-1")
	must(t, err)
	connect, err := service.CreateConnectSession(ctx, actor, project.ID, "req-2")
	must(t, err)
	connected, err := service.ConnectDevice(ctx, app.ConnectDeviceInput{ConnectKey: connect.PlaintextConnectKey, Hostname: "test-mac", Platform: "darwin", Arch: "arm64", Version: "test", Capabilities: capabilities()})
	must(t, err)
	if connected.ProjectID != project.ID {
		t.Fatal("device grant project mismatch")
	}
	if _, err := service.ConnectDevice(ctx, app.ConnectDeviceInput{ConnectKey: connect.PlaintextConnectKey, Hostname: "replay"}); err == nil {
		t.Fatal("connect key replay must fail")
	}
	evidence := createAcceptedEvidence(t, ctx, service, actor, project.ID, "Incense stick", nil)
	knowledge, err := service.CreateKnowledge(ctx, actor, app.CreateKnowledgeInput{ProjectID: project.ID, Kind: "fact", Title: "Product truth", Statement: "This is an incense stick", RiskLevel: "low", AllowedChannels: []string{"douyin"}, Evidence: []domain.EvidenceRef{evidence}}, "req-3")
	must(t, err)
	knowledge, err = service.ReviewKnowledge(ctx, actor, knowledge.ID, "approve", "req-4")
	must(t, err)
	if knowledge.Status != "approved" {
		t.Fatalf("unexpected knowledge status %s", knowledge.Status)
	}
	benchmark, err := service.CreateBenchmark(ctx, actor, app.CreateBenchmarkInput{ProjectID: project.ID, Title: "Reference", Platform: "douyin", RightsMode: "analysis_only", ValidationLevel: "observed"}, "req-5")
	must(t, err)
	framework, err := service.CreateFramework(ctx, actor, app.CreateFrameworkInput{BenchmarkID: benchmark.ID, Name: "Problem to proof", VisualSequence: []string{"hook", "proof"}, CopySequence: []string{"problem", "solution"}}, "req-5")
	must(t, err)
	point, err := service.CreateSellingPoint(ctx, actor, app.CreateSellingPointInput{ProjectID: project.ID, Title: "daily ritual", Priority: 1, KnowledgeIDs: []string{knowledge.ID}}, "req-5")
	must(t, err)
	plan, err := service.CreateVisualizationPlan(ctx, actor, app.CreateVisualizationPlanInput{SellingPointID: point.ID, Title: "Product proof", ProofType: "process", Implementation: "real product", ProductTruthStrategy: "real_asset_composite", AcceptanceCriteria: []string{"product remains legible"}}, "req-5")
	must(t, err)
	plan, err = service.ReviewVisualizationPlan(ctx, actor, plan.ID, "approve", "req-5")
	must(t, err)
	brief, err := service.CreateBrief(ctx, actor, app.CreateBriefInput{ProjectID: project.ID, Objective: "awareness", Audience: "urban users", DemandMoment: "after work", Scene: "quiet living room", Conflict: "information overload prevents relaxation", PrimarySellingPoint: "daily ritual", EvidenceSummary: "approved product truth and visible ritual", CTA: "read guide", Channel: "douyin", AspectRatio: "9:16", TargetDurationSeconds: 30, PrimaryTestVariable: "hook", ApprovedKnowledgeIDs: []string{knowledge.ID}, FrameworkIDs: []string{framework.ID}, VisualizationPlanIDs: []string{plan.ID}}, "req-5")
	must(t, err)
	brief, err = service.ReviewBrief(ctx, actor, brief.ID, "submit", "req-6")
	must(t, err)
	brief, err = service.ReviewBrief(ctx, actor, brief.ID, "approve", "req-7")
	must(t, err)
	run, err := service.CreateScriptRun(ctx, actor, brief.ID, "test-run", "req-8")
	must(t, err)
	deviceActor, device, err := service.DeviceActor(ctx, connected.DeviceToken)
	must(t, err)
	lease, err := service.Poll(ctx, deviceActor, device, capabilities())
	must(t, err)
	if lease.Run.ID != run.ID {
		t.Fatal("leased wrong run")
	}
	if _, err := service.HeartbeatRun(ctx, deviceActor, device, run.ID, lease.Attempt.ID, lease.RunToken, domain.RunHeartbeat{Sequence: 1, Phase: "executing", Label: "working"}, "req-heartbeat"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.HeartbeatRun(ctx, deviceActor, device, run.ID, lease.Attempt.ID, lease.RunToken, domain.RunHeartbeat{Sequence: 1, Phase: "executing", Label: "replayed"}, "req-heartbeat-replay"); err == nil {
		t.Fatal("replayed heartbeat sequence must fail")
	}
	pkg := cli.GenerateFixtureScript(lease.Contract)
	script, err := service.ReportRun(ctx, deviceActor, device, run.ID, lease.RunToken, pkg, "req-9")
	must(t, err)
	if script.Status != "review_ready" || !script.Validation.Valid {
		t.Fatalf("script was not review ready: %#v", script.Validation)
	}
	idempotent, err := service.ReportRun(ctx, deviceActor, device, run.ID, lease.RunToken, pkg, "req-9-replay")
	must(t, err)
	if idempotent.ID != script.ID {
		t.Fatal("same report must return the existing ScriptVersion")
	}
	conflicting := pkg
	conflicting.Title = "different report"
	if _, err := service.ReportRun(ctx, deviceActor, device, run.ID, lease.RunToken, conflicting, "req-9-conflict"); err == nil {
		t.Fatal("different report for a terminal run must conflict")
	}
	script, err = service.ReviewScript(ctx, actor, script.ID, "submit", "", "req-10")
	must(t, err)
	blockingComment, err := service.CreateReviewComment(ctx, actor, app.CreateReviewCommentInput{SubjectID: script.ID, ShotID: script.Package.Shots[0].ShotID, Body: "resolve before internal approval", Visibility: "internal"}, "req-comment-blocking")
	must(t, err)
	_, err = service.ReviewScript(ctx, actor, script.ID, "approve_internal", "ready for client review", "req-11-blocked")
	assertDomainCode(t, err, "REVIEW_COMMENTS_UNRESOLVED")
	_, err = service.ResolveReviewComment(ctx, actor, blockingComment.ID, "req-comment-blocking-resolve")
	must(t, err)
	script, err = service.ReviewScript(ctx, actor, script.ID, "approve_internal", "ready for client review", "req-11")
	must(t, err)
	_, err = service.CreateReviewComment(ctx, actor, app.CreateReviewCommentInput{SubjectID: script.ID, ShotID: script.Package.Shots[0].ShotID, Body: "internal note", Visibility: "internal"}, "req-comment-internal")
	must(t, err)
	clientComment, err := service.CreateReviewComment(ctx, actor, app.CreateReviewCommentInput{SubjectID: script.ID, ShotID: script.Package.Shots[0].ShotID, Body: "client note", Visibility: "client"}, "req-comment-client")
	must(t, err)
	grant, err := service.CreateReviewGrant(ctx, actor, script.ID, "client@example.com", "req-12")
	must(t, err)
	revokedGrant, err := service.CreateReviewGrant(ctx, actor, script.ID, "revoked@example.com", "req-12-revoked")
	must(t, err)
	revokedToken := revokedGrant.PlaintextToken
	revokedGrant, err = service.RevokeReviewGrant(ctx, actor, revokedGrant.ID, "req-12-revoke")
	must(t, err)
	if revokedGrant.RevokedAt == nil {
		t.Fatal("review grant revocation was not persisted")
	}
	if _, err := service.ReviewProjection(ctx, revokedToken); err == nil {
		t.Fatal("revoked review token must fail immediately")
	}
	grants, err := service.ReviewGrants(ctx, actor, script.ID)
	must(t, err)
	if len(grants) != 2 || grants[0].TokenHash != "" || grants[0].OTPHash != "" {
		t.Fatalf("review grant list leaked secrets or omitted history: %#v", grants)
	}
	projection, err := service.ReviewProjection(ctx, grant.PlaintextToken)
	must(t, err)
	if projection.Verified || projection.Script.ID != "" {
		t.Fatal("unverified review link must not expose the script")
	}
	if _, err = service.VerifyReviewGrant(ctx, grant.PlaintextToken, "000000"); err == nil {
		t.Fatal("wrong OTP must fail")
	}
	verified, err := service.VerifyReviewGrant(ctx, grant.PlaintextToken, grant.PlaintextOTP)
	must(t, err)
	if len(verified.Comments) != 1 || verified.Comments[0].Body != "client note" {
		t.Fatal("customer projection must only contain client-visible comments")
	}
	_, err = service.DecideReviewGrant(ctx, grant.PlaintextToken, "approve", "approved", "", "req-13-blocked")
	assertDomainCode(t, err, "REVIEW_COMMENTS_UNRESOLVED")
	_, err = service.ResolveReviewComment(ctx, actor, clientComment.ID, "req-comment-client-resolve")
	must(t, err)
	script, err = service.DecideReviewGrant(ctx, grant.PlaintextToken, "approve", "approved", "", "req-13")
	must(t, err)
	if script.Status != "approved" {
		t.Fatalf("expected approved, got %s", script.Status)
	}
	revisedBrief, err := service.CreateBrief(ctx, actor, app.CreateBriefInput{ProjectID: project.ID, Objective: "conversion", Audience: brief.Audience, DemandMoment: brief.DemandMoment, Scene: brief.Scene, Conflict: brief.Conflict, PrimarySellingPoint: brief.PrimarySellingPoint, EvidenceSummary: brief.EvidenceSummary, CTA: "view product details", Channel: brief.Channel, AspectRatio: brief.AspectRatio, TargetDurationSeconds: brief.TargetDurationSeconds, PrimaryTestVariable: "cta", ApprovedKnowledgeIDs: brief.ApprovedKnowledgeIDs, FrameworkIDs: brief.FrameworkIDs, VisualizationPlanIDs: brief.VisualizationPlanIDs, SupersedesID: brief.ID, RevisionReason: "change the primary objective and test variable"}, "req-brief-revision")
	must(t, err)
	revisedBrief, err = service.ReviewBrief(ctx, actor, revisedBrief.ID, "submit", "req-brief-revision-submit")
	must(t, err)
	revisedBrief, err = service.ReviewBrief(ctx, actor, revisedBrief.ID, "approve", "req-brief-revision-approve")
	must(t, err)
	if revisedBrief.ApprovedAt == nil || revisedBrief.ApprovedBy != actor.UserID {
		t.Fatalf("brief approval identity was not captured: %#v", revisedBrief)
	}
	previousBrief, err := service.Brief(ctx, actor, brief.ID)
	must(t, err)
	if previousBrief.Status != "superseded" {
		t.Fatalf("old brief must be superseded, got %s", previousBrief.Status)
	}
	script, err = service.Script(ctx, actor, script.ID)
	must(t, err)
	if script.Status != "review_required" {
		t.Fatalf("script based on superseded Brief must require review, got %s", script.Status)
	}
	approvals, err := service.Approvals(ctx, actor, revisedBrief.ID)
	must(t, err)
	if len(approvals) != 1 || approvals[0].SubjectHash != revisedBrief.ContentHash {
		t.Fatalf("brief approval must bind content hash: %#v", approvals)
	}
	revisionRun, err := service.CreateScriptChangeRun(ctx, actor, script.ID, app.CreateScriptChangeRunInput{BriefVersionID: revisedBrief.ID, ChangeType: "revision", RevisionReason: "根据新版 Brief 重写转化目标", IdempotencyKey: "script-revision-1"}, "req-script-revision")
	must(t, err)
	revisionLease, err := service.Poll(ctx, deviceActor, device, capabilities())
	must(t, err)
	if revisionLease.Run.ID != revisionRun.ID || revisionLease.Contract.BaselineScriptVersion == nil || revisionLease.Contract.BaselineScriptVersion.ID != script.ID || revisionLease.Contract.ChangeRequest == nil || revisionLease.Contract.ChangeRequest.ChangeType != "revision" {
		t.Fatalf("script revision contract did not freeze lineage: %#v", revisionLease.Contract)
	}
	revisionPackage := cli.GenerateFixtureScript(revisionLease.Contract)
	revision, err := service.ReportRunAttempt(ctx, deviceActor, device, revisionRun.ID, revisionLease.Attempt.ID, revisionLease.RunToken, revisionPackage, "req-script-revision-report")
	must(t, err)
	if revision.ScriptID != script.ScriptID || revision.Version != 2 || revision.SupersedesID != script.ID || revision.ChangeType != "revision" || len(revision.ChangedFields) == 0 {
		t.Fatalf("script revision lineage is incomplete: %#v", revision)
	}
	revisionCycles, err := service.ReviewCycles(ctx, actor, revision.ID)
	must(t, err)
	revisionComments, err := service.ReviewComments(ctx, actor, revision.ID)
	must(t, err)
	if len(revisionCycles) != 1 || len(revisionComments) != 1 || revisionComments[0].Body != "internal note" || revisionComments[0].CarriedFromID == "" || revisionComments[0].ReviewCycleID != revisionCycles[0].ID {
		t.Fatalf("unresolved comments were not carried into the new review cycle: cycles=%#v comments=%#v", revisionCycles, revisionComments)
	}

	variantRun, err := service.CreateScriptChangeRun(ctx, actor, revision.ID, app.CreateScriptChangeRunInput{BriefVersionID: revisedBrief.ID, ChangeType: "variant", ChangedFields: []string{"/title"}, Hypothesis: "具体标题能提高前三秒停留", RevisionReason: "创建标题单变量版本", IdempotencyKey: "script-variant-1"}, "req-script-variant")
	must(t, err)
	variantLease, err := service.Poll(ctx, deviceActor, device, capabilities())
	must(t, err)
	variantPackage := cli.GenerateFixtureScript(variantLease.Contract)
	variantPackage.Title += " A"
	variant, err := service.ReportRunAttempt(ctx, deviceActor, device, variantRun.ID, variantLease.Attempt.ID, variantLease.RunToken, variantPackage, "req-script-variant-report")
	must(t, err)
	if variant.Version != 3 || variant.BaselineID != revision.ID || len(variant.ChangedFields) != 1 || variant.ChangedFields[0] != "/title" {
		t.Fatalf("single-variable variant lineage is invalid: %#v", variant)
	}

	invalidVariantRun, err := service.CreateScriptChangeRun(ctx, actor, revision.ID, app.CreateScriptChangeRunInput{BriefVersionID: revisedBrief.ID, ChangeType: "variant", ChangedFields: []string{"/title"}, Hypothesis: "标题实验", RevisionReason: "验证单变量门禁", IdempotencyKey: "script-variant-invalid"}, "req-script-variant-invalid")
	must(t, err)
	invalidVariantLease, err := service.Poll(ctx, deviceActor, device, capabilities())
	must(t, err)
	invalidVariantPackage := cli.GenerateFixtureScript(invalidVariantLease.Contract)
	invalidVariantPackage.Title += " B"
	invalidVariantPackage.Narrative[0] = "changed outside declared variable"
	_, err = service.ReportRunAttempt(ctx, deviceActor, device, invalidVariantRun.ID, invalidVariantLease.Attempt.ID, invalidVariantLease.RunToken, invalidVariantPackage, "req-script-variant-invalid-report")
	assertDomainCode(t, err, "CAPABILITY_OUTPUT_INVALID")
	invalidVariantRun, err = service.Run(ctx, actor, invalidVariantRun.ID)
	must(t, err)
	if invalidVariantRun.State != "queued" {
		t.Fatalf("invalid variant should be queued for a bounded retry, got %s", invalidVariantRun.State)
	}
	_, err = service.CancelRun(ctx, actor, invalidVariantRun.ID, "req-script-variant-invalid-cancel")
	must(t, err)
	canceledRun, err := service.CreateScriptRun(ctx, actor, revisedBrief.ID, "cancel-run", "req-14")
	must(t, err)
	canceledRun, err = service.CancelRun(ctx, actor, canceledRun.ID, "req-15")
	must(t, err)
	if canceledRun.State != "canceled" {
		t.Fatalf("queued cancellation must be terminal, got %s", canceledRun.State)
	}
	if _, err := service.Poll(ctx, deviceActor, device, capabilities()); err == nil {
		t.Fatal("canceled task must not be leased")
	}
}

func TestTenantIsolation(t *testing.T) {
	ctx := context.Background()
	service := app.New(memory.New(), slog.Default())
	aSession, _ := service.Register(ctx, "a@example.com", "long-enough-password", "A", "Tenant A")
	bSession, _ := service.Register(ctx, "b@example.com", "long-enough-password", "B", "Tenant B")
	a, _, _ := service.SessionActor(ctx, aSession.ID)
	b, _, _ := service.SessionActor(ctx, bSession.ID)
	project, _ := service.CreateProject(ctx, a, app.CreateProjectInput{BrandName: "Same", ProductName: "Product"}, "")
	if _, err := service.Project(ctx, b, project.ID); err == nil {
		t.Fatal("tenant B accessed tenant A project")
	}
}

func capabilities() []domain.Capability {
	return []domain.Capability{
		{ID: domain.KnowledgeExtractCapability, Version: "1.0.0", Kind: "business_capability", InputSchema: domain.TaskContractSchema, OutputSchema: domain.KnowledgeCandidatesSchema, LocalOnly: true},
		{ID: domain.ScriptCapability, Version: "1.1.0", Kind: "business_capability", InputSchema: domain.TaskContractSchema, OutputSchema: domain.ScriptPackageSchema, LocalOnly: true},
	}
}
func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

var _ = time.Second

// inviteFixture 返回一个租户管理员 actor 与一封发给 invitedEmail 的待接受邀请。
func inviteFixture(t *testing.T, service *app.Service, invitedEmail, role string) (app.Actor, domain.MembershipInvite) {
	t.Helper()
	ctx := context.Background()
	adminSession, err := service.Register(ctx, "inviter@example.com", "long-enough-password", "管理员", "邀请方租户")
	must(t, err)
	admin, _, err := service.SessionActor(ctx, adminSession.ID)
	must(t, err)
	invite, err := service.CreateMembershipInvite(ctx, admin, invitedEmail, role, "req-invite")
	must(t, err)
	return admin, invite
}

func TestRegisterWithInviteJoinsInvitingTenant(t *testing.T) {
	ctx := context.Background()
	service := app.New(memory.New(), slog.Default())
	admin, invite := inviteFixture(t, service, "newbie@example.com", "reviewer")

	session, err := service.RegisterWithInvite(ctx, "newbie@example.com", "long-enough-password", "新同事", invite.PlaintextToken)
	must(t, err)
	if session.TenantID != admin.TenantID {
		t.Fatalf("session must land in inviting tenant: got %s want %s", session.TenantID, admin.TenantID)
	}
	actor, user, err := service.SessionActor(ctx, session.ID)
	must(t, err)
	if actor.Role != "reviewer" {
		t.Fatalf("role must come from invite: got %s", actor.Role)
	}
	if user.DisplayName != "新同事" {
		t.Fatalf("unexpected display name %q", user.DisplayName)
	}
	// 关键区别：不得创建一个属于自己的新租户。
	tenants, err := service.Tenants(ctx, actor)
	must(t, err)
	if len(tenants) != 1 || tenants[0].ID != admin.TenantID {
		t.Fatalf("invited user must belong to exactly the inviting tenant: %#v", tenants)
	}
	if _, err := service.RegisterWithInvite(ctx, "other@example.com", "long-enough-password", "", invite.PlaintextToken); err == nil {
		t.Fatal("accepted invite must not be reusable")
	}
}

func TestRegisterWithInviteRejectsMismatchedEmailWithoutCreatingUser(t *testing.T) {
	ctx := context.Background()
	service := app.New(memory.New(), slog.Default())
	_, invite := inviteFixture(t, service, "newbie@example.com", "reviewer")

	_, err := service.RegisterWithInvite(ctx, "attacker@example.com", "long-enough-password", "", invite.PlaintextToken)
	assertDomainCode(t, err, "INVITE_INVALID")
	// 邀请校验失败不得留下孤儿用户：该邮箱仍可正常注册自己的团队。
	if _, err := service.Register(ctx, "attacker@example.com", "long-enough-password", "", "自建租户"); err != nil {
		t.Fatalf("email must remain unregistered after failed invite: %v", err)
	}
}

func TestRegisterWithInviteRejectsRevokedAndUnknownToken(t *testing.T) {
	ctx := context.Background()
	service := app.New(memory.New(), slog.Default())
	adminSession, err := service.Register(ctx, "inviter@example.com", "long-enough-password", "管理员", "邀请方租户")
	must(t, err)
	admin, _, err := service.SessionActor(ctx, adminSession.ID)
	must(t, err)
	invite, err := service.CreateMembershipInvite(ctx, admin, "revoked@example.com", "viewer", "req-invite")
	must(t, err)
	if _, err := service.RevokeMembershipInvite(ctx, admin, invite.ID, "req-revoke"); err != nil {
		t.Fatal(err)
	}

	_, err = service.RegisterWithInvite(ctx, "revoked@example.com", "long-enough-password", "", invite.PlaintextToken)
	assertDomainCode(t, err, "INVITE_INVALID")
	_, err = service.RegisterWithInvite(ctx, "nobody@example.com", "long-enough-password", "", "cci_not-a-real-token")
	assertDomainCode(t, err, "INVITE_INVALID")
}

func TestRegisterWithInviteStillValidatesCredentials(t *testing.T) {
	ctx := context.Background()
	service := app.New(memory.New(), slog.Default())
	_, invite := inviteFixture(t, service, "newbie@example.com", "reviewer")

	_, err := service.RegisterWithInvite(ctx, "newbie@example.com", "short", "", invite.PlaintextToken)
	assertDomainCode(t, err, "REGISTRATION_INVALID")
	// 凭据校验先于邀请核销，邀请必须仍然可用。
	if _, err := service.RegisterWithInvite(ctx, "newbie@example.com", "long-enough-password", "", invite.PlaintextToken); err != nil {
		t.Fatalf("invite must survive a rejected registration attempt: %v", err)
	}
}

func TestAcceptMembershipInviteOnlySucceedsOnce(t *testing.T) {
	ctx := context.Background()
	service := app.New(memory.New(), slog.Default())
	admin, invite := inviteFixture(t, service, "member@example.com", "editor")
	memberSession, err := service.Register(ctx, "member@example.com", "long-enough-password", "成员", "成员自己的租户")
	must(t, err)
	member, _, err := service.SessionActor(ctx, memberSession.ID)
	must(t, err)

	start := make(chan struct{})
	errs := make([]error, 2)
	var wait sync.WaitGroup
	for index := range errs {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			_, errs[index] = service.AcceptMembershipInvite(ctx, member, invite.PlaintextToken, "req-accept")
		}(index)
	}
	close(start)
	wait.Wait()

	successes := 0
	for _, acceptErr := range errs {
		if acceptErr == nil {
			successes++
			continue
		}
		var domainErr *domain.Error
		if !errors.As(acceptErr, &domainErr) || domainErr.Code != "INVITE_INVALID" {
			t.Fatalf("unexpected concurrent accept error: %v", acceptErr)
		}
	}
	if successes != 1 {
		t.Fatalf("invite must be accepted exactly once, got %d successes", successes)
	}
	accepted, err := service.Members(ctx, admin)
	must(t, err)
	if len(accepted) != 2 {
		t.Fatalf("inviting tenant must contain admin and accepted member: %#v", accepted)
	}
}
