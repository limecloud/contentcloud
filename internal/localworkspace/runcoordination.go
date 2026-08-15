package localworkspace

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

const (
	RunClaimSchemaVersion = "contentcloud.run-claim/2.0"
	HandoffSchemaVersion  = "contentcloud.handoff/1.0"
	defaultClaimTTL       = 30 * time.Minute
	maximumClaimTTL       = 4 * time.Hour
)

type RunClaim struct {
	SchemaVersion   string    `json:"schema_version"`
	RunID           string    `json:"run_id"`
	OwnerKind       string    `json:"owner_kind"`
	OwnerID         string    `json:"owner_id"`
	Epoch           uint64    `json:"epoch"`
	Token           string    `json:"token,omitempty"`
	TokenHash       string    `json:"token_hash,omitempty"`
	ContextRevision uint64    `json:"context_revision"`
	ClaimedAt       time.Time `json:"claimed_at"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type RunClaimSummary struct {
	Claimed   bool       `json:"claimed"`
	OwnerKind string     `json:"owner_kind,omitempty"`
	OwnerID   string     `json:"owner_id,omitempty"`
	Epoch     uint64     `json:"epoch,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Expired   bool       `json:"expired"`
}

type ClaimRunOptions struct {
	Root             string
	RunID            string
	OwnerKind        string
	OwnerID          string
	ExpectedRevision uint64
	TTL              time.Duration
	TakeoverExpired  bool
	Now              time.Time
}

type TakeoverRunClaimOptions struct {
	Root              string
	RunID             string
	OwnerKind         string
	OwnerID           string
	ExpectedOwnerKind string
	ExpectedOwnerID   string
	ExpectedEpoch     uint64
	ExpectedRevision  uint64
	TTL               time.Duration
	Now               time.Time
}

type HandoffInputDigest struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

type HandoffHistory struct {
	From string    `json:"from,omitempty"`
	To   string    `json:"to"`
	At   time.Time `json:"at"`
}

type HandoffRecord struct {
	SchemaVersion    string               `json:"schema_version"`
	HandoffID        string               `json:"handoff_id"`
	RunID            string               `json:"run_id"`
	Status           string               `json:"status"`
	ContextRevision  uint64               `json:"context_revision"`
	FromOwner        string               `json:"from_owner"`
	ClaimedBy        string               `json:"claimed_by,omitempty"`
	NextCapabilityID string               `json:"next_capability_id"`
	NextAction       string               `json:"next_action"`
	InputDigests     []HandoffInputDigest `json:"input_refs"`
	OutputPaths      []string             `json:"output_refs"`
	CompletedChecks  []string             `json:"completed_checks"`
	Blockers         []string             `json:"blockers"`
	PendingDecisions []string             `json:"pending_decisions"`
	History          []HandoffHistory     `json:"history"`
	CreatedAt        time.Time            `json:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at"`
	ClaimedAt        *time.Time           `json:"claimed_at,omitempty"`
	CompletedAt      *time.Time           `json:"completed_at,omitempty"`
	SupersededAt     *time.Time           `json:"superseded_at,omitempty"`
}

type CreateReadyHandoffOptions struct {
	Root             string
	HandoffID        string
	RunID            string
	ClaimToken       string
	ExpectedRevision uint64
	NextCapabilityID string
	NextAction       string
	InputPaths       []string
	Blockers         []string
	PendingDecisions []string
	Now              time.Time
}

type AcceptHandoffOptions struct {
	Root            string
	HandoffID       string
	OwnerKind       string
	OwnerID         string
	TTL             time.Duration
	TakeoverExpired bool
	Now             time.Time
}

func ClaimRun(options ClaimRunOptions) (RunClaim, error) {
	root, run, now, ttl, err := validateClaimOptions(options)
	if err != nil {
		return RunClaim{}, err
	}
	releaseCoordination, err := acquireEnvironmentCoordinationLock(root, now)
	if err != nil {
		return RunClaim{}, err
	}
	defer releaseCoordination()
	if err := ensureEnvironmentPreparationIdle(root, now); err != nil {
		return RunClaim{}, err
	}
	if err := ensureRunClaimOwnerBinding(root, options.OwnerKind, options.OwnerID, now); err != nil {
		return RunClaim{}, err
	}
	path := runClaimPath(root, run.RunID)
	if existing, readErr := loadRunClaimPath(path); readErr == nil {
		if existing.ExpiresAt.After(now) {
			conflict := domain.Conflict("RUN_ALREADY_CLAIMED", "本地运行已被其他对话锁定")
			conflict.Details = map[string]any{"run_id": run.RunID, "owner_kind": existing.OwnerKind, "owner_id": existing.OwnerID, "epoch": existing.Epoch, "expires_at": existing.ExpiresAt}
			return RunClaim{}, conflict
		}
		if !options.TakeoverExpired {
			policy := domain.Policy("RUN_CLAIM_TAKEOVER_CONFIRMATION_REQUIRED", "已有运行锁已过期", "确认前一个对话不再写入后，以 takeover_expired=true 接管")
			policy.Details = map[string]any{"run_id": run.RunID, "previous_owner_kind": existing.OwnerKind, "previous_owner_id": existing.OwnerID, "previous_epoch": existing.Epoch, "expired_at": existing.ExpiresAt}
			return RunClaim{}, policy
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return RunClaim{}, err
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return RunClaim{}, readErr
	}
	token, tokenHash, err := domain.NewOpaqueToken("rcl_", 32)
	if err != nil {
		return RunClaim{}, err
	}
	epoch, err := nextRunClaimEpoch(root, run.RunID)
	if err != nil {
		return RunClaim{}, err
	}
	claim := RunClaim{
		SchemaVersion:   RunClaimSchemaVersion,
		RunID:           run.RunID,
		OwnerKind:       strings.TrimSpace(options.OwnerKind),
		OwnerID:         strings.TrimSpace(options.OwnerID),
		Epoch:           epoch,
		Token:           token,
		TokenHash:       tokenHash,
		ContextRevision: run.ContextRevision,
		ClaimedAt:       now,
		ExpiresAt:       now.Add(ttl),
	}
	if err := writeExclusiveJSON(path, persistedRunClaim(claim)); err != nil {
		if errors.Is(err, os.ErrExist) {
			return RunClaim{}, domain.Conflict("RUN_ALREADY_CLAIMED", "本地运行已被其他对话锁定")
		}
		return RunClaim{}, err
	}
	return claim, nil
}

func TakeoverRunClaim(options TakeoverRunClaimOptions) (RunClaim, error) {
	if !validRunClaimOwnerKind(strings.TrimSpace(options.ExpectedOwnerKind)) || strings.TrimSpace(options.ExpectedOwnerID) == "" || options.ExpectedEpoch == 0 {
		return RunClaim{}, domain.Invalid("RUN_CLAIM_TAKEOVER_EXPECTATION_INVALID", "主动接管需要完整的 expected_owner_kind、expected_owner_id 和 expected_epoch")
	}
	if strings.TrimSpace(options.OwnerKind) == strings.TrimSpace(options.ExpectedOwnerKind) && strings.TrimSpace(options.OwnerID) == strings.TrimSpace(options.ExpectedOwnerID) {
		return RunClaim{}, domain.Invalid("RUN_CLAIM_TAKEOVER_OWNER_UNCHANGED", "同一 owner 应续期而不是主动接管")
	}
	root, run, now, ttl, err := validateClaimOptions(ClaimRunOptions{
		Root: options.Root, RunID: options.RunID, OwnerKind: options.OwnerKind, OwnerID: options.OwnerID,
		ExpectedRevision: options.ExpectedRevision, TTL: options.TTL, Now: options.Now,
	})
	if err != nil {
		return RunClaim{}, err
	}
	releaseCoordination, err := acquireEnvironmentCoordinationLock(root, now)
	if err != nil {
		return RunClaim{}, err
	}
	defer releaseCoordination()
	if err := ensureEnvironmentPreparationIdle(root, now); err != nil {
		return RunClaim{}, err
	}
	if err := ensureRunClaimOwnerBinding(root, options.OwnerKind, options.OwnerID, now); err != nil {
		return RunClaim{}, err
	}
	existing, err := loadRunClaimPath(runClaimPath(root, run.RunID))
	if errors.Is(err, os.ErrNotExist) {
		return RunClaim{}, domain.Conflict("RUN_CLAIM_REQUIRED", "主动接管需要一个仍然存在的运行锁")
	}
	if err != nil {
		return RunClaim{}, err
	}
	if !existing.ExpiresAt.After(now) {
		return RunClaim{}, domain.Conflict("RUN_CLAIM_EXPIRED", "主动接管的运行锁已经过期，请使用过期锁接管流程")
	}
	if existing.OwnerKind != strings.TrimSpace(options.ExpectedOwnerKind) || existing.OwnerID != strings.TrimSpace(options.ExpectedOwnerID) || existing.Epoch != options.ExpectedEpoch {
		conflict := domain.Conflict("RUN_CLAIM_FENCE_CONFLICT", "接管时观察到的运行锁所有者或 epoch 已经变化")
		conflict.Details = map[string]any{"owner_kind": existing.OwnerKind, "owner_id": existing.OwnerID, "epoch": existing.Epoch, "context_revision": existing.ContextRevision}
		return RunClaim{}, conflict
	}
	if existing.ContextRevision != run.ContextRevision {
		return RunClaim{}, domain.Conflict("LOCAL_RUN_REVISION_CONFLICT", "接管时运行锁绑定的上下文版本已经变化")
	}
	token, tokenHash, err := domain.NewOpaqueToken("rcl_", 32)
	if err != nil {
		return RunClaim{}, err
	}
	epoch, err := nextRunClaimEpoch(root, run.RunID)
	if err != nil {
		return RunClaim{}, err
	}
	claim := RunClaim{
		SchemaVersion: RunClaimSchemaVersion, RunID: run.RunID,
		OwnerKind: strings.TrimSpace(options.OwnerKind), OwnerID: strings.TrimSpace(options.OwnerID), Epoch: epoch,
		Token: token, TokenHash: tokenHash, ContextRevision: run.ContextRevision, ClaimedAt: now, ExpiresAt: now.Add(ttl),
	}
	if err := replaceJSON(runClaimPath(root, run.RunID), persistedRunClaim(claim), 0o600); err != nil {
		return RunClaim{}, err
	}
	return claim, nil
}

func RenewRunClaim(root, runID, token string, ttl time.Duration, now time.Time) (RunClaim, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return RunClaim{}, err
	}
	at := localNow(now)
	claim, err := validateRunClaim(resolved, runID, token, at)
	if err != nil {
		return RunClaim{}, err
	}
	if ttl == 0 {
		ttl = defaultClaimTTL
	}
	if ttl <= 0 || ttl > maximumClaimTTL {
		return RunClaim{}, domain.Invalid("RUN_CLAIM_TTL_INVALID", "运行锁有效期必须大于 0 且不超过 4 小时")
	}
	claim.ExpiresAt = at.Add(ttl)
	claim.Token = strings.TrimSpace(token)
	if err := replaceJSON(runClaimPath(resolved, runID), persistedRunClaim(claim), 0o600); err != nil {
		return RunClaim{}, err
	}
	return claim, nil
}

func ReleaseRunClaim(root, runID, token string, now time.Time) error {
	resolved, err := FindRoot(root)
	if err != nil {
		return err
	}
	at := localNow(now)
	releaseCoordination, err := acquireEnvironmentCoordinationLock(resolved, at)
	if err != nil {
		return err
	}
	defer releaseCoordination()
	if _, err := validateRunClaim(resolved, runID, token, at); err != nil {
		return err
	}
	return os.Remove(runClaimPath(resolved, runID))
}

func RunClaimStatus(root, runID string, now time.Time) (RunClaimSummary, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return RunClaimSummary{}, err
	}
	claim, err := loadRunClaimPath(runClaimPath(resolved, runID))
	if errors.Is(err, os.ErrNotExist) {
		return RunClaimSummary{Claimed: false}, nil
	}
	if err != nil {
		return RunClaimSummary{}, err
	}
	expiresAt := claim.ExpiresAt
	expired := !expiresAt.After(localNow(now))
	return RunClaimSummary{Claimed: !expired, OwnerKind: claim.OwnerKind, OwnerID: claim.OwnerID, Epoch: claim.Epoch, ExpiresAt: &expiresAt, Expired: expired}, nil
}

func CreateReadyHandoff(options CreateReadyHandoffOptions) (HandoffRecord, error) {
	root, err := FindRoot(options.Root)
	if err != nil {
		return HandoffRecord{}, err
	}
	now := localNow(options.Now)
	claim, err := validateRunClaim(root, options.RunID, options.ClaimToken, now)
	if err != nil {
		return HandoffRecord{}, err
	}
	if err := ensureRunClaimOwnerBinding(root, claim.OwnerKind, claim.OwnerID, now); err != nil {
		return HandoffRecord{}, err
	}
	run, err := loadLocalRun(root, options.RunID)
	if err != nil {
		return HandoffRecord{}, err
	}
	if options.ExpectedRevision == 0 || options.ExpectedRevision != run.ContextRevision {
		conflict := domain.Conflict("LOCAL_RUN_REVISION_CONFLICT", "创建交接时，本地运行上下文的版本已经变化")
		conflict.Details = map[string]any{"expected_revision": options.ExpectedRevision, "current_revision": run.ContextRevision, "claim_revision": claim.ContextRevision}
		return HandoffRecord{}, conflict
	}
	if strings.TrimSpace(options.NextCapabilityID) == "" || strings.TrimSpace(options.NextAction) == "" {
		return HandoffRecord{}, domain.Invalid("HANDOFF_NEXT_ACTION_REQUIRED", "交接记录需要 next_capability_id 和 next_action")
	}
	inputs, err := handoffInputDigests(root, options.InputPaths)
	if err != nil {
		return HandoffRecord{}, err
	}
	if len(inputs) == 0 {
		return HandoffRecord{}, domain.Invalid("HANDOFF_INPUT_REQUIRED", "交接记录至少需要一个带摘要的输入文件")
	}
	handoffID := strings.TrimSpace(options.HandoffID)
	if handoffID == "" {
		handoffID = "hnd_" + strings.ReplaceAll(domain.NewID(), "-", "")
	}
	if !localSourceIDPattern.MatchString(handoffID) {
		return HandoffRecord{}, domain.Invalid("HANDOFF_ID_INVALID", "handoff ID 无效")
	}
	if ready, listErr := ListReadyHandoffs(root); listErr != nil {
		return HandoffRecord{}, listErr
	} else {
		for _, existing := range ready {
			if existing.RunID == run.RunID {
				return HandoffRecord{}, domain.Conflict("HANDOFF_READY_EXISTS", "该本地运行已经有一条待接手的交接记录")
			}
		}
	}
	record := HandoffRecord{
		SchemaVersion:    HandoffSchemaVersion,
		HandoffID:        handoffID,
		RunID:            run.RunID,
		Status:           "ready",
		ContextRevision:  run.ContextRevision,
		FromOwner:        claim.OwnerID,
		NextCapabilityID: strings.TrimSpace(options.NextCapabilityID),
		NextAction:       strings.TrimSpace(options.NextAction),
		InputDigests:     inputs,
		OutputPaths:      append([]string(nil), run.OutputPaths...),
		CompletedChecks:  passedRunChecks(run),
		Blockers:         uniqueStrings(options.Blockers),
		PendingDecisions: uniqueStrings(options.PendingDecisions),
		History:          []HandoffHistory{{To: "draft", At: now}, {From: "draft", To: "ready", At: now}},
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := writeExclusiveJSON(handoffPath(root, handoffID), record); err != nil {
		if errors.Is(err, os.ErrExist) {
			return HandoffRecord{}, domain.Conflict("HANDOFF_EXISTS", "相同 handoff ID 已存在")
		}
		return HandoffRecord{}, err
	}
	if err := ReleaseRunClaim(root, run.RunID, options.ClaimToken, now); err != nil {
		return HandoffRecord{}, err
	}
	return record, nil
}

func AcceptHandoff(options AcceptHandoffOptions) (HandoffRecord, RunClaim, error) {
	root, err := FindRoot(options.Root)
	if err != nil {
		return HandoffRecord{}, RunClaim{}, err
	}
	record, err := loadHandoff(root, options.HandoffID)
	if err != nil {
		return HandoffRecord{}, RunClaim{}, err
	}
	if record.Status != "ready" {
		return HandoffRecord{}, RunClaim{}, domain.Conflict("HANDOFF_NOT_READY", "当前交接记录不可接管")
	}
	run, err := loadLocalRun(root, record.RunID)
	if err != nil {
		return HandoffRecord{}, RunClaim{}, err
	}
	if run.ContextRevision != record.ContextRevision {
		return HandoffRecord{}, RunClaim{}, domain.Conflict("HANDOFF_REVISION_CONFLICT", "交接记录引用的本地运行版本已经变化")
	}
	if err := verifyHandoffDigests(root, record.InputDigests); err != nil {
		return HandoffRecord{}, RunClaim{}, err
	}
	claim, err := ClaimRun(ClaimRunOptions{Root: root, RunID: run.RunID, OwnerKind: options.OwnerKind, OwnerID: options.OwnerID, ExpectedRevision: run.ContextRevision, TTL: options.TTL, TakeoverExpired: options.TakeoverExpired, Now: options.Now})
	if err != nil {
		return HandoffRecord{}, RunClaim{}, err
	}
	if err := verifyHandoffDigests(root, record.InputDigests); err != nil {
		_ = ReleaseRunClaim(root, run.RunID, claim.Token, options.Now)
		return HandoffRecord{}, RunClaim{}, err
	}
	now := localNow(options.Now)
	record.Status = "claimed"
	record.ClaimedBy = claim.OwnerID
	record.ClaimedAt = &now
	record.UpdatedAt = now
	record.History = append(record.History, HandoffHistory{From: "ready", To: "claimed", At: now})
	if err := replaceJSON(handoffPath(root, record.HandoffID), record, 0o600); err != nil {
		_ = ReleaseRunClaim(root, run.RunID, claim.Token, options.Now)
		return HandoffRecord{}, RunClaim{}, err
	}
	return record, claim, nil
}

func CompleteHandoff(root, handoffID, claimToken string, now time.Time) (HandoffRecord, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return HandoffRecord{}, err
	}
	record, err := loadHandoff(resolved, handoffID)
	if err != nil {
		return HandoffRecord{}, err
	}
	if record.Status != "claimed" {
		return HandoffRecord{}, domain.Conflict("HANDOFF_NOT_CLAIMED", "只有已接手的交接记录可以完成")
	}
	claim, err := validateRunClaim(resolved, record.RunID, claimToken, localNow(now))
	if err != nil {
		return HandoffRecord{}, err
	}
	if err := ensureRunClaimOwnerBinding(resolved, claim.OwnerKind, claim.OwnerID, now); err != nil {
		return HandoffRecord{}, err
	}
	at := localNow(now)
	record.Status = "completed"
	record.CompletedAt = &at
	record.UpdatedAt = at
	record.History = append(record.History, HandoffHistory{From: "claimed", To: "completed", At: at})
	if err := replaceJSON(handoffPath(resolved, record.HandoffID), record, 0o600); err != nil {
		return HandoffRecord{}, err
	}
	if err := os.Remove(runClaimPath(resolved, record.RunID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return HandoffRecord{}, err
	}
	return record, nil
}

func SupersedeReadyHandoff(root, handoffID string, now time.Time) (HandoffRecord, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return HandoffRecord{}, err
	}
	record, err := loadHandoff(resolved, handoffID)
	if err != nil {
		return HandoffRecord{}, err
	}
	if record.Status != "ready" {
		return HandoffRecord{}, domain.Conflict("HANDOFF_NOT_READY", "只有待接手的交接记录可以被取代")
	}
	at := localNow(now)
	record.Status = "superseded"
	record.SupersededAt = &at
	record.UpdatedAt = at
	record.History = append(record.History, HandoffHistory{From: "ready", To: "superseded", At: at})
	if err := replaceJSON(handoffPath(resolved, record.HandoffID), record, 0o600); err != nil {
		return HandoffRecord{}, err
	}
	return record, nil
}

func ListReadyHandoffs(root string) ([]HandoffRecord, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(resolved, "40-work", "handoffs", "*.json"))
	if err != nil {
		return nil, err
	}
	records := []HandoffRecord{}
	for _, path := range paths {
		record, err := loadHandoffPath(path)
		if err != nil {
			return nil, err
		}
		if record.Status == "ready" {
			records = append(records, record)
		}
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].HandoffID < records[j].HandoffID
		}
		return records[i].CreatedAt.Before(records[j].CreatedAt)
	})
	return records, nil
}

func validateClaimOptions(options ClaimRunOptions) (string, LocalRunContext, time.Time, time.Duration, error) {
	root, err := FindRoot(options.Root)
	if err != nil {
		return "", LocalRunContext{}, time.Time{}, 0, err
	}
	ownerKind := strings.TrimSpace(options.OwnerKind)
	ownerID := strings.TrimSpace(options.OwnerID)
	if !validRunClaimOwnerKind(ownerKind) {
		return "", LocalRunContext{}, time.Time{}, 0, domain.Invalid("RUN_CLAIM_OWNER_KIND_INVALID", "运行锁 owner_kind 必须是 agent 或 browser")
	}
	if ownerID == "" || len(ownerID) > 128 {
		return "", LocalRunContext{}, time.Time{}, 0, domain.Invalid("RUN_CLAIM_OWNER_ID_INVALID", "运行锁 owner_id 必填，且不能超过 128 个字符")
	}
	run, err := loadLocalRun(root, options.RunID)
	if err != nil {
		return "", LocalRunContext{}, time.Time{}, 0, err
	}
	if run.Status == "completed" {
		return "", LocalRunContext{}, time.Time{}, 0, domain.Conflict("LOCAL_RUN_COMPLETED", "已完成的本地运行不能再加锁")
	}
	if options.ExpectedRevision == 0 || options.ExpectedRevision != run.ContextRevision {
		conflict := domain.Conflict("LOCAL_RUN_REVISION_CONFLICT", "加锁时使用的本地运行上下文版本已经过期")
		conflict.Details = map[string]any{"expected_revision": options.ExpectedRevision, "current_revision": run.ContextRevision}
		return "", LocalRunContext{}, time.Time{}, 0, conflict
	}
	ttl := options.TTL
	if ttl == 0 {
		ttl = defaultClaimTTL
	}
	if ttl <= 0 || ttl > maximumClaimTTL {
		return "", LocalRunContext{}, time.Time{}, 0, domain.Invalid("RUN_CLAIM_TTL_INVALID", "运行锁有效期必须大于 0 且不超过 4 小时")
	}
	return root, run, localNow(options.Now), ttl, nil
}

func validateRunClaim(root, runID, token string, now time.Time) (RunClaim, error) {
	claim, err := loadRunClaimPath(runClaimPath(root, runID))
	if errors.Is(err, os.ErrNotExist) {
		return RunClaim{}, domain.Conflict("RUN_CLAIM_REQUIRED", "写入前必须取得有效的运行锁")
	}
	if err != nil {
		return RunClaim{}, err
	}
	if claim.RunID != runID {
		return RunClaim{}, domain.Invalid("RUN_CLAIM_INVALID", "运行锁的 run_id 与文件路径不一致")
	}
	token = strings.TrimSpace(token)
	actualHash := domain.TokenHash(token)
	if token == "" || subtle.ConstantTimeCompare([]byte(claim.TokenHash), []byte(actualHash)) != 1 {
		return RunClaim{}, domain.Conflict("RUN_CLAIM_TOKEN_INVALID", "运行锁凭据不匹配")
	}
	if !claim.ExpiresAt.After(now) {
		return RunClaim{}, domain.Conflict("RUN_CLAIM_EXPIRED", "运行锁已过期")
	}
	return claim, nil
}

func ValidateRunOwnership(root, runID, token, ownerKind, ownerID string, epoch, expectedRevision uint64, now time.Time) (RunClaim, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return RunClaim{}, err
	}
	run, err := loadLocalRun(resolved, runID)
	if err != nil {
		return RunClaim{}, err
	}
	if expectedRevision == 0 || run.ContextRevision != expectedRevision {
		conflict := domain.Conflict("LOCAL_RUN_REVISION_CONFLICT", "运行所有权校验使用的上下文版本已经变化")
		conflict.Details = map[string]any{"expected_revision": expectedRevision, "current_revision": run.ContextRevision}
		return RunClaim{}, conflict
	}
	claim, err := validateRunClaim(resolved, run.RunID, token, localNow(now))
	if err != nil {
		return RunClaim{}, err
	}
	if claim.OwnerKind != strings.TrimSpace(ownerKind) || claim.OwnerID != strings.TrimSpace(ownerID) || claim.Epoch != epoch {
		conflict := domain.Conflict("RUN_CLAIM_FENCE_CONFLICT", "运行所有权的 owner 或 epoch 已经变化")
		conflict.Details = map[string]any{"owner_kind": claim.OwnerKind, "owner_id": claim.OwnerID, "epoch": claim.Epoch, "context_revision": claim.ContextRevision}
		return RunClaim{}, conflict
	}
	if claim.ContextRevision != run.ContextRevision {
		return RunClaim{}, domain.Conflict("LOCAL_RUN_REVISION_CONFLICT", "运行锁绑定的上下文版本已经变化")
	}
	if err := ensureRunClaimOwnerBinding(resolved, claim.OwnerKind, claim.OwnerID, now); err != nil {
		return RunClaim{}, err
	}
	return claim, nil
}

func validateClaimedRunWrite(root string, run LocalRunContext, token string, expectedRevision uint64, now time.Time) error {
	if expectedRevision == 0 || expectedRevision != run.ContextRevision {
		conflict := domain.Conflict("LOCAL_RUN_REVISION_CONFLICT", "写入时使用的本地运行上下文版本已经过期")
		conflict.Details = map[string]any{"expected_revision": expectedRevision, "current_revision": run.ContextRevision}
		return conflict
	}
	claim, err := validateRunClaim(root, run.RunID, token, localNow(now))
	if err != nil {
		return err
	}
	if err := ensureRunClaimOwnerBinding(root, claim.OwnerKind, claim.OwnerID, now); err != nil {
		return err
	}
	return nil
}

func updateRunClaimRevision(root, runID, token string, revision uint64, now time.Time) error {
	claim, err := validateRunClaim(root, runID, token, localNow(now))
	if err != nil {
		return err
	}
	claim.ContextRevision = revision
	return replaceJSON(runClaimPath(root, runID), persistedRunClaim(claim), 0o600)
}

func loadRunClaimPath(path string) (RunClaim, error) {
	var claim RunClaim
	if err := readJSON(path, &claim); err != nil {
		return RunClaim{}, err
	}
	if claim.SchemaVersion != RunClaimSchemaVersion || claim.RunID == "" || !validRunClaimOwnerKind(claim.OwnerKind) || claim.OwnerID == "" || claim.Epoch == 0 || len(claim.TokenHash) != 64 || claim.Token != "" || claim.ContextRevision == 0 || claim.ClaimedAt.IsZero() || claim.ExpiresAt.IsZero() {
		return RunClaim{}, domain.Invalid("RUN_CLAIM_INVALID", "运行锁文件无效")
	}
	return claim, nil
}

type runClaimEpoch struct {
	SchemaVersion string `json:"schema_version"`
	RunID         string `json:"run_id"`
	Epoch         uint64 `json:"epoch"`
}

func nextRunClaimEpoch(root, runID string) (uint64, error) {
	path := runClaimEpochPath(root, runID)
	value := runClaimEpoch{SchemaVersion: "contentcloud.run-claim-epoch/1.0", RunID: runID}
	if err := readJSON(path, &value); err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	if value.SchemaVersion != "contentcloud.run-claim-epoch/1.0" || value.RunID != runID {
		return 0, domain.Invalid("RUN_CLAIM_EPOCH_INVALID", "运行锁 epoch 文件无效")
	}
	value.Epoch++
	if value.Epoch == 0 {
		return 0, domain.Conflict("RUN_CLAIM_EPOCH_EXHAUSTED", "运行锁 epoch 已耗尽")
	}
	if err := replaceJSON(path, value, 0o600); err != nil {
		return 0, err
	}
	return value.Epoch, nil
}

func persistedRunClaim(claim RunClaim) RunClaim {
	claim.Token = ""
	return claim
}

func validRunClaimOwnerKind(value string) bool {
	return value == "agent" || value == "browser"
}

func ensureRunClaimOwnerBinding(root, ownerKind, ownerID string, now time.Time) error {
	if ownerKind == "browser" {
		return nil
	}
	_, err := EnsureSessionBinding(root, ownerID, now)
	return err
}

func loadHandoff(root, handoffID string) (HandoffRecord, error) {
	if !localSourceIDPattern.MatchString(strings.TrimSpace(handoffID)) {
		return HandoffRecord{}, domain.Invalid("HANDOFF_ID_INVALID", "handoff ID 无效")
	}
	record, err := loadHandoffPath(handoffPath(root, handoffID))
	if errors.Is(err, os.ErrNotExist) {
		return HandoffRecord{}, domain.NotFound("交接记录")
	}
	return record, err
}

func loadHandoffPath(path string) (HandoffRecord, error) {
	var record HandoffRecord
	if err := readJSON(path, &record); err != nil {
		return HandoffRecord{}, err
	}
	if record.SchemaVersion != HandoffSchemaVersion || record.HandoffID == "" || record.RunID == "" || record.ContextRevision == 0 || record.InputDigests == nil || record.CompletedChecks == nil || record.History == nil || !validHandoffStatus(record.Status) {
		return HandoffRecord{}, domain.Invalid("HANDOFF_INVALID", "交接记录文件无效")
	}
	return record, nil
}

func validHandoffStatus(status string) bool {
	switch status {
	case "draft", "ready", "claimed", "completed", "superseded":
		return true
	default:
		return false
	}
}

func handoffInputDigests(root string, paths []string) ([]HandoffInputDigest, error) {
	inputs := make([]HandoffInputDigest, 0, len(paths))
	seen := map[string]bool{}
	for _, raw := range paths {
		relative, body, err := readWorkspaceHandoffInput(root, raw)
		if err != nil {
			return nil, err
		}
		if seen[relative] {
			continue
		}
		seen[relative] = true
		inputs = append(inputs, HandoffInputDigest{ID: "file:" + relative, Path: relative, Digest: "sha256:" + digest(body)})
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].Path < inputs[j].Path })
	return inputs, nil
}

func verifyHandoffDigests(root string, inputs []HandoffInputDigest) error {
	for _, input := range inputs {
		relative, body, err := readWorkspaceHandoffInput(root, input.Path)
		if err != nil {
			return err
		}
		actual := "sha256:" + digest(body)
		if input.ID == "" || relative != input.Path || actual != input.Digest {
			conflict := domain.Conflict("HANDOFF_INPUT_DIGEST_MISMATCH", "交接输入在交接后已经变化")
			conflict.Details = map[string]any{"path": input.Path, "expected_digest": input.Digest, "actual_digest": actual}
			return conflict
		}
	}
	return nil
}

func readWorkspaceHandoffInput(root, raw string) (string, []byte, error) {
	relative := filepath.Clean(filepath.FromSlash(strings.TrimSpace(raw)))
	if relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", nil, domain.Invalid("HANDOFF_INPUT_PATH_INVALID", "交接输入必须是工作区内的相对路径")
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", nil, err
	}
	candidate, err := filepath.EvalSymlinks(filepath.Join(root, relative))
	if err != nil {
		return "", nil, err
	}
	contained, err := filepath.Rel(canonicalRoot, candidate)
	if err != nil || contained == ".." || strings.HasPrefix(contained, ".."+string(filepath.Separator)) {
		return "", nil, domain.Invalid("HANDOFF_INPUT_PATH_INVALID", "交接输入超出工作区边界")
	}
	body, err := os.ReadFile(candidate)
	if err != nil {
		return "", nil, err
	}
	return filepath.ToSlash(contained), body, nil
}

func writeExclusiveJSON(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

func runClaimPath(root, runID string) string {
	return filepath.Join(root, ".contentcloud", "locks", "runs", runID+".claim.json")
}

func runClaimEpochPath(root, runID string) string {
	return filepath.Join(root, ".contentcloud", "locks", "runs", runID+".epoch.json")
}

func handoffPath(root, handoffID string) string {
	return filepath.Join(root, "40-work", "handoffs", handoffID+".json")
}

func passedRunChecks(run LocalRunContext) []string {
	checks := []string{}
	for _, check := range run.Checks {
		if check.Status == "passed" {
			checks = append(checks, check.Name)
		}
	}
	return uniqueStrings(checks)
}
