# Business Workbench Plugins

`workbench-plugins/` 是业务工作台插件层，和根目录 `plugins/` 的 Agent Plugin 层严格分开：

```text
plugins/                 Agent Plugin：Skill、MCP、宿主安装和本地执行能力
workbench-plugins/       Business Workbench Plugin：客户导航、阶段、画布和对象语言
```

业务工作台插件只能声明客户体验契约，并引用平台已经发布的 `ExperienceTemplate`。它不能携带任意服务端脚本、数据库迁移、Provider 凭据、Runtime 状态机或自动发布逻辑。

## 包结构

```text
workbench-plugins/<plugin-id>/
├── workbench.json       # contracts/workbench-plugins/1.0.0 schema
├── references/          # 可选：面向客户的帮助和字段说明
└── assets/              # 可选：经过审核的静态图标或预览资源
```

`workbench.json` 通过 `experience.template_id` 绑定已发布体验模板，UI 只能选择平台批准的 renderer/layout/density/theme 和阶段动作。所有写操作仍通过客户 BFF 返回的 action contract，插件不能在浏览器中自行拼接 API 或状态机。

## 与 Agent Plugin 的关系

一次业务工作台可以同时依赖两个独立发布物：

```text
Business Workbench Plugin
  -> 客户页面的导航、阶段、画布和对象语言
  -> ExperienceTemplate / Published SOP / BFF action contract

Agent Plugin
  -> Codex / Claude Code 的 Skill、MCP 和执行宿主投影
  -> Runtime capability / local workspace / provider adapter
```

它们使用不同的身份、摘要、审核和生命周期治理，不允许把 Agent Plugin 的 `plugin.json` 当作客户工作台 manifest。

## 当前状态

1.0.0 manifest 校验和服务端 Registry 已实现于 `internal/experience/workbench`。Customer BFF 会返回已发布工作台的 `plugin_id`、版本、digest、布局、密度、导航和阶段动作；缺少批准工作台的 SOP 不会直接进入客户面。视频、文章、电商和连载小说的首批声明式入口由平台内置 Registry 提供，前端宿主在 `apps/web/src/workbench` 按业务选择对应 feature。

当前 Registry 由进程内首方声明和平台范围的持久化版本合并组成。首方定义位于 `apps/web/src/workbenches/<business>/definition.ts`，业务简报字段位于同一业务目录；`apps/web/src/workbench` 只负责宿主和安全回退。平台管理员通过 `/api/bff/admin/workbenches` 登记 `draft`、发布为 `published`、正常退役为 `retired`、恢复已退役版本，或填写原因后安全撤销为 `revoked`，并设置租户范围。发布时服务端会检查体验模板是否在平台批准目录中，并阻止同一发布范围内同一内容类型出现两个默认版本。Customer BFF 只返回当前租户可用的 `published` 声明。`retired` 只阻止新任务选择，已固定任务继续读取原版本；`revoked` 不可恢复，也不可覆盖租户范围或撤销原因，历史任务会进入安全阻断。

当租户工作台 manifest 已批准并绑定到当前任务时，客户面优先使用 manifest 的导航、阶段、面板标题、对象语言和入口动作；未声明的部分才回退到对应首方业务定义。这个优先级只影响客户体验投影，不会覆盖平台事实或执行协议：任务、SOP、Gate、Review、Runtime、ApprovedSnapshot、Artifact、DeliveryPackage 和 PerformanceObservation 仍由平台服务和各自业务模块拥有。任意代码 bundle、第三方远程 iframe 和插件自带 API 仍不开放。
