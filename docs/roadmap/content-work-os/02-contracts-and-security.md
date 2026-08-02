# 契约与安全

## 1. 契约原则

API 不让客户端直接修改投影状态。所有写入都表达业务命令，服务端根据事实和版本重新计算状态。

```text
命令 -> 权限检查 -> 作用域检查 -> 版本检查 -> 写入事实/编排记录 -> 审计 -> 返回投影
```

禁止：

- 前端直接提交 `status=approved`、`status=completed` 或 `status=delivered`；
- 以 Task 状态覆盖 Revision、Gate 或 Delivery 事实；
- 以页面刷新产生 TaskRun、GateEvaluation 或 Decision；
- 用未签名的 SOP JSON 触发本地或自动化执行；
- 把 Prompt、模型参数、本机路径、聊天 transcript 或外部账号写入普通 Task API。

## 2. SOP 契约

### 2.1 SOP Version

```json
{
  "schema_version": "contentcloud.sop/1.0",
  "sop_id": "sop-video-production",
  "version": "1.2.0",
  "digest": "sha256:<64 hex>",
  "scope": {"tenant_id": "tenant-1", "environment_id": "env-1"},
  "status": "published",
  "name": "短视频内容生产",
  "content_types": ["video_script"],
  "stages": [],
  "default_execution_mode": "local",
  "metrics": [],
  "published_at": "2026-08-01T00:00:00Z"
}
```

服务端必须校验：版本号唯一、digest 可重算、Stage ID 唯一、顺序稳定、输入输出 Schema 存在、角色和能力已注册、Gate 模式合法、无未知 action。

### 2.2 Stage Definition

```json
{
  "stage_id": "script",
  "name": "脚本创作",
  "order": 30,
  "owner_roles": ["editor"],
  "input_refs": ["brief_snapshot", "knowledge_snapshot"],
  "output_schema": "contentcloud.content_batch/3.0",
  "required_capabilities": ["content.script.compose"],
  "execution_modes": ["local", "agent"],
  "checks": ["content.schema", "claim.references", "rights.references"],
  "gate_ids": ["optional-internal-review"],
  "retry_policy": {"max_attempts": 3},
  "escalation_policy": {"on": "blocked", "to_roles": ["strategist"]}
}
```

Stage 不能包含任意命令、任意 URL、本机目录或自动写文件指令。执行资源根据 Capability Registry 解析真实能力。

### 2.3 Gate Definition

```json
{
  "gate_id": "optional-internal-review",
  "mode": "internal_review",
  "required": false,
  "blocking": false,
  "assignee_roles": ["reviewer"],
  "input_refs": ["submission_revision"],
  "checks": ["brand.rules", "content.schema"],
  "on_reject": "changes_requested",
  "escalation": {"after_hours": 48, "to_roles": ["project_manager"]}
}
```

`required=false` 不代表可以绕过平台安全硬门禁。`blocking=false` 只适用于业务确认；Evidence、Rights、权限和数据披露的硬失败始终阻断。

## 3. Task 契约

### 3.1 创建 Task

```http
POST /api/bff/tasks
```

```json
{
  "project_id": "project-1",
  "sop_id": "sop-video-production",
  "sop_version": "1.2.0",
  "title": "生成 10 条新品短视频脚本",
  "intent": "product_launch",
  "content_type": "video_script",
  "input_refs": ["brief-snapshot-1", "knowledge-snapshot-3"],
  "requested_output": {"count": 10, "schema": "contentcloud.content_batch/3.0"},
  "assignee_user_id": "user-1",
  "priority": "normal",
  "due_at": "2026-08-08T09:00:00Z",
  "risk_profile": "external_marketing"
}
```

服务端返回 Task、TaskRun、绑定的 Environment digest 和第一条 next action。客户端不能自行拼接 SOP 版本或输入快照。

### 3.2 Task Detail

```http
GET /api/bff/tasks/{taskID}
```

返回内容包括：

- Task 基本字段和状态；
- Project、Environment、SOP Version 和 digest；
- StageRun、执行摘要、Gate 和 Revision 摘要；
- 当前下一动作；
- `allowed_actions`；
- 审计高水位和投影生成时间。

普通用户不能通过该接口读取未 publish 的本地正文、模型 transcript 或本机目录。

### 3.3 ClientAdapter 注册

客户端适配器由本地 Workspace 节点注册，服务端只保存能力和版本，不保存本机会话路径：

```json
{
  "schema_version": "contentcloud.client_adapter/1.0",
  "client_id": "claude-code",
  "client_version": "1.0.82",
  "adapter_version": "0.1.0",
  "node_id": "node-local-1",
  "owned_formats": ["claude-code.transcript-jsonl/v1"],
  "capabilities": {
    "supports_summary": true,
    "supports_selected_turns": true,
    "supports_full_transcript": true,
    "redaction_handled_locally": true
  },
  "redaction_policy_digest": "sha256:<64 hex>"
}
```

`client_id + adapter_version` 决定格式解析实现。Codex CLI 和 Claude Code CLI 可以输出同一 `ConversationBundle`，但不得共享客户端私有输入 Schema，也不得把格式分支转移到 Web。

### 3.4 创建 ConversationImport

```http
POST /api/bff/tasks/{taskID}/conversation-imports
```

```json
{
  "client_id": "claude-code",
  "purpose": "task_handoff",
  "requested_scope": "selected_turns",
  "attach_as": "task_input",
  "retention_days": 30
}
```

服务端只创建请求：

```json
{
  "import_id": "conversation-import-1",
  "status": "awaiting_client_confirmation",
  "adapter_id": "claude-code@0.1.0",
  "expires_at": "2026-08-01T01:00:00Z"
}
```

该命令不上传内容、不读取本地文件，也不代表用户已授权完整 Transcript。适配器必须在客户端再次展示会话、轮次、脱敏结果和最终导出范围。

### 3.5 ConversationBundle

客户端确认后生成统一 Bundle：

```json
{
  "schema_version": "contentcloud.conversation_bundle/1.0",
  "bundle_id": "bundle-1",
  "import_id": "conversation-import-1",
  "client": {
    "id": "claude-code",
    "client_version": "1.0.82",
    "adapter_version": "0.1.0"
  },
  "source": {
    "format": "claude-code.transcript-jsonl/v1",
    "session_ref": "hmac:opaque-session-reference"
  },
  "purpose": "task_handoff",
  "scope": {"mode": "selected_turns", "selected_count": 3},
  "target": {"task_id": "task-1", "stage_run_id": "stage-run-4"},
  "content": [
    {"kind": "summary", "text": "已完成资料核对，仍缺少一项素材权利说明。"}
  ],
  "redaction": {
    "applied": true,
    "policy_digest": "sha256:<64 hex>",
    "removed_types": ["local_path", "credential", "account_identifier"]
  },
  "consent": {"full_transcript": false, "confirmed_at": "2026-08-01T00:15:00Z"},
  "content_digest": "sha256:<64 hex>",
  "exported_at": "2026-08-01T00:15:01Z"
}
```

服务端必须校验：

- Import 处于 `awaiting_client_confirmation` 且未过期；
- 上传节点和 adapter 与请求一致；
- Tenant、Environment、Project、Task 和 StageRun 作用域一致；
- Bundle Schema、`content_digest` 和大小限制有效；
- `redaction.applied=true` 且规则摘要符合 Environment 策略；
- `scope.mode=full_transcript` 时适配器声明支持，且 `consent.full_transcript=true`；
- `session_ref` 为环境范围内不可逆引用，不包含本机路径或客户端账号；
- 内容不包含模型内部思考、凭据、未授权来源正文或可执行任意命令。

Bundle 接收后只可转为 `task_input` 或 `evidence_candidate`。转为 Revision、Knowledge、Evidence、Decision 或 AcceptedSnapshot 必须使用独立业务命令和对应权限。

## 4. 知识契约

### 4.1 KnowledgeObject

所有类型化知识对象共享稳定身份和追溯字段，类型自己的载荷放在 `payload`，不能把状态、来源和权利塞进自由文本：

```json
{
  "id": "claim:light-portable",
  "object_type": "Claim",
  "layer": "expression",
  "status": "needs_review",
  "version": 18,
  "project_id": "project_new_product",
  "payload": {
    "statement": "轻量便携",
    "scope": ["short_video"],
    "qualifiers": ["current_batch"]
  },
  "evidence_refs": ["evidence:spec-weight", "evidence:decision-42"],
  "relation_refs": ["product:new-product", "gap:public-claim-scope"],
  "rights_refs": [],
  "decision_ref": null,
  "digest": "sha256:...",
  "created_at": "2026-08-01T08:00:00Z"
}
```

`object_type`、`status` 和 `payload` 必须通过对应 Schema 组合校验。状态变更命令必须携带当前 version/digest、目标状态、理由、主体和所依据的 Evidence；未知状态和跨类型状态转换直接拒绝。

### 4.2 Source 与 Evidence

```json
{
  "source": {
    "id": "source:spec-v3",
    "kind": "workspace_file",
    "display_name": "新品规格表 v3.xlsx",
    "mime": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    "digest": "sha256:...",
    "owner_id": "user_02",
    "disclosure": "project"
  },
  "evidence": {
    "id": "evidence:spec-weight",
    "source_id": "source:spec-v3",
    "locator": { "sheet": "Sheet1", "range": "B3:B12" },
    "content_digest": "sha256:...",
    "status": "verified"
  }
}
```

服务端保存 locator 和 digest；是否返回原文由 disclosure policy 决定。来源新 digest 不允许覆盖旧 Evidence，只能创建新证据版本和候选对象。

### 4.3 KnowledgePack 与 Snapshot

知识包定义用途、对象选择条件和查询策略；Snapshot 固化实际对象版本与 digest：

```json
{
  "pack_id": "pack:new-product-v4",
  "pack_version": 4,
  "purpose": "new_product_content",
  "object_refs": ["product:new-product@13", "assertion:base-parameters@7"],
  "query_policy": {
    "eligible_statuses": ["verified", "approved", "valid", "active"],
    "block_on_conflict": true,
    "block_on_rights_failure": true
  },
  "snapshot_id": "knowledge_snapshot_42",
  "digest": "sha256:..."
}
```

发布 Pack、生成 Snapshot 和绑定 TaskRun 是三个独立命令。Snapshot 生成后不可修改；对象更新必须发布新 Pack 版本或生成新 Snapshot。

### 4.4 KnowledgeQueryResult

```json
{
  "snapshot_id": "knowledge_snapshot_42",
  "query_digest": "sha256:...",
  "eligible": [{ "object_id": "assertion:base-parameters", "evidence_refs": ["evidence:spec-weight"] }],
  "blocked": [{ "object_id": "claim:light-portable", "reason": "STATUS_NEEDS_REVIEW" }],
  "gaps": [{ "object_id": "gap:public-claim-scope", "next_action": "REQUEST_SOURCE" }]
}
```

模型生成的回答只能引用 `eligible`。`blocked` 和 `gaps` 必须原样保留给 SOP 检查、Task 下一动作和审计。

## 5. 命令 API

| 命令 | 说明 | 重要约束 |
| --- | --- | --- |
| `POST /tasks` | 创建 Task 和首个 Run | 固定 Project/SOP/Environment |
| `POST /tasks/{id}/claim` | 领取当前 StageRun | 单写者、幂等、过期 lease |
| `POST /tasks/{id}/start` | 开始执行 | 只允许 ready/claimed |
| `POST /tasks/{id}/pause` | 暂停 | 记录原因和恢复条件 |
| `POST /tasks/{id}/resume` | 恢复 | 重新检查 SOP、能力、输入和权限 |
| `POST /tasks/{id}/cancel` | 取消 | 不删除历史 Run 和 Revision |
| `POST /tasks/{id}/publish` | 提交 Revision 摘要 | 必须绑定输入和 Environment digest |
| `POST /tasks/{id}/gates/{gateID}/evaluate` | 提交检查结果 | 绑定当前 Revision digest |
| `POST /tasks/{id}/decisions` | 提交人工决定 | 角色、依据、版本和防重放 |
| `POST /tasks/{id}/deliveries` | 创建交付包 | 只接受有效 AcceptedSnapshot |
| `POST /tasks/{id}/conversation-imports` | 创建客户端导出请求 | 不上传或解析 Transcript |
| `POST /conversation-imports/{id}/bundle` | 客户端提交 ConversationBundle | adapter、脱敏、授权、作用域和 digest 校验 |
| `POST /conversation-imports/{id}/cancel` | 取消未完成导入 | 不删除已经转成的业务事实 |
| `GET /conversation-imports/{id}` | 查询导入状态 | 只返回授权范围内的元数据和内容 |
| `POST /knowledge/sources` | 登记 Source | owner、披露范围和 digest 必填 |
| `POST /knowledge/ingestions` | 创建摄取任务 | 只产生 Evidence 和候选对象 |
| `POST /knowledge/objects/{id}/transitions` | 提交对象状态决定 | 类型状态机、Evidence、理由和 digest |
| `POST /knowledge/conflicts/{id}/resolve` | 解决冲突 | 保留双方 Evidence 和解决依据 |
| `POST /knowledge/gaps/{id}/tasks` | 从缺口创建补料 Task | 绑定 Project、owner 和阻断影响 |
| `POST /knowledge/packs` | 创建知识包草稿 | 用途、选择器和查询策略 |
| `POST /knowledge/packs/{id}/publish` | 发布知识包版本 | lint、冲突、Rights 和影响分析 |
| `POST /knowledge/snapshots` | 生成不可变快照 | 固化对象版本、Evidence 和 digest |
| `POST /knowledge/query` | 确定性知识查询 | 返回 eligible、blocked 和 gaps |
| `POST /sops` | 创建 SOP 草稿 | 管理员或流程负责人 |
| `POST /sops/{id}/versions` | 创建新版本 | 不修改历史版本 |
| `POST /sops/{id}/publish` | 发布版本 | lint、影响分析、审批权限 |
| `POST /sops/{id}/retire` | 退休版本 | 不能影响已绑定历史 Run |
| `GET /workspace/tasks` | 工作区 Task Projection | 保留筛选和分页游标 |
| `GET /projects/{id}/sop` | Project 当前 SOP Projection | 只读绑定和使用情况 |

## 6. 版本、幂等和并发

- 所有写命令必须接受 `Idempotency-Key`。
- Task、TaskRun、SOPVersion、Revision、Decision 和 Delivery 都使用服务端 ID。
- Project、Task 和 SOP 编辑使用 `row_version` 或 `If-Match`，冲突返回明确错误码。
- Gate、Decision 和 Publish 必须携带当前 digest；digest 不匹配时只能读取，不能写入。
- TaskRun 的 claim 使用 lease；失联由服务端过期回收，不允许第二个客户端静默抢写。
- Projection 查询带 `generated_at`、`source_high_watermark` 和 `schema_version`。

## 7. 权限矩阵

| 能力 | Tenant Admin | Project Manager | Process Owner | Editor | Reviewer | Client Approver | Viewer |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 查看 Task | 全部 | 项目范围 | 项目范围 | 分派范围 | 审核范围 | 被邀请范围 | 授权范围 |
| 创建 Task | 是 | 是 | 是 | 按策略 | 否 | 否 | 否 |
| 管理 SOP 草稿 | 是 | 可授权 | 是 | 否 | 否 | 否 | 否 |
| 发布 SOP | 是 | 可授权 | 可授权 | 否 | 否 | 否 | 否 |
| 修改 Gate | 是 | 可授权 | 是 | 否 | 否 | 否 | 否 |
| 执行 Task | 是 | 是 | 是 | 是 | 可授权 | 否 | 否 |
| 内部决定 | 是 | 可授权 | 可授权 | 否 | 是 | 否 | 否 |
| 客户决定 | 否 | 否 | 否 | 否 | 否 | 是 | 否 |
| 创建 Delivery | 是 | 是 | 可授权 | 可授权 | 否 | 否 | 否 |
| 导入本地对话 | 是 | 是 | 是 | 自己的 Task | 审核范围 | 否 | 否 |
| 查看知识对象 | 全部 | 项目范围 | 项目范围 | 分派范围 | 审核范围 | 被披露范围 | 授权范围 |
| 登记来源/发起摄取 | 是 | 是 | 是 | 可授权 | 否 | 否 | 否 |
| 接受知识/解决冲突 | 是 | 可授权 | 是 | 否 | 可授权 | 被邀请决定 | 否 |
| 发布知识包/生成快照 | 是 | 可授权 | 是 | 否 | 否 | 否 | 否 |
| 查看审计 | 是 | 项目范围 | 项目范围 | 自己相关 | 审核范围 | 决定范围 | 否 |

“可授权”必须来自服务端 Role Policy，不能由前端角色字符串推断。

## 8. 数据披露与本地优先

### Web 可以看到

- Task 标题、状态、Stage、负责人和下一动作；
- 输入 Snapshot、Evidence/Rights 元数据和 digest；
- LocalRun/AgentRun 的阶段、检查、版本和上报时间；
- 已 publish 的 Revision、Gate、Decision、AcceptedSnapshot 和 Delivery；
- 当前角色有权访问的 ConversationImport 状态、脱敏摘要和选择性内容；
- 当前角色有权访问的知识对象元数据、状态、关系、Source/Evidence 定位和 Snapshot digest；
- 审计、错误码和恢复建议。

### Web 默认不能看到

- 未 publish 的正文、候选文件和本机路径；
- 完整聊天 transcript、模型内部思考和未授权原始来源；
- 客户端私有会话格式、会话索引、本机文件位置和未导出的轮次；
- 能够重放外部账号操作的令牌；
- 未被当前角色允许披露的 Evidence 正文或 Rights 证明文件。
- 未公开 Source 的原件内容、Workspace 绝对路径和仅用于本地摄取的临时材料。

### 安全硬门禁

- 租户、Environment、Project、Task 和 Revision 全部做服务端作用域校验。
- 任何跨租户 ID、外部 URL、绝对路径和未经签名的 resource link 都拒绝。
- Gate、Decision、Publish、Delivery 和 SOP 发布均写入不可变 AuditEvent。
- 删除操作只允许软删除或退休；正式 Revision、Decision、Snapshot 和 Delivery 不物理删除。
- Agent 和 Automation 只能调用已注册 Capability，输入输出通过 Schema 校验。
- ConversationBundle 必须来自请求中绑定的 ClientAdapter，且完整 Transcript 同时满足客户端能力与显式授权。
- ConversationImport 不得自动创建 Revision、Knowledge、Evidence、Decision 或 AcceptedSnapshot。
- KnowledgeQuery 只能从绑定 Snapshot 和允许状态返回 `eligible`，不能绕过冲突、Rights 或披露范围。

## 9. 错误契约

客户端根据错误码渲染可行动状态，不读取错误字符串猜测业务：

| 错误码 | 含义 | 用户动作 |
| --- | --- | --- |
| `SOP_VERSION_RETIRED` | 新任务不能使用已退休版本 | 选择已发布版本 |
| `SOP_DIGEST_MISMATCH` | 运行绑定与当前定义不一致 | 只读历史，创建新 Run |
| `TASK_CLAIM_CONFLICT` | 已有其他执行者 | 查看当前执行者或等待 lease |
| `INPUT_SNAPSHOT_INVALID` | 输入过期、撤销或不属于 Project | 补料或选择新输入 |
| `GATE_REVISION_STALE` | 决定针对旧 Revision | 打开当前 Revision |
| `CAPABILITY_DISABLED` | 环境未启用所需能力 | 联系管理员或切换执行方式 |
| `RIGHTS_REQUIRED` | 权利硬门禁未满足 | 补充 Rights 或替换素材 |
| `PROJECTION_STALE` | 页面不是当前高水位 | 刷新后重试命令 |
| `CLIENT_ADAPTER_UNAVAILABLE` | 指定客户端未连接或版本不兼容 | 打开本地客户端或更新适配器 |
| `CONVERSATION_SCOPE_UNSUPPORTED` | 适配器不支持请求范围 | 改选摘要或选择性片段 |
| `TRANSCRIPT_CONSENT_REQUIRED` | 完整 Transcript 缺少显式授权 | 回到客户端确认或缩小范围 |
| `CONVERSATION_BUNDLE_INVALID` | Bundle Schema、digest、脱敏或作用域失败 | 在客户端重新导出 |
| `KNOWLEDGE_TRANSITION_INVALID` | 对象类型不允许当前状态转换 | 查看状态机和所需 Evidence |
| `KNOWLEDGE_SOURCE_CHANGED` | Source digest 与 Evidence 版本不一致 | 重新摄取并创建候选版本 |
| `KNOWLEDGE_CONFLICT_BLOCKING` | 查询或发布命中未解决冲突 | 解决冲突或调整查询范围 |
| `KNOWLEDGE_GAP_BLOCKING` | 必需知识缺口尚未补齐 | 创建补料 Task |
| `KNOWLEDGE_SNAPSHOT_STALE` | TaskRun 请求的快照已被撤销或不在范围 | 显式选择新快照并创建新 Run |

## 10. 测试要求

### 契约测试

- SOP Schema 的未知字段、重复 Stage、非法 Gate 和 digest 漂移；
- Task 创建时 Project、Environment、SOP Version 的作用域校验；
- TaskRun 绑定版本不可变；
- Gate 和 Decision 绑定 Revision digest；
- Delivery 只接受 AcceptedSnapshot；
- ClientAdapter 版本、私有格式和 ConversationBundle Schema 相互独立；
- ConversationImport 状态机、幂等上传、过期和取消；
- 知识对象类型状态机、Source/Evidence digest、冲突和缺口阻断；
- KnowledgePack 发布、不可变 Snapshot、query eligible/blocked/gaps；
- 所有命令的幂等和 row version 冲突。

### 安全测试

- 跨租户、跨 Project、越权角色和已撤销成员；
- 恶意 focus、绝对路径、外部 URL、伪造 resource link；
- 本地未 publish 数据不出现在 Projection、日志和错误中；
- Web 不能发现本机会话或解析客户端私有格式；
- 完整 Transcript 缺少能力声明或显式授权时必须拒绝；
- 导入内容不能绕过命令直接成为 Revision、Knowledge、Evidence、Decision 或 AcceptedSnapshot；
- KnowledgeQuery 不能泄露未披露 Source/Evidence 原文或绕过 Rights、状态和冲突；
- Capability 关闭、Environment 变更和 SOP 退休期间的行为。

### 运行测试

- claim 冲突、lease 过期、重复回调、取消和恢复；
- Agent/Automation/Swarm 部分失败和重复 Attempt；
- TaskRun 中断后新客户端继续；
- ConversationImport 请求、客户端离线重试、重复 Bundle 和 adapter 升级；
- Source 重复摄取、Source digest 变化、状态决定并发和 Snapshot 影响分析；
- 新 SOP 版本发布不改变历史 Run。
