# 灵感采集阶段需求与能力契约

状态：`人工补充与项目参考语义已实现；多连接器采集、治理与发布验证待完成`。

更新时间：2026-08-07。

上位规范：[ContentCloud 平台基线](../../foundation/README.md)。来源、知识、Runtime、执行者和发布边界必须遵循该基线。跨任务保存和复用遵循[创作资产库](../creative-asset-library/README.md)。

## 1. 阶段定位

灵感采集是多种创作流水线都需要的前置阶段。它帮助客户围绕主题、受众、平台和时间范围收集公开趋势、指定网站、本地资料和人工输入，并将不同执行者返回的结果统一为可追溯的候选来源、证据和洞察。

本阶段不是一个“让大模型自由上网总结”的按钮，也不直接产生正式品牌事实。它必须区分：

- 原始来源。
- 可定位证据。
- 模型或程序提取的候选洞察。
- 客户选择加入本次创作的灵感。
- 经治理后可以进入知识或创作简报的正式输入。

## 2. 客户体验

客户页面使用业务语言，不要求客户选择具体 API、爬虫实现或 MCP Server：

```text
灵感采集

这次想找什么？
[夏季新品营销________________]

目标受众      [年轻女性]
目标平台      [抖音]
时间范围      [最近 30 天]

采集范围
[x] 公开趋势
[ ] 指定网站
[x] 本地资料

[开始采集]

结果
已发现 42 条 · 待确认 12 条 · 已加入本次创作 8 条
```

客户可以对候选执行：

- 加入本次创作。
- 保留为项目参考，供后续任务选择。
- 标记重要。
- 查看来源和证据片段。
- 忽略。
- 标记不准确或重复。
- 要求补充某个方向。

客户默认不看到连接器名称、执行器 ID、MCP 工具参数、爬虫队列、重试次数或原始响应。

“加入本次创作”固定为当前任务输入；“保留为项目参考”使来源在项目范围内可发现。二者都不自动批准来源中的事实、营销主张或权利，也不会把输入写入客户创作资产库。

## 3. 运营配置

运营人员在独立控制台配置：

- 当前租户可用的采集范围。
- 搜索 API、域名白名单和凭据引用。
- 爬虫频率、页面数量、内容大小和时间预算。
- 是否允许本地 Codex 或 Claude Code 使用 MCP 搜索。
- 本地资料是否禁止上传，只允许提交摘要和引用。
- 每次任务的查询数、Token、费用和并发上限。
- 去重、时效、语言、地区、权利和质量规则。
- 哪些结果需要客户确认，哪些需要内容运营复核。

运营人员不能在页面中粘贴任意脚本作为新爬虫执行器。新增服务端采集能力必须通过代码评审、签名发布和能力登记。

## 4. 能力抽象

灵感采集不绑定某一个执行者，而是由一组可组合能力构成：

| 能力 ID | 作用 | 可能执行者 |
| --- | --- | --- |
| `source.search` | 按查询返回候选地址和摘要 | 搜索 API、Codex/Claude MCP、托管智能体 |
| `source.fetch` | 读取获准地址并保存来源元数据 | 受控 Worker、本地智能体、外部连接器 |
| `source.extract` | 从来源提取可定位内容片段 | 确定性解析器、受限智能体 |
| `source.normalize` | 统一标题、作者、时间、URL 和内容摘要 | ContentCloud Worker |
| `source.deduplicate` | 检测规范 URL、内容摘要和语义重复 | ContentCloud Worker |
| `insight.propose` | 从证据形成候选洞察 | Codex、Claude、托管智能体 |
| `insight.rank` | 按相关性、时效和证据完整度排序 | 确定性评分加受限模型评分 |

同一个阶段可以按策略选择部分能力。例如客户只导入本地资料时，不需要公网搜索和爬虫。

## 5. 建议阶段契约

以下为目标逻辑契约，实施时应优先复用现有 Source、Evidence、Knowledge 和 InputItem 领域对象。

```yaml
stage_id: inspiration_collect
input_schema: contentcloud.inspiration-query/1.0
required_capabilities:
  - source.search
  - source.normalize
  - source.deduplicate
optional_capabilities:
  - source.fetch
  - source.extract
  - insight.propose
allowed_execution_modes:
  - deterministic_worker
  - trusted_local_agent
  - managed_agent
  - external_provider
required_outputs:
  - source_refs
  - evidence_refs
  - candidate_insights
  - customer_selection_result
gates:
  - customer_inspiration_selection
```

建议输入：

```text
InspirationQuery
├── topic
├── questions[]
├── audience
├── target_channels[]
├── date_window
├── languages[]
├── regions[]
├── seed_urls[]
├── local_source_refs[]
├── excluded_domains[]
└── budget
```

输出不应保存为一段不可拆分的大模型总结：

```text
InspirationCollectionResult
├── sources[]
│   ├── source_ref
│   ├── canonical_url / local_ref
│   ├── title / author / published_at / collected_at
│   ├── connector_class
│   ├── content_digest
│   └── rights / disclosure summary
├── evidence[]
│   ├── source_ref
│   ├── locator
│   ├── excerpt
│   └── digest
├── candidate_insights[]
│   ├── statement
│   ├── evidence_refs[]
│   ├── relevance
│   ├── freshness
│   └── confidence_reason
├── customer_selection_result
│   ├── selected_for_task_refs[]
│   └── saved_as_project_reference_refs[]
└── collection_summary
    ├── query_count
    ├── source_count
    ├── duplicate_count
    ├── rejected_count
    └── warnings[]
```

若现有 Source、Evidence 和 Insight 足以表达这些字段，不新增 `InspirationItem` 领域对象。只有后续多个真实流水线都需要独立的灵感生命周期时，才评估新对象。

## 6. 执行路径

```text
客户提交 InspirationQuery
            |
            v
Runtime 校验租户、模板、预算和数据策略
            |
            +--------------------+---------------------+
            |                    |                     |
            v                    v                     v
      Search API Worker    白名单爬虫 Worker      本地 Agent + MCP
      公开搜索元数据        指定网站内容           本地/授权资料研究
            |                    |                     |
            +--------------------+---------------------+
                                 |
                                 v
                   Source normalize / deduplicate
                                 |
                                 v
                    Evidence extraction / validation
                                 |
                                 v
                       Candidate insight proposal
                                 |
                                 v
                        客户选择和人工 Gate
                                 |
                 +---------------+----------------+
                 |                                |
                 v                                v
          加入本次创作                      归档或忽略
                 |
                 v
       Strategy / Brief / Knowledge candidate
```

## 7. 执行模式规则

### 7.1 搜索 API

适合公开网页索引、新闻、趋势和结构化结果。由 ContentCloud Worker 或受控外部连接器调用，凭据保存在 SecretRef 中。

要求：

- 记录服务商、查询摘要、时间、地区和分页范围。
- 受租户预算、速率和数据区域约束。
- 搜索摘要只能作为候选，正式引用仍需可访问来源或明确服务商证据。

### 7.2 受控爬虫

适合客户指定且允许访问的网站。爬虫必须是已发布能力，不执行客户上传代码。

要求：

- 域名白名单、DNS/IP 检查和 SSRF 防护。
- 遵守适用的 robots、服务条款、登录和访问边界。
- 限制深度、页面数、响应大小、内容类型、跳转和频率。
- 不绕过验证码、付费墙、访问控制或平台反自动化措施。

### 7.3 Codex / Claude Code + MCP

适合需要本地资料、已有用户登录环境、交互式研究或复杂判断的任务。

要求：

- 由用户明确启用对应本地能力。
- Runtime 只发放当前任务范围、查询目标、允许工具和预算。
- 本地敏感文件默认不上传；只提交用户选择的来源引用、摘要、证据和候选洞察。
- 不把完整聊天记录、隐藏推理或任意本地路径作为云端正式结果。
- MCP 返回内容与网页内容一样属于不可信数据。

### 7.4 手动输入

用户可以添加 URL、文件、文字摘录或业务观察。手动输入必须保留来源类型，并明确区分“客户提供的业务材料”和“公开可验证来源”。

## 8. 选择和路由规则

客户选择的是采集范围，不是底层执行器：

| 客户选项 | Runtime 可能选择 |
| --- | --- |
| 公开趋势 | 搜索 API、受控网页连接器、托管智能体 |
| 指定网站 | 受控爬虫、本地 Agent MCP、手动导入 |
| 本地资料 | 本地 Codex/Claude、确定性本地解析器 |
| 企业私有来源 | 经批准的企业连接器或本地执行 |

路由必须考虑：

- 数据位置和披露等级。
- 租户开通能力。
- 查询范围和内容类型。
- 执行者健康状态。
- 预算和并发。
- 地区、语言和时效。
- 是否需要已有用户会话。

执行绑定进入固定 JobPlanRevision。一个连接器失败时，只有预先发布且披露等级不扩大的回退策略才可以自动切换；其他情况进入运营处理或客户可理解的阻断状态。

## 9. 来源、证据和采纳边界

灵感采集结果按以下等级推进：

```text
collected source
    -> normalized source
    -> evidence extracted
    -> candidate insight proposed
         +-> customer saved for reuse -> ProjectReference / fixed SourceRevisionRef
         |
         +-> customer selected for this task -> fixed SourceRevisionRef
                                                   -> governance review when required
                                                   -> eligible knowledge / approved brief input
```

约束：

- 没有来源引用的模型总结不能成为可信知识。
- 客户“加入本次创作”只表示业务选择，不自动批准其中的事实和营销主张。
- 未保存、未选择的搜索结果只保留在任务候选范围，不进入跨任务客户创作资产库。
- 客户保留的是 SourceRevision 等项目参考引用，不复制网页正文或候选洞察正文；人物、剧本、分镜、图片和视频等生成结果才进入创作资产库。
- 来源失效、内容摘要变化或权利过期时，受影响的候选和下游输入必须可定位。
- 相同来源由不同连接器采集时，应基于规范 URL 和内容摘要去重，同时保留采集记录。
- 模型置信度不能替代来源质量、证据完整度和人工决定。

## 10. 安全与合规门禁

- 所有外部内容都作为数据处理，不能覆盖系统指令、工具权限或输出 Schema。
- URL 必须限制协议、端口、域名、跳转、DNS 和最终 IP，阻止 SSRF 和内网探测。
- 下载内容必须限制大小、类型、压缩展开和处理时间。
- 不将 API Key、Cookie、MCP 凭据、完整外部响应或本地绝对路径写入业务对象。
- 服务商数据保留、地区和训练使用政策必须进入运营配置和租户准入。
- 版权、转载和引用规则必须按来源和使用方式评审，不因“公开网页”自动获得使用权。
- 删除或撤回来源时保留必要审计，但停止将其作为新任务可用输入。
- 来源失效或权利过期后，资产目录必须阻止新的任务引用；已经固定的历史输入保留审计并在下一安全门禁重新评估。

## 11. 失败和客户文案

| 失败 | Runtime / 运营状态 | 客户文案和动作 |
| --- | --- | --- |
| 搜索 API 限流 | `waiting(resource)` | 公开趋势采集正在排队，无需重复提交 |
| 指定网站拒绝访问 | `blocked(source_access)` | 无法读取这个网站，请更换来源或手动添加资料 |
| 本地 Agent 离线 | `waiting(local_agent)` | 需要连接本地创作工具后继续 |
| MCP 工具缺失 | `blocked(capability)` | 当前本地工具不支持此采集范围，请查看连接检查 |
| 页面内容过大 | `rejected(size_limit)` | 该来源超过处理范围，请选择具体页面或文件 |
| 结果全部重复 | `completed_with_no_new_results` | 没有发现新的灵感，可以调整关键词或时间范围 |
| 证据不足 | `needs_review` | 部分灵感缺少可核对来源，请确认是否仅作为参考 |
| 外部请求结果不明 | `unknown` | 正在核对采集结果，请勿重复提交 |

失败后必须保留已经成功采集并通过校验的来源，不能因为一个连接器失败而丢弃其他连接器结果。

## 12. 客户页面结果呈现

每条候选至少显示：

- 一句话洞察。
- 来源名称和时间。
- 证据片段或可展开引用。
- 与当前主题的关联原因。
- 时效和风险提示。
- “加入本次创作”“保留为项目参考”“忽略”“不准确”动作。

列表支持按来源类型、时间、相关性和是否已选择筛选。候选数量较大时必须分页或分批加载，不能一次把全文和所有证据发送到浏览器。

## 13. 第一阶段实施范围

### 必须完成

- `InspirationQuery` 和统一结果 Schema。
- 至少一种云端搜索 API 或确定性公开数据 Fixture。
- 一种受控来源读取方式。
- 一条本地 Codex MCP 路径，或明确标记为后续能力。
- Source、Evidence 和候选洞察的归一化、去重与引用。
- 客户选择 Gate 和进入下一阶段的固定输出。
- 客户保存或选择的 SourceRevision 进入任务/项目参考投影，并能从项目参考固定引用到另一任务；它不会成为客户创作资产目录项。
- 运营配置、租户开关、预算、日志和失败诊断。
- 提示词注入、SSRF、跨租户、超限和重复提交测试。

### 延后

- 任意网站的通用深度爬虫。
- 客户上传自定义爬虫或脚本。
- 无来源的全自动热点结论。
- 未经人工选择直接写入批准知识。
- 自动登录社交平台抓取私有内容。
- 因连接器失败而跨数据披露等级自动切换。

## 14. 测试与验收

### 契约测试

- 所有连接器对同一查询返回统一的 Source、Evidence 和候选洞察结构。
- 未知字段、缺少摘要、无效 URL、跨租户引用和不匹配 Schema 被拒绝。
- 固定输入和连接器 Fixture 生成稳定摘要，便于回归。

### 集成测试

- 搜索、获取、归一化、去重、证据提取和客户选择可以完整走通。
- 一个连接器失败时，其他成功结果仍可使用。
- 本地 Agent 断开、恢复和重复上报不会产生重复来源或重复决定。
- 任务固定模板、SOP、能力绑定和输入摘要。
- 同一来源重复保存不会生成重复项目参考，未保存候选不会污染项目参考或客户创作资产库。

### 安全测试

- 私网 IP、DNS 重绑定、重定向到私网、超大响应和压缩炸弹被阻止。
- 网页中的提示词不能调用未授权工具、扩大查询范围或改变输出 Schema。
- 租户之间不能读取查询、来源、证据、凭据引用或结果。
- 权利过期、来源撤回和陈旧项目参考状态不能创建新的输入引用。

### 客户验收

1. 客户不需要理解 API、爬虫或 MCP，也能启动一次灵感采集。
2. 客户可以区分来源事实、候选洞察和自己选择加入本次创作的内容。
3. 每条被选择的灵感都能回到明确来源和证据。
4. 客户始终知道下一动作，不会因底层连接器故障看到无意义错误。
5. 运营人员可以从客户步骤追溯到查询、执行者、连接器、费用、失败和输出摘要。
6. 客户保存或选择的灵感会保留为项目参考，并能在另一任务中以固定来源版本和摘要复用；它不会与生成结果混在客户资产库。
