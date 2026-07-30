package environment

import "time"

const (
	ManifestSchemaVersion        = "1.0"
	LocalPlanSchemaVersion       = "1.0"
	ExecutionBundleSchemaVersion = "1.0"
)

type Signature struct {
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"key_id"`
	Value     string `json:"value"`
}

type PluginRef struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Version      string   `json:"version"`
	SourceRef    string   `json:"source_ref"`
	Digest       string   `json:"digest"`
	Required     bool     `json:"required"`
	Scope        string   `json:"scope"`
	Capabilities []string `json:"capabilities"`
}

type Distribution struct {
	Marketplace string      `json:"marketplace"`
	Plugins     []PluginRef `json:"plugins"`
}

type WorkspaceTemplateRef struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

type Policies struct {
	PublishRequiresConfirmation bool `json:"publish_requires_confirmation"`
	AutomationEnabled           bool `json:"automation_enabled"`
	BackgroundUpgrade           bool `json:"background_upgrade"`
}

type Manifest struct {
	SchemaVersion      string               `json:"schema_version"`
	ProjectID          string               `json:"project_id"`
	ProfileID          string               `json:"profile_id"`
	ProfileVersion     string               `json:"profile_version"`
	EnvironmentVersion string               `json:"environment_version"`
	Harness            string               `json:"harness"`
	Distribution       Distribution         `json:"distribution"`
	WorkspaceTemplate  WorkspaceTemplateRef `json:"workspace_template"`
	Capabilities       []string             `json:"capabilities"`
	ContentTypes       []string             `json:"content_types"`
	Policies           Policies             `json:"policies"`
	IssuedAt           time.Time            `json:"issued_at"`
	ExpiresAt          time.Time            `json:"expires_at"`
	Digest             string               `json:"digest"`
	Signature          Signature            `json:"signature"`
}

type ProfilePlugin struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Version      string   `json:"version"`
	Required     bool     `json:"required"`
	Scope        string   `json:"scope"`
	Capabilities []string `json:"capabilities"`
}

type Profile struct {
	ID                 string               `json:"id"`
	Version            string               `json:"version"`
	EnvironmentVersion string               `json:"environment_version"`
	Harness            string               `json:"harness"`
	Marketplace        string               `json:"marketplace"`
	Plugins            []ProfilePlugin      `json:"plugins"`
	WorkspaceTemplate  WorkspaceTemplateRef `json:"workspace_template"`
	Capabilities       []string             `json:"capabilities"`
	Policies           Policies             `json:"policies"`
}

type EnvironmentLock struct {
	SchemaVersion      string         `json:"schema_version"`
	ProjectID          string         `json:"project_id"`
	ProfileID          string         `json:"profile_id"`
	ProfileVersion     string         `json:"profile_version"`
	EnvironmentVersion string         `json:"environment_version"`
	Harness            string         `json:"harness"`
	ManifestDigest     string         `json:"manifest_digest"`
	Plugins            []LockedPlugin `json:"plugins"`
	VerifiedAt         time.Time      `json:"verified_at"`
}

type LockedPlugin struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Version   string `json:"version"`
	Digest    string `json:"digest"`
	Installed bool   `json:"installed"`
}

type LocalPlanRequest struct {
	ProjectID            string   `json:"project_id"`
	RunID                string   `json:"run_id"`
	Intent               string   `json:"intent"`
	RequiredCapabilities []string `json:"required_capabilities"`
	InputRefs            []string `json:"input_refs"`
}

type PluginPreparation struct {
	Plugin PluginRef `json:"plugin"`
	Reason string    `json:"reason"`
}

type LocalExecutionPlan struct {
	SchemaVersion        string              `json:"schema_version"`
	PlanID               string              `json:"plan_id"`
	RunID                string              `json:"run_id"`
	Intent               string              `json:"intent"`
	RequiredCapabilities []string            `json:"required_capabilities"`
	Plugins              []PluginRef         `json:"plugins"`
	Preparation          []PluginPreparation `json:"preparation"`
	InputRefs            []string            `json:"input_refs"`
	EnvironmentDigest    string              `json:"environment_digest"`
	State                string              `json:"state"`
	RequiresServer       bool                `json:"requires_server"`
}

type ExecutionSubject struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	Digest string `json:"digest"`
}

type CapabilityRequirement struct {
	ID            string `json:"id"`
	SchemaVersion string `json:"schema_version"`
	Digest        string `json:"digest"`
}

type PackRequirement struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	PluginVersion string `json:"plugin_version"`
	Digest        string `json:"digest"`
	Scope         string `json:"scope"`
	Required      bool   `json:"required"`
}

type CreativeExecutionBundle struct {
	SchemaVersion        string                  `json:"schema_version"`
	BundleID             string                  `json:"bundle_id"`
	ProjectID            string                  `json:"project_id"`
	ProfileID            string                  `json:"profile_id"`
	EnvironmentVersion   string                  `json:"environment_version"`
	Subject              ExecutionSubject        `json:"subject"`
	RequiredCapabilities []CapabilityRequirement `json:"required_capabilities"`
	Packs                []PackRequirement       `json:"packs"`
	IssuedAt             time.Time               `json:"issued_at"`
	ExpiresAt            time.Time               `json:"expires_at"`
	Digest               string                  `json:"digest"`
	Signature            Signature               `json:"signature"`
}

type ExecutionBundleRequest struct {
	ProjectID            string
	ContentTypes         []string
	Subject              ExecutionSubject
	RequiredCapabilities []CapabilityRequirement
	PackIDs              []string
}

type BundleVerifyOptions struct {
	ProjectID       string
	ExpectedSubject ExecutionSubject
	Now             time.Time
}

type CapabilityPreparation struct {
	Capability CapabilityRequirement `json:"capability"`
	Reason     string                `json:"reason"`
}

type BundleResolution struct {
	BundleID              string                  `json:"bundle_id"`
	State                 string                  `json:"state"`
	Packs                 []PackRequirement       `json:"packs"`
	PluginPreparation     []PluginPreparation     `json:"plugin_preparation"`
	CapabilityPreparation []CapabilityPreparation `json:"capability_preparation"`
}
