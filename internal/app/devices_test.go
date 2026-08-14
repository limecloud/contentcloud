package app

import (
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func TestRotateDeviceCredentialInvalidatesPreviousToken(t *testing.T) {
	service, actor, connect := bootstrapFixture(t)
	verifier := bootstrapTestVerifier("rotate-device")
	authorization, err := service.StartBootstrapAuthorization(t.Context(), "https://content.example.com", StartBootstrapAuthorizationInput{SessionID: connect.ID, CodeChallenge: bootstrapCodeChallenge(verifier)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ApproveBootstrapAuthorization(t.Context(), actor, connect.ID, authorization.AttemptID, "approve-rotate"); err != nil {
		t.Fatal(err)
	}
	connected, err := service.CompleteBootstrapAuthorization(t.Context(), CompleteBootstrapAuthorizationInput{AttemptToken: authorization.AttemptToken, CodeVerifier: verifier, Device: ConnectDeviceInput{MachineID: bootstrapTestMachineID("rotate-device"), Hostname: "rotate-device"}})
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := service.RotateDeviceCredential(t.Context(), actor, connected.Device.ID, "rotate-request")
	if err != nil {
		t.Fatal(err)
	}
	if rotated.Device.CredentialVersion != connected.Device.CredentialVersion+1 || rotated.DeviceToken == "" || rotated.DeviceToken == connected.DeviceToken {
		t.Fatalf("credential rotation did not return a new version/token: %#v", rotated)
	}
	if _, _, err := service.DeviceActor(t.Context(), connected.DeviceToken); !hasAppDomainCode(err, "DEVICE_TOKEN_INVALID") {
		t.Fatalf("old token remained valid after explicit rotation: %v", err)
	}
	if current, _, err := service.DeviceActor(t.Context(), rotated.DeviceToken); err != nil || current.DeviceID != connected.Device.ID {
		t.Fatalf("new token was not accepted: actor=%#v err=%v", current, err)
	}
}

func TestDaemonInstanceRejectsDuplicateAndSameEpochResurrection(t *testing.T) {
	service, actor, connect := bootstrapFixture(t)
	verifier := bootstrapTestVerifier("daemon-fencing")
	authorization, err := service.StartBootstrapAuthorization(t.Context(), "https://content.example.com", StartBootstrapAuthorizationInput{SessionID: connect.ID, CodeChallenge: bootstrapCodeChallenge(verifier)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ApproveBootstrapAuthorization(t.Context(), actor, connect.ID, authorization.AttemptID, "approve-daemon-fencing"); err != nil {
		t.Fatal(err)
	}
	connected, err := service.CompleteBootstrapAuthorization(t.Context(), CompleteBootstrapAuthorizationInput{AttemptToken: authorization.AttemptToken, CodeVerifier: verifier, Device: ConnectDeviceInput{MachineID: bootstrapTestMachineID("daemon-fencing"), Hostname: "daemon-fencing"}})
	if err != nil {
		t.Fatal(err)
	}
	deviceActor, _, err := service.DeviceActor(t.Context(), connected.DeviceToken)
	if err != nil {
		t.Fatal(err)
	}
	startedAt := time.Now().UTC().Add(-time.Minute)
	input := DaemonInstanceReportInput{ID: domain.NewID(), ConnectionEpoch: 1, ReportSequence: 1, PID: 42, Version: "test", State: "connected", StartedAt: startedAt}
	if _, err := service.ReportDaemonInstance(t.Context(), deviceActor, input); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReportDaemonInstance(t.Context(), deviceActor, input); !hasAppDomainCode(err, "DAEMON_INSTANCE_REPORT_STALE") {
		t.Fatalf("duplicate daemon report was accepted: %v", err)
	}
	input.ReportSequence = 2
	input.State = "stopped"
	if _, err := service.ReportDaemonInstance(t.Context(), deviceActor, input); err != nil {
		t.Fatal(err)
	}
	input.ReportSequence = 3
	input.State = "connected"
	if _, err := service.ReportDaemonInstance(t.Context(), deviceActor, input); !hasAppDomainCode(err, "DAEMON_INSTANCE_REPORT_STALE") {
		t.Fatalf("stopped daemon instance resurrected without a new epoch: %v", err)
	}
	input.ConnectionEpoch = 2
	input.ReportSequence = 1
	if _, err := service.ReportDaemonInstance(t.Context(), deviceActor, input); err != nil {
		t.Fatalf("new daemon connection epoch was rejected: %v", err)
	}
}
