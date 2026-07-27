package httpapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/httpapi"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestDevBootstrapAndDashboard(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	resp, err := client.Post(server.URL+"/api/v1/dev/bootstrap", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bootstrap status %d", resp.StatusCode)
	}
	resp, err = client.Get(server.URL + "/api/bff/dashboard")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var value struct {
		OK   bool `json:"ok"`
		Data struct {
			Projects []any `json:"projects"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&value); err != nil {
		t.Fatal(err)
	}
	if !value.OK || len(value.Data.Projects) != 1 {
		t.Fatalf("unexpected dashboard %#v", value)
	}
}

func TestBFFRequiresSession(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	defer server.Close()
	resp, err := http.Get(server.URL + "/api/bff/dashboard")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestSessionCookieSecurityFollowsRequestScheme(t *testing.T) {
	tests := []struct {
		name           string
		forwardedProto string
		expectedSecure bool
	}{
		{name: "plain HTTP", expectedSecure: false},
		{name: "HTTPS reverse proxy", forwardedProto: "https", expectedSecure: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := app.New(memory.New(), slog.Default())
			server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
			defer server.Close()
			request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/api/v1/auth/register", strings.NewReader(`{"email":"cookie@example.com","password":"long-enough-password","display_name":"Cookie User","tenant_name":"Cookie Tenant"}`))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			if test.forwardedProto != "" {
				request.Header.Set("X-Forwarded-Proto", test.forwardedProto)
			}
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("registration status %d", response.StatusCode)
			}
			cookies := response.Cookies()
			if len(cookies) != 1 || cookies[0].Name != "cc_session" {
				t.Fatalf("unexpected session cookies %#v", cookies)
			}
			if cookies[0].Secure != test.expectedSecure {
				t.Fatalf("Secure=%v, want %v", cookies[0].Secure, test.expectedSecure)
			}
		})
	}
}

func TestPlatformAdminEndpointsRequireExplicitGrant(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	response, err := client.Post(server.URL+"/api/v1/dev/bootstrap", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	response, err = client.Get(server.URL + "/api/v1/admin/dashboard")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("expected explicit platform grant to be required, got %d", response.StatusCode)
	}
}

func TestPlatformAdminOverviewAndTenantStatusEndpoint(t *testing.T) {
	service := app.New(memory.New(), slog.Default(), app.WithPlatformAdminEmails("demo@contentcloud.local"))
	targetSession, err := service.Register(t.Context(), "customer@example.com", "long-enough-password", "Customer", "Customer Tenant")
	if err != nil {
		t.Fatal(err)
	}
	targetActor, _, err := service.SessionActor(t.Context(), targetSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	response, err := client.Post(server.URL+"/api/v1/dev/bootstrap", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	overview := callBFF[domain.PlatformOverview](t, client, http.MethodGet, server.URL+"/api/v1/admin/dashboard", nil)
	if overview.Counts.Tenants != 2 || overview.Counts.Users != 2 {
		t.Fatalf("unexpected platform overview %#v", overview.Counts)
	}
	tenant := callBFF[domain.Tenant](t, client, http.MethodPatch, server.URL+"/api/v1/admin/tenants/"+targetActor.TenantID, map[string]string{"status": "suspended"})
	if tenant.Status != "suspended" {
		t.Fatalf("unexpected tenant status %#v", tenant)
	}
	if _, _, err := service.SessionActor(t.Context(), targetSession.ID); err == nil {
		t.Fatal("tenant status endpoint did not revoke active sessions")
	}
}

func TestBFFTeamProjectAndConnectionOperations(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	response, err := client.Post(server.URL+"/api/v1/dev/bootstrap", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("bootstrap status %d", response.StatusCode)
	}

	members := callBFF[[]app.MemberView](t, client, http.MethodGet, server.URL+"/api/bff/team/members", nil)
	if len(members) != 1 || members[0].Membership.Role != "tenant_admin" {
		t.Fatalf("unexpected members %#v", members)
	}
	invite := callBFF[domain.MembershipInvite](t, client, http.MethodPost, server.URL+"/api/bff/team/invites", map[string]string{"email": "new-member@example.com", "role": "editor"})
	if invite.PlaintextToken == "" || invite.Status != "pending" {
		t.Fatalf("unexpected invitation %#v", invite)
	}
	invite = callBFF[domain.MembershipInvite](t, client, http.MethodPost, server.URL+"/api/bff/team/invites/"+invite.ID+"/revoke", map[string]any{})
	if invite.Status != "revoked" {
		t.Fatalf("invite was not revoked %#v", invite)
	}

	template := callBFF[domain.ProjectTemplate](t, client, http.MethodPost, server.URL+"/api/bff/project-templates", app.CreateProjectTemplateInput{Name: "抖音验证", Channel: "douyin", StageObjective: "验证主卖点"})
	if template.Name != "抖音验证" {
		t.Fatalf("unexpected template %#v", template)
	}
	projects := callBFF[[]domain.Project](t, client, http.MethodGet, server.URL+"/api/bff/projects", nil)
	if len(projects) != 1 {
		t.Fatalf("unexpected projects %#v", projects)
	}
	project := projects[0]
	project = callBFF[domain.Project](t, client, http.MethodPatch, server.URL+"/api/bff/projects/"+project.ID, map[string]any{"row_version": project.RowVersion, "owner_name": "项目负责人"})
	if project.OwnerName != "项目负责人" || project.RowVersion != projects[0].RowVersion+1 {
		t.Fatalf("unexpected project update %#v", project)
	}
	connect := callBFF[domain.ConnectSession](t, client, http.MethodPost, server.URL+"/api/bff/projects/"+project.ID+"/connect-sessions", map[string]any{})
	if connect.ID == "" || connect.State != "waiting_for_computer" || connect.Progress != nil {
		t.Fatalf("unexpected connect session %#v", connect)
	}
	connect = callBFF[domain.ConnectSession](t, client, http.MethodPost, server.URL+"/api/bff/connect-sessions/"+connect.ID+"/cancel", map[string]any{})
	if connect.State != "canceled" {
		t.Fatalf("connect session was not canceled %#v", connect)
	}
	project = callBFF[domain.Project](t, client, http.MethodPost, server.URL+"/api/bff/projects/"+project.ID+"/archive", map[string]any{"row_version": project.RowVersion})
	if project.Status != "archived" {
		t.Fatalf("project was not archived %#v", project)
	}
	project = callBFF[domain.Project](t, client, http.MethodPost, server.URL+"/api/bff/projects/"+project.ID+"/restore", map[string]any{"row_version": project.RowVersion})
	if project.Status != "active" {
		t.Fatalf("project was not restored %#v", project)
	}
}

func TestBFFBriefRevisionSubmitAndReturn(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
	server := httptest.NewServer(httpapi.New(service, slog.Default(), true, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	response, err := client.Post(server.URL+"/api/v1/dev/bootstrap", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("bootstrap status %d", response.StatusCode)
	}
	baseURL, _ := url.Parse(server.URL)
	cookies := jar.Cookies(baseURL)
	if len(cookies) == 0 {
		t.Fatal("bootstrap did not establish a session")
	}
	actor, _, err := service.SessionActor(t.Context(), cookies[0].Value)
	if err != nil {
		t.Fatal(err)
	}
	projects, err := service.Projects(t.Context(), actor)
	if err != nil || len(projects) != 1 {
		t.Fatalf("demo project: %v %#v", err, projects)
	}
	briefs, err := service.Briefs(t.Context(), actor, projects[0].ID)
	if err != nil || len(briefs) != 1 {
		t.Fatalf("demo brief: %v %#v", err, briefs)
	}
	base := briefs[0]
	input := app.CreateBriefInput{
		Objective:              "验证更明确的收藏转化目标",
		Audience:               base.Audience,
		DemandMoment:           base.DemandMoment,
		Scene:                  base.Scene,
		Conflict:               base.Conflict,
		PrimarySellingPoint:    base.PrimarySellingPoint,
		SecondarySellingPoints: base.SecondarySellingPoints,
		CTA:                    base.CTA,
		Channel:                base.Channel,
		AspectRatio:            base.AspectRatio,
		EvidenceSummary:        base.EvidenceSummary,
		TargetDurationSeconds:  base.TargetDurationSeconds,
		PrimaryTestVariable:    base.PrimaryTestVariable,
		ApprovedKnowledgeIDs:   base.ApprovedKnowledgeIDs,
		FrameworkIDs:           base.FrameworkIDs,
		VisualizationPlanIDs:   base.VisualizationPlanIDs,
		Viewpoint:              base.Viewpoint,
		Constraints:            base.Constraints,
		RevisionReason:         "收窄阶段目标",
	}
	revised := callBFF[domain.BriefVersion](t, client, http.MethodPost, server.URL+"/api/bff/briefs/"+base.ID+"/versions", input)
	if revised.SupersedesID != base.ID || revised.ProjectID != base.ProjectID || revised.Version != 2 {
		t.Fatalf("unexpected immutable revision %#v", revised)
	}
	revised = callBFF[domain.BriefVersion](t, client, http.MethodPost, server.URL+"/api/bff/briefs/"+revised.ID+"/review", map[string]string{"decision": "submit"})
	if revised.Status != "internal_review" {
		t.Fatalf("submit status = %s", revised.Status)
	}
	revised = callBFF[domain.BriefVersion](t, client, http.MethodPost, server.URL+"/api/bff/briefs/"+revised.ID+"/review", map[string]string{"decision": "return", "reason": "补充目标人群证据"})
	if revised.Status != "revision_requested" {
		t.Fatalf("return status = %s", revised.Status)
	}
}

func TestBFFSourceRevisionEvidenceAndImpactRoutes(t *testing.T) {
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(t.Context(), "source-bff@example.com", "long-enough-password", "Reviewer", "Source BFF Tenant")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(t.Context(), actor, app.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.UploadSource(t.Context(), actor, project.ID, "Manual", "brand_manual", "manual-v1.txt", "text/plain", []byte("first revision"), "")
	if err != nil {
		t.Fatal(err)
	}
	confidence := 0.5
	worker := actor
	worker.Type = "worker"
	_, err = service.CompleteSource(t.Context(), worker, first.ID, app.CompleteSourceInput{
		DetectedMIME: "text/plain",
		Status:       "ready",
		Evidence:     []app.CreateEvidenceInput{{LocatorKind: "paragraph", Locator: map[string]any{"paragraph": 1}, QuoteText: "first revision", OCRConfidence: &confidence}},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	spans, err := service.Evidence(t.Context(), actor, first.ID)
	if err != nil || len(spans) != 1 {
		t.Fatalf("seed evidence: %v %#v", err, spans)
	}

	server := httptest.NewServer(httpapi.New(service, slog.Default(), false, "").Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	baseURL, _ := url.Parse(server.URL)
	jar.SetCookies(baseURL, []*http.Cookie{{Name: "cc_session", Value: session.ID, Path: "/"}})
	client := &http.Client{Jar: jar}

	assertBFFOK(t, client, http.MethodGet, server.URL+"/api/bff/sources/"+first.SourceID+"/revisions", "", nil)
	assertBFFOK(t, client, http.MethodPost, server.URL+"/api/bff/evidence/"+spans[0].ID+"/review", "application/json", strings.NewReader(`{"decision":"accept"}`))
	assertBFFOK(t, client, http.MethodPost, server.URL+"/api/bff/projects/"+project.ID+"/knowledge-extraction-runs", "application/json", strings.NewReader(`{"source_revision_ids":["`+first.ID+`"],"output_count":5,"idempotency_key":"bff-extract"}`))
	runs, err := service.Runs(t.Context(), actor, project.ID)
	if err != nil || len(runs) != 1 || runs[0].TaskType != "knowledge_extract" {
		t.Fatalf("knowledge extraction route did not queue a local run: %v %#v", err, runs)
	}
	assertBFFOK(t, client, http.MethodPost, server.URL+"/api/bff/projects/"+project.ID+"/assets", "application/json", strings.NewReader(`{"name":"Product hero","asset_type":"product_image","source_revision_id":"`+first.ID+`","usage_mode":"generation_reference"}`))
	assets, err := service.Assets(t.Context(), actor, project.ID)
	if err != nil || len(assets) != 1 {
		t.Fatalf("asset route did not persist asset: %v %#v", err, assets)
	}
	assertBFFOK(t, client, http.MethodPost, server.URL+"/api/bff/assets/"+assets[0].ID+"/rights", "application/json", strings.NewReader(`{"rights_holder":"Brand","rights_type":"owned","territories":["CN"],"channels":["douyin"],"proof_source_revision_id":"`+first.ID+`","restrictions":[]}`))
	rights, err := service.RightsRecords(t.Context(), actor, assets[0].ID)
	if err != nil || len(rights) != 1 {
		t.Fatalf("rights route did not persist record: %v %#v", err, rights)
	}
	assertBFFOK(t, client, http.MethodPost, server.URL+"/api/bff/rights/"+rights[0].ID+"/review", "application/json", strings.NewReader(`{"decision":"approve"}`))
	assertBFFOK(t, client, http.MethodGet, server.URL+"/api/bff/assets/"+assets[0].ID+"/rights", "", nil)

	var uploadBody bytes.Buffer
	writer := multipart.NewWriter(&uploadBody)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, "manual-v2.txt"))
	header.Set("Content-Type", "text/plain")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("second revision")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	assertBFFOK(t, client, http.MethodPost, server.URL+"/api/bff/sources/"+first.SourceID+"/revisions/upload", writer.FormDataContentType(), &uploadBody)
	assertBFFOK(t, client, http.MethodGet, server.URL+"/api/bff/sources/"+first.SourceID+"/impact", "", nil)

	updated, err := service.SourceRevisions(t.Context(), actor, first.SourceID)
	if err != nil || len(updated) != 2 || updated[0].SupersedesID != first.ID {
		t.Fatalf("revision route did not persist a revision chain: %v %#v", err, updated)
	}
	accepted, err := service.Evidence(t.Context(), actor, first.ID)
	if err != nil || accepted[0].ReviewStatus != "accepted" {
		t.Fatalf("evidence route did not persist decision: %v %#v", err, accepted)
	}
}

func assertBFFOK(t *testing.T, client *http.Client, method, target, contentType string, body io.Reader) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), method, target, body)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var envelope struct {
		OK    bool          `json:"ok"`
		Error *domain.Error `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !envelope.OK {
		t.Fatalf("%s %s failed: status=%d error=%#v", method, target, response.StatusCode, envelope.Error)
	}
}

func callBFF[T any](t *testing.T, client *http.Client, method, target string, input any) T {
	t.Helper()
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), method, target, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var envelope struct {
		OK    bool          `json:"ok"`
		Data  T             `json:"data"`
		Error *domain.Error `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !envelope.OK {
		t.Fatalf("%s %s failed: status=%d error=%#v", method, target, response.StatusCode, envelope.Error)
	}
	return envelope.Data
}
