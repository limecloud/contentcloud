# ContentCloud V6：Content Work OS

状态：`需求基线、独立原型与 React 官网 Beta 已形成，等待产品、工程和真实租户共同评审`。

更新时间：2026-07-31。

V6 的目标不是再增加一个内容生成入口，而是把 ContentCloud 从“本地优先的内容生产与云端治理系统”升级为 **Content Work OS**：一个把本地 Agent、云端控制面、租户能力、Content Pack、审核和多渠道交付组织在同一条可追溯生产链上的工作操作系统。

V6 继承 V3、V4、V5 的领域事实和安全边界，不创建新的数据平面，也不把服务端变成 LLM 代理。V5 已经提供了可复用的 Daemon、租约、进度、恢复和版本治理底座；V6 将这些底座从“Automation 细节”提升为用户看得懂、平台管得住、Content Pack 可复用的产品能力。

## 1. V6 要解决的问题

当前用户能够完成视频剧本和（按租户开通的）微信公众号文章，但仍需要自己理解以下概念之间的关系：

- 哪个 Agent 客户端正在运行，以及它连接了哪个 Workspace。
- 当前租户开通了哪些内容形态，为什么某个入口不可用。
- Skill、MCP、Content Pack、Daemon 和云端审核分别负责什么。
- 本地产物如何变成可审计的 Submission，又如何变成可交付的批准快照。
- Automation 运行失败时，是租约、设备、内容能力、Skill 还是业务门禁出了问题。

V6 必须把这些关系变成产品的一等对象和可诊断状态，而不是隐藏在命令、日志和开发文档中。

## 2. 产品主张

### 2.1 对外主张

> 一个受治理的内容工作区，让本地 Agent、团队审核与多渠道交付在同一条可追溯生产链中协作。

### 2.2 产品名称

- 中文：**Content Work OS**（内容工作操作系统）。
- 英文：**Content Work OS**。
- 解释性副标题：`Local agents. Governed production. Traceable delivery.`

“OS”描述的是统一工作面和运行时抽象，不暗示 ContentCloud 提供操作系统级沙箱、模型或外部平台账号。

### 2.3 记忆点

用户第一次看到产品后，应记住：

> 我可以在熟悉的本地 Agent 里创作，但团队、租户和合规边界始终可见。

## 3. 系统模型

```text
Public Website
      |
Cloud Control Plane
  Tenant / Workspace / Capability / Registry / Audit
      |
Signed Environment Manifest
      |
Local Workspace
      |
Agent Runtime: Codex / Claude Code / future clients
      |
Content Pack: video_script / wechat_article / future packs
      |
Candidate -> Lint -> Publish -> Review -> Approved Snapshot -> Delivery
```

V6 的唯一事实源仍然是：

1. Tenant Content Capability：租户是否开通某种内容形态。
2. 签名 Environment Manifest：本地运行时允许使用的客户端、Pack、Capability 和策略投影。
3. ContentBatch、ContentItem/ArticleItem、SubmissionRevision、ApprovedSnapshot：内容生产与审批事实。
4. TaskContract、RunAttempt、run_progress_events：Automation 执行事实。

官网、Web 工作台、CLI、MCP、Skill 和文档都只能投影这些事实，不能各自维护一份“支持列表”。

## 4. 内容形态基线

| 内容形态 | 平台状态 | 租户默认状态 | V6 处理方式 |
| --- | --- | --- | --- |
| `video_script` | 可用 | 默认启用 | 继续作为默认 Content Pack，完整继承 V5 视频闭环 |
| `wechat_article` | 可用 | 默认关闭 | 保持租户显式开通，继续使用 ArticleBrief/ArticleItem 和本地交付包 |
| Newsletter | 未实现 | 不可用 | 只进入 Registry 的 planned，不进入生产入口 |
| 社交媒体帖子 | 未实现 | 不可用 | 只保留产品模型讨论，不承诺执行流程 |
| 播客/直播脚本 | 未实现 | 不可用 | 等待独立 Content Pack 评审 |

## 5. V6 交付范围

### 必须交付

- Content Work OS 对外产品定位、信息架构和官网。
- Cloud Control Plane 的 Runtime/Workspace/Content Pack 状态视图。
- 租户能力矩阵：`enabled`、`disabled`、`unavailable`、`misconfigured` 四种可区分状态。
- Capability/Manifest/Pack 的诊断链路，显示版本、digest、来源和最近健康状态。
- 统一的 Work OS 首页：从项目、待审核内容、运行中的 Automation 和需要处理的异常进入工作流。
- 运行时可观测性：Daemon 状态、当前 Workspace、最近 Attempt、进度游标、待重报和 dead-letter。
- 视频剧本和微信公众号文章的统一导航与内容类型安全渲染。
- 官网原型和未来生产官网的内容、技术、无障碍与性能验收标准。
- v5 现有失败项在进入生产发布前完成固定 Plugin、内置 Registry/Profile 和 Bootstrap 兼容性收敛。

### 明确不在 V6

- 云端代替用户登录 Seedance、抖音、微信公众号后台或其他外部平台。
- 服务端调用或代理 LLM，保存用户模型密钥，执行客户上传的任意代码。
- 把所有租户一次性打开所有内容形态。
- 以“Agent 会自动完成一切”替代审批、权利、事实和交付门禁。
- 为了官网叙事重写高频工作台，或把后台变成营销型 Hero 页面。

## 6. 里程碑

| 里程碑 | 目标 | 退出条件 |
| --- | --- | --- |
| M6-0 定位冻结 | 产品命名、对象层级、边界和状态语义完成评审 | 产品、工程、内容运营和安全共同签字 |
| M6-1 控制面可见 | Workspace、Runtime、Pack、租户 Capability 可查看和诊断 | Web 与 CLI 显示同一 Manifest 投影，正反例测试通过 |
| M6-2 Work OS 首页 | 项目、审批、运行、异常和下一步动作汇总 | 高优先工作不超过两次点击可达，权限和租户隔离通过 |
| M6-3 Content Pack 统一入口 | 视频与公众号使用统一路由和 Pack 元数据 | 未开通公众号的租户不可创建、提交或交付文章 |
| M6-4 官网 Beta | 官网可独立部署，内容来自稳定产品事实 | 移动端、键盘、性能、链接和文案验收通过 |
| M6-5 生产验收 | v5 回归、真实设备、真实租户和官网发布 | `go test -race ./...`、Web build、Registry/Plugin 校验和 Golden Journey 全通过 |

## 7. 文档导航

| 文件 | 内容 |
| --- | --- |
| [01-product-and-platform.md](./01-product-and-platform.md) | Content Work OS 定位、用户、对象和控制面要求 |
| [02-runtime-and-content-packs.md](./02-runtime-and-content-packs.md) | Runtime、租户能力、Manifest、Content Pack 和治理要求 |
| [03-official-website.md](./03-official-website.md) | 官网目标、页面结构、文案、视觉和技术验收 |
| [04-migration-and-acceptance.md](./04-migration-and-acceptance.md) | 从 V5 迁移、兼容策略、风险和验收矩阵 |
| [PLAN.md](./PLAN.md) | V6 实施台账和工作包 |
| [prototype.html](./prototype.html) | Content Work OS 官网原型，可直接在浏览器打开 |

## 8. 当前风险

- 当前 `go test ./...`、`go test -race ./...` 和 `go vet ./...` 已通过；真实设备矩阵、真实租户验收和发布治理仍是 M6-5 前置条件。
- 官网如果静态写死客户端或 Content Pack 列表，会与 Registry 漂移；Beta 先使用受控内容源，生产版必须接入发布流水线校验。
- Daemon 的全权限模式只适用于服务端签发租约的 Automation Attempt，官网和工作台不能把它宣传成无条件的本机自动执行。

## 9. 决策记录

| 日期 | 决策 | 原因 |
| --- | --- | --- |
| 2026-07-31 | V6 对外升级为 Content Work OS | V5 已具备 Runtime、Workspace、Pack 和治理链路，需要统一产品心智 |
| 2026-07-31 | 官网采用明亮自然光首屏，工作台保持冷白高密度 | 使用创作空间与纸张折页意象，用真实工作面和运行状态承接产品可信度 |
| 2026-07-31 | `video_script` 默认启用，`wechat_article` 继续租户显式开通 | 保持当前能力和租户隔离事实，不用营销页面越权开关 |
| 2026-07-31 | 官网原型与业务 Web 工作台分开 | 官网负责定位与信任，工作台负责高频生产效率 |
| 2026-07-31 | `/` 承载公共官网，`/workspace` 承载登录后工作台 | 保留项目深链和 ConnectSession 的项目上下文，避免公共入口与生产入口混用 |
| 2026-07-31 | 首页背景跨 Hero 与产品段落连续呈现 | 避免 Hero 结束时硬切白色，并保证常用桌面和移动视口露出下一段内容 |
