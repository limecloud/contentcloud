# ContentCloud V5 实施台账

状态：`方案已形成，Automation 本地集成闭环已完成，待真实设备与业务验收`。

更新时间：2026-07-31。

本文件是 V5 唯一进度台账。V5 继承 V3/V4，不能以新路线图为由跳过现有 Schema、审批、publish/pull、租户、权利和 Browser 安全边界。执行分工以 [05-execution-boundaries.md](./05-execution-boundaries.md) 为准。

执行标签采用命令级硬边界：`contentcloud local ...` 只改本机 candidate；`publish --dry-run` 只做本机预检；带已确认 `plan_id` 的 publish 才在服务端创建 Revision；`submission approve` 只在服务端以授权用户身份产生 ApprovedSnapshot；Seedance/抖音操作始终在外部平台由用户执行。不得用“命令从 Codex 发起”混淆实际写入位置。

## 1. 里程碑

| 里程碑 | 目标 | 退出条件 | 状态 |
| --- | --- | --- | --- |
| M5-0 方案评审 | 冻结业务闭环、对象边界与实验口径 | 产品、内容、投放、合规和工程共同确认 D5 决策 | 待评审 |
| M5-1 策略纵向切片 | Codex 候选到服务端 approved strategy | 单人群/对比/探索、publish/review/pull、来源与证据门禁通过 | 实施中 |
| M5-2 分镜纵向切片 | Codex 分镜到服务端 storyboard ApprovedSnapshot | 本地独立图、publish review subset、服务端评论/lock 和 pull 通过 | 实施中 |
| M5-3 Seedance 交付 | Codex locked storyboard 到外部平台可复制包 | 本地验证通过，用户在真实平台按 README 成功生成 | 实施中 |
| M5-4 发布与归因 | 本地最终成片到服务端人工学习 | 用户外部发布、服务端 binding/结果导入和两类实验口径通过 | 待实施 |
| M5-5 生产试点 | 非生产试点后受控发布 | Golden Journey、权限、权利、成本和回归证据齐全 | 待实施 |

## 2. 工作包

### W5-00 方案与决策

| ID | 工作 | 产物 | 状态 |
| --- | --- | --- | --- |
| W5-00-01 | 梳理当前剧本、Delivery 和结果对象 | V5 兼容基线 | 已完成 |
| W5-00-02 | 调研抖音电商人群、投放方法与 Seedance 上游 Skill | 来源清单与适用边界 | 已完成 |
| W5-00-03 | 定义闭环、对象、流程、归因和验收 | 本目录总览、5 份专题方案及本台账 | 已完成 |
| W5-00-04 | 跨职能评审 D5-01 至 D5-12 | 决策记录、异议与结论 | 待开始 |
| W5-00-05 | 核实上游 LICENSE/作者授权及固定 commit | 已固定调研 commit；上游未声明 LICENSE，待归档作者书面授权 Evidence | 实施中 |
| W5-00-06 | 冻结 Codex、服务端与外部平台执行边界 | 职责矩阵、时序和边界测试 | 已完成 |

### W5-01 八大人群与策略

执行平面：服务端管理 taxonomy/审批；Codex 在本机生成和 lint candidate；用户在服务端完成选择确认与审核。

| ID | 工作 | 验收 | 状态 |
| --- | --- | --- | --- |
| W5-01-01 | 定义 AudienceTaxonomySnapshot Schema | 来源、版本、捕获、有效期和摘要可校验 | 已完成 |
| W5-01-02 | 定义 AudienceStrategyVersion Schema 与审批 | 模型假设不能无证据进入 review_ready | 已完成 |
| W5-01-03 | 实现单人群、2 至 3 类对比、八类探索 | 八类探索不默认生成完整媒体 | 已完成 |
| W5-01-04 | 复用 Brief `strategy_version_id` 和 audience 摘要 | 不升级 Brief 3.0 也能完成血缘 lint | 待开始 |
| W5-01-05 | 增加 strict A/B 与匹配探索交互 | 主变量和测试类型不冲突 | 实施中 |

### W5-02 Offer 与商品真实性

执行平面：服务端治理 OfferSnapshot 和 approved claims；Codex/本地后期工具在渲染时读取已 pull 快照并做发布前检查；用户负责最终权益确认。

| ID | 工作 | 验收 | 状态 |
| --- | --- | --- | --- |
| W5-02-01 | 评审 CommerceOfferSnapshot 最小字段 | 仅在动态权益进入交付时要求 | 已完成 |
| W5-02-02 | 增加 valid-at-render/publish 门禁 | 过期权益不能发布 | 实施中 |
| W5-02-03 | 将产品 truth strategy 映射到生产模式 | 真实资产、引导生成、合成和实拍 Plan B 可选择 | 待开始 |
| W5-02-04 | 建立文字/价格/LOGO 后期合成策略 | 生成底片默认不烘焙动态文字 | 实施中 |

### W5-03 分镜生产

执行平面：Codex 在本机生成图片、manifest 和 review sheet；服务端在显式 publish 后运行 ReviewCycle、记录批准与 lock digest；Codex pull 后继续。

| ID | 工作 | 验收 | 状态 |
| --- | --- | --- | --- |
| W5-03-01 | 定义 StoryboardPackage Schema、状态和摘要 | 本地 review_ready 与服务端 ApprovedSnapshot lock 责任分离 | 已完成 |
| W5-03-02 | 实现 ContentItem 到 shot task 的确定性映射 | 首尾帧、连续性、素材、权利和禁止项不丢失 | 已完成 |
| W5-03-03 | 接入图片生成的异步任务与 Artifact | 模型、版本、prompt、seed/参数和摘要可追溯 | 待开始 |
| W5-03-04 | 发现独立图与 review sheet 并计算摘要 | review sheet 不被误作默认模型输入 | 已完成 |
| W5-03-05 | 接入评论、修订、批准和 lock | 单 shot 修订不破坏其他已审镜头 | 实施中 |
| W5-03-06 | 真实商品一致性与 Plan B 评测 | 失真时 blocked，不依赖模型自评 | 待开始 |

### W5-04 Seedance 适配

执行平面：export Skill、编号、编译和 validator 均在 Codex 本机执行；用户在 Seedance 外部界面上传、生成和下载；服务端只保存显式 publish 的交付 manifest/Artifact。

| ID | 工作 | 验收 | 状态 |
| --- | --- | --- | --- |
| W5-04-01 | 创建经 ContentCloud 约束的 Seedance export Skill | 只读取从服务端 pull 的 storyboard ApprovedSnapshot，不修改领域事实 | 已完成 |
| W5-04-02 | 固定上游 commit 并提取 provider profile | 已固定调研 commit；真实平台 profile 与授权 Evidence 待补 | 实施中 |
| W5-04-03 | 定义 SeedancePromptPackage 和 upload manifest | 每个 `@引用` 可反查 Artifact 和 SHA-256 | 已完成 |
| W5-04-04 | 实现按镜头分段、编号和提示词编译 | 相同输入生成稳定编号；超限镜头阻断并要求上游叙事拆镜 | 已完成 |
| W5-04-05 | 实现 package validator | 覆盖引用、时长、上限、rights、动态 Offer 文本、绝对路径和摘要 | 已完成 |
| W5-04-06 | 生成 package.json、README 和纯文本 prompts | 用户无需聊天上下文即可操作 | 已完成 |
| W5-04-07 | 在真实 Seedance 入口完成手工 E2E | 至少正常、超限、profile 漂移三个场景 | 待开始 |

### W5-05 成片、发布与结果

执行平面：Codex/本地工具导入 take 并后期；用户在抖音发布；服务端创建正式 binding、导入 Observation 并承载人工 RatingDecision/Learning。

| ID | 工作 | 验收 | 状态 |
| --- | --- | --- | --- |
| W5-05-01 | 定义 generated plate 与 rendered creative Artifact 约定 | take、后期输入和最终二进制可追溯 | 待开始 |
| W5-05-02 | 定义 PublishedCreativeBinding | Delivery、成片、平台 ID、策略、Offer 和 arm 完整 | 实施中 |
| W5-05-03 | 扩展 PerformanceObservation 输入与服务端补全 | CSV 冲突不能覆盖 binding 事实 | 待开始 |
| W5-05-04 | 实现 strict A/B 和匹配探索校验 | 结果解释与实验设计一致 | 待开始 |
| W5-05-05 | 扩展 Lineage 与 ProjectProjection | Web/Codex 可从结果定位到策略和成片 | 待开始 |
| W5-05-06 | 接入 RatingDecision/Learning 人工闭环 | 系统不自动升级策略、知识或模板 | 待开始 |

### W5-06 QA、治理与发布

执行平面：本地测试验证 Codex 文件和包；服务端测试验证租户、审核、血缘和结果；真实宿主测试覆盖用户跨 Seedance/抖音的人工动作。

| ID | 工作 | 验收 | 状态 |
| --- | --- | --- | --- |
| W5-06-01 | Schema、状态机和 invariant 单元测试 | 所有硬门禁有正反例 | 实施中 |
| W5-06-02 | Workspace 文件、摘要和 stale 集成测试 | 修改任一锁定输入都能精确失效下游 | 实施中 |
| W5-06-03 | 租户、权利、敏感字段和外部披露安全测试 | 无跨租户、token、绝对路径和未授权原件泄漏 | 待开始 |
| W5-06-04 | 成本、重试和异步任务观测 | 八类探索零媒体调用，重试最小到 shot/segment | 待开始 |
| W5-06-05 | 执行完整 Golden Journey | 15 个步骤证据齐全 | 待开始 |
| W5-06-06 | V3/V4 回归与真实宿主验收 | publish/pull、Browser、审批和 Delivery 无回归 | 待开始 |
| W5-06-07 | 执行平面越权测试 | Codex 不能伪造批准，服务端不能扫描本机，外部平台动作必须人工确认 | 实施中 |

### W5-07 Automation Runtime

执行平面：服务端签发冻结租约；本机 Daemon 常驻轮询；租约内 Codex/Claude 全权限、无交互执行；结果由服务端 Schema 和业务规则复验。

| ID | 工作 | 验收 | 状态 |
| --- | --- | --- | --- |
| W5-07-01 | 移除 Automation Adapter 的只读、禁工具、禁网络限制 | Codex danger full access；Claude bypassPermissions；Provider 环境可继承 | 已完成 |
| W5-07-02 | 增加 Agent 进程组和取消回收 | 超时/取消后无子进程残留，Windows 构建通过 | 已完成 |
| W5-07-03 | 实现 daemon start/stop/status/restart | 幂等启动、PID/版本/路径/日志可查询 | 已完成 |
| W5-07-04 | Bootstrap 注册成功自动启动 | apply/resume 均启动一次；Plan 明确 would_enable_daemon | 已完成 |
| W5-07-05 | CLI 更新后重载已安装 daemon | npm 校验下载后由新二进制 restart --if-installed | 已完成 |
| W5-07-06 | Daemon 运行版本上报 | 每次 poll 刷新服务端 Device.daemon_version | 已完成 |
| W5-07-07 | 完成结果持久重报和实时进度 fallback | Daemon 崩溃/断网恢复后不丢已完成结果；long-poll/SSE 可重连 | 已完成 |
| W5-07-08 | 多 Workspace 并发、日志轮转和服务端更新策略 | 配额、日志上限、update_available 与兼容窗口可测 | 已完成 |
| W5-07-09 | Attempt 不可变进度事件 | 心跳、失败、取消、成功均可按 cursor 增量读取 | 已完成 |
| W5-07-10 | 全流程验收 | 本地集成已覆盖升级、断网、重启、多 Workspace、版本门禁和 SSE；真实设备继续验收 | 本地完成，真实设备待验收 |

## 3. 推荐实施顺序

```text
M5-0 决策冻结
   |
   v
M5-1 AudienceStrategyVersion + Brief 复用
   |
   +----> M5-2 StoryboardPackage + lock
                    |
                    v
              M5-3 SeedancePromptPackage
                    |
                    v
              M5-4 Creative Binding + Results
                    |
                    v
                 M5-5 试点
```

不要先写一个能输出 `@图片1` 的大 Prompt，再补领域对象。最小纵向切片也必须从服务端 approved ContentItem 开始，由 Codex pull 后生成，以一个本地 validated Seedance 包结束，并保留完整摘要。

## 4. 评审门

M5-0 至少确认以下问题：

1. 八大人群是预置 taxonomy 还是本地知识页；建议采用版本化 taxonomy snapshot。
2. 首版是否需要 CommerceOfferSnapshot；建议动态价格/优惠场景启用，纯品牌静态素材可选。
3. StoryboardPackage 是否进入云端正式事实；建议 manifest 和审核决定发布，受限原件继续遵循 SourceDisclosure。
4. Seedance 上游内容采用何种许可证/授权证据和固定 commit。
5. 真实使用的 Seedance 产品入口、账户区域、模型标签和能力上限。
6. PublishedCreativeBinding 的平台 ID 从人工录入、CSV 还是未来 API 获得。
7. V5 首个试点商品、目标、素材权利和预算负责人。
8. 是否接受首版边界：Codex 本地生产、服务端治理、用户手工操作 Seedance/抖音。

任何一个问题都不应通过在 Schema 中预留大量未验证字段解决。先冻结最小用例，再按真实能力扩展。

## 5. 依赖与风险

| 风险 | 控制 |
| --- | --- |
| 平台 taxonomy 或 Seedance 能力变化 | 版本化 snapshot/profile，设置有效期，旧交付不可变 |
| 上游 Skill 与业务边界冲突 | 包装为 ContentCloud adapter，先执行领域门禁再编译 |
| 八类探索导致成本失控 | 只生成文本策略卡，人工选择后才创建媒体任务 |
| 商品或优惠幻觉 | 真实资产优先，动态文字后期合成，Offer 发布时复验 |
| 多变量结果被错误归因 | 强制 test_type、primary variable、controlled variables 和 warning |
| 新对象与 V3 重复 | 复用 ApprovedSnapshot、Artifact、DeliveryPackage、Review 和 PerformanceObservation |
| 分镜批准后素材漂移 | storyboard ApprovedSnapshot 锁定 digest，任何本地变化阻断下游导出 |
| 用户无法真正复制使用 | 真实平台逐步操作验收，README 不依赖聊天历史 |
| Codex 与服务端职责再次混淆 | 所有工作包标执行平面，跨边界只走 publish/pull |

## 6. 当前结论

当前已完成 V5 契约与领域门禁、Codex 本地 audience/storyboard/Seedance 命令、服务端 V5 Submission 复验、增量 migration 和三个实际 Plugin Skill。尚未完成的是 Web 审核交互、正式 provider profile/作者授权 Evidence、媒体生成能力接入、PublishedCreativeBinding/结果归因和真实商品 E2E；因此 V5 仍处于纵向切片实施阶段，不能宣称生产闭环已经验收。

## 7. 变更记录

| 日期 | 变更 |
| --- | --- |
| 2026-07-28 | 建立抖音电商八大人群到策略、剧本、分镜、Seedance、成片和结果的完整闭环方案 |
| 2026-07-28 | 确认 ContentItem 厂商无关，新增派生 StoryboardPackage 与 SeedancePromptPackage 边界 |
| 2026-07-28 | 明确八类探索、人群定制探索与严格单变量 A/B 的不同归因口径 |
| 2026-07-28 | 增加 Creative Binding、Offer 时效、后期合成和真实平台验收门 |
| 2026-07-28 | 增加 Codex 本地、服务端治理和外部平台人工操作的执行边界、时序与越权测试 |
| 2026-07-29 | 将执行边界落实到 `local`、`publish/pull`、服务端批准和外部平台人工操作的命令级门禁 |
| 2026-07-29 | 增加 audience/storyboard/Seedance 本地纵向切片、服务端摘要复算、V5 submission migration 和可复制交付包 |
| 2026-07-29 | 固定上游调研 commit；确认仓库未声明 LICENSE，保留作者授权 Evidence 门禁且不复制上游 Skill 原文 |
| 2026-07-31 | 采用实战型 Automation 模式：租约内 Agent 全权限无审批，Bootstrap 自动启动 Daemon，补齐生命周期、进程回收和版本重启闭环 |
| 2026-07-31 | 完成 journal/outbox、重启恢复、多绑定并发、日志轮转、版本门禁、long-poll、SSE 和停止态 health 的自动化集成验证 |
