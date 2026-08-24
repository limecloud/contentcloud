# Workbench Host

这里是客户业务工作台宿主层，负责加载已发布的工作台契约、校验租户可用性、映射平台 action contract 和隔离业务 feature。

## 目标目录

```text
workbench/
├── host/             进入、切换、错误隔离和生命周期
├── registry/         已发布 manifest 与租户启用状态查询
├── contract/         阶段、状态、动作和引用的前端契约
└── renderer/         平台批准的布局与渲染原语
```

宿主从服务端读取 `plugin_id + version + digest`，不能按 `latest` 在浏览器中自行选择版本。manifest 只决定业务体验的声明式输入；状态、权限和主要动作仍以 Customer BFF 返回的 contract 为准。

`ui.layout` 是宿主选择批准 renderer 的唯一入口：`stage-canvas-context` 进入视频/镜头类 renderer，`article-editor` 进入文章编辑 renderer，`product-variants` 进入商品变体 renderer，`novel-editor` 进入 Canon/章节编辑 renderer。插件可以替换导航、阶段标签、面板、对象语言和入口动作，但不能上传新的前端 bundle；未知 layout 直接进入安全回退。`ui.panels` 只能引用同一 manifest 的阶段，并使用平台批准的 tone、target 和图标语义。

宿主可以统一提供：

- 会话、租户、权限、错误边界和可访问性上下文。
- `content_type` 到业务工作台的路由选择。
- 版本化的阶段状态、资产引用、审核、交付和 action contract。
- 未知或未批准插件的安全回退和诊断信息。

宿主不能规定所有业务使用相同的导航、画布、对象语言或密度。视频、文章、电商、连载小说等页面应位于 `../workbenches/<business>/`，通过受限的工作台定义接入。

当前 `../studio/` 仍承载迁移中的路由和页面；新业务专有组件不要继续加入该目录。

## 依赖方向

```text
platform -> workbench host -> workbenches/<business>
                      \\-> approved renderer / generated contracts
```

`workbench` 不直接访问数据库、Runtime、Provider 或 Agent Plugin 安装状态。工作台 feature 也不能绕过宿主调用这些边界；需要执行能力时只能提交平台批准的 action contract。

工作台提交的业务简报会被应用编排层规范化，写入 `WorkTask.RequestedOutput` 并参与幂等比较和 Runtime 输入摘要。当前任务页面展示的阶段、审核决定、创作结果、ApprovedSnapshot、Artifact、DeliveryPackage 和 PerformanceObservation 都来自 Customer BFF 投影；工作台不保存第二份状态。
