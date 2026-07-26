# Changelog

ContentCloud 的重要变更记录在此文件中。

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
