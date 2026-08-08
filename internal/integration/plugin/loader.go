package plugin

import (
	"fmt"
	"sort"

	"github.com/limecloud/contentcloud/internal/domain"
)

func Load(root string) (Package, error) {
	return LoadWithLimits(root, DefaultLimits())
}

func LoadWithLimits(root string, limits Limits) (Package, error) {
	if err := validateLimits(limits); err != nil {
		return Package{}, err
	}
	resolved, files, total, err := inspectPackageRoot(root, limits)
	if err != nil {
		return Package{}, domain.Invalid("AGENT_PLUGIN_PACKAGE_INVALID", err.Error())
	}
	manifestBody, err := readPackageFile(resolved, "plugin.json", limits.MaxFileBytes)
	if err != nil {
		return Package{}, domain.Invalid("AGENT_PLUGIN_MANIFEST_MISSING", "plugin root must contain a regular plugin.json")
	}
	manifest, diagnostics, err := loadManifest(manifestBody)
	if err != nil {
		return Package{}, err
	}
	skills, skillDiagnostics := discoverSkills(resolved, limits)
	servers, mcpDiagnostics := discoverMCPServers(resolved, limits)
	claims, claimsDigest, err := loadClaims(resolved, manifest, limits)
	if err != nil {
		return Package{}, err
	}
	diagnostics = append(diagnostics, skillDiagnostics...)
	diagnostics = append(diagnostics, mcpDiagnostics...)
	sort.Slice(diagnostics, func(left, right int) bool {
		if diagnostics[left].Path != diagnostics[right].Path {
			return diagnostics[left].Path < diagnostics[right].Path
		}
		if diagnostics[left].Component != diagnostics[right].Component {
			return diagnostics[left].Component < diagnostics[right].Component
		}
		return diagnostics[left].Code < diagnostics[right].Code
	})
	digest, err := packageDigest(resolved, files)
	if err != nil {
		return Package{}, domain.Invalid("AGENT_PLUGIN_DIGEST_FAILED", err.Error())
	}
	return Package{
		Root: resolved, SpecVersion: SpecVersion, Manifest: manifest, Skills: skills,
		MCPServers: servers, Claims: claims, ClaimsDigest: claimsDigest,
		Digest: digest, Files: len(files), Bytes: total, Diagnostics: diagnostics,
	}, nil
}

func validateLimits(limits Limits) error {
	if limits.MaxFiles < 1 || limits.MaxFileBytes < 1 || limits.MaxPackBytes < limits.MaxFileBytes || limits.MaxDepth < 1 {
		return domain.Invalid("AGENT_PLUGIN_LIMITS_INVALID", fmt.Sprintf("invalid package limits: %+v", limits))
	}
	return nil
}
