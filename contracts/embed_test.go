package contracts

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"github.com/limecloud/contentcloud/internal/agentadapter"
	"gopkg.in/yaml.v3"
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

func TestEmbeddedStudioSurfaceContractIsValidJSON(t *testing.T) {
	var contract struct {
		SchemaVersion string                     `json:"schema_version"`
		Order         []string                   `json:"order"`
		Views         map[string]json.RawMessage `json:"views"`
	}
	if err := json.Unmarshal(StudioSurfacesV1Contract, &contract); err != nil {
		t.Fatal(err)
	}
	if contract.SchemaVersion != "contentcloud.studio-surfaces/1.0" || len(contract.Order) == 0 || len(contract.Order) != len(contract.Views) {
		t.Fatalf("embedded Studio surface contract is incomplete: %#v", contract)
	}
	for _, view := range contract.Order {
		if len(contract.Views[view]) == 0 {
			t.Fatalf("Studio surface %q is missing from views", view)
		}
	}
}

func TestStudioOpenAPIContractKeepsProductBoundaries(t *testing.T) {
	var document struct {
		OpenAPI    string                    `yaml:"openapi"`
		Paths      map[string]map[string]any `yaml:"paths"`
		Components struct {
			Schemas map[string]any `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(OpenAPIYAML, &document); err != nil {
		t.Fatalf("openapi.yaml is invalid or contains duplicate YAML keys: %v", err)
	}
	if document.OpenAPI != "3.1.0" {
		t.Fatalf("openapi version=%q, want 3.1.0", document.OpenAPI)
	}
	for _, path := range []string{
		"/studio/execution-clients",
		"/studio/projects/{project_id}/connect-sessions",
		"/studio/connect-sessions/{session_id}",
	} {
		if _, ok := document.Paths[path]; !ok {
			t.Fatalf("studio path %q is missing from OpenAPI", path)
		}
	}
	for _, schema := range []string{
		"StudioExecutionClientCatalogEnvelope",
		"StudioConnectSessionEnvelope",
		"StudioAssetItem",
	} {
		if _, ok := document.Components.Schemas[schema]; !ok {
			t.Fatalf("studio schema %q is missing from OpenAPI", schema)
		}
	}
}
