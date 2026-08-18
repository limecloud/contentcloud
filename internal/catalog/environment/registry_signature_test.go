package environment_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/limecloud/contentcloud/internal/catalog/environment"
)

func TestRegistryCanonicalPayloadMatchesNodeConformanceVector(t *testing.T) {
	body, err := os.ReadFile("../../../contracts/plugin-release-signature-v1.fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Entry         environment.RegistryEntry `json:"entry"`
		PayloadSHA256 string                    `json:"payload_sha256"`
	}
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatal(err)
	}
	payload, err := environment.RegistryEntrySigningPayload(fixture.Entry)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	actual := "sha256:" + hex.EncodeToString(sum[:])
	if actual != fixture.PayloadSHA256 {
		t.Fatalf("Go registry payload digest = %s, want shared Node vector %s", actual, fixture.PayloadSHA256)
	}
}
