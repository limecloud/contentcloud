# 内容创作 AI Infra 架构与流程图谱

状态：`当前架构图 + 外部接通边界`。

更新时间：2026-08-11。

## 1. 阅读规则

- `current-local`：本地工作区、Plugin、Skill、MCP 和 CLI 已有可验证实现。
- `current-server`：服务端提交、审核、批准快照、Artifact 或投影已有实现。
- `runtime`：V8 JobRun/NodeRun/RuntimeAttempt 负责自动化执行事实。
- `partial`：内部 Adapter、契约或局部链路已经存在，但仍缺少真实外部验证。
- `external-dependency`：需要真实平台账号、授权、配额、模型集群或第三方 SaaS。
- 实线表示正式引用或已验证调用；虚线表示候选、事件、投影或外部接通边界。
- 图中的模块边界不是网络拆分建议，首阶段继续使用模块化单体和本地 Plugin。
- 图中的 Agent/SaaS 名称是执行适配示例；业务节点只绑定 Capability、Schema、ExecutionProfile 和固定引用。

## 2. 当前三执行平面架构图

这张图是当前系统的事实图，不把本地交互生产误画成云端 Runtime 工作流。

```mermaid
flowchart TB
    subgraph Local["current-local 本地交互生产平面"]
        Host["Implemented local Agent host paths\nCodex / Claude（发布层级不同）"]
        Plugin["Agent Plugin\nSkills + MCP + claims"]
        Workspace["Bound Workspace\n10-context / 20-sources / 30-knowledge\n40-work / 50-production / 60-delivery / 70-results"]
        Run["LocalRun 3.0\nClaim + CAS + Handoff 1.0"]
        Host --> Plugin
        Plugin --> Workspace
        Workspace --> Run
    end

    subgraph Server["current-server 服务端治理与审核平面"]
        BFF["CLI/API/BFF"]
        Submit["SubmissionBundle 3.0\nSubmissionRevision"]
        Review["Internal Review / Client Review"]
        Approved["ApprovedSnapshot"]
        Facts["Source / Knowledge / Rights\nArtifact / Delivery / Projections"]
        BFF --> Submit --> Review --> Approved
        Approved --> Facts
    end

    subgraph Runtime["runtime V8 自动化执行平面"]
        Job["WorkTask -> JobRun"]
        Node["NodeRun -> RuntimeAttempt"]
        Context["ContextView / State / Checkpoint"]
        Effect["Effect / Inbox / Outbox\nProvider reconciliation"]
        Job --> Node --> Context
        Node --> Effect
    end

    Plugin -->|"显式 publish/pull/status"| BFF
    Approved -->|"用户明确 pull"| Workspace
    Facts -->|"固定 refs + digest"| Job
    Runtime -. "业务结果事件" .-> Facts
```

当前 Codex/Claude 是已实现基线，不是封闭名单。Pi Agent、其他本地宿主或 Agent SaaS 只有通过统一能力、隔离、恢复、事件和回执门槛后才能进入已发布执行目录。

### 2.1 开放执行生态架构

```mermaid
flowchart TB
    SOP["Experience / SOP node"] --> Contract["Capability + input/output Schema\nExecutionProfile + fixed refs + budget"]

    subgraph Executors["可替换执行者"]
        Local["本地通用 Agent\nCodex / Claude / Pi / 其他 CLI\ncurrent-local / external-dependency"]
        Remote["远程/托管 Agent\ncurrent-server / external-dependency"]
        AgentSaaS["Agent Workflow SaaS\ncurrent-server / external-dependency"]
        CreativeSaaS["图片/视频/音频/排版 SaaS\npartial / external-dependency"]
        Worker["Deterministic Worker\ncurrent"]
        Browser["Browser / Computer Use\npartial / external-dependency"]
        Human["Human creator / reviewer / operator\ncurrent"]
    end

    Contract --> HostAdapter["Plugin Host Adapter（架构角色）\n安装/能力声明/环境摘要"]
    Contract --> Harness["Agent Harness Adapter\nDetect/Start/Resume/Interrupt/Inspect"]
    Contract --> Connector["Provider/Connector Adapter\nEffect/Webhook/Inspect/Receipt"]
    Contract --> HumanTask["Human Task / Runbook"]

    HostAdapter --> Local
    Harness --> Local
    Harness --> Remote
    Connector --> AgentSaaS
    Connector --> CreativeSaaS
    Connector --> Browser
    Contract --> Worker
    HumanTask --> Human

    Local --> Ingress["Candidate / Artifact / Event ingress"]
    Remote --> Ingress
    AgentSaaS --> Ingress
    CreativeSaaS --> Ingress
    Worker --> Ingress
    Browser --> Ingress
    Human --> Ingress

    Ingress --> Gate["Schema / digest / rights / cost / quality Gate"]
    Gate --> Existing["现有领域事实\nHandoff / Submission / ApprovedSnapshot\nArtifact / Delivery / Effect / Receipt"]
```

SOP 不直接引用 `codex`、`claude`、`pi` 或某个 SaaS 名称。品牌绑定属于已发布 ExecutionProfile；停用一个执行者不会改变历史内容、审批和交付事实。

## 3. 当前本地到服务端流程图

### 3.1 内容工作区主链

```mermaid
flowchart LR
    Context["workspace_context"] --> Source["source register / ingest"]
    Source --> Evidence["EvidenceBundle 3.0"]
    Evidence --> Claim["LocalRun claim"]
    Claim --> Knowledge["knowledge import / lint / pack"]
    Knowledge --> Brief["Brief 3.0\n或 ArticleBrief 1.0"]
    Brief --> Batch["ContentBatch 3.0"]
    Batch --> Item["ContentItem 3.0\n或 Article 1.0"]
    Item --> Lint["item lint -> batch lint -> finalize"]
    Lint --> Preflight["publish_preflight\nexact files + disclosure"]
    Preflight --> Confirm{"用户确认同一 plan_id?"}
    Confirm -->|否| Stop["不产生云端写入"]
    Confirm -->|是| Apply["publish_apply"]
    Apply --> Revision["SubmissionRevision"]
    Revision --> Review["服务端审核"]
    Review --> Decision{"批准?"}
    Decision -->|否| Changes["反馈拉取\n本地新修订"]
    Decision -->|是| Snapshot["ApprovedSnapshot"]
    Snapshot --> Pull["approved_snapshot_pull"]
```

### 3.2 公众号当前交付流程

```mermaid
flowchart LR
    Snapshot["Article ApprovedSnapshot"] --> Export["wechat_package_export"]
    Export --> Lint["wechat_package_lint"]
    Lint --> Package["WeChatDeliveryPackage 1.0\nHTML / Markdown / JSON / assets"]
    Package --> Operator["操作员登录公众号后台"]
    Operator --> Upload["手工上传素材和正文"]
    Upload --> Publish["手工预览并发布"]
    Publish --> Binding["外部绑定/结果另行登记"]
```

`Package` 生成和校验没有外部公众号副作用；`Publish` 不是 ContentCloud 可以代替用户声称完成的动作。

### 3.3 公众号排版与交付派生图

```mermaid
flowchart LR
    Approved["Article ApprovedSnapshot"] --> Blocks["语义内容块"]
    Blocks --> Layout["模板 + Design Token + CSS 内联"]
    Layout --> Sanitize["HTML allowlist / 链接与复杂块降级"]
    Sanitize --> Media["图片压缩 / 封面 / 占位符映射 / 上传顺序"]
    Media --> Preview["窄屏 / 长文 / 深浅背景 / 图片失败预览"]
    Preview --> Diff["平台编辑器清洗后差异检查"]
    Diff --> Package["HTML + Markdown + JSON + assets + manifest"]
    Package --> Manual["current-local 人工上传/预览/发布"]
    Package -. "partial / external-dependency" .-> API["草稿/发布 Adapter"]
    Manual --> Binding["external binding"]
    API --> Receipt["Effect / Inspect / Receipt"]
```

排版是 ApprovedSnapshot 的渠道派生，不能反向静默修改已批准正文。模板、转换器、渠道规格和所有图片派生都要固定版本与 digest。

## 4. 当前 V3 对象与血缘图

```mermaid
flowchart LR
    Material["WorkspaceMaterial\nversion + digest"] --> SourceRevision["SourceRevision"]
    SourceRevision --> Evidence["EvidenceBundle 3.0\nlocator + quote + digest"]
    Evidence --> KnowledgePages["Markdown knowledge pages"]
    KnowledgePages --> KnowledgePack["KnowledgePack 3.0"]
    KnowledgePack --> KnowledgeSubmit["knowledge Submission"]
    KnowledgeSubmit --> KnowledgeSnapshot["KnowledgeSnapshot 1.0"]

    KnowledgeSnapshot --> Brief["Brief / ArticleBrief"]
    Brief --> Batch["ContentBatch 3.0"]
    Batch --> Item["ContentItem / Article"]
    Item --> ContentSubmit["content_batch Submission"]
    ContentSubmit --> ContentApproved["content_batch ApprovedSnapshot"]
    ContentApproved --> Storyboard["StoryboardPackage 1.0"]
    Storyboard --> StoryboardSubmit["storyboard Submission"]
    StoryboardSubmit --> StoryboardApproved["storyboard ApprovedSnapshot"]
    StoryboardApproved --> Artifact["Artifact / media"]
    Artifact --> Delivery["DeliveryPackage / Seedance / WeChat package"]

    ContentApproved --> Result["CreativeResultAssetProjection"]
    Delivery --> Result
    Result --> Reuse["CreativeAssetRef\n下一任务固定引用"]
```

### 4.1 首个用户前的旧入口收口与删除图

这张图描述技术契约的破坏性收口；它不删除内容修订、批准快照、Artifact 或发布回执。

```mermaid
flowchart TB
    Duplicate["旧 Evidence 草案 / 虚假 Schema 标识\n顶层单工作区配置 / SOP 自动认领\n已退役 Runtime 与品牌专用入口"] --> Inventory["扫描代码 / API / Fixture / 文档 / 旁路"]
    Inventory --> Owner["选定唯一 current owner"]
    Owner --> Rewrite["一次性改内部 DTO、解析器、测试和 Fixture"]
    Rewrite --> Delete["删除旧常量、文件、别名、Fallback、导航和正向断言"]
    Delete --> Rebuild["重建无用户开发数据库"]
    Rebuild --> Guard["治理扫描 + Contract Tests"]
    Guard --> Current["唯一 current owner"]
    Guard --> Block["发现旧引用：阻断回流"]
```

不做在线双写、请求级版本协商或永久适配层。当前没有用户迁移义务，重复入口直接删除。

## 5. 当前两类 Handoff 与跨执行者 Agent

Studio 启动交接与 LocalRun 恢复不是同一状态机：前者选择宿主并固定目标，后者在已绑定 workspace 内延续受治理的运行。

```mermaid
flowchart LR
    Studio["Studio / Review UI"] --> API["通用 Agent Handoff API\ncontentcloud.agent-handoff/1.0"]
    API --> Catalog{"Client capability available?"}
    Catalog -->|Codex current| Codex["Codex Adapter\ndeep link + plugin + prompt"]
    Catalog -.->|Claude / Pi / SaaS\nAdapter + capability validation| Other["各自 Adapter\nlaunch / auth / validation"]
    Codex --> Context["宿主选择 workspace\n首次 workspace_context"]
    Other -.-> Context
    Context --> Verify{"project_id / scope / digest 匹配?"}
    Verify -->|否| Stop["停止，不扫描其他目录"]
    Verify -->|是| Local["LocalRun + contentcloud.handoff/1.0\nclaim / CAS / next_action"]
    Local --> Candidate["候选输出 / 检查 / Submission"]
```

`contentcloud.agent-handoff/1.0` 是通用契约，Codex 只是当前第一个可用 Adapter。新增 Claude Code、Pi Agent、远程 Agent 或 Agent SaaS 时，只新增客户端能力、启动/鉴权 Adapter 和契约测试，不新增品牌专用业务路由。

### 5.1 LocalRun 跨对话恢复时序

```mermaid
sequenceDiagram
    autonumber
    actor A as Agent/对话 A
    participant MCP as ContentCloud Local MCP
    participant Run as LocalRun 3.0
    participant Files as Workspace files
    actor B as 对话 B（当前同宿主；未来可异构）

    A->>MCP: workspace_context
    MCP-->>A: selected run + context_revision
    A->>MCP: local_run_claim(run_id, expected_revision)
    MCP-->>A: claim token
    A->>Files: 写入 workspace-relative outputs
    A->>MCP: checks + output refs
    MCP->>Run: CAS 保存新 revision
    A->>MCP: handoff_create_ready
    MCP->>Run: 保存 input/output digests + next_action
    MCP->>Run: 释放 claim
    B->>MCP: workspace_context
    MCP-->>B: ready Handoff
    B->>MCP: handoff_accept
    MCP->>Run: 校验 context revision 和文件 digest
    MCP-->>B: 新 claim token + next action
    B->>MCP: handoff_complete
```

这条链不依赖旧聊天记录，也不应被替换成一个把全文对话放入 Runtime 的通用 Envelope。当前已发布的交互式恢复路径仍以 Codex 为主；Handoff 契约和固定摘要边界应允许未来由 Claude Code、Pi Agent 或其他宿主接续，但必须分别验收，不能为未发布宿主保留运行时兼容分支。

### 5.2 跨 Agent、SaaS、Worker 和人工异步协作时序

```mermaid
sequenceDiagram
    autonumber
    actor User as 用户/主编
    participant CC as ContentCloud
    participant Agent as 任一已发布 Agent
    participant SaaS as Agent/Creative SaaS
    participant Worker as Deterministic Worker
    participant Human as 审核/发布人

    User->>CC: 固定 Brief、渠道、预算和允许的执行方式
    CC->>Agent: Capability + fixed refs + output schema
    Agent-->>CC: candidate + digest + handoff/events
    CC->>CC: schema/rights/cost/quality Gate
    CC->>SaaS: Effect(fixed refs, idempotency key)
    alt 明确完成
        SaaS-->>CC: external task ID + result refs + usage
    else 超时/连接中断
        CC->>SaaS: Inspect(task/idempotency ref)
        SaaS-->>CC: succeeded / failed / unknown
    end
    CC->>Worker: 排版/转码/渲染/打包/规格检查
    Worker-->>CC: Artifact + validation report
    CC->>Human: 固定版本审核和最终渠道预览
    Human-->>CC: approve / changes requested
    CC->>Human: 人工 Runbook 或发布授权
    Human-->>CC: external binding / receipt
```

Agent 会话 ID、SaaS task ID 和人工工单 ID 都只是执行引用；ContentCloud 的 Handoff、Revision、ApprovedSnapshot、Artifact、Delivery 和 Effect 才是跨执行者协作边界。

## 6. 当前审核与批准时序图

```mermaid
sequenceDiagram
    autonumber
    actor User as 客户/运营
    participant Local as Local Workspace
    participant API as ContentCloud API
    participant Review as Review Service
    participant Store as ApprovedSnapshot Store

    User->>Local: 完成本地 lint 和精确 preflight
    Local->>API: publish_apply(plan_id, accept=true)
    API-->>Local: SubmissionRevision + content_hash
    API->>Review: internal approval gate
    Review-->>API: internal decision
    API-->>User: 创建客户审核 Grant
    User->>Review: verify + approve/request changes
    alt request changes
        Review-->>API: feedback bound to revision
        API-->>Local: review feedback inbox
        Local->>Local: based_on_version 新修订
    else approve
        Review->>Store: 创建不可变 ApprovedSnapshot
        Store-->>API: snapshot id + digest
        API-->>Local: 可显式 pull 的批准快照
    end
```

## 7. 搜索、采集与证据物化流程图

Search/Fetch/Connector 的内部执行器已经存在；公开搜索源、平台趋势和企业账号属于 `external-dependency`。无论来源来自 API、网页、文件还是 SaaS，最终只能进入 `Source -> SourceRevision -> Evidence`。

```mermaid
flowchart TB
    Query["SearchQuery\n固定查询、区域、预算"] --> Policy{"租户策略、区域、预算允许?"}
    Policy -->|否| Block["blocked + 可行动原因"]
    Policy -->|是| Search["source.search\n真实受治理执行器"]
    Search --> Candidate["候选 URL/摘要\nSearchReceipt + cost"]
    Candidate --> Fetch["source.fetch\n白名单/SSRF/robots/大小限制"]
    Fetch --> Normalize["SourceRevision\n规范化 + digest + parser version"]
    Normalize --> Evidence["EvidenceBundle 3.0\nlocator + quote + collected_at"]
    Normalize --> Tombstone["tombstone / 删除传播"]
    Fetch -. "崩溃/超时" .-> Replay["cursor/lease 重放\n原子提交"]
    Replay --> Fetch
    Evidence --> Dedup["去重/冲突/权利检查"]
    Dedup --> Gate{"客户选择?"}
    Gate -->|本任务| Input["LocalRun input_refs"]
    Gate -->|项目参考| Reference["ProjectReference"]
    Gate -->|拒绝| Rejected["拒绝原因和重复反馈"]
```

Connector 不直接写 Knowledge、Content 或 Publication；它只产生可追溯的 SourceRevision/Evidence，后续知识提取和内容生产必须重新经过项目权限与批准 Gate。

## 8. Runtime 自动化执行图

Runtime 只适用于需要持久化自动化、服务端 Worker 或受控 Provider 的任务节点；普通本地交互不应强制进入此图。

```mermaid
flowchart LR
    WorkTask["WorkTask"] --> Admission["Experience/SOP/Capability binding\n固定版本和摘要"]
    Admission --> Job["JobRun"]
    Job --> Node["NodeRun"]
    Node --> Attempt["RuntimeAttempt\nlease + fence"]
    Attempt --> Context["ContextView\nrefs + digests + policy"]
    Context --> Agent["Agent Harness\nCodex / Claude / Pi / other\ncurrent-server / external-dependency"]
    Context --> SaaS["Remote Agent / Agent SaaS\ncurrent-server / external-dependency"]
    Context --> Worker["Deterministic Worker"]
    Context --> Provider["Model/Media Provider"]
    Agent --> Validate["Schema / owner / rights / cost checks"]
    SaaS --> Validate
    Worker --> Validate
    Provider --> Validate
    Validate --> Event["JobEvent / outbox"]
    Event --> Domain["Source/Content/Artifact/Delivery owner"]
    Provider --> Effect["Effect\nunknown / reconcile / bill"]
    SaaS --> Effect
    Effect --> Domain
```

### 8.1 Agent SaaS 回调与崩溃重放时序

```mermaid
sequenceDiagram
    autonumber
    participant SaaS as Agent SaaS
    participant API as Callback API
    participant Inbox as runtime_provider_inbox
    participant Attempt as RuntimeAttempt/Session
    participant Runtime as Runtime dispatcher
    participant Domain as ContentCloud domain

    SaaS->>API: signed callback(session_id, event_id, sequence, payload)
    API->>API: verify HMAC / clock skew / body limit
    API->>Inbox: insert received + payload_digest
    Inbox-->>API: accepted / duplicate / conflict
    API-->>SaaS: 202 + inbox_id
    Inbox->>Attempt: validate tenant, task, session and monotonic sequence
    Attempt->>Runtime: RecordHarnessEvent / FinalizeDispatch / YieldDispatch
    Runtime->>Domain: candidate/result/usage/event
    Runtime->>Inbox: mark applied
    alt worker crash before apply
        Inbox->>Runtime: replay received event
        Runtime->>Inbox: applied (idempotent)
    else same event id with different digest
        Inbox-->>Runtime: conflict -> human reconciliation
    end
```

## 9. 渠道发布、回执与对账时序图

Channel Binding、Publication、Callback、Inspect、Reconcile 和 Performance 已是服务端当前能力；真实平台账号/API 仍标记为 `external-dependency`。`published` 只能由外部回执或 Inspect 对账进入。

```mermaid
sequenceDiagram
    autonumber
    actor Approver as 发布授权人
    participant Delivery as Delivery/Channel Adapter
    participant Rights as Rights/Policy
    participant RT as Runtime Effect
    participant Platform as 外部平台
    participant Inbox as Receipt Inbox

    Approver->>Delivery: 选择已批准内容和固定 Artifact
    Delivery->>Rights: 校验账号、用途、权利、渠道规格
    Rights-->>Delivery: eligible + policy digest
    Delivery->>RT: Prepare effect(idempotency key)
    RT-->>Delivery: effect_ref + fence
    Delivery->>Platform: Submit(effect_ref, fixed refs)
    alt 平台明确接收
        Platform-->>Delivery: external_id + accepted
        Delivery->>RT: Finalize submitted receipt
    else 超时/连接中断
        Delivery->>RT: Mark unknown
        Delivery->>Platform: Inspect by idempotency/external ref
        alt 确认已发布
            Platform-->>Delivery: published + external_id
            Delivery->>RT: Reconcile success
        else 确认不存在
            Delivery->>RT: Policy-controlled retry
        else 仍然未知
            Delivery->>RT: Human reconciliation required
        end
    end
    Platform-->>Inbox: signed callback / metrics
    Inbox->>RT: dedup + bind effect + save receipt
```

## 10. 三个高价值场景的纵向血缘图

### 10.1 微信公众号排版流水线

```mermaid
flowchart TB
    Article["Article ApprovedSnapshot"] --> Blocks["semantic blocks\nheading / paragraph / quote / image / CTA"]
    Blocks --> Template["template + design tokens\nversion + digest"]
    Template --> Inline["inline CSS + HTML allowlist"]
    Inline --> Assets["asset mapping\ncover / body images / upload order"]
    Assets --> Preview["mobile preview\nlong text / dark mode / missing image"]
    Preview --> Diff["DOM digest / platform sanitize diff"]
    Diff --> Package["WeChatDeliveryPackage\nHTML + Markdown + JSON + manifest"]
    Package --> Manual["manual backend publish\nexternal dependency"]
    Manual --> Receipt["ChannelPublication + external receipt"]
```

### 10.2 小说 Canon 与连续性流水线

```mermaid
flowchart LR
    Premise["题材 / 命题 / 读者"] --> Canon["Novel Canon\n世界观 / 角色 / 术语 / 正史版本"]
    Canon --> Outline["Novel Outline\n卷幕 / 章节 / 伏笔台账"]
    Outline --> Chapter["Novel Chapter\n候选 -> 修订 -> ApprovedSnapshot"]
    Chapter --> Lint["continuity lint\n时间 / 地点 / 状态 / 知识边界"]
    Lint --> Release["Novel Release\n章节包 / 封面 / 元数据 / 排期"]
    Release --> Delivery["DeliveryPackage"]
    Delivery --> Publication["ChannelPublication\n平台上架/连载"]
    Publication --> Receipt["外部回执 + 章节指标"]
    Receipt -. "反馈候选" .-> Outline
```

### 10.3 抖音电商完整发布血缘

```mermaid
flowchart TB
    Audience["AudienceStrategy\nApprovedSnapshot"] --> Offer["CommerceOffer\nApprovedSnapshot"]
    Offer --> Content["ContentItem\nApprovedSnapshot"]
    Content --> Storyboard["StoryboardPackage\nApprovedSnapshot + locked digest"]
    Storyboard --> Produce["实拍 / Agent / 视频 SaaS / Worker"]
    Produce --> Artifact["最终成片 Artifact\nmedia digest"]
    Artifact --> Delivery["DeliveryPackage manifest\n封面 / 字幕 / 落地页 / 商品锚点"]
    Offer --> Validate["DouyinCommerceValidationReceipt\n价格 / 币种 / 权益 / 条件 / 文本 digest"]
    Content --> Validate
    Storyboard --> Validate
    Artifact --> Validate
    Validate --> Prepare["PrepareChannelPublication\ntyped refs + profile"]
    Delivery --> Prepare
    Prepare --> Publication["ChannelPublication\naccount / schedule / idempotency"]
    Publication --> Callback["Callback / Inspect / Reconcile"]
    Callback --> Performance["PerformanceObservation\n播放 / 完播 / 点击 / 成交"]
```

## 11. 当前状态图

### 11.1 内容对象状态

```mermaid
stateDiagram-v2
    [*] --> candidate
    candidate --> blocked: lint/rights/knowledge failure
    candidate --> review_ready: local checks pass
    review_ready --> submitted: publish_apply
    submitted --> internally_approved: server decision
    internally_approved --> client_review: grant created
    client_review --> changes_requested: client requests changes
    changes_requested --> candidate: new based_on version
    client_review --> approved: client approves
    approved --> delivered: delivery created or external binding recorded
    approved --> superseded: newer approved revision
    delivered --> superseded: newer delivered revision
```

### 11.2 外部 Effect 状态

```mermaid
stateDiagram-v2
    [*] --> registered
    registered --> prepared
    prepared --> submitted
    submitted --> succeeded
    submitted --> failed
    submitted --> unknown
    unknown --> reconciling
    reconciling --> succeeded
    reconciling --> failed
    reconciling --> human_required
```

公众号人工上传不进入自动 Effect 状态；它使用 WeChatDeliveryPackage 和后续外部绑定记录。

### 11.3 ChannelPublication 状态机

```mermaid
stateDiagram-v2
    [*] --> prepared
    prepared --> manual_action_required: 无可用 API / 需要登录态
    prepared --> submitted: Submit effect accepted
    prepared --> failed: preflight rejected
    manual_action_required --> submitted: 操作员确认已提交
    submitted --> unknown: 超时或连接中断
    submitted --> failed: 外部明确拒绝
    unknown --> submitted: Inspect/Callback 发现已接收
    unknown --> prepared: Inspect 确认未产生副作用
    submitted --> published: 外部 Receipt/Inspect 明确成功
    published --> withdrawn: 外部撤回回执
    published --> published: 指标/重复回执幂等
    failed --> prepared: 新的授权和版本
    withdrawn --> [*]
```

本地验证、模型输出、Worker 成功或 ContentCloud 内部状态都不能直接把 Publication 标记为 `published`。

## 12. 当前部署拓扑与外部边界

```mermaid
flowchart TB
    subgraph Device["用户设备"]
        Browser["Web Studio / Admin"]
        CLI["ContentCloud CLI"]
        Host["Local Agent host paths\nCodex / Claude / Pi / other\ncurrent-local / external-dependency"]
        Plugin["Plugin Skill + MCP"]
        Workspace["Bound Workspace files"]
        Browser --> API
        CLI --> API
        Host --> Plugin --> Workspace
        Workspace --> CLI
    end

    subgraph Cloud["ContentCloud 模块化单体"]
        API["Go Server / BFF / CLI API"]
        Domains["Business Domains\nWork / Source / Knowledge / Review / Delivery"]
        Runtime["V8 Runtime\nJob / Node / Attempt / Effect"]
        Worker["Worker\nparse / media / projection / reconcile"]
        API --> Domains
        API --> Runtime
        Domains <--> Runtime
        Worker <--> Runtime
        Worker --> Domains
    end

    subgraph Data["数据基础"]
        PG["PostgreSQL + RLS"]
        Blob["Object Storage + digest"]
        Secret["Secret / Environment lock"]
        Obs["Logs / Metrics / Traces"]
    end

    subgraph External["外部系统（按能力逐步接入）"]
        Search["Search/Crawl API\ncurrent-server / external-dependency"]
        Models["Cloud Model / vLLM / SGLang\ncurrent-server / external-dependency"]
        Media["Media Provider\npartial / external-dependency"]
        AgentSaaS["Remote Agent / Agent SaaS\ncurrent-server / external-dependency"]
        CreativeSaaS["Creative/Layout/Edit SaaS\npartial / external-dependency"]
        Channels["WeChat manual / Channel API\ncurrent-local / external-dependency"]
    end

    Domains --> PG
    Runtime --> PG
    Worker --> Blob
    Domains --> Blob
    API --> Secret
    Runtime --> Obs
    Worker --> Obs
    Worker --> Search
    Worker --> Models
    Worker -. "partial / external-dependency" .-> Media
    Runtime --> AgentSaaS
    Worker -. "partial / external-dependency" .-> CreativeSaaS
    Domains -. "current-local / external-dependency" .-> Channels
```

## 13. 故障恢复决策图

```mermaid
flowchart TB
    Failure["节点或外部调用异常"] --> SideEffect{"可能产生外部副作用?"}
    SideEffect -->|否| Retryable{"错误类型允许重试?"}
    Retryable -->|是| Budget{"预算/截止/次数允许?"}
    Budget -->|是| Attempt["创建新 RuntimeAttempt\n从 ContextView/Checkpoint 恢复"]
    Budget -->|否| Human["人工处置"]
    Retryable -->|否| Blocked["节点失败 + 恢复动作"]
    SideEffect -->|是| Known{"外部结果已确认?"}
    Known -->|成功| Finalize["固定 Artifact/费用/回执"]
    Known -->|明确不存在| EffectRetry{"策略允许受控重试?"}
    EffectRetry -->|是| NewEffect["复用 Effect 意图\n创建新尝试"]
    EffectRetry -->|否| Human
    Known -->|未知| Inspect["Inspect / Callback / Bill 对账"]
    Inspect -->|成功| Finalize
    Inspect -->|不存在| EffectRetry
    Inspect -->|仍未知| Human
```

## 14. 图谱与事实来源

| 图 | 事实来源 | 状态 |
| --- | --- | --- |
| 三执行平面 | Workspace Skill、LocalRun、Submission、Foundation、V8 | `current` |
| 本地到服务端主链 | CLI/MCP publish、V3 contracts、Review tests | `current` |
| 公众号交付 | WeChatDelivery contract/Skill/CLI | `current-local` |
| V3 对象血缘 | localworkspace、app submission、asset projections | `current-server` / `current-local` |
| 搜索采集 | `internal/sourceinfra`、`internal/connector`、SourceRevision/Evidence | `current-server` / `external-dependency` |
| Runtime 自动化 | Runtime V8、Effect、Provider inbox | `current-server` |
| 开放执行生态 | AgentHarnessAdapter、ExecutionProfile、Plugin claims、Effect | `current-server` / `external-dependency` |
| 公众号排版派生 | Article/WeChatDelivery、DOM digest、移动 lint | `current-local` |
| 跨 Agent/SaaS 协作 | LocalRun/Handoff、Agent callback、Effect | `current-server` / `external-dependency` |
| 自动渠道发布 | Channel Binding/Publication/Callback/Reconcile/Performance | `current-server` / `external-dependency` |
