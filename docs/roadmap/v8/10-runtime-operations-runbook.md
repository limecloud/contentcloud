# 10：Runtime 运维手册

> 本手册面向平台运维和支持人员，定义 Runtime Infra V2 的健康检查、准入灰度、排空、故障处置和回退边界。它以当前代码、迁移 `00035`～`00043` 和 `/api/v1/admin/runtime-health` 为事实源。

## 1. 上线前检查

上线或扩大灰度前，必须确认：

- 数据库已按顺序应用至 `00043_runtime_tool_call_results.sql`；不得跳过迁移或把旧迁移文件改写成当前事实。新媒体 Job 必须显式绑定 Runtime Effect，历史空关联行只能按 `legacy_unledgered` 处理。
- Server 和 standalone Worker 使用同一版本；`contentcloud-worker` 连接 PostgreSQL，并启用 `CONTENTCLOUD_AUTO_MIGRATE=1` 或由发布流程显式完成迁移。
- 至少有一个 Worker 正常运行，能够执行 reaper、业务结果 consumer 和 Runtime Explorer projector。
- `CONTENTCLOUD_RUNTIME_ADMISSION_ENABLED`、`CONTENTCLOUD_RUNTIME_DYNAMIC_GRAPH_ENABLED` 和 `CONTENTCLOUD_RUNTIME_CANARY_TENANT_IDS` 已记录在发布变更中。
- 平台管理员能够请求健康接口，并能保存一份发布前 JSON 作为对照证据。

代码级提交后故障钩子、核心 RLS 和 FairnessReport 已具备；真实 PostgreSQL 故障环境、生产容量压测、在线 Codex/Claude/MCP/Provider 和生产 Canary 仍是独立验收项。本手册不能把 Memory Store、离线 Harness 或可控钩子测试当作这些验收的替代品。

## 2. 健康检查

请求：

```text
GET /api/v1/admin/runtime-health
```

接口要求平台管理员身份，返回平台状态和每个 active tenant 的：

- `projection_outbox`：Runtime Explorer 投影订阅的 pending 数量和最早 pending 时间。
- `business_outbox`：终态业务结果订阅的 pending 数量和最早 pending 时间。
- `reaper` / `delivery`：最近启动、成功、失败状态及 Worker ID。
- `alerts`：稳定错误码、severity、积压数和年龄。

当前阈值由服务端返回在 `thresholds` 字段中，默认值如下：

| 信号 | Warning | Critical |
| --- | ---: | ---: |
| reaper/delivery 成功心跳年龄 | 15 秒 | 60 秒 |
| projection/business 最早 pending 年龄 | 60 秒 | 300 秒 |
| projection/business pending 数量 | 100 | 1000 |

没有成功心跳或维护状态为 `failed` 时直接视为 `critical`。顶层 `status` 取所有 active tenant 的最严重状态。租户被暂停不会产生新的健康条目，但已存在的 JobRun 仍必须通过 reaper 和订阅者收敛。

建议每 15 秒抓取一次接口并告警去重；不要把 tenant ID、JobRun ID 或 message ID 放入高基数指标标签。

## 3. 灰度和停止准入

环境变量由 Server 启动时读取，修改后需要安全重启 Server：

```text
CONTENTCLOUD_RUNTIME_ADMISSION_ENABLED=1|0
CONTENTCLOUD_RUNTIME_DYNAMIC_GRAPH_ENABLED=1|0
CONTENTCLOUD_RUNTIME_CANARY_TENANT_IDS=<uuid,uuid,...>
```

准入策略按以下规则解释：

1. 两个开关未配置时默认按 `0` 处理，生产进程必须 fail-closed，不能因为环境文件漏项而全量开放 Runtime。
2. `ADMISSION_ENABLED=0`：拒绝新的 Runtime JobRun；读取、reaper、投影、业务结果消费和已运行 Attempt 不停止。
3. `ADMISSION_ENABLED=1` 且 `CANARY_TENANT_IDS` 非空：只允许列表中的租户创建新 JobRun。
4. `DYNAMIC_GRAPH_ENABLED=0`：拒绝 GraphPatch、Fanout 等新的动态图变更；已冻结执行图继续运行。
5. `CANARY_TENANT_IDS` 同时限制准入和动态图。扩大列表前先检查每个租户的健康状态和 backlog。

推荐灰度顺序：

```text
关闭新准入 -> 应用迁移 -> 单租户 Canary -> 观察健康接口和业务结果 -> 扩大租户列表 -> 最后打开动态图
```

发生事故时，先把 `ADMISSION_ENABLED` 设为 `0`；如果问题只涉及图变更，再单独把 `DYNAMIC_GRAPH_ENABLED` 设为 `0`。不要通过删除 JobRun、手工改 Attempt 状态或直接清空 outbox 来止血。

## 4. 排空和重启

排空顺序：

1. 关闭新准入，保留 Server 读接口和至少一个 Worker。
2. 轮询健康接口，直到 projection/business pending 处于 0 或已在可接受阈值内，并确认没有需要人工处理的 `unknown` Effect。
3. 确认没有 `prepared`/`active` Attempt，或已记录其恢复计划；不要用进程退出代替租约回收。
4. 先停止要维护的 Worker，保留另一实例接管；单实例维护时接受心跳告警，但要在租约 TTL 内恢复。
5. Server 重启后先验证健康接口、数据库连接和准入配置，再恢复租户列表。

Worker 每轮约每 2 秒执行一次，每个 active tenant 的处理上限由 `limit=50` 控制。reaper 会把过期 Attempt 收敛为 `expired` 并释放 Node/Agent/Reservation；旧 worker 随后的心跳、事件和终态提交必须被 fence 拒绝。

## 5. 故障处置

### 5.1 `RUNTIME_REAPER_STALLED`

检查 Worker 日志中的数据库连接、迁移版本和 `RUNTIME_REAPER_FAILED`。确认另一 Worker 是否已有成功心跳；若没有，恢复一个 Worker，而不是手工更新 heartbeat。恢复后验证过期 Attempt 已收敛、资源预留已释放，旧 fence 上报仍返回 stale 错误。

### 5.2 `RUNTIME_PROJECTION_LAG`

权威 JobRun/JobEvent 不受投影延迟影响。先确认 projector receipt 是否持续被 claim，检查数据库锁和 Worker 错误，再重启或增加 Worker。不要修改 JobEvent 序列，也不要手工 ack 未处理的 receipt。必要时使用现有 projection rebuild/dry-run 生成新投影，确认 `external_calls=0` 后再切换读模型。

### 5.3 `RUNTIME_BUSINESS_RESULT_BACKLOG`

检查 `runtime-result:` Blob 是否存在、摘要是否匹配，以及业务对象是否已经按确定性 ID 写入。业务写入成功但 ack 失败时允许同一 receipt 重领并幂等核对；不同摘要必须保持冲突并退避。禁止直接标记 receipt delivered 或重复执行外部 Provider 请求。

### 5.4 Provider `unknown` 或费用差异

维持 `unknown/reconciling`，先调用对账入口或按 Provider 支持流程查询外部编号和账单。确认前不重提；确认已执行后补写本地事实，确认未执行且策略允许时才创建新的 Attempt。账单差异保留为审计事实并升级人工处理。

### 5.5 Fence stale、事件重复或乱序

这是预期保护行为。保留错误码、Attempt ID 和 request ID，确认新 Attempt 已持有租约；不要为旧 worker 延长 lease，不要删除迟到事件。相同终态摘要可以幂等重报，不同摘要必须保持冲突。

## 6. 前向回退

Runtime 迁移采用前向演进。应用 `00035` 后，`runtime_outbox` 的消费者状态已迁到 `runtime_outbox_receipts`；应用 `00036` 后，旧 session 镜像表不再存在；`00042`/`00043` 还增加了 Explorer/幂等索引和 ToolCall 安全结果字段。因此生产回退只允许：

- 保留已理解 `00035`～`00043` schema 的当前二进制，关闭新准入或动态图；
- 修复配置、Worker 或消费逻辑后重新启动，并用健康接口确认追平；
- 需要恢复旧版本时，先在隔离数据库副本验证 schema 兼容性，再走经批准的数据库恢复流程。

禁止在生产直接执行迁移 Down、重新创建 `runtime_agent_sessions/events`、恢复 `DurableHarness`/`SessionStore` 或把 receipt 合并回 outbox。`00036` 的 Down 迁移是有意留空，表示旧事实源 forbidden-to-restore。

## 7. 事故结束条件

事故只有在以下条件全部满足后关闭：

- 所有 active tenant 的顶层健康状态为 `healthy`，连续观察至少 5 分钟。
- reaper 和 delivery 均有新的成功心跳，projection/business backlog 没有持续增长。
- 受影响 JobRun 的 Attempt、Reservation、Effect 和业务结果均有可追踪终态；没有未登记的手工状态修改。
- 准入和动态图配置恢复到事故前的明确值，Canary 列表和变更记录一致。
- 已保存健康接口 JSON、Worker 日志、相关 JobEvent/request ID 和故障时间线，供支持案例和后续复盘使用。
