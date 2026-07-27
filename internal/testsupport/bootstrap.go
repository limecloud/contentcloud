package testsupport

import (
	"context"
	"crypto/sha256"
	"encoding/base64"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
)

func ConnectBootstrap(ctx context.Context, service *app.Service, actor app.Actor, session domain.ConnectSession, device app.ConnectDeviceInput) (app.ConnectDeviceResult, error) {
	verifier := base64.RawURLEncoding.EncodeToString([]byte("contentcloud-test-verifier-32byte"))
	sum := sha256.Sum256([]byte(verifier))
	started, err := service.StartBootstrapAuthorization(ctx, "https://contentcloud.test", app.StartBootstrapAuthorizationInput{
		SessionID: session.ID, CodeChallenge: base64.RawURLEncoding.EncodeToString(sum[:]),
		Platform: device.Platform, Arch: device.Arch, CLIVersion: device.Version,
	})
	if err != nil {
		return app.ConnectDeviceResult{}, err
	}
	if _, err := service.ApproveBootstrapAuthorization(ctx, actor, session.ID, started.AttemptID, "test-bootstrap-approve"); err != nil {
		return app.ConnectDeviceResult{}, err
	}
	return service.CompleteBootstrapAuthorization(ctx, app.CompleteBootstrapAuthorizationInput{AttemptToken: started.AttemptToken, CodeVerifier: verifier, Device: device})
}
