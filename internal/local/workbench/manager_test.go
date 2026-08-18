package workbench

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	localworkspace "github.com/limecloud/contentcloud/internal/local/workspace"
)

type testClock struct {
	mu    sync.Mutex
	value time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.value
}

func (c *testClock) Advance(duration time.Duration) {
	c.mu.Lock()
	c.value = c.value.Add(duration)
	c.mu.Unlock()
}

type browserCredentials struct {
	Capability string    `json:"capability"`
	CSRF       string    `json:"csrf"`
	ExpiresAt  time.Time `json:"expires_at"`
}

func TestWorkbenchHandoffAndHTTPBoundary(t *testing.T) {
	fixture := newWorkbenchFixture(t, "50-production/local.md", []byte("local workbench\n"))

	index := fixture.request(t, http.MethodGet, "/", nil, requestOptions{})
	assertStatus(t, index, http.StatusOK)
	indexBody := readBody(t, index)
	if !strings.Contains(index.Header.Get("Content-Security-Policy"), "default-src 'self'") ||
		!strings.Contains(index.Header.Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatalf("workbench index is missing restrictive CSP: %q", index.Header.Get("Content-Security-Policy"))
	}
	if strings.Contains(indexBody, fixture.root) || strings.Contains(indexBody, fixture.token) {
		t.Fatalf("workbench index leaked workspace root or handoff token: %s", indexBody)
	}

	badHost := fixture.request(t, http.MethodGet, "/", nil, requestOptions{host: "evil.example"})
	assertStatus(t, badHost, http.StatusForbidden)
	closeBody(badHost)

	badOrigin := fixture.exchange(t, fixture.token, "https://evil.example")
	assertStatus(t, badOrigin, http.StatusForbidden)
	closeBody(badOrigin)

	credentials, exchanged := fixture.exchangeCredentials(t)
	assertStatus(t, exchanged, http.StatusOK)
	resourceCookie := resourceCookieFromResponse(t, exchanged)
	closeBody(exchanged)
	if credentials.Capability == "" || credentials.CSRF == "" || !credentials.ExpiresAt.After(fixture.clock.Now()) {
		t.Fatalf("exchange returned incomplete browser credentials: %#v", credentials)
	}
	if resourceCookie.Path != "/api/v1/resources/" || !resourceCookie.HttpOnly || resourceCookie.SameSite != http.SameSiteStrictMode || resourceCookie.MaxAge != 0 || !resourceCookie.Expires.IsZero() {
		t.Fatalf("resource cookie exceeded its session-only read scope: %#v", resourceCookie)
	}

	replayed := fixture.exchange(t, fixture.token, fixture.origin)
	assertStatus(t, replayed, http.StatusGone)
	closeBody(replayed)

	missingCapability := fixture.request(t, http.MethodGet, "/api/v1/bootstrap", nil, requestOptions{origin: fixture.origin})
	assertStatus(t, missingCapability, http.StatusUnauthorized)
	closeBody(missingCapability)

	badCapabilityOrigin := fixture.request(t, http.MethodGet, "/api/v1/bootstrap", nil, requestOptions{origin: "https://evil.example", capability: credentials.Capability})
	assertStatus(t, badCapabilityOrigin, http.StatusForbidden)
	closeBody(badCapabilityOrigin)

	bootstrap := fixture.request(t, http.MethodGet, "/api/v1/bootstrap", nil, requestOptions{origin: fixture.origin, capability: credentials.Capability})
	assertStatus(t, bootstrap, http.StatusOK)
	bootstrapBody := readBody(t, bootstrap)
	if strings.Contains(bootstrapBody, fixture.root) || strings.Contains(bootstrapBody, fixture.token) || strings.Contains(bootstrapBody, credentials.Capability) {
		t.Fatalf("bootstrap leaked private workspace or capability state: %s", bootstrapBody)
	}

	fixture.clock.Advance(capabilityTTL + time.Second)
	expired := fixture.request(t, http.MethodGet, "/api/v1/bootstrap", nil, requestOptions{origin: fixture.origin, capability: credentials.Capability})
	assertStatus(t, expired, http.StatusUnauthorized)
	closeBody(expired)
}

func TestWorkbenchRangeDigestAndCloseLifecycle(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 1, 2, 3, 4, 5, 6, 7, 8}
	fixture := newWorkbenchFixture(t, "50-production/source.png", png)
	credentials, exchanged := fixture.exchangeCredentials(t)
	resourceCookie := resourceCookieFromResponse(t, exchanged)
	closeBody(exchanged)

	bootstrap := fixture.request(t, http.MethodGet, "/api/v1/bootstrap", nil, requestOptions{origin: fixture.origin, capability: credentials.Capability})
	assertStatus(t, bootstrap, http.StatusOK)
	var snapshot Snapshot
	if err := json.NewDecoder(bootstrap.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	closeBody(bootstrap)
	if len(snapshot.Resources) != 1 || snapshot.Resources[0].MIMEType != "image/png" {
		t.Fatalf("unexpected browser resource descriptor: %#v", snapshot.Resources)
	}

	missingResourceCapability := fixture.request(t, http.MethodGet, snapshot.Resources[0].URL, nil, requestOptions{origin: fixture.origin})
	assertStatus(t, missingResourceCapability, http.StatusUnauthorized)
	closeBody(missingResourceCapability)

	ranged := fixture.request(t, http.MethodGet, snapshot.Resources[0].URL, nil, requestOptions{
		origin: fixture.origin, resourceCookie: resourceCookie.Value, rangeHeader: "bytes=2-7",
	})
	assertStatus(t, ranged, http.StatusPartialContent)
	if ranged.Header.Get("Content-Range") != "bytes 2-7/16" || !bytes.Equal([]byte(readBody(t, ranged)), png[2:8]) {
		t.Fatalf("unexpected range response: content-range=%q", ranged.Header.Get("Content-Range"))
	}

	if err := os.WriteFile(filepath.Join(fixture.root, filepath.FromSlash(fixture.ref)), append(png, 9), 0o600); err != nil {
		t.Fatal(err)
	}
	stale := fixture.request(t, http.MethodGet, snapshot.Resources[0].URL, nil, requestOptions{origin: fixture.origin, resourceCookie: resourceCookie.Value})
	assertStatus(t, stale, http.StatusConflict)
	if body := readBody(t, stale); !strings.Contains(body, "WORKSPACE_VIEW_STALE") {
		t.Fatalf("stale resource returned the wrong error: %s", body)
	}

	missingCSRF := fixture.request(t, http.MethodDelete, "/api/v1/session", strings.NewReader(`{}`), requestOptions{origin: fixture.origin, capability: credentials.Capability, idempotencyKey: "close-001"})
	assertStatus(t, missingCSRF, http.StatusForbidden)
	closeBody(missingCSRF)

	closed := fixture.request(t, http.MethodDelete, "/api/v1/session", strings.NewReader(`{}`), requestOptions{origin: fixture.origin, capability: credentials.Capability, csrf: credentials.CSRF, idempotencyKey: "close-002"})
	assertStatus(t, closed, http.StatusOK)
	closeBody(closed)
	waitForClosedSession(t, fixture.manager, fixture.root)

	reopened, err := fixture.manager.Open(context.Background(), OpenOptions{Root: fixture.root, View: "file", Ref: fixture.ref})
	if err != nil {
		t.Fatalf("reopen after browser close failed: %v", err)
	}
	if reopened.Descriptor.WorkbenchID == fixture.opened.Descriptor.WorkbenchID || reopened.Private.Origin == fixture.origin {
		t.Fatalf("browser close reused a closed workbench session: before=%#v after=%#v", fixture.opened, reopened)
	}
}

func TestWorkbenchServiceWorkerInjectsOnlyResourceCapability(t *testing.T) {
	fixture := newWorkbenchFixture(t, "50-production/local.md", []byte("local workbench\n"))
	response := fixture.request(t, http.MethodGet, "/assets/sw.js", nil, requestOptions{})
	assertStatus(t, response, http.StatusOK)
	if got := response.Header.Get("Service-Worker-Allowed"); got != "/" {
		t.Fatalf("service worker cannot control the Workbench resource scope: %q", got)
	}
	body := readBody(t, response)
	for _, expected := range []string{"/api/v1/resources/", "Authorization", "Bearer ${capability}", "workbench-capability-request", "self.clients.get(clientID)", "windows.length !== 1", "clientCapabilities.get(client.id)", "event.source.id"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("service worker is missing %q: %s", expected, body)
		}
	}
	if strings.Contains(body, "/api/v1/session/exchange") || strings.Contains(body, "X-Workbench-CSRF") {
		t.Fatalf("service worker widened its authority beyond resource reads: %s", body)
	}
	if strings.Contains(body, "let capability") {
		t.Fatalf("service worker persisted a capability across clients or worker restarts: %s", body)
	}
}

func TestWorkbenchUIKeepsTheBootstrappedViewCurrent(t *testing.T) {
	body, err := fs.ReadFile(embeddedUI, "ui/app.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(body)
	for _, expected := range []string{
		"state.query = initialBootstrap ? queryFromSnapshot(state.snapshot) : {...query}",
		"return {view: view.kind, ...(view.ref ? {ref: view.ref} : {})}",
		"window.addEventListener('hashchange'",
		"state.snapshot = null",
		"await acceptBrowserHandoff()",
		"await reloadServerView()",
		"await reloadServerView();",
		"state.pendingClaims = body.claims || {}",
		"history.state?.[historyStateKey]",
		"state.pendingClaims[state.runID] = state.claim.token",
		"persistSession();",
		"clearPersistedSession();",
		"response.status === 401",
		"renderMarkdown(view.text)",
		"renderStructured(view.data)",
		"navigator.serviceWorker.addEventListener('message'",
		"workbench-capability-response",
		"event.topic === 'session.closed'",
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("Workbench UI can lose the bootstrapped view during refresh: missing %q", expected)
		}
	}
}

func TestWorkbenchBrowserClaimProposalApplyEndToEnd(t *testing.T) {
	fixture := newWorkbenchFixture(t, "50-production/draft.md", []byte("before\n"))
	run, err := localworkspace.InitLocalRun(localworkspace.InitLocalRunOptions{Root: fixture.root, RunID: "run-browser-edit", Intent: "intent:content", Now: fixture.clock.Now()})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := fixture.manager.Open(context.Background(), OpenOptions{
		Root: fixture.root, View: "file", Ref: fixture.ref, RunID: run.RunID, ExpectedContextRevision: run.ContextRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if opened.Descriptor.WorkbenchID != fixture.opened.Descriptor.WorkbenchID || opened.Descriptor.RunID != run.RunID {
		t.Fatalf("workbench did not bind the editable view to the LocalRun: %#v", opened.Descriptor)
	}
	credentials, exchanged := fixture.exchangeCredentials(t)
	assertStatus(t, exchanged, http.StatusOK)
	closeBody(exchanged)

	bootstrap := fixture.request(t, http.MethodGet, "/api/v1/bootstrap", nil, requestOptions{origin: fixture.origin, capability: credentials.Capability})
	assertStatus(t, bootstrap, http.StatusOK)
	var snapshot Snapshot
	if err := json.NewDecoder(bootstrap.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	closeBody(bootstrap)
	if snapshot.View.RunID != run.RunID || snapshot.Ownership == nil || snapshot.Ownership.Claimed {
		t.Fatalf("bootstrap did not expose editable run ownership state: %#v", snapshot)
	}

	claimResponse := fixture.requestJSON(t, http.MethodPost, "/api/v1/ownership/claim", map[string]any{
		"run_id": run.RunID, "expected_context_revision": run.ContextRevision,
	}, requestOptions{origin: fixture.origin, capability: credentials.Capability, csrf: credentials.CSRF, idempotencyKey: "claim-browser-001"})
	assertStatus(t, claimResponse, http.StatusOK)
	var claim localworkspace.RunClaim
	if err := json.NewDecoder(claimResponse.Body).Decode(&claim); err != nil {
		t.Fatal(err)
	}
	closeBody(claimResponse)
	if claim.OwnerKind != "browser" || claim.OwnerID != fixture.opened.Descriptor.WorkbenchID || claim.Epoch == 0 || claim.Token == "" {
		t.Fatalf("browser claim is incomplete: %#v", claim)
	}

	prepareInput := map[string]any{
		"run_id": run.RunID, "claim_token": claim.Token, "owner_epoch": claim.Epoch,
		"expected_context_revision": run.ContextRevision, "typed_action": "workspace_file.replace",
		"ref": fixture.ref, "expected_digest": snapshot.View.ObservedDigest, "content": "after\n",
	}
	prepared := fixture.requestJSON(t, http.MethodPost, "/api/v1/proposals", prepareInput, requestOptions{
		origin: fixture.origin, capability: credentials.Capability, csrf: credentials.CSRF, idempotencyKey: "prepare-browser-001",
	})
	assertStatus(t, prepared, http.StatusCreated)
	var proposal localworkspace.WorkspaceProposal
	if err := json.NewDecoder(prepared.Body).Decode(&proposal); err != nil {
		t.Fatal(err)
	}
	closeBody(prepared)
	if current, err := os.ReadFile(filepath.Join(fixture.root, filepath.FromSlash(fixture.ref))); err != nil || string(current) != "before\n" {
		t.Fatalf("Proposal preparation changed the file: body=%q err=%v", current, err)
	}

	prepareReplay := fixture.requestJSON(t, http.MethodPost, "/api/v1/proposals", prepareInput, requestOptions{
		origin: fixture.origin, capability: credentials.Capability, csrf: credentials.CSRF, idempotencyKey: "prepare-browser-001",
	})
	assertStatus(t, prepareReplay, http.StatusOK)
	var replayedProposal localworkspace.WorkspaceProposal
	if err := json.NewDecoder(prepareReplay.Body).Decode(&replayedProposal); err != nil {
		t.Fatal(err)
	}
	closeBody(prepareReplay)
	if replayedProposal.ProposalID != proposal.ProposalID {
		t.Fatalf("idempotent prepare created a second Proposal: first=%s replay=%s", proposal.ProposalID, replayedProposal.ProposalID)
	}

	applyInput := map[string]any{"claim_token": claim.Token, "owner_epoch": claim.Epoch, "expected_context_revision": run.ContextRevision, "confirm": true}
	applyPath := "/api/v1/proposals/" + proposal.ProposalID + "/apply"
	applied := fixture.requestJSON(t, http.MethodPost, applyPath, applyInput, requestOptions{
		origin: fixture.origin, capability: credentials.Capability, csrf: credentials.CSRF, idempotencyKey: "apply-browser-001",
	})
	assertStatus(t, applied, http.StatusOK)
	var applyResult localworkspace.WorkspaceProposalApplyResult
	if err := json.NewDecoder(applied.Body).Decode(&applyResult); err != nil {
		t.Fatal(err)
	}
	closeBody(applied)
	if !applyResult.Applied || applyResult.ContextRevision != run.ContextRevision+1 {
		t.Fatalf("unexpected browser Apply result: %#v", applyResult)
	}
	if current, err := os.ReadFile(filepath.Join(fixture.root, filepath.FromSlash(fixture.ref))); err != nil || string(current) != "after\n" {
		t.Fatalf("browser Apply did not write the proposal: body=%q err=%v", current, err)
	}

	applyReplay := fixture.requestJSON(t, http.MethodPost, applyPath, applyInput, requestOptions{
		origin: fixture.origin, capability: credentials.Capability, csrf: credentials.CSRF, idempotencyKey: "apply-browser-001",
	})
	assertStatus(t, applyReplay, http.StatusOK)
	closeBody(applyReplay)

	conflictingApply := map[string]any{"claim_token": "different-token", "owner_epoch": claim.Epoch, "expected_context_revision": run.ContextRevision, "confirm": true}
	keyConflict := fixture.requestJSON(t, http.MethodPost, applyPath, conflictingApply, requestOptions{
		origin: fixture.origin, capability: credentials.Capability, csrf: credentials.CSRF, idempotencyKey: "apply-browser-001",
	})
	assertStatus(t, keyConflict, http.StatusConflict)
	closeBody(keyConflict)

	consumed := fixture.requestJSON(t, http.MethodPost, applyPath, applyInput, requestOptions{
		origin: fixture.origin, capability: credentials.Capability, csrf: credentials.CSRF, idempotencyKey: "apply-browser-002",
	})
	assertStatus(t, consumed, http.StatusNotFound)
	closeBody(consumed)

	closed := fixture.request(t, http.MethodDelete, "/api/v1/session", strings.NewReader(`{}`), requestOptions{origin: fixture.origin, capability: credentials.Capability, csrf: credentials.CSRF, idempotencyKey: "close-after-claim"})
	assertStatus(t, closed, http.StatusOK)
	closeBody(closed)
	waitForClosedSession(t, fixture.manager, fixture.root)
	ownership, err := localworkspace.RunClaimStatus(fixture.root, run.RunID, fixture.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if ownership.Claimed {
		t.Fatalf("closing a Workbench left its browser claim active: %#v", ownership)
	}
}

func TestWorkbenchCapabilitiesKeepIndependentViews(t *testing.T) {
	fixture := newWorkbenchFixture(t, "50-production/first.md", []byte("first\n"))
	if err := os.WriteFile(filepath.Join(fixture.root, "50-production", "second.md"), []byte("second\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstCredentials, firstExchange := fixture.exchangeCredentials(t)
	assertStatus(t, firstExchange, http.StatusOK)
	closeBody(firstExchange)
	second, err := fixture.manager.Open(context.Background(), OpenOptions{Root: fixture.root, View: "file", Ref: "50-production/second.md"})
	if err != nil {
		t.Fatal(err)
	}
	secondURL, err := url.Parse(second.Private.URL)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := url.ParseQuery(secondURL.Fragment)
	if err != nil {
		t.Fatal(err)
	}
	secondResponse := fixture.exchange(t, fragment.Get("handoff"), fixture.origin)
	assertStatus(t, secondResponse, http.StatusOK)
	var secondCredentials browserCredentials
	if err := json.NewDecoder(secondResponse.Body).Decode(&secondCredentials); err != nil {
		t.Fatal(err)
	}
	closeBody(secondResponse)
	for _, test := range []struct {
		name, capability, want string
	}{
		{name: "first", capability: firstCredentials.Capability, want: "first.md"},
		{name: "second", capability: secondCredentials.Capability, want: "second.md"},
	} {
		response := fixture.request(t, http.MethodGet, "/api/v1/bootstrap", nil, requestOptions{origin: fixture.origin, capability: test.capability})
		assertStatus(t, response, http.StatusOK)
		var snapshot Snapshot
		if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
			t.Fatal(err)
		}
		closeBody(response)
		if snapshot.View.View.Ref != "50-production/"+test.want {
			t.Fatalf("%s capability view was overwritten: got %q", test.name, snapshot.View.View.Ref)
		}
	}
}

type workbenchFixture struct {
	t          *testing.T
	root       string
	ref        string
	clock      *testClock
	manager    *Manager
	opened     OpenResult
	origin     string
	token      string
	httpClient *http.Client
}

func newWorkbenchFixture(t *testing.T, ref string, body []byte) *workbenchFixture {
	t.Helper()
	root := filepath.Join(t.TempDir(), "project")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "project-workbench", WorkspaceID: "workspace-workbench", CLIVersion: "test", Target: "none"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(ref)), body, 0o600); err != nil {
		t.Fatal(err)
	}
	clock := &testClock{value: time.Date(2026, 8, 14, 6, 0, 0, 0, time.UTC)}
	manager := NewManager(clock.Now)
	opened, err := manager.Open(context.Background(), OpenOptions{Root: root, View: "file", Ref: ref})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(opened.Private.URL)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := url.ParseQuery(parsed.Fragment)
	if err != nil || fragment.Get("handoff") == "" {
		t.Fatalf("invalid private handoff URL: %s", opened.Private.URL)
	}
	fixture := &workbenchFixture{
		t: t, root: root, ref: ref, clock: clock, manager: manager, opened: opened,
		origin: opened.Private.Origin, token: fragment.Get("handoff"), httpClient: &http.Client{Timeout: 3 * time.Second},
	}
	t.Cleanup(func() { _ = manager.Close() })
	return fixture
}

type requestOptions struct {
	origin         string
	host           string
	capability     string
	csrf           string
	rangeHeader    string
	idempotencyKey string
	resourceCookie string
}

func (f *workbenchFixture) request(t *testing.T, method, target string, body io.Reader, options requestOptions) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, f.origin+target, body)
	if err != nil {
		t.Fatal(err)
	}
	if options.origin != "" {
		request.Header.Set("Origin", options.origin)
		request.Header.Set("Sec-Fetch-Site", "same-origin")
	}
	if options.host != "" {
		request.Host = options.host
	}
	if options.capability != "" {
		request.Header.Set("Authorization", "Bearer "+options.capability)
	}
	if options.resourceCookie != "" {
		request.AddCookie(&http.Cookie{Name: resourceCookieName, Value: options.resourceCookie})
	}
	if options.csrf != "" {
		request.Header.Set("X-Workbench-CSRF", options.csrf)
	}
	if options.rangeHeader != "" {
		request.Header.Set("Range", options.rangeHeader)
	}
	if method != http.MethodGet && method != http.MethodHead {
		request.Header.Set("Content-Type", "application/json")
	}
	if options.idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", options.idempotencyKey)
	}
	response, err := f.httpClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func (f *workbenchFixture) requestJSON(t *testing.T, method, target string, value any, options requestOptions) *http.Response {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return f.request(t, method, target, bytes.NewReader(body), options)
}

func (f *workbenchFixture) exchange(t *testing.T, token, origin string) *http.Response {
	t.Helper()
	body, err := json.Marshal(map[string]string{"token": token})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, f.origin+"/api/v1/session/exchange", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if origin != "" {
		request.Header.Set("Origin", origin)
		request.Header.Set("Sec-Fetch-Site", "same-origin")
	}
	response, err := f.httpClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func (f *workbenchFixture) exchangeCredentials(t *testing.T) (browserCredentials, *http.Response) {
	t.Helper()
	response := f.exchange(t, f.token, f.origin)
	var credentials browserCredentials
	if response.StatusCode == http.StatusOK {
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		response.Body = io.NopCloser(bytes.NewReader(body))
		if err := json.Unmarshal(body, &credentials); err != nil {
			t.Fatal(err)
		}
		if cookie := resourceCookieFromResponse(t, response); strings.Contains(string(body), cookie.Value) {
			t.Fatal("resource capability leaked into the exchange JSON body")
		}
	}
	return credentials, response
}

func resourceCookieFromResponse(t *testing.T, response *http.Response) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Cookies() {
		if cookie.Name == resourceCookieName && cookie.Value != "" {
			return cookie
		}
	}
	t.Fatal("exchange response is missing the resource-only session cookie")
	return nil
}

func assertStatus(t *testing.T, response *http.Response, expected int) {
	t.Helper()
	if response.StatusCode != expected {
		body := readBody(t, response)
		t.Fatalf("unexpected HTTP status: got=%d want=%d body=%s", response.StatusCode, expected, body)
	}
}

func readBody(t *testing.T, response *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	return string(body)
}

func closeBody(response *http.Response) {
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
}

func waitForClosedSession(t *testing.T, manager *Manager, root string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := manager.Status(root); err != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("closed browser session remained ready in the manager")
}
