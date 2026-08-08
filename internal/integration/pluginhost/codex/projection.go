package codex

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const marketplaceManifestRelativePath = ".agents/plugins/marketplace.json"

type marketplaceManifest struct {
	Name      string                      `json:"name"`
	Interface marketplaceInterface        `json:"interface"`
	Plugins   []marketplacePluginManifest `json:"plugins"`
}

type marketplaceInterface struct {
	DisplayName string `json:"displayName"`
}

type marketplacePluginManifest struct {
	Name   string                  `json:"name"`
	Source marketplacePluginSource `json:"source"`
	Policy marketplacePluginPolicy `json:"policy"`
}

type marketplacePluginSource struct {
	Source string `json:"source"`
	Path   string `json:"path"`
}

type marketplacePluginPolicy struct {
	Installation   string `json:"installation"`
	Authentication string `json:"authentication"`
}

func (h *Host) projectionManifestPath() string {
	return filepath.Join(h.config.ProjectionRoot, marketplaceManifestRelativePath)
}

func (h *Host) readProjection() (marketplaceManifest, []byte, error) {
	path := h.projectionManifestPath()
	body, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		manifest := h.emptyProjection()
		encoded, encodeErr := encodeProjection(manifest)
		return manifest, encoded, encodeErr
	}
	if err != nil {
		return marketplaceManifest{}, nil, err
	}
	var manifest marketplaceManifest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return marketplaceManifest{}, nil, fmt.Errorf("parse Codex marketplace projection: %w", err)
	}
	if manifest.Name != h.config.MarketplaceName {
		return marketplaceManifest{}, nil, fmt.Errorf("Codex marketplace projection name %q does not match %q", manifest.Name, h.config.MarketplaceName)
	}
	return manifest, body, nil
}

func (h *Host) emptyProjection() marketplaceManifest {
	return marketplaceManifest{
		Name:      h.config.MarketplaceName,
		Interface: marketplaceInterface{DisplayName: "ContentCloud"},
		Plugins:   []marketplacePluginManifest{},
	}
}

func (h *Host) upsertProjection(pluginName, packageRoot string) ([]byte, error) {
	manifest, previous, err := h.readProjection()
	if err != nil {
		return nil, err
	}
	relativePackageRoot, err := h.relativePluginSource(packageRoot)
	if err != nil {
		return nil, err
	}
	entry := marketplacePluginManifest{
		Name: pluginName,
		Source: marketplacePluginSource{
			Source: "local",
			Path:   relativePackageRoot,
		},
		Policy: marketplacePluginPolicy{
			Installation:   "AVAILABLE",
			Authentication: "ON_INSTALL",
		},
	}
	found := false
	for index := range manifest.Plugins {
		if manifest.Plugins[index].Name == pluginName {
			manifest.Plugins[index] = entry
			found = true
			break
		}
	}
	if !found {
		manifest.Plugins = append(manifest.Plugins, entry)
	}
	sort.Slice(manifest.Plugins, func(left, right int) bool {
		return manifest.Plugins[left].Name < manifest.Plugins[right].Name
	})
	if err := h.writeProjection(manifest); err != nil {
		return nil, err
	}
	return previous, nil
}

func (h *Host) relativePluginSource(packageRoot string) (string, error) {
	projectionRoot, err := canonicalPath(h.config.ProjectionRoot)
	if err != nil {
		return "", err
	}
	canonicalPackageRoot, err := canonicalPath(packageRoot)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(projectionRoot, canonicalPackageRoot)
	if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) || len(relative) >= 3 && relative[:3] == ".."+string(filepath.Separator) {
		return "", fmt.Errorf("Codex plugin package must stay inside marketplace projection root")
	}
	return "./" + filepath.ToSlash(relative), nil
}

func (h *Host) resolvePluginSource(source string) (string, error) {
	if source == "." || source == "./" {
		return canonicalPath(h.config.ProjectionRoot)
	}
	relative := strings.TrimPrefix(source, "./")
	if relative == source || relative == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("Codex marketplace plugin source must be a relative ./ path")
	}
	for _, component := range strings.Split(filepath.ToSlash(relative), "/") {
		if component == "" || component == "." || component == ".." {
			return "", fmt.Errorf("Codex marketplace plugin source escapes projection root")
		}
	}
	return canonicalPath(filepath.Join(h.config.ProjectionRoot, filepath.FromSlash(relative)))
}

func (h *Host) removeProjection(pluginName string) ([]byte, bool, error) {
	manifest, previous, err := h.readProjection()
	if err != nil {
		return nil, false, err
	}
	plugins := manifest.Plugins[:0]
	for _, entry := range manifest.Plugins {
		if entry.Name != pluginName {
			plugins = append(plugins, entry)
		}
	}
	manifest.Plugins = plugins
	if err := h.writeProjection(manifest); err != nil {
		return nil, false, err
	}
	return previous, len(manifest.Plugins) == 0, nil
}

func (h *Host) restoreProjection(body []byte) error {
	var manifest marketplaceManifest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return fmt.Errorf("parse saved Codex marketplace projection: %w", err)
	}
	if manifest.Name != h.config.MarketplaceName {
		return fmt.Errorf("saved Codex marketplace projection name %q does not match %q", manifest.Name, h.config.MarketplaceName)
	}
	return h.writeProjection(manifest)
}

func (h *Host) writeProjection(manifest marketplaceManifest) error {
	body, err := encodeProjection(manifest)
	if err != nil {
		return err
	}
	path := h.projectionManifestPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".marketplace-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(body); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func encodeProjection(manifest marketplaceManifest) ([]byte, error) {
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}
