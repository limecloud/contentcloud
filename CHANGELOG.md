# Changelog

ContentCloud 的重要变更记录在此文件中。

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
