# Seedance 2.5 Provider 融合设计

状态：**单镜头生产闭环代码已实现；真实 Provider 凭据、账单和输出域名仍需受控环境验收**。

本文把近期 ModelArk/Seedance 2.5 执行项目的可复用部分收敛到 ContentCloud 现有的媒体运行时。它不是把 Showvi、MCP 服务或完整应用搬进仓库，也不新增一套绕开 ContentCloud 的视频任务状态。

## 1. 结论

ContentCloud 继续拥有以下权威事实：

- 已批准的 `StoryboardSnapshot`、`SeedancePromptPackage` 和锁定摘要；
- `MediaGenerationJob`、`ProviderAttempt`、`ExternalEffect` 和费用审批；
- 服务商状态、未知结果对账、Artifact 下载、MP4 技术校验和内容审核；
- 输入与输出的租户/项目范围、摘要和权利血缘。

Seedance 2.5 只作为 `provider_worker` 使用 ModelArk Data Plane 的执行适配器。插件 Skill 只能调用 ContentCloud 控制面，不能直接调用 ModelArk，也不能把 MCP 工具暴露为第二条生成入口。

## 2. 当前完整闭环

当前实现覆盖单镜头生产纵向切片：

```text
Approved StoryboardSnapshot
  -> MediaGenerationJob
  -> awaiting_cost_approval
  -> Seedance25Provider.Submit
  -> ProviderAttempt / ExternalEffect
  -> poll / process restart recovery
  -> download MP4
  -> Artifact + SHA-256
  -> technical review approved
  -> content review pending
```

已开放的 Provider Profile 模式：

| 模式 | 输入 | 说明 |
| --- | --- | --- |
| `text_to_video` | 已批准单镜头提示词 | 不读取本地路径；提示词来自受控 Artifact 解析器 |
| `image_to_video` | 单镜头提示词和图片引用 | 图片必须转成 `data:image/...;base64` 或受控 HTTPS URL |

尚未核验的 `extend`、视频编辑、首尾帧组合、音频驱动、超长视频和多镜头批量提交仍由 Adapter 明确拒绝。Provider Profile 即使声明这些能力，也不能绕过拒绝；这些能力需要独立的分段数据模型、费用归属、取消和真实接口验收。

第一阶段 Adapter 还有不可被 Profile 放宽的硬上限：单任务时长不超过 30 秒、图片引用不超过 30 张、单镜头提示词不超过 32,000 个 Unicode 字符。Profile 可以声明更严格的时长或图片限制，但不能绕过这些硬上限。

Seedance 2.5 的配置事实固定在 Profile 中，而不是散落在 Skill 或用户提示词里：

以下金额只是字段结构示例，不是服务商报价；生产 Profile 必须填入核验后的值。

```json
{
  "provider_id": "modelark-seedance25",
  "model": "dreamina-seedance-2-5-260628",
  "modes": ["text_to_video", "image_to_video"],
  "limits": {
    "max_duration_seconds": 30,
    "max_reference_images": 30,
    "resolution": "720p"
  },
  "pricing": {
    "currency": "CNY",
    "per_second_minor": 1,
    "per_job_minor": 0
  }
}
```

正式环境不能把空费用当作免费。若 Profile 没有经核验的价格，Adapter 必须返回 `PROVIDER_PRICING_UNAVAILABLE`，任务不能进入队列。

## 3. 请求和输入解析

`MediaGenerationJob` 只保存 Artifact ID，不把绝对路径、长期 URL、密钥或完整提示词写入 Job 事件。`Seedance25Provider` 通过受控 `InputResolver` 获取一次性提交输入：

1. 读取 `PromptPackageArtifactID`，校验 `contentcloud.seedance-prompt-package/1.0`、批准快照 ID、Profile 版本、锁定摘要和单片段限制。
2. 按 `UploadManifest` 的顺序解析 Artifact，并重新核对租户、项目、媒体类型、大小和 SHA-256。
3. 图片使用短期 data URL 或受控 HTTPS URL；视频/音频在第一阶段直接拒绝，禁止把本地绝对路径传给 ModelArk。
4. 仅将解析后的 prompt、引用角色和固定设置发送到 `POST /contents/generations/tasks`。

输入解析失败是本地确定性错误，不能重试提交。解析器不能从提示词正文中读取权限、URL、费用或工具配置。

## 4. Provider API 映射

| Adapter | ModelArk | ContentCloud 语义 |
| --- | --- | --- |
| `Submit` | `POST /contents/generations/tasks`，Bearer，`Idempotency-Key` | 创建一次外部 Effect；超时/5xx 为 unknown，不自动重新提交 |
| `Status` | `GET /contents/generations/tasks/{id}` | `queued/running` 保持等待；`succeeded` 才允许下载 |
| `Cancel` | `DELETE /contents/generations/tasks/{id}` | 先请求外部取消，成功后才能把本地 Job 标为 `cancelled` |
| `Download` | 任务返回的短期 `content.video_url` | 经过域名白名单、MIME、大小、MP4 box 和 SHA-256 校验后落 Artifact |

Provider 返回未知或不完整响应时使用 `PROVIDER_STATUS_UNKNOWN`、`PROVIDER_SUBMIT_UNKNOWN` 或 `PROVIDER_CANCEL_UNKNOWN`。未知状态必须进入对账，不能伪造失败、取消或成功。

## 5. 费用和取消

创建 Job 时调用 Adapter `Estimate`，优先使用 `per_job_minor + per_second_minor * duration` 的保守估算，再检查租户 Binding 的 `MaxJobCostMinor`。估算费用大于零时保持 `awaiting_cost_approval` 门禁；没有价格事实时直接阻断。

取消规则：

- 尚未产生外部任务 ID：可以直接取消本地 Job；
- 已有外部任务 ID：必须调用 Provider `Cancel`，外部取消成功后再保存本地 `cancelled`；
- Provider 取消返回超时、5xx 或结果不明：进入 `awaiting_external_result` 对账状态，记录 `CancelRequestedAt` 和 `PROVIDER_CANCEL_UNKNOWN`，等待 Status/账单对账；
- 任何路径都不能通过本地状态覆盖外部操作事实。

## 6. 插件边界

`contentcloud-video-production` 继续是唯一插件包，新增 `contentcloud-seedance-execution` Skill 只负责：绑定工作区、检查已批准快照、创建 Media Job、展示费用审批和查看审核结果。它不新增 `modelark-mcp` 的 `mcp.json` 注册，不允许智能体绕过 Media Job、Effect、费用或审核。

用户仍然可以使用 `contentcloud-seedance-export` 生成手动上传包；手动上传和服务端执行是两个独立入口，下载结果必须重新导入为候选 Artifact，不能直接成为最终成片。

Worker 只有在显式设置 `CONTENTCLOUD_SEEDANCE25_API_KEY` 时才注册 Provider；同时必须设置 `CONTENTCLOUD_SEEDANCE25_ALLOWED_HOSTS`，列出 ModelArk API 和结果下载域名。`CONTENTCLOUD_SEEDANCE25_BASE_URL`、`CONTENTCLOUD_SEEDANCE25_MODEL` 和 `CONTENTCLOUD_SEEDANCE25_RESOLUTION` 只能改变已审核部署配置，不能由 Skill 或提示词传入。

## 7. 后续多镜头计划

多镜头不是把多个 prompt 串在一个外部任务里。后续会为 `MediaGenerationJob` 增加 `SegmentID`、`SegmentOrder` 和分段输入摘要，每个分段独立拥有 Attempt、费用、取消、Artifact 和审核结果；汇聚和剪辑仍由 ContentCloud 后期节点负责。只有在单镜头重启恢复、取消、账单对账和真实输出验收通过后，才开放该阶段。

## 8. 验收门槛

- Submit 请求包含正确模型、设置、Bearer 和幂等键，且请求体不含本地路径或凭据；
- 5xx/超时不会重复提交，重启后只凭持久化外部任务 ID 轮询；
- queued/running/succeeded/failed/cancelled/expired 状态映射稳定；
- 取消调用真实 Provider，未知取消不伪造本地终态；
- 输出下载通过域名、MIME、大小、MP4 容器和摘要校验；
- 费用估算、`awaiting_cost_approval`、Runtime Effect 和 Artifact 血缘均有回归测试；
- 只有单镜头 Profile 能力被执行，未验证能力返回明确错误。

### HTTP 入口

ContentCloud BFF 暴露两个与第一阶段执行直接相关的入口，均要求登录会话和当前任务权限：

| Method | Path | 说明 |
| --- | --- | --- |
| `POST` | `/api/bff/tasks/{taskID}/seedance-prompt-package` | `multipart/form-data` 上传已批准分镜快照绑定的 JSON PromptPackage，表单字段为 `snapshot_id` 和 `file`；成功后返回 `prompt_package` Artifact。 |
| `POST` | `/api/bff/media-jobs/{id}/reconcile-submit` | 使用 `expected_version` 和人工确认的 `external_job_id` 补录未知提交；只允许 `awaiting_external_result`，不重新提交、不覆盖已有外部 ID。 |

Media Job 创建接口仍是 `/api/bff/tasks/{taskID}/media-jobs`，请求中的 `prompt_package_artifact_id` 必须引用上一步返回的 Artifact。所有写接口使用统一错误 Envelope；未知提交、状态或取消结果必须保留对账状态和错误码。

### Provider Profile 和 Binding 初始化 API

Provider 配置不再要求直接写数据库。以下接口均挂在需要登录的 BFF 下，并由 Service 做第二层权限和状态校验：

| Method | Path | 权限 | 说明 |
| --- | --- | --- | --- |
| `GET` | `/api/bff/admin/provider-profiles?provider_id=...` | 平台管理员 | 查看 Profile 管理列表；响应不含凭据字段。 |
| `GET` | `/api/bff/provider-profiles?provider_id=...` | 已登录租户 | 查看当前可绑定的已发布、未过期 Profile；draft 和过期版本不会返回。 |
| `POST` | `/api/bff/admin/provider-profiles` | 平台管理员 | 创建 `draft` Profile。请求只包含能力、限制、费用和核验时间，不接受 API Key。 |
| `POST` | `/api/bff/admin/provider-profiles/{providerID}/{version}/publish` | 平台管理员 | 发布已核验且未过期的 Profile；发布操作幂等。 |
| `GET` | `/api/bff/provider-bindings/{providerID}` | 当前租户成员（只读） | 查看当前租户 Binding；`credential_ref` 永远不会序列化，只返回 `credential_configured` 布尔状态。 |
| `PUT` | `/api/bff/provider-bindings/{providerID}` | 当前租户管理员 | 配置当前租户 Binding。启用非 fake Provider 时，凭据只能是 `secret://`、`vault://` 或 `env://` 引用。 |
| `PUT` | `/api/bff/admin/tenants/{tenantID}/provider-bindings/{providerID}` | 平台管理员 | 代租户配置 Binding；同样要求 Profile 版本精确匹配且处于 published、未过期状态。 |

Binding 的读取响应会保留状态、Profile 版本、预算和出口策略，但只返回 `credential_configured`，不会返回凭据引用。租户工作台和平台后台都消费这一安全状态；Binding 的写入仍只允许租户管理员或平台管理员。

Profile 创建示例（时间字段使用 RFC3339）：

```json
{
  "provider_id": "modelark-seedance25",
  "version": "1.0.0",
  "digest": "sha256:<64 个小写十六进制字符>",
  "adapter_version": "modelark/1.0.0",
  "model": "dreamina-seedance-2-5-260628",
  "region": "cn-beijing",
  "modes": ["text_to_video", "image_to_video"],
  "input_media_types": ["image/png", "application/json"],
  "output_media_type": "video/mp4",
  "limits": {"max_duration_seconds": 30, "max_reference_images": 30},
  "data_retention": "provider_policy",
  "pricing": {"currency": "CNY", "per_second_minor": 1},
  "verified_at": "2026-08-15T00:00:00Z",
  "expires_at": "2026-09-15T00:00:00Z"
}
```

创建成功仍是 `draft`，必须再调用 publish。Binding 请求示例：

```json
{
  "profile_version": "1.0.0",
  "state": "active",
  "credential_ref": "secret://providers/modelark-seedance25",
  "egress_policy": "provider-only",
  "monthly_budget_minor": 100000,
  "max_job_cost_minor": 3000,
  "max_concurrency": 2,
  "max_retries": 2
}
```

系统只保存 `credential_ref`，不会接收或写入明文 API Key；Profile 未发布、已过期、版本不一致或 active Binding 缺少凭据引用时都会拒绝配置。部署前仍必须在沙箱凭据下完成一次提交、轮询、取消、下载和重启恢复演练，并核验费用和输出域名。
