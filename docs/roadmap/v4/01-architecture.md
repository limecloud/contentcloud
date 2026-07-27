# V4 Browser-First 架构

## 1. 架构目标

V4 让 ContentCloud 从“Codex 本地创作 + 独立 Web 后台”变成两个可并排工作的产品表面，同时保持 V3 的事实源和权限边界：

```text
本地候选与连续创作                  云端治理工作台
─────────────────                 ─────────────────
Workspace                          ContentCloud Web
LocalRunContext                    ProjectProjection
Knowledge Pages                    SubmissionRevision
ContentBatch                       Decision
HandoffRecord                      ApprovedSnapshot
        │                                  │
        └──────── explicit exchange ───────┘
                 CLI Gateway
```

Browser 解决“用户看什么、定位到哪里”，CLI Gateway 解决“哪些数据跨边界”。两者不能合并。

## 2. 组件职责

| 组件 | 负责 | 不负责 |
| --- | --- | --- |
| ContentCloud Scene Plugin | 识别查看意图、选择 MCP Tool、要求 Browser 导航、验证可见结果 | 保存客户数据、替代 Browser 宿主 |
| `contentcloud-local` MCP | 解析当前 WorkspaceBinding、校验目标、构造受控页面链接、返回 resource link | 自动点击 Web、执行审核、接受页面指令 |
| Codex/ChatGPT Browser | 打开 URL、保持 Web 可见、允许用户或 Agent 在授权范围内交互 | 成为业务状态同步协议 |
| ContentCloud Web | 展示 ProjectProjection、Revision 和证据；执行 Comment、Decision、Assignment、Context Revision、Automation Plan 等授权命令 | 读取任意本机目录、编辑未发布本地草稿、绕过命令确认 |
| CLI Gateway | publish/pull、幂等、披露、认证和业务交换 | 操作 Browser 布局 |
| Page Contract Registry | 将类型化 view/focus 映射为稳定路由 | 保存业务对象状态 |

## 3. 逻辑架构

```text
User request
   │
   ▼
Scene Skill
   │  1. read conversation context
   │  2. choose view + focus ref
   ▼
contentcloud_open_project_view
   │
   ├─ resolve explicit directory / cwd
   ├─ read .contentcloud/workspace.yaml
   ├─ verify project + environment binding
   ├─ validate allowlisted view/focus combination
   ├─ build canonical Web route
   └─ return MCP resource_link + structuredContent
          │
          ▼
Agent invokes Browser navigate
          │
          ▼
ContentCloud Web auth + tenant authorization
          │
          ▼
ProjectProjection / focused object
```

MCP Tool 完成后不能声称 Browser 已经打开。只有宿主 Browser 调用成功并检查到目标页面，Agent 才能报告可见结果。

## 4. 云端治理工作台不是只读页面

`contentcloud_open_project_view` 是只读导航 Tool；它的只读注解不代表 ContentCloud Web 整体只读。页面打开后，用户可以按 V3 命令边界执行：

- Comment 与 Review feedback。
- 类型化 Decision，并绑定当前 revision digest。
- 创建或调整 WorkAssignment。
- 创建 ProjectContext 修订请求。
- 在权限和确认允许时管理 Automation Plan。

Web 不能执行：

- 直接修改 Workspace 中未发布的 Knowledge、Brief、Script 或 Asset 候选。
- 在没有 publish 的情况下把本地状态伪装成云端 Revision。
- 自动 pull 决定、反馈或 Assignment 到本机。
- 因为页面已打开就自动批准 Claim、Rights 或其他最终人工决定。

Browser 应保持当前项目和焦点可见，使用户可以在 Agent 工作期间查看云端状态并完成自身职责。它是持续工作台，但不是同步通道或第四个数据平面。

## 5. Browser 导航不是同步

### 5.1 允许的数据

导航结果可以包含：

- ContentCloud 服务端基址的受控派生 URL。
- project ID、view ID。
- 已 publish 的 subject、revision、assignment 或 snapshot ID。
- 用于检测陈旧页面的 expected digest。
- 不敏感的页面标题和恢复说明。

### 5.2 禁止的数据

导航结果不能包含：

- Workspace Credential、Bearer Token、PKCE verifier。
- 本机绝对路径。
- Codex thread/conversation ID。
- LocalRun 的未发布正文、prompt、transcript 或隐藏推理。
- 原件正文或未授权 Evidence。
- 任意调用者提供的外部 URL。

## 6. 视图模型

首版 view ID 与 V3 Web 信息架构保持一致：

| View ID | Web 页面 | 可选焦点 |
| --- | --- | --- |
| `setup` | 接入、初始化与环境诊断 | bootstrap attempt / support code / workspace binding / environment health |
| `overview` | 项目总览 | next action |
| `context` | 方法论与上下文 | context version / methodology dimension |
| `knowledge` | 可信知识 | source / evidence / fact / claim / asset / rights / conflict |
| `intelligence` | 市场情报 | insight / intelligence revision |
| `strategy` | 营销策略 | strategy subject / adopted insight |
| `planning` | 内容策划 | campaign / brief / experiment |
| `creative` | 创意与剧本 | content batch / script / media candidate |
| `review` | 审核协作 | submission revision / review cycle |
| `delivery` | 交付制作 | delivery package / snapshot |
| `learning` | 结果学习 | observation / learning candidate |
| `automation` | Automation 与运行 | plan / task run / attempt |

Page Contract Registry 是 view 到 route/query/authorization/test 的单一事实源。MCP、Web 导航、`next_actions` 和测试都复用它的稳定 view ID，不能各自维护一套路由映射。

当前开发中的 V3 已在 `web/src/v3/page-contracts.ts` 提供 view ID 草案，并由 `web/src/router.tsx` 生成项目路由；这些代码是联合收敛的工作基线，不是已冻结契约。V3/V4 应在同一个 Registry 上补齐并确认 focus、authorization、query 和 link builder 契约，不另建第二份 view allowlist。环境诊断暂复用 `setup` 页面并通过类型化 focus 定位；如 V3 信息架构评审改变这一归属，应同步更新 Registry、MCP 和测试，而不是只修改文档或单侧路由。

## 7. 双向入口

### 7.1 Codex 到 Web

```text
Codex local context
  -> view + published focus ref
  -> resource_link
  -> Browser
  -> authenticated Web projection
```

典型触发：

- “打开项目总览”。
- “让我看这次提交的审核状态”。
- “打开阻断这个脚本的 Claim 和证据”。
- “打开刚拉取的 Assignment”。
- “查看环境诊断”。

### 7.2 Web 到 Codex

```text
Web next action / Assignment / feedback
  -> authenticated BFF returns codex://new?prompt=...
  -> Codex 只预填新对话，不自动发送
  -> 用户选择本机 Workspace
  -> workspace_context
  -> verify project_id; mismatch stops
  -> explicit pull when required
  -> local Run claim
```

Web 只能把业务引用和不含秘密的恢复 Prompt 交给 Codex。服务端不知道本机绝对路径，因此不得伪造 `path=` 或声称已打开 verified Workspace Root；真正的本地接管由用户选择的 Workspace、`workspace_context`、digest 和 RunClaim 决定。当前实现覆盖 Project 与 review feedback；Assignment 入口等待 V3 W3-01 的 WorkAssignment 契约。

## 8. 状态一致性

V4 接受两侧状态存在合理时间差，并明确展示边界：

| 场景 | 正确行为 |
| --- | --- |
| 本地刚产生候选但未 publish | Web 显示最近已知投影，并标注本地可能有未提交工作，不显示候选正文 |
| publish 成功 | 返回本次 revision 的精确 Web 入口 |
| Web 已做决定但本地未 pull | Codex 可以打开决定页面；本地状态仍保持旧 Snapshot，直到显式 pull |
| Revision 已被更新 | 旧链接显示历史 Revision；决定动作必须绑定该 digest，不自动跳到新版本后继续操作 |
| 上游 Snapshot 失效 | Web 显示 ImpactAction；本地在下次 query/pull 后重新计算 eligibility |

Browser 刷新不触发 publish，Web 页面轮询也不能读取本地文件。

## 9. 宿主能力与降级

| 宿主 | 行为 |
| --- | --- |
| ChatGPT Desktop + Browser | Agent 导航并验证右侧页面 |
| ChatGPT Web + Browser | 打开可访问的云端 Web 页面；不能访问用户本机 localhost |
| Codex CLI | 返回 URL 和目标摘要，不声称已打开 Browser |
| Codex IDE Extension | 返回 URL 和目标摘要；用户在外部浏览器打开 |
| Browser 未安装/未启用 | 返回安装/启用说明和可点击链接，当前业务操作仍可继续 |

ContentCloud 不能仅根据 plugin manifest 或文案推断 Browser 可用。应尝试使用宿主能力，并对不可用结果显式降级。

## 10. 失败恢复

| 失败 | 恢复 |
| --- | --- |
| Workspace 未绑定 | 返回 `WORKSPACE_NOT_BOUND`，引导 setup，而不是拼接项目 URL |
| Web 基址缺失或环境不可信 | 返回 `WEB_TARGET_UNTRUSTED`，运行 environment doctor |
| Browser 不可用 | 保留 resource link，说明当前宿主限制 |
| Browser 未登录 | Web 登录后通过 allowlisted return path 回到目标页 |
| 用户无项目权限 | Web 返回统一无权限页，不泄露对象是否存在 |
| focus 对象不存在 | 打开所属 view 的空/错误状态，并保留 project context |
| digest 不匹配 | 显示 stale 状态和新版本入口，禁止把决定静默应用到新版本 |
| publish 网络状态不明 | 先按 idempotency key 查询结果，再生成 revision 链接 |
| 页面加载失败 | Agent 报告失败并保留 URL，不把“Tool 调用成功”当成页面成功 |
