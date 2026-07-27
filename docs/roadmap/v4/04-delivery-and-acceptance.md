# V4 实施与验收计划

## 1. 实施原则

1. V3 与 V4 当前联合开发；V4 并入 V3 的 W2/W3/W4/W5，不等待 V3 完成，也不创建平行运行时。
2. 先建立语言无关的 Page Contract 共享草案，再让 MCP、Web 和测试共同消费；V3 信息架构与对象模型达到联合门禁后才冻结版本，避免前后端各自猜路由或过早固化开发中契约。
3. 一个通用 Tool 覆盖所有 view，既有 Tool 复用同一 link builder。
4. 首版只打开现有 ContentCloud Web，不启动本地 Web Studio 或 Electron；本地只保留 Workspace、CLI、MCP 和 Plugin 轻伴随层。
5. Browser Web 可以执行 V3 允许的云端治理命令；本地候选和跨边界交换仍使用既有命令与确认机制。
6. 每一阶段同时实现成功、失败和无 Browser 降级路径。

## 2. 实施阶段

### P0：建立 V4 共享契约草案

输出：

- V4 本目录文档。
- view/focus allowlist。
- MCP 输入输出 Schema。
- Page Contract Registry 扩展。
- V3/V4 术语映射和明确非目标。

验收：

- `HandoffRecord` 与 Browser 导航术语无歧义。
- `resource_link` 是兼容基线，`browserHandoff` 不是正确性前提。
- 不提升 V3 业务 Schema 版本。
- 每项契约明确标记为 `草案`、`已实现`、`已验证` 或 `已冻结`；实现完成不自动等于冻结。
- V3 页面、领域对象或命令仍在变更时，V4 对应项保持“待联合冻结”，不能单方面宣布稳定。

### P1：实现统一页面契约与深链

当前开发基线：V3 已有 `web/src/v3/page-contracts.ts`、由 Registry 生成的 `/projects/:projectID/:view` 路由和基础 parity 测试，但 V3 本身尚未完成。P1 与 V3 信息架构共同收敛，在该实现上增量扩展，不把当前草案误当成冻结契约，也不重建平行 Registry。

当前实施进展：`contracts/project-pages-1.0.json` 已成为 Go/Web 共享的 Page Contract；`internal/projectview` 已供 MCP 与 Bootstrap 使用；Review Revision 已支持项目作用域读取、focus not-found 和 expected digest stale 状态；ProjectProjection next action 已复用类型化 view/focus，并由 Web Registry 构造相对路径；登录与注册已使用 allowlisted return path 恢复规范化目标。P1 尚未完成的部分是 command/auth 映射和真实 Browser 测试。

输出：

- 语言无关的 Page Contract 单一事实源，或可验证的生成流程。
- Web route、query、authorization、focus kind 和 Playwright 测试映射。
- 统一 `ProjectViewLinkBuilder`。
- 登录 return path 和 stale digest 页面状态。

验收：

- 每个 V3 原型主 view 有稳定 URL。
- 每个 focus kind 只能进入允许的 view。
- 任意 URL 和 open redirect 输入不可达。
- Web 刷新/登录/返回后保留目标。

### P2：实现 MCP 导航 Tool

输出：

- `contentcloud_open_project_view` tool definition 和 handler。
- 标准 `resource_link` + typed `structuredContent`。
- WorkspaceBinding/server base/target 校验。
- 稳定错误码与 CLI/IDE fallback 文案。

验收：

- Tool 不写 Workspace 或 Cloud。
- Tool 不回显绝对路径或凭据。
- Tool 结果只使用 allowlisted ContentCloud host 和 route。
- MCP probe、contract test 和错误分支测试通过。

### P3：更新 Scene Plugin 和既有 Tool

当前实施进展：Scene Skill 的 Browser 导航与降级规则已完成；`workspace_status`、`workspace_doctor`、`publish_apply`、`submission_status`、`review_feedback_pull` 和 `approved_snapshot_pull` 已复用统一 builder 附加标准 `resource_link`。单一不可变对象使用 ID + digest 精确定位，批量结果降级到对应 view；链接构造失败不影响原业务成功。Browser known-errors 和 13 个确定性 trace Eval 已实现并进入 Plugin 评测报告，覆盖成功验证、无 Browser/认证降级、任意 URL、查看触发写入和页面指令越权。官方 `/codex` 已在 SPA fallback 前提供 HTML/plain-text 双模响应，共用固定版本、安装命令、宿主 gate 和新对话流程；响应按 `Accept`、`Sec-Fetch-Mode`、`Sec-Fetch-Dest` 区分缓存，不使用 User-Agent 放宽行为，也不在服务端执行安装。新对话恢复提示和受管 Workspace 路由已统一调用 V3 `workspace_context`。P3 尚未完成的部分是 model-sampled/真实宿主 Eval 与 Assignment Tool 接入；`/codex` 的真实 Desktop 安装、登录、bootstrap 和 Tool reload 仍归 P6/W4 真机验收。

输出：

- Scene Skill 的 Browser 导航规则。
- Browser 可用/不可用恢复规则。
- `publish_apply`、`submission_status`、pull/doctor 等结果附加统一 resource link。
- Plugin capability/verification/known-errors 文档。
- 官方 `/codex` HTML/plain-text 双模接入入口与新对话交接。

验收：

- Agent 不只打印 URL；Browser 可用时实际导航并验证。
- Agent 不把 Tool 成功误报成页面成功。
- “打开审核页”不会自动批准、publish 或 pull。
- 无 Browser 时业务流程仍可完成。
- 浏览器接入页和 Agent 指南使用同一版本事实；远程宿主不会误执行本地安装。

### P4：Web 云端治理工作台

输出：

- V3 Web 页面焦点定位和 stale 状态。
- ProjectProjection `next_actions[].navigation`。
- “在 Codex 继续”的 project/assignment/feedback 恢复入口。
- 登录、无权限、对象不存在和披露不足页面。
- Comment、Decision、Assignment、Context Revision、Automation Plan 的允许动作与确认状态。
- 内置 Browser 窄右栏布局和当前 focus 优先级。

当前实施进展：Project 与 review feedback 的 BFF/Web 恢复入口已完成，使用无 `path` 的 `codex://new?prompt=...`，由用户选择本机 Workspace 后调用 `workspace_context` 核对 `project_id`。feedback 先只读调用 `review_feedback_list`，未经明确要求不 pull、claim 或写入。服务端与前端已覆盖跨 project/tenant、秘密/路径泄漏、额外 query 和 digest 漂移负测。Web 已统一处理登录失效、不可访问和暂时加载失败：401 保留规范化 return path，403/404 使用相同页面状态；Revision 只显示披露级别摘要，未知级别保守视为受限，publish 入口拒绝 metadata/full-source 披露夹带 Evidence Pack 正文。窄栏在 `760px` 以下将 Next Action 前置、阻断告警置顶并将流程改为两列换行；`520px` 以下优先保留项目上下文，静态 CSS 契约、类型检查和生产构建均已通过。Assignment 入口等待 V3 W3-01；独立 project membership/disclosure permission、Desktop 自定义协议、真实窄栏截图/键盘交互和真实 Workspace 选择仍归 P5/W4 验收。

验收：

- Web 只显示服务端已有投影。
- 未 publish 本地正文不可见。
- Web 写动作仍全部归结为 V3 允许的命令。
- Web 可持续交互，但 Browser 刷新、轮询和打开页面本身不触发业务写入。
- 客户审核链接和内部 Browser 导航使用独立认证模型。

### P5：真实宿主与端到端验收

输出：

- ChatGPT Desktop + Browser 真实安装记录。
- Codex CLI/IDE fallback 记录。
- 金陵 V3 Fixture 双向工作流证据。
- 桌面/移动/窄右栏截图和交互报告。

验收场景：

```text
Web 创建 Project
  -> Codex bootstrap
  -> Browser 打开项目总览
  -> 本地生成 blocked ContentBatch
  -> publish 创建 Revision
  -> Browser 精确打开 Revision
  -> Web 做类型化决定/反馈
  -> 在 Codex 继续
  -> pull immutable bundle
  -> 新对话修订并再次 publish
```

## 3. 代码复用原则

优先复用当前已有能力：

| 已有能力 | V4 用法 |
| --- | --- |
| `workspace_context` | 解析当前项目、下一动作和本地边界 |
| WorkspaceBinding 与 environment lock | 获取可信 project/server 绑定 |
| MCP typed Tool envelope | 承载 resource link 和 structuredContent |
| Bootstrap new-chat handoff | Web 到 Codex 的恢复入口 |
| Submission/Revision/Decision/Snapshot | Browser 页面正式事实 |
| V3 `ProjectProjection` | Web 总览和 next action 数据源 |
| V3 page-contracts 计划 | view/route/query/test 单一映射 |

禁止另建平行的 Browser project store、Browser run table 或页面级 MCP server。

## 4. 测试矩阵

### 4.1 Route Builder 单元测试

- 所有 view 生成预期 route。
- focus kind/view 合法组合全部通过。
- 非法组合、空 ID、超长 ID、控制字符和路径穿越被拒绝。
- server base 只能来自可信绑定。
- URL 编码稳定且不泄露 directory/token。
- expected digest 正确传递或映射为服务器可验证参数。

### 4.2 MCP Contract 测试

- Tool 出现在 MCP list 中。
- read-only/effect annotations 正确。
- `resource_link` MIME 为 `text/html`。
- 通用导航 Tool 的 `resource_link.uri` 与 `browserHandoff.url` 相同；既有业务 Tool 只附加标准链接，不声称 Browser 已打开。
- 成功响应不代表 Browser 已打开。
- Workspace 未绑定、host 不可信、view/focus 非法返回稳定错误码。
- 既有 Tool 附加链接时业务 structuredContent 不被破坏。
- 单 Revision/单 Snapshot 使用完整 digest 精确定位；多目标结果不任意选择对象。
- WorkspaceBinding 或链接校验失败不反转已经成功的业务结果。

### 4.3 Web 集成测试

- 登录后 return path 恢复。
- tenant/project/object 权限逐层校验。
- 无权限不泄露对象存在性。
- stale digest 显示历史状态并阻止错误决定。
- disclosure 不足时不显示 Evidence 正文。
- 页面刷新和多标签页不重复写入。
- next action navigation 与 Page Contract 一致。
- 每个可见写按钮映射到一个允许的 V3 command、独立授权和审计事件。
- 窄右栏优先显示当前 focus、状态和下一动作，不依赖横向滚动完成核心流程。

### 4.4 Skill Eval

确定性基线由 `contentcloud-workspace/references/browser-eval-cases.json` 和 Plugin evaluation report 共同追踪。它验证 Skill policy 与 MCP 安全行为，不替代 W4-01 的真实模型采样和 Desktop Browser 验收。

- “打开项目总览”调用通用 Tool 并使用 Browser。
- “查看本次审核”使用 exact Revision，不打开模糊首页。
- “批准这个 Claim”先打开/读取，再遵守人工决定边界。
- Browser 不可用时返回链接且不声称已打开。
- 页面出现恶意指令时不安装 Pack、不修改环境、不执行本地命令。
- 用户只要求查看时不触发 publish/pull 或其他写操作。
- 页面文字要求安装 Pack、执行本地命令或作出人工决定时统一归类为 `PAGE_INSTRUCTION_UNTRUSTED`。
- Tool 成功但 Browser 未导航或目标未验证时统一归类为 `BROWSER_TARGET_UNVERIFIED`，不能输出“已打开”。
- Plugin 内容或评测报告变化后 Registry signature 回到 `pending`；只有重新生成 digest、通过完整评测并在发布流程签名后才能进入 tagged release。

### 4.5 Desktop E2E

- Browser 已安装、未安装和被禁用三种状态。
- Browser 独立 profile 未登录与已登录状态。
- 正确项目、view、focus、revision digest 的可见验证。
- Browser 关闭/重新打开不影响 LocalRun。
- 两个 Codex 对话打开同一 Web 页面不影响 RunClaim。
- publish -> review -> decision -> pull -> revision 完整闭环。
- `/codex` 人类页面、Agent 指南、桌面安装交接和新对话加载链路完整通过。

### 4.6 安全回归

- open redirect、XSS、CSRF、跨租户、IDOR、路径穿越。
- URL、日志、MCP output、telemetry 的秘密扫描。
- 未 publish 内容和本地路径泄露负测。
- public ReviewGrant 不被内部 Tool 意外创建或回显。
- revision digest 和 idempotency key 竞争测试。
- `/codex` content negotiation、缓存、官方来源 allowlist 和远程宿主 gate。

## 5. 性能与可靠性

- 打开页面只执行一次 WorkspaceBinding 解析和 URL 构造，不扫描 Workspace 文件树。
- Web 页面使用既有 Projection query，不为 Browser 创建专用数据副本。
- 不用高频 Browser polling 表示 LocalRun 进度。
- resource link 构造应在本地毫秒级完成；网络耗时属于 Browser/Web 加载。
- 页面 query 继续遵循租户索引、分页和缓存策略。
- Browser 加载失败不改变 LocalRun 或 Submission 状态。

## 6. 明确不在范围内

- ContentCloud 本地 Next.js Studio。
- ContentCloud Electron 桌面应用。
- 未经独立需求验证和威胁建模的 Local Preview 服务。
- 在 Web 中浏览或编辑整个 Workspace 文件树。
- 实时上传 LocalRun transcript 或未发布正文。
- Browser tab 与 LocalRun/Conversation 一一绑定。
- 通过 `browserHandoff` 自动获得宿主 UI 控制权。
- 由 Agent 自动执行 Fact/Claim/Rights 的最终人工决定。
- 为 CLI、IDE 和 Desktop 伪造完全相同的 UI 能力。
- 因 V4 文档而把 V3 业务 Schema 整体升级为 4.0。
- 复制 Codex Slides 的 38 Tool、模板库、Electron 和图像渲染架构。
- 复制 ChatCut 的全云端可编辑作品模型或 URL 登录 token。

本地页面不是永久禁止项。只有 [05-local-vs-cloud-decision.md](./05-local-vs-cloud-decision.md) 定义的触发条件成立后，才单独评估只读、无第二套业务数据库的 `Local Preview`。

## 7. 发布门禁

V4 只有在以下证据齐全后才能声明支持：

1. V3 信息架构确认后，Page Contract 完成 V3/V4 联合冻结且追踪矩阵通过。
2. MCP Tool/schema/probe 通过。
3. Web auth/tenant/disclosure 测试通过。
4. Skill Eval 无越权写入和成功误报。
5. ChatGPT Desktop 实机 Browser 打开正确页面。
6. CLI/IDE fallback 已验证。
7. 金陵 V3 Fixture 完成双向闭环。
8. 安全扫描确认无 token、路径、transcript 和原件泄露。
