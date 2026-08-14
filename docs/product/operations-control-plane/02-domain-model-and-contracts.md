# 运营后台的业务对象与接口约定

状态：`目标契约；先冻结名称和边界，再按阶段实现`。

更新时间：2026-08-08。

## 1. 先用人话说清楚

运营后台里有几类容易混淆的东西。它们不是一回事：

| 后台名称 | 运营人员可以理解为 | 它负责什么 | 它不负责什么 |
| --- | --- | --- | --- |
| 创作产品（Experience） | 客户看到的一套产品 | 客户要做什么、按几步完成、最后得到什么 | 不指定由哪个客户端或服务商执行 |
| 能力（Capability） | 完成一类工作的标准 | 例如“收集灵感”“生成分镜”“检查版权” | 不保存某个客户的结果 |
| 执行方式（Executor） | 谁来做这件事 | 云端程序、本地 Codex、Claude Code、人工等 | 不决定客户流程和审批结果 |
| 技能包（Skill） | 某种能力的具体做法 | 一套经过验证的执行说明、工具和限制 | 不直接成为客户产品 |
| 连接（Connector） | 去哪里找资料 | 搜索、抓取、企业资料库等入口 | 不代表一个完整的创作能力 |
| 服务商（Provider） | 外部服务 | 图片、视频、语音或平台接口 | 不拥有 ContentCloud 的业务状态 |
| 绑定规则（Binding Policy） | 在什么情况下选谁做 | 按租户、资料敏感度、地区、预算选择合适执行方式 | 不修改已经开始的任务 |
| 运行快照（Binding Snapshot） | 这次任务最终选了谁 | 开始时记录确定的版本和执行方式 | 不能被后台悄悄替换 |

最重要的一句话：**客户只选择“我要完成什么”；运营人员决定“平台允许怎么完成”；系统在任务开始时记录“这一次实际由谁完成”。**

## 2. 业务对象之间的关系

```text
创作产品
  └── 需要哪些能力
        └── 哪些技能包可以完成
              └── 哪些执行方式可以运行
                    └── 哪些连接和服务商可以被使用

任务开始
  └── 按绑定规则选出本次组合
        └── 固定为运行快照
              └── 运行记录、结果和费用都引用这份快照
```

运营后台的页面可以查看这条关系，但不能把它们揉成一个“万能配置表”。

## 3. 创作产品版本（ExperienceRelease）

这是客户真正能使用的产品版本，例如“IP 人设视频”“品牌故事短片”。同一个产品可以有多个版本，但一个客户任务只能使用一个已经发布的版本。

```text
ExperienceRelease
├── identity
│   ├── experience_id
│   ├── release_id
│   ├── version
│   └── lifecycle
├── customer_journey
│   ├── title
│   ├── customer_steps[]
│   ├── required_inputs[]
│   └── result_presentations[]
├── workflow
│   ├── sop_revision_ref
│   ├── stages[]
│   └── gates[]
├── required_capabilities[]
├── asset_policy
├── tenant_eligibility
├── release_evidence
└── audit_ref
```

发布前必须完成：步骤检查、输入检查、能力可用性检查、结果呈现检查、权限检查、Canary 试跑和回退说明。发布后不允许直接修改原版本；修改必须生成新版本。

## 4. 能力目录（Capability）

能力是平台对外承诺的标准工作，不等于某个模型或某个 Skill。建议字段：

```text
Capability
├── capability_id / name / description
├── input_schema / output_schema
├── accepted_data_classifications[]
├── side_effect_class
├── timeout / retry / concurrency_limit
├── cost_limit
├── approved_skill_refs[]
├── owner / support_runbook
└── lifecycle
```

能力的发布状态只影响“能不能被新任务选用”，不能改写历史任务已经产生的结果。

## 5. 执行方式（Executor）

执行方式描述信任边界和运行位置，而不是业务能力。类型包括：

| 类型 | 适用场景 | 关键限制 |
| --- | --- | --- |
| `deterministic_worker` | 固定格式转换、校验、投影 | 不可自行扩大工具和数据范围 |
| `codex_client` | 用户授权的本地搜索、资料整理或创作 | 本地资料、MCP 和心跳必须受任务授权限制 |
| `claude_code_client` | 与 Codex 类似的本地执行面 | 不能保存平台权威状态 |
| `provider_adapter` | 调用图片、视频、语音等外部服务 | 必须建立 Effect 台账并处理 unknown |
| `human_runbook` | 需要人工判断或线下处理 | 必须记录操作者、决定和证据 |

Executor 记录健康、区域、心跳、并发、数据访问级别和支持负责人。Executor 下线只会影响新任务的候选资格；已经开始的任务仍按运行快照处理。

## 6. 技能包说明（SkillManifest）

技能包是能力的一种可验证实现。运营人员不需要编辑代码，但需要能看懂它的使用边界：

```text
SkillManifest
├── skill_id / version / digest
├── implementation_kind
├── capability_refs[]
├── compatible_runtime
├── input_schema / output_schema
├── allowed_tools[]
├── data_classification_limit
├── network_policy
├── side_effect_class
├── cost_model / limits
├── fixtures / contract_tests
├── owner / support_runbook
└── lifecycle / canary_evidence
```

技能包生命周期：`草稿 -> 验证中 -> 已批准 -> Canary -> 可用 -> 暂停 -> 退役`。只有“已批准”且满足绑定规则的技能包，才可以进入新任务。

## 7. 绑定规则（BindingPolicy）

绑定规则用来回答：**面对一个具体客户和任务，平台应该选择哪种已经批准的执行方式？**

```text
BindingPolicy
├── policy_id / version / priority
├── scope
│   ├── tenant_ids / project_ids
│   ├── region
│   └── environment
├── conditions
│   ├── data_classification
│   ├── compliance_tags
│   ├── budget_window
│   └── time_window
├── candidates[]
├── limits
│   ├── concurrency / timeout
│   ├── cost_ceiling
│   └── network_and_tool_limits
├── fallback_rules[]
└── approval / audit
```

候选按优先级、健康、兼容性和成本筛选。回退候选不得扩大资料披露、工具权限、副作用或预算。规则改变后只影响新任务，除非通过明确的人工恢复流程创建新的运行分支。

## 8. 运行快照（ExecutionBindingSnapshot）

任务开始时生成不可变快照：

```json
{
  "snapshot_id": "ebs_01J...",
  "experience_release_ref": "ip-video@2.1.0",
  "capability_bindings": [
    {
      "capability_id": "inspiration_collection",
      "skill_ref": "web-research@1.4.2",
      "executor_ref": "codex-client:workspace-12",
      "connector_refs": ["search-provider-a"],
      "provider_refs": [],
      "policy_ref": "tenant-default@3",
      "digest": "sha256:..."
    }
  ],
  "data_classification": "internal",
  "cost_ceiling": {"currency": "CNY", "amount": 20},
  "created_at": "2026-08-08T10:00:00+08:00"
}
```

运行记录、费用、外部请求和结果都必须引用 `snapshot_id`。后台展示“当前健康候选”时，要明确标记它是未来任务候选，不代表已开始任务会被换绑。

## 9. 给工程的接口约定

从这里开始是实现后台时需要的接口约定，运营人员只需要理解前面的中文对象关系。

### 9.1 运营后台查询约定（Operations BFF）

运营前端使用独立 BFF，不复用客户 Studio 的页面 DTO，也不直接读取数据库。

```text
/api/bff/operations/overview
/api/bff/operations/releases
/api/bff/operations/capabilities
/api/bff/operations/executors
/api/bff/operations/executors/:executor_id
/api/bff/operations/skills
/api/bff/operations/skills/:skill_id/versions/:version
/api/bff/operations/connectors
/api/bff/operations/providers
/api/bff/operations/binding-policies
/api/bff/operations/runtime
/api/bff/operations/assets
/api/bff/operations/tenants
/api/bff/operations/audit
/api/bff/operations/costs
```

所有查询返回统一外壳：

```json
{
  "data": {},
  "page": {"next_cursor": null},
  "generated_at": "2026-08-08T10:00:00+08:00",
  "projection_cursor": "ops-000123",
  "staleness_seconds": 4,
  "support_reference": "sup_..."
}
```

分页、筛选、排序和搜索条件必须可复现。敏感字段在 BFF 层就被裁剪，不依赖前端隐藏。

当前执行端首切片已经使用独立 DTO 投影真实设备登记，列表和详情返回：执行端标识、当前租户、设备类型、DaemonInstance 三轴健康与依据、主机/平台/架构、运行版本、Runtime inventory、脱敏 Workspace inventory、能力声明、项目授权、最近状态报告和撤销时间。Workspace inventory 只包含 `workspace_id/project_id`、状态、原因、generation、服务端声明摘要、本地 receipt/观察摘要和观察时间，不返回本地绝对路径。在线状态使用服务端 `last_seen_at` 的 45 秒 freshness；连接代际和报告序列用于同一 DaemonInstance 的乱序/旧报告围栏。地区、资料范围、并发占用和近 24 小时失败率尚无权威字段，BFF 不推算也不从 Environment 补齐。

DaemonInstance 的状态报告是 current-state 快照，不是追加日志：首帧和每次变化都带实例 ID、epoch、序列、进程和能力。重复/倒序报告、同 epoch stopped 复活、旧实例在新实例接管后写入都会被拒绝；断开连接会报告 stopped，Presence 仍由服务端 45 秒 freshness 计算。执行端列表优先选择当前 live 实例，不能用设备 online 推导 Environment ready 或 Runtime 可用。

Workspace 在 Daemon 启动时观察，之后每 30 秒刷新；Runtime inventory 在启动时探测，之后每 5 分钟刷新。两者变化都立即触发完整 current-state，但都只是执行端当前状态。新 Attempt 创建前，Runtime 必须按项目匹配唯一 Workspace，要求状态为 `ready`，并比对 `ExecutionBindingSnapshot` 冻结的 Environment、Plugin、Skill、MCP、Workspace 五类服务端声明摘要。Plugin receipt 和本地 Skill/MCP/Workspace 观察摘要是不同字段，不允许互相替代。漂移阻断新 Attempt，不改写已运行 Attempt 的快照。

当前技能包首切片只投影服务端启动时已经完成签名校验、且 `kind=skill_pack` 的插件 Registry 条目。列表和详情返回：技能包标识、版本、摘要、生命周期、新任务候选资格、代码来源和不可变引用、许可证、签名状态和 key ID、兼容配置、权限、默认数据流、云端动作、费用声明、输出 Schema、评测报告/摘要/证据和撤销状态。BFF 不返回签名正文。Registry 未配置时返回 `configured=false` 和 `skills=[]`，不扫描本地 Skill 文件，也不读取测试夹具补数。

这份 Registry 投影不是完整 `SkillManifest`。当前 Registry 没有能力引用、输入 Schema、负责人、支持手册、工具白名单、资料等级、副作用、Canary 和绑定影响字段，因此页面不得宣称技能包已经通过业务审批或被客户使用；相应审核和影响操作继续等待独立契约。

## 10. 运营命令约定

写操作使用命令，而不是通用 `PATCH /admin/object/:id`：

```text
POST /api/bff/operations/releases/:id/submit-validation
POST /api/bff/operations/releases/:id/start-canary
POST /api/bff/operations/releases/:id/publish
POST /api/bff/operations/releases/:id/rollback
POST /api/bff/operations/skills/:id/submit-approval
POST /api/bff/operations/binding-policies/:id/simulate
POST /api/bff/operations/binding-policies/:id/publish
POST /api/bff/operations/runtime/jobs/:id/pause
POST /api/bff/operations/runtime/jobs/:id/reconcile-effect
POST /api/bff/operations/assets/projections/rebuild
```

命令请求必须带：`idempotency_key`、`expected_version`、`reason` 和必要的审批人。响应返回新的版本、受影响对象、审计 ID、可继续动作和冲突信息。

```json
{
  "command_id": "cmd_...",
  "status": "accepted",
  "new_version": 8,
  "affected": {"releases": 1, "tenants": 3, "active_runs": 0},
  "audit_event_id": "audit_...",
  "allowed_actions": []
}
```

## 11. 运营角色与权限

| 角色 | 可做什么 | 不可做什么 |
| --- | --- | --- |
| 内容运营 | 编辑客户步骤、结果呈现、Experience 草稿、租户启用申请 | 改执行代码、改 Runtime 事实、看凭据 |
| 能力管理员 | 注册 Capability、审核 Skill、维护绑定规则 | 修改客户批准结果、越过数据策略 |
| 运行运营 | 查看队列、处理失败、发起对账、暂停/取消允许的节点 | 强制成功、删除事件、改历史输入 |
| 资产治理 | 处理权利待办、重复候选、投影重建 | 改写 Source、Approval、Artifact、Delivery 权威事实 |
| 支持人员 | 按授权查看租户任务和支持编号 | 查看不必要的受限资料、发布平台版本 |
| 平台管理员 | 管理角色、全局策略和紧急回退 | 绕过审计和双人审批 |

高风险动作（发布、回退、扩大数据范围、改变 Provider 或预算）默认要求双人批准或紧急授权记录。

## 12. 审计事件字段

```text
AuditEvent
├── audit_event_id / occurred_at
├── actor_id / actor_role / tenant_scope
├── action / object_type / object_id
├── expected_version / resulting_version
├── before_digest / after_digest
├── reason / approval_ref
├── request_id / support_reference
├── affected_refs[]
└── outcome / error_code
```

审计记录不能包含密钥、完整提示词、完整资料正文、本地绝对路径或完整外部响应。回放和投影重建只能产生读模型事件，不能重复外部调用。

## 13. 契约与测试要求

每个新增对象至少提供：Schema 示例、版本兼容说明、权限负向测试、幂等测试、陈旧版本冲突测试、跨租户隔离测试和审计断言。

最低验收路径：

1. 发布一个使用两个已批准能力的 Experience，并能回退。
2. 同一任务在开始后修改绑定规则，运行快照不变。
3. Executor 离线时新任务阻断或使用预先批准的安全回退。
4. 外部结果 unknown 时只能对账，不能重复提交。
5. 资产投影重建后目录摘要一致，且没有外部副作用。
6. 执行端详情显示 Workspace 状态、reason、ID、generation 和 freshness，但响应与页面都不包含本地绝对路径。
7. 同项目缺少 Workspace、多个 Workspace、非 ready 或任一声明摘要漂移时，`prepare_next` 在创建 RuntimeAttempt 前 fail-closed；修复后的新 current-state 才允许后续 Attempt。
