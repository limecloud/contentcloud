package pluginbuiltin

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	contentcloud "github.com/limecloud/contentcloud"
	"github.com/limecloud/contentcloud/internal/integration/plugin"
)

const (
	VideoProduction        = "contentcloud-video-production"
	VideoProductionVersion = "0.22.0"
	WechatArticle          = "contentcloud-wechat-article"
	WechatArticleVersion   = "0.1.0"
)

func Load(storeRoot, name, version string) (plugin.Package, error) {
	if strings.TrimSpace(storeRoot) == "" || strings.TrimSpace(version) == "" {
		return plugin.Package{}, fmt.Errorf("bundled Agent Plugin requires store root and version")
	}
	destination := filepath.Join(storeRoot, "bundles", name, version)
	if pkg, err := plugin.Load(destination); err == nil {
		if pkg.Manifest.Name != name || pkg.Manifest.Version != version {
			return plugin.Package{}, fmt.Errorf("bundled Agent Plugin cache identity mismatch")
		}
		return pkg, nil
	} else if _, statErr := os.Stat(destination); statErr == nil {
		return plugin.Package{}, fmt.Errorf("bundled Agent Plugin cache is invalid: %w", err)
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return plugin.Package{}, statErr
	}
	source, err := contentcloud.AgentPlugin(name)
	if err != nil {
		return plugin.Package{}, err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return plugin.Package{}, err
	}
	stage, err := os.MkdirTemp(filepath.Dir(destination), ".bundle-")
	if err != nil {
		return plugin.Package{}, err
	}
	defer os.RemoveAll(stage)
	if err := materialize(source, stage); err != nil {
		return plugin.Package{}, err
	}
	pkg, err := plugin.Load(stage)
	if err != nil {
		return plugin.Package{}, err
	}
	if pkg.Manifest.Name != name || pkg.Manifest.Version != version {
		return plugin.Package{}, fmt.Errorf("bundled Agent Plugin identity mismatch")
	}
	if err := os.Rename(stage, destination); err != nil {
		if existing, loadErr := plugin.Load(destination); loadErr == nil && existing.Digest == pkg.Digest {
			return existing, nil
		}
		return plugin.Package{}, err
	}
	return plugin.Load(destination)
}

func materialize(source fs.FS, destination string) error {
	return fs.WalkDir(source, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "." {
			return nil
		}
		target := filepath.Join(destination, filepath.FromSlash(path))
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("bundled Agent Plugin contains non-regular file %s", path)
		}
		body, err := fs.ReadFile(source, path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		return os.WriteFile(target, body, 0o600)
	})
}
