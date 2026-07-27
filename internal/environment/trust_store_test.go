package environment_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/environment"
)

func TestTrustedKeySetBuildsManifestAndRegistryVerifiers(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(fmt.Sprintf(`{"$schema":"test","schema_version":"1.0","keys":[{"key_id":"release-test","algorithm":"ed25519","status":"active","public_key":"%s"}]}`, base64.StdEncoding.EncodeToString(publicKey)))
	if _, err := environment.ManifestVerifierJSON(body); err != nil {
		t.Fatalf("manifest verifier: %v", err)
	}
	if _, err := environment.RegistryVerifierJSON(body); err != nil {
		t.Fatalf("registry verifier: %v", err)
	}
}

func TestReleaseRegistryAndProfileUseBuiltInTrust(t *testing.T) {
	root := filepath.Join("..", "..")
	registryBody, err := os.ReadFile(filepath.Join(root, ".agents", "plugins", "registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	var registry environment.Registry
	if err := json.Unmarshal(registryBody, &registry); err != nil {
		t.Fatal(err)
	}
	verifier, err := environment.DefaultRegistryVerifier()
	if err != nil {
		t.Fatal(err)
	}
	verified, err := verifier.Verify(registry)
	if err != nil {
		t.Fatalf("verify release Registry with built-in trust: %v", err)
	}

	profileBody, err := os.ReadFile(filepath.Join(root, "deploy", "systemd", "environment-profile.json"))
	if err != nil {
		t.Fatal(err)
	}
	var profile environment.Profile
	if err := json.Unmarshal(profileBody, &profile); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	manifest, err := environment.BuildManifest("release-validation", profile, verified, now, now.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("build release Manifest: %v", err)
	}
	if manifest.ProfileID != "contentcloud.video-production" || len(manifest.Distribution.Plugins) != 1 {
		t.Fatalf("unexpected release Manifest: %#v", manifest)
	}
}

func TestTrustedKeySetFailsClosed(t *testing.T) {
	tests := [][]byte{
		[]byte(`{"$schema":"test","schema_version":"1.0","keys":[]}`),
		[]byte(`{"$schema":"test","schema_version":"1.0","keys":[],"unknown":true}`),
		[]byte(`{"$schema":"test","schema_version":"1.0","keys":[{"key_id":"bad","algorithm":"ed25519","status":"active","public_key":"eA=="}]}`),
	}
	for _, body := range tests {
		if _, err := environment.ManifestVerifierJSON(body); err == nil {
			t.Fatalf("invalid trust store accepted: %s", body)
		}
	}
	if _, err := environment.DefaultManifestVerifier(); err != nil {
		t.Fatalf("built-in production manifest trust store: %v", err)
	}
	if _, err := environment.DefaultRegistryVerifier(); err != nil {
		t.Fatalf("built-in production Plugin trust store: %v", err)
	}
}
