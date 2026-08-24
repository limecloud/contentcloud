# 视频生产迁移：本轮执行计划

状态：`complete`。

更新时间：2026-08-23。

本文件是阶段 D 的短周期执行计划，完成后将结果回写到 [执行台账](./13-video-production-execution.md)、[迁移计划](./12-video-production-migration.md)、[能力地图](../../infra/01-capability-map.md) 和 [交付路线图](../../infra/03-delivery-roadmap.md)。它不新增业务事实模型，也不替代阶段门槛。

## 目标

闭环 Provider 已返回终态时的本地一致性：

```text
Provider Status(终态)
  -> ProviderAttempt.ActualCostMinor
  -> MediaGenerationJob.ActualCostMinor
  -> MediaGenerationJob / ProviderAttempt 终态
  -> Runtime Effect 终态（存在 Runtime 绑定时）
```

重点覆盖：

- `succeeded`、`failed`、`cancelled` 返回实际费用时，任务和调用尝试使用同一数值；
- 取消请求超时后，后续状态查询确认 `cancelled` 时，任务、调用尝试和 Runtime Effect 一起收敛；
- 状态查询仍为未知时继续进入可调度的外部对账状态，不重复提交；
- Memory Store 与 PostgreSQL 使用同一持久化契约。

## 任务清单

- [x] 抽取终态费用回写逻辑，避免成功、失败、取消分支产生不同事实。
- [x] 修复取消 unknown 后确认取消时 Runtime Effect 未收敛的问题。
- [x] 增加失败、取消恢复和状态查询 unknown 的回归测试；成功路径复用既有营销视频 Golden Journey。
- [x] 运行 `go test ./...` 与 `git diff --check`。
- [x] 将证据同步到四份路线图文档；阶段 D 保持 `in_progress`，直到真实 Provider 验收和完整账单对账完成。

本轮结果：终态 Provider 返回的 `ActualMinor` 会在下载、Artifact 或审核处理前同时写入 `ProviderAttempt.ActualCostMinor` 和 `MediaGenerationJob.ActualCostMinor`；取消 unknown 后收到 `cancelled` 状态时，Runtime Effect 会从 unknown 经对账收敛到 failed。`TestFailedMediaGenerationPersistsActualProviderCost` 与取消恢复测试覆盖了失败和取消分支，完整测试命令见执行台账。

## 完成门槛

只有同时满足以下条件，才可勾选本轮完成：

1. 终态实际费用在 `ProviderAttempt` 与 `MediaGenerationJob` 可查询且一致。
2. 取消 unknown 的恢复路径不调用第二次取消或提交，并能收敛 Runtime Effect。
3. 测试覆盖正常、失败、重复处理和恢复路径。
4. 文档中的状态、证据和下一步与代码及测试结果一致。
