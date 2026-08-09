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
3. `JobEvent` 是追加事实，`runtime_outbox` 只负责可靠投递；投影、SSE、对账器都不能直接读取未提交的内存状态。
4. PostgreSQL 是生产权威实现，Memory Store 必须保持相同的命令和幂等语义，用于契约测试。
5. 旧的宽写方法在没有调用者后删除；不保留“新命令失败时回退到旧 Save + Event”的双写分支。V7 执行表、`RunAttempt` 和 daemon 命令链属于 dead；`TaskRun` 仅作为 Runtime 只读业务 DTO 暂时兼容。

命令边界如下：

```text
Service command
    -> validate + lock/CAS aggregate
    -> snapshot version + JobEvent + runtime_outbox
    -> commit
    -> projector / SSE / reconciler consumes outbox
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
- 迁移 `00024`～`00033` 固定 Job 契约、关系化计划/Fanout、Provider、Yield/Resume、投影重建、SessionStore、业务绑定和受控输入输出。
- 迁移 `00034_remove_v7_execution.sql` 解开 JobRun 对单一 WorkTask 的物理外键，并删除 V7 `task_runs/run_attempts/run_progress_events/creative_execution_bundles`。
- Runtime Service 的 Job/Node/State/Effect/StateRecord/ToolCall/Dispatch 写路径已切换到命令端口；Harness 事件仍通过 `AppendRuntimeEvent` 进入同一 outbox 边界，不能直接写表。
- `TaskRun` 只保留为 Runtime 的 JSON 业务投影 DTO；V7 Store、写 API、RunAttempt 领域对象和 daemon 执行协议已删除，禁止恢复。

## 安全与运行影响

- `runtime_outbox` 与 Runtime 表使用同一租户 RLS。
- 投影失败只会增加 outbox 重试，不会回滚已经提交的业务状态。
- outbox 消费者必须按 `event_id` 幂等，不能因为重试再次触发外部副作用。
- outbox 认领使用 `locked_by + locked_until` 围栏；过期消费者不能确认或安排重试。
- 运营页面需要显示投影延迟和积压数量，不能把投影状态伪装成权威实时状态。

## 验证

1. Memory/PostgreSQL 命令契约覆盖成功、版本冲突、重复幂等键和事件重复。
2. 注入事件或 outbox 写入失败时，快照更新整体回滚。
3. 同一事件只能有一条 outbox 记录；重复消费不会产生第二条状态变化。
4. `GOMAXPROCS=2 go test -p 1 ./...`、迁移集合校验和 `node scripts/check-architecture.mjs` 通过；真实 PostgreSQL RLS/迁移集成与提交后崩溃注入仍未完成。

## 回退

若新命令端口导致准入或运行阻断，停止新 Runtime 准入并保留已提交事件与 outbox；修复命令实现后从权威状态继续。不得恢复 Service 层的跨调用双写路径。

## 后果

正面：状态、事件和投影通知有单一提交边界；故障可重试，读模型可重建；Memory 与 PostgreSQL 共享契约。

代价：新增命令端口、outbox 迁移和契约测试；已删除旧 Store 宽写方法，后续只允许通过命令端口扩展 Runtime 写入。
