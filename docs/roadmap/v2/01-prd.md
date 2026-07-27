# ContentCloud V2 产品需求文档

## 1. 文档信息

- 产品：ContentCloud
- 版本：V2
- 目标客户：为传统行业提供 AI 数字营销转型的内容营销服务商
- 首个 Golden Journey：金陵古都香 AI 视频营销剧本全流程
- 架构基线：V1 增量演进，本地优先创作，服务端 zero-agent-execution

## 2. 背景与问题

内容营销公司同时服务多个传统品牌时，业务事实、市场经验、创作过程和客户决策通常散落在网盘、飞书、聊天、表格和个人电脑中。通用 AI 可以生成文本，却无法保证：

1. 使用的是客户批准的事实、主张和素材。
2. 剧本能够被 AI 视频工具实现，而不是只有抽象文案。
3. 团队能看见每个客户的进度、阻断、责任和产能。
4. 一个客户验证过的方法能安全复用到其他客户，而不串数据。
5. 投放结果能回到具体卖点、框架、镜头和测试变量。
6. 本地成熟 Agent 能参与执行，但客户数据、模型凭据和责任边界不被云端接管。

## 3. 产品目标

### 3.1 业务目标

- 把内容营销服务从一次性人工项目升级为可复制、可持续运营的客户 Agent 服务。
- 让营销公司在一个系统内管理九域业务事实和端到端客户交付。
- 显著降低从资料到首批可审剧本的时间，同时提高证据、权利和审批完整率。
- 支持多客户、多品牌、多产品和不同传统行业的方法论复用与差异化配置。

### 3.2 用户目标

- 公司负责人查看客户组合、风险、产能和交付质量。
- 项目负责人按 Gate 推动客户项目，明确负责人和下一动作。
- 策略人员把资料、情报、受众、场景和卖点转为可批准 Brief。
- 编导使用本地 Agent 生成、比较和修订 AI 视频就绪剧本。
- 审核员基于引用、权利、品牌规范和版本差异作出决策。
- 品牌客户通过安全链接补充决策、批注和批准指定版本。

### 3.3 非目标

- 不在 V2 自动调用可灵、即梦或其他视频生成服务。
- 不自动发布内容、操作广告账户或作出预算决策。
- 不建设任意 Agent 市场、任意低代码工作流平台或通用 BI 产品。
- 不把未发布本地草稿当成云端已批准事实，也不把 Agent transcript 或生成 HTML 当作审批事实源。
- 不让 Agent 自动批准事实、主张、权利、Brief、剧本或投放因果结论。

## 4. 用户与责任

| 角色 | 主要责任 |
| --- | --- |
| `tenant_admin` | 租户、成员、服务模板、安全和商业配置 |
| `project_manager` | 客户项目、角色、Gate、自动化和交付结果 |
| `strategist` | 情报、受众、场景、卖点、内容计划和 Brief |
| `editor` | 创意方向、剧本生成、修订、交付准备 |
| `reviewer` | 事实、权利、Brief、剧本和影响项审核 |
| `viewer` | 只读查看授权项目 |
| 品牌客户审批人 | 通过版本绑定的受限链接批注、批准或退回 |

Agent、Daemon 和 Worker 是工具或系统参与者，不进入业务责任矩阵的 A/R。

## 5. 核心业务规则

1. 业务对象版本与 Agent Run 使用独立状态机。
2. 任何审批必须绑定不可变 SubmissionRevision 和其内容哈希，不能审批”最新版本”；云端不存在第二条平行审批轨道。
3. 正式内容只能使用 verified Fact、approved Claim、valid Rights 和允许等级的 Asset。
4. 候选知识不足时可以生成 CreativeDraft，但必须 `publishable=false` 并列明阻断原因。
5. 正式剧本必须先有卖点可视化方案，再生成镜头和话术。
6. 变体必须声明唯一主要变化项和保持不变项。
7. 上游来源、事实、权利、策略或 Brief 变化后，下游对象进入 `review_required`。
8. 客户端执行成功不等于业务产物可交付。
9. 所有程序化服务通信经 `contentcloud` CLI。
10. 定时任务只允许监控、复盘和治理，不允许无人值守生成正式内容或批准产物。
11. 普通本地交互不创建云端 TaskRun；只有 Automation 使用租约、Attempt 和 heartbeat。
12. 本地草稿与云端批准版本通过显式 publish/pull 同步，不持续镜像目录。

## 6. 功能需求

### FR-01 项目与治理

- 创建客户、品牌、产品和项目，支持一个客户多个品牌、一个品牌多个产品和活动项目。
- 指定服务模板、项目负责人、策略、编导、内部审核人和客户审批人。
- 显示 Gate、风险、阻断、待决策、设备在线状态和下一动作。
- 管理项目设备授权、Automation Plan、通知订阅、影响项和 append-only 审计。
- 项目归档后只读；重新开启需记录原因和责任人。

### FR-02 可信知识

- 客户端从文档、表格、图片、视频、网页快照及企业数据源建立本地不可变来源登记。
- Evidence 支持页码、段落、单元格、图片区域、视频时间码和网页快照定位。
- 分离 Fact、Claim、Asset、Rights、Conflict、DecisionRequest 和 Synthesis。
- 客户端执行解析、OCR、候选提取、冲突发现和 lint；用户确认 publish 后，服务端保存 Submission、证据披露等级、状态和人工门禁。
- 支持过期、来源变化、权利变化及下游影响传播。

### FR-03 市场与内容情报

- 创建一次性研究或持续监控任务，明确市场范围、平台、竞品、关键词和时间窗口。
- 区分品牌自身历史资产、同品类竞品、跨品类案例、平台趋势和文化表达边界。
- 案例可拆为画面框架、文案框架、钩子、镜头模式、证据类型、CTA 和可借鉴边界。
- 洞察必须保留来源、采集时间、可信等级、适用范围和人工采纳状态。
- 市场结构不能成为本品牌事实、权利或功效依据。

### FR-04 产品营销策略

- 管理 Audience、Scenario、PainPoint、DemandMoment、SellingPoint 和 VisualizationPlan。
- 卖点排序必须记录目标人群、适用场景、支持知识和风险。
- VisualizationPlan 包含主体、场景、道具、实施方式、真实性策略、Plan B 和验收条件。
- 以上选择组合为不可变 StrategyVersion，经 publish strategy 检查点审批后才能被 Brief 引用。
- 策略人员可比较候选方案；审核员批准后才能进入正式 Brief。

### FR-05 内容策划

- 管理 Topic、ContentPlan、Campaign、Experiment 和不可变 BriefVersion。
- ContentPlan 明确渠道、目标、频次、内容支柱、负责人和测量窗口。
- Brief 明确受众、需求时刻、核心卖点、画面证据、叙事约束、CTA、测试变量和禁止项。
- Brief 支持草稿、内审、批准、退回和上游影响复核。

### FR-06 创意生产

- 从本地已拉取的批准 Brief/ApprovedSnapshot 创建 CreativeDirection 和 CreativeBatch。
- 批次声明数量、意图、变化维度、输出协议和目标 capability。
- 支持生成单条剧本、批量候选、基于批注修订和基于基线版本生成变体。
- ScriptPackage V2 覆盖完整 AI 视频就绪信息，详见 `05-script-production-system.md`。
- 本地对候选执行引用、权利、品牌、结构、时长、连续性、单变量和可生成性校验；通过后显式 publish Script Submission。

### FR-07 审核与客户协作

- 审核主体统一为 SubmissionRevision：knowledge、strategy、brief、script、delivery 各自的 Submission 分别开 ReviewCycle。
- 批注支持对象级、镜头级、字段级定位，区分内部与客户可见性。
- 未解决阻断批注时不得进入下一审批阶段。
- 客户审批链接绑定 tenant、project、具体 SubmissionRevision、email 和有效期；出现新 revision 后旧链接自动失效。
- 客户作出最终决策前使用一次性邮件验证码；撤销立即生效。

### FR-08 交付与外部制作

- 将客户批准的 script ApprovedSnapshot 组成 DeliveryPackage。
- 支持 canonical JSON、Markdown 和 XLSX 导出，并记录内容哈希。
- ProductionHandoff 包含素材清单、缺失素材、镜头制作方式、生成工具建议、权利边界和验收清单。
- V2 记录外部制作状态和成片关联，不在系统内自动生成视频。

### FR-09 投放结果与学习

- 支持 CSV/XLSX/人工方式导入平台或门店结果，保存不可变 ImportBatch 和 Observation。
- 指标必须绑定内容版本（script ApprovedSnapshot）、渠道、统计窗口、单位、分母、自然/付费来源和定义版本。
- 系统可计算派生指标并生成候选评级建议；因果归因和策略采纳必须人工确认。
- Learning 可回到 Framework、ShotPattern、SellingPoint、VisualizationPlan 和 Experiment。

### FR-10 Automation 与 Run

- 从受治理业务模板创建 Automation Plan，配置业务范围、触发、负责人、设备和通知。
- 支持 remote、event 和受限 schedule 触发，支持 pause、resume、run once 和 archive。
- 每次执行生成 TaskRun 和不可变 RunAttempt，记录租约、心跳、进度、usage、结果摘要和错误。
- RunOutput 必须转换为 SubmissionRevision，不得直接改变需要人工批准的对象。
- 自然语言修改生成 PlanChangeRequest；客户端产生 diff，人工确认后形成新 PlanVersion。

### FR-11 多客户上下文

- 支持平台方法论、租户模板、客户/品牌知识包、项目快照四层继承。
- 七层知识包固定为 identity、product、market、expression、operations、content_engine、compliance。
- 支持 15 维素材诊断、覆盖率、冲突、缺口、资源、限制和研发节点。
- 上层升级不能静默修改已运行项目快照；项目负责人显式 rebase 后进入影响复核。

### FR-12 产物展示

- 核心业务对象使用服务端原生结构化视图。
- 扩展产物按 `cloud_native -> safe_projection -> safe_rendition -> local_open -> metadata_only` 降级。
- Preview 不可用时必须显示可理解占位和本机打开动作，不得出现空白 iframe。
- Hosted Preview 属于第三波最后能力，不替代 SubmissionRevision 的审批和哈希。

### FR-13 本地工作区、Skills、MCP 与发布

- Web 创建项目后生成公开 ConnectSession；固定版本 bootstrap CLI 通过浏览器 PKCE 授权，初始化签名环境并绑定项目。
- 初始化项目级方法论、ontology、knowledge、raw、work、outputs、workflows、Skills 和 MCP 配置。
- 默认不覆盖已有文件、不启动后台自动化、不上传原始资料。
- 提供 workspace doctor/status/upgrade/diff，模板升级使用 lock 和三方差异，不静默覆盖客户修改。
- publish 支持 knowledge、brief、script 等检查点，先本地 preflight，再创建不可变 SubmissionRevision。
- 原始资料按 metadata-only、evidence-pack、full-source 分级披露；默认 evidence-pack。
- pull 下载 ReviewFeedbackBundle、DecisionDelta 和 ApprovedSnapshot，先进入本地 inbox，不覆盖未提交修改。
- 云端不直接编辑知识、Brief 和剧本正文。

## 7. 非功能需求

| 编号 | 要求 |
| --- | --- |
| NFR-01 | 所有租户数据读写强制 tenant scope，并有跨租户负向自动化测试 |
| NFR-02 | 服务端没有 LLM SDK、模型密钥、prompt 编排和 Agent runtime 依赖 |
| NFR-03 | 普通 BFF/CLI Gateway 读取 p95 小于 500ms；大文件和研究任务异步处理 |
| NFR-04 | 业务写入具备幂等键或乐观锁；任务报告和审批不能重复生效 |
| NFR-05 | TaskRun、审批、导出、影响传播具备 trace ID、结构化日志和告警 |
| NFR-06 | 客户审批支持移动端；内部工作台满足桌面高密度使用和 WCAG 2.1 AA |
| NFR-07 | 核心 Schema 至少支持当前版本和前一版本，CLI 与服务端协商 capability 版本 |
| NFR-08 | 原始资料、业务对象、运行摘要和临时文件使用独立保留策略 |
| NFR-09 | 定时调度重复投递不得产生重复业务产物；过期租约可恢复或人工终止 |
| NFR-10 | Prototype 和生产 UI 的术语、导航、状态及核心流程保持一致 |

## 8. 成功指标

### 8.1 采用与交付

- 至少两个不同行业客户完成项目建档和隔离验证。
- 南京试点团队至少 5 名内部用户完成真实角色分工。
- 至少 1 名客户审批人通过受限链接完成独立审批。
- 九域均有活跃业务对象和负责人，不以 Run 数量代替业务采用。

### 8.2 效率

- 完整资料下，从项目就绪到首批可内审剧本不超过 1 个工作日。
- 按批注修订无需重新人工拼接上下文，修订输入 100% 绑定基线版本和批注。
- 交付包三种格式由同一 canonical ScriptPackage 生成。

### 8.3 治理质量

- 100% 客户审批可定位到唯一对象版本、哈希和审计事件。
- 100% 可交付剧本确定性表述具有 eligible 知识引用。
- 100% 使用素材具有可检查权利状态或明确 blocked 标记。
- 上游失效后，下游影响对象无静默遗漏。

### 8.4 内容学习

- 每个正式实验只声明一个主要变化变量。
- 导入结果能定位到 script ApprovedSnapshot 及其中的 ScriptVersion、CreativeDirection、卖点、框架和镜头模式。
- 自动评级建议不能直接改写正式策略；人工采纳率和拒绝原因可统计。

## 9. V2 总体验收

1. 从 Web 创建客户、品牌、产品和项目，选择租户服务模板。
2. 创建公开 ConnectSession，通过浏览器核对短码并授权本机 Creative Runtime。
3. 在本地导入金陵古都香来源，完成 15 维诊断、七层知识包和 lint，显式 publish 后完成人工知识决策。
4. 创建市场研究，采纳案例结构，但不把竞品事实混入品牌知识。
5. 完成受众、场景、卖点、可视化方案和 Brief 审批。
6. 在本地创建 CreativeBatch，由本地 Agent 返回至少 3 个 ScriptPackage V2 候选，全程不创建云端 TaskRun。
7. 展示 blocked 和 review_ready 两种结果及其准确原因。
8. publish 候选，完成云端镜头批注；本地 pull 批注并基线修订，再完成内部和客户批准。
9. 导出可解析且内容一致的 JSON、Markdown 和 XLSX。
10. 导入最小结果，创建候选 Learning，由策略人员采纳或拒绝。
11. 启用一个竞品监控 Automation，验证调度、离线恢复、Run 详情和通知。
12. 变更一个来源或权利状态，验证下游影响传播和重新审核。
