# ContentCloud V5 抖音电商视频生产闭环方案

状态：`方案已形成，Automation 本地集成闭环已实现，进入真实设备与业务验收`。

更新时间：2026-07-31。

V5 解决一个当前尚未闭环的问题：ContentCloud 已能生成厂商无关的营销剧本，但还不能稳定地把“抖音电商人群策略、剧本、可审核分镜图、Seedance 可复制提示词、实际发布成片和投放结果”串成可追溯流程。

V5 不创建第四个数据平面，也不把 ContentCloud 绑定成 Seedance 专用产品。它继承 V3 的 Workspace、ContentBatch、ContentItem、SubmissionRevision、ApprovedSnapshot、Artifact、DeliveryPackage 和 PerformanceObservation，并在生产与交付边界增加少量派生对象和厂商适配能力。

执行归属必须清晰：Codex 在本机 Workspace 生成候选、组织本地媒体并执行可恢复任务；ContentCloud 服务端通过显式 `publish` 接收 Revision、执行审核/权限/审计并提供治理视图，再通过 `pull` 分发批准事实；Seedance 与抖音是用户操作的外部平台，不由 ContentCloud 服务端代登录或代上传。完整职责见 [05-execution-boundaries.md](./05-execution-boundaries.md)。

## 1. 最终闭环

```text
服务端正式事实：商品知识 / 价格权益 / 品牌规则 / 合规证据 / 人群目录
                                      |
                                      | pull
                                      v
Codex 本机：八大人群候选 -> 策略候选 -> Brief / ContentBatch -> ContentItem
                                      |
                                      | publish，服务端审核并生成 ApprovedSnapshot
                                      v
Codex 本机：分镜候选与本地媒体 -> publish review subset -> 服务端批准/锁定
                                      |
                                      | pull storyboard ApprovedSnapshot
                                      v
Codex 本机：SeedancePromptPackage
             素材顺序 + @引用映射 + 可复制提示词
                                      |
                                      | 用户手工上传/复制
                                      v
外部平台：Seedance 生成 -> 本机导回底片 -> 本机后期合成
                                      |
                                      | publish final delivery；用户在抖音发布
                                      v
服务端：PublishedCreativeBinding -> 导入 PerformanceObservation
                                      |
                                      v
服务端人工决定：RatingDecision / Learning
```

用户最终拿到的不是一段泛化“视频提示词”，而是一个可以照着执行的 Seedance 包：先按清单上传已锁定的分镜和参考素材，再设置模式、比例和时长，然后逐段复制提示词。生成后的字幕、价格、优惠、LOGO 和 CTA 通过确定性后期合成，发布记录再绑定回具体版本与实验臂。

## 2. 现状判断

当前设计是一个正确的剧本基础，但不是抖音电商视频生产的完整最佳实践。

| 能力 | 当前基线 | V5 判断 |
| --- | --- | --- |
| 厂商无关剧本 | `ContentItem 3.0` 已有首帧、运动、尾帧、素材、权利、连续性和商品真实性 | 保留，不写入 Seedance 私有语法 |
| 人群策略 | Brief 有 `strategy_version_id` 和 `audience` | 缺可版本化的八大人群来源、证据、有效期和交互 |
| 分镜生产 | 路线图出现 `storyboard_generate`，尚无正式领域对象和审核闭环 | 增加 StoryboardPackage 与锁定门禁 |
| Seedance 交付 | 尚无素材编号、设置和逐段可复制提示词 | 增加厂商适配的 SeedancePromptPackage |
| 成片追溯 | DeliveryPackage 能绑定 ApprovedSnapshot | 缺生成成片、平台 post/creative、人群策略和实验臂的绑定 |
| 效果学习 | PerformanceObservation 已支持曝光、观看、留存、点击、转化、消耗和 GMV | 需补发布创意和策略血缘，继续坚持人工采纳 |

V5 所称“最佳实践”不是承诺单一模板必然带来 GMV，而是满足五个条件：创意假设有证据、商品事实不被生成模型篡改、分镜与提示词可复现、投放变体可解释、结果能回到具体策略和成片。

## 3. 核心决策

| ID | 决策 | 原因 |
| --- | --- | --- |
| D5-01 | `ContentItem` 继续作为厂商无关的规范剧本 | 防止业务剧本被 `@图片N`、模型版本和平台参数污染 |
| D5-02 | 八大人群作为有来源和有效期的策略预置，不作为永久人口事实 | 平台口径和消费行为会变化，避免把标签刻板化 |
| D5-03 | 首版通过 Brief 的 `strategy_version_id` 引用人群策略，`audience` 保存可读摘要 | 复用 V3 契约，暂不引入不必要的 Brief 3.1 |
| D5-04 | 八类探索只生成策略候选卡，不默认生成八套分镜和媒体 | 控制成本，并要求人工选择有证据的方向 |
| D5-05 | 本地分镜只经过 `candidate -> review_ready`；服务端批准生成的 storyboard ApprovedSnapshot 代表 locked | 本地文件不能伪造服务端状态，Seedance 只能消费 pull 回来的稳定已审快照 |
| D5-06 | 分镜接触图用于审核，单张首尾帧用于模型输入 | 避免把带编号、文字和网格的接触图误当生成参考图 |
| D5-07 | Seedance Skill 是交付适配器，不是领域事实源 | 上游提示技巧可以复用，但不能绕过知识、权利、合规和审批门禁 |
| D5-08 | 价格、优惠、字幕、LOGO 和 CTA 默认后期合成 | 生成模型不适合保证文字准确性和时效权益真实性 |
| D5-09 | 发布后建立 Creative Binding，再导入效果 | 仅绑定 ApprovedSnapshot 无法确认究竟是哪条成片、哪组人群在产生结果 |
| D5-10 | 人群定制创意属于探索或“人群与表达匹配测试” | 同时改变人群和创意不是严格单变量 A/B，不能伪装成因果结论 |
| D5-11 | 候选与媒体生产在 Codex 本机执行，正式审批、存证和归因在服务端执行 | 保持 V3 本地创作、云端治理和显式 publish/pull 边界 |
| D5-12 | Seedance/抖音的登录、上传、生成和发布由用户在外部平台执行 | 服务端不持有平台账号、素材上传权限或生成平台代理权 |
| D5-13 | 租约内 Automation Agent 采用全权限、无交互执行，控制面在执行前后收口 | 真实文件、媒体、Shell 和网络任务不能在只读/禁工具模式下完成 |

## 4. 业务边界

V5 面向抖音电商短视频，核心目标是围绕商品成交形成可测的创意，而不是只制作品牌观感片。每个方向至少回答：给谁看、在什么需求时刻看、前三秒为什么停留、商品如何解决问题、证据是什么、为什么现在行动、用什么指标判断。

V5 不做以下事情：

- 不自动创建或修改抖音投放计划。
- 不将八大人群标签当作个体身份推断或敏感属性判断。
- 不承诺某类人群、某个钩子或 Seedance 提示词必然转化。
- 不让生成模型凭空生成 SKU 外观、功能演示、检测结论、价格或优惠。
- 不在未验证平台最新能力时，把社区 Skill 中的参数当作永久官方规格。
- 不因接入 Seedance 而废弃其他视频生成厂商的适配可能。

## 5. 文档导航

| 文档 | 内容 |
| --- | --- |
| [01-douyin-commerce-and-audience.md](./01-douyin-commerce-and-audience.md) | 抖音电商目标、八大人群交互、证据门禁与实验口径 |
| [02-domain-model-and-contracts.md](./02-domain-model-and-contracts.md) | 新增对象、关系、状态机、路径、不变量和兼容策略 |
| [03-storyboard-and-seedance-workflow.md](./03-storyboard-and-seedance-workflow.md) | 分镜生产、上游 Skill 引用边界、Seedance 最终复制格式 |
| [04-results-and-acceptance.md](./04-results-and-acceptance.md) | 发布绑定、指标归因、学习闭环、验收矩阵与 Golden Journey |
| [05-execution-boundaries.md](./05-execution-boundaries.md) | Codex、本地媒体、服务端治理与外部平台的唯一执行方和时序 |
| [06-automation-runtime-and-daemon.md](./06-automation-runtime-and-daemon.md) | Automation Codex 全权限模型、Daemon 生命周期、已安装升级和 Alook 借鉴边界 |
| [PLAN.md](./PLAN.md) | V5 唯一实施台账、工作包和评审门 |

## 6. 完成定义

V5 完成必须同时满足：

1. 用户可以选择单人群、2 至 3 类对比或八类探索，并看到每个策略的来源、版本、证据和置信度。
2. 被采用的 AudienceStrategyVersion、Brief、ContentItem、StoryboardPackage 和 SeedancePromptPackage 具有完整血缘。
3. 每个分镜都有可审核图片、首尾状态、运动、连续性、商品真实性、权利和禁止项。
4. 未锁定分镜不能导出 Seedance 包，引用素材缺失或摘要变化时旧包自动 stale。
5. 用户不需要重写提示词，即可按上传顺序和 `@图片N/@视频N/@音频N` 映射逐段复制到 Seedance。
6. 超过单段能力的内容按叙事节奏分段，每段有明确衔接点，不能简单按固定秒数截断。
7. 成片中的文字、价格、优惠和 CTA 可追溯到有效 OfferSnapshot，并通过后期合成与发布前检查。
8. 发布记录能唯一绑定 DeliveryPackage、实际成片、平台 creative/post、人群策略和实验臂。
9. PerformanceObservation 可以区分严格人群测试、创意测试和人群定制探索，人工决定不会被系统自动替代。
10. 官方能力变化、上游 Skill 变化或素材权利失效时，可以精确识别并重导出，不改写历史交付。

## 7. 参考事实与使用原则

以下资料用于建立 2026-07-28 的方案基线，实施时必须重新验证页面、能力和平台规则：

- 巨量学主页：<https://school.oceanengine.com/>
- 千川学堂：<https://school.oceanengine.com/page/academy-qianchuan?division_id=3003002>
- 巨量千川短视频投放技巧：<https://school.oceanengine.com/premium/course/6977642469018042375/intro>
- 巨量千川精细化投放攻略，涉及 A/B、人群与创意测试：<https://school.oceanengine.com/premium/course/7017684426054172679/intro>
- 云图极速版八大人群破圈课程，页面 API 标记 `modify_time=2025-03-27`：<https://school.oceanengine.com/premium/course/7336028836495366890/intro>
- 即创 AIGC 文图生视频素材生产方法论，页面日期 2025-12-25：<https://school.oceanengine.com/live/7587669755947909147>
- 上游 Seedance Prompt Skill 调研固定点：commit [`57d1e2f273747c238dd892698a05137ab2f10d4a`](https://github.com/songguoxs/seedance-prompt-skill/blob/57d1e2f273747c238dd892698a05137ab2f10d4a/.claude/skills/seedance/SKILL.md)，查询时间 2026-07-29。

巨量学资料作为营销方法和平台人群体系的优先参考；Seedance 上游 Skill 仅作为提示词工程与导出格式参考，不等同于字节跳动官方 API 契约。2026-07-29 查询时上游仓库 GitHub license 字段为 `null`，根目录没有 LICENSE；因此本仓库不复制其 SKILL.md，只记录固定 commit 并实现独立的 ContentCloud 适配流程。作者沟通记录或书面授权仍需作为内部 Evidence 归档，之后才能评估是否分发原文或派生模板。真实产品入口和能力上限仍必须按 provider profile 人工验证。
