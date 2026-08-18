package cli

import (
	"encoding/json"
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
	"strings"
)

func GenerateFixtureKnowledge(contract sourcedomain.TaskContract, limit int) sourcedomain.KnowledgeExtractionPackage {
	if limit <= 0 {
		limit = 20
	}
	candidates := []sourcedomain.KnowledgeCandidate{}
	for _, source := range contract.Sources {
		for _, evidence := range source.Evidence {
			locator, _ := json.Marshal(evidence.Locator)
			kind := "fact"
			if source.SourceType == "visual_asset" || strings.Contains(source.SourceType, "visual") {
				kind = "visual_rule"
			}
			candidates = append(candidates, sourcedomain.KnowledgeCandidate{Kind: kind, Title: source.Name, Statement: evidence.Quote, Subject: contract.Project.ProductName, Predicate: source.SourceType, Value: sourcedomain.TypedValue{Type: "text", Text: evidence.Quote}, Scope: sourcedomain.KnowledgeScope{Regions: []string{}, Channels: []string{}, Audiences: []string{}, ProductVariants: []string{}}, RiskLevel: "medium", AllowedChannels: []string{}, Evidence: []sourcedomain.EvidenceRef{{SourceRevisionID: source.RevisionID, LocatorKind: evidence.LocatorKind, Locator: string(locator), Quote: evidence.Quote}}, ForbiddenExtensions: []string{}, DependsOnFactIDs: []string{}})
			if len(candidates) >= limit {
				return sourcedomain.KnowledgeExtractionPackage{SchemaVersion: "1.0", Candidates: candidates, Warnings: []string{}}
			}
		}
	}
	return sourcedomain.KnowledgeExtractionPackage{SchemaVersion: "1.0", Candidates: candidates, Warnings: []string{}}
}
