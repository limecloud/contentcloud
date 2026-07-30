package contracts

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"github.com/limecloud/contentcloud/internal/agentadapter"
)

func TestEmbeddedSchemasAreValidJSON(t *testing.T) {
	for name, body := range map[string][]byte{
		"workspace-3.0":                     WorkspaceV3Schema,
		"source-registry-3.0":               SourceRegistryV3Schema,
		"knowledge-page-3.0":                KnowledgePageV3Schema,
		"knowledge-pack-3.0":                KnowledgePackV3Schema,
		"local-run-3.0":                     LocalRunV3Schema,
		"handoff-1.0":                       HandoffV1Schema,
		"content-batch-3.0":                 ContentBatchV3Schema,
		"content-item-3.0":                  ContentItemV3Schema,
		"article-brief-1.0":                 ArticleBriefV1Schema,
		"article-1.0":                       ArticleV1Schema,
		"wechat-delivery-1.0":               WeChatDeliveryV1Schema,
		"brief-3.0":                         BriefV3Schema,
		"creative-directions-3.0":           CreativeDirectionsV3Schema,
		"submission-bundle-3.0":             SubmissionBundleV3Schema,
		"knowledge-candidates-1.0":          KnowledgeCandidatesSchema,
		"task-contract-1.0":                 TaskContractSchema,
		"creative-environment-manifest-1.0": CreativeEnvironmentManifestSchema,
		"creative-environment-profile-1.0":  CreativeEnvironmentProfileSchema,
		"environment-lock-1.0":              EnvironmentLockSchema,
		"environment-trusted-keys-1.0":      EnvironmentTrustedKeysSchema,
		"environment-preparation-plan-1.0":  EnvironmentPreparationPlanSchema,
		"local-execution-plan-1.0":          LocalExecutionPlanSchema,
		"creative-execution-bundle-1.0":     CreativeExecutionBundleSchema,
		"audience-taxonomy-1.0":             AudienceTaxonomyV1Schema,
		"audience-strategy-1.0":             AudienceStrategyV1Schema,
		"commerce-offer-1.0":                CommerceOfferV1Schema,
		"storyboard-package-1.0":            StoryboardPackageV1Schema,
		"seedance-prompt-package-1.0":       SeedancePromptPackageV1Schema,
		"published-creative-binding-1.0":    PublishedCreativeBindingV1Schema,
	} {
		var schema map[string]any
		if len(body) == 0 || json.Unmarshal(body, &schema) != nil || schema["$id"] == "" {
			t.Fatalf("embedded schema %s is missing or invalid", name)
		}
	}
}

func TestEnvironmentSchemasReserveRegisteredAgentClients(t *testing.T) {
	want := agentadapter.ClientIDs()
	for name, body := range map[string][]byte{
		"profile":  CreativeEnvironmentProfileSchema,
		"manifest": CreativeEnvironmentManifestSchema,
		"lock":     EnvironmentLockSchema,
	} {
		var schema struct {
			Properties struct {
				Harness struct {
					Enum []string `json:"enum"`
				} `json:"harness"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(body, &schema); err != nil {
			t.Fatal(err)
		}
		got := schema.Properties.Harness.Enum
		sort.Strings(got)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s harness enum=%v, want registry IDs %v", name, got, want)
		}
	}
}

func TestEmbeddedProjectPageContractIsValidJSON(t *testing.T) {
	var contract struct {
		SchemaVersion string                     `json:"schema_version"`
		Order         []string                   `json:"order"`
		Views         map[string]json.RawMessage `json:"views"`
	}
	if err := json.Unmarshal(ProjectPagesV1Contract, &contract); err != nil {
		t.Fatal(err)
	}
	if contract.SchemaVersion != "contentcloud.project-pages/1.0" || len(contract.Order) == 0 || len(contract.Order) != len(contract.Views) {
		t.Fatalf("embedded project page contract is incomplete: %#v", contract)
	}
	for _, view := range contract.Order {
		if len(contract.Views[view]) == 0 {
			t.Fatalf("project page %q is missing from views", view)
		}
	}
}
