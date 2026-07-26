# V1 完成度与验收证据矩阵

> 更新日期：2026-07-26  
> 判定原则：本表以 PRD、用例、协议和 Definition of Done 为准。代码存在但缺少自动化或运行证据时标记 `unverified`；只有部分子能力时标记 `partial`；依赖客户、生产环境或外部评审的门禁不得由开发者自签。

## 1. 状态定义

| 状态 | 含义 |
| --- | --- |
| `complete` | 需求、权限、异常路径和自动化验收均有直接证据 |
| `partial` | 核心路径存在，但至少一个 V1 强制子项缺失 |
| `missing` | 没有可用实现 |
| `unverified` | 有实现，但缺少要求的真实平台、部署或独立验收证据 |
| `external` | 必须由客户、运维或独立安全角色在目标环境完成 |

## 2. 功能需求

| ID | V1 验收范围 | 当前证据 | 状态 | 完成缺口 | 验收 |
| --- | --- | --- | --- | --- | --- |
| FR-01 | 邮箱身份、会话、多个租户、成员与固定角色、即时撤销 | 注册/登录/退出、多租户切换、邀请创建/接受/撤销、固定角色变更、最后管理员保护和成员撤销后 session 即时失效已实现；Web/CLI/BFF 与领域测试对齐 | partial | 接入生产事务邮件适配器后实现邮箱验证、忘记密码和重置密码；补全全部角色/API 矩阵和真实浏览器 UAT | `identity_project_test.go` 覆盖邀请、角色、租户切换、最后管理员和 session 撤销；`server_test.go` 覆盖团队 BFF；CLI schema/dry-run 测试 |
| FR-02 | 项目模板、角色关系、更新/归档/恢复、连接引导和设备复用 | 模板创建/选择、row-version 更新、归档只读、管理员恢复、连接会话创建/查询/取消及设备授权已实现；Web/CLI/BFF 均有入口 | partial | 保存干净 macOS 安装/LaunchAgent 证据，补浏览器响应式验收和南京非工程角色 UAT | `identity_project_test.go` 覆盖模板、并发冲突、归档写阻断、恢复和连接取消；`server_test.go` 覆盖项目/连接 BFF；CLI schema/dry-run 测试 |
| FR-03 | 两阶段上传、不可变来源修订、解析/OCR、证据复核和影响范围 | Source/Revision/Evidence、Blob、Worker、MIME/hash/OCR 门禁已有实现 | partial | 预签名两阶段流程、为现有 Source 创建修订、上传人/有效期、重新解析、Evidence 人工接受/拒绝、影响 API | 支持类型及损坏/伪 MIME/超限测试；定位抽查；修订影响传播测试 |
| FR-04 | 本地知识提取任务、typed knowledge、冲突、权利、过期和传播 | `knowledge.extract` CLI/Run/Task Contract、本地候选导入、typed value、Conflict/DecisionRequest、证据越界拒绝、有效期和来源影响传播均有自动化测试 | partial | 品牌联系人受限决策链接、人工规则例外策略、权利变化到全部下游对象的通用传播 | `knowledge_runs_test.go`、`knowledge_evidence_test.go`、PostgreSQL extraction Run 持久化测试；批准项 lineage 完整 |
| FR-05 | Asset/Rights、Benchmark 双框架、验证对象、正反例和 analysis-only | Benchmark、Framework、ShotPattern 基础实现 | partial | Asset/RightsRecord、片段/来源/发布日期、验证对象、正反例、Task Contract 权利过滤 | 权利过期/渠道/地域负测；analysis-only 不进入 bundle |
| FR-06 | 卖点排序、可配置目标、可视化方案门禁 | SellingPoint、VisualizationPlan 创建/审核及 Brief 门禁已有实现 | partial | 卖点重排乐观锁、租户目标配置、事实/权利/可实现性统一门禁 | 排序并发测试；未批准/无权方案不能批准 Brief |
| FR-07 | 完整 Brief、内部批准、不可变修订及下游传播 | BriefVersion 创建/审核和内容 hash 已实现 | partial | 场景/冲突/证据/画幅字段、基于旧版修订、批准时 ApprovalDecision、下游 review_required | Brief 状态机；修改创建新行；旧剧本影响传播测试 |
| FR-08 | 设备/capability、不可变 Task Contract、数量/形态、租约/重试/插件产物 | 不可变 RunAttempt、短期 token、lease reclaim、最多 3 次 Attempt、取消优先、late-report 拒绝、知识任务、capability version/schema/digest 匹配及脱敏失败收敛已有单测和 PostgreSQL 证据；Extension Artifact Envelope 可由设备 CLI 校验并幂等登记 | partial | 用户选择产出数量/交付形态、Review Projection/安全 rendition 字节上报、Codex/Claude golden fixture | `run_attempts_test.go` 覆盖 lease/late-report/cancel/重试/contract mismatch；`artifact_validation_test.go` 覆盖 Envelope；PostgreSQL 覆盖 Attempt/Artifact 与 RLS |
| FR-09 | 结构化镜头、策略/引用/权利校验、blocked、单变量变体 | 逻辑 Script 与不可变 ScriptVersion、baseline/supersedes、revision/variant Run、递归 JSON Pointer diff、invariant 和单变量 hypothesis 门禁已实现；Web/CLI 均有入口 | partial | Script Package 的完整引用/素材权利校验、用户选择多份产出、blocked 可行动报告的浏览器 E2E | `script_diff_test.go`、`service_test.go` 修订/变体 E2E、`00009_script_version_lineage.sql` PostgreSQL 持久化测试 |
| FR-10 | 内审批注/结论/指派/修订、客户安全链接/撤销/最终门禁 | ReviewCycle、整版结论、责任人、不可变修订、跨版本未解决批注、批注批准门禁、ReviewGrant 历史/撤销和批准前 Brief/Knowledge/Framework/VisualizationPlan/Asset Rights 瞬时复核均已实现，Web/CLI/OpenAPI 已对齐 | partial | 客户安全投影中的依据摘要与版本 diff、客户姓名/7 天策略、IP/设备摘要、上游失效时主动撤回在途审批 | `service_test.go` 覆盖撤销立即失效、OTP、批注门禁、版本 hash；`00010_review_cycles.sql` 直连持久化与 RLS 测试 |
| FR-11 | 三格式、导出元数据、扩展产物四级展示和本机打开 | Markdown/XLSX/JSON、Artifact Envelope、服务端 tier/actions、active-content 拒绝、设备在线降级、60 秒 local-open 状态机、CLI 本地索引与 Web 动作均已实现；Gateway 永不接收 path/command/args/URL | partial | generation time/validation 元数据复核、ReviewProjection 内容上报、对象存储直传与 malware scan 后的安全 rendition | `artifacts_test.go` 覆盖 HTML/local-open/offline/脱敏原因；`artifact_validation_test.go` 覆盖固定 Schema；`00011_artifact_envelopes.sql` 持久化与双租户 RLS |
| FR-12 | 单条/CSV/XLSX 导入、完整指标、批次隔离和人工评级 | 单条与 CLI JSON/CSV/XLSX、不可变 ImportBatch、整批预检/事务、结构化行错、单一币种、spend/GMV/服务端 ROI、重复键和 RatingDecision 已实现；Web/CLI/OpenAPI 及结果到评级 lineage 对齐 | partial | 在真实 PostgreSQL 执行 `00012` 事务/RLS 测试；完成南京角色 UAT | `performance_results_test.go`、`result_import_test.go`、`performance_results_integration_test.go`；CLI `/tmp` help/schema/doctor smoke |
| FR-13 | 全状态审计、双向 lineage、影响对象/原因/状态/动作 | 统一只读 LineageGraph/ImpactAnalysis 覆盖 Source 到 RatingDecision，支持任一对象上下游/双向 BFS；Web 阶段工作台、CLI/BFF/OpenAPI、跨租户负测和结果写入审计断言已实现 | partial | 补齐逐写命令的审计覆盖矩阵、主动状态传播和真实浏览器/南京角色 UAT | `lineage_test.go` source-to-rating 双向 E2E；`performance_results_test.go` BFF lineage/impact/audit；`lineage show/impact` 与 `audit list` schema |

FR-14 Hosted Preview 是 V1.1/P3，不进入本矩阵完成门禁；V1 只保留枚举和扩展点，不部署或执行用户静态包。

## 3. 非功能需求

| ID | 要求 | 当前状态 | 证据或缺口 |
| --- | --- | --- | --- |
| NFR-01 | 全表 tenant 过滤，核心表 RLS | partial | `migrations/00003_runtime_rls.sql`、`00008`-`00012`；双租户测试代码已覆盖 RunAttempt、逻辑 Script、ReviewCycle、Artifact/OpenRequest、ImportBatch/Observation/RatingDecision，`00012` 仍需保存真实 PostgreSQL 重跑证据 |
| NFR-02 | 跨租户、对象和任务 100% 阻断 | partial | RLS/Service 负测已覆盖 Artifact 与 OpenRequest；仍需覆盖全部业务对象矩阵 |
| NFR-03 | 在线派发 p95 < 5s | unverified | 尚无可重复基准和结果工件 |
| NFR-04 | 普通读取 p95 < 500ms | unverified | 尚无 Web/CLI Gateway 基准 |
| NFR-05 | 所有写入幂等或乐观锁 | partial | Run 唯一键、Project row version 部分存在；需逐写命令覆盖 |
| NFR-06 | 结构化日志、trace、指标和告警 | partial | `slog`/request ID 有；OpenTelemetry、metrics、告警规则缺失 |
| NFR-07 | 云端 zero-exec/zero-model-credential | complete | 架构与代码无服务端模型调用；Adapter 只在 `contentcloud` 客户端 |
| NFR-08 | 桌面工作台、移动审批、WCAG/键盘 | unverified | Web 已有响应式页面；缺浏览器可访问性自动化和真实截图验收 |
| NFR-09 | 简中、UTC 存储、上海时区显示 | partial | 业务文案为中文、时间为 UTC；Web 统一时区 helper 需复核 |
| NFR-10 | 每日备份、RPO 24h/RTO 4h | external | 需要目标托管环境备份策略和恢复演练记录 |
| NFR-11 | 所有程序化通讯经 CLI | partial | FR-01/FR-02 的租户、成员、邀请、模板、项目更新/生命周期和连接会话均有 CLI dispatch/schema；Artifact、Daemon、结果和 Lineage 同样经 CLI，仍需逐 API 生成覆盖清单 |
| NFR-12 | 单二进制、安全凭据、稳定机器契约 | partial | Go binary、JSON envelope、npm wrapper 已有；真实 Keychain/launchd、Linux Secret Service、更新回滚待验收 |

## 4. 核心用例

| 用例 | 状态 | 仍需通过的主证据 |
| --- | --- | --- |
| UC-01 创建租户与项目 | partial | 成员邀请/RBAC、模板、项目角色、归档恢复和连接取消已形成 Web/CLI/BFF 闭环；仍缺邮箱验证/重置、干净电脑安装和非工程用户 UAT |
| UC-02 上传并解析 | partial | 两阶段预签名、支持格式矩阵、人工 Evidence 复核 |
| UC-03 提取和审核知识 | partial | 本地 `knowledge.extract`、冻结 Evidence、typed candidate、Conflict/DecisionRequest 已闭环；仍缺品牌联系人受限决策链接和对应 UAT |
| UC-04 登记并拆解案例 | partial | 权利对象、正反例/验证对象、片段来源 |
| UC-05 锁定卖点和 Brief | partial | 完整 Brief 字段、卖点重排、批准 Decision |
| UC-06 生成剧本和变体 | partial | RunAttempt/有限重试/lease reclaim/capability 匹配/variant lineage 与本地 Extension Artifact 登记已闭环；仍缺用户选择数量/形态、Review Projection 上传和两个 Adapter golden fixture |
| UC-07 内审与修订 | partial | 不可变 Script 修订、真实字段 diff、责任人、ReviewCycle、跨版本批注及解决状态已闭环；仍缺上游失效主动撤回和浏览器 E2E |
| UC-08 客户审批 | partial | OTP、版本/hash 绑定、撤销立即失效、批注与最终依赖门禁已闭环；仍缺客户安全依据摘要/版本 diff、IP/设备摘要和 7 天策略 |
| UC-09 导出 | partial | 三格式导出与扩展产物四级展示/local-open 已闭环；仍缺完整 generation/validation 元数据、对象存储直传安全 rendition 和临时预签名下载 |
| UC-10 结果复盘 | partial | 代码已闭环 import batch 隔离、重复/币种/ROI、人工 RatingDecision 和结果 lineage 页面；仍需真实 PostgreSQL 重跑与南京试点 UAT |

## 5. Definition of Done

| 门禁 | 状态 | 判定 |
| --- | --- | --- |
| PRD 验收、权限和错误状态全部实现 | partial | 见 FR/UC 缺口 |
| Domain、CLI、BFF contract 自动化 | partial | Artifact、Performance Import/Rating、Lineage/Impact 已有 Domain/App/CLI/BFF 测试代码和 OpenAPI；真实数据库与浏览器证据仍需补齐 |
| Tenant A/B 负向访问测试 | partial | PostgreSQL RLS 已验证，全部对象矩阵待补 |
| loading/empty/partial/blocked/error/offline | partial | 页面有部分状态，需逐页浏览器验收 |
| 日志、指标和审计 | partial | 指标/trace/告警及审计覆盖待补 |
| 文案、错误码、runbook | partial | README/路线图已有，运维 runbook 不完整 |
| 无开发者电脑隐式路径或凭据 | unverified | npm/二进制基础通过；真实干净系统和对象存储部署待验收 |
| 真实角色 UAT 且不改数据救场 | external | 必须由南京试点团队完成并签署 |

## 6. 固定验收命令

```bash
make check
go test -race ./...
pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web build
make build
```

数据库、浏览器、安装器和部署级验收必须另外保存可复核的环境信息与报告；本地 Memory Store 通过不能替代 PostgreSQL、RLS 或真实平台验证。
