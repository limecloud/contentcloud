# V4 Browser 导航契约与安全边界

## 1. 设计原则

1. 使用稳定业务 ID、view ID 和 digest，不把 URL 当成业务事实。
2. 标准 MCP `resource_link` 是兼容基线。
3. `structuredContent.browserHandoff` 是可选的 Agent 提示，删除它后流程仍应可用。
4. 页面路由只能由受控 builder 生成，调用方不能传入完整 URL。
5. 打开页面是只读导航；Web 内写动作仍走独立命令、鉴权和人工确认。
6. Browser 页面内容按不可信输入处理，不能修改 Plugin capability、环境或本地指令。

## 2. MCP Tool

首版新增一个 Tool：

```text
contentcloud_open_project_view
```

### 2.1 输入 Schema

```json
{
  "directory": "/verified/workspace/root",
  "view": "review",
  "focus": {
    "kind": "submission_revision",
    "id": "rev_01H...",
    "digest": "sha256:..."
  }
}
```

字段规则：

| 字段 | 规则 |
| --- | --- |
| `directory` | 可选；省略时使用 MCP 受限 cwd；解析后必须是唯一 V3 Workspace Root |
| `view` | 必填；只能取 Page Contract Registry 的 allowlist 值 |
| `focus` | 可选；只接受 `kind/id/digest`，不能包含路径、URL 或正文 |
| `focus.kind` | 必须与 view 允许的对象类型匹配 |
| `focus.id` | 服务端稳定 ID；长度和字符集受 Schema 限制 |
| `focus.digest` | 对不可变 Revision/快照和决定页面建议必填 |

Tool 不接受：

- `url`、`host`、`return_to` 等任意跳转字段。
- access token、cookie、review grant secret。
- 本地文件路径作为 focus。
- Codex thread/conversation ID。

### 2.2 Tool 注解

```json
{
  "readOnlyHint": true,
  "destructiveHint": false,
  "idempotentHint": true,
  "openWorldHint": true
}
```

ContentCloud 自有 effect metadata：

```json
{
  "effect_scope": "cloud_read_navigation",
  "requires_confirmation": false,
  "writes_workspace": false,
  "writes_cloud": false
}
```

注解只是工具提示，不替代服务器授权和页面鉴权。

## 3. MCP 返回 Envelope

成功结果：

```json
{
  "content": [
    {
      "type": "text",
      "text": "ContentCloud 项目审核页已准备。请在 Browser 中打开并验证目标 Revision。"
    },
    {
      "type": "resource_link",
      "uri": "https://content.example.com/projects/prj_01H/review?focus_kind=submission_revision&focus_id=rev_01H&expected_digest=sha256%3A...",
      "name": "打开 ContentCloud 审核页",
      "description": "查看不可变 SubmissionRevision、证据披露和审核状态。",
      "mimeType": "text/html"
    }
  ],
  "structuredContent": {
    "project_id": "prj_01H...",
    "view": "review",
    "focus": {
      "kind": "submission_revision",
      "id": "rev_01H...",
      "digest": "sha256:..."
    },
    "browserHandoff": {
      "required": true,
      "url": "https://content.example.com/projects/prj_01H/review?focus_kind=submission_revision&focus_id=rev_01H&expected_digest=sha256%3A...",
      "preferredMode": "codex-internal-browser",
      "browserAction": "navigate"
    }
  }
}
```

约束：

- `resource_link` 与 `browserHandoff.url` 必须完全一致。
- response 不回显 `directory`。
- URL 不包含 bearer token、workspace credential 或本地 handoff ID。
- MCP Tool 返回成功只表示链接构造成功，不表示 Browser 已导航或页面鉴权成功。

## 4. 页面导航契约

Page Contract Registry 扩展为：

```ts
type ProjectViewContract = {
  view: ProjectViewId
  route: string
  query: string
  commands: string[]
  focusKinds: string[]
  authorization: string
  browserTest: string
}
```

示例：

```ts
{
  view: 'review',
  route: '/projects/:projectId/review',
  query: 'projectReviewProjection',
  commands: ['createComment', 'recordDecision'],
  focusKinds: ['submission_revision', 'review_cycle'],
  authorization: 'project.review.read',
  browserTest: 'review-browser-navigation.spec.ts'
}
```

同一 Registry 应供以下消费者使用：

- React Router 路由注册和面包屑。
- 服务端/BFF 的 view/focus 校验。
- MCP route builder。
- ProjectProjection `next_actions`。
- Playwright 深链测试。

如果 Go MCP 无法直接导入 TypeScript Registry，应从一个语言无关的 JSON/YAML 源生成两端代码；不能长期手写两份映射。

首个实现切片已采用 `contracts/project-pages-1.0.json` 作为语言无关 Registry：Web 从它生成项目路由与 focus allowlist，Go `internal/projectview` 从嵌入的同一文件构造 URL。首版 focus 使用受控 query 参数，以复用开发中的 V3 `/projects/:projectID/:view` 路由；未来如果切换为对象级 path segment，必须先修改 Registry，再由两端共同消费。

## 5. Projection 导航目标

`ProjectProjection.next_actions` 增加类型化导航信息：

```json
{
  "kind": "decision",
  "subject_id": "conflict:package-dimensions",
  "label": "确认包装尺寸",
  "navigation": {
    "view": "knowledge",
    "focus": {
      "kind": "conflict",
      "id": "conflict:package-dimensions",
      "digest": "sha256:..."
    }
  }
}
```

Projection 返回类型化目标，不返回任意绝对 URL。Web 和 MCP 分别通过同一 Page Contract 构造适合当前宿主的链接。

### 5.1 Web 到 Codex 恢复契约

已实现两个受会话保护的 BFF 读取入口：

```text
GET /api/bff/projects/{projectID}/codex-handoff
GET /api/bff/projects/{projectID}/submission-revisions/{revisionID}/codex-handoff
```

统一响应：

```json
{
  "schema_version": "contentcloud.codex-handoff/1.0",
  "kind": "review_feedback",
  "project_id": "prj_01H...",
  "target": {
    "kind": "submission_revision",
    "id": "rev_01H...",
    "digest": "sha256:..."
  },
  "plugin_id": "contentcloud-video-production@contentcloud",
  "plugin_version": "0.6.0",
  "requires_new_chat": true,
  "requires_workspace_selection": true,
  "launch_url": "codex://new?prompt=...",
  "prompt": "...",
  "steps": ["..."],
  "fallback_url": "/codex"
}
```

约束：

1. Project 必须存在且至少已有一个 Workspace/Device 绑定；服务端仍不持有本机 Workspace 路径。
2. review feedback 入口先验证 Revision 属于 URL 中的 project，再要求存在审核评论或 `changes_requested` 状态；跨 project/tenant 统一返回 404。
3. `launch_url` 固定为 `codex://new?prompt=...`，只有一个 `prompt` query；禁止 `path`、`originUrl`、token、客户正文、评论正文和本机路径。
4. Prompt 固定引用 Plugin ID、project ID；feedback 额外包含 Revision ID 与完整 digest。它先调用 `workspace_context` 并验证 `project_id`，不匹配则停止。
5. feedback Prompt 只先调用 `review_feedback_list`；pull、claim、本地写入和新修订 Run 都需要用户后续明确要求。
6. Web 在打开自定义协议前再次校验 schema、Plugin 版本、new-chat/workspace gate、当前页面 project/revision/digest、query allowlist 和 Prompt 一致性。校验失败只显示错误，不导航。
7. Assignment handoff 尚未实现，等待 V3 W3-01 的 WorkAssignment 与 pull 契约，不能复用 project target 冒充精确 Assignment。

## 6. 既有 Tool 的链接复用

以下既有 Tool 已在业务成功后复用统一 route builder 附加 resource link：

| Tool | 页面与精确定位规则 |
| --- | --- |
| `workspace_status` | 项目总览 |
| `workspace_doctor` | 接入与初始化页；当前不伪造本地环境对象 focus |
| `publish_apply` | 新创建的 SubmissionRevision，并携带服务端返回的完整 content digest |
| `submission_status` | 能解析当前 Revision 与完整 digest 时精确定位，否则只打开审核页 |
| `review_feedback_pull` | 所有 bundle 唯一指向同一 Revision/digest 时精确定位；多目标或无目标时只打开审核页 |
| `approved_snapshot_pull` | 单 Snapshot 且 ID/digest 合法时精确定位；批量或空结果时只打开交付页 |

附加规则：

1. 业务结果和副作用先完成，页面链接随后以 best-effort 方式构造；WorkspaceBinding、可信 origin、Page Contract 或对象 ID/digest 校验失败时不附加链接，也不把已经成功的业务结果反转为失败。
2. `structuredContent` 保持原 Tool 的业务类型和字段，不包装成导航专用对象；标准 `resource_link` 只追加到 `content`。
3. 批量结果不选择任意一个对象制造虚假的精确上下文，而是返回对应 view 的通用链接。
4. 这些 Tool 不自行调用 Browser，也不附加声称 Browser 已打开的 `browserHandoff`；Skill 根据宿主能力决定是否打开并验证。
5. Assignment pull Tool 尚未存在，待 V3 Assignment 契约落地后再按相同规则接入，不能计入当前实现。

## 7. 认证与会话

内置 Browser 使用独立浏览器资料，不假设共享用户常用浏览器的登录态。

规则：

1. 普通项目视图使用 ContentCloud 正常会话认证。
2. 未登录时，SPA 只从当前同源位置生成相对 return path，并通过共享 Page Contract allowlist 规范化；登录和注册入口只携带该规范化结果。
3. 登录后重新执行 tenant/project/object 授权，不能只信任 return path。
4. MCP 不读取、写入或搬运 Browser cookie。
5. 内部项目视图不通过带 token 的 query string 绕过登录。
6. 客户公开审核链接继续使用独立 ReviewGrant 生命周期；本 Tool 首版不创建或回显 ReviewGrant secret。
7. PKCE bootstrap 授权与普通 Browser 导航是两条协议，不共享 verifier 或 session state。

首版 return path 只允许 `/`、`/team`、固定 Admin 页面和 `/projects/{projectID}/{registered route}`。项目 query 必须能解析为该 view 允许的唯一 focus；外部/协议相对 URL、反斜杠、fragment、未知页面、未知 query、重复字段、非法 ID 和非法 digest 全部回退到 `/`。return path 只负责恢复位置，不承载授权；目标页加载后仍由服务端重新验证 tenant、project、object 和 disclosure。

## 8. 授权边界

访问顺序：

```text
authenticated user
  -> tenant membership
  -> project membership
  -> view permission
  -> focus object belongs to project
  -> disclosure permission
  -> command-specific permission
```

错误响应不能通过 404/403 差异向跨租户用户泄露对象是否存在。敏感 Evidence 仍受 `metadata_only / evidence_pack / full_source` 披露级别控制。

当前 Web 展示层已经实现以下保守规则：

1. 页面打开后遇到 401，显示登录失效状态并通过 allowlisted return path 返回原 view/focus。
2. 项目或 focus 读取返回 403/404 时使用同一个 `PROJECT_TARGET_UNAVAILABLE` 页面状态，不回显服务端角色码、对象 ID、品牌或产品信息。
3. Revision 页面只汇总 `metadata_only / evidence_pack / full_source` 数量，不直接渲染 `evidence_pack` 正文；`evidence_limited=true` 或未知级别统一显示为受限。
4. Submission 校验拒绝 `metadata_only` 或 `full_source` 携带 `evidence_pack` 字段，防止依赖前端隐藏修复错误标记的披露数据。

这些规则完成 W3-04 的页面状态，不代表 V3 已经具备独立的 project membership 或 disclosure permission 授权模型；后者及真实 Browser 授权链仍由 W4-04 验收。

## 9. 写动作与确认

Browser 导航本身是只读动作。`readOnlyHint` 只描述 `contentcloud_open_project_view`，不把整个 Web 降级为只读站点。打开页面后：

- Fact 的 verify/reject/request evidence 使用类型化 Decision 命令。
- Claim 的 approve/prohibit/request changes 使用类型化 Decision 命令。
- Rights 的 validate/expire/reject/request proof 使用类型化 Decision 命令。
- publish 仍必须经过 preflight、同一 plan ID 和明确确认。
- 创建客户审核链接、Environment 变更、Automation 启用继续使用各自确认机制。

Agent 不能因为用户说“打开审核页”而自动执行批准。页面上的文字也不能被解释为安装 Plugin、扩大权限或执行本地命令的授权。用户在 Web 中执行允许的治理命令，与 Agent 导航或本地 publish 是三个独立动作和审计事件。

## 10. `/codex` 双模接入契约

官方接入入口：

```text
https://contentcloud.example.com/codex
```

同一 canonical URL 服务两类消费者：

| 请求 | 响应 |
| --- | --- |
| 浏览器页面导航（`Sec-Fetch-Dest: document` 且接受 `text/html`） | 人类可读的 Plugin 安装、登录、连接和故障恢复页面 |
| 非页面导航请求，或明确接受 `text/plain` | 版本化、无秘密、可执行的安装与 bootstrap 指南 |

约束：

1. 响应使用 `Vary: Accept, Sec-Fetch-Mode, Sec-Fetch-Dest`，不能用脆弱的 User-Agent allowlist 决定安全行为。
2. Agent 指南只引用官方 Plugin ID、固定 Marketplace 来源、稳定命令和官方域名。
3. 指南不包含 token、客户数据、Workspace 路径或生产凭据。
4. 指南先检查宿主是否有权修改本机 Plugin 配置；远程 Web/隔离宿主不得伪装本地安装成功。
5. 安装完成后创建新对话，使宿主重新加载 Plugin/MCP Tool inventory；安装对话不能假定动态获得新 Tool。
6. 新对话先运行 conversation context 与 bootstrap/doctor，再打开 setup 或 overview 精确页面。
7. 读取官方指南不等于用户授权危险操作；安装、登录、文件写入继续遵守宿主确认和权限策略。
8. Browser HTML 与 Agent plain text 表达同一版本和流程，不维护两套相互漂移的安装事实。

`/codex` 解决接入和交接，不参与业务数据同步，也不替代 Plugin manifest、签名验证、PKCE 或 WorkspaceBinding。

## 11. 安全测试

必须覆盖：

- 任意 URL/open redirect 输入被 Schema 拒绝。
- `../`、编码斜杠、控制字符和超长 ID 不影响路由边界。
- 跨 tenant/project focus 被拒绝且不泄露存在性。
- URL、日志、MCP content 和 telemetry 中无 token/credential/绝对路径。
- stale digest 页面不能把决定应用到新 Revision。
- Browser 页面中的 prompt injection 不能改变 Scene Plugin capability 或触发本地写入。
- 未披露 Evidence 不因深链而可见。
- Browser 返回按钮、重复刷新和多标签页不重复执行写命令。
- CSRF、SameSite、CSP、frame policy 和 download headers 符合 Web 安全基线。
- `/codex` HTML/plain-text 内容一致，缓存正确区分 `Accept`，且不包含秘密或任意第三方安装源。
- 远程或隔离宿主读取 `/codex` 时不会执行本地安装，也不会报告 Workspace 已连接。
