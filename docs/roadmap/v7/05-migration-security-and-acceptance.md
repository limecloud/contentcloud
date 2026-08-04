# V7 迁移、安全、测试与验收

## 1. 迁移原则

V7 采用 strangler 迁移，不一次性重写全部 V3-V6 能力：

1. 先增加规范 `TaskStageOutput` 和任务聚合投影。
2. 让新 `marketing_video` SOP 只写规范对象。
3. 工作台优先读取规范投影，并保留旧 TaskRevision/TaskDelivery fallback。
4. 拒绝新建空 manifest 的 delivered 记录。
5. 对历史记录离线计算 `integrity_status`，不改变原始状态和审计事件。
6. 经过一个兼容窗口后，再评审旧写 API 和裸 `output_refs` 的删除。

## 2. 历史数据分类

| 分类 | 条件 | V7 显示 |
| --- | --- | --- |
| `complete` | 有最终 Artifact、通过质检和 DeliveryPackage | 正常已交付 |
| `script_only` | 有 TaskRevision 内容，无视频 Artifact | 已交付脚本，不是成片 |
| `legacy_incomplete` | TaskDelivery delivered 但 manifest 空或不可解析 | 旧版不完整交付 |
| `missing_artifact` | manifest 引用不存在或摘要不匹配 | 完整性失败，需人工处理 |
| `unclassified` | 无法确定旧引用语义 | 保留原记录并提示检查 |

迁移脚本只新增分类和投影，不删除、伪造或自动下载历史外部文件。

## 3. 发布顺序

```text
R1 additive migrations + read models
  -> R2 dual-read UI + legacy integrity labels
  -> R3 typed StageOutput write path
  -> R4 knowledge/content/storyboard canonical pipeline
  -> R5 Media Worker + FakeProvider
  -> R6 one real Provider behind tenant feature flag
  -> R7 final QA + Delivery hard gate
  -> R8 migrate default marketing_video SOP
  -> R9 disable legacy empty-delivery writes
```

每个 Release 都可通过 Feature Flag 回退到上一读路径；数据库迁移保持向后兼容，不在同一发布删除列或表。

## 4. 安全威胁模型

| 威胁 | 控制 |
| --- | --- |
| 跨租户对象引用 | tenant/project 复验、RLS、服务端派生归属 |
| SSRF / 云元数据读取 | 固定 Provider endpoint、DNS/IP 检查、禁止任意 URL、下载 allowlist |
| Secret 泄露 | SecretRef、最小权限注入、日志脱敏、前端永不返回 |
| Provider callback 伪造/重放 | 签名、时间窗、event id、body digest、二次对账 |
| 恶意或损坏媒体 | 大小限制、MIME 嗅探、quarantine、媒体探测、可选恶意文件扫描 |
| Prompt/来源指令注入 | 来源正文始终是不可信数据，不作为系统指令执行 |
| 权利或事实越权 | approved snapshot、rights、有效期和 Gate 强制复验 |
| 重复提交导致重复扣费 | 稳定 idempotency key、活动 Job 唯一约束、未知结果先对账 |
| 签名下载 URL 泄露 | 短 TTL、授权后签发、日志过滤、禁止长期缓存 |
| 超额费用 | 预算、并发、重试上限、费用 Gate 和审计 |
| Provider 数据保留不符合租户政策 | Profile 披露、租户能力开关、区域和保留策略匹配 |
| 视频解码资源耗尽 | 受限 Worker、超时、CPU/内存配额、最大像素/时长/轨道数 |

## 5. 失败模式

| 代码路径 | 生产失败 | 处理 | 测试 | 用户体验 |
| --- | --- | --- | --- | --- |
| Source ingest | 对象存储写入中断 | 不创建 ready Revision，可重试同一 digest | 集成测试 | 显示上传失败和重试 |
| Evidence parse | 不支持格式或 OCR 低置信 | failed/needs_review，不阻断其他来源 | Worker 单测 | 显示具体来源和原因 |
| Stage report | 引用不存在或 digest 漂移 | 409/Policy，Stage 保持 running/blocked | 服务层测试 | 导航到失效对象 |
| Provider submit | timeout，结果未知 | 先对账，禁止盲重提 | FakeProvider 集成 | 显示“正在确认是否已提交” |
| Provider poll | 429/5xx | Retry-After + backoff | Worker 时钟测试 | 显示最近确认时间 |
| Callback | 重复或乱序 | 幂等忽略并审计 | HTTP 测试 | 无重复状态跳动 |
| Download | URL 过期或跳到私网 | 重新取 URL或阻断，绝不跟随私网 | 安全测试 | 显示输出下载失败 |
| Artifact validation | HTML/空文件/损坏 MP4 | output_invalid，不完成 Job | 媒体 fixture 测试 | 可重试或联系运营 |
| Take selection | 两标签页同时选择 | expected version 冲突 | 服务+E2E | 提示刷新，保留已选结果 |
| Final render | Offer 已过期 | 阻断渲染，要求更新 Offer | 服务测试 | 显示失效权益 |
| Delivery | manifest 空或 Artifact 缺失 | 拒绝 delivered | 回归测试 | 显示缺少最终成片 |

没有任何错误允许静默进入 `delivered`。

## 6. 测试覆盖图

```text
CODE PATHS                                      USER FLOWS
[+] Stage output validation                    [+] 来源到知识
  |-- valid canonical ref [UNIT+STORE]            |-- upload/parse/review [E2E]
  |-- missing/cross-tenant/drift [UNIT+RLS]        |-- empty/failed source recovery [E2E]
  `-- stale stage/idempotent replay [UNIT]

[+] MediaGenerationJob                         [+] 剧本到视频
  |-- cost estimate/approval [UNIT+HTTP]           |-- read script and approve [E2E]
  |-- claim/lease/recovery [STORE+RACE]             |-- storyboard lock [E2E]
  |-- submit/poll/callback [INTEGRATION]             |-- cost confirm/cancel/retry [E2E]
  |-- unknown submit reconciliation [INTEGRATION]    |-- live progress/reconnect [E2E]
  |-- download/validate/store [INTEGRATION]          `-- take preview/select [E2E]
  `-- budget/rate/credential failures [UNIT]

[+] Final render and Delivery                  [+] 质检到交付
  |-- overlays and Offer expiry [UNIT]             |-- review/request changes [E2E]
  |-- exact artifact digest [UNIT+STORE]            |-- final approve/download [E2E]
  |-- non-empty manifest [REGRESSION]               |-- legacy incomplete label [E2E]
  `-- cross-tenant download [RLS+HTTP]               `-- artifact unavailable recovery [E2E]

[+] Provider adapters                           [+] Operations
  |-- contract suite for every adapter [TESTKIT]     |-- misconfigured Provider [E2E]
  |-- FakeProvider all states [INTEGRATION]           |-- budget/concurrency dashboard [E2E]
  `-- real provider opt-in smoke [STAGING]            `-- worker crash/recovery [CANARY]
```

## 7. 测试清单

### Domain / Service

- TaskStageOutput 类型、状态、digest 和 Stage Contract 正反例。
- Provider Job 全状态转移、非法跳转、取消、重试和终态幂等。
- 费用 decimal、预算边界、并发和重试计费。
- MediaReview 精确绑定 Artifact digest、单一 selected take 和 stale write。
- DeliveryPackage 必须包含 final video 和 manifest。
- **CRITICAL regression**：空 `manifest` 不得把 Task 标成 `delivered`。
- **CRITICAL regression**：工作台 Stage report 不能用 `{passed:true}` 绕过 required checks。

### Store / Migration

- Memory Store 和 PostgreSQL Store 使用同一 contract tests。
- 新表 tenant RLS、跨项目引用、唯一索引和 SKIP LOCKED 并发领取。
- Migration up 从真实 V6 schema 执行；旧 Server 在 additive migration 后仍可启动。
- 历史 complete/script_only/legacy_incomplete/missing_artifact 分类 fixtures。

### Provider / Worker

- submit success、timeout unknown、429、5xx、policy rejection、budget block。
- poll 乱序、重复 callback、签名失败、重放和未知 Job。
- 下载 redirect、私网地址、超大文件、错误 MIME、空文件、损坏 MP4。
- Worker 在 submitting/downloading/validating 阶段退出后恢复。
- 相同 idempotency key 不产生两个外部 Job。

### Web

- Revision 正文、diff、引用、分镜帧、Job、takes、MediaReview 和 Delivery manifest 渲染。
- loading/empty/error/blocked/stale/unauthorized 状态。
- SSE cursor 恢复和轮询 fallback。
- 双击提交、页面离开、会话过期、慢 API 和两标签页冲突。
- 1440x1000、390x844、320px 宽度布局与播放器。

### E2E

- FakeProvider 下完整 Golden Journey：来源 -> Evidence -> 知识 -> 内容 -> 分镜 -> 视频 -> 质检 -> 交付。
- 金陵古都香 blocked candidate 不得越过产品/rights Gate。
- 管理员禁用媒体能力后，历史可读但不能创建新 Job。
- Provider misconfigured 时任务显示修复动作，不显示通用 500。
- final Artifact 被 quarantine 或 missing 时拒绝交付。

### Staging

- 真实 Provider 小额 submit/poll/download/cancel。
- Provider 账单与 actual cost 对账。
- 对象存储大文件流式上传/下载和签名 URL。
- Worker 滚动重启、数据库故障、Provider 故障和回滚演练。

## 8. 性能与容量

- Job 列表、Stage outputs、Artifacts 和审计分页；不在 Task 聚合中无限内联历史 Attempt。
- Task projection 使用批量查询或明确 preloading，禁止按 Stage/Artifact N+1。
- 视频二进制从对象存储或流式 endpoint 传输，API 不 `ReadAll` 大文件。
- 生成进度以事件 cursor 增量读取，避免每秒重载完整任务聚合。
- 缩略图和 review proxy 异步生成；任务页不直接加载所有原始 4K 文件。
- Provider concurrency 与 Worker concurrency 分开，防止一个慢 Provider 占满 Source/Render 工作槽。
- 保留策略异步执行，并保证被 DeliveryPackage 引用的 Artifact 不被提前回收。

## 9. 发布门禁

以下任一失败都阻断 V7 生产发布：

- `go test ./...`、`go test -race ./...`、`go vet ./...`。
- PostgreSQL migration、RLS 和并发 claim 集成测试。
- Web test、typecheck、build 和关键 E2E。
- Provider contract suite 和 FakeProvider Golden Journey。
- 真实 Provider smoke、预算上限和账单对账。
- S3 流式下载、媒体校验、签名 URL 和 quarantine 测试。
- 空 manifest/无 final Artifact 不可 delivered 的回归测试。
- 金陵古都香候选状态、Evidence、rights 和内容 Gate 验收。
- 生产 Canary 观察队列、Job 错误、对象存储和前端控制台。

## 10. 回滚

- Provider 能力按 tenant feature flag 关闭，新 Job 停止，已有 Job 继续对账或受控取消。
- Web 可回退到 V6 只读任务投影，但不能重新开放空 Delivery 写路径。
- Worker 可独立回滚；数据库 migration 首阶段只 additive。
- Provider Adapter 通过 Profile 撤回，不需要发布 Server 才能停止新提交。
- 回滚不删除 Job、Attempt、Artifact、费用或审计记录。

## 11. 完成定义

V7 只有在以下条件同时满足时才完成：

- 新营销视频任务的每个业务 Stage 都有类型化、可验证、可展示的服务端输出。
- 知识、剧本、分镜、takes、质检和 Delivery 在 Web 中显示真实内容。
- 至少一个真实托管 Provider 在生产候选环境完整生成 MP4，并通过费用和安全验收。
- `delivered` 严格绑定最终 Artifact、通过的 final review 和 DeliveryPackage。
- 历史空交付被如实标记，未删除或伪造。
- 金陵古都香候选资料完成受控导入路径，blocked 状态未被越权提升。
