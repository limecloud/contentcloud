# V2 业务流程、Gate 与恢复机制

## 1. Gate 总览

V2 保留 V1 Gate 0-5 的语义并细化输入，不为九域各造一套门禁。

```mermaid
flowchart LR
    G0[Gate 0<br/>客户项目与来源就绪] --> G1[Gate 1<br/>可信知识就绪]
    G1 --> G2[Gate 2<br/>情报、策略与 Brief 就绪]
    G2 --> G3[Gate 3<br/>剧本编译通过]
    G3 --> G4[Gate 4<br/>内审与客户批准]
    G4 --> X[外部制作/投放]
    X --> G5[Gate 5<br/>结果回流与学习]
    G5 --> G2
```

Gate 是条件集合，不是页面步骤，不跟随 Run 成功自动前进。GateDecision 必须记录检查结果、豁免、责任人和时间。

## 1.1 四种执行路径

| 路径 | 发起位置 | 云端对象 | 适用范围 |
| --- | --- | --- | --- |
| 本地交互 | Codex/Claude Code/CLI | 无 TaskRun | ingest、query、策略、Brief、生成、修订 |
| 显式发布 | 本地 CLI | SubmissionRevision | 知识、Brief、剧本等门禁检查点 |
| 云端治理 | Web | Review/Decision/ApprovedSnapshot | 批注、批准、退回、锁版、影响 |
| 云端自动化 | Scheduler/Event/Web remote run | TaskRun/Attempt/RunOutput/Submission | 监控、复盘、Follow-up、远程批处理 |

普通本地交互不得为了记录过程而创建空的云端 TaskRun。

## 2. 客户接入与项目创建

```mermaid
sequenceDiagram
    autonumber
    actor PM as 项目负责人
    participant Web as ContentCloud Web
    participant BFF as 云端 BFF
    participant CLI as contentcloud CLI
    participant D as Creative Runtime

    PM->>Web: 创建客户/品牌/产品/项目
    Web->>BFF: 保存角色、模板和审批人
    BFF-->>Web: 项目 + Gate 0 检查
    PM->>Web: 生成一次性连接码
    Web-->>PM: 安装命令和短期连接码
    PM->>CLI: contentcloud init --connect <code> ./project
    CLI->>BFF: 消费init code并获取签名模板
    BFF-->>CLI: 项目绑定 + WorkspaceTemplateManifest
    CLI->>CLI: 初始化目录、Skills、MCP、lint
    CLI->>BFF: 注册WorkspaceBinding与模板版本
    BFF-->>Web: 工作区就绪，更新Gate 0
```

Gate 0 退出条件：角色完整、审批人可达、服务模板已锁定、至少一个本地工作区完成 doctor。只有启用 Automation 时才要求获授权 Daemon 在线；普通本地创作不依赖后台轮询。

## 3. 客户素材诊断与四层上下文

```mermaid
flowchart TB
    A[选择租户服务模板] --> B[登记客户/品牌/产品资料]
    B --> C[本地 raw/inbox 登记来源与hash]
    C --> D[本地解析/OCR/候选提取]
    D --> E[15维覆盖诊断]
    E --> F[七层 KnowledgePack candidate]
    F --> G{来源、冲突、缺口检查}
    G -- 不足 --> H[补料/客户决策清单]
    H --> B
    G -- 可审 --> I[人工事实、主张、权利决策]
    I --> J[本地 KnowledgePack candidate]
    J --> K[publish Knowledge Submission]
    K --> L[云端审核与 ApprovedSnapshot]
    L --> M[本地 pull 并继续]
```

方法论、知识包和本体知识不得混成一个长文档。方法论说明收集什么，知识包组织 Agent 需要理解什么，本体知识决定哪些事实和表达具备资格。

## 4. Gate 1：本地知识工程与云端审核

```mermaid
sequenceDiagram
    autonumber
    actor S as 策略/资料人员
    actor R as 审核员
    participant CLI as contentcloud CLI/MCP
    participant Agent as 本地 Agent
    participant API as 云端
    participant Web as Web审核

    S->>CLI: 把资料放入raw/inbox并发起ingest
    CLI->>Agent: 初始化LocalRunContext并执行提取
    Agent-->>CLI: KnowledgeCandidates
    CLI->>CLI: lint/query/knowledge-pack/preflight
    CLI-->>S: 展示差异、冲突、缺口和来源披露
    S->>CLI: 确认publish knowledge --review
    CLI->>API: SubmissionBundle + evidence packs
    API-->>Web: needs_review SubmissionRevision
    R->>Web: 按 ID 决策 Fact/Claim/Rights
    Web->>API: 记录决定并生成ApprovedSnapshot
    S->>CLI: pull approved
    CLI->>API: 获取DecisionDelta/ApprovedSnapshot
    CLI-->>S: 放入只读cache/inbox，显示Gate 1结果
```

失败规则：无法定位原文、证据越界、冲突未解决、权利未知或关键事实缺失时保持 blocked；不能用模型常识补齐。

## 5. 市场情报到策略

```mermaid
flowchart LR
    Q[本地研究问题/可选持续监控] --> R[客户端公网与授权企业资料研究]
    R --> C[案例/趋势候选]
    C --> F[框架和镜头模式拆解]
    F --> I[带来源的 Insight]
    I --> A{人工采纳}
    A -- 拒绝 --> X[保留历史]
    A -- 采纳 --> S[StrategyVersion 候选]
    K[品牌已决知识] --> S
    S --> V[卖点排序与 VisualizationPlan]
```

市场情报回答“什么结构值得测试”，品牌知识回答“本品牌什么可以说和展示”。两者在 StrategyVersion 中组合，但资格和引用类型保持分离。

## 6. Gate 2：策略、内容计划与 Brief

```mermaid
flowchart TB
    A[本地选择渠道/目标/受众] --> B[选择需求时刻]
    B --> C[卖点排序]
    C --> D[为核心卖点创建可视化方案]
    D --> E{画面能证明且可实现?}
    E -- 否 --> F[换主体/场景/道具/Plan B]
    F --> D
    E -- 是 --> G[创建 Campaign/Experiment]
    G --> H[生成 BriefVersion]
    H --> I[本地引用/权利/单变量校验]
    I -- 失败 --> H
    I -- 通过 --> J[内部审核]
    J -- 本地preflight通过 --> K[publish Brief Submission]
    K --> L[云端审核Approved Brief]
    L --> M[本地pull后Gate 2 ready]
```

退出条件：approved StrategyVersion、approved VisualizationPlan、approved BriefVersion、目标渠道和测量窗口明确、禁止项无冲突。

## 7. Gate 3：剧本生产

```mermaid
sequenceDiagram
    autonumber
    actor E as 编导
    participant CLI as 本地CLI/MCP
    participant A as 本地 Agent/Skill
    participant V as 确定性 Validator
    participant API as 云端Submission API
    participant Web as 云端审阅

    E->>CLI: 使用ApprovedSnapshot创建CreativeBatch
    CLI->>A: 本地交互执行，无云端TaskRun
    A-->>CLI: ScriptPackage V2 + 扩展产物
    CLI->>V: 本地Schema/引用/权利/连续性校验
    alt blocked
        V-->>E: CreativeDraft + 原因 + 补料
    else review_ready
        V-->>E: 本地review_ready候选
        E->>CLI: publish script --review
        CLI->>API: 不可变Script Submission
        API-->>Web: 创建ReviewCycle
    end
```

普通本地生成没有云端 Run 状态；只有本地 lint 通过并成功 publish 的 SubmissionRevision 才进入云端 review。Automation Run succeeded 仍只表示执行协议完成。

## 8. Gate 4：审核、修订与批准

```mermaid
flowchart TB
    A[ScriptVersion review_ready] --> B[内部 ReviewCycle]
    B --> C{阻断批注?}
    C -- 是 --> D[本地pull反馈并创建revise LocalRunContext]
    D --> E[新不可变版本 + 结构化 diff]
    E --> P[publish新SubmissionRevision]
    P --> B
    C -- 否 --> F[内部批准]
    F --> G[生成客户审批 Grant]
    G --> H[邮件 OTP 验证]
    H --> I{客户决策}
    I -- 退回 --> D
    I -- 批准 --> J[锁定批准 hash]
    J --> K[Gate 4 ready]
```

客户只能看到 client-visible 批注、安全引用摘要和允许展示的素材。链接过期、撤销、邮箱不匹配或对象状态变化时拒绝决策。

## 9. 交付与外部制作

```mermaid
sequenceDiagram
    autonumber
    actor E as 编导
    actor P as 外部制作人员
    participant Web as ContentCloud
    participant Export as 确定性导出器
    participant Store as 对象存储

    E->>Web: 从客户批准版本创建交付包
    Web->>Export: canonical ScriptPackage + manifest
    Export->>Store: JSON/Markdown/XLSX
    Export-->>Web: hashes + ProductionHandoff
    E-->>P: 发送受控交付链接/下载
    P->>Web: 确认收件并更新制作状态
    P->>Web: 关联成片或外部位置
    Web->>Web: 记录审计，不执行视频生成
```

## 10. Gate 5：结果与学习

```mermaid
flowchart LR
    A[导入 CSV/XLSX/人工快照] --> B[ImportBatch 校验]
    B -- 字段/窗口/分母不足 --> Q[quarantined]
    B -- 通过 --> O[不可变 Observation]
    O --> M[确定性指标计算]
    M --> L[candidate Learning]
    L --> H{人工判断}
    H -- 拒绝 --> R[记录原因]
    H -- 采纳 --> I[创建策略/Brief影响动作]
    I --> N[下一 Campaign/Experiment]
```

样本不足只能记录现象。自然和付费流量、平台和门店结果、不同统计窗口不能静默混算。

## 11. 来源变化与影响重编

```mermaid
sequenceDiagram
    autonumber
    actor R as 审核员
    participant S as Source/Knowledge
    participant L as Lineage Service
    participant B as Business Objects
    participant A as Audit/Notification

    R->>S: 修订来源/权利/事实状态
    S->>L: 发布 upstream.changed
    L->>L: 遍历显式 lineage edge
    L->>B: 标记当前下游 review_required
    L->>A: 创建 ImpactAction 和通知
    A-->>R: 受影响策略/Brief/剧本/交付列表
    R->>B: 复核、退役或发起重编
```

历史 ApprovalDecision 保持不变；当前版本可用性和历史上曾批准是两个不同事实。

## 12. Automation 业务流程

```mermaid
flowchart TB
    T[选择受治理模板] --> P[配置业务范围/触发/负责人/设备]
    P --> V[确定性校验]
    V -- 失败 --> P
    V -- 通过 --> A[启用 PlanVersion]
    A --> X{remote/event/schedule}
    X --> R[创建 TaskRun]
    R --> E[客户端执行]
    E --> O[RunOutput]
    O --> S[自动创建SubmissionRevision]
    S --> I[进入业务待审核队列]
    I --> H{人工或确定性规则处理}
    H --> B[业务对象]
```

自然语言变更：

```mermaid
sequenceDiagram
    actor U as 用户
    participant Web as Web
    participant API as 云端
    participant D as 客户端
    actor PM as 项目负责人

    U->>Web: 用自然语言描述调整
    Web->>API: 创建 PlanChangeRequest
    D->>API: 领取 change capability
    API-->>D: 当前 PlanVersion + 用户意图
    D->>API: 返回结构化 diff + 风险摘要
    API-->>PM: 待确认变更
    PM->>API: 批准或拒绝
    API->>API: 批准时创建新 PlanVersion
```

## 13. 恢复和人工干预

| 故障 | 系统行为 | 人工动作 |
| --- | --- | --- |
| 客户端离线 | Run 保持 queued 或按模板策略 skipped | 连接设备或 run once |
| 租约过期 | Attempt 记 expired，Run 可重新租约 | 查看是否存在外部副作用 |
| Capability 不匹配 | 不投递，显示所需版本 | 升级客户端或换设备 |
| 输出 Schema 失败 | Run failed，原始安全摘要保留 | 修复客户端并新建 attempt/run |
| 业务校验 blocked | Run 可 succeeded，产物 blocked | 补料、决策或改 Brief |
| 通知失败 | 不回滚业务事务，进入重试队列 | 检查渠道配置 |
| 上游失效 | 下游 review_required | 复核、退役或重编 |

## 14. 通知

V2 首发站内加邮件。事件包括：任务失败、长时间无心跳、知识待审、决策请求、Brief/剧本待审、客户审批、链接将到期、交付完成和上游影响。

通知只包含租户允许的项目名称、对象类型、状态、责任人和安全链接；不包含原始客户内容、Agent transcript、模型信息或密钥。
