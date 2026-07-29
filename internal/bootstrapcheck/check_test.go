package bootstrapcheck_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/limecloud/contentcloud/internal/bootstrapcheck"
)

type runnerResponse struct {
	output string
	err    error
}

type fakeRunner map[string]runnerResponse

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func (r fakeRunner) Run(_ context.Context, name string, _ ...string) (string, error) {
	response, ok := r[name]
	if !ok {
		return "", errors.New("command unavailable")
	}
	return response.output, response.err
}

func TestPreflightReportsStableNodeFailures(t *testing.T) {
	tests := []struct {
		name      string
		node      runnerResponse
		checkID   string
		errorCode string
	}{
		{name: "missing", node: runnerResponse{err: errors.New("not found")}, checkID: "runtime.node.available", errorCode: "NODE_NOT_FOUND"},
		{name: "unsupported", node: runnerResponse{output: "v18.20.0"}, checkID: "runtime.node.version", errorCode: "NODE_VERSION_UNSUPPORTED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := healthyRunner()
			runner["node"] = test.node
			report := runOffline(t, runner)
			if report.OK || report.FirstFailure == nil || report.FirstFailure.CheckID != test.checkID || report.FirstFailure.ErrorCode != test.errorCode {
				t.Fatalf("unexpected first failure: %#v", report.FirstFailure)
			}
		})
	}
}

func TestPreflightReportsCodexCLIAndDesktopFailures(t *testing.T) {
	tests := []struct {
		name      string
		command   string
		response  runnerResponse
		checkID   string
		errorCode string
	}{
		{name: "cli missing", command: "codex", response: runnerResponse{err: errors.New("not found")}, checkID: "codex.cli.available", errorCode: "CODEX_CLI_NOT_FOUND"},
		{name: "desktop missing", command: "open", response: runnerResponse{err: errors.New("not found")}, checkID: "codex.desktop.available", errorCode: "CODEX_DESKTOP_NOT_FOUND"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := healthyRunner()
			runner[test.command] = test.response
			report := runOffline(t, runner)
			if report.FirstFailure == nil || report.FirstFailure.CheckID != test.checkID || report.FirstFailure.ErrorCode != test.errorCode {
				t.Fatalf("unexpected first failure: %#v", report.FirstFailure)
			}
		})
	}
}

func TestPreflightRejectsUnavailableMacOSKeychain(t *testing.T) {
	runner := healthyRunner()
	runner["security"] = runnerResponse{err: errors.New("default keychain unavailable")}
	report := runOffline(t, runner)
	if report.OK || report.FirstFailure == nil || report.FirstFailure.CheckID != "runtime.credential_store.available" || report.FirstFailure.ErrorCode != "MACOS_KEYCHAIN_UNAVAILABLE" {
		t.Fatalf("unexpected keychain failure: %#v", report.FirstFailure)
	}
}

func TestOfflinePreflightSkipsEveryNetworkCheck(t *testing.T) {
	report := runOffline(t, healthyRunner())
	if !report.OK {
		t.Fatalf("healthy offline report failed: %#v", report.FirstFailure)
	}
	networkChecks := 0
	for _, check := range report.Checks {
		if check.Stage != "network_ready" {
			continue
		}
		networkChecks++
		if check.Status != "skipped" {
			t.Fatalf("network check was not skipped: %#v", check)
		}
	}
	if networkChecks != 4 {
		t.Fatalf("network check count = %d, want 4", networkChecks)
	}
	if err := bootstrapcheck.ValidateReport(report); err != nil {
		t.Fatalf("offline report schema is invalid: %v", err)
	}
}

func TestOnlinePreflightDoesNotProbeOpenAIDocumentation(t *testing.T) {
	openAIRequested := false
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "developers.openai.com" {
			openAIRequested = true
			return nil, errors.New("OpenAI documentation unavailable")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header), Request: request}, nil
	})}
	report := bootstrapcheck.Run(t.Context(), bootstrapcheck.Options{
		Directory:  t.TempDir(),
		ServerURL:  "https://content.example.com",
		Platform:   "darwin",
		Arch:       "arm64",
		Runner:     healthyRunner(),
		HTTPClient: client,
	})
	if openAIRequested {
		t.Fatal("preflight used the OpenAI documentation site as a Codex readiness probe")
	}
	if !report.OK || report.FirstFailure != nil {
		t.Fatalf("healthy online report failed: %#v", report.FirstFailure)
	}
	for _, check := range report.Checks {
		if check.CheckID == "network.openai.reachable" {
			if check.Status != "skipped" || check.ActionID != "guide.network.openai" {
				t.Fatalf("unexpected OpenAI network check: %#v", check)
			}
			return
		}
	}
	t.Fatal("OpenAI network check is missing")
}

func healthyRunner() fakeRunner {
	return fakeRunner{
		"node":     {output: "v20.10.0"},
		"npx":      {output: "10.2.3"},
		"codex":    {output: "codex-cli 0.145.0"},
		"open":     {},
		"security": {output: `"/Users/test/Library/Keychains/login.keychain-db"`},
	}
}

func runOffline(t *testing.T, runner fakeRunner) bootstrapcheck.Report {
	t.Helper()
	return bootstrapcheck.Run(t.Context(), bootstrapcheck.Options{Directory: t.TempDir(), ServerURL: "https://content.example.com", Offline: true, Platform: "darwin", Arch: "arm64", Runner: runner})
}
