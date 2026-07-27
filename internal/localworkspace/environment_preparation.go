package localworkspace

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
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
	Owner     string    `json:"owner"`
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
		return EnvironmentPreparationLease{}, domain.Invalid("ENVIRONMENT_PREPARATION_ID_INVALID", "Environment preparation_id 无效")
	}
	if existing, readErr := loadEnvironmentPreparationLease(resolved); readErr == nil {
		if existing.ExpiresAt.After(at) {
			conflict := domain.Conflict("ENVIRONMENT_PREPARATION_IN_PROGRESS", "另一个 Environment Preparation 正在执行")
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
		blocked := domain.Conflict("ENVIRONMENT_PREPARATION_RUN_ACTIVE", "存在活跃 RunClaim，禁止修改创作环境")
		blocked.Details = claims
		return EnvironmentPreparationLease{}, blocked
	}
	lease := EnvironmentPreparationLease{SchemaVersion: "1.0", PreparationID: preparationID, Token: domain.NewID(), StartedAt: at, ExpiresAt: at.Add(environmentPreparationTTL)}
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
		return domain.Policy("ENVIRONMENT_PREPARATION_TOKEN_INVALID", "Environment Preparation token 无效", "只允许持有当前 preparation lease 的进程完成环境变更")
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
	blocked := domain.Conflict("ENVIRONMENT_PREPARATION_IN_PROGRESS", "Environment Preparation 期间不能 claim 或领取 Run")
	blocked.Details = map[string]any{
		"preparation_id": lease.PreparationID,
		"expires_at":     lease.ExpiresAt,
		"expired":        !lease.ExpiresAt.After(localNow(now)),
	}
	return blocked
}

func activeRunClaims(root string, now time.Time) ([]ActiveRunClaim, error) {
	directory := filepath.Join(root, "work", "claims")
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return []ActiveRunClaim{}, nil
	}
	if err != nil {
		return nil, err
	}
	claims := []ActiveRunClaim{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		claim, err := loadRunClaimPath(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, err
		}
		if claim.ExpiresAt.After(now) {
			claims = append(claims, ActiveRunClaim{RunID: claim.RunID, Owner: claim.Owner, ExpiresAt: claim.ExpiresAt})
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
			return nil, domain.Conflict("ENVIRONMENT_COORDINATION_BUSY", "另一个进程正在协调 RunClaim 或 Environment Preparation")
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if err := acquire(); err != nil {
			return nil, domain.Conflict("ENVIRONMENT_COORDINATION_BUSY", "另一个进程正在协调 RunClaim 或 Environment Preparation")
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
		return EnvironmentPreparationLease{}, domain.Invalid("ENVIRONMENT_PREPARATION_LEASE_INVALID", "Environment Preparation lease 无效")
	}
	return lease, nil
}

func environmentPreparationPath(root string) string {
	return filepath.Join(root, ".contentcloud", "environment-preparation.lock")
}
