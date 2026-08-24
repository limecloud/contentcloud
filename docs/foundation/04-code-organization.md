# 代码组织、模块边界与依赖规范

状态：`Accepted 目标规范；顶层命名空间、业务事实模块、命名应用服务和显式依赖装配已完成，业务工作台插件层已冻结 1.0.0 契约`。

更新时间：2026-08-23。

## 1. 重构前结构诊断

本次整改开始前，Go 代码按技术层横向组织：

```text
internal/domain/      多个业务域的模型和状态
internal/app/         全系统应用服务与用例
internal/store/       一个覆盖全系统的 Store 接口
internal/httpapi/     客户、管理员、CLI 和公开接口处理
```

这种结构在早期简单直接，但随着业务增长会产生：

- `store.Store` 变成胖接口，任何实现和测试替身都感知全系统。
- `app.Service` 持有全局依赖，业务用例容易跨域直接调用。
- `domain` 只能按文件区分所有权，无法用 Go import 规则阻止越界。
- HTTP DTO、领域对象和投影容易共用同一结构，兼容变更影响扩大。
- 新功能自然继续加入旧宽包，目录搬迁后仍会复制相同问题。

旧顶层包和旧 import 已删除。业务事实已落到命名模块，跨域用例由 `internal/application` 中的命名应用服务协调，启动边界通过 `application.Dependencies` 显式装配；后续新增能力必须直接更新调用方，不得重新建立别名、Facade 或转发包。

## 2. 目标架构层次

```text
Interfaces
Web Studio / Web Admin / HTTP / CLI / MCP
        |
        v
Application
Commands / Queries / Coordinators / Projections
        |
        v
Business Modules                Runtime Module
Identity / Catalog / Work       Job / Node / Attempt / Effect
Source / Review / Delivery             |
        |                               |
        +---------------+---------------+
                        v
Ports
Repository / Blob / Clock / Queue / Provider / Agent
                        |
                        v
Adapters
PostgreSQL / Memory / S3 / Local Agent / External Provider

Composition Root 是唯一知道所有具体实现的地方
```

依赖只向内。领域模块不导入 HTTP、数据库、文件系统、Agent SDK 或服务商实现。

## 3. 目标目录

目录表示目标所有权，迁移按 [07-migration-and-delivery.md](./07-migration-and-delivery.md) 一次性切换。

```text
internal/
├── identity/                 用户、租户、成员、会话
├── workspace/                项目、品牌上下文、设备与 Workspace 绑定
├── source/                   来源、证据、知识和权利
├── catalog/                  Experience、SOP、Gate、Capability、发布
├── work/                     WorkTask 和客户业务生命周期
├── application/              命名应用服务、跨域协调器和显式 Dependencies
├── runtime/                  Job、Plan、Node、Attempt、Lease、State、Effect
├── review/                   Submission、Revision、GateEvaluation、Approval
├── delivery/                 Artifact、Media、DeliveryPackage、外部发布回执
├── performance/              结果导入、观察、评级和学习候选
├── experience/
│   ├── projection/           跨域可重建读模型、游标和重建
│   ├── studio/               客户读模型、状态翻译和客户命令
│   └── operations/           运营读模型、配置和诊断命令
├── integration/
│   ├── agent/                Codex、Claude、Fake Adapter
│   ├── connector/            搜索、抓取和企业数据连接器
│   └── provider/             图片、视频、语音和平台 API
├── persistence/
│   ├── postgres/             各模块 Repository 实现
│   └── memory/               确定性测试与本地开发实现
├── transport/
│   ├── http/                 studio、admin、agent、public 路由和 DTO
│   ├── cli/                  公共 CLI 命令与 envelope
│   └── mcp/                  本地受限工具表面
├── platform/                 clock、id、digest、pagination 等窄技术原语
└── bootstrap/                依赖装配、配置和进程启动

contracts/
├── business/                 内容、业务包、资产目录与引用 JSON Schema
├── runtime/                  Job、Node、State、Effect 契约
├── integration/              Agent、Connector、Provider、Composition Worker 契约
└── openapi/                  按表面拆分或生成的 OpenAPI

apps/web/src/
├── app/                      Router 和根级 Provider 装配
├── platform/                 会话、租户、权限、错误边界和平台契约
├── workbench/                工作台宿主、manifest 校验和 registry
├── workbenches/              按业务拆分的客户 UI feature
│   ├── marketing-video/      视频导航、阶段、画布和业务上下文
│   ├── article/              文章目录、编辑器、引用和校对
│   ├── commerce/             商品事实、内容变体和渠道预览
│   └── serialized-novel/     Canon、卷章、章节编辑、连续性和交付
├── studio/                   迁移中的客户路由和公共旧 feature；不得继续加入业务专有面板
├── admin/                    平台运营控制台（发布、能力、运行、资产治理）
├── public/                   官网、登录、公开审核和文档
├── shared/                   纯 UI 原语、品牌、格式化和 API 基础设施
└── plugins/                  业务插件投影和租户启用状态（不执行插件代码）

apps/desktop/
├── src/main/                 Electron 生命周期、系统权限、更新和通知
├── src/preload/              版本化、运行时校验的 typed IPC
├── src/renderer/             持续项目工作面
└── tests/e2e/                真实 Daemon/Cloud fixture 的进程验收

packages/ui/                  Web/Desktop 共用的纯 UI 原语和设计 Token
packages/contracts-ts/        从 OpenAPI/Schema 生成的 TypeScript 契约

internal/local/
├── workspace/                本地目录、Claim、Proposal、Apply、View
├── sync/                     outbox、cursor、上传、下载、冲突和恢复
├── workbench/                Direct Browser 和 MCP Apps Presenter
├── desktopapi/               Desktop command/query/event surface
└── config/                   设备、绑定和本地配置

contracts/workbench-plugins/1.0.0/  业务工作台插件 manifest 与版本化 JSON Schema
workbench-plugins/                  业务工作台插件包和示例
plugins/                            Agent Plugin 包（Skill、MCP、宿主投影），与上者分开
```

目录必须由真实实现一次性填充，不创建没有所有权和测试的空壳包。

### 3.1 业务工作台插件层

业务工作台插件是产品层扩展，不是执行层插件。它只提供声明式的客户体验描述，并通过已发布 `ExperienceTemplate` 进入客户面：

```text
workbench-plugins/<id>/workbench.json
        |
        v
internal/experience/workbench
  DecodeManifest -> schema / id / layout / stage / action 校验
        |
        v
ExperienceTemplate Registry
  运营预览 -> 租户启用 -> Customer BFF Projection
        |
        v
apps/web/src/workbench (宿主)
        |
        +--> apps/web/src/workbenches/<business> (业务 feature)
        +--> apps/web/src/platform (公共会话、权限、错误和可访问性)
```

业务工作台的导航、阶段、面板文案和业务简报字段必须与业务 feature 同目录维护：

```text
apps/web/src/workbenches/marketing-video/definition.ts
apps/web/src/workbenches/marketing-video/VideoBriefFields.tsx
apps/web/src/workbenches/article/definition.ts
apps/web/src/workbenches/article/ArticleBriefFields.tsx
apps/web/src/workbenches/commerce/definition.ts
apps/web/src/workbenches/commerce/CommerceBriefFields.tsx
apps/web/src/workbenches/serialized-novel/definition.ts
apps/web/src/workbenches/serialized-novel/NovelBriefFields.tsx
apps/web/src/workbenches/serialized-novel/NovelTaskCanvas.tsx
```

`apps/web/src/workbench` 只负责宿主解析、平台路由映射、版本化简报契约和安全回退；不得重新集中保存任一业务的导航、阶段或表单文案。

工作台插件可以声明：

- 支持的 `content_types` 和已发布 `experience.template_id`。
- 客户导航、阶段、阶段结果、唯一主要动作、布局、密度和主题 token key。
- 已审核的帮助、字段说明和静态预览资源。

工作台插件不能声明或携带：

- Go 服务端代码、数据库迁移、Runtime 状态机或 Provider 凭据。
- 任意浏览器脚本、远程 iframe、任意 API URL 或自定义状态写路径。
- 超出服务端 action contract 的批准、发布、费用和租户权限。

`plugins/` 根目录保留给 Agent Plugin；它负责 Skill、MCP、宿主安装和本地执行能力。两类插件的 manifest、摘要、审核、撤回和安装回执不可互换。一个业务工作台可以引用一个或多个 Agent Plugin，但引用关系由 `ExperienceTemplate`/能力绑定维护，不写入客户页面的执行逻辑。

### 3.2 渐进迁移顺序

当前 `apps/web/src/studio` 仍是已上线客户路由，先禁止新增业务专有组件，再按以下顺序迁移：

1. 将会话、租户、权限、错误边界和公共 API 边界收敛到 `platform/`。
2. 将工作台选择、manifest 解析和公共 stage/action contract 放入 `workbench/`。
3. 将视频、文章、电商和连载小说页面分别维护在 `workbenches/<business>/`，每次只迁移一个业务并保留现有路由验收。
4. 运营端完成插件预览、租户启用和回退后，再允许外部 `workbench-plugins/<id>` 进入 registry。

迁移期间不创建 `common-business`、`generic-workbench`、全局 `types.ts` 或把不同业务重新合并的兼容层。所有共享内容必须证明自己是平台契约或无业务所有权的 UI 原语。

## 4. 模块内部结构

首阶段避免为每个模块复制复杂分层目录。一个模块默认使用简单 Go 文件：

```text
internal/<module>/
├── model.go          聚合和值对象
├── command.go        写用例
├── query.go          读用例
├── repository.go     该模块需要的窄端口
├── projection.go     由本模块拥有的读模型
└── *_test.go
```

稳定错误 envelope 位于 `internal/platform/fault`，业务模块只声明自己的错误码和触发条件。只有模块体量和团队所有权证明需要时，才在模块内部增加子包；禁止预先建立空的 `domain/application/infrastructure` 三层模板。

## 5. 依赖规则

### 5.1 Go

1. 业务模块只能导入标准库、`internal/platform` 的窄原语和明确允许的稳定值类型。
2. 一个业务模块不得直接导入另一个模块的存储实现。
3. 跨模块写操作通过公开 Command 接口或应用协调器，不直接改对方聚合。
4. Repository 接口由使用它的模块定义，具体实现位于 `persistence`。
5. `persistence`、`transport` 和 `integration` 可以导入业务模块，反向依赖禁止。
6. `bootstrap` 可以导入所有模块用于装配，任何业务模块不得导入 `bootstrap`。
7. Runtime 只依赖业务端口和值引用，不导入具体内容类型业务包。
8. 禁止新增全局 `Service`、全局 `Store`、`common`、`utils` 或 `models` 杂物包。

### 5.2 Web

1. `studio` 不导入 `admin`；`admin` 不导入客户 feature。
2. `shared` 只能包含无业务所有权的 UI 原语、品牌、通用网络和格式化能力。
3. 业务 API 类型按 feature 维护或从契约生成，不继续扩大全局 `types.ts`。
4. 路由 Shell 独立加载各自会话和读模型，不能用 CSS 隐藏不应访问的功能。
5. 客户动作使用服务端返回的 action contract，不在组件中复制状态机。
6. 页面级组件不直接拼接 Runtime 或管理员内部 URL。
7. 我的资产、创作结果和运营资产治理可以共享无业务 UI 原语，但必须使用各自 Schema、页面 DTO、权限判断和命令实现；客户 BFF 只做显式组合。

### 5.3 数据库

- 只有对应 Repository 实现可以访问该模块表。
- 跨域读取优先使用 ID 引用、专用 Query 或投影，不使用任意 JOIN 形成隐藏所有权。
- 需要跨域原子写入时，由应用协调器调用各模块事务端口，并记录明确事务边界。
- RLS、tenant_id 和项目范围是持久化契约，不依赖上层已经检查。
- 迁移只向前追加；破坏性清理在兼容读写退场后独立执行。

## 6. 公共类型规则

跨模块允许共享：

- 稳定 ID 值类型。
- TenantScope、Actor、PageToken、Digest、Money、TimeWindow 等基础值。
- 版本化引用，如 `ArtifactRef`、`SourceRevisionRef`、`ApprovedSnapshotRef`、`CreativeAssetRef`。
- 标准错误 envelope 和审计关联 ID。

禁止共享：

- 其他模块的聚合指针或可变对象。
- 数据库行结构和 SQL DTO。
- Web 页面 DTO。
- 服务商 SDK 类型。
- 任意 `map[string]any` 作为长期跨模块协议。

## 7. 已退役路径迁移映射

| 当前路径 | 目标 | 策略 |
| --- | --- | --- |
| `internal/domain/*.go` | 对应业务模块 `model.go` | 一次性按事实所有者迁移；不保留类型别名 |
| `internal/app/*.go` | 模块 command/query 或应用协调器 | 直接拆分并更新调用方，不保留全局 Service |
| `internal/store/store.go` | 各模块 `repository.go` | 迁移完成后删除全局 Store |
| `internal/store/postgres` | `internal/persistence/postgres` | 只实现模块端口，不保留旧包适配器 |
| `internal/store/memory` | `internal/persistence/memory` | 与 PostgreSQL 运行同一契约测试 |
| `internal/httpapi` | `internal/transport/http` | 按 studio/admin/agent/public 路由拆 DTO 和 Handler |
| `internal/agentadapter` | `internal/integration/agent` | 直接迁移 Harness、FakeHarness 和 Runtime Node 契约 |
| `internal/environment` | `catalog` + `integration` + `runtime binding` | 一次性拆清配置、信任和运行绑定语义 |
| `internal/mediapipeline` | `delivery` + `integration/provider` | 业务状态归 delivery，SDK 调用归 provider |
| `internal/worker` | Runtime worker / 模块 worker | 节点执行与业务处理按能力拆分 |
| `internal/localworkspace` | `internal/local/workspace` | 直接迁移本地目录事实、Claim、Proposal 和 View |
| `internal/workbench` | `internal/local/workbench` | 直接迁移 Presenter、MCP Apps 和 Direct Browser |
| `internal/localconfig` | `internal/local/config` | 直接迁移设备和 Workspace 绑定配置 |
| `web` | `apps/web` | Git rename 后同步所有构建、CI、脚本和文档引用 |
| `apps/web/src/workspace` | `apps/web/src/studio` / `apps/web/src/admin` / `apps/web/src/styles` | 客户页面已迁入 Studio；workspace 目录已退场，Admin 共享规范化器位于 `admin/operationsData.ts`，共享工作面样式位于 `styles/workSurface.css` |
| `apps/web/src/admin` | 保留并按 overview/experiences/catalog/binding-policies/runtime/assets/governance 分区 | 复用现有独立 Shell 和权限；中文页面蓝图见 `docs/product/operations-control-plane/04-page-blueprints-and-language.md`；不得继续扩大全能 Admin 页面 |
| `apps/web/src/types.ts` | feature types / generated contracts | 禁止继续加入新业务大类型 |
| `internal/domain/projection.go` / `internal/app/projection.go` | `internal/experience/projection` | 复用现有投影模式，先增加窄 Query/Projector；禁止把目录项放入 Source 或 Delivery 聚合 |

## 8. 兼容代码规范

本次早期研发整改不创建兼容层、类型别名、双写或旧路径转发。只有已经对外发布且有真实消费者的契约，才可以通过独立 ADR 定义明确的 major/minor 兼容窗口；本地目录、内部 Go 包、开发数据库和未发布 API 不具备该条件。

任何临时迁移脚本必须在同一次整改中运行并删除，不能把脚本留在仓库作为下一次开发路径。

## 9. 架构门禁

CI 最终必须加入不依赖新大型框架的架构检查：

- 使用 `go list` 或小型仓库脚本验证禁止的 import 边。
- 检查 Runtime 不导入内容类型业务包。
- 检查 `studio` 和 `admin` 前端互相导入。
- 检查 `persistence` 是否只暴露 12 个窄 Repository，禁止恢复聚合 `persistence.Repository` 或全局 `Store`；应用启动通过 `application.Dependencies` 显式装配。
- 检查跨模块 DTO 是否使用版本化引用而非数据库结构。
- 检查 `CreativeAssetCatalogItem` 只位于读路径，目录更新不能直接修改 Source、Rights、Approval、Artifact 或 Delivery 状态。
- 检查旧包、旧 import、兼容别名、双写和通用 IPC 是否为零。

任何例外必须有 ADR、到期时间和对应测试，不能通过注释永久豁免。

## 10. 代码评审检查表

1. 变更属于哪个业务模块，谁拥有事实？
2. 是否创建了第二套状态或重复 Schema？
3. 跨模块依赖方向是否向内，是否可以通过引用或端口变窄？
4. 是否扩大了全局 Service、Store、types 或 utils？
5. 状态变更是否通过领域命令、幂等和版本检查？
6. 是否包含 nil、empty、invalid、conflict、timeout 和 retry 测试？
7. 兼容逻辑是否有对账、回退和删除门槛？
8. 页面是否按客户或运营表面读取正确 BFF？
9. 文档是否区分当前实现与目标能力？
10. 跨任务资产是否固定底层事实版本与摘要，而不是复制正文或只保存目录 ID？

## 11. 统一事实链的兼容边界（2026-08-23）

`internal/application/task_governance.go` 中的旧内容版本入口只负责兼容协议和 DTO 转换。视频脚本、文章和电商正式提交必须经过 `SubmissionBundle`、`SubmissionRevision` 和既有 Review 门禁；`TaskRevision` 只能由统一事实投影或历史兼容读取构造，不能成为新的审批、Artifact 或 Delivery 所有者。唯一受控例外是模型 Provider 返回的 `draft` 候选：它只与 `ModelGenerationReceipt` 原子绑定，不可批准、不可交付，选中后仍须重新经过业务 Schema 校验并提交到 Submission。普通视频脚本在既有流程已经进入 accepted 时使用显式 `automated_gate` 决定锁定批准快照，不代表工作台拥有审批事实。交付包校验通过 ApprovedSnapshot 引用回溯任务工作区，业务对象 ID 不得承担跨域事实关联职责。新增业务工作台时，应复用该入口或直接调用 Submission 命令，不得新增业务专用版本表。
