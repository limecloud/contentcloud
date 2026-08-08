package claude

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

	"github.com/limecloud/contentcloud/internal/integration/plugin"
	"github.com/limecloud/contentcloud/internal/integration/pluginhost"
)

const (
	marketplaceManifestRelativePath = ".claude-plugin/marketplace.json"
	projectionMarkerFile            = ".contentcloud-plugin-host.json"
)

type marketplaceManifest struct {
	Name        string                   `json:"name"`
	Owner       marketplaceOwner         `json:"owner"`
	Description string                   `json:"description"`
	Plugins     []marketplacePluginEntry `json:"plugins"`
}

type marketplaceOwner struct {
	Name string `json:"name"`
}

type marketplacePluginEntry struct {
	Name        string         `json:"name"`
	Source      string         `json:"source"`
	Version     string         `json:"version,omitempty"`
	Description string         `json:"description,omitempty"`
	Author      *plugin.Author `json:"author,omitempty"`
	Homepage    string         `json:"homepage,omitempty"`
	Repository  string         `json:"repository,omitempty"`
	License     string         `json:"license,omitempty"`
	Keywords    []string       `json:"keywords,omitempty"`
	Strict      bool           `json:"strict"`
}

type claudePluginManifest struct {
	Name        string         `json:"name"`
	DisplayName string         `json:"displayName,omitempty"`
	Version     string         `json:"version,omitempty"`
	Description string         `json:"description,omitempty"`
	Author      *plugin.Author `json:"author,omitempty"`
	Homepage    string         `json:"homepage,omitempty"`
	Repository  string         `json:"repository,omitempty"`
	License     string         `json:"license,omitempty"`
	Keywords    []string       `json:"keywords,omitempty"`
}

type claudeMCPManifest struct {
	Servers map[string]claudeMCPServer `json:"mcpServers"`
}

type claudeMCPServer struct {
	Type    string            `json:"type,omitempty"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	CWD     string            `json:"cwd,omitempty"`
}

type projectionMarker struct {
	SchemaVersion string `json:"schema_version"`
	PluginID      string `json:"plugin_id"`
	Version       string `json:"version"`
	Digest        string `json:"digest"`
}

func (h *Host) marketplaceManifestPath() string {
	return filepath.Join(h.config.ProjectionRoot, marketplaceManifestRelativePath)
}

func (h *Host) emptyMarketplace() marketplaceManifest {
	return marketplaceManifest{
		Name:        h.config.MarketplaceName,
		Owner:       marketplaceOwner{Name: "ContentCloud"},
		Description: "ContentCloud managed Agent Plugins",
		Plugins:     []marketplacePluginEntry{},
	}
}

func (h *Host) readMarketplaceProjection() (marketplaceManifest, []byte, error) {
	body, err := os.ReadFile(h.marketplaceManifestPath())
	if errors.Is(err, fs.ErrNotExist) {
		manifest := h.emptyMarketplace()
		encoded, encodeErr := encodeJSON(manifest)
		return manifest, encoded, encodeErr
	}
	if err != nil {
		return marketplaceManifest{}, nil, err
	}
	var manifest marketplaceManifest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return marketplaceManifest{}, nil, fmt.Errorf("parse Claude marketplace projection: %w", err)
	}
	if manifest.Name != h.config.MarketplaceName {
		return marketplaceManifest{}, nil, fmt.Errorf("Claude marketplace projection name %q does not match %q", manifest.Name, h.config.MarketplaceName)
	}
	return manifest, body, nil
}

func (h *Host) materializePackage(pkg plugin.Package, packageRoot string) (string, error) {
	digest := strings.TrimPrefix(pkg.Digest, "sha256:")
	if digest == "" || strings.ContainsAny(digest, `/\\`) {
		return "", fmt.Errorf("invalid plugin digest for Claude projection")
	}
	destination := filepath.Join(h.config.ProjectionRoot, "plugins", pkg.Manifest.Name, digest)
	if marker, err := readProjectionMarker(destination); err == nil {
		if marker.PluginID != pkg.Manifest.Name || marker.Version != pkg.Manifest.Version || marker.Digest != pkg.Digest {
			return "", fmt.Errorf("existing Claude projection identity does not match requested release")
		}
		return destination, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	for _, relative := range []string{".claude-plugin", ".mcp.json"} {
		if _, err := os.Stat(filepath.Join(packageRoot, relative)); err == nil {
			return "", fmt.Errorf("standard plugin package contains Claude-private path %s", relative)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(filepath.Dir(destination), ".claude-stage-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	if err := copyProjectionTree(packageRoot, stage); err != nil {
		return "", err
	}
	manifest := claudePluginManifest{
		Name:        pkg.Manifest.Name,
		DisplayName: pkg.Manifest.Name,
		Version:     pkg.Manifest.Version,
		Description: pkg.Manifest.Description,
		Author:      pkg.Manifest.Author,
		Homepage:    pkg.Manifest.Homepage,
		Repository:  pkg.Manifest.Repository,
		License:     pkg.Manifest.License,
		Keywords:    pkg.Manifest.Keywords,
	}
	if err := writeJSON(filepath.Join(stage, ".claude-plugin", "plugin.json"), manifest); err != nil {
		return "", err
	}
	if len(pkg.MCPServers) > 0 {
		mcp := claudeMCPManifest{Servers: map[string]claudeMCPServer{}}
		for _, server := range pkg.MCPServers {
			if !server.Supported {
				continue
			}
			translated := claudeMCPServer{
				Type:    server.Type,
				Command: translateCommand(server.Command),
				Args:    translateStrings(server.Args),
				Env:     translateMap(server.Env),
				CWD:     translateCWD(server.CWD),
			}
			mcp.Servers[server.Name] = translated
		}
		if len(mcp.Servers) > 0 {
			if err := writeJSON(filepath.Join(stage, ".mcp.json"), mcp); err != nil {
				return "", err
			}
		}
	}
	marker := projectionMarker{
		SchemaVersion: "contentcloud.claude-projection/1.0",
		PluginID:      pkg.Manifest.Name,
		Version:       pkg.Manifest.Version,
		Digest:        pkg.Digest,
	}
	if err := writeJSON(filepath.Join(stage, projectionMarkerFile), marker); err != nil {
		return "", err
	}
	if _, err := os.Stat(destination); err == nil {
		return destination, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if err := os.Rename(stage, destination); err != nil {
		return "", err
	}
	return destination, nil
}

func readProjectionMarker(root string) (projectionMarker, error) {
	body, err := os.ReadFile(filepath.Join(root, projectionMarkerFile))
	if err != nil {
		return projectionMarker{}, err
	}
	var marker projectionMarker
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&marker); err != nil {
		return projectionMarker{}, fmt.Errorf("parse Claude projection marker: %w", err)
	}
	if marker.SchemaVersion != "contentcloud.claude-projection/1.0" {
		return projectionMarker{}, fmt.Errorf("unsupported Claude projection marker %q", marker.SchemaVersion)
	}
	return marker, nil
}

func (h *Host) upsertMarketplaceProjection(pkg plugin.Package, projectedRoot string) ([]byte, error) {
	manifest, previous, err := h.readMarketplaceProjection()
	if err != nil {
		return nil, err
	}
	root, err := canonicalPath(h.config.ProjectionRoot)
	if err != nil {
		return nil, err
	}
	projected, err := canonicalPath(projectedRoot)
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(root, projected)
	if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("Claude plugin projection must stay inside marketplace root")
	}
	entry := marketplacePluginEntry{
		Name:        pkg.Manifest.Name,
		Source:      "./" + filepath.ToSlash(relative),
		Version:     pkg.Manifest.Version,
		Description: pkg.Manifest.Description,
		Author:      pkg.Manifest.Author,
		Homepage:    pkg.Manifest.Homepage,
		Repository:  pkg.Manifest.Repository,
		License:     pkg.Manifest.License,
		Keywords:    pkg.Manifest.Keywords,
		Strict:      true,
	}
	found := false
	for index := range manifest.Plugins {
		if manifest.Plugins[index].Name == pkg.Manifest.Name {
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
	if err := h.writeMarketplaceProjection(manifest); err != nil {
		return nil, err
	}
	return previous, nil
}

func (h *Host) projectedPackageRoot(pluginName string) (string, error) {
	manifest, _, err := h.readMarketplaceProjection()
	if err != nil {
		return "", err
	}
	for _, entry := range manifest.Plugins {
		if entry.Name == pluginName {
			return h.resolveMarketplaceSource(entry.Source)
		}
	}
	return "", fmt.Errorf("Claude marketplace projection lacks plugin %s", pluginName)
}

func (h *Host) resolveMarketplaceSource(source string) (string, error) {
	relative := strings.TrimPrefix(source, "./")
	if relative == source || relative == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("Claude marketplace plugin source must be a relative ./ path")
	}
	for _, component := range strings.Split(filepath.ToSlash(relative), "/") {
		if component == "" || component == "." || component == ".." {
			return "", fmt.Errorf("Claude marketplace plugin source escapes projection root")
		}
	}
	return canonicalPath(filepath.Join(h.config.ProjectionRoot, filepath.FromSlash(relative)))
}

func (h *Host) removeMarketplaceProjection(pluginName string) ([]byte, bool, error) {
	manifest, previous, err := h.readMarketplaceProjection()
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
	if err := h.writeMarketplaceProjection(manifest); err != nil {
		return nil, false, err
	}
	return previous, len(plugins) == 0, nil
}

func (h *Host) restoreMarketplaceProjection(body []byte) error {
	var manifest marketplaceManifest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return fmt.Errorf("parse saved Claude marketplace projection: %w", err)
	}
	if manifest.Name != h.config.MarketplaceName {
		return fmt.Errorf("saved Claude marketplace name %q does not match %q", manifest.Name, h.config.MarketplaceName)
	}
	return h.writeMarketplaceProjection(manifest)
}

func (h *Host) writeMarketplaceProjection(manifest marketplaceManifest) error {
	return writeJSON(h.marketplaceManifestPath(), manifest)
}

func writeJSON(path string, value any) error {
	body, err := encodeJSON(value)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".contentcloud-write-")
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

func encodeJSON(value any) ([]byte, error) {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

func copyProjectionTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("Claude projection rejects non-regular file %s", relative)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		return os.WriteFile(target, body, info.Mode().Perm())
	})
}

func translateCommand(value string) string {
	if strings.HasPrefix(value, "./") {
		return "${CLAUDE_PLUGIN_ROOT}/" + strings.TrimPrefix(value, "./")
	}
	return translateVariables(value)
}

func translateCWD(value string) string {
	if strings.HasPrefix(value, "./") {
		return "${CLAUDE_PLUGIN_ROOT}/" + strings.TrimPrefix(value, "./")
	}
	return translateVariables(value)
}

func translateStrings(values []string) []string {
	if values == nil {
		return nil
	}
	translated := make([]string, len(values))
	for index, value := range values {
		translated[index] = translateVariables(value)
	}
	return translated
}

func translateMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	translated := make(map[string]string, len(values))
	for name, value := range values {
		translated[name] = translateVariables(value)
	}
	return translated
}

func translateVariables(value string) string {
	value = strings.ReplaceAll(value, "${PLUGIN_ROOT}", "${CLAUDE_PLUGIN_ROOT}")
	return strings.ReplaceAll(value, "${PLUGIN_DATA}", "${CLAUDE_PLUGIN_DATA}")
}

func canonicalPath(path string) (string, error) {
	return pluginhost.CanonicalPath(path)
}
