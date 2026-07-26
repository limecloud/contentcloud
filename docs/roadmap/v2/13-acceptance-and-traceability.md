# V2 验收、追踪与完成度基线

## 1. 状态定义

| 状态 | 含义 |
| --- | --- |
| `planned` | 方案和验收已定义，尚未实现 |
| `partial` | 部分主路径已有实现，关键异常或证据缺失 |
| `implemented` | 代码完成，自动化验证通过，尚未真实 UAT |
| `accepted` | 自动化证据和有责任人的业务 UAT 均通过 |
| `deferred` | 明确不进入当前 V2 波次 |

本文建立 V2 开工基线，不把文档完成视为功能 implemented。

## 2. 功能追踪矩阵

| FR | V1 基线 | V2 交付 | 波次 | 首要验收证据 |
| --- | --- | --- | --- | --- |
| FR-01 项目治理 | Project/成员/设备/审计 | Client/Brand/Product、Gate、Risk、Impact | 1-2 | 多客户项目与角色 UAT |
| FR-02 可信知识 | 来源、证据、知识、冲突、权利 | 本地15维诊断、Knowledge Submission/批准快照 | 1 | 金陵古都香本地迁移/publish/决策 |
| FR-03 市场情报 | Benchmark/Framework/ShotPattern | ResearchTask、Insight、监控 | 2-3 | 公网+企业资料研究及采纳 |
| FR-04 营销策略 | SellingPoint/VisualizationPlan | StrategyVersion、Audience/Scenario 组合 | 1-2 | 策略到 Brief lineage |
| FR-05 内容策划 | Brief/Experiment 基础 | ContentPlan/Campaign/完整 Brief | 1-2 | 单变量 Brief 审批 |
| FR-06 创意生产 | 云端script run、Package 1.x | 本地CreativeBatch、Package 2.0、Script Submission | 1 | 无TaskRun三候选、publish、修订 |
| FR-07 审核协作 | Review/Comment/Grant/Approval | 字段定位、客户安全投影完善 | 1 | 内审、OTP、固定 hash |
| FR-08 交付制作 | JSON/MD/XLSX Artifact | DeliveryPackage/Handoff/外部状态 | 1-2 | 三格式一致与交接清单 |
| FR-09 结果学习 | Import/Observation/Rating/Memory | Learning 和跨域回流 | 2-3 | 人工采纳/拒绝与新实验 |
| FR-10 Automation | TaskRun/Attempt/lease/heartbeat | Plan/remote/event/schedule/Submission | 3 | 隔离工作区与故障矩阵 |
| FR-11 多客户上下文 | 单项目快照 | 四层继承、rebase、七层知识包 | 1 | 两客户隔离与模板复用 |
| FR-12 产物展示 | 原生核心和基础降级 | 安全投影、Run详情、Hosted Preview | 2-3 | 降级、隔离、无空白视图 |
| FR-13 本地工作区 | CLI/Daemon/embedded skills基础 | init、模板锁、Skills/MCP、publish/pull | 1 | 空/非空目录、披露、冲突 UAT |

## 3. 用例验收矩阵

| 用例 | 正常路径 | 必测异常 |
| --- | --- | --- |
| UC-01 项目与客户端初始化 | Web 建项目 -> init code -> workspace doctor | 过期/重复码、非空目录、模板/Agent冲突 |
| UC-02 素材诊断与知识包 | 本地来源 -> lint -> publish -> 决策 -> pull | 证据越界、披露不足、冲突、pull覆盖保护 |
| UC-03 市场研究 | 研究 -> 来源洞察 -> 人工采纳 | URL 漂移、过期洞察、竞品事实误用 |
| UC-04 策略与 Brief | 卖点 -> 可视化 -> Brief 批准 | 无画面证据、多变量、上游失效 |
| UC-05 剧本批次 | 本地3候选 -> lint -> publish -> review | blocked 草稿、Schema 错、无云端TaskRun证明 |
| UC-06 审核修订 | 云端批注 -> pull -> 本地revise -> republish | 基线变化、本地未提交修改、链接撤销/过期 |
| UC-07 交付学习 | 三格式 -> handoff -> 结果 -> Learning | hash 不一致、缺分母、混合窗口、样本不足 |
| UC-08 持续自动化 | 模板 -> schedule -> RunOutput -> 采纳 | 重复触发、租约过期、late report、自动暂停 |

## 4. 金陵古都香 Golden Journey

### 数据准备

- 在本地工作区迁移现有 source registry、232 个本体实例的可映射对象、冲突、知识缺口和权利候选。
- 保留 stable external ref、source locator、状态和 blocked 原因。
- 在本地保留首批十条 CreativeDraft 为一个 CreativeBatch，不提升发布资格、不默认上传 raw。

### 业务验收

1. Web 完成客户、品牌、产品、项目和服务模板，生成 init code。
2. 在空目录执行 init，验证模板、Skills、MCP、doctor 和默认不开启 Daemon。
3. 本地完成 15 维覆盖、七层知识包和 lint，明确缺口与冲突。
4. 选择来源披露等级并 publish；审核员按 ID 决定 Fact、Claim 和 Rights。
5. 本地 pull ApprovedSnapshot，创建抖音 Campaign、单变量 Experiment 和 Brief，再 publish 审批。
6. 本地选择至少两个 CreativeDirection，生成至少三条 ScriptPackage V2，证明没有云端 TaskRun。
7. 验证一条 blocked、一条本地 review_ready，并准确解释差异。
8. publish 剧本；云端对具体镜头和口播批注，本地 pull 后按基线修订并 republish。
9. 完成内部批准和客户 OTP 批准，本地 pull 最终 ApprovedSnapshot。
10. 导出 JSON、Markdown、XLSX，内容和 hash 一致。
11. 导入结果、生成 candidate Learning，由策略人员明确采纳或拒绝。
12. 修改一个来源或权利，验证受影响 Strategy/Brief/Script 进入 review_required。

### 责任签署

项目负责人确认流程和职责；策略人员确认策略/Brief；编导确认 AI 视频可执行性；审核员确认引用/权利；品牌客户确认最终批准体验。

## 5. 第二客户验收

- 使用不同行业、不同视觉规范和不同客户联系人。
- 复用平台方法论和同一租户服务模板。
- 对至少一个允许覆盖项定制，对禁止覆盖项验证拒绝。
- 证明客户、Workspace Credential、Submission、对象存储、审批链接和通知均不串租户/项目。
- 完成一条从来源到批准剧本的缩短闭环，证明没有硬编码金陵古都香概念。

## 6. Automation 场景

### Monitor

- 每周触发竞品监控；客户端离线采用 run_latest，不补跑全部历史。
- 输出进入 MarketInsight 待采纳队列，不直接更新 Strategy。
- 输出自动形成 SubmissionRevision，不直接写用户当前本地目录。

### Generate

- approved Brief 事件创建 script generation Run。
- schedule 配置被拒绝；重复事件只产生一个 Run。

### Review

- 周期汇总 Observation，样本不足时只写现象。
- Learning 需要人工采纳；拒绝原因可统计。

### Change

- 自然语言要求扩大竞品范围并提高频率。
- 客户端返回结构化 diff，页面突出数据范围和调度风险。
- PM 批准后创建新 PlanVersion；基线变化时原提案 stale。

## 7. 非功能验收

| NFR | 验收 |
| --- | --- |
| 多租户 | 主要聚合和对象存储全覆盖跨租户负测 |
| Zero Agent Server | 服务端依赖和部署配置无 LLM SDK/key/prompt runtime |
| 性能 | 约定数据规模下 BFF/CLI 普通读取和写入达到 p95 目标 |
| 可靠性 | publish/pull、scheduler、lease、outbox、通知和 late report 故障注入通过 |
| 安全 | 上传、路径、Prompt Injection、审批、Preview 隔离测试通过 |
| 可访问性 | 桌面工作台和移动审批核心任务满足 WCAG 2.1 AA |
| 兼容 | 当前/前一 Task Contract 与 ScriptPackage contract fixtures 通过 |
| 可观测性 | publish 路径串联 Submission/Approval；Automation 路径另串联 Run/Output/Submission |

## 8. 固定自动化命令

最终命令以仓库 Makefile/package scripts 为准，V2 发布门禁至少覆盖：

```bash
go test -race ./...
pnpm --dir web test
pnpm --dir web build
make check
make build
```

另需增加：OpenAPI breaking change、JSON Schema fixtures、migration integration、跨租户、Prototype 浏览器和可访问性检查。

## 9. 文档和原型验收

- `docs/roadmap/v2/README.md` 中的所有链接存在。
- Mermaid code block 可解析，参与者和术语与领域模型一致。
- PRD FR、用例、领域对象、CLI 和验收矩阵可双向追踪。
- Prototype 覆盖项目init、Workspace、Submission/审批、九域治理、Automation、Run详情和本地剧本生产主路径。
- Prototype 在 1440px、1024px、390px 视口无文本溢出、遮挡和不可达操作。

## 10. 发布结论规则

- 文档完成：只能标记规划完成。
- 代码和自动化通过：标记 implemented。
- 南京业务人员和第二客户场景完成：对应项才可 accepted。
- Hosted Preview、飞书深度连接和视频生成平台不得因为存在原型或接口占位而标记完成。
