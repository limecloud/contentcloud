# V7 领域模型与状态机

## 1. 建模原则

1. PostgreSQL 是在线事务事实源；对象存储是二进制事实源；本地 Workspace 是候选创作事实源。
2. WorkTask 只负责编排，业务正文必须存在规范领域对象中。
3. 每个正式对象都绑定 tenant、project、schema/version、digest 和创建主体。
4. 所有跨对象引用都由服务端校验存在性、租户、项目、状态和摘要。
5. 可变运行状态与不可变业务 Revision 分离；重试不覆盖已经产生的事实。
6. 费用、Provider 状态和生成结果不能由客户端自报为成功。

## 2. 规范对象关系

```text
WorkTask 1 ----- n StageRun
   |                 |
   |                 +----- n TaskStageOutput
   |                                |
   |                                +-- SourceRevision / EvidenceSpan
   |                                +-- KnowledgeObject / Snapshot
   |                                +-- SubmissionRevision / ApprovedSnapshot
   |                                +-- StoryboardPackage / Artifact
   |                                +-- MediaGenerationJob / MediaReview
   |                                +-- DeliveryPackage
   |
   +----- n TaskRun ----- n RunAttempt          (local/agent execution)
   |
   +----- n MediaGenerationJob ----- n ProviderAttempt
                                      |
                                      +----- n Artifact (generated takes)
```

`TaskStageOutput` 是编排层到业务层的唯一桥梁，不保存业务正文。

## 3. TaskStageOutput

### 最小字段

| 字段 | 规则 |
| --- | --- |
| `id` | UUIDv7，服务端生成 |
| `tenant_id` / `project_id` / `task_id` | 从 Task 和鉴权上下文派生 |
| `stage_run_id` / `stage_id` | 必须指向当前活动 StageRun |
| `output_type` | `source_revision`、`evidence_set`、`knowledge_object`、`knowledge_snapshot`、`submission_revision`、`approved_snapshot`、`storyboard_package`、`artifact`、`generation_job`、`media_review`、`delivery_package` |
| `object_id` | 规范对象稳定 ID |
| `object_version` | 对象支持版本时必填 |
| `object_digest` | 对象规范摘要，禁止空值 |
| `role` | `primary`、`supporting`、`preview`、`selected_take`、`final` |
| `status` | `candidate`、`validated`、`approved`、`blocked`、`failed` |
| `metadata` | 只保存投影所需的小型结构化数据，不保存正文或秘密 |
| `created_by` / `created_at` | 用户、设备或 Worker 身份 |

### 上报规则

Stage completion API 不再接受任意业务 `output_refs`。新请求形态：

```json
{
  "stage_run_id": "...",
  "status": "completed",
  "outputs": [
    {
      "output_type": "approved_snapshot",
      "object_id": "...",
      "object_digest": "sha256:...",
      "role": "primary"
    }
  ],
  "checks": {
    "content.schema": {"status": "passed", "evidence": ["..."]},
    "claim.references": {"status": "passed", "evidence": ["..."]},
    "rights.references": {"status": "passed", "evidence": ["..."]}
  }
}
```

服务端根据 Stage Contract 解析并验证 outputs。前端不得再固定发送 `checks: {passed: true}`；检查结果来自确定性 validator、Worker 或 Gate 决定。

## 4. Stage Contract

SOP Stage 增加以下版本化约束：

| 字段 | 用途 |
| --- | --- |
| `accepted_input_types` | 当前 Stage 可消费的规范对象类型和最低状态 |
| `required_output_types` | 完成时必须存在的输出类型、数量和角色 |
| `output_schema_refs` | 对输出正文或 manifest 的 Schema 约束 |
| `completion_policy` | `all_required`、`at_least_one`、`control_only` |
| `executor_policy` | `local_agent`、`automation_daemon`、`server_worker`、`managed_provider`、`human` |
| `retry_policy` | 最大尝试、退避、局部重试和可重试错误码 |
| `cost_policy` | Provider Stage 的预算、超额 Gate 和估算有效期 |

发布 SOP 时检查 Stage 依赖图无环、输入能由上游输出满足、最终 Stage 必须产生 `delivery_package:final`。

## 5. MediaGenerationJob

### 字段

| 字段 | 说明 |
| --- | --- |
| `id` / tenant / project / task / stage_run | 归属和血缘 |
| `storyboard_snapshot_id` | 锁定的 storyboard ApprovedSnapshot |
| `prompt_package_artifact_id` | Provider 派生提示包或请求 manifest |
| `provider_id` / `profile_version` / `profile_digest` | 精确 Provider 能力版本 |
| `model` / `mode` / `aspect_ratio` / `duration_seconds` | 经 profile 校验的生成参数 |
| `input_artifact_refs` | 已批准图片、视频和音频 Artifact |
| `state` | 见状态机 |
| `idempotency_key` | 同一输入只允许一个活动 Job |
| `estimated_cost` / `actual_cost` / `currency` | 费用事实，不使用浮点存货币 |
| `attempt_count` / `max_attempts` | 重试边界 |
| `cancel_requested_at` | 协作式取消 |
| `error_code` / `error_detail_safe` | 可读且不泄密的失败信息 |
| `created_by` / timestamps | 审计 |

### Job 状态机

```text
draft
  |
  v
awaiting_cost_approval --> cancelled
  |
  v
queued --> submitting --> submitted --> generating
  |           |              |              |
  |           v              v              v
  |        retry_wait <--- retryable_failed -+
  |                                         |
  |                                         v
  +------------------------------------- downloading
                                            |
                                            v
                                         validating
                                         /        \
                                        v          v
                                  succeeded   output_invalid
                                                      |
                                                      v
                                                   retry_wait

terminal: succeeded / failed / cancelled / budget_blocked
```

状态只能由领域服务转移。Provider callback、轮询和操作员动作都携带 expected state/version；重复或乱序事件写审计后幂等忽略。

## 6. ProviderAttempt

ProviderAttempt 保存一次真实外部提交：

- `generation_job_id`、attempt number、request digest。
- `external_job_id`、provider state、last polled at、next poll at。
- request/response 安全摘要；原始响应仅在受限诊断存储按保留策略保存。
- submit/poll/download timestamps、HTTP status、retry-after。
- Provider 费用、request id 和 rate-limit 元数据。
- 输入和输出披露清单，确保审计能回答“哪些客户资料发给了哪个 Provider”。

同一个 Job 只有一个活动 Attempt。提交超时且不知道 Provider 是否已接收时，先通过 idempotency key 或查询接口对账，禁止直接重提。

## 7. Artifact 扩展

复用现有 Artifact，增加或规范以下媒体元数据：

```json
{
  "kind": "generated_video_take",
  "media_type": "video/mp4",
  "sha256": "sha256:...",
  "byte_size": 12345678,
  "metadata": {
    "duration_ms": 15000,
    "width": 1080,
    "height": 1920,
    "video_codec": "h264",
    "audio_codec": "aac",
    "frame_rate": "30/1",
    "generation_job_id": "...",
    "provider_attempt_id": "...",
    "segment_id": "segment-01"
  }
}
```

V7 Artifact kind 至少包括：

- `storyboard_frame`
- `storyboard_review_sheet`
- `provider_request_manifest`
- `generated_video_take`
- `generated_audio_take`
- `review_proxy_video`
- `caption_file`
- `final_video`
- `delivery_manifest`

对象存储 key 不进入 API；下载通过短期签名 URL 或同源授权流式响应。

## 8. MediaReview

| 字段 | 说明 |
| --- | --- |
| `subject_artifact_id` | 被审核的 take 或 final video |
| `review_kind` | `technical`、`content`、`final` |
| `status` | `pending`、`approved`、`changes_requested`、`rejected` |
| `checks` | 时长、画幅、黑帧、静音、连续性、商品真实性、字幕、rights 等结构化结果 |
| `selected` | take 选择只允许一个 active selected |
| `decision_reason` | 明确修改范围或拒绝原因 |
| `decided_by` / `decided_at` | 人工决定 |
| `subject_digest` | 审核绑定精确 Artifact 摘要 |

技术检查可以由 Worker 产生，但“商品是否真实”“品牌是否可接受”“最终是否交付”必须由人工决定。

## 9. Delivery 收敛

V7 新写路径使用现有 `DeliveryPackage`：

```text
Approved final MediaReview
        +
final video Artifact
        +
lineage/delivery manifest Artifact
        |
        v
DeliveryPackage(status=ready)
        |
        v
WorkTask(status=delivered) projection
```

TaskDelivery 只作为兼容投影，至少新增 `delivery_package_id` 和 `integrity_status`；新路径不允许通过传入字符串 manifest 创建 delivered 事实。

## 10. API 投影

`GET /api/bff/tasks/{id}` 返回面向页面的一次性投影：

- task、SOP、StageRun、Gate 和 allowed actions。
- 每个 Stage 的类型化 outputs、状态、摘要和预览信息。
- 最新/历史 Submission Revision 的安全渲染内容和 diff 元数据。
- Storyboard shots、frame Artifact 和 locked digest。
- MediaGenerationJob、attempt、费用、进度和 take。
- MediaReview 和 selected take。
- DeliveryPackage manifest 和可请求下载的 Artifact ID。

大正文和媒体二进制不内联。正文按类型分页或按 Revision 加载；视频使用 proxy/stream endpoint。

## 11. 数据库约束

- 所有新表启用 tenant RLS，应用 runtime role 不得绕过。
- `TaskStageOutput` 对 `(stage_run_id, output_type, object_id, object_digest, role)` 建唯一索引。
- `MediaGenerationJob` 对活动 `idempotency_key` 建部分唯一索引。
- ProviderAttempt 对 `(provider_id, external_job_id)` 建唯一索引。
- MediaReview 对 active selected take 建部分唯一索引。
- 所有状态更新使用 row version 或 `WHERE state = expected_state`。
- Job claim 使用 `FOR UPDATE SKIP LOCKED`，并有 lease expiry 恢复。

## 12. 兼容策略

```text
V6 read/write                         V7 read/write
TaskRevision ----------------------> SubmissionRevision projection
TaskDelivery ----------------------> DeliveryPackage projection
StageRun.output_refs --------------> TaskStageOutput
       |                                  |
       +---- legacy read only ------------+
```

V7 新任务只写右侧规范对象。旧 API 在兼容期内仍可读取历史记录，但创建空 TaskDelivery 的路径被拒绝；旧客户端收到稳定错误码和升级提示。
