# V2 单轨审批收敛实施计划

对应 `docs/roadmap/v2/03-domain-and-data-model.md` §2.1/§2.2 与 `14-implementation-status.md` 的 P0 缺口。

目标：把客户审批、导出、交付和结果绑定从 V1 `script_version` 收敛到 `SubmissionRevision` / `ApprovedSnapshot`，并补上 Brief 的策略血缘。

## Stage 1: Brief 策略血缘
**Goal**: `contracts/brief-2.0.schema.json` 已要求 `strategy_version_id`，让本地 Brief lint 与之一致。
**Success Criteria**: 缺少 `strategy_version_id` 的 Brief 被 lint 拒绝并给出稳定错误码。
**Tests**: `internal/localworkspace/script_test.go` 增加缺字段用例；既有用例补字段后仍通过。
**Status**: Complete
**Notes**: `LocalBrief.StrategyVersionID` 加入必填校验；新增 `BRIEF_STRATEGY_NOT_APPROVED` 校验其落在已 pull 的 strategy ApprovedSnapshot；新增 `domain.IsNotFound` helper 用于区分"缺对象"与真实故障。

## Stage 2: 客户审批改挂 SubmissionRevision
**Goal**: ReviewGrant 绑定具体 SubmissionRevision，内部/客户两阶段决定写入同一 revision，客户批准后生成 ApprovedSnapshot。
**Success Criteria**:
- `ApprovalDecision.DecisionStage` 区分 `internal` / `client`
- 客户批准前必须已有同一 revision 的 internal 批准
- 新 revision 出现后旧 grant 自动失效
- 客户批准生成 ApprovedSnapshot，hash 等于 revision content_hash
**Tests**: internal→client 正常路径、跳过 internal 被拒、grant 失效、OTP 错误、重复决定。
**Status**: Complete
**Notes**: `ApprovalDecision.decision_stage` 区分 internal/client/legacy；script 内审仅进入 `internally_approved`，客户 OTP 批准后原子创建当前 ApprovedSnapshot；新 revision 自动撤销旧未决 grant；公开审批同时兼容 V1 历史链接和 V2 revision 链接。Web 与 CLI 的新授权入口只接受 SubmissionRevision。

## Stage 3: 导出改由 ApprovedSnapshot 驱动
**Goal**: JSON/Markdown/XLSX 从批准快照的 canonical 内容生成，不再要求 V1 ScriptVersion。
**Success Criteria**: 三种格式由同一快照生成，manifest 记录 snapshot ID 与 revision hash。
**Tests**: 三格式导出内容一致性、未批准快照拒绝导出。
**Status**: Complete
**Notes**: 服务端和本地 CLI 共用 `localworkspace.RenderScriptPackageV2`；`artifact export/package` 与 Web 均从 `origin=current` 的 script ApprovedSnapshot 生成 JSON/Markdown/XLSX，并在 Artifact metadata 中保存 snapshot ID、revision hash 和 script hash。

## Stage 4: 交付、结果与影子快照回填
**Goal**: DeliveryPackage 引用 ApprovedSnapshot；PerformanceObservation 绑定快照；V1 已批准 ScriptVersion 回填 `origin=v1_import` 只读影子快照。
**Success Criteria**: 回填后历史导出内容与 hash 不变；dry-run 报告数量与不可映射项。
**Tests**: 回填幂等、hash 不变、跨租户负测。
**Status**: Complete
**Notes**: DeliveryPackage/Artifact/PerformanceObservation/Rating/Lineage 已接到 ApprovedSnapshot。`00014_v2_single_track_delivery.sql` 完成结构迁移，`00015_v1_shadow_backfill_report.sql` 提供幂等回填与 inserted/skipped_invalid_hash 报告；真实 PostgreSQL 测试覆盖 hash 保持、重复回填为 0、RLS 和不可变约束。
