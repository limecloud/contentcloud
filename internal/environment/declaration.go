package environment

import (
	"sort"

	"github.com/limecloud/contentcloud/internal/domain"
)

// DeclarationDigests are stable policy identities. They deliberately exclude
// manifest issuance time, signature and local installation receipts so the
// server declaration can be compared with the same declaration observed on a
// device without conflating it with host state.
type DeclarationDigests struct {
	Environment string `json:"environment_digest"`
	Plugin      string `json:"plugin_digest"`
	Skill       string `json:"skill_digest"`
	MCP         string `json:"mcp_digest"`
}

func DigestsForManifest(manifest Manifest) (DeclarationDigests, error) {
	plugins := append([]PluginRef(nil), manifest.Distribution.Plugins...)
	for index := range plugins {
		plugins[index].Capabilities = sortedDeclarationValues(plugins[index].Capabilities)
	}
	sort.Slice(plugins, func(i, j int) bool {
		if plugins[i].ID != plugins[j].ID {
			return plugins[i].ID < plugins[j].ID
		}
		if plugins[i].Version != plugins[j].Version {
			return plugins[i].Version < plugins[j].Version
		}
		return plugins[i].Digest < plugins[j].Digest
	})
	pluginDigest, err := declarationDigest("plugins", plugins)
	if err != nil {
		return DeclarationDigests{}, err
	}
	// Current manifests declare component ownership at plugin granularity. The
	// context label keeps Skill and MCP declarations distinct until the Agent
	// Plugins manifest exposes component-level release references.
	skillDigest, err := declarationDigest("skills", plugins)
	if err != nil {
		return DeclarationDigests{}, err
	}
	mcpDigest, err := declarationDigest("mcp", plugins)
	if err != nil {
		return DeclarationDigests{}, err
	}
	environmentDigest, err := declarationDigest("environment", struct {
		SchemaVersion      string               `json:"schema_version"`
		ProfileID          string               `json:"profile_id"`
		ProfileVersion     string               `json:"profile_version"`
		EnvironmentVersion string               `json:"environment_version"`
		Harness            string               `json:"harness"`
		Marketplace        string               `json:"marketplace"`
		Plugins            []PluginRef          `json:"plugins"`
		WorkspaceTemplate  WorkspaceTemplateRef `json:"workspace_template"`
		Capabilities       []string             `json:"capabilities"`
		ContentTypes       []string             `json:"content_types"`
		Policies           Policies             `json:"policies"`
	}{
		SchemaVersion: manifest.SchemaVersion, ProfileID: manifest.ProfileID, ProfileVersion: manifest.ProfileVersion,
		EnvironmentVersion: manifest.EnvironmentVersion, Harness: manifest.Harness, Marketplace: manifest.Distribution.Marketplace,
		Plugins: plugins, WorkspaceTemplate: manifest.WorkspaceTemplate,
		Capabilities: sortedDeclarationValues(manifest.Capabilities), ContentTypes: sortedDeclarationValues(manifest.ContentTypes), Policies: manifest.Policies,
	})
	if err != nil {
		return DeclarationDigests{}, err
	}
	return DeclarationDigests{Environment: environmentDigest, Plugin: pluginDigest, Skill: skillDigest, MCP: mcpDigest}, nil
}

func declarationDigest(context string, value any) (string, error) {
	hash, err := domain.CanonicalHash(struct {
		Context string `json:"context"`
		Value   any    `json:"value"`
	}{Context: "contentcloud.environment-declaration." + context + "/1.0", Value: value})
	if err != nil {
		return "", err
	}
	return "sha256:" + hash, nil
}

func sortedDeclarationValues(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}
