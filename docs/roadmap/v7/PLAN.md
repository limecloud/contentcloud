# ContentCloud V7 实施台账

状态：`方案草案完成，待 M7-0 评审`。

更新时间：2026-08-03。

本台账把 V7 拆成可验证工作包。实现顺序先收敛事实源，再引入付费 Provider；不能先做生成按钮，再补数据、费用和交付门禁。

## 1. 工作包

| ID | 工作包 | 主要产物 | 依赖 | 状态 |
| --- | --- | --- | --- | --- |
| W7-00 | 方案与契约冻结 | V7 文档、状态机、Stage Contract、错误码 | - | 草案完成 |
| W7-01 | 类型化 Stage Output | domain、migration、store、service、API | W7-00 | 待实施 |
| W7-02 | 任务规范对象投影 | Submission/Knowledge/Storyboard/Artifact 聚合读模型 | W7-01 | 待实施 |
| W7-03 | 知识与内容工作面 | Source/Evidence/Knowledge/Script 正文和审核 UI | W7-02 | 待实施 |
| W7-04 | Storyboard 服务端闭环 | frame Artifact、review sheet、lock 和任务投影 | W7-01、W7-02 | 待实施 |
| W7-05 | Media Job 领域 | MediaGenerationJob、ProviderAttempt、MediaReview | W7-00 | 待实施 |
| W7-06 | Media Worker 与 FakeProvider | claim、submit、poll、callback、download、validation | W7-05 | 待实施 |
| W7-07 | 真实 Provider Adapter | Profile、SecretRef、预算、一个真实 Vendor | W7-06 | 待实施 |
| W7-08 | 视频生成工作面 | Job 进度、费用、取消、重试、takes 和播放器 | W7-02、W7-05 | 待实施 |
| W7-09 | 质检与后期 | technical QA、take 选择、deterministic render、final review | W7-04、W7-06 | 待实施 |
| W7-10 | Delivery 收敛 | final Artifact、DeliveryPackage、下载和 delivered hard gate | W7-09 | 待实施 |
| W7-11 | 历史兼容与分类 | legacy integrity backfill、dual-read、旧写路径阻断 | W7-02、W7-10 | 待实施 |
| W7-12 | 金陵古都香验收 | 受控来源导入、candidate/blocked、完整 Golden Journey | W7-03、W7-10 | 待实施 |
| W7-13 | 运维与发布 | 指标、告警、Canary、runbook、回滚和账单对账 | W7-07、W7-10 | 待实施 |

## 2. 依赖图

```text
W7-00
  |-- W7-01 --> W7-02 --> W7-03 -----------------+
  |      |          |                              |
  |      `--> W7-04 --------------------+          |
  |                                     |          |
  `--> W7-05 --> W7-06 --> W7-07        |          |
                 |                      |          |
                 +--> W7-08 <-----------+          |
                         |                          |
                         v                          |
                       W7-09 --> W7-10 --> W7-11   |
                                      |            |
                                      +--> W7-12 <-+
                                      |
                                      `--> W7-13
```

## 3. 并行实施车道

| 车道 | 顺序 | 模块 | 冲突说明 |
| --- | --- | --- | --- |
| A 事实源 | W7-01 -> W7-02 -> W7-11 | domain/store/app/httpapi | 与 B 共享 domain，W7-05 合并前先冻结接口 |
| B 媒体 Runtime | W7-05 -> W7-06 -> W7-07 | domain/store/worker/provider | W7-05 完成后可与 A 的 UI 投影并行 |
| C 内容工作面 | W7-03 -> W7-04 | web/knowledge/localworkspace | 依赖 A 读模型，可先用 fixture 开发 |
| D 视频工作面 | W7-08 -> W7-09 -> W7-10 | web/media/app/render | 依赖 A+B，内部顺序执行 |
| E 验收运维 | W7-12 + W7-13 | tests/docs/deploy/observability | W7-10 后并行 |

执行顺序：先合并 W7-01 和 W7-05 的领域契约；随后 A、B、C 分工作区并行；A+B 稳定后启动 D；最后 E 并行完成业务与生产验收。

## 4. Implementation Tasks

- [ ] **T1 (P1, human: ~4d / CC: ~4h)** - 编排事实源 - 新增 TaskStageOutput 并校验规范对象引用
  - 来源：架构评审，裸 `output_refs` 无法证明业务产物存在。
  - 范围：domain、store/memory、store/postgres、migrations、app、httpapi。
  - 验证：domain/store/service/HTTP tests + RLS integration。
- [ ] **T2 (P1, human: ~5d / CC: ~5h)** - 任务读模型 - 投影知识、Submission、Storyboard、Artifacts 和 Delivery
  - 来源：工作台当前只显示状态和数量。
  - 范围：app projection、BFF、web types/query components。
  - 验证：task detail integration + Web rendering/E2E。
- [ ] **T3 (P1, human: ~6d / CC: ~6h)** - 媒体领域 - 实现 MediaGenerationJob、ProviderAttempt 和 MediaReview 状态机
  - 来源：当前不存在视频生成 Job 和质检事实。
  - 范围：domain、store、migrations、app。
  - 验证：全状态转移、幂等、预算、并发和 race tests。
- [ ] **T4 (P1, human: ~7d / CC: ~8h)** - Media Worker - 实现 Provider contract、FakeProvider、下载校验和 Artifact 入库
  - 来源：V7 需要真实异步生成和失败恢复。
  - 范围：worker、provider adapter、blob/media validation、cmd worker。
  - 验证：FakeProvider integration、SSRF、损坏媒体、worker crash recovery。
- [ ] **T5 (P1, human: ~4d / CC: ~4h)** - Delivery hard gate - 禁止无最终视频的 delivered
  - 来源：`CreateTaskDelivery` 当前允许空 manifest 直接交付。
  - 范围：app delivery、domain、store、httpapi、compat projection。
  - 验证：CRITICAL regression + legacy classification migration tests。
- [ ] **T6 (P2, human: ~6d / CC: ~7h)** - 内容工作面 - 展示知识、剧本正文、diff、分镜和检查结果
  - 来源：服务端有内容但用户看不到。
  - 范围：web workspace、BFF read endpoints、safe renderers。
  - 验证：unit、interaction、responsive 和 E2E。
- [ ] **T7 (P1, human: ~5d / CC: ~6h)** - 视频工作面 - 展示费用、进度、takes、质检、后期和下载
  - 来源：完整交付必须可操作且可恢复。
  - 范围：web media pages、SSE、allowed actions、stream endpoints。
  - 验证：E2E、双击/断线/两标签页/会话过期测试。
- [ ] **T8 (P1, human: ~5d / CC: ~5h)** - 真实 Provider - 接入一个 allowlisted managed HTTP video adapter
  - 来源：FakeProvider 不能满足生产成片交付。
  - 范围：provider adapter、Profile、SecretRef、deploy config、runbook。
  - 验证：staging smoke、预算、取消、账单和数据披露验收。
- [ ] **T9 (P1, human: ~4d / CC: ~4h)** - Golden Journey - 验收金陵古都香 candidate 到成片的受控路径
  - 来源：需要真实业务资料证明 Gate 和内容展示有效。
  - 范围：fixtures/import plan/E2E/docs，不批量上传目录。
  - 验证：candidate/blocked 不越权、批准输入可生成并交付真实 MP4。

## 5. 每里程碑退出条件

| 里程碑 | 必须通过 |
| --- | --- |
| M7-0 | 文档评审、Provider 边界、迁移和安全签字 |
| M7-1 | 所有新 Stage 输出均为类型化引用，裸字符串只读兼容 |
| M7-2 | 空知识库有导入路径；任务页显示知识和剧本正文 |
| M7-3 | 分镜媒体服务端可预览、可审核并锁定 |
| M7-4 | FakeProvider 全状态 + 一个真实 Provider staging 闭环 |
| M7-5 | take 可选、后期可重跑、final digest 可批准 |
| M7-6 | 无 final video Artifact 无法 delivered；历史记录正确分类 |
| M7-7 | 金陵路径、真实租户、性能、安全、Canary 和回滚通过 |

## 6. 当前基线与回归门禁

开始实现前记录当前 `go test ./...`、`go test -race ./...`、`go vet ./...`、Web test/typecheck/build 和 migration 集成结果。每个工作包只增加测试，不允许通过删除或弱化现有 Gate 获得绿色状态。

CI 不调用真实付费 Provider；Provider smoke 只在显式发布候选流程、低预算测试租户和人工确认后执行。

## 7. 风险台账

| 风险 | 级别 | 缓解 |
| --- | --- | --- |
| 两套 Task/Submission 模型继续并行 | P1 | 新写路径只写规范对象，Task 只做投影 |
| 外部 Job 重复提交和重复扣费 | P1 | idempotency、唯一索引、未知结果先对账 |
| Provider 凭据或客户资料泄露 | P1 | SecretRef、allowlist、日志脱敏、披露审计 |
| 视频下载导致 API/Worker OOM | P1 | 流式传输、大小/时长限制、独立媒体 Worker |
| 自动化越过事实和 rights Gate | P1 | ApprovedSnapshot 和 rights 服务端复验 |
| 真实 Provider 能力漂移 | P2 | versioned Profile、expiry、撤回和 contract smoke |
| 历史 delivered 语义被改写 | P2 | 原状态保留，新增 integrity classification |
| 工作台一次加载过多媒体 | P2 | 分页、proxy、lazy load、SSE 增量事件 |

## 8. NOT in scope

- 自动发布到抖音或其他账号。
- 通用服务端 LLM 平台。
- 浏览器内完整视频剪辑器。
- 首版同时维护多个真实付费 Provider。
- 删除 V6 表、历史 Revision 或 Audit。

## 9. 评审结论

V7 应按完整范围实施。缩小为“只把 Revision JSON 显示出来”能缓解页面空白，但不会解决知识未入库、视频未生成和空交付三个根因；继续扩展 TaskRevision/TaskDelivery 也会放大既有双事实源问题。推荐复用规范 Submission、Artifact 和 DeliveryPackage，并只新增编排桥梁和媒体生成领域对象。

## GSTACK REVIEW REPORT

| Review | Trigger | Why | Runs | Status | Findings |
| --- | --- | --- | --- | --- | --- |
| CEO Review | `/plan-ceo-review` | Scope & strategy | 0 | not run | V7 改变产品边界，实施前建议单独评审 |
| Codex Review | `/codex review` | Independent 2nd opinion | 0 | not run | - |
| Eng Review | `/plan-eng-review` | Architecture & tests (required) | 1 | clear | 5 个根因已纳入方案，0 个静默关键缺口 |
| Design Review | `/plan-design-review` | UI/UX gaps | 0 | not run | 实施工作台前需要按 DESIGN.md 评审 |
| DX Review | `/plan-devex-review` | Developer experience gaps | 0 | not run | - |

**VERDICT:** ENG CLEARED FOR PLANNING - 实施前仍需冻结真实 Provider 与产品/设计评审。

**UNRESOLVED DECISIONS:**

- M7-0 必须确认首个真实托管视频 Provider 的 Vendor、区域、模型、计费和数据保留配置。
