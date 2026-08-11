# ADR-0016：Runtime 统一事务命令内核

状态：`Proposed`（代码已按本决策实现，正式 Accepted 仍需架构评审）。

日期：2026-08-08。

决策者：平台工程与运行时维护者。

关联：

- [Runtime Infra V2](../../roadmap/v8/09-runtime-infra-v2.md)
- [ADR 规范](./README.md)

## 背景

V8 Runtime 已经有 JobRun、NodeRun、RuntimeAttempt、State、Effect 和 JobEvent。若业务服务先调用宽写方法修改快照，再单独写事件，进程在两个调用之间退出时，读模型和 SSE 没有办法知道状态已经改变；事件写入失败还可能被忽略。

这不是 PostgreSQL 或 Memory Store 的实现细节，而是 Runtime 的权威边界错误。所有会改变 Runtime 事实的操作都需要一个明确的命令入口。

## 决策

1. Runtime 写入通过 `RuntimeCommandStore` 窄端口完成；Service 不再组合 `Save*` 与 `AppendRuntimeEvent` 来表达状态迁移。
2. 一个命令必须在同一提交边界内完成快照版本更新、追加 JobEvent 和写入 `runtime_outbox`。
3. `JobEvent` 是追加事实，`runtime_outbox` 是不可变消息；每个订阅者通过独立 `runtime_outbox_receipts` 维护领取、重试和确认状态。投影、业务结果 consumer、SSE、对账器都不能直接读取未提交的内存状态，也不能共享一个全局 `delivered_at`。
4. PostgreSQL 是生产权威实现，Memory Store 必须保持相同的命令和幂等语义，用于契约测试。
5. 旧的宽写方法在没有调用者后删除；不保留“新命令失败时回退到旧 Save + Event”的双写分支。V7 执行表、`RunAttempt`、daemon 命令链和旧公开 DTO 名称属于 dead；公开读取统一使用 `RuntimeRun` / `RuntimeRunEvent`。

命令边界如下：

```text
Service command
    -> validate + lock/CAS aggregate
    -> snapshot version + JobEvent + immutable runtime_outbox
    -> one receipt per matching subscriber
    -> commit
    -> projector / business result consumer / reconciler consume independently
```

## 备选方案

### 方案 A：继续在 Service 中组合多个 Store 方法

实现最少，但无法证明状态和事件原子提交，故障恢复依赖调用顺序和人工修复。不采用。

### 方案 B：引入 Temporal/Kafka 作为新的权威层

可以获得成熟的工作流和消息能力，但会引入第二套状态、部署和运维面；当前吞吐和团队边界还没有证明需要这笔复杂度。不采用。

### 方案 C：PostgreSQL-first RuntimeCommandStore

保留模块化单体和现有 RLS/复合外键，在命令边界稳定后再按指标拆分投影或调度服务。采用。

## 事实所有权与边界

| 事实 | 权威所有者 | 其他模块 |
| --- | --- | --- |
| JobRun、NodeRun、State、Effect | RuntimeCommandStore | 只能通过命令读取或修改 |
| JobEvent | Runtime append-only event log | 只能追加，不得覆盖或删除 |
| 结构化业务结果 Blob 引用 | Runtime Attempt/Node output refs | 固定引用和摘要，不拥有知识、内容或交付业务事实 |
| 知识、内容、交付对象 | 对应业务拥有域 | 从已验证 output ref 幂等派生，不回写 Runtime 终态 |
| 投影、SSE、运营列表 | Projector / read model | 允许延迟和重建，不回写 Runtime |
| 宿主会话和进程日志 | Harness / execution evidence | 不能作为恢复依据 |

## 兼容与迁移

- `runtime_job_events` 和既有 Runtime 表保留，不重写历史事件。
- 新迁移 `00018_runtime_command_kernel.sql` 只新增 outbox，不删除旧数据。
- 迁移 `00019_runtime_outbox_delivery.sql` 为 outbox 增加持久化消费者租约；消费者只能通过 `claim/ack/retry` 窄接口推进投递。
- 迁移 `00020_runtime_append_only_permissions.sql` 撤掉 Runtime 角色对 JobEvent、计划、检查点和其他不可变快照的直接更新/删除权限。
- 迁移 `00021_runtime_fencing_and_resources.sql` 增加 Node/Attempt fence、租户资源配额和带围栏的 Reservation 账本。
- 迁移 `00022_runtime_state_tool_calls.sql` 增加 Checkpoint 游标/水位、StateCollection/StateRecord、ToolCall，并让历史 Effect 的 Attempt/Reservation 绑定保持可空。
- 迁移 `00023_runtime_projection.sql` 增加带 RLS 的 Runtime Explorer 快照。
- 迁移 `00024`～`00029`、`00031`～`00033` 固定 Job 契约、关系化计划/Fanout、Provider、Yield/Resume、投影重建、业务绑定和受控输入输出；Session Mirror 不属于首个用户 current Runtime。
- 迁移 `00034_remove_v7_execution.sql` 解开 JobRun 对单一 WorkTask 的物理外键，并删除 V7 `task_runs/run_attempts/run_progress_events/creative_execution_bundles`。
- 迁移 `00035_runtime_outbox_subscribers.sql` 把投递状态从 `runtime_outbox` 移入独立 receipts；历史 Projector 状态原样迁移，历史成功 Attempt 为业务结果 subscriber 补 pending receipt 并依赖摘要围栏幂等重放。
- Runtime Service 的 Job/Node/State/Effect/StateRecord/ToolCall/Dispatch 写路径已切换到命令端口；Harness 事件通过 `AppendFencedRuntimeEvent` 校验 Attempt、owner、lease 和 fence，并在同一事务内追加 JobEvent/outbox，不能走普通事件入口或直接写表。
- `RuntimeRun` / `RuntimeRunEvent` 是从 JobRun/NodeRun/JobEvent 生成的 current JSON 读模型；`TaskRun` / `RunProgressEvent` 类型、V7 Store、写 API、RunAttempt 领域对象和 daemon 执行协议已删除，禁止恢复。

## 安全与运行影响

- `runtime_outbox`、`runtime_outbox_receipts` 与 Runtime 表使用同一租户 RLS；Runtime 角色不能更新或删除不可变 outbox 消息。
- 投影失败只会增加 outbox 重试，不会回滚已经提交的业务状态。
- outbox 消费者必须按 `subscriber + event_id` 幂等，不能因为重试再次触发外部副作用或重复业务写入。
- 每个 subscriber receipt 使用独立 `locked_by + locked_until` 围栏；一个订阅者的 ack 不能吞掉另一个订阅者，过期消费者不能确认或安排重试。
- 运营页面需要显示投影延迟和积压数量，不能把投影状态伪装成权威实时状态。

## 验证

1. Memory/PostgreSQL 命令契约覆盖成功、版本冲突、重复幂等键和事件重复。
2. 注入事件或 outbox 写入失败时，快照更新整体回滚。
3. 同一事件只能有一条 outbox 消息；每个匹配 subscriber 只能有一条 receipt，一个 subscriber 的 ack 不改变其他 receipt。
4. 终态后、业务消费前重启，以及业务写成功、ack 前重启，均从 pending receipt 恢复；不同 Blob 摘要拒绝，重复消费不产生第二个业务对象。
5. `GOMAXPROCS=2 go test -p 1 ./...`、迁移集合校验和 `node scripts/check-architecture.mjs` 通过；真实 PostgreSQL RLS/迁移集成已覆盖核心 Runtime 表，代码级 `after_commit` 故障钩子和幂等恢复用例已补，真实数据库进程/网络故障演练仍未完成。

## 回退

若新命令端口导致准入或运行阻断，停止新 Runtime 准入并保留已提交事件与 outbox；修复命令实现后从权威状态继续。不得恢复 Service 层的跨调用双写路径。

## 后果

正面：状态、事件和投影通知有单一提交边界；故障可重试，读模型可重建；Memory 与 PostgreSQL 共享契约。

代价：新增命令端口、outbox 迁移和契约测试；已删除旧 Store 宽写方法，后续只允许通过命令端口扩展 Runtime 写入。
