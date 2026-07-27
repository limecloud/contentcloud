# ContentCloud V4 实施跟踪计划

状态：`核心决策已确认，进入 V3/V4 联合实施`。

更新时间：2026-07-27。

架构入口：[README.md](./README.md)。V4 继承的业务基线：[../v3/README.md](../v3/README.md)。

## 0. 当前进展

| 范围 | 状态 | 说明 |
| --- | --- | --- |
| V4 问题定义 | 已完成 | 识别 V3 缺少 Codex 到 Web 的精确导航闭环 |
| Codex Slides benchmark | 已完成 | 已审计 Browser-first、resource link、Skill 和 durable run 模式 |
| ChatCut benchmark | 已完成 | 已审计远程 MCP、云端编辑器、Browser handoff、本地导入和双模接入入口 |
| V4 架构 | 已完成 | Browser 定位为可操作的云端治理工作台，不成为新数据平面 |
| V4 契约与安全 | 已完成 | Tool、route、auth、effect 和 disclosure 边界已定义 |
| 本地与云端拓扑决策 | 已完成 | 学习 Slides 的 Browser-first，不复制其本地项目服务；首版采用云端治理 Web + 本地轻伴随层 |
| V4 Browser 交互原型 | 已完成 | 单文件原型覆盖桌面、移动、发布、登录恢复、stale Revision 和 Browser 降级 |
| V3/V4 联合实施 | 九个切片已落地 | 语言无关 Page Contract、Go LinkBuilder、导航 MCP、Review focus/stale BFF/Web、Bootstrap 链接、类型化 Projection navigation、allowlisted 登录恢复、既有 Tool 统一页面链接、Browser Skill 确定性安全 Eval、`/codex` 双模指南、Web Project/feedback handoff、统一页面/披露状态与窄栏优先级布局已实现；V3 其余领域仍在开发 |
| V4 真实宿主验收 | 0% | 尚未在 ChatGPT Desktop Browser 完成端到端证据 |

## 1. 已确认事实

| ID | 事实 | 证据 |
| --- | --- | --- |
| F4-01 | V3 已设计 Web 到 Codex 的连接 Prompt 和“在 Codex 继续” | V3 prototype / business workflows |
| F4-02 | V3 尚未定义 Codex 到 Web 的通用精确导航 Tool | 当前 MCP Tool inventory |
| F4-03 | MCP 可以返回标准 `resource_link` | MCP SDK 与 Codex Slides 实现 |
| F4-04 | `browserHandoff` 是应用自定义提示，不是宿主打开面板的标准 API | Codex Slides 静态审计 |
| F4-05 | Browser 面板由 ChatGPT Desktop/Web 宿主提供 | OpenAI Browser 文档 |
| F4-06 | Codex CLI/IDE 当前不能依赖内置 Browser | OpenAI Browser 文档 |
| F4-07 | 内置 Browser 使用独立浏览器 profile | OpenAI Browser 文档 |
| F4-08 | V3 Web 只能显示服务端投影，不能读取未 publish 的本地正文 | V3 architecture/web contracts |
| F4-09 | ChatCut 使用远程 MCP OAuth 和云端编辑器，不为 Codex 启动完整 localhost 编辑器 | 公开 Plugin `.mcp.json`、README 与基础 Skill |
| F4-10 | ChatCut 本地辅助层只负责媒体准备/上传，云端 Project/Timeline 仍是可编辑事实 | 公开 asset-import 与 verification Skill |
| F4-11 | ChatCut `/chatgpt` 同时服务人类安装页和 Agent 安装指南 | 公开 HTTP 响应与安装页 |

## 2. V4 决策

| ID | 决策 | 状态 |
| --- | --- | --- |
| D4-01 | V4 继承 V3 全部业务不变量 | 已确认 |
| D4-02 | Browser 打开可操作的云端治理工作台 | 已确认 |
| D4-03 | 使用一个通用 `contentcloud_open_project_view` MCP Tool | 已确认 |
| D4-04 | MCP 返回标准 `resource_link`；`browserHandoff` 只作可选提示 | 已确认 |
| D4-05 | MCP Server 不直接启动或控制 Browser | 已确认 |
| D4-06 | 页面定位使用 project/object/revision ID 与 digest | 已确认 |
| D4-07 | Web 写动作仍只能生成 Assignment、Comment、Decision、Context Revision 或 Automation Plan | 已确认 |
| D4-08 | CLI/IDE 无 Browser 时返回可点击链接并明确降级 | 已确认 |
| D4-09 | 首版不引入 ContentCloud 本地 Next.js 服务或 Electron 壳 | 已确认 |
| D4-10 | V4 文档版本不触发业务 Schema 4.0 | 已确认 |
| D4-11 | 提供官方 `/codex` 人类/Agent 双模接入入口 | 已确认 |

## 3. 工作流

### W0 V4 方案

| ID | 任务 | 状态 | 验收证据 |
| --- | --- | --- | --- |
| W0-01 | 审计 Codex Slides Browser-first 实现 | 已完成 | MCP/Skill/resource link/Browser 边界分析 |
| W0-02 | 对照 V3 Workspace、Server、Web 和 Contract | 已完成 | V4 本目录文档 |
| W0-03 | 完成本地与云端部署拓扑分析 | 已完成 | `05-local-vs-cloud-decision.md` |
| W0-04 | 完成 Browser 双栏交互原型 | 已完成 | `prototype.html`；桌面、移动和异常状态交互 |
| W0-05 | 审计 ChatCut 远程 MCP、云端编辑器与 Agent 接入 | 已完成 | `06-chatcut-benchmark.md` |
| W0-06 | 确认 V4 决策 D4-01 至 D4-11 | 已完成 | 用户确认混合架构与 ChatCut 式产品体验 |

### W1 页面与导航契约

| ID | 任务 | 状态 | 验收标准 |
| --- | --- | --- | --- |
| W1-01 | 与 V3 联合冻结 view/focus allowlist | 已实现，待冻结 | `contracts/project-pages-1.0.json` 覆盖 12 个 V3 view 与 focus；V3 信息架构完成前继续联合评审 |
| W1-02 | 联合扩展 Page Contract Registry | 进行中 | Web route/section/submission/snapshot/focus 已消费共享 Registry；command/auth/browser test 尚待补齐 |
| W1-03 | 实现统一 ProjectViewLinkBuilder | 已完成 | Go builder 已供 MCP 和 Bootstrap 复用；Projection next action 复用同一 Target 类型和 Page Contract 校验但不生成 URL |
| W1-04 | 实现 auth return/stale/not-found 状态 | 已实现，待真机 | Review Revision 项目作用域读取、digest stale/not-found 和 allowlisted 登录/注册 return path 已实现；真实 Browser 登录仍归 W4-01 验收 |

### W2 MCP 与 Plugin

| ID | 任务 | 状态 | 验收标准 |
| --- | --- | --- | --- |
| W2-01 | 增加 `contentcloud_open_project_view` | 已完成 | 返回标准 `resource_link`、typed `structuredContent` 和可选 Browser handoff |
| W2-02 | 接入 WorkspaceBinding 与可信 host | 已完成 | 严格参数解码；拒绝任意 URL/host、非 origin、远程 HTTP、路径和无效 focus |
| W2-03 | 更新 Scene Skill Browser 规则 | 已完成 | Tool 成功与 Browser 成功分离；验证、失败、无 Browser 降级和页面不可信边界已写入 |
| W2-04 | 既有 Tool 附加统一页面链接 | 已完成 | `workspace_status`、`workspace_doctor`、publish/status/pull 复用同一 builder；精确单对象携带 digest，批量结果不任意选中对象，链接失败不反转业务成功 |
| W2-05 | 增加 Skill Eval 与 known-errors | 已完成确定性验证，待重新评测/真机 | 13 个 Browser trace 覆盖只查看不写、Tool/Browser 成功分离、页面注入、任意 URL 和无 Browser 降级；既有报告曾通过 9 个场景，但当前 `0.7.0` Plugin 内容已在报告后变化，`pnpm check:plugin` 正确报告 Plugin/报告/Registry digest 漂移和 pending 签名，正式发布前必须重新评测并重签；model-sampled/真实宿主仍归 W4 |
| W2-06 | 实现官方 `/codex` 双模接入与新对话交接 | 已实现，待真机 | `Sec-Fetch-Dest` + `Accept` 协商 HTML/plain text，共用固定版本/命令模型；无 User-Agent 放宽、不执行服务端安装，新对话恢复已统一到 V3 `workspace_context`，真实 Desktop 安装与 Tool reload 归 W4/W5 验收 |

### W3 Web 云端治理工作台

| ID | 任务 | 状态 | 验收标准 |
| --- | --- | --- | --- |
| W3-01 | 实现稳定 view/focus 深链 | 已实现，待真机 | 直接打开、刷新和登录/注册返回均保留 Registry 校验后的 view/focus；真实 Browser 登录仍待验收 |
| W3-02 | 扩展 ProjectProjection navigation target | 已完成 | next action 返回 Page Contract 校验的类型化 view/focus；Web 生成相对路径，不接收任意 URL |
| W3-03 | 实现 Web 到 Codex 恢复入口 | 部分实现，待真机 | Project 与 review feedback 已使用 `codex://new?prompt=...` 安全引用、Workspace/project gate 和前端 runtime 校验；Assignment 等待 V3 W3-01，真实 Desktop 打开待 W4 |
| W3-04 | 实现 stale/forbidden/disclosure 状态 | 已实现，待真机 | 401 保留 return path 恢复登录；403/404 使用不可枚举的统一页面状态；Review 显示 Revision 披露摘要且不渲染 Evidence 正文，未知披露级别按受限处理；真实 Browser 仍待 W4 |
| W3-05 | 浏览器、响应式和窄右栏验收 | 响应式实现完成，待真机 | `760px` 以下总览和领域页将 Next Action 前置（存在阻断时 Alert 优先），流程改为两列换行而非核心横向滚动；`520px` 以下优先保留项目上下文。CSS 契约测试、类型检查和生产构建通过；真实 Browser 截图、键盘和交互仍归 W4-01 |
| W3-06 | 实现云端治理工作台允许动作 | 待实施 | Comment/Decision/Assignment/Context Revision/Automation Plan 映射到 V3 command |

### W4 端到端与安全

| ID | 任务 | 状态 | 验收标准 |
| --- | --- | --- | --- |
| W4-01 | ChatGPT Desktop Browser 实机 | 待实施 | open/navigate/verify 完整通过 |
| W4-02 | CLI/IDE fallback | 待实施 | 返回链接且不声称打开 |
| W4-03 | 金陵 Fixture 双向闭环 | 待实施 | bootstrap 到 review feedback revision |
| W4-04 | auth/tenant/disclosure 安全测试 | 待实施 | 无 IDOR、open redirect 和数据泄露 |
| W4-05 | Prompt injection 与权限负测 | 待实施 | 页面内容不能扩大本地能力 |

## 4. 实施顺序

```text
V3 W1/W2/W3 开发中业务契约
  -> V4 W1 Page Contract + Link Builder
  -> V4 W2 MCP + Scene Skill
  -> V3/V4 Web routes + Projection navigation
  -> V4 W4 Desktop/fallback/security
```

V4 不应等待 V3 全部完成后再补。Page Contract、MCP 返回和真实宿主验收应分别并入 V3 的 W2、W4 和 W5，以免 Web 路由与 Agent 入口再次脱节。

V3 仍在开发，因此这里的“并入”是双向约束：V3 改动领域对象或页面信息架构时要同步验证 V4 导航；V4 新增 focus 或 Browser 工作流时也必须先确认 V3 是否已有对应的正式对象、Projection 和命令。任何一侧都不能复制一份契约后独立演进。

## 5. 测试追踪

| 范围 | 单元 | 集成 | E2E/Eval |
| --- | --- | --- | --- |
| Route builder | view/focus/encoding | Page Contract parity | Web deep-link Playwright |
| MCP Tool | Schema/envelope/error code | WorkspaceBinding/host | MCP probe |
| Scene Skill | routing fixtures | tool result/fallback | Skill Eval + Browser |
| Web auth | return path/permission | tenant/project/disclosure | Desktop Browser login |
| Revision | digest/stale | Decision binding | publish-review-pull |
| Security | URL/path/secret | IDOR/CSRF/CSP | prompt injection |

## 6. 风险与控制

| 风险 | 控制 |
| --- | --- |
| 把 Browser 误当同步机制 | 所有跨边界数据仍只走 publish/pull |
| Tool 数量膨胀 | 一个通用 Tool + 一个 link builder |
| 路由映射重复 | Page Contract 单一事实源或代码生成 |
| Browser 登录态与主浏览器不同 | 正常登录 + allowlisted return path |
| Agent 自动执行人工决定 | 打开与写入分离；Skill Eval 和权限门禁 |
| 深链泄露敏感数据 | URL 只含稳定 ID；无 token/path/transcript |
| Desktop 能力被过度宣称 | 真实安装实测 + CLI/IDE 明确降级 |
| V4 造成 V3 范围失控 | 不新增业务聚合、Schema 或本地应用 |
| Web 被实现成被动仪表盘 | 窄右栏工作台验收；允许治理命令、当前 focus 和下一动作必须可操作 |
| 双模安装指南漂移或越权 | `/codex` 共用版本事实、官方来源 allowlist、宿主 gate 和安全测试 |

## 7. 变更记录

| 日期 | 变更 |
| --- | --- |
| 2026-07-27 | 审计 Codex Slides 的 Browser-first、MCP resource link 和 Skill 编排模式 |
| 2026-07-27 | 确认 Browser 应作为 V3 云端治理表面，而不是新数据平面 |
| 2026-07-27 | 建立 V4 架构、导航契约、安全边界、双向流程和实施计划 |
| 2026-07-27 | 对比 Slides 本地服务与 ContentCloud 云端治理职责，形成首版部署拓扑建议 |
| 2026-07-27 | 审计 ChatCut 远程 MCP、云端编辑器、本地媒体伴随层和 `/chatgpt` 双模入口 |
| 2026-07-27 | 确认混合架构，并将 Browser 从治理视图明确升级为可操作的云端治理工作台 |
| 2026-07-27 | 对齐现有 V3 Page Contract：采用 `intelligence` view，环境诊断收敛到 `setup` focus，并记录 W1 已有实现基线 |
| 2026-07-27 | 明确 V3 仍在开发：现有 Page Contract 仅为联合收敛基线，V3/V4 变更必须双向校验后再冻结 |
| 2026-07-27 | 落地首个 V4 纵向切片：共享 Page Contract、ProjectViewLinkBuilder、导航 MCP、Review Revision stale 状态和 Bootstrap setup 深链 |
| 2026-07-27 | 落地第二个 V4 纵向切片：ProjectProjection 输出类型化 navigation，待审动作绑定 Revision digest，Web 通过共享 Page Contract 构造相对路径并拒绝未知目标 |
| 2026-07-27 | 落地第三个 V4 纵向切片：登录与注册使用 allowlisted 相对 return path，规范化项目 view/focus，并拒绝外部、未知、重复和畸形导航输入 |
| 2026-07-27 | 落地第四个 V4 纵向切片：既有 MCP Tool 统一附加 Page Contract `resource_link`；单 Revision/Snapshot 精确定位，批量 pull 降级到对应页面，链接构造失败不改变业务结果 |
| 2026-07-27 | 落地第五个 V4 纵向切片：Workspace Skill 增加 Browser known-errors 与 13 个确定性安全场景；Plugin 评测升级到 V3 Content 契约并将未重新签名的开发态 Registry 明确标为 pending |
| 2026-07-27 | 落地第六个 V4 纵向切片：`/codex` 以同一固定 Plugin/版本模型提供 HTML 与 plain-text 接入指南，新对话恢复统一到 `workspace_context` |
| 2026-07-27 | 落地第七个 V4 纵向切片：Web Project/feedback handoff 使用无 `path` 的 canonical `codex://new?prompt=...`；BFF 验证 Workspace 绑定与 Revision 归属，前端拒绝协议、query、ID、digest 和 gate 漂移；Assignment 明确保留待 V3 W3-01 |
| 2026-07-27 | 落地第八个 V4 纵向切片：Web 统一登录失效、不可访问和加载失败状态，403/404 不区分对象存在性；Revision 只展示披露级别计数，V3 publish 拒绝 `metadata_only/full_source` 夹带 Evidence Pack 正文；完整 project membership/disclosure permission 仍归 W4-04 |
| 2026-07-27 | 落地第九个 V4 纵向切片：窄栏下将 Next Action、阻断状态和项目上下文置于优先位置；项目流程改为换行网格，最窄宽度隐藏非核心新建入口。静态 CSS 契约、类型检查和生产构建通过，真实 Browser 验收待 W4 |
