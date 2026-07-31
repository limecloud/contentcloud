package app

import (
	"testing"

	"log/slog"

	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestDaemonRuntimePolicyExposesOptionalAndRequiredUpdates(t *testing.T) {
	service := New(memory.New(), slog.Default(), WithDaemonVersionPolicy("0.10.0", "0.12.0", "https://content.example.com/downloads"))
	policy := service.daemonRuntimePolicy("0.11.0")
	if !policy.UpdateAvailable || policy.UpdateRequired || policy.UpdateCommand == "" {
		t.Fatalf("unexpected optional update policy: %#v", policy)
	}
	policy = service.daemonRuntimePolicy("0.9.0")
	if !policy.UpdateAvailable || !policy.UpdateRequired {
		t.Fatalf("minimum version was not enforced: %#v", policy)
	}
}

func TestCompareDaemonVersionsHandlesPrefixAndPrerelease(t *testing.T) {
	for _, test := range []struct {
		left, right string
		want        int
	}{
		{"v1.2.0", "1.2.0", 0}, {"1.2.0-beta", "1.2.0", -1}, {"1.3", "1.2.9", 1},
	} {
		if got := compareDaemonVersions(test.left, test.right); got != test.want {
			t.Fatalf("compare(%q,%q)=%d want %d", test.left, test.right, got, test.want)
		}
	}
}
