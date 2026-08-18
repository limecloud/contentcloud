package localworkspace

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
)

const environmentPreparationTTL = 10 * time.Minute

type EnvironmentPreparationLease struct {
	SchemaVersion string    `json:"schema_version"`
	PreparationID string    `json:"preparation_id"`
	Token         string    `json:"token"`
	StartedAt     time.Time `json:"started_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}

type ActiveRunClaim struct {
	RunID     string    `json:"run_id"`
	OwnerKind string    `json:"owner_kind"`
	OwnerID   string    `json:"owner_id"`
	Epoch     uint64    `json:"epoch"`
	ExpiresAt time.Time `json:"expires_at"`
}

func BeginEnvironmentPreparation(root, preparationID string, now time.Time) (EnvironmentPreparationLease, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return EnvironmentPreparationLease{}, err
	}
	at := localNow(now)
	release, err := acquireEnvironmentCoordinationLock(resolved, at)
	if err != nil {
		return EnvironmentPreparationLease{}, err
	}
	defer release()
	if !strings.HasPrefix(preparationID, "epp_") {
		return EnvironmentPreparationLease{}, fault.Invalid("ENVIRONMENT_PREPARATION_ID_INVALID", "环境准备标识（preparation_id）无效")
	}
	if existing, readErr := loadEnvironmentPreparationLease(resolved); readErr == nil {
		if existing.ExpiresAt.After(at) {
			conflict := fault.Conflict("ENVIRONMENT_PREPARATION_IN_PROGRESS", "另一个环境准备流程正在执行")
			conflict.Details = map[string]any{"preparation_id": existing.PreparationID, "expires_at": existing.ExpiresAt}
			return EnvironmentPreparationLease{}, conflict
		}
		if err := os.Remove(environmentPreparationPath(resolved)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return EnvironmentPreparationLease{}, err
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return EnvironmentPreparationLease{}, readErr
	}
	claims, err := activeRunClaims(resolved, at)
	if err != nil {
		return EnvironmentPreparationLease{}, err
	}
	if len(claims) > 0 {
		blocked := fault.Conflict("ENVIRONMENT_PREPARATION_RUN_ACTIVE", "存在有效的运行锁，禁止修改创作环境")
		blocked.Details = claims
		return EnvironmentPreparationLease{}, blocked
	}
	lease := EnvironmentPreparationLease{SchemaVersion: "1.0", PreparationID: preparationID, Token: idgen.New(), StartedAt: at, ExpiresAt: at.Add(environmentPreparationTTL)}
	if err := writeExclusiveJSON(environmentPreparationPath(resolved), lease); err != nil {
		return EnvironmentPreparationLease{}, err
	}
	return lease, nil
}

func FinishEnvironmentPreparation(root, token string) error {
	resolved, err := FindRoot(root)
	if err != nil {
		return err
	}
	lease, err := loadEnvironmentPreparationLease(resolved)
	if err != nil {
		return err
	}
	if token == "" || token != lease.Token {
		return fault.Policy("ENVIRONMENT_PREPARATION_TOKEN_INVALID", "环境准备凭据无效", "只允许持有当前环境准备租约的进程完成环境变更")
	}
	return os.Remove(environmentPreparationPath(resolved))
}

func ensureEnvironmentPreparationIdle(root string, now time.Time) error {
	lease, err := loadEnvironmentPreparationLease(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	blocked := fault.Conflict("ENVIRONMENT_PREPARATION_IN_PROGRESS", "环境准备期间不能加锁或领取任务运行")
	blocked.Details = map[string]any{
		"preparation_id": lease.PreparationID,
		"expires_at":     lease.ExpiresAt,
		"expired":        !lease.ExpiresAt.After(localNow(now)),
	}
	return blocked
}

func activeRunClaims(root string, now time.Time) ([]ActiveRunClaim, error) {
	directory := filepath.Join(root, ".contentcloud", "locks", "runs")
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return []ActiveRunClaim{}, nil
	}
	if err != nil {
		return nil, err
	}
	claims := []ActiveRunClaim{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".claim.json") {
			continue
		}
		claim, err := loadRunClaimPath(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, err
		}
		if claim.ExpiresAt.After(now) {
			claims = append(claims, ActiveRunClaim{RunID: claim.RunID, OwnerKind: claim.OwnerKind, OwnerID: claim.OwnerID, Epoch: claim.Epoch, ExpiresAt: claim.ExpiresAt})
		}
	}
	sort.Slice(claims, func(i, j int) bool { return claims[i].RunID < claims[j].RunID })
	return claims, nil
}

func acquireEnvironmentCoordinationLock(root string, now time.Time) (func(), error) {
	path := filepath.Join(root, ".contentcloud", "environment-coordination.lock")
	acquire := func() error {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		_, writeErr := file.WriteString(localNow(now).Format(time.RFC3339Nano) + "\n")
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			_ = os.Remove(path)
			if writeErr != nil {
				return writeErr
			}
			return closeErr
		}
		return nil
	}
	if err := acquire(); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		info, statErr := os.Stat(path)
		if statErr != nil || localNow(now).Sub(info.ModTime()) <= time.Minute {
			return nil, fault.Conflict("ENVIRONMENT_COORDINATION_BUSY", "另一个进程正在协调运行锁或环境准备")
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if err := acquire(); err != nil {
			return nil, fault.Conflict("ENVIRONMENT_COORDINATION_BUSY", "另一个进程正在协调运行锁或环境准备")
		}
	}
	return func() { _ = os.Remove(path) }, nil
}

func loadEnvironmentPreparationLease(root string) (EnvironmentPreparationLease, error) {
	var lease EnvironmentPreparationLease
	if err := readStrictJSON(environmentPreparationPath(root), &lease); err != nil {
		return EnvironmentPreparationLease{}, err
	}
	if lease.SchemaVersion != "1.0" || !strings.HasPrefix(lease.PreparationID, "epp_") || lease.Token == "" || lease.StartedAt.IsZero() || !lease.ExpiresAt.After(lease.StartedAt) {
		return EnvironmentPreparationLease{}, fault.Invalid("ENVIRONMENT_PREPARATION_LEASE_INVALID", "环境准备租约无效")
	}
	return lease, nil
}

func environmentPreparationPath(root string) string {
	return filepath.Join(root, ".contentcloud", "environment-preparation.lock")
}
