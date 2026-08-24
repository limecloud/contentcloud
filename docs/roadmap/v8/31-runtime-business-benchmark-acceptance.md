# Runtime 业务标杆验收

状态：`in_progress`。

更新时间：2026-08-23。

本文件记录 W8-13 和 W8-14 的业务标杆证据。标杆的目的不是把某个业务流程硬编码进 Runtime，而是证明不同的工作台表达可以复用同一套执行图、Fanout/Join、审批快照、产物、交付和效果事实。

## 分层边界

```text
业务工作台 / 业务简报
        -> 应用编排与 SOP
        -> Runtime JobRun / NodeRun / FanoutSet / Join
        -> SubmissionRevision / ApprovedSnapshot
        -> Artifact / DeliveryPackage
        -> PerformanceObservation / RatingDecision
```

工作台只提供业务语言、成员集合和版本化输出契约；Runtime 不拥有营销活动、文章或渠道专用表，也不直接创建审批、产物或交付事实。

## W8-13 营销活动

已完成的 Memory 验收：

- 以营销活动 SOP 启动一个 Runtime JobRun；固定 `BusinessType`、输入摘要、执行策略和契约版本。
- 通过平台 Fanout/Join 冻结 10 个渠道变体成员，使用 Runtime 依赖解锁、节点版本 CAS、租约状态和汇聚策略。
- 为十个内容变体分别创建 `SubmissionRevision`，完成内部审核和客户 OTP 决定。
- 每个客户批准版本生成三个格式的 `Artifact` 和一个 `DeliveryPackage`，所有产物携带 `ApprovedSnapshotID`。
- 结果导入按批准快照归因，并为十个版本创建 `RatingDecision`；没有复制活动专用的审批或交付状态。

证据：

- `internal/runtime/marketing_campaign_benchmark_test.go`
- `internal/application/marketing_campaign_benchmark_test.go`

这证明了平台主链在十路并行规模下的结构和血缘，但不等于真实媒体服务商、字幕/旁白合成、外部渠道发布或生产容量已经验收。

## W8-14 文章复盘

已完成的 Memory 验收：

- 使用结构不同的文章复盘 SOP，把结果导入、渠道归因和学习候选拆成 Runtime 阶段。
- 通过四渠道 Fanout/Join 并行归因，汇聚节点只在所有成员成功后变为可执行。
- 四条渠道观察均指向同一个 `ApprovedSnapshot`，由一条 `RatingDecision` 记录下一轮受控假设。
- 既有 50 节点容量边界测试继续验证动态规模上限，第二批超限返回 `JOB_PLAN_NODE_LIMIT`，没有新增文章复盘调度表。

证据：

- `internal/application/article_retrospective_benchmark_test.go`
- `internal/runtime/fanout_test.go`

## 仍未通过的生产门槛

- 专用 PostgreSQL/RLS 和 Blob 故障组合的真实执行，不能用 Memory 测试替代。
- 真实媒体 Provider、真实账单回执、字幕/旁白/品牌画面合成和外部渠道回执。
- 100 节点/20 worker 的生产公平性、容量和告警压测。
- Runtime Explorer 动态图操作已完成 Memory/BFF/Web 验收；仍缺跨进程清理恢复和生产 Canary/回退演练。

在这些证据完成前，W8-13/W8-14 只能标记为“业务标杆已具备、生产验收进行中”，不能宣称平台已经完成生产上线。
