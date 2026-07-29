# Changelog

ContentCloud 的重要变更记录在此文件中。

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
