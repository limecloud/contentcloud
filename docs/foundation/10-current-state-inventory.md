# 当前代码证据与迁移基线

状态：`实施基线；随迁移持续更新`。

更新时间：2026-08-08。

## 1. 用途

本文把目标架构映射到当前仓库事实，防止把“文档已经定义”误认为“代码已经迁移”。对象生命周期仍以 [07-migration-and-delivery.md](./07-migration-and-delivery.md) 为准；本文只记录可搜索、可测试的实现证据和下一退场门槛。

运营后台的目标页面名称、中文菜单和旧入口处置以
[《运营后台页面蓝图与中文语言规范》](../product/operations-control-plane/04-page-blueprints-and-language.md) 为准；本表中的“目标所有者”不代表代码已经迁移完成。

## 2. 当前物理结构

当前主要生产依赖仍位于：

```text
internal/domain
internal/app
internal/store
internal/httpapi
internal/environment
internal/mediapipeline
```

这些目录中仍有事实服务或 Agent 适配器，但已删除的客户 Web Shell 不再作为 `compat` 保留。服务端 BFF 只有在 CLI、Agent 或本地工作区仍有真实调用时才保留，并且不得重新承载客户页面。

第一个目标模块已经建立：

```text
internal/experience/studio
```

它当前拥有客户资产表面的 transport-neutral DTO：工作区资料投影、创作结果投影、结果详情、媒体引用和下载引用。`internal/app` 暂时通过 Go 类型别名保持原 API 与测试兼容，不再拥有这些结构定义。

## 3. 证据矩阵

| 当前对象或路径 | 当前证据 | 目标所有者 | 下一动作 | 退场门槛 |
| --- | --- | --- | --- | --- |
| `internal/experience/studio` | 已拥有客户资产 DTO；禁止反向导入 `app/store/httpapi` | Experience Studio | 增加窄 Asset Query/Command 端口 | 查询和命令不再经全局 `app.Service` |
| `internal/app/customer_workspace_assets.go` | 仍编排资料查询、上传和任务附加 | Workspace + Experience Studio | 先拆 Query，再拆 Command | 文件只剩薄兼容调用且指标归零 |
| `internal/app/customer_studio.go` | 仍承载 Bootstrap、任务、结果、交付等多个客户用例 | Work + Experience Studio | 按客户 use case 拆分，不整体搬家 | 每个用例拥有窄端口和契约测试 |
| `internal/httpapi/customer_studio_handlers.go` | Studio 路由已独立，但 Handler 仍依赖全局 Service | `transport/http/studio` | 改依赖 Studio Facade 接口 | 旧 handler 无注册和调用 |
| `internal/store/store.go` | 226 个方法的全局接口 | 各业务模块 | 只允许减少，不允许新增 | 所有方法迁入模块端口 |
| `web/src/studio/assets` | 资产独立路由、页面编排、格式化和类型渲染已迁入 feature | Studio Assets | 后续新增资产能力只在 feature 内演进 | 客户资产不再依赖旧平面页面 |
| `web/src/workspace` | 已退场；共享规范化器和工作面样式已迁入 `web/src/admin` 与 `web/src/styles` | 无 | 新客户功能只进入 `web/src/studio` | 不得重新创建该目录或注册旧客户路由 |
| `web/src/admin/AdminWorkOSPage` | 仍把 SOP、Gate、Capability 和 Environment 放在同一管理页面 | Operations compat | O1 先接入独立总览，O2-O4 再拆创作产品、能力与绑定规则 | 新工作面覆盖旧入口，旧页面无新增调用 |
| `web/src/admin/AdminContext` | 使用单一 `work-os` 请求拼装粗粒度概览 | Operations projection compat | 增加运营专用总览查询和待办队列 | 新总览数据可追溯、带更新时间和延迟 |
| `web/src/admin` Runtime 页面 | 可查看部分执行信息，但客户任务、体验版本、绑定和资产治理关联不完整 | Operations Runtime Explorer | O5 产品化运行诊断和安全恢复动作 | 常见故障无需查库，操作全有审计 |
| `web/src/admin/AdminShell.tsx` | 当前标题和导航仍以“运行基础设施、运行配置、治理”组织 | Operations UI compat | O1 改为“平台运营后台”，O2-O6 按页面蓝图拆分中文菜单 | 新导航覆盖旧导航，旧标题和菜单无入口 |
| `web/src/admin/WorkOSConfigPanels.tsx` | 环境、SOP 和本地能力在同一配置工作面 | Operations config compat | O2-O4 分别迁入创作产品、能力运营、执行端和选择规则 | 旧配置面不再承载新写操作 |
| 本地执行端接入 | 当前平台能力与环境配置存在混合表达，缺少“登记、客户确认、健康检查、暂停/恢复”的独立运营流程 | Operations executor binding | O3 增加执行端接入和客户侧简单连接状态 | 未经客户确认不能扩大本地资料范围；离线有可解释恢复路径 |
| 客户创作结果目录投影 | 已有客户结果展示和固定引用基础，运营侧权利、重复和投影治理尚未独立 | Operations Asset Governance | O6 增加失效影响、重复候选和投影重建 | 失效结果不能产生新的不安全引用 |
| `internal/runtime` | V8 内核已存在，仍使用 `internal/domain` 兼容值 | Runtime | 逐步替换为稳定引用和端口 | 不导入具体业务模块或内容类型 |

## 4. 已启用门禁

`pnpm architecture` 当前检查：

- `web/src/studio` 与 `web/src/admin` 不得互相导入。
- Runtime 不得导入 Identity、Workspace、Source、Catalog、Work、Review、Delivery、Performance 或 Experience 具体业务包。
- `internal/experience` 不得反向导入旧 `app`、`store` 或 `httpapi`。
- `store.Store` 方法数不得超过当前 226 个基线，只能迁出和减少。

门禁已接入根 `Makefile check` 和 GitHub CI。`internal/domain` 仍是 Runtime 的显式兼容依赖，本阶段不伪装为已经完成迁移。

## 5. 客户资产语义

客户“资产”是统一入口，不是统一写模型：

```text
我的资产        -> WorkspaceMaterialProjection -> 上传、导入、处理、预览
创作结果        -> CreativeResultProjection    -> 人物原型、剧本、分镜、图片、视频
创作结果详情    -> 固定版本只读查看             -> 回到来源任务提出修改并产生新版本
加入新的创作    -> 固定底层事实引用与摘要         -> 不复制正文，不覆盖旧版本
```

因此，资产详情可以有文档阅读器、媒体播放器、表格查看器和结构化创作结果工作面，但不能提供直接覆盖固定版本的伪编辑器。编辑能力属于来源任务的修订命令，成功后必须生成新的可追踪版本。

## 6. 精确清理候选

只有满足引用归零和兼容门槛后才删除：

1. `web/src/studio/StudioAssetDetailPage.tsx` 及旧测试入口已在路由和测试切换后删除；新实现只位于 `web/src/studio/assets`。
2. Studio Asset Query/Command 切换后，删除 `internal/app` 中对应类型别名和兼容方法。
3. 所有调用切换后，从 `store.Store` 移除已迁移方法；不能先删除整个 `internal/store`。
4. `.DS_Store` 属于忽略文件，不影响构建；清理它不代表架构迁移完成。
