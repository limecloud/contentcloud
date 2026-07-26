# Changelog

ContentCloud 的重要变更记录在此文件中。

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
