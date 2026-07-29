# 结果归因与验收

## 1. 为什么现有结果绑定不够

当前 PerformanceObservation 绑定 `approved_snapshot_id`，可以说明结果来自哪版批准内容，但不能回答：

- 具体使用了哪个 DeliveryPackage 和哪条最终视频文件。
- Seedance 生成后是否更换了镜头、字幕、Offer 或 CTA。
- 实际发布的抖音 creative/post ID 是什么。
- 使用了哪个 AudienceStrategyVersion 和实验臂。
- 这是严格单变量测试，还是同时变化人群与表达的探索。

因此 V5 必须先创建 PublishedCreativeBinding，再把平台聚合结果挂到 binding。ApprovedSnapshot 继续保留，形成从业务内容到实际二进制成片的完整血缘。

执行边界：发布前文件检查在 Codex/本地工具执行，抖音发布由用户在外部平台执行，PublishedCreativeBinding、PerformanceObservation、RatingDecision 和 Learning 的正式写入与校验在 ContentCloud 服务端执行。

## 2. 发布前检查

执行方：`Codex 本机确定性检查 + 人工 QA`。检查结果随最终 DeliveryPackage publish；服务端复验 Schema、摘要、权利/Offer 状态和正式引用，不远程打开本机后期工程。

rendered creative 进入 DeliveryPackage 前执行确定性检查：

| 维度 | 检查 |
| --- | --- |
| 文件 | SHA-256、媒体类型、编码、分辨率、9:16 比例、时长、文件大小 |
| 画面 | 商品一致性、异常肢体/物体、闪烁、镜头衔接、平台安全区 |
| 声音 | 旁白内容、音画同步、峰值、静音、音乐和音色权利 |
| 文字 | 字幕准确、错别字、免责声明、LOGO、价格和优惠 |
| 商品 | SKU、规格、使用方式、效果、approved claim 和禁止声明 |
| Offer | `verified`、在计划发布时间有效、适用条件完整 |
| 实验 | experiment/arm、人群策略、主变量和受控变量明确 |
| 权利 | 商品、人物、场景、音乐、字体、旁白和生成物均可用于目标渠道 |

任一高风险项失败都 blocked，不能用“生成效果不错”覆盖事实或权利问题。

## 3. 发布绑定

执行方：`用户在抖音发布 + ContentCloud 服务端建立 binding`。Codex 可以收集和提交用户提供的平台 ID，但不能直接把本地记录当作正式 binding。

创建 PublishedCreativeBinding 时至少要求：

- `delivery_package_id`。
- `rendered_creative_artifact_id` 及摘要。
- `platform=douyin`、`account_alias`。
- 至少一个 `platform_creative_id` 或 `platform_post_id`。
- `audience_strategy_version_id`。
- `experiment_id` 和 `experiment_arm_id`。
- 可选但在权益素材中必需的 `offer_snapshot_id`。
- `published_at` 和创建者。

平台 ID 暂时无法取得时，记录保持 `pending_platform_id`，不能导入为正式可归因结果。后补 ID 通过追加绑定确认事件完成，不改写原始 DeliveryPackage。

## 4. PerformanceObservation 演进

执行方：`ContentCloud 服务端`。用户可通过 Web、CLI 或 CSV 发起导入，所有租户、binding、去重、币种、窗口和 ROI 校验由服务端完成。

### 4.1 推荐新增引用

下一版输入契约增加：

| 字段 | 必需性 | 说明 |
| --- | --- | --- |
| `published_creative_binding_id` | 新数据必需 | 归因主键 |
| `delivery_package_id` | 可由 binding 投影 | 便于查询和导出 |
| `rendered_creative_artifact_id` | 可由 binding 投影 | 精确到二进制成片 |
| `platform_creative_id` / `platform_post_id` | 至少一个 | 与导入源对账 |
| `audience_strategy_version_id` | 必需 | 策略血缘 |
| `experiment_arm_id` | 必需 | 防止跨臂混合 |
| `test_type` | 必需 | `strict_ab`、`exploration_batch`、`audience_expression_fit_test` |

服务端应从 binding 补全冗余字段并校验一致，不能相信 CSV 同时提供的冲突 ID。

### 4.2 指标漏斗

沿用现有指标并按决策阶段组织：

| 阶段 | 指标 | 用途边界 |
| --- | --- | --- |
| 分发 | `impressions` | 判断是否有足够样本，不单独判断创意好坏 |
| 停留 | `views`、`three_second_retention_rate` | 初步检查钩子与开场匹配 |
| 消费 | `completion_rate` | 检查节奏、信息密度和内容承诺 |
| 兴趣 | `clicks`、互动 | 检查商品兴趣；互动不等同成交意愿 |
| 成交 | `conversions`、GMV | 检查业务结果，需要考虑 Offer、流量和归因窗口 |
| 效率 | spend、服务端计算 ROI | 不能接受客户端提交的 ROI 覆盖服务端公式 |

每条 Observation 继续包含统计窗口、样本状态、币种、去重键和 issue category。不同窗口、币种、账户或实验臂不能直接合并。

## 5. 归因规则

### 5.1 严格 A/B

只有满足以下条件才标为 `strict_ab`：

- 一个明确主变量。
- 受控变量列表完整。
- 相同或可比的 Offer、投放时段、预算与落地页。
- 相同指标定义和观察窗口。
- 每个 arm 都有唯一 PublishedCreativeBinding。

系统执行契约检查，但不自动宣称统计显著。样本不足、分发不均或外部变量变化时必须输出 warning。

### 5.2 人群与表达匹配探索

不同人群使用不同剧本、分镜或 CTA 时标为 `audience_expression_fit_test`。允许比较组合表现和筛选下一轮候选，但 Learning 应写成：

> 在当前 Offer、投放和观察窗口下，`精致妈妈 + 省时实拍` 组合比候选组合获得更高的三秒留存和点击率，需用受控实验验证钩子或人群的独立贡献。

禁止写成：

> 精致妈妈一定更喜欢该商品，Seedance 提示词已证明有效。

### 5.3 跨轮次比较

跨时间比较前检查分类版本、账户、季节、价格、库存、竞价、落地页和指标定义。任一关键上下文变化时，只能作为趋势证据，不能当作同一实验继续累积。

## 6. 学习闭环

执行方：`服务端工作流 + 人工决策者`。Codex 可以基于 pull 的结果起草候选解释，但 adopt/reject 和正式 Learning 必须通过服务端治理命令。

```text
PerformanceObservation
       |
       v
人工选择可比较数据 -> RatingDecision -> Learning candidate
                                            |
                                      人工 adopt / reject
                                            |
                 +--------------------------+------------------------+
                 v                                                   v
    新 AudienceStrategyVersion / Brief                        保留历史，不改模板
```

Learning 至少包含：

- target type/id，可以是 audience strategy、hook、shot pattern 或 CTA。
- observation IDs 和 PublishedCreativeBinding IDs。
- 陈述、置信度、样本/混杂因素 warning。
- 建议动作与人工采纳决定。

系统不能根据一次高 ROI 自动升级人群模板、改写品牌知识或批准下一版内容。学习进入新版本时必须保留来源和人工决定。

## 7. 验收矩阵

| ID | 范围 | 验收场景 | 预期结果 |
| --- | --- | --- | --- |
| A5-01 | 人群目录 | taxonomy 过期后创建新策略 | 只能 candidate，明确要求更新来源 |
| A5-02 | 人群交互 | 选择八类探索 | 生成八张策略卡，不生成八套完整媒体 |
| A5-03 | 证据 | 卡片只有模型推断 | 显示 low/待验证，不能 review_ready |
| A5-04 | 实验 | 同时改变人群和创意却选择 strict A/B | lint 拒绝或要求改为匹配探索 |
| A5-05 | 剧本 | ContentItem 导出前检查 | 不出现 `@图片N` 或厂商私有字段 |
| A5-06 | 分镜 | 商品包装与真实素材不一致 | 分镜 blocked，提供 composite/实拍 Plan B |
| A5-07 | 分镜 | review sheet 通过但独立图发生变化 | locked digest 变化，旧批准不能直接导出 |
| A5-08 | Seedance | prompt 引用不存在的 `@图片4` | 验证失败并定位 segment/引用 |
| A5-09 | Seedance | 素材数量超过当前 profile | 按叙事拆包或 blocked，不静默删除素材 |
| A5-10 | Seedance | provider profile 已过期 | 旧包可读，新包要求重新验证能力 |
| A5-11 | 交付 | 用户打开 README | 能独立完成上传、设置和逐段复制，不依赖聊天历史 |
| A5-12 | 权益 | Offer 在计划发布前过期 | 阻止发布；更新 Offer 后只需重合成动态层 |
| A5-13 | 成片 | 同一剧本换字幕再发布 | 产生新 rendered artifact 和 binding |
| A5-14 | 归因 | CSV 平台 ID 与 binding 冲突 | 整行或整批按既有原子规则拒绝，不能覆盖服务端事实 |
| A5-15 | 归因 | 没有 binding 的平台数据 | 进入隔离/待绑定状态，不形成正式 Learning |
| A5-16 | 学习 | 单次结果看似高 ROI | 允许生成候选结论，不自动修改策略或评级 |
| A5-17 | 安全 | 导出包含绝对路径或 token | 验证失败，敏感字段不进入包 |
| A5-18 | 权利 | 参考音乐授权到期 | 新导出和发布 blocked，历史交付保持可审计 |

## 8. Golden Journey

端到端验收使用一个真实但非生产的抖音电商商品：

1. `[服务端/人工]` 导入并批准商品知识、RightsRecord、有效 OfferSnapshot 和八大人群 taxonomy。
2. `[Codex]` pull 已批准输入，进入八类探索。
3. `[Codex]` 生成八张轻量候选卡，用户对比后选择 2 类进一步细化。
4. `[Codex -> 服务端/人工]` publish 1 个 AudienceStrategyVersion；服务端审核证据、置信度和实验类型后批准。
5. `[Codex]` pull 策略，生成 Brief、ContentBatch 和 ContentItem，完成本地 lint 后 publish。
6. `[服务端/人工 -> Codex]` 批准 ContentItem；Codex pull ApprovedSnapshot 后生成分镜独立图和 review sheet。
7. `[Codex -> 服务端/人工]` publish review subset；审核人员驳回商品失真的镜头，Codex pull 评论并只修订相关 shot。
8. `[服务端/人工 -> Codex]` 新版 StoryboardPackage Revision 获批；服务端用 storyboard ApprovedSnapshot 锁定 manifest digest，Codex pull 该快照。
9. `[Codex]` Seedance 适配 Skill 根据当前 provider profile 生成素材清单、设置和逐段可复制中文提示词。
10. `[Codex]` 删除一个引用图片后运行本地验证，导出被拒；恢复正确 Artifact 后通过。
11. `[用户/Seedance]` 用户按 README 手工上传并生成多个 takes，选择合格底片导回 Workspace。
12. `[Codex/本地工具 + 人工]` 后期合成批准旁白、字幕、LOGO、CTA 和有效 Offer，发布前检查通过。
13. `[Codex -> 服务端]` publish 最终 Artifact 和 DeliveryPackage；`[用户/抖音]` 发布后提交 creative/post 与 experiment arm，服务端创建 PublishedCreativeBinding。
14. `[用户 -> 服务端]` 导入 24h/72h 聚合结果，服务端验证 binding、币种、窗口、去重和 ROI。
15. `[服务端/人工]` 用户创建 RatingDecision 和 Learning candidate，明确混杂因素，再决定是否派生下一版策略。

## 9. 非功能验收

- 可复现：相同 locked digest、provider profile 和 adapter digest 生成相同 manifest 与引用编号。
- 可追溯：任一平台结果可定位最终视频摘要、DeliveryPackage、提示词包、分镜、剧本、策略和证据。
- 可恢复：单镜头失败、Offer 过期或 profile 漂移不要求重建全部上游对象。
- 成本可控：八类探索默认不调用媒体生成；重试以 shot/segment 为最小单位。
- 安全：租户隔离、权利检查、敏感字段检查和外部披露确认有审计记录。
- 性能：一个常规 15 至 30 秒 ContentItem 的 manifest 和 lint 在本地交互时间内完成；媒体生成时长不计入同步请求。
- 可观测：每次生成、验证、批准、锁定、导出、导入和发布绑定都有 capability digest、操作者和时间。

## 10. 发布门

只有以下证据全部存在，V5 才能从“方案”进入“可发布能力”：

1. 至少一个商品完成 Golden Journey。
2. 在真实 Seedance 产品入口验证 provider profile 和可复制包。
3. 完成人工审核、商品真实性、权利和 Offer 过期测试。
4. 完成 strict A/B 与 audience-expression-fit 两类归因契约测试。
5. 上游 Skill 的 commit、许可证或作者授权 Evidence 可审计。
6. V3/V4 现有 publish/pull、ApprovedSnapshot、DeliveryPackage 和 Browser 治理边界无回归。
7. Codex、服务端和外部平台的执行边界测试全部通过，任何一方都不能伪造另一方的决定或副作用。
