package httpapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/internal/domain"
)

func TestSafeProviderBindingOnlyExposesCredentialPresence(t *testing.T) {
	value := safeProviderBinding(domain.ProviderBinding{
		TenantID: "tenant-1", ProviderID: "modelark-seedance25", ProfileVersion: "1.0.0",
		State: "active", CredentialRef: "secret://providers/modelark-seedance25", EgressPolicy: "provider-only",
		MaxConcurrency: 1,
	})
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(body)
	if !value.CredentialConfigured || !strings.Contains(encoded, `"credential_configured":true`) {
		t.Fatalf("credential presence missing: %s", encoded)
	}
	if strings.Contains(encoded, "credential_ref") || strings.Contains(encoded, "secret://providers/modelark-seedance25") {
		t.Fatalf("credential reference leaked: %s", encoded)
	}
}
