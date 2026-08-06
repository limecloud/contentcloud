package environment

import (
	"crypto/ed25519"
	"encoding/json"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

type ControlPlane struct {
	issuer   *Issuer
	profile  Profile
	registry VerifiedRegistry
	ttl      time.Duration
	resolver *Resolver
}

func NewControlPlane(issuer *Issuer, profile Profile, registry VerifiedRegistry, ttl time.Duration) (*ControlPlane, error) {
	if issuer == nil || ttl < time.Minute || ttl > 30*24*time.Hour {
		return nil, domain.Invalid("ENVIRONMENT_CONTROL_PLANE_INVALID", "环境控制面需要签发器，以及 1 分钟至 30 天的有效期")
	}
	publicKey, ok := issuer.privateKey.Public().(ed25519.PublicKey)
	if !ok {
		return nil, domain.Invalid("ENVIRONMENT_SIGNING_KEY_INVALID", "环境签发器无法导出 Ed25519 公钥")
	}
	verifier, err := NewVerifier([]TrustedKey{{KeyID: issuer.keyID, Status: "active", PublicKey: publicKey}})
	if err != nil {
		return nil, err
	}
	return NewControlPlaneWithVerifier(issuer, verifier, profile, registry, ttl)
}

func NewControlPlaneWithVerifier(issuer *Issuer, verifier *Verifier, profile Profile, registry VerifiedRegistry, ttl time.Duration) (*ControlPlane, error) {
	if issuer == nil || verifier == nil || ttl < time.Minute || ttl > 30*24*time.Hour {
		return nil, domain.Invalid("ENVIRONMENT_CONTROL_PLANE_INVALID", "环境控制面需要签发器、可信校验器，以及 1 分钟至 30 天的有效期")
	}
	profileCopy, err := clone(profile)
	if err != nil {
		return nil, err
	}
	validationTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	manifest, err := BuildManifest("control-plane-validation", []string{domain.ContentTypeVideoScript}, profileCopy, registry, validationTime, validationTime.Add(ttl))
	if err != nil {
		return nil, err
	}
	signedManifest, err := issuer.Sign(manifest)
	if err != nil {
		return nil, err
	}
	if err := verifier.Verify(signedManifest, VerifyOptions{ProjectID: "control-plane-validation", ProfileID: profileCopy.ID, Harness: profileCopy.Harness, Now: validationTime}); err != nil {
		return nil, err
	}
	resolver, err := NewResolver(verifier)
	if err != nil {
		return nil, err
	}
	return &ControlPlane{issuer: issuer, profile: profileCopy, registry: registry, ttl: ttl, resolver: resolver}, nil
}

func (controlPlane *ControlPlane) Issue(projectID string, contentTypes []string, now time.Time) (Manifest, error) {
	if controlPlane == nil {
		return Manifest{}, domain.Conflict("ENVIRONMENT_CONTROL_PLANE_UNAVAILABLE", "尚未配置环境控制面")
	}
	now = now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	manifest, err := BuildManifest(projectID, contentTypes, controlPlane.profile, controlPlane.registry, now, now.Add(controlPlane.ttl))
	if err != nil {
		return Manifest{}, err
	}
	return controlPlane.issuer.Sign(manifest)
}

func (controlPlane *ControlPlane) IssueExecutionBundle(request ExecutionBundleRequest, now time.Time) (CreativeExecutionBundle, error) {
	if controlPlane == nil {
		return CreativeExecutionBundle{}, domain.Conflict("ENVIRONMENT_CONTROL_PLANE_UNAVAILABLE", "尚未配置环境控制面")
	}
	now = now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	manifest, err := BuildManifest(request.ProjectID, request.ContentTypes, controlPlane.profile, controlPlane.registry, now, now.Add(controlPlane.ttl))
	if err != nil {
		return CreativeExecutionBundle{}, err
	}
	bundle, err := BuildExecutionBundle(manifest, request.Subject, request.RequiredCapabilities, request.PackIDs, now, now.Add(controlPlane.ttl))
	if err != nil {
		return CreativeExecutionBundle{}, err
	}
	return controlPlane.issuer.SignBundle(bundle)
}

func (controlPlane *ControlPlane) Registry() (Registry, error) {
	if controlPlane == nil {
		return Registry{}, domain.Conflict("ENVIRONMENT_CONTROL_PLANE_UNAVAILABLE", "尚未配置环境控制面")
	}
	registry, err := clone(controlPlane.registry.raw())
	if err != nil {
		return Registry{}, err
	}
	return registry, nil
}

func (controlPlane *ControlPlane) ResolveExecutionBundle(bundle CreativeExecutionBundle, manifest Manifest, lock EnvironmentLock, capabilities []domain.Capability, options BundleVerifyOptions) (BundleResolution, error) {
	if controlPlane == nil || controlPlane.resolver == nil {
		return BundleResolution{}, domain.Conflict("ENVIRONMENT_CONTROL_PLANE_UNAVAILABLE", "尚未配置环境控制面")
	}
	return controlPlane.resolver.ResolveBundle(bundle, manifest, controlPlane.registry, lock, capabilities, options)
}

func clone[T any](value T) (T, error) {
	var result T
	body, err := json.Marshal(value)
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return result, err
	}
	return result, nil
}
