package localworkspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/limecloud/contentcloud/internal/domain"
	"gopkg.in/yaml.v3"
)

const (
	WorkspaceProposalSchemaVersion = "contentcloud.workspace-proposal/1.0"
	workspaceProposalTTL           = 10 * time.Minute
	workspaceProposalMaxBytes      = 2 * 1024 * 1024
)

type WorkspaceProposal struct {
	SchemaVersion       string                     `json:"schema_version"`
	ProposalID          string                     `json:"proposal_id"`
	WorkspaceID         string                     `json:"workspace_id"`
	ProjectID           string                     `json:"project_id"`
	RunID               string                     `json:"run_id"`
	OwnerKind           string                     `json:"owner_kind"`
	OwnerID             string                     `json:"owner_id"`
	OwnerEpoch          uint64                     `json:"owner_epoch"`
	BaseContextRevision uint64                     `json:"base_context_revision"`
	BaseFileDigests     []WorkspaceProposalFile    `json:"base_file_digests"`
	TypedAction         string                     `json:"typed_action"`
	ValidatedArguments  WorkspaceProposalArguments `json:"validated_arguments"`
	AffectedPaths       []string                   `json:"affected_paths"`
	Effects             []WorkspaceProposalEffect  `json:"effects"`
	Checks              []WorkspaceProposalCheck   `json:"checks"`
	CreatedAt           time.Time                  `json:"created_at"`
	ExpiresAt           time.Time                  `json:"expires_at"`
	proposedBody        []byte
}

type WorkspaceProposalFile struct {
	Ref    string `json:"ref"`
	Digest string `json:"digest"`
}

type WorkspaceProposalArguments struct {
	Ref           string `json:"ref"`
	ContentDigest string `json:"content_digest"`
	ByteSize      int64  `json:"byte_size"`
}

type WorkspaceProposalEffect struct {
	Operation    string `json:"operation"`
	Ref          string `json:"ref"`
	BeforeDigest string `json:"before_digest"`
	AfterDigest  string `json:"after_digest"`
	BeforeBytes  int64  `json:"before_bytes"`
	AfterBytes   int64  `json:"after_bytes"`
}

type WorkspaceProposalCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type PrepareWorkspaceProposalOptions struct {
	Root                    string
	RunID                   string
	ClaimToken              string
	OwnerKind               string
	OwnerID                 string
	OwnerEpoch              uint64
	ExpectedContextRevision uint64
	TypedAction             string
	Ref                     string
	ExpectedDigest          string
	Content                 string
	Now                     time.Time
}

type ApplyWorkspaceProposalOptions struct {
	Root                    string
	Proposal                WorkspaceProposal
	ClaimToken              string
	OwnerKind               string
	OwnerID                 string
	OwnerEpoch              uint64
	ExpectedContextRevision uint64
	Now                     time.Time
}

type WorkspaceProposalApplyResult struct {
	SchemaVersion   string                  `json:"schema_version"`
	ProposalID      string                  `json:"proposal_id"`
	RunID           string                  `json:"run_id"`
	ContextRevision uint64                  `json:"context_revision"`
	Applied         bool                    `json:"applied"`
	Outputs         []WorkspaceProposalFile `json:"outputs"`
	AppliedAt       time.Time               `json:"applied_at"`
}

type ProposalStore struct {
	mu          sync.Mutex
	commandMu   sync.Mutex
	proposals   map[string]WorkspaceProposal
	idempotency map[string]proposalIdempotencyRecord
}

type proposalIdempotencyRecord struct {
	Operation   string
	Fingerprint string
	Value       any
}

func NewProposalStore() *ProposalStore {
	return &ProposalStore{proposals: map[string]WorkspaceProposal{}, idempotency: map[string]proposalIdempotencyRecord{}}
}

func (s *ProposalStore) PrepareIdempotent(key string, options PrepareWorkspaceProposalOptions) (WorkspaceProposal, error) {
	s.commandMu.Lock()
	defer s.commandMu.Unlock()
	fingerprint, err := proposalIdempotencyFingerprint(struct {
		Root, RunID, ClaimToken, OwnerKind, OwnerID, TypedAction, Ref, ExpectedDigest, Content string
		OwnerEpoch, ExpectedContextRevision                                                    uint64
	}{options.Root, options.RunID, options.ClaimToken, options.OwnerKind, options.OwnerID, options.TypedAction, options.Ref, options.ExpectedDigest, options.Content, options.OwnerEpoch, options.ExpectedContextRevision})
	if err != nil {
		return WorkspaceProposal{}, err
	}
	if replay, found, err := s.idempotentValue(key, "prepare", fingerprint); err != nil {
		return WorkspaceProposal{}, err
	} else if found {
		return replay.(WorkspaceProposal), nil
	}
	proposal, err := s.Prepare(options)
	if err != nil {
		return WorkspaceProposal{}, err
	}
	s.storeIdempotentValue(key, "prepare", fingerprint, proposal)
	return proposal, nil
}

func (s *ProposalStore) Prepare(options PrepareWorkspaceProposalOptions) (WorkspaceProposal, error) {
	proposal, err := PrepareWorkspaceProposal(options)
	if err != nil {
		return WorkspaceProposal{}, err
	}
	s.mu.Lock()
	s.proposals[proposal.ProposalID] = proposal
	s.mu.Unlock()
	return proposal, nil
}

func (s *ProposalStore) ApplyIdempotent(key, proposalID string, options ApplyWorkspaceProposalOptions) (WorkspaceProposalApplyResult, error) {
	s.commandMu.Lock()
	defer s.commandMu.Unlock()
	fingerprint, err := proposalIdempotencyFingerprint(struct {
		ProposalID, Root, ClaimToken, OwnerKind, OwnerID string
		OwnerEpoch, ExpectedContextRevision              uint64
	}{proposalID, options.Root, options.ClaimToken, options.OwnerKind, options.OwnerID, options.OwnerEpoch, options.ExpectedContextRevision})
	if err != nil {
		return WorkspaceProposalApplyResult{}, err
	}
	if replay, found, err := s.idempotentValue(key, "apply", fingerprint); err != nil {
		return WorkspaceProposalApplyResult{}, err
	} else if found {
		return replay.(WorkspaceProposalApplyResult), nil
	}
	result, err := s.Apply(proposalID, options)
	if err != nil {
		return WorkspaceProposalApplyResult{}, err
	}
	s.storeIdempotentValue(key, "apply", fingerprint, result)
	return result, nil
}

func (s *ProposalStore) Apply(proposalID string, options ApplyWorkspaceProposalOptions) (WorkspaceProposalApplyResult, error) {
	s.mu.Lock()
	proposal, ok := s.proposals[strings.TrimSpace(proposalID)]
	if ok {
		delete(s.proposals, proposal.ProposalID)
	}
	s.mu.Unlock()
	if !ok {
		return WorkspaceProposalApplyResult{}, domain.NotFound("Workspace Proposal")
	}
	options.Proposal = proposal
	return ApplyWorkspaceProposal(options)
}

func (s *ProposalStore) Clear() {
	s.mu.Lock()
	s.proposals = map[string]WorkspaceProposal{}
	s.idempotency = map[string]proposalIdempotencyRecord{}
	s.mu.Unlock()
}

func (s *ProposalStore) idempotentValue(key, operation, fingerprint string) (any, bool, error) {
	if len(strings.TrimSpace(key)) < 8 || len(strings.TrimSpace(key)) > 128 {
		return nil, false, domain.Invalid("WORKSPACE_IDEMPOTENCY_KEY_INVALID", "idempotency_key 必须包含 8 到 128 个字符")
	}
	s.mu.Lock()
	record, found := s.idempotency[key]
	s.mu.Unlock()
	if !found {
		return nil, false, nil
	}
	if record.Operation != operation || record.Fingerprint != fingerprint {
		return nil, false, domain.Conflict("WORKSPACE_IDEMPOTENCY_CONFLICT", "idempotency_key 已用于不同的 Proposal 操作或参数")
	}
	return record.Value, true, nil
}

func (s *ProposalStore) storeIdempotentValue(key, operation, fingerprint string, value any) {
	s.mu.Lock()
	s.idempotency[key] = proposalIdempotencyRecord{Operation: operation, Fingerprint: fingerprint, Value: value}
	s.mu.Unlock()
}

func proposalIdempotencyFingerprint(value any) (string, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return domain.TokenHash(string(body)), nil
}

func PrepareWorkspaceProposal(options PrepareWorkspaceProposalOptions) (WorkspaceProposal, error) {
	root, err := FindRoot(options.Root)
	if err != nil {
		return WorkspaceProposal{}, err
	}
	now := localNow(options.Now)
	if strings.TrimSpace(options.TypedAction) != "workspace_file.replace" {
		return WorkspaceProposal{}, domain.Invalid("WORKSPACE_PROPOSAL_ACTION_INVALID", "typed_action 必须是 workspace_file.replace")
	}
	if _, err := ValidateRunOwnership(root, options.RunID, options.ClaimToken, options.OwnerKind, options.OwnerID, options.OwnerEpoch, options.ExpectedContextRevision, now); err != nil {
		return WorkspaceProposal{}, err
	}
	file, err := readWorkspaceViewFile(root, options.Ref)
	if err != nil {
		return WorkspaceProposal{}, err
	}
	if err := validateProposalWritableRef(file.Ref); err != nil {
		return WorkspaceProposal{}, err
	}
	if err := verifyWorkspaceViewDigest(file.Digest, options.ExpectedDigest); err != nil {
		return WorkspaceProposal{}, proposalStale("创建 Proposal 时源文件 digest 已变化", err)
	}
	proposed := []byte(options.Content)
	if err := validateProposalContent(file.Ref, file.MIMEType, proposed); err != nil {
		return WorkspaceProposal{}, err
	}
	afterDigest := workspaceDigest(proposed)
	if afterDigest == file.Digest {
		return WorkspaceProposal{}, domain.Invalid("WORKSPACE_PROPOSAL_NO_CHANGES", "草稿内容与当前文件完全相同")
	}
	status, err := LoadStatus(root)
	if err != nil {
		return WorkspaceProposal{}, err
	}
	proposal := WorkspaceProposal{
		SchemaVersion: WorkspaceProposalSchemaVersion, ProposalID: "pro_" + strings.ReplaceAll(domain.NewID(), "-", ""),
		WorkspaceID: status.Binding.WorkspaceID, ProjectID: status.Binding.ProjectID, RunID: options.RunID,
		OwnerKind: options.OwnerKind, OwnerID: options.OwnerID, OwnerEpoch: options.OwnerEpoch,
		BaseContextRevision: options.ExpectedContextRevision,
		BaseFileDigests:     []WorkspaceProposalFile{{Ref: file.Ref, Digest: file.Digest}}, TypedAction: "workspace_file.replace",
		ValidatedArguments: WorkspaceProposalArguments{Ref: file.Ref, ContentDigest: afterDigest, ByteSize: int64(len(proposed))},
		AffectedPaths:      []string{file.Ref},
		Effects:            []WorkspaceProposalEffect{{Operation: "replace", Ref: file.Ref, BeforeDigest: file.Digest, AfterDigest: afterDigest, BeforeBytes: file.Size, AfterBytes: int64(len(proposed))}},
		Checks:             []WorkspaceProposalCheck{{Name: "ownership_fence", Status: "passed"}, {Name: "context_revision", Status: "passed"}, {Name: "source_digest", Status: "passed"}, {Name: "document_schema", Status: "passed"}},
		CreatedAt:          now, ExpiresAt: now.Add(workspaceProposalTTL), proposedBody: append([]byte(nil), proposed...),
	}
	return proposal, nil
}

func ApplyWorkspaceProposal(options ApplyWorkspaceProposalOptions) (WorkspaceProposalApplyResult, error) {
	root, err := FindRoot(options.Root)
	if err != nil {
		return WorkspaceProposalApplyResult{}, err
	}
	now := localNow(options.Now)
	proposal := options.Proposal
	if err := validateWorkspaceProposal(proposal); err != nil {
		return WorkspaceProposalApplyResult{}, err
	}
	if !now.Before(proposal.ExpiresAt) {
		return WorkspaceProposalApplyResult{}, proposalStale("Proposal 已过期", nil)
	}
	if proposal.OwnerKind != options.OwnerKind || proposal.OwnerID != options.OwnerID || proposal.OwnerEpoch != options.OwnerEpoch || proposal.BaseContextRevision != options.ExpectedContextRevision {
		return WorkspaceProposalApplyResult{}, proposalStale("Apply 使用的 owner、epoch 或 revision 与 Proposal 不匹配", nil)
	}
	if _, err := ValidateRunOwnership(root, proposal.RunID, options.ClaimToken, options.OwnerKind, options.OwnerID, options.OwnerEpoch, options.ExpectedContextRevision, now); err != nil {
		return WorkspaceProposalApplyResult{}, proposalStale("Apply 时运行所有权已经变化", err)
	}
	status, err := LoadStatus(root)
	if err != nil {
		return WorkspaceProposalApplyResult{}, err
	}
	if status.Binding.WorkspaceID != proposal.WorkspaceID || status.Binding.ProjectID != proposal.ProjectID {
		return WorkspaceProposalApplyResult{}, proposalStale("Proposal 不属于当前 Workspace 或 Project", nil)
	}
	base := proposal.BaseFileDigests[0]
	file, err := readWorkspaceViewFile(root, base.Ref)
	if err != nil {
		return WorkspaceProposalApplyResult{}, proposalStale("Apply 前无法重新读取源文件", err)
	}
	if file.Digest != base.Digest {
		return WorkspaceProposalApplyResult{}, proposalStale("Apply 前源文件 digest 已变化", nil)
	}
	if workspaceDigest(proposal.proposedBody) != proposal.ValidatedArguments.ContentDigest {
		return WorkspaceProposalApplyResult{}, domain.Invalid("WORKSPACE_PROPOSAL_INVALID", "Proposal 内存草稿摘要无效")
	}
	if err := validateProposalContent(file.Ref, file.MIMEType, proposal.proposedBody); err != nil {
		return WorkspaceProposalApplyResult{}, err
	}
	info, err := os.Stat(file.Path)
	if err != nil {
		return WorkspaceProposalApplyResult{}, err
	}
	before := append([]byte(nil), file.Body...)
	if err := replaceFile(file.Path, proposal.proposedBody, info.Mode().Perm()); err != nil {
		return WorkspaceProposalApplyResult{}, err
	}
	updated, runErr := RecordClaimedLocalRun(RecordLocalRunOptions{
		Root: root, RunID: proposal.RunID, ClaimToken: options.ClaimToken,
		ExpectedRevision: options.ExpectedContextRevision, OutputPaths: []string{file.Ref}, Now: now,
	})
	if runErr != nil {
		if rollbackErr := replaceFile(file.Path, before, info.Mode().Perm()); rollbackErr != nil {
			return WorkspaceProposalApplyResult{}, fmt.Errorf("推进 LocalRun 失败且草稿回滚失败: %w", errors.Join(runErr, rollbackErr))
		}
		return WorkspaceProposalApplyResult{}, runErr
	}
	return WorkspaceProposalApplyResult{
		SchemaVersion: "contentcloud.workspace-proposal-apply/1.0", ProposalID: proposal.ProposalID, RunID: proposal.RunID,
		ContextRevision: updated.ContextRevision, Applied: true,
		Outputs: []WorkspaceProposalFile{{Ref: file.Ref, Digest: proposal.ValidatedArguments.ContentDigest}}, AppliedAt: now,
	}, nil
}

func validateWorkspaceProposal(proposal WorkspaceProposal) error {
	if proposal.SchemaVersion != WorkspaceProposalSchemaVersion || !strings.HasPrefix(proposal.ProposalID, "pro_") ||
		proposal.WorkspaceID == "" || proposal.ProjectID == "" || proposal.RunID == "" ||
		!validRunClaimOwnerKind(proposal.OwnerKind) || proposal.OwnerID == "" || proposal.OwnerEpoch == 0 ||
		proposal.BaseContextRevision == 0 || len(proposal.BaseFileDigests) != 1 || len(proposal.AffectedPaths) != 1 ||
		proposal.TypedAction != "workspace_file.replace" || proposal.ValidatedArguments.Ref != proposal.BaseFileDigests[0].Ref ||
		proposal.ValidatedArguments.Ref != proposal.AffectedPaths[0] || proposal.ValidatedArguments.ByteSize != int64(len(proposal.proposedBody)) ||
		!validSHA256Digest(proposal.BaseFileDigests[0].Digest) || !validSHA256Digest(proposal.ValidatedArguments.ContentDigest) ||
		proposal.CreatedAt.IsZero() || !proposal.ExpiresAt.After(proposal.CreatedAt) {
		return domain.Invalid("WORKSPACE_PROPOSAL_INVALID", "Proposal 结构或绑定无效")
	}
	return nil
}

func validateProposalWritableRef(ref string) error {
	clean := filepath.ToSlash(filepath.Clean(ref))
	allowed := strings.HasPrefix(clean, "40-work/") || strings.HasPrefix(clean, "50-production/")
	governed := strings.HasPrefix(clean, "40-work/runs/") || strings.HasPrefix(clean, "40-work/handoffs/")
	if !allowed || governed {
		return domain.Policy("WORKSPACE_PROPOSAL_PATH_DENIED", "Proposal 只允许修改 40-work 或 50-production 中的普通草稿文件", "选择已有草稿或生产候选文件")
	}
	return nil
}

func validateProposalContent(ref, mimeType string, body []byte) error {
	if int64(len(body)) > workspaceProposalMaxBytes {
		return workspaceFileTooLarge(int64(len(body)), workspaceProposalMaxBytes)
	}
	if !utf8.Valid(body) || !strings.HasPrefix(mimeType, "text/") && !strings.Contains(mimeType, "json") && !strings.HasSuffix(ref, ".yaml") && !strings.HasSuffix(ref, ".yml") {
		return domain.Policy("WORKSPACE_PROPOSAL_MIME_DENIED", "Proposal 只允许修改 UTF-8 文本、JSON 或 YAML 草稿", "媒体资源保持只读")
	}
	if strings.Contains(mimeType, "json") {
		var value any
		if err := json.Unmarshal(body, &value); err != nil {
			return domain.Invalid("WORKSPACE_PROPOSAL_DOCUMENT_INVALID", "Proposal 中的 JSON 无法解析")
		}
	}
	if strings.HasSuffix(ref, ".yaml") || strings.HasSuffix(ref, ".yml") {
		var value any
		if err := yaml.Unmarshal(body, &value); err != nil {
			return domain.Invalid("WORKSPACE_PROPOSAL_DOCUMENT_INVALID", "Proposal 中的 YAML 无法解析")
		}
	}
	return nil
}

func proposalStale(message string, cause error) error {
	err := domain.Conflict("WORKSPACE_PROPOSAL_STALE", message)
	err.Hint = "重新读取当前 View，创建新的 Proposal 并再次确认"
	if cause != nil {
		err.Details = map[string]any{"cause": cause.Error()}
	}
	return err
}
