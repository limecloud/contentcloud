package pluginbuiltin_test

import (
	"path/filepath"
	"testing"

	"github.com/limecloud/contentcloud/internal/integration/pluginbuiltin"
	"github.com/limecloud/contentcloud/internal/integration/pluginidentity"
)

func TestLoadBundledStandardPlugin(t *testing.T) {
	pkg, err := pluginbuiltin.Load(t.TempDir(), pluginidentity.VideoProduction, pluginidentity.VideoProductionVersion)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Manifest.Name != pluginidentity.VideoProduction || pkg.SpecVersion != "1.0.0" || len(pkg.Skills) == 0 || len(pkg.MCPServers) != 1 {
		t.Fatalf("unexpected bundled Agent Plugin: %#v", pkg)
	}
	if filepath.Base(pkg.Root) != pluginidentity.VideoProductionVersion {
		t.Fatalf("bundle was not materialized in the versioned store: %s", pkg.Root)
	}
}

func TestLoadBundledWeChatSkillPack(t *testing.T) {
	pkg, err := pluginbuiltin.Load(t.TempDir(), pluginidentity.WechatArticle, pluginidentity.WechatArticleVersion)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Manifest.Name != pluginidentity.WechatArticle || len(pkg.Skills) != 4 || len(pkg.MCPServers) != 0 {
		t.Fatalf("unexpected bundled WeChat Skill Pack: %#v", pkg)
	}
}

func TestLoadBundledMarketingSkillPack(t *testing.T) {
	pkg, err := pluginbuiltin.Load(t.TempDir(), pluginidentity.Marketing, pluginidentity.MarketingVersion)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Manifest.Name != pluginidentity.Marketing || pkg.Manifest.Version != pluginidentity.MarketingVersion || len(pkg.Skills) != 8 || len(pkg.MCPServers) != 0 {
		t.Fatalf("unexpected bundled marketing Skill Pack: %#v", pkg)
	}
	if filepath.Base(pkg.Root) != pluginidentity.MarketingVersion {
		t.Fatalf("bundle was not materialized in the versioned store: %s", pkg.Root)
	}
}
