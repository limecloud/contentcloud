package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
)

var (
	claimCapabilityPattern = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)+$`)
	claimVersionPattern    = regexp.MustCompile(`^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$`)
	claimSchemaPattern     = regexp.MustCompile(`^contracts/.+\.schema\.json$`)
)

var allowedClaimPermissions = map[string]struct{}{
	"workspace:read":          {},
	"workspace:write-managed": {},
	"contentcloud-control-plane:explicit-actions-only": {},
	"credential-store:macos-keychain":                  {},
}

var allowedHostRequirements = map[string]struct{}{
	"skills": {}, "mcp_stdio": {}, "new_session_required": {},
}

type Claims struct {
	SchemaVersion         string            `json:"schema_version"`
	PluginID              string            `json:"plugin_id"`
	PluginVersion         string            `json:"plugin_version"`
	PackageSpecVersion    string            `json:"package_spec_version"`
	Kind                  string            `json:"kind"`
	RequestedCapabilities []ClaimCapability `json:"requested_capabilities"`
	PermissionsRequested  []string          `json:"permissions_requested"`
	DataFlow              ClaimDataFlow     `json:"data_flow"`
	Cost                  ClaimCost         `json:"cost"`
	Hosts                 []ClaimHost       `json:"hosts"`
	Support               ClaimSupport      `json:"support"`
}

type ClaimCapability struct {
	ID            string   `json:"id"`
	Version       string   `json:"version"`
	InputSchemas  []string `json:"input_schemas"`
	OutputSchemas []string `json:"output_schemas"`
}

type ClaimDataFlow struct {
	LocalByDefault       bool     `json:"local_by_default"`
	DeclaredCloudActions []string `json:"declared_cloud_actions"`
}

type ClaimCost struct {
	Model     string `json:"model"`
	Currency  string `json:"currency,omitempty"`
	Unit      string `json:"unit,omitempty"`
	UnitPrice string `json:"unit_price,omitempty"`
	Notice    string `json:"notice"`
}

type ClaimHost struct {
	ID       string   `json:"id"`
	Required []string `json:"required"`
}

type ClaimSupport struct {
	Owner   string `json:"owner"`
	Runbook string `json:"runbook"`
}

type claimExtension struct {
	Claims string `json:"claims"`
}

func loadClaims(root string, manifest Manifest, limits Limits) (*Claims, string, error) {
	raw, exists := manifest.Extensions[ExtensionNamespace]
	if !exists {
		return nil, "", nil
	}
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return nil, "", invalidClaims("ContentCloud extension is invalid", err)
	}
	var extension claimExtension
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&extension); err != nil || strings.TrimSpace(extension.Claims) == "" {
		return nil, "", invalidClaims("ContentCloud extension must declare exactly one claims path", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, "", invalidClaims("ContentCloud extension contains trailing data", err)
	}
	if !isSafeSlashRelative(extension.Claims) {
		return nil, "", invalidClaims("claims path must be a safe ./ plugin-relative path", nil)
	}
	relative := strings.TrimPrefix(extension.Claims, "./")
	body, err := readPackageFile(root, relative, limits.MaxFileBytes)
	if err != nil {
		return nil, "", invalidClaims("claims file cannot be read", err)
	}
	if err := rejectDuplicateJSONKeys(body); err != nil {
		return nil, "", invalidClaims("claims JSON contains duplicate keys", err)
	}
	var claims Claims
	decoder = json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&claims); err != nil {
		return nil, "", invalidClaims("claims JSON does not match the closed schema", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, "", invalidClaims("claims JSON contains trailing data", err)
	}
	if err := validateClaims(root, manifest, claims); err != nil {
		return nil, "", err
	}
	return &claims, bytesDigest(body), nil
}

func validateClaims(root string, manifest Manifest, claims Claims) error {
	if claims.SchemaVersion != ClaimsSchemaVersion || claims.PackageSpecVersion != SpecVersion || claims.PluginID != manifest.Name || claims.PluginVersion != manifest.Version || !validPluginKind(claims.Kind) {
		return invalidClaims("claims identity must match the standard manifest and supported schema", nil)
	}
	capabilities := map[string]struct{}{}
	for _, capability := range claims.RequestedCapabilities {
		if !claimCapabilityPattern.MatchString(capability.ID) || !claimVersionPattern.MatchString(capability.Version) {
			return invalidClaims("requested capability identity is invalid", nil)
		}
		if _, duplicate := capabilities[capability.ID]; duplicate {
			return invalidClaims("requested capabilities contain a duplicate ID", nil)
		}
		capabilities[capability.ID] = struct{}{}
		if err := validateClaimSchemas(capability.InputSchemas); err != nil {
			return err
		}
		if err := validateClaimSchemas(capability.OutputSchemas); err != nil {
			return err
		}
	}
	if err := validateUniqueClaimValues(claims.PermissionsRequested, allowedClaimPermissions, "permissions_requested"); err != nil {
		return err
	}
	if !claims.DataFlow.LocalByDefault {
		return invalidClaims("first-party ContentCloud plugins must be local_by_default", nil)
	}
	if err := validateUniqueNonEmpty(claims.DataFlow.DeclaredCloudActions, "declared_cloud_actions"); err != nil {
		return err
	}
	if !containsString([]string{"free", "included", "metered", "external"}, claims.Cost.Model) || strings.TrimSpace(claims.Cost.Notice) == "" {
		return invalidClaims("cost must declare a supported model and notice", nil)
	}
	if claims.Cost.Model == "metered" && (len(claims.Cost.Currency) != 3 || strings.TrimSpace(claims.Cost.Unit) == "" || strings.TrimSpace(claims.Cost.UnitPrice) == "") {
		return invalidClaims("metered cost must declare currency, unit, and unit_price", nil)
	}
	if len(claims.Hosts) == 0 {
		return invalidClaims("claims must declare at least one supported host", nil)
	}
	hosts := map[string]struct{}{}
	for _, host := range claims.Hosts {
		if host.ID != "codex" && host.ID != "claude" {
			return invalidClaims("claims contain an unsupported host", nil)
		}
		if _, duplicate := hosts[host.ID]; duplicate {
			return invalidClaims("claims contain a duplicate host", nil)
		}
		hosts[host.ID] = struct{}{}
		if err := validateUniqueClaimValues(host.Required, allowedHostRequirements, "hosts.required"); err != nil {
			return err
		}
	}
	if strings.TrimSpace(claims.Support.Owner) == "" || !isSafeSlashRelative(claims.Support.Runbook) {
		return invalidClaims("support must declare an owner and safe ./ runbook path", nil)
	}
	runbook := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(claims.Support.Runbook, "./")))
	resolvedRunbook, err := filepath.EvalSymlinks(runbook)
	if err != nil || !pathWithin(root, resolvedRunbook) {
		return invalidClaims("support runbook must resolve within the plugin package", err)
	}
	info, err := os.Stat(resolvedRunbook)
	if err != nil || !info.Mode().IsRegular() {
		return invalidClaims("support runbook must be a regular file", err)
	}
	return nil
}

func validateClaimSchemas(values []string) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		if !claimSchemaPattern.MatchString(value) {
			return invalidClaims("capability schema reference is invalid", nil)
		}
		if _, duplicate := seen[value]; duplicate {
			return invalidClaims("capability schema references contain duplicates", nil)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateUniqueClaimValues(values []string, allowed map[string]struct{}, field string) error {
	if len(values) == 0 {
		return invalidClaims(field+" must not be empty", nil)
	}
	seen := map[string]struct{}{}
	for _, value := range values {
		if _, known := allowed[value]; !known {
			return invalidClaims(fmt.Sprintf("%s contains unsupported value %q", field, value), nil)
		}
		if _, duplicate := seen[value]; duplicate {
			return invalidClaims(field+" contains duplicates", nil)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateUniqueNonEmpty(values []string, field string) error {
	sorted := append([]string(nil), values...)
	sort.Strings(sorted)
	for index, value := range sorted {
		if strings.TrimSpace(value) == "" || (index > 0 && value == sorted[index-1]) {
			return invalidClaims(field+" contains an empty or duplicate value", nil)
		}
	}
	return nil
}

func validPluginKind(value string) bool {
	return value == "scene_plugin" || value == "skill_pack" || value == "provider_mcp_pack"
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func invalidClaims(message string, cause error) error {
	if cause != nil {
		message += ": " + cause.Error()
	}
	return domain.Invalid("CONTENTCLOUD_PLUGIN_CLAIMS_INVALID", message)
}
