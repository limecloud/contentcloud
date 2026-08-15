# 项目级执行客户端连接

状态：`Codex 工作区连接已实现；Claude Code 等客户端的客户侧 bootstrap 待适配`。

更新时间：2026-08-07。

上位规范：[平台基线](../../foundation/README.md)、[客户创作台需求](./02-customer-studio-requirements.md)和 [V8 宿主能力证据](../../roadmap/v8/02-host-capability-evidence.md)。

## 1. 为什么需要连接

客户登录的是简单创作台，但一部分创作步骤需要访问客户本地资料、MCP 工具或本地创作环境。连接因此是一个明确的信任和准入动作：客户确认某台电脑可以为某个项目执行获准的本地步骤，ContentCloud 记录项目绑定和连接健康状态。

连接解决的是“这项任务是否具备可用的本地执行面”，不是“把客户送进某个 Agent 客户端里完成全部工作”。搜索 API、受控采集、平台 Worker、图片/视频服务商和人工 Gate 仍可由 Runtime 按流水线能力绑定完成。

```text
客户登录
   |
   v
选择项目 ---- 未连接 ----> 发起连接会话 -> Codex 只读检查 -> 客户核对代码并确认
   |                                                        |
   +---------------- 已连接项目 <---------------------------+
                            |
                            v
                     创建客户创作任务
```

## 2. 当前发布范围

| 客户看到的能力 | 当前状态 | 说明 |
| --- | --- | --- |
| 项目连接入口 | 已实现 | `/studio/connect`，按项目选择或从新建任务跳转 |
| 可用连接目录 | 已实现 | 只返回具备 `workspace_bootstrap` 能力的客户端 |
| Codex 工作区 bootstrap | 已实现 | 生成一次性连接会话，执行只读检查，回到页面核对代码并确认 |
| Claude Code 客户侧 bootstrap | 未发布 | 具备部分本地自动化能力，但没有同等的项目连接、授权和健康检查契约 |
| 其他客户端连接 | 计划中 | 先完成 capability、bootstrap、授权、撤销、健康检查和一致性测试 |

当前连接 API 不接收客户自选的 `client_id`。这是有意的：客户只能看到服务端已经验证并发布的连接方式，不能选择一个 UI 显示了但后端仍按 Codex 流程处理的“伪多客户端”选项。

## 3. 客户体验规则

1. 连接状态按项目保存。一个项目已连接，不代表其他项目自动连接。
2. 没有任何可用项目连接时，客户进入“连接创作电脑”页面；已有连接项目可以直接进入创作台。
3. 客户选择未连接项目创建任务时，页面必须先引导连接该项目，服务端也必须以 `STUDIO_EXECUTION_CLIENT_REQUIRED` 拒绝绕过请求。
4. 已有任务、结果资产和交付不因客户端暂时离线而不可查看；只有依赖本地执行能力的新任务或下一个步骤被阻断。
5. 连接页只说明客户需要完成的动作，不展示 adapter、lease、MCP 参数、设备令牌或完整诊断日志。
6. 连接成功后，客户不需要再次选择每个步骤的执行者；Runtime 根据已发布流水线和能力绑定分配 Worker、客户端、Provider 或人工节点。

连接后的本地数据不会因为设备在线而自动全量上传。Daemon 在启动时和之后每 30 秒只读观察项目 Workspace，只同步脱敏的 ID、声明/观察摘要、状态、原因、generation 和时间；绝对路径、客户文件正文、提示词和完整 Agent 会话不进入 current-state。某项任务确实需要把资料交给 MCP、Provider 或云端 Worker 时，仍必须由该任务冻结的数据分类、TaskContract、工具白名单和可审计调用单独授权。

## 4. 多客户端发布门槛

Claude Code、Codex 和其他客户端在 Runtime 内部属于同一类执行者，但“能执行本地自动化”不等于“能作为客户项目连接方式发布”。新客户端至少必须通过：

- 项目范围的 bootstrap 和 workspace registration。
- 一次性授权、代码核对、拒绝、取消、过期和撤销。
- 连接健康检查、能力快照和项目隔离。
- 断线、重试、重复请求和客户端版本变化的契约测试。
- 最小权限、数据披露、审计和支持编号验证。

在这些条件满足前，客户端只可以作为 Runtime 的内部执行适配器或灵感采集的可选能力，不能出现在客户连接选择器中，也不能出现在“已连接”数量中。

Plugin、Skill、stdio MCP、Workspace 绑定、MCP Apps、private Browser handoff 和客户侧 bootstrap 是不同能力维度。Cursor、VS Code GitHub Copilot、Hermes、OpenClaw 等进入上游兼容目录，只能作为实现候选；完整宿主矩阵和逐宿主验收门禁见[本地工作台技术方案](./05-local-workbench-browser.md)。

## 5. 对代码和契约的约束

- `StudioProject.execution_client_connected` 是项目级客户投影字段，不是 Runtime 的全局状态。
- `/api/studio/execution-clients` 只返回已发布且具备 `workspace_bootstrap` 的客户端为 `available=true`。
- `/api/studio/projects/{project_id}/connect-sessions` 创建的是受控 `ConnectSession`，不把连接会话当作任务状态或业务结果。
- `ConnectSession` 的状态变化必须通过服务端 bootstrap 授权流程产生，前端不能自行把项目标成已连接。
- 任务创建仍需回源检查项目是否有已连接设备；读投影过期时不能绕过该门禁。
- 未来支持第二种客户端时，先扩展连接契约和适配器测试，再增加客户 UI 选项；不在现有 Codex 流程上增加未生效的多选字段。
- 项目连接投影只回答“是否具备获准的本地执行面”；实际领取前还必须由 Runtime 校验该 Daemon 的 Workspace current-state 为 `ready`，并逐项比对 Environment、Plugin、Skill、MCP、Workspace 五类冻结声明。
- Codex、Claude Code 等宿主必须实现同一 `AgentHarnessAdapter` 端口。Skill 以完整只读 `SKILL.md` 注入 Attempt 专属目录；MCP 只通过 Attempt-scoped Gateway 和工具白名单提供；Plugin 只负责宿主分发和安装，不拥有连接、租约或任务终态。
- Agent 不直接在客户交互式 Workspace 中执行。Runtime 创建与 Attempt 一一对应的隔离目录并注入 TaskContract、Output Schema、Skill 和租约；终态后只清理该目录，不删除客户 Workspace。

## 6. 验收

1. 未连接项目不能创建新的客户任务，错误明确指向连接动作。
2. 连接会话可被创建、查询、批准、拒绝、取消和过期，且跨租户不可读写。
3. 客户页面只显示已验证的连接方式；Claude Code 在 bootstrap 未完成前不能显示为可连接。
4. 连接成功后，至少一个已发布客户体验可以开始任务；任务内部仍可混用 Worker、Provider、Agent 和人工 Gate。
5. 本地客户端断线不会删除或隐藏历史任务、结果资产和交付包。
6. Workspace 观察不包含绝对路径或客户文件正文；状态或五类声明漂移时，新 Attempt 在创建前被拒绝，已有 Attempt 保持原快照。
7. 同一个冻结 TaskContract 可经统一 Harness 端口交给 Codex 或 Claude Code；两者获得相同的 Skill、Output Schema、MCP 工具范围和 Runtime 围栏语义。
