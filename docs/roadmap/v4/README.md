# ContentCloud V4 Agent-Native 双向工作台方案

状态：`核心决策已确认，进入 V3/V4 联合实施`。

更新时间：2026-07-27。

V4 不是新的业务领域版本，也不替换 V3 的 Workspace、Submission、Decision、ApprovedSnapshot 或 ProjectProjection。V4 解决的是一个更窄但直接影响产品体验的问题：如何让用户在 Codex 中持续本地创作时，同时把 ContentCloud Web 的项目投影、证据、审核和下一动作作为可见、可操作的并排工作台打开。

## 1. 问题定义

V3 已经设计了从 Web 到 Codex 的入口：

```text
ContentCloud Web
  -> 复制连接 Prompt / 在 Codex 继续
  -> Codex 新对话预填 Prompt
  -> 用户选择本机 Workspace
  -> workspace_context 验证 project_id
  -> 本地 Run / Handoff / 创作
```

但从 Codex 回到 Web 的路径仍主要依赖用户手工切换窗口、寻找项目和定位对象：

```text
Codex 完成本地检查点
  -> publish 得到 SubmissionRevision
  -> 用户手工打开 ContentCloud
  -> 手工找到项目
  -> 手工找到本次 Revision 或待决策对象
```

V4 补齐反方向入口，并把云端 Web 变成与 Codex 并排工作的治理工作台：

```text
Codex Scene Plugin
  -> 返回精确 ContentCloud Web resource_link
  -> Codex 调用宿主 Browser 导航
  -> 右侧打开对应 Project / Revision / Assignment / Subject
  -> 用户在 Web 完成证据查看、评论、任务和人工决定
```

其中 Browser 导航链路只负责定位页面，不传输未发布业务正文。打开后的 Web 不是只读截图：它可以执行 V3 允许的云端治理命令，但不能替代 publish/pull、编辑本地候选或改变人工决定边界。

## 2. 核心结论

V4 采用“Browser 作为云端治理工作台，Workspace 作为本地创作事实源”的模式：

```text
┌──────────────────────── Codex / ChatGPT Desktop ────────────────────────┐
│                                                                         │
│  左侧：Codex 对话                       右侧：内置 Browser               │
│  ─────────────────                     ─────────────────────────         │
│  Workspace 文件                         ContentCloud Web                 │
│  LocalRunContext                        ProjectProjection                │
│  RunClaim / HandoffRecord               SubmissionRevision              │
│  lint / eligible / blocked              Evidence / Decision             │
│  候选知识和内容                          Assignment / ApprovedSnapshot    │
│                                                                         │
└───────────────┬───────────────────────────────────────┬─────────────────┘
                │                                       │
                └──── CLI Gateway: explicit publish/pull ┘
```

Browser 不是第四个数据平面。V3 的三个平面和 CLI Gateway 保持不变。

最终推荐不是照搬 Slides 的全本地应用，也不是照搬 ChatCut 的全云端创作，而是保留 ContentCloud 自己的混合边界：产品体验学习 ChatCut 的 Plugin、精确深链和持续可见 Browser；数据架构坚持本地候选、云端正式事实以及显式 publish/pull。除非真实使用数据触发 [Local Preview 的评估条件](./05-local-vs-cloud-decision.md#6-是否保留本地页面的可能性)，V4 不建设第二套本地 Web 运行时。

## 3. V4 决策基线

| ID | 决策 | 原因 |
| --- | --- | --- |
| D4-01 | V4 继承 V3 全部业务不变量 | 避免以体验升级为由重开领域模型 |
| D4-02 | Browser 打开可操作的云端治理工作台 | Web 可以执行授权的治理命令，但不能成为本地文件编辑器 |
| D4-03 | 使用一个通用 `contentcloud_open_project_view` MCP Tool | 避免为每个页面复制工具与权限逻辑 |
| D4-04 | MCP 返回标准 `resource_link`；`browserHandoff` 只作可选提示 | 标准链接负责兼容，私有提示不能成为正确性前提 |
| D4-05 | MCP Server 不直接启动或控制 Browser | Browser 是宿主能力，应由 Skill/Agent 调用 |
| D4-06 | 页面定位使用 project/object/revision ID 与 digest | URL、文件路径和对话 ID 都不是业务主键 |
| D4-07 | Web 写动作仍只能生成 Assignment、Comment、Decision、Context Revision 或 Automation Plan | Browser 不能绕过 V3 命令边界 |
| D4-08 | CLI/IDE 无 Browser 时返回可点击链接并明确降级 | 不伪造跨宿主能力一致性 |
| D4-09 | 首版不引入 ContentCloud 本地 Next.js 服务或 Electron 壳 | 采用云端治理 Web + 本地轻伴随层；是否增加只读 Local Preview 由真实需求另行立项 |
| D4-10 | V4 文档版本不触发业务 Schema 4.0 | 现有 V3 Schema 未发生语义破坏 |
| D4-11 | 提供官方 `/codex` 人类/Agent 双模接入入口 | 降低 Plugin 安装、登录、bootstrap 和新对话交接成本 |

## 4. 与 V3 的关系

V4 是 V3 工作项的横向验收增强，而不是排在 V3 完成之后的独立重写：

V3 当前仍在开发中，下表描述的是联合实施关系，不代表 V3 契约已经冻结。V3 领域模型、Page Contract 或 Web 路由发生变化时，必须在同一个变更中检查 V4 的 view/focus、MCP 链接和 Browser 验收；V4 也不能绕过 V3 评审，单方面固化尚未确认的页面或对象模型。

| V3 工作流 | V4 增量 |
| --- | --- |
| W2 CLI/MCP/Plugin | 增加通用 Web 视图导航 Tool、resource link 和 Skill 编排 |
| W3 Server | ProjectProjection 的下一动作提供类型化导航目标，不提供任意 URL |
| W4 Web | 所有原型页面拥有稳定深链、焦点定位和鉴权恢复 |
| W5 真实宿主 | 增加 Desktop Browser 打开、降级和安全边界验收 |

V4 当前不主动改写 V3 已确认的下列责任边界；它们是否最终冻结、何时实现，仍以开发中的 V3 评审和 `../v3/PLAN.md` 为准：

- `.contentcloud/workspace.yaml` 与 Environment Lock。
- LocalRunContext、RunClaim 和本地 HandoffRecord。
- SubmissionBundle、SubmissionRevision、Decision 和 ApprovedSnapshot。
- WorkAssignment、ExecutionBundle 和 Automation Attempt。
- SourceDisclosure、显式 publish/pull 和人工决定。

## 5. 术语边界

V4 必须区分两个容易混淆的概念：

| 术语 | 含义 | 是否持久化 |
| --- | --- | --- |
| `HandoffRecord` | 不同 Codex 对话之间接管同一 LocalRun 的业务交接 | 是，保存在 Workspace |
| `browserHandoff` | MCP 返回给 Agent 的临时导航提示 | 否，不进入业务文件或服务端事实 |

实现与产品文案中统一把后者称为“Browser 导航”，不创建 `BrowserHandoff` 领域对象。

## 6. 文档导航

| 文档 | 内容 |
| --- | --- |
| [01-architecture.md](./01-architecture.md) | 产品边界、组件职责、数据流与失败恢复 |
| [02-contracts-and-security.md](./02-contracts-and-security.md) | MCP Tool、resource link、页面深链、鉴权与安全契约 |
| [03-product-workflows.md](./03-product-workflows.md) | Web 到 Codex、Codex 到 Web、审核和 Assignment 流程 |
| [04-delivery-and-acceptance.md](./04-delivery-and-acceptance.md) | 实施顺序、测试矩阵、验收和明确不做的范围 |
| [05-local-vs-cloud-decision.md](./05-local-vs-cloud-decision.md) | Slides 本地模式对比、服务端弊端与推荐部署拓扑 |
| [06-chatcut-benchmark.md](./06-chatcut-benchmark.md) | ChatCut 远程 MCP、云端编辑器、双模接入与 V4 采纳边界 |
| [prototype.html](./prototype.html) | Codex 左侧 + 云端 ContentCloud Browser 右侧的交互原型 |
| [PLAN.md](./PLAN.md) | V4 唯一进度台账 |

## 7. 完成定义

V4 完成必须同时满足：

1. 用户在 Codex 中说“打开项目总览”时，Desktop Browser 打开正确租户和项目。
2. publish 成功后可以直接打开本次不可变 SubmissionRevision，而不是项目首页。
3. blocked 内容可以打开对应阻断对象和证据，但不能从 Browser 绕过门禁。
4. Web 中“在 Codex 继续”只预填不含路径和秘密的新对话 Prompt；用户选择本机 Workspace 后，由 `workspace_context` 验证 `project_id` 并决定本地任务。
5. Browser 不可用、未安装、未登录、无权限、对象过期时都有明确降级与恢复路径。
6. URL 和 MCP 结果不包含 token、本机绝对路径、transcript、隐藏推理或未授权原件。
7. 未 publish 的本地正文不会因为 Browser 打开而进入服务端或 Web。
8. 所有写动作继续受 V3 的人工确认、CSRF、tenant boundary、revision digest 和审计约束。
9. ChatGPT Desktop 真实安装环境完成端到端验收；不能只以单元测试或宣传截图声明支持。
10. `/codex` 在浏览器中提供人类接入页，在 Agent 请求中提供版本化、无秘密的安装与 bootstrap 指南。

## 8. 参考事实

- Codex Slides 审计基线：`nexu-io/codex-slides`，本地审计 commit `dbc2a59`。
- ChatCut Agent Plugin 审计基线：`ChatCut-Inc/agent-plugin`，公开审计 commit `4748eef373ff7dbe64e959ae44f3351d468564ba`。
- OpenAI Browser 文档：<https://learn.chatgpt.com/docs/browser?surface=app>。
- OpenAI Plugin 文档：<https://developers.openai.com/plugins/>。
- V3 架构基线：[../v3/README.md](../v3/README.md)。
- V3 Web 边界：[../v3/05-web-console.md](../v3/05-web-console.md)。
