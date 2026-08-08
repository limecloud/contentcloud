# 当前代码证据与迁移基线

状态：`实施基线；随迁移持续更新`。

更新时间：2026-08-08。

## 1. 用途

本文把目标架构映射到当前仓库事实，防止把“文档已经定义”误认为“代码已经迁移”。对象生命周期仍以 [07-migration-and-delivery.md](./07-migration-and-delivery.md) 为准；本文只记录可搜索、可测试的实现证据和下一退场门槛。

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

这些目录不是 `dead`，不能直接删除。它们分别处于 `compat` 或 `deprecated`，必须按一个真实业务切片完成“目标端口 -> 兼容适配 -> 调用切换 -> 引用归零 -> 精确删除”。

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
| `web/src/studio` | 客户 Shell 已独立；资产详情已有独立路由和类型渲染 | Studio features | 按 `assets/tasks/connect/delivery` 归档 | 页面不再堆入全局文件 |
| `web/src/workspace` | 仍承载旧客户工作区路由 | Studio compat | 只修兼容问题，不新增客户功能 | 旧路由访问量归零 |
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

1. 资产详情迁入 `web/src/studio/assets` 后，旧页面路径先保留薄 re-export；所有路由和测试切换后再删除。
2. Studio Asset Query/Command 切换后，删除 `internal/app` 中对应类型别名和兼容方法。
3. 所有调用切换后，从 `store.Store` 移除已迁移方法；不能先删除整个 `internal/store`。
4. `.DS_Store` 属于忽略文件，不影响构建；清理它不代表架构迁移完成。
