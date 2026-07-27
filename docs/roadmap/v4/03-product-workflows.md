# V4 双向产品流程

## 1. 总原则

用户应该在最合适的表面完成动作：

- Codex：读取本地资料、生成候选、运行 lint、管理 LocalRun、publish/pull。
- Browser 中的 ContentCloud Web：作为云端治理工作台，查看投影、证据和版本，并执行授权的评论、人工决定和任务分派。
- CLI Gateway：唯一结构化跨边界交换。

“并排显示”不等于“自动同步”，也不等于“允许 Agent 代替审核人决定”。

## 2. 统一 Codex 接入入口

```text
用户或 Agent 打开 /codex
  -> Browser: 显示安装、登录和连接页面
  -> Agent: 读取同版本 plain-text 指南
  -> 检查当前宿主是否可安装本地 Plugin
  -> 安装并验证 Scene Plugin / MCP
  -> PKCE 登录 ContentCloud
  -> 创建新的 Workspace 对话
  -> conversation context + bootstrap/doctor
  -> Browser 打开 setup 或 overview
```

新对话是必要边界：安装对话启动时已经冻结 Tool inventory，不能因为安装命令成功就声称当前对话已经获得 ContentCloud Tool。远程 Web 或隔离执行环境不能修改用户桌面 Plugin 配置，应返回桌面端继续入口，而不是尝试绕过宿主边界。

## 3. 打开项目总览

```text
用户：打开 ContentCloud 项目总览
  -> Scene Skill 读取 conversation context
  -> 调用 contentcloud_open_project_view(view=overview)
  -> MCP 从 WorkspaceBinding 解析 project_id 和可信 server base
  -> 返回 resource_link
  -> Agent 调用 Browser navigate
  -> Web 验证登录与项目权限
  -> 显示 ProjectProjection、Gate、阻断和下一动作
  -> Agent 验证页面项目 ID/标题后报告完成
```

如果 Browser 不可用，Agent 返回链接和项目摘要，不要求用户重新描述目标。

## 4. 本地 publish 后打开审核页

```text
本地业务对象保存
  -> lint
  -> publish preflight
  -> 用户确认同一 plan_id
  -> publish apply
  -> Server 创建 immutable SubmissionRevision
  -> Tool 返回 revision ID/digest + review resource_link
  -> Browser 打开该 Revision
```

页面必须明确展示：

- Submission 类型和 Revision 号。
- revision digest。
- base ApprovedSnapshot。
- 对象/字段 diff。
- SourceDisclosure 和 Evidence 可见性。
- LocalRunSummary 的 checks，不显示 transcript。
- 当前审核状态和允许动作。

## 5. 从 blocked 内容定位阻断对象

```text
Codex knowledge_query / content lint
  -> blocked_ids
  -> 用户要求查看原因
  -> 仅当阻断对象已 publish 时生成 Web focus ref
  -> Browser 打开 knowledge/creative 对应对象
```

未 publish 的本地阻断对象只在 Codex 和 Workspace 中展示，不能为了右侧预览而静默上传。

Web 中的“解决阻断”只能：

- 打开已有 DecisionRequest。
- 创建补料 Assignment。
- 查看 Source/Evidence/Impact。

不能提供“忽略阻断”或直接修改本地候选的按钮。

## 6. Web 人工决定后回到 Codex

```text
审核人在 Browser/Web 记录 Decision
  -> Server 绑定 revision digest
  -> 生成 DecisionDelta / ApprovedSnapshot
  -> Web 显示“在 Codex 继续”
  -> BFF 返回 codex://new?prompt=...，不包含 path
  -> Codex 预填新对话，用户选择本机 Workspace
  -> workspace_context 验证 project_id
  -> review_feedback_list 只读核对 Revision + digest
  -> 用户明确要求 pull
  -> immutable bundle 写入 inbox/cache
  -> 新建或接管本地修订 Run
```

Web 的恢复 Prompt 可以包含 project ID、submission revision ID、assignment ID 和完整 digest 等不敏感引用，但不能包含 token、绝对路径、客户正文、评论正文或旧对话 transcript。`codex://new` 只预填 composer，不自动发送；未选择 Workspace 或 `workspace_context.project_id` 不匹配时必须停止，不扫描目录。未经用户明确要求，review feedback 恢复不得自动 pull、claim 或写入。

当前代码已实现 Project 与 review feedback 两个入口。Assignment 仍依赖 V3 W3-01，不能以通用 project Prompt 伪装成已实现的 Assignment pull。

## 7. Assignment 流程

### 7.1 Web 创建任务

```text
Web ProjectProjection / 缺口页面
  -> 用户创建 WorkAssignment
  -> Server 绑定 input snapshots + ExecutionBundle
  -> Web 显示 Assignment 详情
  -> “在 Codex 继续”
```

### 7.2 Codex 拉取与执行

```text
Codex 新对话
  -> conversation context
  -> 显式 pull assignment
  -> 验证 signature / environment / capability
  -> init + claim LocalRun
  -> 本地生成、lint、checkpoint
```

### 7.3 Codex 回看云端任务

执行中用户可要求“打开当前 Assignment”，由同一通用 Tool 定位 Browser 页面。Web 只能显示已知状态和显式上报摘要，不能声称掌握未 publish 的本地进度。

## 8. 初始化与环境诊断

```text
ContentCloud Web setup
  -> 连接 Prompt
  -> Codex bootstrap plan/apply
  -> PKCE 浏览器授权
  -> WorkspaceBinding + V3 files + doctor
  -> 新 Workspace 对话
  -> Browser 打开 setup 页面并聚焦 environment health，验证连接结果
```

Browser 导航不能替代 bootstrap 的 PKCE 授权，也不能仅凭页面显示“已连接”判断本地 `.contentcloud/` 可写。完成状态仍来自真实 doctor 和原子写探针。

## 9. 多对话与 Browser

多个 Codex 对话可以打开同一项目 Web 页面，但本地写权限仍由 RunClaim 控制：

```text
Conversation A ─┐
Conversation B ─┼─ use same cloud governance workbench
Conversation C ─┘

Only claimed conversation
  -> mutate one LocalRun revision
```

Browser tab、Codex conversation 和 LocalRun 不是一一对应关系。任何实现都不能用 Browser tab ID 或 Codex thread ID 作为业务锁。

## 10. 页面焦点与返回路径

页面深链必须满足：

- 刷新后仍定位到同一业务对象或不可变 Revision。
- 关闭 Browser 不影响 LocalRun。
- 从对象详情返回列表时保留项目和 view 上下文。
- 登录重定向后恢复原目标。
- 对象已删除/不可见时回到所属 view 的明确错误状态。
- stale Revision 保持历史可读，不把焦点悄悄切到最新版。

## 11. 用户可见文案原则

- 使用“在 ContentCloud 查看”“在 Codex 继续”“打开本次审核”等业务文案。
- 普通用户不看到 `resource_link`、`browserHandoff`、MCP、Projection builder 等实现术语。
- “已打开”只在 Browser 导航和页面验证成功后显示。
- Web 状态必须区分“本地可能有未提交工作”“已提交”“已批准”。
- Browser 不可用时明确说“当前宿主不支持内置 Browser”，不伪装成功。

## 12. 代表性场景

| 场景 | Codex 左侧 | Browser 右侧 |
| --- | --- | --- |
| 项目启动 | 初始化、doctor、conversation context | setup、环境健康、项目边界 |
| 知识建设 | Source ingest、候选对象、lint | 已提交 Evidence、类型化决定、缺口 |
| 内容生产 | eligible/blocked query、ContentBatch | Brief/Batch 投影、阻断和审核状态 |
| 审核修订 | publish/pull、新版本修订 | Revision diff、评论、Decision |
| 交付 | Delivery lint、package publish | ApprovedSnapshot、验收和交付状态 |
| 结果学习 | Observation/Learning 候选 | 结果投影、adopt/reject 和影响范围 |
