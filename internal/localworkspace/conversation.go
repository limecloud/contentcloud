package localworkspace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/environment"
)

const VideoProductionProfileID = "contentcloud.video-production"

type BootstrapHandoff struct {
	SchemaVersion     string    `json:"schema_version"`
	Kind              string    `json:"kind"`
	Status            string    `json:"status"`
	WorkspaceID       string    `json:"workspace_id"`
	ProjectID         string    `json:"project_id"`
	PluginID          string    `json:"plugin_id"`
	PluginVersion     string    `json:"plugin_version"`
	MarketplaceRef    string    `json:"marketplace_ref"`
	EnvironmentDigest string    `json:"environment_digest"`
	NextCapabilityID  string    `json:"next_capability_id"`
	NextAction        string    `json:"next_action"`
	CreatedAt         time.Time `json:"created_at"`
}

type WorkspaceResolution struct {
	Root   string `json:"root"`
	Source string `json:"source"`
}

type WorkspaceRunSummary struct {
	RunID           string          `json:"run_id"`
	ContextRevision uint64          `json:"context_revision"`
	Intent          string          `json:"intent"`
	Stage           string          `json:"stage"`
	Status          string          `json:"status"`
	Claim           RunClaimSummary `json:"claim"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type WorkspaceHandoffSummary struct {
	HandoffID        string `json:"handoff_id"`
	RunID            string `json:"run_id"`
	Status           string `json:"status"`
	ContextRevision  uint64 `json:"context_revision"`
	NextCapabilityID string `json:"next_capability_id"`
	NextAction       string `json:"next_action"`
}

type WorkspaceConversationContext struct {
	SchemaVersion         string                    `json:"schema_version"`
	WorkspaceID           string                    `json:"workspace_id"`
	ProjectID             string                    `json:"project_id"`
	ProfileID             string                    `json:"profile_id"`
	Root                  string                    `json:"root"`
	ResolutionSource      string                    `json:"resolution_source"`
	EnvironmentHealth     string                    `json:"environment_health"`
	ContentTypes          []string                  `json:"content_types"`
	BootstrapHandoff      *BootstrapHandoff         `json:"bootstrap_handoff,omitempty"`
	ActiveRuns            []WorkspaceRunSummary     `json:"active_runs"`
	ReadyHandoffs         []WorkspaceHandoffSummary `json:"ready_handoffs"`
	PendingLocalDecisions []string                  `json:"pending_local_decisions"`
	CachedApprovedInputs  []string                  `json:"cached_approved_inputs"`
	ReviewInboxCount      int                       `json:"review_inbox_count"`
	LastCloudPullAt       *time.Time                `json:"last_cloud_pull_at,omitempty"`
	SuggestedIntents      []string                  `json:"suggested_intents"`
	Onboarding            WorkspaceOnboarding       `json:"onboarding"`
	Offline               bool                      `json:"offline"`
	GeneratedAt           time.Time                 `json:"generated_at"`
}

func ResolveWorkspaceRoot(directory, cwd string) (WorkspaceResolution, error) {
	requested := strings.TrimSpace(directory)
	source := "explicit_directory"
	if requested == "" {
		source = "process_cwd"
		requested = strings.TrimSpace(cwd)
		if requested == "" {
			var err error
			requested, err = os.Getwd()
			if err != nil {
				return WorkspaceResolution{}, err
			}
		}
	}
	root, err := FindRoot(requested)
	if err != nil {
		if source == "process_cwd" {
			notFound := domain.NotFound("唯一 ContentCloud 工作区")
			notFound.Hint = "请在 ContentCloud Workspace 中打开 Codex，或向工具显式传入 workspace directory"
			return WorkspaceResolution{}, notFound
		}
		return WorkspaceResolution{}, err
	}
	if canonical, evalErr := filepath.EvalSymlinks(root); evalErr == nil {
		root = canonical
	}
	return WorkspaceResolution{Root: root, Source: source}, nil
}

func ConversationContext(directory, cwd string, now time.Time) (WorkspaceConversationContext, error) {
	generatedAt := now.UTC()
	if now.IsZero() {
		generatedAt = time.Now().UTC()
	}
	resolution, err := ResolveWorkspaceRoot(directory, cwd)
	if err != nil {
		return WorkspaceConversationContext{}, err
	}
	status, err := LoadStatus(resolution.Root)
	if err != nil {
		return WorkspaceConversationContext{}, err
	}
	doctor, err := Doctor(resolution.Root)
	if err != nil {
		return WorkspaceConversationContext{}, err
	}
	runs, err := activeLocalRuns(resolution.Root, generatedAt)
	if err != nil {
		return WorkspaceConversationContext{}, err
	}
	var bootstrapHandoff *BootstrapHandoff
	if len(runs) == 0 {
		bootstrapHandoff, err = loadBootstrapHandoff(resolution.Root)
		if err != nil {
			return WorkspaceConversationContext{}, err
		}
	}
	handoffs, err := readyHandoffSummaries(resolution.Root)
	if err != nil {
		return WorkspaceConversationContext{}, err
	}
	decisions, err := localBundleIDs(filepath.Join(resolution.Root, ".contentcloud", "inbox", "decision-deltas"))
	if err != nil {
		return WorkspaceConversationContext{}, err
	}
	approvedSnapshots, err := ApprovedSnapshotInbox(resolution.Root, "")
	if err != nil {
		return WorkspaceConversationContext{}, err
	}
	approved := make([]string, 0, len(approvedSnapshots))
	for _, snapshot := range approvedSnapshots {
		approved = append(approved, snapshot.ID)
	}
	health := "ready"
	if !doctor.OK {
		health = "repair_required"
	}
	onboarding, err := DeriveWorkspaceOnboarding(resolution.Root, status, runs, handoffs, approved)
	if err != nil {
		return WorkspaceConversationContext{}, err
	}
	intents := suggestedWorkspaceIntents(status, runs, approved)
	if onboarding.State == OnboardingNeedsProjectBrief {
		intents = []string{"project_brief"}
	} else if bootstrapHandoff != nil {
		intents = append([]string{"bootstrap_continue"}, intents...)
	}
	if bootstrapHandoff != nil && onboarding.State == OnboardingNeedsProjectBrief {
		copy := *bootstrapHandoff
		copy.NextAction = "先调用 workspace_context；当前唯一业务下一步是确认项目简报，完成后只按 onboarding.next_step 继续。"
		bootstrapHandoff = &copy
	}
	return WorkspaceConversationContext{
		SchemaVersion:         "1.0",
		WorkspaceID:           status.Binding.WorkspaceID,
		ProjectID:             status.Binding.ProjectID,
		ProfileID:             VideoProductionProfileID,
		Root:                  resolution.Root,
		ResolutionSource:      resolution.Source,
		EnvironmentHealth:     health,
		ContentTypes:          []string{},
		BootstrapHandoff:      bootstrapHandoff,
		ActiveRuns:            runs,
		ReadyHandoffs:         handoffs,
		PendingLocalDecisions: decisions,
		CachedApprovedInputs:  approved,
		ReviewInboxCount:      status.PendingFeedbackCount,
		LastCloudPullAt:       status.Sync.LastPulledAt,
		SuggestedIntents:      intents,
		Onboarding:            onboarding,
		Offline:               true,
		GeneratedAt:           generatedAt,
	}, nil
}

func ConversationContextWithEnvironment(directory, cwd string, now time.Time, verifier *environment.Verifier, registryVerifier *environment.RegistryVerifier) (WorkspaceConversationContext, error) {
	context, err := ConversationContext(directory, cwd, now)
	if err != nil {
		return WorkspaceConversationContext{}, err
	}
	if !EnvironmentCheck(context.Root, verifier, registryVerifier, now).OK {
		context.EnvironmentHealth = "repair_required"
	}
	return context, nil
}

func StoreBootstrapHandoff(root, pluginID, pluginVersion, marketplaceRef string, now time.Time) (BootstrapHandoff, string, error) {
	if strings.TrimSpace(pluginID) == "" || strings.TrimSpace(pluginVersion) == "" || strings.TrimSpace(marketplaceRef) == "" {
		return BootstrapHandoff{}, "", domain.Invalid("BOOTSTRAP_HANDOFF_SPEC_INVALID", "Bootstrap handoff 需要固定 Plugin ID、版本和 Marketplace ref")
	}
	status, err := LoadStatus(root)
	if err != nil {
		return BootstrapHandoff{}, "", err
	}
	createdAt := now.UTC()
	if now.IsZero() {
		createdAt = time.Now().UTC()
	}
	templateJSON, err := json.Marshal(status.Template)
	if err != nil {
		return BootstrapHandoff{}, "", err
	}
	handoff := BootstrapHandoff{
		SchemaVersion:     "1.0",
		Kind:              "bootstrap_handoff",
		Status:            "ready",
		WorkspaceID:       status.Binding.WorkspaceID,
		ProjectID:         status.Binding.ProjectID,
		PluginID:          pluginID,
		PluginVersion:     pluginVersion,
		MarketplaceRef:    marketplaceRef,
		EnvironmentDigest: digest(templateJSON),
		NextCapabilityID:  "contentcloud-workspace",
		NextAction:        "先调用工作区上下文工具（workspace_context）；如果还没有项目简报，先确认项目简报，完成后只按 onboarding.next_step 继续。",
		CreatedAt:         createdAt,
	}
	path := filepath.Join(status.Root, ".contentcloud", "bootstrap-handoff.json")
	if err := replaceJSON(path, handoff, 0o600); err != nil {
		return BootstrapHandoff{}, "", err
	}
	return handoff, path, nil
}

func loadBootstrapHandoff(root string) (*BootstrapHandoff, error) {
	path := filepath.Join(root, ".contentcloud", "bootstrap-handoff.json")
	var handoff BootstrapHandoff
	if err := readJSON(path, &handoff); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	if handoff.SchemaVersion != "1.0" || handoff.Kind != "bootstrap_handoff" || handoff.Status != "ready" || handoff.WorkspaceID == "" || handoff.ProjectID == "" || handoff.PluginID == "" || handoff.PluginVersion == "" || handoff.MarketplaceRef == "" || handoff.EnvironmentDigest == "" || handoff.NextCapabilityID == "" || handoff.NextAction == "" || handoff.CreatedAt.IsZero() {
		return nil, domain.Invalid("BOOTSTRAP_HANDOFF_INVALID", "Bootstrap handoff 文件无效")
	}
	return &handoff, nil
}

func activeLocalRuns(root string, now time.Time) ([]WorkspaceRunSummary, error) {
	paths, err := filepath.Glob(filepath.Join(root, "40-work", "runs", "*", "context.json"))
	if err != nil {
		return nil, err
	}
	runs := make([]WorkspaceRunSummary, 0, len(paths))
	for _, path := range paths {
		var run LocalRunContext
		if err := readJSON(path, &run); err != nil {
			return nil, err
		}
		if problems := validateLocalRun(run); len(problems) > 0 {
			invalid := domain.Invalid("LOCAL_RUN_CONTEXT_INVALID", "LocalRunContext 校验失败")
			invalid.Details = map[string]any{"path": path, "errors": problems}
			return nil, invalid
		}
		if run.Status == "completed" {
			continue
		}
		claim, err := RunClaimStatus(root, run.RunID, now)
		if err != nil {
			return nil, err
		}
		runs = append(runs, WorkspaceRunSummary{RunID: run.RunID, ContextRevision: run.ContextRevision, Intent: run.Intent, Stage: run.Stage, Status: run.Status, Claim: claim, UpdatedAt: run.UpdatedAt})
	}
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].UpdatedAt.Equal(runs[j].UpdatedAt) {
			return runs[i].RunID < runs[j].RunID
		}
		return runs[i].UpdatedAt.After(runs[j].UpdatedAt)
	})
	return runs, nil
}

func readyHandoffSummaries(root string) ([]WorkspaceHandoffSummary, error) {
	records, err := ListReadyHandoffs(root)
	if err != nil {
		return nil, err
	}
	summaries := make([]WorkspaceHandoffSummary, 0, len(records))
	for _, record := range records {
		summaries = append(summaries, WorkspaceHandoffSummary{
			HandoffID:        record.HandoffID,
			RunID:            record.RunID,
			Status:           record.Status,
			ContextRevision:  record.ContextRevision,
			NextCapabilityID: record.NextCapabilityID,
			NextAction:       record.NextAction,
		})
	}
	return summaries, nil
}

func localBundleIDs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		ids = append(ids, strings.TrimSuffix(entry.Name(), ".json"))
	}
	sort.Strings(ids)
	return ids, nil
}

func localDirectoryIDs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			ids = append(ids, entry.Name())
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func suggestedWorkspaceIntents(status Status, runs []WorkspaceRunSummary, approved []string) []string {
	intents := []string{}
	if len(runs) > 0 {
		intents = append(intents, "continue_run")
	}
	if status.SourceCount == 0 {
		intents = append(intents, "source_intake")
	} else {
		intents = append(intents, "knowledge_extraction")
	}
	if len(approved) > 0 {
		intents = append(intents, "marketing_video_script")
	}
	if status.PendingFeedbackCount > 0 {
		intents = append(intents, "review_revision")
	}
	return intents
}
