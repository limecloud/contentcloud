package cli

import (
	"encoding/json"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
)

func GenerateFixtureKnowledge(contract domain.TaskContract, limit int) domain.KnowledgeExtractionPackage {
	if limit <= 0 {
		limit = 20
	}
	candidates := []domain.KnowledgeCandidate{}
	for _, source := range contract.Sources {
		for _, evidence := range source.Evidence {
			locator, _ := json.Marshal(evidence.Locator)
			kind := "fact"
			if source.SourceType == "visual_asset" || strings.Contains(source.SourceType, "visual") {
				kind = "visual_rule"
			}
			candidates = append(candidates, domain.KnowledgeCandidate{Kind: kind, Title: source.Name, Statement: evidence.Quote, Subject: contract.Project.ProductName, Predicate: source.SourceType, Value: domain.TypedValue{Type: "text", Text: evidence.Quote}, Scope: domain.KnowledgeScope{Regions: []string{}, Channels: []string{}, Audiences: []string{}, ProductVariants: []string{}}, RiskLevel: "medium", AllowedChannels: []string{}, Evidence: []domain.EvidenceRef{{SourceRevisionID: source.RevisionID, LocatorKind: evidence.LocatorKind, Locator: string(locator), Quote: evidence.Quote}}, ForbiddenExtensions: []string{}, DependsOnFactIDs: []string{}})
			if len(candidates) >= limit {
				return domain.KnowledgeExtractionPackage{SchemaVersion: "1.0", Candidates: candidates, Warnings: []string{}}
			}
		}
	}
	return domain.KnowledgeExtractionPackage{SchemaVersion: "1.0", Candidates: candidates, Warnings: []string{}}
}
