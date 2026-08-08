package pluginhost

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/integration/plugin"
)

type Adapter struct {
	Host  NativeHost
	Store *Store
	Now   func() time.Time
}

func New(host NativeHost, store *Store) (*Adapter, error) {
	if host == nil || store == nil {
		return nil, domain.Invalid("PLUGIN_HOST_ADAPTER_INVALID", "插件宿主适配器和存储必须存在")
	}
	return &Adapter{Host: host, Store: store, Now: time.Now}, nil
}

func (a *Adapter) Detect(ctx context.Context, pkg plugin.Package) (State, error) {
	if err := validatePackage(pkg); err != nil {
		return State{}, err
	}
	return a.Host.Detect(ctx, TargetFromPackage(pkg))
}

func (a *Adapter) Plan(ctx context.Context, pkg plugin.Package, mode string) (Plan, error) {
	if err := validatePackage(pkg); err != nil {
		return Plan{}, err
	}
	if mode != "install" && mode != "remove" {
		return Plan{}, domain.Invalid("PLUGIN_HOST_PLAN_MODE_INVALID", "插件宿主计划必须是 install 或 remove")
	}
	capabilities, err := a.Host.Capabilities(ctx)
	if err != nil {
		return Plan{}, err
	}
	state, err := a.Host.Detect(ctx, TargetFromPackage(pkg))
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{
		SchemaVersion:        SchemaVersion,
		Mode:                 mode,
		HostID:               a.Host.ID(),
		Release:              TargetFromPackage(pkg).Release,
		ObservedGeneration:   state.Generation,
		State:                state.Status,
		Actions:              []Action{},
		BlockingReasons:      []string{},
		RequiresConfirmation: false,
	}
	if mode == "install" && !capabilities.Skills && len(pkg.Skills) > 0 {
		plan.BlockingReasons = append(plan.BlockingReasons, "宿主不支持 Agent Skills")
	}
	if mode == "install" && !capabilities.MCPStdio && hasSupportedMCP(pkg) {
		plan.BlockingReasons = append(plan.BlockingReasons, "宿主不支持 stdio MCP")
	}
	if state.Status == StatusBlocked {
		plan.BlockingReasons = append(plan.BlockingReasons, state.Reason)
	}
	if mode == "install" {
		receipt, receiptErr := a.Store.LoadReceipt(a.Host.ID(), pkg.Manifest.Name)
		if receiptErr != nil {
			return Plan{}, receiptErr
		}
		if receipt != nil && receipt.Release.Version == pkg.Manifest.Version && receipt.Release.Digest != pkg.Digest {
			plan.BlockingReasons = append(plan.BlockingReasons, "同一插件版本不能对应不同的标准包摘要，请提升插件版本")
		}
	}
	if len(plan.BlockingReasons) > 0 {
		plan.State = StatusBlocked
	} else if mode == "install" && state.Status == StatusReady {
		plan.State = StatusReady
	} else if mode == "remove" && state.Status == StatusAbsent {
		plan.State = StatusRemoved
	} else {
		plan.State = StatusStaged
		plan.RequiresConfirmation = true
		for _, skill := range pkg.Skills {
			reason := "标准包 Skill 需要通过宿主原生插件机制激活"
			kind := "activate"
			if mode == "remove" {
				kind = "deactivate"
				reason = "标准包 Skill 需要通过宿主原生插件机制停用"
			}
			plan.Actions = append(plan.Actions, Action{Kind: kind, Component: "skill", Name: skill.Name, Reason: reason})
		}
		for _, server := range pkg.MCPServers {
			if server.Supported {
				reason := "标准 stdio MCP 需要通过宿主原生插件机制激活"
				kind := "activate"
				if mode == "remove" {
					kind = "deactivate"
					reason = "标准 stdio MCP 需要通过宿主原生插件机制停用"
				}
				plan.Actions = append(plan.Actions, Action{Kind: kind, Component: "mcp", Name: server.Name, Reason: reason})
			}
		}
	}
	plan.PlanDigest, err = digestPlan(plan)
	if err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func (a *Adapter) Apply(ctx context.Context, pkg plugin.Package, approved Plan, confirmed bool) (Receipt, error) {
	if approved.SchemaVersion != SchemaVersion || approved.HostID != a.Host.ID() || approved.Release.Digest != pkg.Digest {
		return Receipt{}, domain.Invalid("PLUGIN_HOST_PLAN_INVALID", "安装计划与标准包或宿主不一致")
	}
	if approved.Mode != "install" {
		return Receipt{}, domain.Invalid("PLUGIN_HOST_PLAN_INVALID", "Apply 只接受 install 计划")
	}
	currentPlan, err := a.Plan(ctx, pkg, "install")
	if err != nil {
		return Receipt{}, err
	}
	if currentPlan.PlanDigest != approved.PlanDigest {
		return Receipt{}, domain.Conflict("PLUGIN_HOST_PLAN_STALE", "插件安装状态在确认后发生变化，请重新生成计划")
	}
	if currentPlan.State == StatusBlocked {
		return Receipt{}, domain.Conflict("PLUGIN_HOST_INSTALL_BLOCKED", strings.Join(currentPlan.BlockingReasons, "; "))
	}
	if currentPlan.State == StatusReady {
		receipt, err := a.Store.LoadReceipt(a.Host.ID(), pkg.Manifest.Name)
		if err != nil {
			return Receipt{}, err
		}
		if receipt == nil {
			return Receipt{}, domain.Conflict("PLUGIN_HOST_RECEIPT_MISSING", "宿主报告已就绪但本地安装回执缺失")
		}
		return *receipt, nil
	}
	if !confirmed {
		return Receipt{}, domain.Policy("PLUGIN_HOST_CONFIRMATION_REQUIRED", "安装计划需要明确确认后才能修改宿主配置", "确认同一 plan_digest 后重试")
	}
	unlock, err := a.Store.Lock(a.Host.ID())
	if err != nil {
		return Receipt{}, err
	}
	defer unlock()
	// Re-check after acquiring the CAS lock. This prevents two callers from
	// applying a plan that was valid before either one started staging.
	latest, err := a.Plan(ctx, pkg, "install")
	if err != nil {
		return Receipt{}, err
	}
	if latest.PlanDigest != approved.PlanDigest {
		return Receipt{}, domain.Conflict("PLUGIN_HOST_PLAN_STALE", "插件安装计划在 CAS 锁后已过期")
	}
	installationID := uuid.NewString()
	stage, err := a.Store.Stage(pkg, installationID)
	if err != nil {
		return Receipt{}, err
	}
	defer func() { _ = removeStage(a.Store, stage) }()
	ref := latest.Release
	packageRoot, err := a.Store.CommitStage(stage, ref)
	if err != nil {
		return Receipt{}, err
	}
	previous, err := a.Store.LoadReceipt(a.Host.ID(), pkg.Manifest.Name)
	if err != nil {
		return Receipt{}, err
	}
	now := a.clock()
	change, installed, err := a.Host.Apply(ctx, NativeApply{Target: TargetFromPackage(pkg), Package: pkg, PackageRoot: packageRoot, PluginDataRoot: a.Store.DataPath(a.Host.ID(), ref), InstallationID: installationID, InstalledAt: now})
	if err != nil {
		rollbackErr := a.rollbackNative(context.WithoutCancel(ctx), change)
		return Receipt{}, mutationError("PLUGIN_HOST_APPLY_ROLLBACK_FAILED", "插件宿主安装失败且无法完整回滚", err, rollbackErr)
	}
	receipt := Receipt{SchemaVersion: SchemaVersion, InstallationID: installationID, HostID: a.Host.ID(), Release: ref, PlanDigest: latest.PlanDigest, Status: StatusReady, Installed: installed, PreviousReceipt: previous, NativeData: change.Data, InstalledAt: now, VerifiedAt: now, RollbackReference: installationID}
	if err := a.Store.SaveReceipt(receipt); err != nil {
		rollbackErr := a.rollbackNative(context.WithoutCancel(ctx), change)
		return Receipt{}, mutationError("PLUGIN_HOST_RECEIPT_ROLLBACK_FAILED", "插件安装回执保存失败且无法完整回滚", err, rollbackErr)
	}
	if err := a.Host.Commit(ctx, change); err != nil {
		rollbackErr := a.rollbackNative(context.WithoutCancel(ctx), change)
		receiptErr := a.restoreReceipt(previous, a.Host.ID(), pkg.Manifest.Name)
		return Receipt{}, mutationError("PLUGIN_HOST_COMMIT_FAILED", "插件宿主提交失败", err, errors.Join(rollbackErr, receiptErr))
	}
	return receipt, nil
}

func (a *Adapter) Inspect(ctx context.Context, pkg plugin.Package) (State, error) {
	return a.Detect(ctx, pkg)
}

func (a *Adapter) Remove(ctx context.Context, pkg plugin.Package, approved Plan, confirmed bool) (Receipt, error) {
	if approved.SchemaVersion != SchemaVersion || approved.Mode != "remove" || approved.HostID != a.Host.ID() || approved.Release.PluginID != pkg.Manifest.Name || approved.Release.Digest != pkg.Digest {
		return Receipt{}, domain.Invalid("PLUGIN_HOST_REMOVE_PLAN_INVALID", "删除计划与宿主或插件不一致")
	}
	if !confirmed {
		return Receipt{}, domain.Policy("PLUGIN_HOST_REMOVE_CONFIRMATION_REQUIRED", "删除会修改宿主 Skill 和 MCP 配置，需要明确确认", "确认同一插件 Release 后重试")
	}
	unlock, err := a.Store.Lock(a.Host.ID())
	if err != nil {
		return Receipt{}, err
	}
	defer unlock()
	latest, err := a.Plan(ctx, pkg, "remove")
	if err != nil {
		return Receipt{}, err
	}
	if latest.PlanDigest != approved.PlanDigest {
		return Receipt{}, domain.Conflict("PLUGIN_HOST_PLAN_STALE", "插件删除计划在 CAS 锁后已过期")
	}
	receipt, err := a.Store.LoadReceipt(a.Host.ID(), pkg.Manifest.Name)
	if err != nil {
		return Receipt{}, err
	}
	if receipt == nil {
		return Receipt{}, domain.NotFound("插件宿主安装回执")
	}
	change, err := a.Host.Remove(ctx, NativeRemove{Target: TargetFromPackage(pkg), Receipt: *receipt, PluginDataRoot: a.Store.DataPath(a.Host.ID(), receipt.Release)})
	if err != nil {
		rollbackErr := a.rollbackNative(context.WithoutCancel(ctx), change)
		return Receipt{}, mutationError("PLUGIN_HOST_REMOVE_ROLLBACK_FAILED", "插件宿主删除失败且无法完整回滚", err, rollbackErr)
	}
	removed := *receipt
	currentReceipt := *receipt
	removed.PreviousReceipt = &currentReceipt
	removed.Status = StatusRemoved
	removed.NativeData = change.Data
	removed.VerifiedAt = a.clock()
	if err := a.Store.DeleteReceipt(a.Host.ID(), pkg.Manifest.Name); err != nil {
		rollbackErr := a.rollbackNative(context.WithoutCancel(ctx), change)
		return Receipt{}, mutationError("PLUGIN_HOST_REMOVE_RECEIPT_ROLLBACK_FAILED", "插件删除回执更新失败且无法完整回滚", err, rollbackErr)
	}
	if err := a.Host.Commit(ctx, change); err != nil {
		rollbackErr := a.rollbackNative(context.WithoutCancel(ctx), change)
		receiptErr := a.Store.SaveReceipt(*receipt)
		return Receipt{}, mutationError("PLUGIN_HOST_REMOVE_COMMIT_FAILED", "插件宿主删除提交失败", err, errors.Join(rollbackErr, receiptErr))
	}
	return removed, nil
}

func (a *Adapter) Rollback(ctx context.Context, receipt Receipt) error {
	if receipt.HostID != a.Host.ID() || receipt.NativeData == nil {
		return domain.Invalid("PLUGIN_HOST_ROLLBACK_INVALID", "回滚回执不属于当前宿主或缺少原生状态")
	}
	var change NativeChange
	change.Data = receipt.NativeData
	if err := a.Host.Rollback(ctx, change); err != nil {
		return err
	}
	if receipt.PreviousReceipt == nil {
		return a.Store.DeleteReceipt(receipt.HostID, receipt.Release.PluginID)
	}
	return a.Store.SaveReceipt(*receipt.PreviousReceipt)
}

func (a *Adapter) clock() time.Time {
	if a.Now == nil {
		return time.Now().UTC()
	}
	return a.Now().UTC()
}

func validatePackage(pkg plugin.Package) error {
	if pkg.Manifest.Name == "" || pkg.Manifest.Version == "" || pkg.Digest == "" {
		return domain.Invalid("PLUGIN_HOST_PACKAGE_INVALID", "标准包缺少可安装的身份和摘要")
	}
	return nil
}

func hasSupportedMCP(pkg plugin.Package) bool {
	for _, server := range pkg.MCPServers {
		if server.Supported {
			return true
		}
	}
	return false
}

func removeStage(store *Store, stage string) error {
	if stage == "" {
		return nil
	}
	if err := os.RemoveAll(stage); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove staging: %w", err)
	}
	return nil
}

func (a *Adapter) restoreReceipt(previous *Receipt, host HostID, pluginID string) error {
	if previous == nil {
		return a.Store.DeleteReceipt(host, pluginID)
	}
	return a.Store.SaveReceipt(*previous)
}

func (a *Adapter) rollbackNative(ctx context.Context, change NativeChange) error {
	if len(change.Data) == 0 {
		return nil
	}
	return a.Host.Rollback(ctx, change)
}

func mutationError(code, message string, cause, rollback error) error {
	if rollback == nil {
		return cause
	}
	err := domain.Invalid(code, message)
	err.Details = map[string]any{"cause": cause.Error(), "rollback": rollback.Error()}
	return err
}
