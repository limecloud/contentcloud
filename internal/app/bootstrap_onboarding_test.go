package app

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestBootstrapAuthorizationRequiresApprovalAndMatchingVerifier(t *testing.T) {
	service, actor, connect := bootstrapFixture(t)
	verifier := bootstrapTestVerifier("matching-verifier")
	started, err := service.StartBootstrapAuthorization(t.Context(), "https://content.example.com", StartBootstrapAuthorizationInput{SessionID: connect.ID, CodeChallenge: bootstrapCodeChallenge(verifier), Platform: "darwin", Arch: "arm64", CLIVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if started.VerificationURL != "https://content.example.com/studio/connect?session="+connect.ID {
		t.Fatalf("bootstrap verification URL did not use the customer connection route: %q", started.VerificationURL)
	}
	_, err = service.CompleteBootstrapAuthorization(t.Context(), CompleteBootstrapAuthorizationInput{AttemptToken: started.AttemptToken, CodeVerifier: verifier, Device: ConnectDeviceInput{MachineID: bootstrapTestMachineID("test-mac"), Hostname: "test-mac"}})
	assertBootstrapError(t, err, "BOOTSTRAP_AUTHORIZATION_PENDING")
	if _, err := service.ApproveBootstrapAuthorization(t.Context(), actor, connect.ID, started.AttemptID, "approve"); err != nil {
		t.Fatal(err)
	}
	_, err = service.CompleteBootstrapAuthorization(t.Context(), CompleteBootstrapAuthorizationInput{AttemptToken: started.AttemptToken, CodeVerifier: bootstrapTestVerifier("wrong-verifier"), Device: ConnectDeviceInput{MachineID: bootstrapTestMachineID("test-mac"), Hostname: "test-mac"}})
	assertBootstrapError(t, err, "BOOTSTRAP_VERIFIER_INVALID")
	connected, err := service.CompleteBootstrapAuthorization(t.Context(), CompleteBootstrapAuthorizationInput{AttemptToken: started.AttemptToken, CodeVerifier: verifier, Device: ConnectDeviceInput{MachineID: bootstrapTestMachineID("test-mac"), Hostname: "test-mac", Platform: "darwin", Arch: "arm64"}})
	if err != nil || connected.ProjectID != connect.ProjectID || connected.WorkspaceToken == "" || connected.DeviceToken == "" {
		t.Fatalf("complete authorization failed: result=%#v error=%v", connected, err)
	}
	if connected.Device.Capabilities == nil || len(connected.Device.Capabilities) != 0 {
		t.Fatalf("missing device capabilities must normalize to an empty array: %#v", connected.Device.Capabilities)
	}
	workspaceActor, binding, err := service.WorkspaceActor(t.Context(), connected.WorkspaceToken)
	if err != nil {
		t.Fatal(err)
	}
	summary := domain.BootstrapDiagnosticSummary{
		SchemaVersion: domain.BootstrapSchemaVersion,
		AttemptID:     started.AttemptID,
		Platform:      "darwin",
		Arch:          "arm64",
		Versions:      map[string]string{"contentcloud_cli": "0.8.0"},
		Checks:        []domain.BootstrapDiagnosticCheck{{CheckID: "runtime.node.version", Status: "passed"}},
	}
	diagnostic, err := service.UploadBootstrapDiagnostic(t.Context(), workspaceActor, binding, summary)
	if err != nil || diagnostic.AttemptID != started.AttemptID || diagnostic.SupportCode != started.SupportCode || diagnostic.Digest == "" {
		t.Fatalf("diagnostic upload failed: result=%#v error=%v", diagnostic, err)
	}
	replayed, err := service.UploadBootstrapDiagnostic(t.Context(), workspaceActor, binding, summary)
	if err != nil || replayed.ID != diagnostic.ID || !replayed.CreatedAt.Equal(diagnostic.CreatedAt) {
		t.Fatalf("diagnostic replay was not idempotent: first=%#v replayed=%#v error=%v", diagnostic, replayed, err)
	}
}

func TestBootstrapAuthorizationRejectsMalformedSessionID(t *testing.T) {
	service, _, _ := bootstrapFixture(t)
	_, err := service.StartBootstrapAuthorization(t.Context(), "https://content.example.com", StartBootstrapAuthorizationInput{
		SessionID: "not-a-uuid", CodeChallenge: bootstrapCodeChallenge(bootstrapTestVerifier("invalid-session")),
	})
	assertBootstrapError(t, err, "BOOTSTRAP_AUTHORIZATION_INPUT_INVALID")
}

func TestBootstrapAuthorizationAllowsOnlyOneActiveAttemptPerSession(t *testing.T) {
	service, _, connect := bootstrapFixture(t)
	firstVerifier := bootstrapTestVerifier("first-attempt")
	if _, err := service.StartBootstrapAuthorization(t.Context(), "https://content.example.com", StartBootstrapAuthorizationInput{SessionID: connect.ID, CodeChallenge: bootstrapCodeChallenge(firstVerifier)}); err != nil {
		t.Fatal(err)
	}
	secondVerifier := bootstrapTestVerifier("second-attempt")
	_, err := service.StartBootstrapAuthorization(t.Context(), "https://content.example.com", StartBootstrapAuthorizationInput{SessionID: connect.ID, CodeChallenge: bootstrapCodeChallenge(secondVerifier)})
	assertBootstrapError(t, err, "BOOTSTRAP_AUTHORIZATION_ALREADY_STARTED")
}

func TestBootstrapAuthorizationDenialAndExpiryAreDistinct(t *testing.T) {
	service, actor, deniedConnect := bootstrapFixture(t)
	verifier := bootstrapTestVerifier("denied-verifier")
	denied, err := service.StartBootstrapAuthorization(t.Context(), "https://content.example.com", StartBootstrapAuthorizationInput{SessionID: deniedConnect.ID, CodeChallenge: bootstrapCodeChallenge(verifier)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.DenyBootstrapAuthorization(t.Context(), actor, deniedConnect.ID, denied.AttemptID, "deny"); err != nil {
		t.Fatal(err)
	}
	deniedStatus, err := service.ConnectSession(t.Context(), actor, deniedConnect.ID)
	if err != nil || deniedStatus.State != "canceled" {
		t.Fatalf("denied authorization did not cancel ConnectSession: status=%#v error=%v", deniedStatus, err)
	}
	_, err = service.CompleteBootstrapAuthorization(t.Context(), CompleteBootstrapAuthorizationInput{AttemptToken: denied.AttemptToken, CodeVerifier: verifier, Device: ConnectDeviceInput{MachineID: bootstrapTestMachineID("test-mac"), Hostname: "test-mac"}})
	assertBootstrapError(t, err, "BOOTSTRAP_AUTHORIZATION_DENIED")

	expiredConnect, err := service.CreateConnectSession(t.Context(), actor, deniedConnect.ProjectID, "expired-connect")
	if err != nil {
		t.Fatal(err)
	}
	expired, err := service.StartBootstrapAuthorization(t.Context(), "https://content.example.com", StartBootstrapAuthorizationInput{SessionID: expiredConnect.ID, CodeChallenge: bootstrapCodeChallenge(verifier)})
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return expired.ExpiresAt.Add(time.Second) }
	expiredStatus, statusErr := service.ConnectSession(t.Context(), actor, expiredConnect.ID)
	if statusErr != nil || expiredStatus.State != "expired" || expiredStatus.Progress != nil {
		t.Fatalf("expired ConnectSession was not projected: status=%#v error=%v", expiredStatus, statusErr)
	}
	_, err = service.CompleteBootstrapAuthorization(t.Context(), CompleteBootstrapAuthorizationInput{AttemptToken: expired.AttemptToken, CodeVerifier: verifier, Device: ConnectDeviceInput{MachineID: bootstrapTestMachineID("test-mac"), Hostname: "test-mac"}})
	assertBootstrapError(t, err, "BOOTSTRAP_AUTHORIZATION_EXPIRED")
}

func TestBootstrapProgressSequenceIsIdempotentAndProjected(t *testing.T) {
	service, actor, connect := bootstrapFixture(t)
	verifier := bootstrapTestVerifier("progress-verifier")
	started, err := service.StartBootstrapAuthorization(t.Context(), "https://content.example.com", StartBootstrapAuthorizationInput{SessionID: connect.ID, CodeChallenge: bootstrapCodeChallenge(verifier)})
	if err != nil {
		t.Fatal(err)
	}
	event := domain.BootstrapProgressEvent{SchemaVersion: domain.BootstrapSchemaVersion, Sequence: 2, OccurredAt: service.now().UTC(), Stage: "plugin_installing", Status: "started", Facts: map[string]any{}}
	first, err := service.AppendBootstrapProgress(t.Context(), started.AttemptToken, event)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := service.AppendBootstrapProgress(t.Context(), started.AttemptToken, event)
	if err != nil || replayed.AttemptID != first.AttemptID {
		t.Fatalf("idempotent replay failed: event=%#v error=%v", replayed, err)
	}
	conflict := event
	conflict.Status = "failed"
	_, err = service.AppendBootstrapProgress(t.Context(), started.AttemptToken, conflict)
	assertBootstrapError(t, err, "BOOTSTRAP_PROGRESS_SEQUENCE_CONFLICT")
	gap := event
	gap.Sequence = 4
	_, err = service.AppendBootstrapProgress(t.Context(), started.AttemptToken, gap)
	assertBootstrapError(t, err, "BOOTSTRAP_PROGRESS_SEQUENCE_GAP")
	status, err := service.ConnectSession(t.Context(), actor, connect.ID)
	if err != nil || status.Progress == nil || status.Progress.AttemptID != started.AttemptID || status.Progress.Stage != "plugin_installing" {
		t.Fatalf("progress projection mismatch: status=%#v error=%v", status, err)
	}
}

func TestBootstrapAttemptCannotCompleteBeforeAuthorizationIsConsumed(t *testing.T) {
	service, actor, connect := bootstrapFixture(t)
	verifier := bootstrapTestVerifier("attempt-state-verifier")
	started, err := service.StartBootstrapAuthorization(t.Context(), "https://content.example.com", StartBootstrapAuthorizationInput{SessionID: connect.ID, CodeChallenge: bootstrapCodeChallenge(verifier)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.CompleteBootstrapAttempt(t.Context(), started.AttemptToken, "completed")
	assertBootstrapError(t, err, "BOOTSTRAP_ATTEMPT_STATE_INVALID")
	if _, err := service.ApproveBootstrapAuthorization(t.Context(), actor, connect.ID, started.AttemptID, "approve"); err != nil {
		t.Fatal(err)
	}
	_, err = service.CompleteBootstrapAttempt(t.Context(), started.AttemptToken, "completed")
	assertBootstrapError(t, err, "BOOTSTRAP_ATTEMPT_STATE_INVALID")
	if _, err := service.CompleteBootstrapAuthorization(t.Context(), CompleteBootstrapAuthorizationInput{AttemptToken: started.AttemptToken, CodeVerifier: verifier, Device: ConnectDeviceInput{MachineID: bootstrapTestMachineID("test-mac"), Hostname: "test-mac"}}); err != nil {
		t.Fatal(err)
	}
	completed, err := service.CompleteBootstrapAttempt(t.Context(), started.AttemptToken, "completed")
	if err != nil || completed.State != "completed" || completed.CompletedAt == nil {
		t.Fatalf("consumed attempt did not complete: attempt=%#v error=%v", completed, err)
	}
	replayed, err := service.CompleteBootstrapAttempt(t.Context(), started.AttemptToken, "completed")
	if err != nil || replayed.State != "completed" {
		t.Fatalf("terminal replay was not idempotent: attempt=%#v error=%v", replayed, err)
	}
	_, err = service.CompleteBootstrapAttempt(t.Context(), started.AttemptToken, "failed")
	assertBootstrapError(t, err, "BOOTSTRAP_ATTEMPT_STATE_INVALID")
	_, err = service.AppendBootstrapProgress(t.Context(), started.AttemptToken, domain.BootstrapProgressEvent{SchemaVersion: domain.BootstrapSchemaVersion, Sequence: 2, OccurredAt: service.now().UTC(), Stage: "complete", Status: "failed", Facts: map[string]any{}})
	assertBootstrapError(t, err, "BOOTSTRAP_PROGRESS_TERMINAL")
}

func TestBootstrapReconnectReusesStableDeviceAndRotatesCredential(t *testing.T) {
	service, actor, firstConnect := bootstrapFixture(t)
	machineID := bootstrapTestMachineID("stable-device")
	firstVerifier := bootstrapTestVerifier("stable-device-first")
	firstAuthorization, err := service.StartBootstrapAuthorization(t.Context(), "https://content.example.com", StartBootstrapAuthorizationInput{SessionID: firstConnect.ID, CodeChallenge: bootstrapCodeChallenge(firstVerifier)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ApproveBootstrapAuthorization(t.Context(), actor, firstConnect.ID, firstAuthorization.AttemptID, "approve-first"); err != nil {
		t.Fatal(err)
	}
	first, err := service.CompleteBootstrapAuthorization(t.Context(), CompleteBootstrapAuthorizationInput{AttemptToken: firstAuthorization.AttemptToken, CodeVerifier: firstVerifier, Device: ConnectDeviceInput{MachineID: machineID, Hostname: "old-host", Version: "1.0.0"}})
	if err != nil {
		t.Fatal(err)
	}
	secondConnect, err := service.CreateConnectSession(t.Context(), actor, firstConnect.ProjectID, "stable-device-second-connect")
	if err != nil {
		t.Fatal(err)
	}
	secondVerifier := bootstrapTestVerifier("stable-device-second")
	secondAuthorization, err := service.StartBootstrapAuthorization(t.Context(), "https://content.example.com", StartBootstrapAuthorizationInput{SessionID: secondConnect.ID, CodeChallenge: bootstrapCodeChallenge(secondVerifier)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ApproveBootstrapAuthorization(t.Context(), actor, secondConnect.ID, secondAuthorization.AttemptID, "approve-second"); err != nil {
		t.Fatal(err)
	}
	second, err := service.CompleteBootstrapAuthorization(t.Context(), CompleteBootstrapAuthorizationInput{AttemptToken: secondAuthorization.AttemptToken, CodeVerifier: secondVerifier, Device: ConnectDeviceInput{MachineID: machineID, Hostname: "new-host", Version: "2.0.0"}})
	if err != nil {
		t.Fatal(err)
	}
	if second.Device.ID != first.Device.ID || second.WorkspaceID != first.WorkspaceID {
		t.Fatalf("stable machine reconnect created duplicate identity: first=%#v second=%#v", first, second)
	}
	if second.Device.CredentialVersion != first.Device.CredentialVersion+1 || second.Device.Hostname != "new-host" {
		t.Fatalf("reconnect did not rotate credential and refresh metadata: %#v", second.Device)
	}
	if _, _, err := service.DeviceActor(t.Context(), first.DeviceToken); !hasAppDomainCode(err, "DEVICE_TOKEN_INVALID") {
		t.Fatalf("old device token remained valid after reconnect rotation: %v", err)
	}
	if current, _, err := service.DeviceActor(t.Context(), second.DeviceToken); err != nil || current.DeviceID != first.Device.ID {
		t.Fatalf("rotated device token is not bound to stable identity: actor=%#v err=%v", current, err)
	}
}

func bootstrapTestMachineID(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return "mach_" + base64.RawURLEncoding.EncodeToString(sum[:24])
}

func bootstrapFixture(t *testing.T) (*Service, Actor, domain.ConnectSession) {
	t.Helper()
	service := New(memory.New(), slog.Default())
	now := time.Now().UTC().Truncate(time.Second)
	service.now = func() time.Time { return now }
	session, err := service.Register(t.Context(), domain.NewID()+"@example.com", "long-enough-password", "Owner", "Tenant")
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(t.Context(), actor, CreateProjectInput{BrandName: "Brand", ProductName: "Product", Channel: "douyin"}, "project")
	if err != nil {
		t.Fatal(err)
	}
	connect, err := service.CreateConnectSession(t.Context(), actor, project.ID, "connect")
	if err != nil {
		t.Fatal(err)
	}
	return service, actor, connect
}

func bootstrapTestVerifier(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func assertBootstrapError(t *testing.T, err error, code string) {
	t.Helper()
	var domainError *domain.Error
	if !errors.As(err, &domainError) || domainError.Code != code {
		t.Fatalf("error = %#v, want code %s", err, code)
	}
}
