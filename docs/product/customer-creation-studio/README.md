# 客户创作台与流水线产品层

状态：`客户创作台首切片已实现；完整运营配置、Runtime 接管与发布验证持续推进`。

更新时间：2026-08-07。

本目录定义 ContentCloud Agentic Job Runtime 之上的产品层：平台运营人员如何配置、发布和维护创作流水线，客户如何通过简单的创作页面使用这些流水线，以及 Codex、Claude Code、确定性 Worker、外部服务商和人工如何作为不同执行者参与同一项任务。

本文档描述下一阶段目标，不代表相关契约、页面、API 或运行时能力已经实现。当前能力仍以仓库代码、变更记录和 [`docs/content`](../../content/README.md) 中标记为“可用”的用户文档为准。

上位规范为 [ContentCloud 平台基线](../../foundation/README.md)，对外表达遵循[产品叙事规范](../00-product-narrative.md)。本目录只定义客户创作体验和运营产品层，不重新定义业务事实所有权、Runtime 状态机或工程边界。

## 1. 核心结论

ContentCloud 不是把所有创作操作堆在同一个工作台中的通用后台，也不是 Codex 或 Claude Code 的外壳。

客户首先理解的是：

```text
任务输入与项目参考 -> Content Work OS 创作任务 -> 创作资产、交付和后续专业工具
```

以下平台关系图用于解释三个清晰分离的表面和一个共享内核，不作为客户首图：

```text
平台运营人员
    |
    v
ContentCloud 运营控制台
配置场景、流水线、执行能力、审核、租户和发布版本
    |
    v
ContentCloud Agentic Job Runtime
保存权威状态，调度程序、智能体、服务商和人工
    |
    +-----------------------------+
    |                             |
    v                             v
客户创作台                    运营诊断台
简单完成业务目标              查看执行、费用、故障和审计
```

Codex、Claude Code 等智能体客户端只是 Runtime 可以选择的执行者之一。固定校验、转换、渲染、版本、审核、外部请求对账和交付等工作，应由 ContentCloud 服务端或受控 Worker 完成。

客户创作台还通过独立“资产”入口管理客户上传/导入的工作区资料，并复用历史生成结果。页面内清楚区分“我的资产”和“创作结果”；搜索候选、灵感、知识和来源证据仍属于当前任务输入或项目参考。详细规范见[客户资产](../creative-asset-library/README.md)。

## 2. 文档导航

| 文档 | 内容 |
| --- | --- |
| [../00-product-narrative.md](../00-product-narrative.md) | 客户十秒叙事、平台架构、工具示例和文案规则 |
| [01-product-planes-and-architecture.md](./01-product-planes-and-architecture.md) | 产品平面、系统边界、核心契约、执行路由和版本关系 |
| [02-customer-studio-requirements.md](./02-customer-studio-requirements.md) | 客户创作台的信息架构、交互状态、角色权限、首个场景和验收标准 |
| [03-inspiration-collection-stage.md](./03-inspiration-collection-stage.md) | “灵感采集”阶段的客户体验、连接器抽象、统一输出、安全门禁和失败恢复 |
| [04-execution-client-connection.md](./04-execution-client-connection.md) | 项目级执行客户端连接、当前 Codex 协议和多客户端发布门槛 |
| [05-local-workbench-browser.md](./05-local-workbench-browser.md) | 本地工作台实现事实源：Skills、stdio MCP、Go loopback Presenter、Browser Handoff、SSE、Range、Claim v2、Proposal/Apply、分发与验收 |
| [06-reference-workbench-analysis.md](./06-reference-workbench-analysis.md) | 三类公开参考实现的匿名化全维度分析、证据等级、安全审计、对比矩阵和采用/拒绝依据 |

## 3. 与现有文档的关系

本目录不替代已有路线图和用户文档：

- [`docs/foundation`](../../foundation/README.md) 是本目录必须遵守的平台架构与工程规范。
- [`docs/product/creative-asset-library`](../creative-asset-library/README.md) 定义跨任务资产积累、统一目录、固定引用和运营治理。
- [`docs/roadmap/v8`](../../roadmap/v8/README.md) 继续定义 ContentCloud Agentic Job Runtime、执行图、状态、资源、安全、故障恢复和运行诊断。
- 历史视频纵向切片仍可在迁移记录中查阅，但不再作为当前产品入口或实现事实源；当前架构以 [`docs/foundation`](../../foundation/README.md)、[`docs/roadmap/v8`](../../roadmap/v8/README.md) 和 [`docs/content`](../../content/README.md) 的事实层级为准。
- [`docs/content/internal/multi-content-expansion.md`](../../content/internal/multi-content-expansion.md) 继续定义多内容形态的类型化扩展边界。
- [`docs/content`](../../content/README.md) 只描述已经验证并可执行的客户能力，不提前发布本目录中的目标方案。

本目录补齐此前缺少的产品层：

```text
V8 Runtime：系统怎样可靠执行
本目录：运营怎样发布创作产品，客户怎样简单使用
内容包：具体视频、文章或其他内容形态怎样生产和交付
```

## 4. 下一阶段范围

第一阶段只验证一个客户可用的纵向场景和一个可复用阶段：

- 客户场景：`IP 人设营销视频`。
- 可复用阶段：`灵感采集`。
- 横向复用闭环：`我的资产或已确认结果 -> 新任务 -> 新结果继续沉淀`。
- 客户目标：不理解 SOP、JobRun、NodeRun 或执行器，也能开始任务、补充资料、确认结果并进入下一步。
- 连接目标：客户只需在项目级完成一次受控客户端连接；当前首切片使用 Codex，其他客户端按能力和连接协议逐步发布。
- 运营目标：不修改客户页面代码，也能控制场景版本、SOP 绑定、执行能力、租户开关、审核规则和回退。
- Runtime 目标：同一个业务阶段可以由搜索 API、受控爬虫、本地 Codex MCP、确定性 Worker 或人工完成，并产生统一、可追溯的结果。

第一阶段明确不建设通用低代码工作流市场，不允许客户自由上传执行代码，不要求所有 V8 动态执行能力先完成，也不把外部平台交付包误写为已经发布。

## 5. 固定原则

1. **客户页面与运营后台完全分离。** 两者使用不同的信息架构、权限、路由和读模型，不是在同一面板中隐藏几个高级菜单。
2. **Runtime 是唯一执行事实源。** 客户创作台只读取业务投影，不维护第二套任务或阶段状态。
3. **流水线按版本发布。** 一项客户任务必须固定客户体验模板、SOP 和执行计划版本，后续发布不改写进行中的任务。
4. **能力先声明，执行者后绑定。** 流水线声明需要什么能力；编译或准入阶段根据租户策略、数据位置、成本和安全要求绑定具体执行者。
5. **执行者不能自行扩大权限。** Codex、Claude Code、Worker 和服务商只取得当前节点所需的最小输入、能力和预算。
6. **候选不等于正式事实。** 搜索、爬虫和智能体结果先进入候选区，只有通过证据、权利和人工门禁后才能成为正式输入。
7. **复杂度留在运营和内核。** 客户只看到业务目标、必要输入、当前结果、阻断原因和唯一推荐动作。
8. **完成一次，复用下一次。** 人物原型、剧本、分镜、图片和视频结果进入统一资产目录；任务输入和项目参考保留在其所属任务范围，新任务引用固定事实版本，不复制正文。

## 6. 决策记录

| 日期 | 决策 | 原因 |
| --- | --- | --- |
| 2026-08-05 | 客户创作台与 ContentCloud 运营控制台作为两个独立产品面 | 客户不应理解平台配置、运行时和治理对象 |
| 2026-08-05 | ContentCloud Agentic Job Runtime 作为共享执行内核 | 多种创作流水线需要复用状态、调度、审核、费用和审计能力 |
| 2026-08-05 | Codex、Claude Code 只是执行者之一 | 确定性和平台治理工作不适合全部交给智能体客户端 |
| 2026-08-05 | 流水线能力与具体执行者分离 | 同一阶段需要支持平台 Worker、本地智能体、外部服务商和人工 |
| 2026-08-05 | 以灵感采集作为首个可复用阶段 | 它能同时验证多连接器、来源追溯、本地与云端执行及人工采纳 |
| 2026-08-05 | 增加客户创作资产库 | 把每次创作沉淀为下一次可直接使用的受治理输入 |
| 2026-08-07 | 创作结果以五类生成结果为中心 | 输入和治理对象留在任务/运营面，结果类型与确认状态保持独立 |
| 2026-08-07 | 客户资产入口组合我的资产与创作结果 | 客户需要管理上传/导入资料，但两个视图不能合并成超级 Asset 写模型 |
| 2026-08-05 | 客户叙事图与平台架构图分离 | 先解释客户输入和结果，再按需要展开 Runtime 与执行者边界 |
| 2026-08-14 | 本地控制面与 Browser 呈现面分离 | Skills + stdio MCP 保持可移植控制面；Go CLI 按需启动同进程 loopback Presenter，本地与云端共用 handoff，所有写入进入同一 Workspace Kernel |
