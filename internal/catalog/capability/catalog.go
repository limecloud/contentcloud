package capabilitycatalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
	"sort"
	"strings"
)

type definition struct {
	ID                   string
	Version              string
	InputSchema          string
	OutputSchema         string
	PresentationProfiles []string
}

var builtins = []definition{
	{
		ID:                   sourcedomain.KnowledgeExtractCapability,
		Version:              "1.0.0",
		InputSchema:          sourcedomain.TaskContractSchema,
		OutputSchema:         sourcedomain.KnowledgeCandidatesSchema,
		PresentationProfiles: []string{"cloud_native"},
	},
}

// Builtins returns the exact local capabilities for one immutable ContentCloud release.
func Builtins(releaseVersion string) []catalogdomain.Capability {
	result := make([]catalogdomain.Capability, 0, len(builtins))
	for _, item := range builtins {
		capability := catalogdomain.Capability{
			ID:                   item.ID,
			Version:              item.Version,
			Kind:                 "business_capability",
			InputSchema:          item.InputSchema,
			OutputSchema:         item.OutputSchema,
			PresentationProfiles: append([]string(nil), item.PresentationProfiles...),
			LocalOnly:            true,
		}
		capability.Digest = Digest(capability, releaseVersion)
		result = append(result, capability)
	}
	return result
}

func Exact(id, releaseVersion string) (catalogdomain.Capability, bool) {
	for _, capability := range Builtins(releaseVersion) {
		if capability.ID == id {
			return capability, true
		}
	}
	return catalogdomain.Capability{}, false
}

func Digest(capability catalogdomain.Capability, releaseVersion string) string {
	profiles := append([]string(nil), capability.PresentationProfiles...)
	sort.Strings(profiles)
	payload := struct {
		Context               string   `json:"context"`
		ImplementationVersion string   `json:"implementation_version"`
		ID                    string   `json:"id"`
		Version               string   `json:"version"`
		Kind                  string   `json:"kind"`
		InputSchema           string   `json:"input_schema"`
		OutputSchema          string   `json:"output_schema"`
		PresentationProfiles  []string `json:"presentation_profiles"`
		LocalOnly             bool     `json:"local_only"`
	}{
		Context:               "contentcloud.capability-manifest.v1",
		ImplementationVersion: strings.TrimSpace(releaseVersion),
		ID:                    capability.ID,
		Version:               capability.Version,
		Kind:                  capability.Kind,
		InputSchema:           capability.InputSchema,
		OutputSchema:          capability.OutputSchema,
		PresentationProfiles:  profiles,
		LocalOnly:             capability.LocalOnly,
	}
	body, _ := json.Marshal(payload)
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}
