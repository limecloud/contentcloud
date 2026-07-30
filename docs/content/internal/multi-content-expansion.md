# ContentCloud 多内容形态扩展方案

状态：`方案草案，待产品与工程评审`。

更新时间：2026-07-29。

本文定义 ContentCloud 从“以营销视频为主的内容生产系统”扩展为“支持多种内容形态的本地优先创作与云端治理平台”的目标架构，并以微信公众号文章作为第一个非视频纵向切片。本文是架构与实施建议，不代表相关契约、命令、Skill、Web 页面或平台集成已经实现。

本文不替代现有 V3、V4、V5 路线图：

- V3 继续定义 Workspace、知识治理、LocalRun、SubmissionRevision、ApprovedSnapshot 和交付主链。
- V4 继续定义 Codex 与 Browser/Web 的双向工作表面及显式 publish/pull 边界。
- V5 继续完成抖音电商视频、分镜、Seedance 交付和结果归因纵向切片。
- 本文在这些稳定边界之上，定义多内容形态的扩展方式和公众号文章纵向切片。

## 1. 执行摘要

ContentCloud 可以扩展到公众号文章、Newsletter、社交媒体帖子、播客脚本、直播话术等更多内容形态，但不能通过“再增加一个 Prompt 或 Skill”完成。当前系统的治理外壳已经较通用，真正限制扩展的是 Brief、ContentItem、本地 lint、交付渲染和 Web 展现仍将内容等同于视频剧本。

目标不是复制多套 ContentCloud，而是形成一套稳定内核和可安装的内容能力包：

```text
ContentCloud Core
  Source / Evidence / Knowledge / Rights
  LocalRun / Handoff / Revision / Decision / Snapshot
  Delivery / Binding / Observation / Learning
                       |
                       v
                 Intent Profile
                       |
          +------------+-------------+
          |                          |
          v                          v
 Video Production Pack       WeChat Article Pack
 Video Brief/Spec             Article Brief/Spec
 Video validators             Article validators
 Storyboard/Seedance          WeChat renderer/package
          |                          |
          +------------+-------------+
                       v
            Channel / Provider Adapter
```

核心决策如下：

1. 保留 Workspace、知识、审批、版本、交接和结果治理主链，不为公众号复制服务端或 Workspace。
2. 继续复用 `brief`、`content_batch`、`delivery` 和 `result` Submission 轨道，不新增 `wechat_article` Submission 类型。
3. 将 `ContentItem` 收缩为通用治理信封，具体内容由版本化、受信任的类型化 payload 描述。
4. 将 Brief 拆为跨内容公共约束和内容类型专用 production spec，避免视频字段污染文章。
5. Skill 负责方法和 Agent 编排；Schema、lint、引用、权利、摘要和状态迁移由确定性工具负责。
6. 内容能力通过 task-scoped Skill Pack 提供；需要第三方登录或 API 的能力通过独立 Provider Adapter 提供。
7. 公众号首版只生成经过审核、可复制发布的交付包，用户在公众号后台确认并发布；自动发布另行设计授权边界。
8. 先完成公众号文章最小纵向切片，再依据真实需求提炼可复用抽象，不预先实现所有内容类型。

## 2. 背景与现状

### 2.1 已具备的通用基础

当前系统已经存在以下可以跨内容形态复用的能力：

| 能力 | 当前所有者 | 多内容形态中的定位 |
| --- | --- | --- |
| Workspace 与项目绑定 | V3 Workspace / CLI / MCP | 所有内容任务共享的本地事实边界 |
| Source 与 Evidence | 本地 Workspace + 服务端投影 | 文章、视频和其他内容的统一证据来源 |
| Fact、Claim、Asset、Rights | 知识治理 | 统一的真实性、可表达性和权利门禁 |
| KnowledgePack | 本地知识包 | 为不同 Intent 查询 eligible/blocked 输入 |
| LocalRun、RunClaim、CAS、Handoff | 本地执行系统 | 跨对话、多人和中断恢复 |
| SubmissionRevision、Decision、ApprovedSnapshot | 服务端治理 | 不可变提交、人工审批和正式事实 |
| Artifact、DeliveryPackage | 交付系统 | 保存内容交付物的摘要、格式和血缘 |
| PerformanceObservation、Learning | 结果系统 | 不同渠道的结果导入和人工采纳 |
| Environment Manifest、Skill Pack、Provider MCP Pack | 创作环境 | 按项目和任务提供版本锁定能力 |

`ContentBatch 3.0` 本身已经接近通用批次模型，只要求 Intent、Brief、知识快照、ContentItem 引用、状态和检查，见 [`contracts/content-batch-3.0.schema.json`](../../../contracts/content-batch-3.0.schema.json)。V3 服务端设计也已把 `content_batch` 定义为 Script、图文和直播话术的共同审核轨，见 [`docs/roadmap/v3/03-server-domain-and-sync.md`](../../roadmap/v3/03-server-domain-and-sync.md)。

### 2.2 当前视频耦合

以下实现仍然是视频专用的：

- [`contracts/brief-3.0.schema.json`](../../../contracts/brief-3.0.schema.json) 强制要求 `duration_min_ms`、`duration_max_ms`、`aspect_ratio` 和 `visualization_plan_ids`。
- [`contracts/content-item-3.0.schema.json`](../../../contracts/content-item-3.0.schema.json) 强制要求 `duration_ms`、`aspect_ratio`、`cover`、`shots`、首尾帧、口播、字幕、相机运动和连续性。
- `review_ready` ContentItem 必须至少包含一个镜头。
- 本地 ContentBatch 固定输出 `contentcloud.content-item/3.0`，交付 profile 固定为 JSON、Markdown 和 XLSX。
- 当前 Markdown/XLSX renderer 输出镜头表，而不是按内容类型选择呈现方式。
- Storyboard 和 Seedance 正确地作为视频生产下游，但当前 ContentItem 不能表达非视频内容。
- Plugin 名称、展示文案和多数业务 Skill 仍以 Video Production 为中心。
- 内置 Capability Catalog 当前主要登记知识提取能力，内容生产能力尚未形成完整、可路由的能力目录。

因此，直接增加公众号写作 Skill 会产生结构性冲突：Agent 可以写出文章正文，但仍必须伪造时长、画幅和镜头才能通过 Schema 和 lint。这会破坏领域事实，也会让 Web、审批和交付错误理解产物。

## 3. 目标与非目标

### 3.1 目标

1. 一个项目可以按 Intent 生产视频、文章和后续新增的其他内容形态。
2. 每种内容形态拥有独立、版本化、可确定性校验的内容契约。
3. 新增一种内容形态时，尽量不修改 ContentCloud Core 的状态机和服务端表结构。
4. 所有内容继续复用证据、知识、权利、审批、版本、交付和结果血缘。
5. Skill、内容契约、渠道规则、Provider 适配器和结果指标各自有明确所有者。
6. 同一语义内容可以派生不同渠道交付包，而不把渠道 HTML 或厂商语法写回领域正文。
7. 公众号文章从 Brief、创作、审核、交付到结果形成可验收的完整纵向切片。
8. 在保持本地优先和 zero-exec 服务端边界的前提下扩展能力。

### 3.2 非目标

- 不在首版支持所有内容平台。
- 不建立通用低代码工作流编辑器。
- 不允许 Workspace 自由加载未经签名的 Schema、代码或 Skill。
- 不让服务端执行 LLM、扫描本机 Workspace 或直接读取未 publish 内容。
- 不在首版代持公众号账号、Cookie、AppSecret 或发布凭据。
- 不将公众号富文本 HTML 作为文章唯一事实源。
- 不用一个包含大量可选字段的“万能 ContentItem”覆盖所有内容类型。
- 不因扩展文章而中断或重写 V5 已验证的视频生产闭环。

## 4. 设计原则

### 4.1 稳定内核，变化外置

Core 只拥有跨内容稳定的不变量：对象身份、版本、状态、摘要、引用、权利、审批、交付和结果。文章段落、视频镜头、播客章节等变化字段由类型化内容契约拥有。

### 4.2 类型化扩展，不使用可选字段袋

不得把 `word_count`、`duration_ms`、`shots`、`article_blocks`、`audio_segments` 等全部加入同一 Schema 并设为可选。每种内容类型必须有明确 Schema ID、版本和确定性 validator。

### 4.3 领域内容与渠道交付分离

文章领域对象描述“表达什么、如何组织、引用什么”；WeChatDeliveryPackage 描述“公众号后台如何呈现和操作”。微信公众号 HTML、临时素材编号和 API media ID 不进入 ArticleItem。

### 4.4 Skill 不拥有正式状态

Skill 可以生成和修订 candidate，但不能把 Fact 标为 verified、Claim 标为 approved、ContentItem 标为正式批准，或宣称外部平台已经发布。正式状态继续由服务端不可变 Revision 和人工 Decision 产生。

### 4.5 确定性门禁优先

以下规则必须由 Schema、lint 或服务端复验执行，而不是仅写在 Prompt：

- 引用对象存在且 eligible。
- 商业主张和高风险表达有批准依据。
- 素材权利覆盖目标渠道和使用时间。
- payload 匹配声明的 Schema ID 与 digest。
- 文章 block、图片槽位、链接和 CTA 引用一致。
- 导出包不包含绝对路径、凭据、隐藏推理或未授权原件。
- ApprovedSnapshot 与本地交付输入摘要一致。

### 4.6 首个真实用例驱动抽象

第一阶段只支持现有视频和公众号文章两种内容形态。只有当第三种内容形态出现相同变化点时，才把重复逻辑上移为 Core 抽象，避免为假想平台过度设计。

## 5. 目标分层

### 5.1 ContentCloud Core

Core 负责：

- WorkspaceBinding、Environment 和租户/项目授权。
- Source、Evidence、Knowledge、Asset 和 Rights 的通用治理。
- LocalRun、RunClaim、Handoff 和中断恢复。
- Intent Profile 的解析、能力需求和输入冻结。
- 通用 ContentItem 信封和 ContentBatch 生命周期。
- SubmissionRevision、ReviewCycle、Decision 和 ApprovedSnapshot。
- Artifact、DeliveryPackage、PublishedContentBinding 和结果血缘。
- Schema/Capability/Channel Profile 的可信来源、版本和摘要验证。

Core 不负责：

- 决定公众号文章应该使用何种标题结构。
- 决定视频需要多少镜头。
- 生成具体正文、图片、音频或视频。
- 把 Markdown 转换成某个平台私有格式。
- 代替用户登录、上传或发布到外部平台。

### 5.2 Intent Profile

Intent Profile 是任务路由契约，描述一次内容任务需要什么，不保存具体生成方法。建议最小字段如下：

```yaml
schema_version: contentcloud.intent-profile/1.0
id: intent:wechat-official-article
content_kind: article
channel_profile_ref: channel:wechat-official-account-cn@1.0.0
brief_schema_ref: contentcloud.article-brief/1.0
content_schema_ref: contentcloud.article/1.0
required_inputs:
  - project_context
  - knowledge_snapshot
required_capabilities:
  - contentcloud.article.plan
  - contentcloud.article.compose
  - contentcloud.article.validate
  - contentcloud.wechat.package
delivery_profiles:
  - wechat-html
  - markdown
metric_profile_ref: metrics:wechat-official-account@1.0.0
```

Intent Profile 还应声明：

- 输入 Gate 和允许的 blocked 探索模式。
- 内容数量和批次约束。
- 可使用的渠道和语言。
- 允许引用的知识类型。
- 所需的人工选择点。
- 默认审核策略和交付格式。
- 结果指标口径。

Intent Profile 不直接包含长 Prompt；写作方法由 Skill Pack 维护。

### 5.3 Content Specification Pack

Content Specification Pack 提供某种内容形态的完整生产能力：

- Brief Schema。
- Content payload Schema。
- 确定性 lint 和 diff 规则。
- Markdown/JSON 等领域渲染器。
- 修订允许路径规则。
- 审核所需的 Presentation Profile。
- 代表性正反例和测试 Fixture。
- 对应 Skills。

视频和文章分别由独立 Specification Pack 拥有。StoryboardPackage 和 SeedancePromptPackage 仍是视频内容的派生对象，不上移到 Core。

### 5.4 Channel Profile

Channel Profile 是有版本和有效期的渠道规则快照，例如：

- 渠道标识与适用地区。
- 标题、摘要、封面和正文能力。
- 支持的 block、HTML 标签和链接规则。
- 图片格式、尺寸、大小和数量限制。
- 外链、转载、原创声明、广告和合规要求。
- 发布前人工检查项。
- 可观测指标及其定义。

渠道规则可能变化，不能把模型常识或未固定网页内容当作永久事实。Profile 必须记录来源、捕获时间、有效期、版本和 digest。

### 5.5 Provider Adapter

Provider Adapter 负责对接具体外部工具或 API，例如未来的微信公众号草稿箱 API。它只能消费已批准内容和有效渠道 Profile，并且必须：

- 独立声明网络、凭据、数据披露、费用和副作用。
- 使用显式用户授权和幂等键。
- 不把凭据写入 Workspace、Run、Handoff 或服务端业务正文。
- 区分“创建草稿”“上传素材”“提交发布”和“发布成功”等状态。
- 失败后可以查询外部状态并恢复，不能盲目重试发布。

公众号首版不实现 Provider Adapter，只生成用户可操作的本地交付包。

## 6. 通用领域模型

### 6.1 ContentItem 治理信封

建议下一版 ContentItem 只保留所有内容形态共有的治理字段：

```json
{
  "schema_version": "contentcloud.content-item/4.0",
  "id": "content-item:wechat:20260729:001:v1",
  "type": "content_item",
  "content_kind": "article",
  "content_schema_ref": "contentcloud.article/1.0",
  "channel_profile_ref": "channel:wechat-official-account-cn@1.0.0",
  "status": "candidate",
  "deliverability": "review_ready",
  "project_id": "project-...",
  "content_batch_id": "content-batch-...",
  "brief_ref": "article-brief:...",
  "context_snapshot_id": "project-context-...",
  "based_on_version_id": "",
  "resolved_comment_ids": [],
  "change_summary": "首稿",
  "citations": [],
  "asset_refs": [],
  "rights_refs": [],
  "blocked_reasons": [],
  "missing_inputs": [],
  "validation_declarations": {},
  "payload": {}
}
```

公共信封负责：

- 对象身份、项目、批次和版本关系。
- 内容类型和 Schema 引用。
- 输入快照和渠道 Profile 冻结。
- candidate/blocked 与 deliverability。
- 通用引用、素材、权利、阻断和检查摘要。
- payload digest 和完整对象 canonical digest。

内容 Pack 负责验证 `payload`。Core 不对未知 payload 放行：Schema 必须来自签名 Environment/Registry，版本和摘要必须与 LocalExecutionPlan 一致。

### 6.2 为什么不使用 JSON Schema `oneOf` 收纳所有内容

在 Core Schema 中不断增加 `oneOf(video, article, podcast, ...)` 会导致每新增一个 Pack 都需要发布 Core 新版本，违背可扩展目标。推荐使用稳定信封 + 可信 Schema Registry：

```text
Core validates envelope
  -> resolve exact schema_ref + digest from signed environment
  -> Pack validator validates payload
  -> publish preflight records validator capability + version + digest
  -> server revalidates trusted schema and governance invariants
```

首个版本可以只允许内置的 `video-script` 与 `article` 两种 Schema，不开放任意第三方 Schema。

### 6.3 Brief 分层

现有 Brief 中很多字段可以跨内容复用：

- strategy、campaign、experiment。
- channel、objective、audience、scenario、demand moment。
- pain point、selling point、support points、positioning。
- tone、brand rules、approved claims、forbidden claims。
- CTA、primary variable、controlled variables 和 measurement window。
- eligible/blocked knowledge、rights、risk decisions。

视频专用字段应移入 `video_spec`：

- 时长范围。
- 画幅。
- 可视化计划。
- 镜头与连续性约束。

文章专用字段应移入 `article_spec`：

- 内容主题和读者承诺。
- 文章结构类型。
- 目标字数区间。
- 标题策略和摘要策略。
- 段落深度、阅读节奏和证据密度。
- 图片计划、封面要求和图注策略。
- 引用展示策略。
- 原创/转载/署名约束。

建议采用公共 Brief 信封和类型化 spec，而不是继续扩充 `Brief 3.0` 的顶层字段。

### 6.4 ContentBatch 复用

`ContentBatch` 继续作为多内容形态的批次对象，不为文章创建 ArticleBatch。下一版只需补充或冻结以下信息：

- `content_kind`。
- `content_schema_ref`。
- `channel_profile_ref`。
- `validator_capability_refs`。
- `delivery_profiles`。

一个批次首版只允许一种 `content_kind` 和一个主渠道 Profile，避免混合文章与视频后无法定义完整性和审核规则。跨渠道改编通过派生 Run 和新 ContentBatch 表达。

### 6.5 DeliveryPackage 与渠道交付包

DeliveryPackage 继续是服务端/治理层的通用交付对象，具体交付文件由渠道适配器生成：

```text
Approved ContentItem
  -> local channel adapter
  -> WeChatDeliveryPackage candidate
  -> deterministic lint
  -> optional delivery publish
  -> DeliveryPackage / Artifact metadata
```

交付包必须记录：

- 来源 ApprovedSnapshot ID 和 digest。
- ContentItem ID、payload digest、Channel Profile 版本。
- renderer/adapter capability 版本与 digest。
- 文件清单、SHA-256、媒体类型和字节数。
- 外部操作说明和发布前检查。
- 是否包含需要外部披露的素材。

### 6.6 PublishedContentBinding

现有 PublishedCreativeBinding 面向视频创意和投放。多内容形态需要一个语义更通用的发布绑定，至少包含：

```text
project_id
delivery_package_id
approved_snapshot_id
content_item_id
content_kind
channel
external_account_ref
external_content_id
external_url
published_at
publication_status
experiment_id / arm_id
content_digest
```

是否直接泛化现有 PublishedCreativeBinding，还是新增 PublishedContentBinding，需在实施前通过使用方和迁移成本评审。本文倾向于新增通用 PublishedContentBinding，并让视频结果也逐步引用它；不建议仅为公众号增加 `WeChatPost` 服务端实体。

### 6.7 通用状态机

本地内容状态保持简单：

```text
candidate
   | lint passed
   v
review_ready ---------> blocked
   | publish               ^
   v                       | missing/invalid input
SubmissionRevision         |
   | human decision        |
   +---- changes_requested-+
   |
   v
ApprovedSnapshot
   |
   v
local channel package -> validated delivery -> external publication
```

`approved` 不能由本地 ContentItem 写入；它由服务端 ApprovedSnapshot 表达。外部平台的 `published` 也不能由“已生成交付包”推断，必须来自用户回填或经授权 Provider Adapter 的已验证结果。

## 7. 公众号文章领域契约

### 7.1 ArticleBrief

ArticleBrief 除公共 Brief 字段外，建议包含：

| 字段组 | 内容 |
| --- | --- |
| 选题 | topic、reader promise、timeliness、content pillar |
| 读者 | target reader、reading context、prior knowledge、expected action |
| 结构 | structure type、section goals、opening strategy、ending strategy |
| 篇幅 | target/min/max word count，作为检查范围而非机械填充目标 |
| 表达 | voice、tone、narrative person、terminology、forbidden patterns |
| 证据 | required facts、approved claims、quotes、citation display policy |
| 图片 | cover intent、inline image plan、captions、rights requirements |
| 渠道 | title/summary constraints、originality/attribution、link policy |
| 转化 | CTA、conversion destination、tracking requirements |
| 实验 | primary variable、controlled variables、metrics、window |

结构类型首版采用少量可验证枚举：

- `problem_solution`：问题、原因、方案、证据、行动。
- `how_to`：目标、前置条件、步骤、常见错误、结果。
- `case_study`：背景、挑战、过程、结果、经验。
- `brand_story`：起点、冲突、选择、证明、今天。
- `opinion_analysis`：问题、判断、论据、反方、结论。
- `curated_guide`：主题、选择标准、分组推荐、总结。

不得为了覆盖所有写作风格而在首版加入任意 DAG 或模板语言。结构只是规划约束，ArticleItem 仍通过 blocks 表达实际正文。

### 7.2 ArticleItem payload

建议 Article payload 采用结构化元数据 + 有稳定锚点的正文 blocks：

```json
{
  "schema_version": "contentcloud.article/1.0",
  "language": "zh-CN",
  "title_candidates": [
    {
      "id": "title-1",
      "text": "...",
      "strategy": "reader-benefit",
      "risk_refs": []
    }
  ],
  "selected_title_id": "title-1",
  "summary": "...",
  "author_display_name": "...",
  "cover": {
    "asset_ref": "asset:...",
    "rights_ref": "rights:...",
    "alt_text": "...",
    "caption": "..."
  },
  "blocks": [
    {
      "id": "block-001",
      "type": "paragraph",
      "text": "...",
      "assertions": [],
      "style_marks": []
    }
  ],
  "cta": {},
  "attribution": {},
  "editorial_checks": {},
  "channel_hints": {}
}
```

`channel_hints` 只能保存不改变正文语义的展示意图，例如重点提示或期望换行；公众号 HTML、CSS、临时 media ID 和后台草稿 ID 不进入 payload。

### 7.3 Article Block

首版支持以下 block：

| Block | 用途 | 关键约束 |
| --- | --- | --- |
| `heading` | H2/H3 层级标题 | 不允许跳级；H1 由文章标题提供 |
| `paragraph` | 正文段落 | 稳定 block ID，支持 assertion 锚点 |
| `list` | 有序或无序列表 | item 不嵌套任意 block |
| `quote` | 原文引用或人物引用 | 必须有 source/evidence 或明确 attribution |
| `image` | 正文图片槽位 | asset、rights、alt、caption 和 purpose |
| `callout` | 提示、结论或注意事项 | 类型受限，不能替代事实引用 |
| `divider` | 结构分隔 | 不承载正文事实 |
| `cta` | 文章行动入口 | 目标、文案、链接/小程序引用和合规检查 |

首版不支持任意 HTML block、脚本、iframe、内联 style 或未知嵌入。需要新增能力时，先更新 Article Schema 和 WeChat Channel Profile。

### 7.4 Assertion 与引用

长文章不能要求每句话都引用，也不能让所有内容绕过证据门禁。建议将可核查表达分为：

| 类型 | 说明 | 门禁 |
| --- | --- | --- |
| `fact` | 产品、历史、数据、流程等可验证事实 | 必须引用 eligible Fact/Evidence |
| `commercial_claim` | 功效、优势、比较或承诺 | 必须引用 approved Claim；高风险需 Decision |
| `quotation` | 对来源或人物的直接/间接引用 | 必须有 locator 和 attribution |
| `editorial_opinion` | 作者判断、建议或价值表达 | 可不引用，但必须明确不是客观事实 |
| `personal_experience` | 经授权的第一人称经历 | 需要来源身份和披露规则，不可伪造 |
| `hypothesis` | 待验证观点或创作假设 | 不得表述为已验证结论 |

每个 assertion 需要稳定 ID，并锚定到 block 或 block 内受控范围。首版可以使用 block 级锚点，避免实现脆弱的字符 offset；同一 block 包含多个关键事实时应拆段或声明多个 assertion。

### 7.5 图片与权利

公众号文章图片分为：

- 封面图。
- 正文解释图。
- 产品/品牌真实素材。
- 数据图表。
- 装饰性生成图片。
- 二维码或 CTA 图片。

每个图片槽位必须说明 purpose、asset_ref、rights_ref、alt_text、caption、truth requirements 和 fallback。生成图片不能伪造产品外观、检测报告、客户案例或历史照片。图表必须能追溯到数据和生成逻辑；二维码和动态权益需在导出或发布时复验。

### 7.6 公众号 Channel Profile

`wechat-official-account-cn` Profile 应保存已验证的渠道事实，而不是在 Skill 中硬编码易变规则：

- 标题、摘要和封面的当前限制。
- 正文允许的 HTML 子集。
- 图片格式、体积和显示行为。
- 外链、小程序、视频和音频能力。
- 原创、转载、署名、广告和隐私规则。
- 草稿、预览、群发和发布状态定义。
- 平台指标字段和数据窗口。
- 来源、捕获时间、有效期和适用账号类型。

Profile 过期时，ArticleItem 仍可继续本地创作，但正式 WeChatDeliveryPackage 必须 blocked，直到用户刷新并确认新的渠道规则。

## 8. 公众号 Skills 与 Capability

### 8.1 Skill 规划

建议首个公众号 Pack 包含四个业务 Skill：

| Skill | 责任 | 不负责 |
| --- | --- | --- |
| `contentcloud-article-planning` | 选题、读者需求、标题方向、结构和 Brief 修订 | 写正式知识、批准主张、发布 |
| `contentcloud-longform-writing` | 基于冻结 Brief 和知识快照生成/修订 ArticleItem | 修改 ApprovedSnapshot、绕过 lint |
| `contentcloud-article-visuals` | 规划封面与插图，调用已授权图片能力 | 推断素材权利、自动外传全部原件 |
| `contentcloud-wechat-delivery` | 从批准 ArticleItem 编译公众号交付包 | 登录公众号或直接发布 |

知识提取继续复用现有 Knowledge Skill；Workspace 选择、RunClaim、Handoff、publish/pull 和 Browser 导航继续由通用 Scene Skill 负责。

### 8.2 Capability 建议

Capability 使用稳定业务能力 ID，而不是 Skill 文件名：

```text
contentcloud.article.plan
contentcloud.article.compose
contentcloud.article.validate
contentcloud.article.visual-plan
contentcloud.wechat.package
contentcloud.wechat.publish-draft       # 后续可选 Provider 能力
contentcloud.wechat.publish             # 后续单独授权，不与 draft 混合
contentcloud.wechat.metrics.import      # 后续可选
```

每个 Capability 必须声明：

- 输入和输出 Schema。
- 实现版本与 digest。
- 所属 Plugin/Pack。
- 是否 local-only。
- 网络、数据披露和费用。
- 是否产生外部副作用。
- Presentation Profile。

`compose` 和 `validate` 应分开：Agent 写作质量与确定性合规检查不是同一责任。`package` 和 `publish` 也必须分开：生成本地交付包不等于操作外部平台。

### 8.3 Plugin 打包策略

目标形态：

```text
contentcloud-core                    scene_plugin, environment scoped
contentcloud-video-production       skill_pack, task scoped
contentcloud-wechat-article         skill_pack, task scoped
contentcloud-image-production       provider_mcp_pack 或 skill_pack
contentcloud-wechat-provider        provider_mcp_pack, task scoped, 后续可选
```

考虑到当前 `contentcloud-video-production` 同时承载 Scene Skill 和视频 Skills，不建议在 V5/v0.9 发布过程中立即大拆包。推荐分两步：

1. 先在逻辑和 Capability Registry 上分离通用 Scene 能力与视频能力，保持现有发布行为稳定。
2. 在多内容内核版本中再发布通用 Core Scene Plugin 和 task-scoped 内容 Pack，并提供明确升级计划和新会话恢复。

任何安装或升级继续使用 Environment Preparation 的只读计划、精确版本/digest 和用户确认，不能因为用户选择公众号 Intent 就静默安装未知 Pack。

## 9. 公众号端到端流程

### 9.1 项目准备

项目上下文至少需要：

- 公众号账号定位、品牌/作者身份和目标读者。
- 内容栏目和选题边界。
- 品牌语气、禁用表达和常用术语。
- 事实、主张、素材、权利和合规知识。
- 历史文章及其结果；历史内容只作为可追溯学习，不自动视为正确事实。
- 当前 WeChat Channel Profile。

### 9.2 纵向流程

```text
用户选择 intent:wechat-official-article
  -> workspace_context 读取本地状态
  -> init/claim 独立 LocalRun
  -> environment_execution_plan 校验 Article Pack
  -> query eligible/blocked knowledge
  -> 创建 ArticleBrief candidate
  -> 生成 3 至 5 个标题/角度与结构候选
  -> 用户选择一个方向
  -> 生成 ArticleItem candidate
  -> schema/citation/claim/rights/editorial/channel lint
  -> blocked 或 review_ready
  -> ContentBatch finalize
  -> publish preflight + 用户确认
  -> 服务端创建 content_batch SubmissionRevision
  -> Browser 展示文章预览、引用和 diff
  -> 人工 approve 或 changes_requested
  -> 用户明确 pull ApprovedSnapshot
  -> 本地 WeChat adapter 编译交付包
  -> package lint + 本地预览
  -> 用户在公众号后台粘贴、检查并发布
  -> 登记 PublishedContentBinding
  -> 导入 Observation
  -> 人工采纳或拒绝 Learning
```

### 9.3 阻断模式

正式知识不足时，可以生成 blocked ArticleItem 用于讨论选题、结构和表达，但必须：

- 明确列出缺失 Fact、Claim、Rights 或 Channel Profile。
- 对没有证据的事实位置使用占位/问题，而不是编造正文。
- 允许 publish `content_batch` 进入创意评审，但禁止生成正式交付包。
- Web 清晰显示“可评审但不可交付”，不展示为发布就绪。

### 9.4 审核修订

审核评论应锚定：

- 标题候选 ID。
- Article block ID。
- Assertion ID。
- 图片槽位 ID。
- CTA ID。

修订必须从已发布基线派生新版本，记录 `based_on_version_id`、已解决评论和 change summary。Article diff 应按语义 block 展示插入、删除、移动和修改，不能只提供整段文本 diff。

## 10. WeChatDeliveryPackage

### 10.1 本地目录

不增加新的 Workspace 顶级目录，继续使用 V3 结构：

```text
50-production/
  briefs/
    <article-brief-id>.json
  batches/
    <content-batch-id>/
      manifest.yaml
      context.json
      items/
        <article-item-id>.json

60-delivery/
  packages/
    <delivery-package-id>/
      manifest.json
      providers/
        wechat-official-account/
          package.json
          README.md
          article.html
          article.md
          assets/
            cover.<ext>
            image-01.<ext>
          preview/
            article-preview.html
```

`article.md` 是渠道无关、便于人工审阅的投影；`article.html` 是由 Channel Profile 编译的公众号兼容交付物；`package.json` 是机器可验证契约；`README.md` 是不依赖聊天历史的操作说明。

### 10.2 package.json

建议记录：

```json
{
  "schema_version": "contentcloud.wechat-delivery/1.0",
  "id": "wechat-package:...",
  "project_id": "project-...",
  "approved_snapshot_id": "snapshot-...",
  "content_item_id": "content-item:...",
  "content_digest": "sha256:...",
  "channel_profile_ref": "channel:wechat-official-account-cn@1.0.0",
  "renderer": {
    "capability_id": "contentcloud.wechat.package",
    "version": "1.0.0",
    "digest": "sha256:..."
  },
  "files": [],
  "asset_mapping": [],
  "external_actions": [],
  "checks": [],
  "status": "validated"
}
```

### 10.3 README 操作说明

README 至少包含：

- 来源 ApprovedSnapshot、文章标题和摘要短指纹。
- 当前 Channel Profile 版本和检查时间。
- 封面及正文图片的上传顺序、文件名、用途和摘要。
- 公众号后台需要填写的标题、摘要、作者、正文和原文链接。
- 原创/转载/广告/隐私等人工确认项。
- 链接、二维码、价格、优惠和 CTA 的最终核对项。
- 预览、发送测试、群发/发布前检查。
- 外部发布后需要回填的内容 ID、URL、时间和账号引用。

### 10.4 外部发布边界

首版以人工发布为准：

```text
ContentCloud/Codex             用户                   微信公众平台
生成 validated package
        |                        |
        +---- 交付清单 --------->|
                                 +---- 登录/上传/粘贴/预览 ---->
                                 <---- 草稿/预览结果 ------------+
                                 +---- 人工确认发布 ------------->
                                 <---- content ID / URL ----------+
        <---- 回填发布绑定 -------+
```

“导出成功”“已复制到剪贴板”“创建草稿”“发送预览”“群发提交”和“发布成功”必须是不同状态。任何未来自动化都不能把创建草稿宣称为正式发布。

## 11. Web 产品改造

### 11.1 信息架构

现有“创意与剧本”应逐步改为更通用的“内容生产”，其内部根据 `content_kind` 展示：

```text
内容生产
  -> Brief
  -> ContentBatch
  -> Video Script / Article / 其他 ContentItem
  -> 类型化生产对象
  -> 交付与发布绑定
```

不能在导航中为每个新内容类型增加一个顶级模块。内容类型是 ContentBatch/ContentItem 的筛选和详情形态，不是新的数据平面。

### 11.2 类型化 Presentation Profile

服务端保存不可变 Revision 和结构化对象；Web 根据受信任 Presentation Profile 渲染：

- 视频：封面、镜头表、口播、字幕、引用、分镜状态。
- 文章：标题候选、摘要、文章预览、block diff、assertion 引用、图片槽位。
- 未识别类型：只显示安全的结构化摘要和下载信息，不执行 payload 中的 HTML/脚本。

Presentation Profile 必须由产品代码或可信 Registry 提供，不能执行客户上传的任意组件。

### 11.3 公众号审核页

审核页至少显示：

- 文章正文的真实阅读预览。
- 当前 Revision/digest 和输入快照。
- 标题候选与最终选择。
- block 级 diff。
- Fact、Claim、Quote、Opinion 的分类和证据。
- 图片、来源、权利和披露级别。
- 渠道 lint 和阻断原因。
- 审核评论、批准、退回和影响范围。

预览 HTML 必须经过 sanitizer 和严格 CSP；不能直接渲染 Article payload 中的任意 HTML。

### 11.4 项目总览与下一动作

总览按活动 Intent 显示：

- 当前内容类型和渠道。
- 最近 ContentBatch 的本地/提交/批准/交付状态。
- 缺少的知识、权利或渠道 Profile。
- 待选择的文章方向。
- 待审核 Revision。
- 待发布交付包和待回填外部 ID。

## 12. 执行边界

| 动作 | Codex 本机 | ContentCloud 服务端 | 用户/微信平台 |
| --- | --- | --- | --- |
| 查询知识和创建 Brief | 执行 | 不读取本机候选 | 不参与 |
| 生成/修订 ArticleItem | 执行 candidate | 未 publish 前不可见 | 不参与 |
| lint | 本地预检 | publish/approve 时复验治理不变量 | 不参与 |
| 内容审核 | 展示本地状态、pull 结果 | 保存 Revision、评论、Decision、Snapshot | 审核人在 Web 决定 |
| 生成 WeChatPackage | 从 pulled Snapshot 本地执行 | 可保存显式 publish 的 manifest/Artifact | 不参与 |
| 创建公众号草稿 | 首版不执行 | 首版不执行 | 用户在微信后台执行 |
| 发布公众号文章 | 不推断成功 | 保存用户回填/Provider 验证的 binding | 用户明确确认发布 |
| 导入结果 | 可准备文件/发起命令 | 校验并保存 Observation | 平台提供数据或用户导出 |
| 采纳学习 | 可生成候选 | 保存人工 RatingDecision/Learning | 负责人决定 |

服务端继续保持 zero-exec，不运行 Article Skill、LLM 或客户上传 renderer。未来 Provider Adapter 如果需要网络调用，应运行在用户授权的本地 Agent/受控 Adapter 平面，而不是隐含改变 Core 服务端边界。

## 13. 本地 CLI/MCP 接口草案

以下命令用于明确责任和验收，名称尚未冻结：

```text
contentcloud local article brief scaffold
contentcloud local article brief lint
contentcloud local article batch create
contentcloud local article item lint
contentcloud local article item diff
contentcloud local article batch finalize
contentcloud local wechat package export
contentcloud local wechat package lint
contentcloud publish brief --dry-run
contentcloud publish content-batch --dry-run
contentcloud publish delivery --dry-run
contentcloud binding create --channel wechat-official-account --dry-run
contentcloud result import --profile wechat-official-account --dry-run
```

MCP Tool 应围绕稳定业务动作提供类型化参数，不暴露任意 shell、任意 Schema 路径或任意 renderer。所有本地写工具继续要求 RunClaim 和最新 `context_revision`。

## 14. 结果指标与学习

### 14.1 Metric Profile

不同渠道指标不能硬塞进视频字段。公众号 Metric Profile 可定义：

- send count / delivered count。
- reads / unique readers。
- completion 或阅读深度（仅在数据真实可用时）。
- likes / recommendations / shares / saves。
- follows / unfollows。
- link clicks / mini-program actions。
- conversions / revenue（有可靠绑定时）。
- observation window、数据来源和采集时间。

每个 Observation 必须记录渠道、账号引用、external_content_id、指标定义版本、窗口和采集方式。没有可靠发布绑定的数据进入隔离区，不能自动归因到 ArticleItem。

### 14.2 实验口径

公众号内容实验可能改变：

- 标题。
- 封面。
- 选题/角度。
- 开头结构。
- CTA。
- 发布时间或分发人群。

只有主变量之外的关键条件可控时才标记为严格实验。自然发布环境中的多变量变化应标记为探索或观察性比较，不能将阅读量差异自动解释为某个标题策略的因果效果。

### 14.3 Learning

系统可以生成 Learning candidate，例如“案例型标题在本账号近四次发布中阅读打开率较高”，但必须同时披露样本量、时间窗口、协变量和数据完整性。只有人工采纳后，Learning 才能进入下一版 Context/Brief；不得自动改写历史文章或通用 Skill。

## 15. 安全、合规与信任边界

### 15.1 内容安全

- 高风险行业的功效、健康、金融、法律和比较性主张必须使用类型化风险规则和人工决定。
- Article Skill 不得根据一般模型知识补全客户产品事实。
- 对人物、客户案例和个人经历的描述必须有授权和来源。
- 引用不得伪造作者、书名、数据、链接或原话。
- 转载、摘编和图片使用必须满足权利范围和署名要求。

### 15.2 HTML 与预览安全

- ArticleItem 不接受任意 HTML、JavaScript、iframe 或事件处理器。
- WeChat renderer 从结构化 blocks 生成允许的 HTML 子集。
- Web 预览再次 sanitizer，并运行严格 CSP。
- 外链协议和目标必须 allowlist 校验，禁止 `javascript:`、本机路径和凭据参数。
- 图片只引用受治理 Artifact，不加载 payload 提供的任意本地路径。

### 15.3 凭据和外部副作用

- AppID、AppSecret、Cookie、access token 不写入业务文件、日志、Handoff 或 Submission。
- Provider 操作必须显示账号、目标、数据范围和预期副作用。
- 草稿创建、素材上传和正式发布分别授权。
- 正式发布需要幂等键、状态查询、失败恢复和审计。
- 删除草稿、撤回或覆盖等破坏性动作需独立确认。

### 15.4 Skill 供应链

- Article/WeChat Pack 必须固定来源、版本、digest 和许可证。
- 不直接复制无明确许可证的第三方 Skill。
- 第三方写作方法只能作为调研证据，重新实现时保持 ContentCloud 的事实、权利和审批边界。
- Plugin 或 Skill 更新不得静默改变已批准文章或已导出的交付包。

## 16. 实施路线

当前 V5 和 v0.9 发布工作仍在进行。多内容形态不应插入同一轮 Schema/Plugin 发布，建议作为后续独立里程碑推进。

### M0：方案与决策冻结

输出：本文评审结论、领域术语、核心决策和公众号试点范围。

退出条件：

- 产品确认首个公众号账号、读者、内容目标和发布边界。
- 工程确认 ContentItem 信封、Schema Registry 和 Brief 分层方向。
- 内容/合规确认 assertion 类型和证据门禁。
- 设计确认文章审核预览和 block diff。
- 运维/安全确认首版不自动发布。

### M1：多内容内核

输出：通用 ContentItem 信封、类型化 payload 校验、Intent Profile 和 ContentBatch 扩展。

退出条件：

- Core 不再依赖 `shots`、`duration_ms` 或 `aspect_ratio`。
- 只允许 Environment 签名范围内的内容 Schema。
- video-script 与 article 两种 payload 可走相同 Batch/Submission 主链。
- 未知/恶意 payload 在本地和服务端都被拒绝。

### M2：视频回归与逻辑拆包

输出：Video Script Spec/validator、现有 Storyboard/Seedance 血缘适配、Capability Catalog。

退出条件：

- V5 视频 Golden Journey 行为、摘要和审批边界无回归。
- Storyboard 只消费批准的 video-script ContentItem。
- 当前 Plugin 的通用 Scene 能力和视频能力在逻辑上可独立解析。

### M3：公众号本地纵向切片

输出：ArticleBrief、ArticleItem、Skills、lint、diff、Fixture 和 WeChatDeliveryPackage。

退出条件：

- 离线可以完成 Brief、方向选择、文章 candidate、lint 和 Handoff。
- 事实、主张、引用、图片和权利门禁有正反例。
- ApprovedSnapshot 可以稳定导出 Markdown、HTML、图片清单和 README。
- 相同输入生成相同 package manifest 和摘要。

### M4：Web 审核与交付

输出：文章列表、真实预览、block diff、引用面板、审核和 Delivery 页面。

退出条件：

- Web 区分本地候选、已提交 Revision、已批准 Snapshot 和已发布外部内容。
- 文章 HTML 不执行任意脚本或未知标签。
- 评论能稳定锚定 block/assertion，并被 Codex 修订消费。
- Browser 深链能打开精确 Revision 和 digest。

### M5：真实账号试点

输出：至少一篇真实公众号文章的完整 Golden Journey 和验收证据。

退出条件：

- 用户无需聊天历史即可按交付包完成后台发布。
- 发布内容与 ApprovedSnapshot/Delivery digest 一致。
- 图片、引用、署名、链接和 CTA 人工复核通过。
- external_content_id、URL 和发布时间建立 Binding。
- 至少一次结果导入和人工 Learning 决定完成。

### M6：可选 Provider 自动化

只有人工流程稳定且确有频繁需求后再立项。至少需要：

- 微信官方 API 能力和账号类型核实。
- OAuth/凭据存储、轮换和撤销方案。
- 素材上传、草稿、预览、发布的独立权限。
- 幂等、限流、超时、状态查询和失败恢复。
- 数据保留、删除、审计和紧急停止。
- 真实沙箱或受控账号验收。

## 17. 工作包建议

| ID | 工作包 | 主要产物 | 依赖 |
| --- | --- | --- | --- |
| C-00 | 冻结多内容术语和边界 | 决策记录、对象关系和非目标 | 无 |
| C-01 | 定义 ContentItem 4.0 信封 | Schema、digest、不变量 | C-00 |
| C-02 | 定义受信 Schema/Capability 解析 | Registry、Environment、LocalExecutionPlan | C-01 |
| C-03 | 拆分公共 Brief 与 typed spec | Brief Schema、lint、迁移策略 | C-01 |
| C-04 | 适配 Video Script Spec | 视频回归 Fixture、Storyboard 门禁 | C-01 至 C-03 |
| C-05 | 定义 ArticleBrief/Article 1.0 | Schema、示例、lint | C-01 至 C-03 |
| C-06 | 建立 Article Skills | Planning、Writing、Visuals | C-05 |
| C-07 | 定义 WeChat Channel Profile | 渠道规则、来源、有效期 | C-00 |
| C-08 | 实现 WeChatDeliveryPackage | renderer、manifest、README、lint | C-05、C-07 |
| C-09 | Web 类型化 Presentation | Article preview、block diff、review | C-05 |
| C-10 | 发布绑定与 Metric Profile | Binding、Observation、Learning | C-08 |
| C-11 | 安全和供应链测试 | HTML、凭据、权限、Skill digest | C-02、C-06、C-08 |
| C-12 | 真实公众号 Golden Journey | E2E 证据和试点评审 | C-05 至 C-11 |

## 18. 测试与验收

### 18.1 Schema 与领域测试

- ContentItem 信封缺少类型、Schema 或快照时拒绝。
- 声明 `article` 却携带 video payload 时拒绝。
- 未注册、摘要不匹配或超出 Environment 的 Schema 拒绝。
- Article block ID 重复、标题层级跳跃、CTA 引用不存在时拒绝。
- fact/commercial_claim 缺 eligible 引用时 blocked。
- editorial_opinion 不会被误标为已验证 Fact。
- 未授权图片、过期 Rights 或 Profile 导致交付 blocked。
- blocked ArticleItem 可以进入创意评审，但不能生成 validated delivery。

### 18.2 确定性测试

- 相同对象 canonical digest 一致。
- 相同 ApprovedSnapshot、Profile 和 renderer 生成相同 manifest。
- 修改正文、图片、Profile 或 renderer 后旧交付正确 stale。
- Article diff 稳定识别 block 插入、删除、移动和修改。
- HTML sanitizer 输出不含脚本、事件属性、危险协议和绝对路径。

### 18.3 权限与边界测试

- Codex 不能将本地 candidate 标为 approved。
- 未 publish 内容在服务端和 Web 不可见。
- 服务端不能扫描本机 `50-production` 或 `60-delivery`。
- 没有用户确认不能 publish Revision。
- 生成 WeChatPackage 不等于创建草稿或发布。
- 未建立 Binding 的结果不能进入正式归因。
- Provider 凭据不出现在日志、Workspace、Submission 或 Handoff。

### 18.4 回归测试

- V3 Workspace、Knowledge、Run/Handoff 和 publish/pull 全部通过。
- V4 Browser 路由、Revision focus 和 digest 校验通过。
- V5 Video Script、Storyboard、Seedance 和结果链通过。
- PostgreSQL RLS、不可变 Revision 和租户隔离无回归。

### 18.5 公众号 Golden Journey

1. 创建或选择绑定项目。
2. 登记公众号账号定位、品牌语气和内容栏目。
3. 摄取资料并形成候选知识。
4. 人工批准必要 Fact、Claim 和 Rights。
5. 选择 `intent:wechat-official-article`。
6. 生成 ArticleBrief 和多个方向。
7. 人工选择方向。
8. 生成带 block/assertion 的 ArticleItem。
9. 执行本地 lint，修复阻断。
10. finalize ContentBatch。
11. publish content_batch Revision。
12. Web 审核、评论并退回一次。
13. Codex pull 反馈并生成新版本。
14. 服务端批准并生成 ApprovedSnapshot。
15. Codex pull Snapshot，导出 WeChatDeliveryPackage。
16. 本地 package lint 和预览通过。
17. 用户在公众号后台上传、粘贴、预览并发布。
18. 回填 external_content_id、URL 和发布时间。
19. 导入至少一个结果窗口。
20. 人工采纳或拒绝一条 Learning candidate。

## 19. 架构决策清单

| ID | 决策 | 状态 |
| --- | --- | --- |
| D-CONTENT-01 | ContentCloud 采用稳定 Core + 内容 Pack + 渠道 Adapter | 建议接受 |
| D-CONTENT-02 | 公众号复用 `content_batch/delivery/result` Submission 轨 | 建议接受 |
| D-CONTENT-03 | ContentItem 使用稳定治理信封 + typed payload | 待工程评审 |
| D-CONTENT-04 | payload Schema 只能来自签名 Environment/Registry | 建议接受 |
| D-CONTENT-05 | Brief 拆分公共字段和 typed production spec | 待工程评审 |
| D-CONTENT-06 | 一个 ContentBatch 首版只包含一种 content kind | 建议接受 |
| D-CONTENT-07 | Article 正文采用结构化 blocks，不保存任意 HTML | 建议接受 |
| D-CONTENT-08 | 引用首版采用 block/assertion 锚点，不使用字符 offset | 建议接受 |
| D-CONTENT-09 | WeChat HTML 是交付投影，不是领域事实源 | 建议接受 |
| D-CONTENT-10 | Channel Profile 版本化并设置来源与有效期 | 建议接受 |
| D-CONTENT-11 | 首版由用户手工操作公众号后台 | 建议接受 |
| D-CONTENT-12 | 创建草稿与正式发布是独立 Provider Capability | 建议接受 |
| D-CONTENT-13 | 不在当前 V5/v0.9 发布中插入多内容重构 | 建议接受 |
| D-CONTENT-14 | 先逻辑拆分当前 Scene/Video 能力，再物理拆 Plugin | 建议接受 |
| D-CONTENT-15 | Web 使用可信 Presentation Profile 渲染不同内容 | 待安全评审 |
| D-CONTENT-16 | 使用通用 PublishedContentBinding 统一发布血缘 | 待领域评审 |
| D-CONTENT-17 | Metric Profile 按渠道定义指标语义 | 建议接受 |
| D-CONTENT-18 | Learning 继续由人工采纳，不自动修改 Skill/Brief | 建议接受 |

## 20. 风险与控制

| 风险 | 后果 | 控制措施 |
| --- | --- | --- |
| 过早泛化所有内容类型 | Schema 和路由复杂度失控 | 只实现 video + article，第三种类型出现后再抽象 |
| ContentItem 4.0 影响 V5 | 视频链回归 | 独立里程碑、视频适配层、完整 Golden Journey |
| 动态 Schema 变成代码执行入口 | 供应链和 RCE 风险 | 只允许签名声明式 Schema 和内置 validator，不执行上传代码 |
| Skill 与 validator 重复规则漂移 | Agent 输出与 lint 冲突 | Schema/lint 为事实源，Skill 引用规则而不复制枚举 |
| 文章事实引用过重 | 文风僵硬、生产效率低 | 只对可核查 assertion 强门禁，区分观点和事实 |
| 文章引用过松 | 幻觉和合规风险 | assertion 分类、eligible Claim、block 级审核 |
| 渠道规则变化 | 交付包不可用 | 版本化 Channel Profile、有效期和导出前复验 |
| HTML 预览注入 | Web 安全事故 | 结构化 blocks、renderer allowlist、sanitizer、CSP |
| 图片和转载权利不清 | 下架或法律风险 | RightsRecord、用途/渠道/期限门禁、人工确认 |
| 自动发布误操作 | 对外错误发布 | 首版手工；后续 draft/publish 独立授权与幂等 |
| 指标定义不一致 | 错误学习和决策 | 版本化 Metric Profile、数据窗口和来源 |
| Plugin 拆分影响安装 | 已绑定 Workspace 无法恢复 | 显式升级计划、digest 校验、新会话和回滚 |

## 21. 待评审问题

### 产品

1. 首个公众号试点是品牌故事、知识科普、产品教育还是销售转化文章？
2. 目标账号是订阅号还是服务号，是否存在现成栏目、模板和历史数据？
3. 首版交付需要“可复制 HTML”，还是 Markdown + 图片清单已经足够？
4. 是否需要多人审核、客户外链审核或仅内部审核？
5. 是否需要在首版登记发布结果和阅读数据？

### 内容与设计

1. Article blocks 是否足以覆盖实际排版，哪些特殊组件是首版必需？
2. 标题、摘要、封面是否需要独立审核决定？
3. 文章 diff 应优先展示结构变化、文本变化还是事实主张变化？
4. 历史文章的语气和结构如何进入项目上下文，而不被误当客户事实？

### 工程

1. ContentItem 4.0 是版本替换还是新 Envelope 类型？
2. payload validator 由 CLI 内置、Plugin 资源还是独立 Wasm/声明式运行时提供？首版建议 CLI 内置并由 Capability digest 标识。
3. 服务端如何安全复验动态 Schema，同时保持 zero-exec？
4. Video Script 迁移是否需要保留历史 Snapshot 的只读呈现？
5. PublishedCreativeBinding 与 PublishedContentBinding 如何收敛？

### 安全与运营

1. WeChat Channel Profile 的权威来源、维护者和有效期是什么？
2. 哪些行业需要额外 assertion 类型和审核 Gate？
3. 未来公众号 API 凭据由本机 Keychain、企业密钥服务还是其他受控平面保存？
4. 外部发布后内容删除、撤回和修订如何审计？

## 22. 完成定义

多内容形态扩展的首阶段完成，必须同时满足：

1. Core 不再假设 ContentItem 一定包含视频时长、画幅和镜头。
2. 视频和公众号文章使用不同的 Brief/Content payload Schema，但复用同一治理主链。
3. 所有 payload Schema、Skill Pack、Channel Profile 和 renderer 都有精确版本与 digest。
4. 公众号文章的事实、主张、引用、图片和权利可以逐项审核和追溯。
5. blocked 文章可以评审但不能生成正式 WeChatDeliveryPackage。
6. ApprovedSnapshot 可以确定性导出自包含公众号交付包。
7. 用户无需聊天历史即可根据 README 在公众号后台完成发布。
8. Web 明确区分本地候选、SubmissionRevision、ApprovedSnapshot、Delivery 和外部 PublishedBinding。
9. 发布绑定和结果能够追溯到具体 ArticleItem、Brief、知识快照和实验。
10. V3/V4/V5 的 Workspace、审批、Browser、安全和视频链没有回归。
11. 至少一个真实账号完成端到端试点，并保存可复核验收证据。

## 23. 推荐下一步

在进入代码设计前，先召开一次 M0 评审，只冻结以下五项：

1. `ContentItem` 通用信封与 typed payload 方向。
2. 公共 Brief 与内容专用 spec 的边界。
3. Article blocks、assertion 和引用最小模型。
4. 公众号首版只生成交付包、由用户手工发布的边界。
5. 首个真实试点账号、文章类型和验收指标。

评审通过后，先用 JSON Schema 和静态 Fixture 验证 video/article 两种内容能共享 ContentBatch 与 Submission，不立即实现 Provider API，也不先拆分现有正式 Plugin。该顺序能够以最小改动验证架构，同时保留后续扩展空间。

## 24. 变更记录

| 日期 | 变更 |
| --- | --- |
| 2026-07-29 | 建立多内容形态扩展方案，确定以微信公众号文章作为第一个非视频纵向切片 |
| 2026-07-29 | 提出 ContentItem 通用信封、typed payload、Intent Profile、Channel Profile 和 Content Pack 分层 |
| 2026-07-29 | 明确公众号首版采用本地交付包和人工发布，不隐含外部平台授权 |
| 2026-07-29 | 完成租户能力、签名 Manifest、公众号契约、本地创作、服务端复验、Web 审核和本地交付纵向切片 |
| 2026-07-29 | 新增独立 `contentcloud-wechat-article` Skill Pack 与多内容治理扫描 |

## 25. 实施基线

### 25.1 唯一事实源

租户可用内容类型只由服务端 Tenant Content Capability 决定；Environment Manifest 是该状态的签名投影；CLI、MCP 和 Skill 只能消费投影，不能自行开启能力。`video_script` 是固定默认能力，`wechat_article` 必须由平台管理员按租户显式开通。

### 25.2 当前分类

| 分类 | 当前路径 |
| --- | --- |
| `current` | Tenant Content Capability、签名 Manifest `content_types`、通用 ContentBatch 路由、视频剧本、ArticleBrief、ArticleItem、公众号 Web 审核、WeChatDeliveryPackage、公众号 Skill Pack |
| `compat` | `contentcloud-video-production` 暂时继续承载通用 Scene/MCP；公众号 Skill Pack 复用该 MCP，但不复制服务端状态和命令实现 |
| `deprecated` | 无新增长期 deprecated API 或 Schema |
| `dead` | 通用入口无条件要求 `shots` / `duration_ms`、公共审核页只渲染视频、通用交付无条件调用视频 renderer、Skill 自行开启租户内容能力 |

`compat` 的退出条件是通用 Core Scene Plugin 完成独立签名发布、已有 Workspace 有明确升级和新会话恢复路径、公众号 Pack 进入签名 Registry/Profile 后仍可通过 Environment Preparation 安装。退出时删除公众号 Pack 对视频插件 MCP 的分发依赖，不保留第二套命令实现。

### 25.3 已完成

- 平台后台按租户开关公众号能力，默认只开启视频剧本，并写入审计事件。
- Bootstrap、Workspace 刷新和 Automation 均签发 `content_types`，本地动作和服务端提交/审批双重复验。
- ContentBatch 通过 `content_kind`、`content_schema_ref` 和 `delivery_profiles` 路由视频与文章。
- ArticleBrief、ArticleItem 和 WeChatDeliveryPackage 1.0 Schema、确定性 lint、批次 finalize、修订 diff 和本地交付包已落地。
- 公共审核页按 Schema 分派，文章使用 React 安全渲染 blocks、assertion、引用和 JSON Pointer 评论锚点。
- 独立 `contentcloud-wechat-article` Skill Pack 包含策划、长文、配图和人工交付四个 Skill，不包含第二 MCP。
- `governance:content` 已进入常规 check 和 CI，防止视频假设与能力旁路回流。

### 25.4 明确延期

- 公众号 Skill Pack 的生产签名 Registry/Profile 条目必须在正式发布时使用仓库外私钥生成；当前工作树不伪造签名。
- 自动登录公众号、创建草稿、上传素材和正式发布仍是非目标。未来必须作为独立 Provider Capability 设计授权、幂等和外部状态恢复。
- PublishedContentBinding、公众号指标导入与 Learning 采纳尚未进入本次纵向切片。
- 完成定义中的真实公众号账号试点仍需产品和运营验收，不由单元测试替代。
