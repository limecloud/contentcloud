package cli

import (
	"errors"
	"testing"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/integration/pluginhost"
	"github.com/limecloud/contentcloud/internal/integration/pluginidentity"
)

func TestBundledPluginRuntimeUsesRequestedEmbeddedPackage(t *testing.T) {
	t.Setenv("CONTENTCLOUD_PLUGIN_STORE", t.TempDir())

	runtime, err := (&Root{}).bundledPluginRuntime(string(pluginhost.HostCodex), pluginidentity.Marketing, pluginidentity.MarketingVersion)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.HostID != pluginhost.HostCodex {
		t.Fatalf("host = %q, want %q", runtime.HostID, pluginhost.HostCodex)
	}
	if runtime.Package.Manifest.Name != pluginidentity.Marketing || runtime.Package.Manifest.Version != pluginidentity.MarketingVersion {
		t.Fatalf("loaded package = %s@%s, want %s@%s", runtime.Package.Manifest.Name, runtime.Package.Manifest.Version, pluginidentity.Marketing, pluginidentity.MarketingVersion)
	}
	if len(runtime.Package.Skills) != 8 {
		t.Fatalf("loaded marketing skills = %d, want 8", len(runtime.Package.Skills))
	}
}

func TestBundledPluginRuntimeReportsUnavailableArtifact(t *testing.T) {
	t.Setenv("CONTENTCLOUD_PLUGIN_STORE", t.TempDir())

	_, err := (&Root{}).bundledPluginRuntime(string(pluginhost.HostCodex), "contentcloud-not-bundled", "9.9.9")
	if err == nil {
		t.Fatal("missing bundled package unexpectedly loaded")
	}
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("error = %T %v, want domain error", err, err)
	}
	if domainErr.Code != "ENVIRONMENT_PLUGIN_ARTIFACT_UNAVAILABLE" {
		t.Fatalf("error code = %q, want ENVIRONMENT_PLUGIN_ARTIFACT_UNAVAILABLE", domainErr.Code)
	}
}
