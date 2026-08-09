# Changelog

ContentCloud 的重要变更记录在此文件中。

## [0.21.0] - 2026-08-09

### Added

- 增加 Runtime fence token、资源配额/预留、类型化状态集合、ToolCall、Provider inbox、费用对账、yield/resume、会话持久化和投影重建能力，对应迁移 `00021` 至 `00033`。
- 增加 Runtime worker 的 prepare/activate/heartbeat/finalize 协议、DurableHarness 跨进程恢复，以及 Runtime Explorer 的暂停、重放、分支和外部副作用对账操作。
- 增加关系化执行计划、GraphPatch、Fanout/Join、业务绑定、输入快照和业务结果交接，使第二种内容流程复用同一 Runtime 内核。

### Changed

- 将 `JobRun`、`NodeRun`、`RuntimeAttempt` 收敛为唯一执行事实源，运行列表、进度、后台诊断和工作器协议统一读取 Runtime 投影。
- 更新 Operations 控制平面、OpenAPI、CLI、架构与 V8 路线文档，明确真实 Provider、故障注入、容量和 Canary 尚未完成的边界。

### Removed

- 迁移 `00034_remove_v7_execution.sql` 删除旧 `task_runs`、`run_attempts`、`run_progress_events` 和 `creative_execution_bundles` 执行表；该变更不可通过应用二进制回滚恢复旧执行数据。

## [0.20.0] - 2026-08-08

### Added

- 引入 Runtime 统一事务命令内核与可靠 Outbox 投递，新增迁移 `00018`、`00019`，为快照、事件和投递状态提供一致的提交边界与租约围栏。
- 增加 Operations 控制平面、执行器与技能目录 API、管理后台视图，以及插件声明、能力契约和本地运行入口。
- 扩展 Customer Studio 的资产、知识和团队工作区，并拆分资产详情与查看器模块。

### Changed

- CLI bootstrap、workspace 和插件宿主统一使用新的运行时/插件边界；同步收敛架构、产品和 V8 路线文档。
- Memory Store 与 PostgreSQL Store 对 Runtime 命令、幂等、Outbox claim/ack/retry 语义保持一致。

### Fixed

- 修复 Memory Store Outbox 批量 claim 回滚时复合租户 key 丢失导致的重复消息领取问题。

## [0.19.0] - 2026-08-07

### Added

- 增加 WorkspaceMaterial 领域模型、项目文件夹、资料上传、检索与使用记录，并提供 Memory/PostgreSQL 存储及迁移 `00017`。
- 增加 Customer Studio「我的资产」工作区资料视图，支持按项目、文件夹和资料类型组织创作输入。

### Changed

- 将客户资产界面收敛为「我的资产」与「创作结果」两类事实来源，任务创建可同时绑定资料和已确认创作结果。
- 更新 Customer Studio API/OpenAPI、服务层、Web 交互和测试，明确资料输入、结果复用与交付边界。
- 收敛产品与基础文档到当前事实源，移除已被取代的历史 roadmap 和旧重设计计划。
- Server、Worker、Web、CLI、Plugin、MCP 和 Environment Profile 统一升级到 `0.19.0`。

## [0.18.0] - 2026-08-07

### Added

- 增强 Customer Studio 客户创作台，补齐创作电脑连接、项目参考、任务资产和交付下载状态。
- 收敛项目、资产与交付边界，新增客户结果资产边界决策记录 ADR-0013。

### Changed

- 改进 Runtime 创作流程、首次引导、工作区路由、官网导航和页面布局。
- 更新 API/OpenAPI 合同、项目视图和对应 Go/Web 回归测试。
- Server、Worker、Web、CLI、Plugin、MCP 和 Environment Profile 统一升级到 `0.18.0`。

## [0.17.0] - 2026-08-04

### Added

- 增加知识对象治理视图，按版本、项目状态、角色、对象类型和 Evidence 完整度返回明确的允许动作与治理说明。
- 增加可验证的真实 MP4 测试夹具、扩展 box 与末尾 box 解析覆盖，以及 V7 开发 Fixture。

### Changed

- Knowledge 列表、管理后台和工作区统一展示审核、只读、历史、待补证据和待解决冲突状态。
- 媒体 Provider 校验使用嵌入式视频 Fixture，并收紧下载内容的容器、大小和摘要验证。
- Server、Worker、Web、CLI、Plugin、MCP 和 Environment Profile 统一升级到 `0.17.0`。

### Fixed

- 修复历史知识版本或归档项目仍暴露治理动作，以及合法 MP4 扩展 box 被误判为无效容器的问题。

## [0.16.0] - 2026-08-04

### Added

- 增加 V7 营销视频生产纵向链路：类型化 Stage 输出、分镜素材、媒体生成任务、Provider 运行、Take 质检、最终渲染和交付包。
- 增加营销视频内置 SOP、媒体任务和任务详情读模型；工作台展示来源、知识、批准快照、媒体 Job、质检和交付事实。
- 增加 V7 路线、领域契约、PostgreSQL 迁移以及 Memory、Postgres、Worker 和 Web 回归测试。

### Changed

- Stage 上报改为验证当前租户和项目内规范对象的 digest、version、status；营销视频任务拒绝裸字符串输出，并要求服务端 DeliveryPackage 才能完成交付。
- 项目内容类型会选择对应的默认 SOP；Server、Worker、Web、CLI、Plugin、MCP 和 Environment Profile 统一升级到 `0.16.0`。
- 保留 V6 任务与交付的兼容读路径，新增媒体事实通过类型化对象和血缘引用进入任务投影。

### Fixed

- 修复从输入创建任务时项目内容类型丢失，以及旧 SOP 绑定不会按项目类型修复的问题。
- 修复任务详情无法展示规范对象、媒体输出和交付事实的问题。

## [0.15.0] - 2026-08-03

### Added

- 增加首次连接后的项目简报入口，让项目、素材、知识和内容生产沿一条可恢复的本地业务链路推进。
- 增加 `workspace_context` 的结构化 onboarding 状态与 `workspace_project_brief`，项目简报确认后写入共享业务上下文。

### Changed

- 新会话在项目简报确认前只展示一个业务下一步，避免把内部工具和未准备好的任务入口同时暴露给用户。
- 工作区 MCP、Codex 接入、Plugin、Web、Environment Profile 和 CLI 安装器统一升级到 `0.15.0`。
- 项目服务计划默认声明 SOP 可配置、审批可选，后续规则由管理后台和工作区配置承载。

### Fixed

- 修复已连接工作区没有明确下一步、连接链路与首次业务流程断开的问题。
- 收敛后台工作区和知识库标签，减少重复入口和含义不清的英文文案。

## [0.14.0] - 2026-08-02

### Added

- 增加 Content Work OS 基础设施：Task/TaskRun、Environment、可版本化 SOP、Stage/Gate、输入收集、知识对象/知识包/不可变快照、会话导入和任务治理契约。
- 增加 Task-first 工作区、知识库、SOP/运行配置后台和对应 BFF API、PostgreSQL 迁移、Memory Store 与领域/服务/HTTP 回归测试。
- 增加短视频脚本与文章任务共用的 Revision、Gate、Delivery 和结果追溯基础链路，并补充 Content Work OS 路线、架构、安全、工作流与验收文档。

### Changed

- 工作区、项目页、后台导航和 API 路由收敛到 Task-first 主路径；旧知识冲突旁路和重复主导航移除，历史事实保持只读。
- 新增显式输入、知识、SOP、任务和阶段动作，所有租户边界、幂等键、版本绑定、RLS 和不可变约束由服务端校验。
- CLI、Web、npm 安装器、Plugin、MCP、Environment Profile、Bootstrap 和 Server/Worker 版本统一升级到 `0.14.0`。

### Fixed

- 修复领域输入项文件未按 Go 标准格式化的问题，并补齐新版工作区和任务治理接口的类型、路由及测试覆盖。

## [0.13.0] - 2026-08-01

### Added

- 增加共享 `BrandMark` 与 `BrandLockup` 组件，为官网、工作台、后台、登录注册、文档和公开页面提供统一的产品标识。
- 增加新的 Content Work OS favicon 与品牌使用规范，明确产品标识、页面角色、颜色和可访问性边界。

### Changed

- 一级产品品牌统一为 `Content Work OS`，并重构官网、工作台、后台、登录注册、文档和公开审批页的导航、文案与视觉层级。
- 更新公开内容目录和 v6 官网路线文档，使产品命名、页面职责与当前实现保持一致。
- CLI、Web、npm 安装器、Plugin、MCP、Environment Profile、Bootstrap 和 Server/Worker 版本统一升级到 `0.13.0`。

### Fixed

- 修复多 Workspace Automation Daemon 并发读取运行时绑定时共享底层配置切片的数据竞争，保证绑定归一化为只读快照。

## [0.12.0] - 2026-07-31

### Added

- 增加 ContentCloud Content Work OS 官网首页，展示本地 Agent 创作、团队审核和多渠道交付的受治理生产链。
- 增加官网视觉资源、产品入口和 v6 产品、运行时及迁移路线文档。

### Changed

- 公共首页与登录后的工作台路由分离：`/` 用于官网，`/workspace` 用于生产工作台；项目深链、团队页、文档页和后台返回路径统一使用新的工作台入口。
- CLI、Web、npm 安装器、Plugin、MCP、Environment Profile、Bootstrap 和 Server/Worker 版本统一升级到 `0.12.0`。

## [0.11.0] - 2026-07-31

### Added

- 增加 V5 Automation Runtime：本地 Daemon 生命周期、LaunchAgent 管理、多 Workspace 绑定、并发租约执行、受管日志轮转和版本更新策略。
- 增加 Automation Attempt 持久重报与实时进度链路；支持断网/重启恢复、不可变 `run_progress_events`、CLI 增量事件读取和 Web SSE 断线续传。

### Changed

- Automation Agent 按已确认 Task Contract 使用隔离 Attempt 工作目录执行；Codex/Claude 的运行参数、进程组回收和 Provider 环境继承统一收敛到无人值守运行模型。
- Bootstrap、Workspace、Handoff、CLI、Web、npm 安装器、Plugin、MCP、Environment Profile 和 Server/Worker 版本统一升级到 `0.11.0`，服务端增加 Daemon 最低版本与最新版本门禁。

### Fixed

- 修复 Daemon 重启、网络失败和服务端永久拒绝时的结果重放与 dead-letter 边界，避免重复执行或丢失已完成 Attempt 结果。

## [0.10.0] - 2026-07-30

### Added

- 增加按租户开通的内容能力管理；视频剧本保持默认能力，平台管理员可在 Web 管理端显式启用微信公众号文章，并由签名 Environment Manifest 同步到本地 Workspace。
- 增加 ArticleBrief、ArticleItem 和 WeChatDeliveryPackage 契约，以及文章规划、长文写作、视觉规划和微信交付四个 Skills；CLI/MCP 支持批次冻结、确定性 lint、受控 revision diff 和本地交付包导出。
- 增加同源公开文档中心，Web `/docs`、JSON API 与 `Accept: text/markdown` 共享内嵌目录；覆盖 Agent 客户端、内容类型、工作流和故障排查，并明确内部架构文档不对外暴露。
- 增加 PostgreSQL `00003_tenant_content_capabilities.sql`，以增量表保存租户内容能力，不改写现有项目、Submission、Revision 或 ApprovedSnapshot 数据。

### Changed

- ContentBatch 从视频专用结构扩展为显式 `content_kind`、Schema 引用和交付 Profile 路由；服务端发布、内审和客户批准均复验租户能力、ArticleBrief、Knowledge ApprovedSnapshot 与商业主张血缘。
- Web 客户审批页可按 Schema 展示视频剧本或结构化公众号文章，并将评论稳定绑定到文章 block、assertion 和 Revision digest。
- CLI、Web、npm 安装器、视频 Plugin、MCP、Environment Profile 和 bootstrap 固定版本统一升级到 `0.10.0`；微信文章 Skill Pack 首版为 `0.1.0`，只提供受治理的本地人工交付，不登录或代替用户发布到微信。

### Fixed

- 发布和批准阶段会再次拒绝未开通内容类型、混合内容批次、缺少已批准 ArticleBrief/Knowledge 基线及 Claim 类型不匹配，避免本地状态或旧 Manifest 绕过服务端治理。
- 文档 API 对 Markdown 协商、未知页面与内部页面执行明确的内容类型、缓存和不可见边界，避免 SPA fallback 将不存在的 Skill 引用伪装为成功响应。

## [0.9.0] - 2026-07-29

### Added

- 增加统一 Agent Client Registry，集中登记 Codex、Claude Code、WorkBuddy、Cursor、Hermes 和 OpenClaw，并按客户端显式公布自动化、Workspace 注册、初始化、交接和创作环境能力状态。
- 增加通用 Agent Handoff API 和 Web 客户端选择器，支持从项目与审核反馈页面生成绑定精确项目、Revision 与 digest 的受验证交接。

### Changed

- Workspace 注册、bootstrap、Automation Adapter 与 Environment Manifest/Profile/Lock 统一使用 Agent Client Registry，并将未实现的客户端能力明确标记为 `planned`。
- Codex 交接收敛为通用 Handoff 策略的已发布实现，旧 Codex 响应保留兼容层；CLI、Web、npm 安装器、Plugin、MCP、Environment Profile 和 bootstrap 固定版本统一升级到 `0.9.0`。
- bootstrap 增加同源 Marketplace/Plugin 版本检测、显式升级计划与 `resume` 恢复；旧 Marketplace 与 Plugin 必须作为同一可回滚快照更新，异常的 Plugin-only 版本漂移会 fail closed。

### Fixed

- 拒绝未知 Agent 客户端和尚未提供的能力，避免将保留客户端误报为可初始化或可交接。
- 收紧 Agent Handoff Web 校验，要求 Codex Prompt 同时绑定 Plugin、Workspace 上下文、项目以及审核反馈的 Revision/digest。

## [0.8.0] - 2026-07-29

### Added

- 增加 V5 抖音电商人群目录、人群策略、商品权益、分镜包、Seedance 提示词包和已发布创意绑定契约，明确本机候选、服务端批准与外部平台人工操作的执行边界。
- 增加人群单选、2 至 3 类对比和八类探索的本地生成与 lint，以及基于已拉取 ApprovedSnapshot 的分镜准备和确定性 Seedance 导出命令。
- 增加抖音人群策略、分镜生产和 Seedance 导出三个 Plugin Skill，并为 V5 本地纵向切片增加独立确定性评测场景。

### Changed

- Submission 与 ApprovedSnapshot 扩展支持 `strategy`、`offer` 和 `storyboard`，服务端在 publish 与批准阶段复核 taxonomy 有效期、ApprovedSnapshot 血缘和分镜锁定摘要。
- CLI、Web、npm 安装器、Plugin、MCP、Environment Profile 和 bootstrap 固定版本统一升级到 `0.8.0`，Plugin 发布门改为校验明确的六 Skill 清单。
- V5 当前按受治理的本地纵向切片发布；Web 审核交互、媒体生成能力、PublishedCreativeBinding 结果归因和真实 Seedance/抖音 E2E 仍保留为后续发布门，不宣称完整生产闭环已验收。

### Fixed

- 修复 bootstrap 测试夹具依赖历史绝对时间、会在日期推进后因测试会话过期而失败的问题。

## [0.7.0] - 2026-07-27

### Added

- 增加 V3 Workspace、Source Registry、Markdown Knowledge Page/Pack、Brief、ContentBatch/ContentItem、LocalRun 和 Handoff 契约，并提供可重复生成完整项目状态的脱敏 Fixture。
- 增加统一的 SubmissionRevision、Decision、ApprovedSnapshot 和 ProjectProjection 主链，覆盖七类 Submission、两阶段审批、交付、结果回流及 12 个 Web 项目视图。
- 增加 Page Contract、项目与 Revision 深链、`/codex` 双模接入、Web 到 Codex 恢复入口及只读 Browser 导航安全评测。
- 增加 PostgreSQL V3 空库基线、RLS/不可变约束集成测试和旧事实源回流治理守卫。

### Changed

- 本地创作事实源统一为 V3 数字目录和 `.contentcloud` 状态；云端正式事实统一为 Revision、Decision、Snapshot 与投影。
- Plugin Skills、MCP、CLI、Web、Environment Profile 和 npm 安装器统一升级到 `0.7.0`，内容生产从 ScriptPackage 收敛为 ContentBatch/ContentItem。
- Delivery、Artifact、Result 和 Learning 只绑定 ApprovedSnapshot；导出选择统一使用 `content_item_id`。
- Web 项目工作台改为消费服务端 ProjectProjection，并使用共享 Page Contract 校验 view、focus、digest 和登录返回路径。

### Fixed

- 将 Submission 内审批准、退回修改和客户审批前置状态切换收紧为基于当前 Revision 与预期旧状态的原子更新，避免并发决定生成互相矛盾的 ApprovedSnapshot 与 Submission 状态。

### Removed

- 移除 V1/V2 Strategy、Brief、ScriptVersion、ReviewCycle、Artifact Presentation/local-open 等平行运行时与兼容入口。
- 移除累计旧迁移并改为唯一 `00001_v3_baseline.sql`。这是破坏性数据库边界：`0.7.0` 只支持新建 V3 数据库，不提供现有 V1/V2/`0.6.0` 数据库的在线升级或回填。

## [0.6.0] - 2026-07-27

### Added

- 增加面向普通客户的 bootstrap 环境预检、浏览器设备授权、实时阶段进度、版本化处置动作和显式确认流程。
- 增加本地脱敏诊断预览与确认后上传能力，并补齐初始化流程、诊断协议和支持 Runbook。
- 增加 Web 初始化进度与短码核对界面，支持后台等待、失败恢复和新 Codex 对话交接。

### Changed

- 初始化 Prompt 改用公开 ConnectSession ID，设备与 Workspace 凭据只通过 PKCE 浏览器授权换取并存入 macOS Keychain。
- 将 CLI、Web、npm 安装器、Plugin、MCP、Environment Profile 与 bootstrap 固定版本统一为 `0.6.0`。
- 移除旧 `cck_` 连接码通路，云端只接收版本化进度事件和用户明确同意上传的脱敏诊断摘要。

### Fixed

- 修复 bootstrap 匿名入口绕过 PostgreSQL 租户上下文直接读取 RLS 表，导致真实数据库无法启动授权的问题。
- 修复生产曾登记旧 `00018` 后会跳过新 bootstrap 表结构的问题，新增兼容迁移并保留已发布迁移不变。
- 修复同一初始化会话可并发创建多个待批准 attempt，以及 CLI 可能打开非同源验证地址的问题。
- 修复 Environment Manifest 签发失败时授权已被提前消费、导致设备凭据无法重试的问题。
- 修复诊断重试返回未落库 ID，以及初始化 attempt 进入终态后仍可追加进度的问题。

## [0.5.0] - 2026-07-27

### Added

- 增加 ContentCloud 精选 Marketplace、`contentcloud-video-production` Scene Plugin、三个 canonical Skills 与 bundled `contentcloud-local` MCP，并建立确定性评测、digest、Ed25519 签名和撤回门禁。
- 增加项目级 Creative Environment Control Plane，覆盖签名 Manifest、可信 Registry、Environment Lock、Pack preparation、升级/重置计划及离线 doctor。
- 增加 CreativeExecutionBundle、capability catalog、Automation 租约前环境校验与隔离执行工作区，使交互式创作和后台执行共享同一套可审计能力契约。
- 增加 `bootstrap plan/apply/resume` 安装事务、Codex Marketplace/Plugin 状态检测、新会话 handoff、RunClaim 和跨对话原子交接。
- 增加本地审核反馈 inbox、ApprovedSnapshot 只读缓存，以及 CLI/MCP 的显式拉取、查看和精确确认发布流程。

### Changed

- 将内置创作 Skills 收敛到 Scene Plugin 单一事实源，由 Go CLI、Workspace Template 和 Codex Plugin 共同引用。
- 将 CLI、Web、npm 安装器、Plugin、MCP 与 bootstrap 固定版本统一为 `0.5.0`。
- 服务端可通过 systemd 配置启用签名 Environment Profile 和 capability release，bootstrap 与 Automation 在缺少可信环境时 fail closed。

### Fixed

- 修复 Environment Preparation 超过 lease TTL 后 RunClaim 可提前进入的问题；只要 preparation 文件存在，运行领取始终保持关闭。
- 修复 CreativeExecutionBundle 在 PostgreSQL 中可被 runtime 更新或删除的问题，增加权限撤销、不可变触发器和 RLS 集成断言。
- 修复 Node 23 下发布签名工具无法为现有 public `KeyObject` 计算指纹的问题。

## [0.4.0] - 2026-07-27

### Added

- 打通 SubmissionRevision 内审、客户 OTP 审批、ApprovedSnapshot、三格式 DeliveryPackage、投放结果、人工评级与 Lineage 的 V2 完整业务链。
- 增加 revision 级客户 ReviewGrant 的创建、查看、撤销、过期与新 revision 自动失效机制，公开审批页继续兼容 V1 历史链接。
- 增加基于 ApprovedSnapshot 的 JSON、Markdown、XLSX 导出与不可变交付包，并支持 Web 和 CLI 下载。
- 增加 `00014`/`00015` 数据库迁移，将交付和结果切换到 ApprovedSnapshot，并以可审计、幂等方式回填 V1 影子快照。
- 增加真实 PostgreSQL、应用层和 HTTP Golden Path 测试，覆盖两阶段审批、交付、结果回流、RLS、不可变约束及脏 hash 跳过。

### Changed

- 内部批准只推进到 `internally_approved`，客户批准才原子生成 ApprovedSnapshot，避免未获客户确认的内容进入正式交付。
- Brief 强制绑定已拉取的 Strategy ApprovedSnapshot；PerformanceObservation、RatingDecision 与 Lineage 统一使用批准快照作为事实源。
- 更新 CLI `review`、`artifact`、`result` 命令和 OpenAPI 契约，使 revision、快照、交付包与结果字段保持一致。
- 重构租户控制台路由与 Editorial Studio 视觉体系，保留旧 `/workspace/*` 地址的兼容跳转，并完善审批、交付和结果操作界面。
- 本地开发改为 `make dev` 同时运行 Go API 与 Vite，默认使用隔离 Memory Store；构建后单体预览使用 `make preview`。

### Fixed

- 修复 V1 影子回填遇到非 SHA-256 历史 hash 时导致迁移失败的问题；无效记录仅计入报告，不伪造或重算历史 hash。
- 修复 CLI `review status` 仍读取 V1 ScriptVersion 的问题，改为返回 Submission、Revision 与客户授权状态。

## [0.3.0] - 2026-07-26

### Added

- 增加独立平台管理员后台，提供全平台租户、用户、项目、在线设备和活跃任务概览。
- 增加租户停用与恢复能力；停用时原子撤销该租户的活动会话，并阻止成员继续登录。
- 增加 `/workspace`、`/admin`、认证和公开审批页面的独立 React Router 路由树及按需加载。
- 增加公开 `/api/bootstrap` Agent 初始化协议和 Web Prompt 引导；只有项目级配置、doctor 与 `workspace.register` 全部完成后才确认连接成功。

### Changed

- 将 npm 包作用域统一迁移到 `@limecloud/contentcloud`。
- 统一 Server、Worker、CLI、Web 和 npm 安装器版本为 `0.3.0`，GitHub 发布标签为 `v0.3.0`。
- 平台管理员权限改为通过 `CONTENTCLOUD_PLATFORM_ADMIN_EMAILS` 显式配置，不复用租户角色。

### Fixed

- 在宝塔 Nginx HTTPS 反向代理及原生 TLS 场景下为会话 Cookie 启用 `Secure` 标记，同时保留本地 HTTP 开发能力。
- 隐藏停用租户的登录入口，并在恢复后重新允许其成员建立会话。

## [0.2.0] - 2026-07-26

### Added

- 增加 V2 本地工作区能力，覆盖来源登记、知识治理、Brief、创意批次、ScriptPackage、LocalRun、发布与拉取流程。
- 增加 V2 JSON Schema、CLI/MCP 命令、XLSX 导出能力及完整回归测试。
- 增加邀请注册流程，受邀用户可直接加入邀请方租户。
- 增加无 Docker 的 systemd 部署配置，支持独立运行 Server 与 Worker。

### Changed

- 更新 V2 产品路线、业务能力、领域模型、交付计划和实现状态文档。
- 统一 CLI 与 npm 工作区版本为 `0.2.0`，GitHub 发布标签为 `v0.2`。

### Fixed

- 移除核心迁移对 `pgcrypto` 扩展的无效依赖。
- 将邀请接受和邀请注册改为原子存储操作，避免并发重复兑换和部分写入。
- 加强本地工作区路径校验，阻止通过符号链接读取工作区外文件。
