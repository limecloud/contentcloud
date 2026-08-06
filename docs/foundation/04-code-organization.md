# 代码组织、模块边界与依赖规范

状态：`目标规范；不要求一次性物理搬迁`。

更新时间：2026-08-05。

## 1. 当前结构诊断

当前 Go 代码按技术层横向组织：

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

本轮不做纯物理搬家。先建立逻辑模块、窄端口和依赖检查，再逐模块迁移。

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

目录表示目标所有权，迁移按 [07-migration-and-delivery.md](./07-migration-and-delivery.md) 分阶段完成。

```text
internal/
├── identity/                 用户、租户、成员、会话
├── workspace/                项目、品牌上下文、设备与 Workspace 绑定
├── source/                   来源、证据、知识和权利
├── catalog/                  Experience、SOP、Gate、Capability、发布
├── work/                     WorkTask 和客户业务生命周期
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
├── integration/              Agent、Connector、Provider 契约
└── openapi/                  按表面拆分或生成的 OpenAPI

web/src/
├── studio/                   客户创作台 Shell、routes、features
├── admin/                    平台运营和 Runtime Explorer
├── public/                   官网、登录、公开审核和文档
├── shared/                   纯 UI 原语、品牌、格式化和 API 基础设施
└── app/                      Router 和根级 Provider 装配
```

目录不是立即创建的空壳。只有迁移一个真实模块时才建立对应目录。

## 4. 模块内部结构

首阶段避免为每个模块复制复杂分层目录。一个模块默认使用简单 Go 文件：

```text
internal/work/
├── model.go          聚合和值对象
├── command.go        写用例
├── query.go          读用例
├── repository.go     该模块需要的窄端口
├── errors.go         稳定领域错误
├── projection.go     由本模块拥有的读模型
└── *_test.go
```

只有模块体量和团队所有权证明需要时，才在模块内部增加子包。禁止预先建立空的 `domain/application/infrastructure` 三层模板。

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
7. 客户资产库和运营资产治理可以共享生成的 Schema 与无业务 UI 原语，但不能共享页面 DTO、权限判断或命令实现。

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

## 7. 当前目录迁移映射

| 当前路径 | 目标 | 策略 |
| --- | --- | --- |
| `internal/domain/*.go` | 对应业务模块 `model.go` | 按聚合迁移；迁移前保留类型别名或 Adapter，禁止复制行为 |
| `internal/app/*.go` | 模块 command/query 或应用协调器 | 先切窄接口和测试，再移动实现 |
| `internal/store/store.go` | 各模块 `repository.go` | 新代码禁止扩大全局 Store；逐模块拆接口 |
| `internal/store/postgres` | `internal/persistence/postgres` | 实现模块端口，迁移期可保留适配器 |
| `internal/store/memory` | `internal/persistence/memory` | 保持与 PostgreSQL 相同契约测试 |
| `internal/httpapi` | `internal/transport/http` | 按 studio/admin/agent/public 路由拆 DTO 和 Handler |
| `internal/agentadapter` | `internal/integration/agent` | 保留 CLI 兼容适配器，补 `AgentHarnessAdapter`、FakeHarness 和 Runtime Node 契约 |
| `internal/environment` | `catalog` + `integration` + `runtime binding` | 先拆清配置、信任和运行绑定语义 |
| `internal/mediapipeline` | `delivery` + `integration/provider` | 业务状态归 delivery，SDK 调用归 provider |
| `internal/worker` | Runtime worker / 模块 worker | 节点执行与业务处理按能力拆分 |
| `web/src/workspace` | `web/src/studio` | 以新 Shell 和业务投影渐进替换，不直接重命名全部页面 |
| `web/src/admin` | 保留并按 operations/runtime explorer 分区 | 复用现有独立 Shell 和权限 |
| `web/src/types.ts` | feature types / generated contracts | 禁止继续加入新业务大类型 |
| `internal/domain/projection.go` / `internal/app/projection.go` | `internal/experience/projection` | 复用现有投影模式，先增加窄 Query/Projector；禁止把目录项放入 Source 或 Delivery 聚合 |

## 8. 兼容代码规范

每个兼容 Adapter 必须包含：

```text
CompatRecord
├── legacy_source
├── target_contract
├── owner
├── introduced_in
├── metrics
├── read_compare_rule
├── rollback_rule
├── removal_gate
└── latest_removal_phase
```

- 兼容层只做语义映射，不承载新业务规则。
- 新功能不得依赖已标记 `Deprecate` 的接口。
- 双写只用于有限迁移窗口，必须有摘要对账和失败告警。
- 达到退场门槛后优先删除兼容路径，不长期保留“保险代码”。

## 9. 架构门禁

CI 最终必须加入不依赖新大型框架的架构检查：

- 使用 `go list` 或小型仓库脚本验证禁止的 import 边。
- 检查 Runtime 不导入内容类型业务包。
- 检查 `studio` 和 `admin` 前端互相导入。
- 检查新 Repository 是否继续扩大全局 `store.Store`。
- 检查跨模块 DTO 是否使用版本化引用而非数据库结构。
- 检查 `CreativeAssetCatalogItem` 只位于读路径，目录更新不能直接修改 Source、Rights、Approval、Artifact 或 Delivery 状态。
- 检查 `Compat` 条目是否有退场阶段和所有者。

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
