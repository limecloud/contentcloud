package workbench

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/limecloud/contentcloud/internal/platform/fault"
)

const ManifestSchema = "https://contentcloud.run/schemas/workbench-plugin/1.0.0/workbench-plugin.schema.json"

var (
	identifierPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)
	actionPattern     = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._-]*[a-z0-9])?$`)
	versionPattern    = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
)

var (
	approvedLayouts   = []string{"stage-canvas-context", "article-editor", "product-variants", "novel-editor"}
	approvedDensities = []string{"compact", "comfortable", "dense"}
	approvedThemes    = []string{"signal-blue", "editorial-green", "production-coral", "review-rose"}
	approvedIcons     = []string{"archive", "book-open", "file-check", "file-text", "folder", "image", "lightbulb", "list-checks", "package-check", "pen-line", "scroll-text", "shopping-bag", "tags", "video"}
)

// ActionContract is the closed platform action boundary for workbench
// manifests. A workbench can label a stage with one of these operations, but
// it cannot invent a second write API or state transition in the browser.
type ActionContract struct {
	ID      string `json:"id"`
	Owner   string `json:"owner"`
	Command string `json:"command"`
}

var approvedActionContracts = map[string]ActionContract{
	"approve_article":    {ID: "approve_article", Owner: "review", Command: "submission.review.decide"},
	"approve_chapter":    {ID: "approve_chapter", Owner: "review", Command: "submission.review.decide"},
	"approve_final":      {ID: "approve_final", Owner: "review", Command: "submission.review.decide"},
	"approve_offer":      {ID: "approve_offer", Owner: "review", Command: "submission.review.decide"},
	"approve_storyboard": {ID: "approve_storyboard", Owner: "review", Command: "submission.review.decide"},
	"attach_sources":     {ID: "attach_sources", Owner: "source", Command: "studio.task.attach_inputs"},
	"create_delivery":    {ID: "create_delivery", Owner: "delivery", Command: "delivery.package.create"},
	"create_variants":    {ID: "create_variants", Owner: "runtime", Command: "work_task.stage.start"},
	"save_brief":         {ID: "save_brief", Owner: "work", Command: "studio.task.create"},
	"save_draft":         {ID: "save_draft", Owner: "submission", Command: "submission.revision.create"},
	"save_product":       {ID: "save_product", Owner: "work", Command: "studio.task.attach_inputs"},
	"select_candidate":   {ID: "select_candidate", Owner: "review", Command: "media.review.decide"},
	"submit_script":      {ID: "submit_script", Owner: "submission", Command: "submission.revision.create"},
	"validate_channel":   {ID: "validate_channel", Owner: "delivery", Command: "delivery.package.validate"},
}

// ApprovedActionContracts returns a defensive copy for control-plane
// diagnostics and contract generation.
func ApprovedActionContracts() []ActionContract {
	result := make([]ActionContract, 0, len(approvedActionContracts))
	for _, contract := range approvedActionContracts {
		result = append(result, contract)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

// ApprovedUIValues returns the closed declarative UI vocabulary. Keeping the
// values server-owned prevents a plugin from smuggling executable renderer or
// unreviewed theme semantics into the customer surface.
func ApprovedUIValues() (layouts, densities, themes, icons []string) {
	return append([]string(nil), approvedLayouts...), append([]string(nil), approvedDensities...), append([]string(nil), approvedThemes...), append([]string(nil), approvedIcons...)
}

// Manifest is the declarative contract for a business workbench plugin.
// It describes customer-facing composition only; execution remains owned by
// the published ExperienceTemplate and the shared Runtime.
type Manifest struct {
	Schema       string        `json:"$schema"`
	ID           string        `json:"id"`
	Version      string        `json:"version"`
	Name         string        `json:"name"`
	ContentTypes []string      `json:"content_types"`
	Experience   ExperienceRef `json:"experience"`
	UI           UIManifest    `json:"ui"`
}

type ExperienceRef struct {
	TemplateID string `json:"template_id"`
}

type UIManifest struct {
	Renderer   string           `json:"renderer"`
	Layout     string           `json:"layout"`
	Density    string           `json:"density"`
	Theme      string           `json:"theme"`
	Navigation []NavigationItem `json:"navigation"`
	Stages     []Stage          `json:"stages"`
	Panels     []Panel          `json:"panels,omitempty"`
}

type NavigationItem struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Icon  string `json:"icon"`
}

type Stage struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	Outcome       string `json:"outcome"`
	PrimaryAction string `json:"primary_action"`
}

// Panel is a declarative business surface block. It changes customer-facing
// composition while remaining inside a platform-approved renderer.
type Panel struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Detail      string   `json:"detail"`
	Tone        string   `json:"tone"`
	Icon        string   `json:"icon"`
	StageIDs    []string `json:"stage_ids"`
	Target      string   `json:"target"`
	ActionLabel string   `json:"action_label"`
}

// DecodeManifest validates the plugin boundary before any registry or UI
// projection can consume it. Unknown fields fail closed so the contract does
// not silently grow executable escape hatches.
func DecodeManifest(body []byte) (Manifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fault.Invalid("WORKBENCH_PLUGIN_MANIFEST_INVALID", fmt.Sprintf("workbench plugin manifest is invalid: %v", err))
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Manifest{}, invalid("workbench plugin manifest must contain one JSON value")
		}
		return Manifest{}, fault.Invalid("WORKBENCH_PLUGIN_MANIFEST_INVALID", fmt.Sprintf("workbench plugin manifest has trailing data: %v", err))
	}
	if err := validateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func validateManifest(manifest Manifest) error {
	if manifest.Schema != ManifestSchema {
		return invalid("$schema must target the ContentCloud workbench plugin 1.0.0 schema")
	}
	if !validIdentifier(manifest.ID) {
		return invalid("id must be a lowercase stable identifier")
	}
	if !versionPattern.MatchString(manifest.Version) {
		return invalid("version must use major.minor.patch")
	}
	if strings.TrimSpace(manifest.Name) == "" {
		return invalid("name must not be empty")
	}
	if len(manifest.ContentTypes) == 0 || len(manifest.ContentTypes) > 8 {
		return invalid("content_types must contain between 1 and 8 content types")
	}
	seenContentTypes := map[string]struct{}{}
	for _, contentType := range manifest.ContentTypes {
		if strings.TrimSpace(contentType) == "" {
			return invalid("content_types must not contain empty values")
		}
		if _, exists := seenContentTypes[contentType]; exists {
			return invalid("content_types must be unique")
		}
		seenContentTypes[contentType] = struct{}{}
	}
	if strings.TrimSpace(manifest.Experience.TemplateID) == "" {
		return invalid("experience.template_id must not be empty")
	}
	if manifest.UI.Renderer != "approved" {
		return invalid("ui.renderer must be approved")
	}
	if !contains(approvedLayouts, manifest.UI.Layout) {
		return invalid("ui.layout is not an approved workbench layout")
	}
	if !contains(approvedDensities, manifest.UI.Density) {
		return invalid("ui.density is not supported")
	}
	if !contains(approvedThemes, manifest.UI.Theme) {
		return invalid("ui.theme is not an approved workbench theme")
	}
	if len(manifest.UI.Navigation) == 0 || len(manifest.UI.Navigation) > 8 {
		return invalid("ui.navigation must contain between 1 and 8 items")
	}
	if len(manifest.UI.Stages) == 0 || len(manifest.UI.Stages) > 16 {
		return invalid("ui.stages must contain between 1 and 16 stages")
	}
	if err := validateNavigation(manifest.UI.Navigation); err != nil {
		return err
	}
	if err := validateStages(manifest.UI.Stages); err != nil {
		return err
	}
	return validatePanels(manifest.UI.Panels, manifest.UI.Stages)
}

func validateNavigation(items []NavigationItem) error {
	seen := map[string]struct{}{}
	for _, item := range items {
		if !validIdentifier(item.ID) {
			return invalid("ui.navigation.id must be a lowercase stable identifier")
		}
		if strings.TrimSpace(item.Label) == "" || !contains(approvedIcons, item.Icon) {
			return invalid("ui.navigation items require label and icon")
		}
		if _, exists := seen[item.ID]; exists {
			return invalid("ui.navigation ids must be unique")
		}
		seen[item.ID] = struct{}{}
	}
	return nil
}

func validateStages(stages []Stage) error {
	seen := map[string]struct{}{}
	for _, stage := range stages {
		if !validIdentifier(stage.ID) {
			return invalid("ui.stages.id must be a lowercase stable identifier")
		}
		if strings.TrimSpace(stage.Label) == "" || strings.TrimSpace(stage.Outcome) == "" || !validAction(stage.PrimaryAction) {
			return invalid("ui.stages require label, outcome and a stable primary_action")
		}
		if _, exists := approvedActionContracts[stage.PrimaryAction]; !exists {
			return invalid("ui.stages.primary_action is not an approved platform action")
		}
		if _, exists := seen[stage.ID]; exists {
			return invalid("ui.stages ids must be unique")
		}
		seen[stage.ID] = struct{}{}
	}
	return nil
}

func validatePanels(panels []Panel, stages []Stage) error {
	if len(panels) > 8 {
		return invalid("ui.panels must contain at most 8 panels")
	}
	if len(panels) == 0 {
		return nil
	}
	stageIDs := map[string]struct{}{}
	for _, stage := range stages {
		stageIDs[stage.ID] = struct{}{}
	}
	seen := map[string]struct{}{}
	for _, panel := range panels {
		if !validIdentifier(panel.ID) || strings.TrimSpace(panel.Title) == "" || strings.TrimSpace(panel.Detail) == "" || !contains(approvedIcons, panel.Icon) || strings.TrimSpace(panel.ActionLabel) == "" {
			return invalid("ui.panels require id, title, detail, icon and action_label")
		}
		if !contains([]string{"source", "knowledge", "strategy", "production", "review"}, panel.Tone) {
			return invalid("ui.panels.tone is not supported")
		}
		if !contains([]string{"start", "tasks", "assets", "deliveries"}, panel.Target) {
			return invalid("ui.panels.target is not supported")
		}
		if len(panel.StageIDs) > 8 {
			return invalid("ui.panels.stage_ids must contain at most 8 stages")
		}
		for _, stageID := range panel.StageIDs {
			if _, exists := stageIDs[stageID]; !exists {
				return invalid("ui.panels.stage_ids must reference ui.stages")
			}
		}
		if _, exists := seen[panel.ID]; exists {
			return invalid("ui.panels ids must be unique")
		}
		seen[panel.ID] = struct{}{}
	}
	return nil
}

func validIdentifier(value string) bool {
	return len(value) > 0 && len(value) <= 64 && identifierPattern.MatchString(value) && !strings.Contains(value, "--") && !strings.Contains(value, "..")
}

func validAction(value string) bool {
	return len(value) > 0 && len(value) <= 64 && actionPattern.MatchString(value) && !strings.Contains(value, "__") && !strings.Contains(value, "..")
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func invalid(message string) error {
	return fault.Invalid("WORKBENCH_PLUGIN_MANIFEST_INVALID", message)
}
