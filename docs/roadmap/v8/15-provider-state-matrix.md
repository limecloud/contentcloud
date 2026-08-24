# 视频生产迁移 Provider 状态查询恢复矩阵计划

状态：`in_progress`。

更新时间：2026-08-23。

本文件是阶段 D 的下一轮短周期计划，负责把 Provider 状态查询的异常、进行中和终态恢复路径固定为可验证矩阵。当前总进度见 [视频生产迁移推进计划](./16-video-production-progress-plan.md)。它不新增任务、资产或外部操作事实模型；结果必须回写到 [执行跟踪台账](./13-video-production-execution.md) 和 [迁移计划](./12-video-production-migration.md)。

## 目标

对已经拥有远端任务 ID 的媒体任务，确保每次状态查询都只产生以下一种可追溯结果：

```text
Status error      -> awaiting_external_result + PROVIDER_STATUS_UNKNOWN
running/queued    -> awaiting_external_result + scheduled poll
succeeded         -> cost -> download -> Artifact/Review -> succeeded
failed            -> cost -> failed
cancelled         -> cost -> cancelled (仅在已请求取消时)
```

恢复不得再次调用 `Submit`；`unknown` 必须保留外部任务 ID、最近轮询时间、下一次轮询时间和安全错误摘要。所有终态实际费用必须同时写入 `ProviderAttempt` 与 `MediaGenerationJob`，并在存在 Runtime Effect 时收敛对应外部操作。

## 验收矩阵

| 情形 | 本地任务状态 | ProviderAttempt | 允许的下一步 |
| --- | --- | --- | --- |
| 状态查询返回错误 | `awaiting_external_result` | `unknown`，写入 `PROVIDER_STATUS_UNKNOWN`、`LastPolledAt`、`NextPollAt` | 到期后只轮询，不重新提交 |
| 状态为 `queued`/`running` | `awaiting_external_result` | 保存 Provider 状态与下一次轮询时间 | 到期后继续轮询 |
| 状态为 `succeeded` | 进入下载/校验，最终 `succeeded` | 记录实际费用、产物摘要和完成时间 | 进入 Artifact、MediaReview |
| 状态为 `failed` | `failed` | 记录实际费用和失败码 | 不得盲目重提 |
| 状态为 `cancelled` 且本地已请求取消 | `cancelled` | 记录实际费用和完成时间 | Runtime Effect 收敛为 failed |
| 重复恢复调用 | 保持当前终态或等待态 | 不新增 Attempt | 不得调用第二次 Submit/Cancel |

## 执行清单

- [x] 增加状态查询错误后恢复到 `running` 的回归测试。
- [x] 增加状态查询错误后恢复到 `failed` 与已请求取消 `cancelled` 的回归测试。
- [x] 验证恢复调用不创建新的 `ProviderAttempt`，也不重复提交外部任务。
- [ ] 在 Memory 与 PostgreSQL 存储上核对相同字段和版本语义。
- [ ] 用真实 Provider 测试环境回放至少一次状态查询错误和终态账单回执。
- [ ] 测试通过后同步四份路线图；真实 Provider 和账单证据不足时，阶段 D 仍保持 `in_progress`。

本轮已完成：状态查询超时会在任务上写入 `PROVIDER_STATUS_UNKNOWN`，保留远端任务 ID、最近轮询时间和下一次轮询时间；下一次处理恢复到 `running`、`failed` 或已请求取消的 `cancelled` 时只调用 `Status`，不创建新的 `ProviderAttempt`，也不调用 `Submit`。终态实际费用仍同时写入任务与调用尝试。证据：`TestProviderStatusUnknownRecoversByPollingWithoutResubmit`、`TestProviderStatusUnknownRecoversToTerminalStatesWithoutDuplicateAttempt`。

本轮文档更新项：D1“模拟状态恢复矩阵”已完成并登记到推进计划；阶段 D 总状态仍保持 `in_progress`，因为 Memory/PostgreSQL 对照、真实 Provider 和真实账单回执尚未完成。

## 完成门槛

只有同时满足以下条件，才能关闭本计划：

1. 矩阵中的每个分支都有正常、异常、重复和恢复测试。
2. `ProviderAttempt`、`MediaGenerationJob` 与 Runtime Effect 的终态和费用可查询且一致。
3. Memory 与 PostgreSQL 的持久化行为一致，并通过租户/版本校验。
4. 至少一条真实 Provider 运行证据补齐模拟测试无法覆盖的协议和账单语义。
