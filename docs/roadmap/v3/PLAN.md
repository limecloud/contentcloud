# ContentCloud V3 实施跟踪计划

状态：`V3 主链已落地，正在清除旧事实源并补真实宿主验收`。

更新时间：2026-07-27。

架构入口：[README.md](./README.md)。界面原型：[prototype.html](./prototype.html)。

## 0. 当前进展

必须把“已有插件基础设施”和“V3 业务产品”分开计算：

| 范围 | 当前完成度 | 说明 |
| --- | --- | --- |
| V3 方案与原型 | 100% | 客户端目录、服务端领域、业务时序、Web、契约、实施计划和原型已形成 |
| 可复用 Plugin 基础设施 | 92% | Bootstrap、Environment、RunClaim、Handoff、Publish/Pull 已接入 V3；本地 V3 Marketplace 已在真实 Codex 新会话加载，正式发布安装与 Desktop 新会话待验收 |
| V3 客户端实现 | 88% | V3 Workspace、Source、Knowledge、Brief、ContentBatch/ContentItem、Run/Handoff、Lint、交付导出和完整脱敏 Fixture 物化已实现 |
| V3 服务端实现 | 90% | Submission/Decision/ApprovedSnapshot、Fixture importer、Projection、客户审批、结果学习、Artifact 和单一 PostgreSQL V3 基线已收敛；WorkAssignment 与类型化资格投影待补齐 |
| V3 Web 实现 | 65% | 12 个项目视图、Projection、focus deep link、审核与初始化入口已落地；各业务域的专用视图和浏览器验收未完成 |
| V3 端到端验收 | 65% | PostgreSQL 空库、真实 CLI Fixture 子进程和新 Codex CLI 会话的 `workspace_context` 已通过；云端 bootstrap 至 learning 串联和 Desktop 双对话仍未验收 |

V3 产品总完成度按实现、治理收口和真实宿主验收加权计算为 `约 80%`。代码、数据库、本地 Fixture 和 Codex CLI 开发态宿主主链已成立，但不能把本地 Marketplace 通过等同于正式发布、Codex Desktop 和客户环境可交付。

真实安装在 `0.6.0` 阶段揭示了发布事实不一致：Git `v0.6.0` 仍包含旧 V2 Plugin，npm 仅发布 `@limecloud/contentcloud@0.2.0` 与 `0.5.0`，因此已经安装的正式 `contentcloud-video-production@contentcloud` 无法启动 MCP。当前工作树已统一推进为 `0.7.0` 发布候选，Registry 对当前 Plugin digest 的签名验证已通过，但远端 `v0.7.0` tag/npm 包尚不存在，`--tagged` 门禁仍失败；开发态验收使用独立的 `contentcloud-dev` 本地 Marketplace 和本地编译 CLI，不覆盖正式安装，也不能替代发布验收。

当前工作区包含 V3 大规模重构以及并行开发中的 `docs/roadmap/v4/`。用户已明确授权在 V3 范围内清理和重构旧实现；V3/V4 文档作为联合实施计划同步维护，不创建 Git 分支，不 commit/push，也不借 V4 新建平行业务运行时。

## 1. 已确认事实

| ID | 事实 | 证据 |
| --- | --- | --- |
| F-01 | 真实客户端是方法论、本体、Wiki、RunContext 和输出门禁组成的系统 | `marketing/jinling-gudu/docs/architecture.md` |
| F-02 | 当前 `jinling-gudu` 是旧目录，只能提炼职责，不能复制为 V3 模板 | 用户明确说明 + V3 设计 |
| F-03 | V2 原型定义了九域工作台，但当前 Web 没有实现 | `v2/prototype.html` 对比 `web/src/components/Layout.tsx` |
| F-04 | V2 状态文档把九域与四层上下文标为 planned | `v2/13-acceptance-and-traceability.md` |
| F-05 | `.codex/config.toml` 是 Codex 项目配置，不是第三方业务状态目录 | Codex 官方配置手册 |
| F-06 | Repo Skills 使用 `.agents/skills`，Plugin 使用 `.codex-plugin` + `skills/` | Codex 官方 Skill/Plugin 手册 |
| F-07 | 默认 workspace-write 可能把 `.codex`/`.agents` 递归保护为只读 | Codex 官方安全手册 |
| F-08 | `.contentcloud` 应保存跨 Harness 的可变业务状态 | V3 Harness 解耦和可写性要求 |

## 2. V3 决策基线

| ID | 决策 | 状态 |
| --- | --- | --- |
| D-01 | 一个 Project 对应一个本地 Workspace，多个对话共享文件与业务状态 | 已确认 |
| D-02 | 使用 V3 数字业务目录；数字只服务人类导航，业务由 manifest/ID/digest 识别 | 已确认 |
| D-03 | `.codex` 只保存 Codex Adapter 配置；`.contentcloud` 保存 ContentCloud 可变状态 | 已确认 |
| D-04 | Knowledge Markdown 是唯一可编辑实例源，import/index 是投影 | 已确认 |
| D-05 | 服务端正式事实统一为 Revision/Decision/Snapshot，业务对象表是只读投影 | 已确认 |
| D-06 | Web 沿用 V2 原型视觉和九域导航，新增“方法论与上下文” | 已确认 |
| D-07 | 当前 Web 直接创建业务正文的路径全部删除，由 Assignment/Decision/Submission 代替 | 已确认 |
| D-08 | 开发期不做 V1/V2 兼容、双写或历史数据迁移 | 用户已明确确认 |
| D-09 | 三 TXT demo seed 删除，改为脱敏 V3 Fixture importer | 已确认 |

## 3. 工作流

### W0 V3 事实、方案与原型

| ID | 任务 | 状态 | 验收证据 |
| --- | --- | --- | --- |
| W0-01 | 完整读取真实客户端架构和必读上下文 | 已完成 | AGENTS、architecture、ontology、focus、workflows、source registry、run、output、service 样本 |
| W0-02 | 核对 Codex 官方目录与最佳实践 | 已完成 | 官方 manual：AGENTS、config、skills、plugins、protected paths |
| W0-03 | 建立 V3 客户端/服务端/Web 统一方案 | 已完成 | `README.md` 至 `07-delivery-plan.md` |
| W0-04 | 建立 V3 可交互原型 | 已完成 | `prototype.html` |
| W0-05 | 用户确认 V3 决策基线 | 已完成 | 2026-07-27 用户明确授权按 V3 清理重构、不保留兼容层 |

### W1 V3 Workspace Template

| ID | 任务 | 状态 | 验收标准 |
| --- | --- | --- | --- |
| W1-01 | 定义 JSON Schema/YAML Schema | 已完成 | Workspace、Source、Knowledge、Pack、Run、Handoff、Brief、ContentBatch/ContentItem 已有 V3 Schema 与测试 |
| W1-02 | 实现 `workspace_marketing_agent@3.0.0` | 已完成 | 空目录 bootstrap、模板锁、Workspace doctor 和 V3 数字目录已实现 |
| W1-03 | 实现 V3 AGENTS/Workflow/Lint | 已完成 | Source/Knowledge/Brief/ContentItem/Batch 的确定性门禁已由本地工具测试覆盖 |
| W1-04 | 实现 V3 RunClaim/Handoff 接入 | 已完成 | claim/renew/release、CAS、超时接管、handoff 创建/接受/完成测试通过 |
| W1-05 | 创建金陵 V3 Fixture Workspace | 已完成 | `fixtures/v3/jinling-gudu.json` 可重复生成 20 来源、15 维、七层 Pack、五类双状态治理对象、completed LocalRun 和 blocked 十条内容；同一数据包通过服务端幂等导入测试 |

### W2 V3 CLI/MCP/Plugin

| ID | 任务 | 状态 | 验收标准 |
| --- | --- | --- | --- |
| W2-01 | 审计已有 Bootstrap/Environment/Publish 复用边界 | 已完成 | V3 入口直接替换旧命令，没有新增兼容 wrapper |
| W2-02 | 重建 conversation context 和本地 Tool | 已完成 | 新对话可离线读取 Workspace、RunClaim、Handoff、知识资格和下一动作 |
| W2-03 | 重建 ingest/query/content batch 工具 | 已完成 | Source ingest、Knowledge query/diagnose/pack、Brief、ContentBatch/ContentItem 本地闭环已实现 |
| W2-04 | 重建 V3 publish/pull | 已完成 | manifest 精确范围、七种 Submission、幂等、披露、feedback/decision/snapshot 不可变 pull 已实现 |
| W2-05 | 更新 Scene Plugin Skills | 已完成 | Workspace、Knowledge、Content Skill 已引用 V3 Tool 和 ContentItem Schema，旧 ScriptPackageV2 引用已删除；V4 Browser known-errors 与确定性 Eval 已通过，新对话恢复提示和受管路由已统一到 `workspace_context`；真实 Codex CLI 开发态新会话已调用 MCP 成功，Page Contract 与正式发布宿主仍待联合验收 |

### W3 V3 Server

| ID | 任务 | 状态 | 验收标准 |
| --- | --- | --- | --- |
| W3-01 | 建立 ProjectContext/Intent/Assignment | 待实施 | Context 可版本化下发，Assignment 绑定 Snapshot/Bundle |
| W3-02 | 重建 Submission 类型 | 已完成 | `context/knowledge/brief/content_batch/asset_batch/delivery/result` 共用不可变 Revision 审核轨 |
| W3-03 | 实现类型化 Decision/资格 | 进行中 | Submission 两阶段 Decision 已完成；Fact/Claim/Rights 的独立资格投影仍需补齐 |
| W3-04 | 实现 Projection builder | 已完成 | ProjectProjection 从 Submission/Snapshot/Delivery/Result 构建 12 视图状态、治理计数和摘要绑定的 next action |
| W3-05 | 删除直接编辑和 demo seed | 已完成 | Web 旧实体路由、硬编码 seed、旧 Strategy/Brief/Script 云端直写链、Artifact 旧旁路、ScriptVersion 平行事实源和累计旧迁移均已删除 |
| W3-06 | 建立 V3 Fixture importer | 已完成 | handler 只解析外部 Fixture；服务幂等导入并生成 Submission、Decision、Snapshot 和 Projection，不硬编码客户正文 |

### W4 V3 Web

| ID | 任务 | 状态 | 验收标准 |
| --- | --- | --- | --- |
| W4-01 | 建立原型追踪契约 | 已完成 | Registry 覆盖 12 个 view、route、section、focus kind 和 digest deep link，并有契约测试 |
| W4-02 | 重建导航和布局 | 已完成 | WorkspaceShell 与 V3 ProjectPage 已按原型信息架构接管项目工作台 |
| W4-03 | 实现接入/总览/方法论/知识 | 进行中 | 初始化和总览使用真实 Bootstrap/Projection；方法论与知识仍以通用域视图为主 |
| W4-04 | 实现情报/策略/策划/创意 | 进行中 | Revision/Snapshot/blocked 已分层，域专用信息密度和动作仍需补齐 |
| W4-05 | 实现审核/交付/学习/Automation | 进行中 | 审核 deep link、Project/feedback Codex 恢复入口、不可枚举页面状态和 Revision 披露摘要已接入；Assignment 和 Automation 专用交互未完成 |
| W4-06 | 浏览器、响应式和可访问性验收 | 响应式实现完成，待真机 | 窄栏下 Next Action/阻断状态优先、项目流程不再依赖核心横向滚动，CSS 契约测试、Vitest、类型检查和生产构建通过；Playwright 截图、键盘和真实移动视口尚无验收证据 |

### W5 端到端与真实宿主

| ID | 任务 | 状态 | 验收标准 |
| --- | --- | --- | --- |
| W5-01 | CLI 端到端 Fixture | 进行中 | 真实 `contentcloud workspace fixture apply` 子进程已完成空目录到 blocked 十条批次；带认证的云端 bootstrap/publish/approval/delivery/learning 串联仍待验收 |
| W5-02 | Codex Desktop 双对话 | 进行中 | `/codex` 双模指南和 Web Project/feedback 新对话恢复契约已实现；真实 Codex CLI 新会话已加载 `contentcloud-dev:contentcloud-workspace` 并调用 `contentcloud-local-dev.workspace_context`；Desktop 的 Workspace 选择、handoff 和 feedback 修订仍需双对话通过 |
| W5-03 | Environment/Pack 真实安装 | 进行中 | `0.7.0` 本地 V3 Marketplace、Plugin、CLI 二进制、MCP initialize/tools-list 和新会话 Tool 调用已通过；正式 Git/npm 版本尚未发布。Fixture 因缺少服务端签发的 Environment Manifest 与 `environment.lock` 正确返回 `repair_required`，必须通过真实 bootstrap 建立信任，禁止伪造签名绕过 |
| W5-04 | Automation 隔离执行 | 进行中 | lease、Attempt、ExecutionBundle 和 Submission 代码已存在；V3 内容输出端到端待验收 |
| W5-05 | 首批客户试点 | 待实施 | 初始化成功率、恢复率、任务完成率和人工支持记录 |

## 4. 实施顺序

```text
W1 V3 Workspace 和 Fixture
  -> W2 本地 CLI/MCP/Plugin
  -> W3 Server Revision/Projection
  -> W4 Web 原型落地
  -> W5 CLI/Desktop/Automation 端到端
  -> 删除旧路径后的全量回归
  -> 才能重新评估发布版本
```

W1 必须先于 Web：没有真实 V3 Fixture 和本地对象契约时，Web 只能再次造假数据。W3 Projection 必须先于 W4 页面：不能让 React 页面直接拼领域真相。

当前执行位置是 `W4 页面补齐 + W5 真实宿主验收`。ScriptVersion 平行事实源和累计 migration 已删除，完整业务 Fixture 已通过真实 CLI 子进程生成，开发态 Codex CLI 新会话也已完成只读 MCP 调用。下一步优先级固定为：收敛新的正式发布版本并打通 Git/npm 安装链，随后验证带认证的云端 bootstrap 主链和 Codex Desktop 双对话。

### W6 零兼容治理收口

| ID | 任务 | 状态 | 验收标准 |
| --- | --- | --- | --- |
| W6-01 | ContentBatch/ContentItem 命名与目录收口 | 已完成 | 无 `local.script`、`script_files`、`ScriptPackageV2`、`script_refs`、`script_id` 或 `batch.json` 运行时入口 |
| W6-02 | Submission hash 与两阶段审批收口 | 已完成 | Revision/Snapshot 始终使用完整 `sha256:` 摘要，客户审批只接受 `submission_revision` |
| W6-03 | Result/Learning 收口 | 已完成 | 导入、存储、评级、Lineage 和 Web 只接受 `approved_snapshot_id` |
| W6-04 | Artifact 收口 | 已完成 | Artifact 只绑定 ApprovedSnapshot；Extension Envelope、Presentation/local-open、设备轮询和 ScriptVersion fallback 已删除 |
| W6-05 | 旧 Script/TaskRun 服务收口 | 已完成 | CLI、dispatch、Service、Store、Domain、Lineage、测试和 Web 不再创建或读取 ScriptVersion；TaskRun 仅承载 Automation 通用契约、租约与 Attempt；DeliveryPackage 以 ContentItemID 选择快照内容 |
| W6-06 | PostgreSQL V3 基线 | 已完成 | migration 集合只含 `00001_v3_baseline.sql`；真实空库迁移和 PostgreSQL 集成测试通过；检测到旧 migration 历史时明确拒绝并要求重建开发库 |
| W6-07 | Legacy 回流守卫 | 已完成 | `governance:v3` 封锁旧 Strategy/Brief/Script、ContentItem、Result、Artifact 和 capability 符号；Go 测试锁定单一 migration 集合和旧历史拒绝规则 |

## 5. 测试矩阵

### 5.1 目录与 Codex

- `.codex/config.toml` 只在需要 Codex 项目配置时生成。
- 普通 Run 不写 `.codex` 或 `.agents` 受保护目录。
- `.contentcloud/locks`、inbox、cache 和 sync 在 workspace-write 中可运行。
- AGENTS、Plugin Skill 和 project config 的作用域不重叠。
- 非 Git Workspace 必须验证已打开正确根目录、已被用户信任且具备有效写权限。

### 5.2 知识与内容

- Fact 无 Evidence 不能进入 review_ready。
- Claim 无批准不能进入正式脚本。
- Asset 无 valid Rights 不能进入 Delivery。
- Conflict 不能被最后写入值静默覆盖。
- blocked CreativeDraft 可提交方向评审但不能交付。

### 5.3 Web 与原型

- 每个侧栏入口对应真实路由和 query。
- 总览数字来自同一 ProjectProjection。
- 空状态、失败、blocked、待审和已批准状态均有 Fixture。
- V2/V3 原型截图与 React 实现做结构对比，不要求像素完全相同，但层级、动作和信息不可缺失。

### 5.4 同步与安全

- 普通对话不联网。
- publish 必须 preflight + 精确确认。
- pull 只写不可变 inbox/cache。
- metadata/evidence/full disclosure 严格生效。
- token、绝对路径、transcript、隐藏推理和未授权原件不进入服务端。

### 5.5 PostgreSQL V3 基线

- 新数据库只能应用 `00001_v3_baseline.sql`，migration 目录不得出现第二个累计或兼容迁移。
- 发现任何旧 migration 记录时必须拒绝启动迁移并要求重建开发数据库，不执行 backfill、双写或在线兼容。
- JSON 数组列的缺省值统一写入 `[]`，禁止使用 JSON `null` 绕过领域语义。
- 空库集成测试必须覆盖 Submission/Decision/ApprovedSnapshot/Delivery/Result、RLS 与不可变触发器。

## 6. 变更记录

| 日期 | 变更 |
| --- | --- |
| 2026-07-27 | 读取金陵古都香真实架构，确认 V2 Web 与客户端工作流脱节 |
| 2026-07-27 | 建立 V3 客户端 Workspace、服务端治理、业务时序、Web、契约和零兼容计划 |
| 2026-07-27 | 依据 Codex 官方手册明确 `.codex`、`.agents`、`.codex-plugin` 与 `.contentcloud` 边界 |
| 2026-07-27 | 记录当前 Web 未实现 V2 九域原型的事实，并建立 V3 原型追踪门禁 |
| 2026-07-27 | 新建 V3 可交互原型，沿用 V2 工作台视觉并替换为真实客户端状态 |
| 2026-07-27 | 用户确认 V3 全量实施并授权清理旧实现，D-01 至 D-09 转为已确认 |
| 2026-07-27 | 复核最新 Codex 官方手册：Desktop/CLI 共享配置层，Plugin 安装后在新会话生效，IDE 首版不作为 Plugin 宿主 |
| 2026-07-27 | 实现 V3 Workspace、Source/Knowledge/Brief、ContentBatch/ContentItem、RunClaim/Handoff 与 Plugin MCP 工具 |
| 2026-07-27 | 实现七种 Submission、两阶段 Decision、ApprovedSnapshot、ProjectProjection、12 视图 Web 和脱敏 Fixture importer |
| 2026-07-27 | Artifact 收敛到 ApprovedSnapshot，删除 Extension Envelope、Presentation/local-open、设备打开轮询和 ScriptVersion 工件旁路，并新增 V3 runtime guard |
| 2026-07-27 | 删除 ScriptPackageV2、旧本地 script 命令、V1 影子快照和 Legacy 客户审批链，统一 Revision `sha256:` 契约 |
| 2026-07-27 | Result/Learning 改为只绑定 ApprovedSnapshot；全量 Go 测试、Web 32 项测试和生产构建通过 |
| 2026-07-27 | 新增外部脱敏金陵 V3 Fixture、空目录安全物化器和 `workspace fixture apply`；真实 CLI 子进程生成 20 来源、15 维、七层 Pack、completed Run 与 blocked 十条内容，并修正 macOS 路径别名和 Pack digest 契约 |
| 2026-07-27 | PostgreSQL 实库迁移未验收：当前环境没有 `CONTENTCLOUD_TEST_DATABASE_URL` 或 Docker，禁止将静态 SQL 检查记为通过 |
| 2026-07-27 | V3/V4 联合实现 ProjectProjection 类型化 navigation 与 allowlisted 登录恢复；Page Contract 仍是开发中草案，待信息架构联合冻结 |
| 2026-07-27 | V3/V4 联合补齐 Browser Skill 确定性安全 Eval：只查看不写、Tool/Browser 成功分离、页面注入和任意 URL 负测进入 Plugin 评测；开发态签名保持 pending，不能冒充已发布证据 |
| 2026-07-27 | V3/V4 联合实现 Web Project/feedback Codex 恢复入口：服务端不伪造本机 `path`，新对话先选择 Workspace 并以 `workspace_context` 验证项目；Assignment 仍等待 W3-01，不计为已完成 |
| 2026-07-27 | 删除 ScriptVersion 平行事实源及 script CLI/dispatch/DAO/Lineage/测试旁路；TaskRun 收缩为 Automation 通用执行骨架，当前只承载知识提取消费链，并扩大 V3 runtime guard |
| 2026-07-27 | DeliveryPackage、导出参数、OpenAPI、SQL、Lineage 与 ContentBatch 引用统一改为 `ContentItemID/content_item_id/content_item_refs`，不保留旧 script 字段别名 |
| 2026-07-27 | 删除 17 个累计 migration 并重建唯一 `00001_v3_baseline.sql`；真实 PostgreSQL 空库迁移、主链、RLS 和不可变约束测试通过，旧 migration 历史改为明确拒绝 |
| 2026-07-27 | PostgreSQL 所有受数组约束的 JSONB 写入统一将 nil 规范化为 `[]`；V3 runtime guard、Web 36 项测试、类型检查和生产构建通过 |
| 2026-07-27 | 在不覆盖正式 `contentcloud@v0.6.0` 的前提下安装 `contentcloud-dev` 本地 Marketplace 与 V3 CLI；MCP 握手、Tool 清单和真实 Codex CLI 新会话 `workspace_context` 调用通过。确认正式 v0.6.0 的 Git/npm 发布事实断裂，随后统一准备并重新签名 `0.7.0` 候选；远端 tag/npm 未发布，tagged 门禁继续失败。Fixture 的 `repair_required` 来自缺少服务端签发 Environment Manifest/lock，保持 fail-closed |
| 2026-07-27 | 将开发态宿主升级到 Plugin `0.7.0-dev.1` 和 CLI `0.7.0`；本地 npm 启动器、MCP initialize/Tool、新 Codex CLI 临时只读会话均通过。全量 Go 测试、V3 legacy guard、Plugin source/signing 门禁、Web 41 项测试与生产构建通过 |
| 2026-07-27 | V3/V4 联合收紧 Web 披露边界：401 可恢复登录、403/404 统一不可访问状态、Revision 只显示披露摘要；Submission 校验拒绝 metadata/full-source 夹带 Evidence Pack，完整授权模型仍未冒充完成 |
