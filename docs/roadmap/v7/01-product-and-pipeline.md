# V7 产品与端到端生产链

## 1. 版本目标

V7 把营销视频从“脚本生产能力”升级为“成片交付能力”。用户不需要理解 `output_refs`、本地目录或内部 Revision API；他们需要看到每一步产生了什么、为什么被阻断、谁可以批准，以及最终视频在哪里。

V7 的默认内容类型从 `video_script` 演进为 `marketing_video`。`video_script` 保留为兼容输入和独立交付类型，但创建“生成短视频”任务时必须选择 `marketing_video`，避免同一个类型同时表示剧本和成片。

## 2. 用户角色

| 角色 | 责任 | 不允许 |
| --- | --- | --- |
| 内容生产者 | 选择资料、创建候选知识、策略、Brief、剧本和分镜 | 批准自己的正式事实或伪造 Provider 输出 |
| 审核者 | 审核 Evidence、知识、剧本、分镜和成片 | 修改已批准 Revision 的正文 |
| 项目经理 | 选择 Provider、确认预算、重试或取消 Job、准备交付 | 查看其他租户的配置和资产 |
| 客户决定人 | 批准品牌主张、分镜和最终成片 | 绕过 Schema、rights 和内容完整性检查 |
| 租户管理员 | 启用媒体能力、设置预算、并发、保留和披露策略 | 读取 Provider Secret 明文 |
| Media Worker | 提交、轮询、下载和校验 Provider 产物 | 自行批准内容、提升知识状态或发布到外部账号 |

## 3. Golden Journey

```text
[1 输入收集]
  上传或登记来源
        |
        v
[2 Evidence]
  解析 -> 定位 -> 人工复核
        |
        v
[3 Knowledge]
  候选对象 -> 冲突/缺口 -> 批准 -> KnowledgeSnapshot
        |
        v
[4 Strategy + Brief]
  受众策略 -> Brief -> 人工选择方向
        |
        v
[5 Script]
  ContentBatch -> ContentItem -> lint -> review -> ApprovedSnapshot
        |
        v
[6 Storyboard]
  shot plan -> 首尾帧 -> review sheet -> lock
        |
        v
[7 Video Generation]
  cost preflight -> submit -> poll/callback -> takes -> ingest
        |
        v
[8 Media QA]
  技术检查 -> 人工选片 -> 退回局部重生成
        |
        v
[9 Post-production]
  字幕/旁白/品牌/CTA/Offer -> final render -> final QA
        |
        v
[10 Delivery]
  DeliveryPackage -> 下载 -> 外部发布绑定 -> 效果回流
```

## 4. 每阶段的输入、输出和 Gate

| Stage | 必须输入 | 规范输出 | 完成 Gate |
| --- | --- | --- | --- |
| `intake` | 用户选择的文件、链接或 Brief | Source + SourceRevision | 文件已存储、摘要和 MIME 有效 |
| `evidence` | ready SourceRevision | EvidenceSpan[] | 解析完成；低置信 Evidence 明确标记 |
| `knowledge` | accepted Evidence | KnowledgeObject candidate + gap/conflict | 候选已保存；批准仍需人工决定 |
| `strategy` | KnowledgeSnapshot + 业务目标 | AudienceStrategyVersion | 引用均 eligible，无过期动态事实 |
| `brief` | 策略、渠道、交付要求 | Brief SubmissionRevision | Schema、引用和 rights 检查通过 |
| `script` | approved Brief + KnowledgeSnapshot | ContentBatch / ContentItem | 内容审核通过并产生 ApprovedSnapshot |
| `storyboard` | approved ContentItem + approved assets | StoryboardPackage + frame Artifacts | 人工审核并锁定 digest |
| `video_generate` | locked Storyboard + Provider Profile + budget approval | MediaGenerationJob + take Artifacts | 至少一个可播放 take；技术检查通过 |
| `media_review` | take Artifacts | MediaReview + selected take | 人工选择或明确要求局部重生成 |
| `post_production` | selected take + approved overlays/audio/Offer | final video Artifact | 渲染成功，动态权益仍有效 |
| `final_review` | final video + lineage manifest | final MediaReview | 客户决定人批准精确 SHA-256 |
| `delivery` | approved final Artifact | DeliveryPackage | manifest 非空、文件可读、摘要匹配 |

任何 Stage 被标记 completed 前，服务端必须验证输出实体属于同一 tenant/project/task、对象类型符合 Stage Contract、digest 匹配且状态满足 Gate。

## 5. 任务状态语义

```text
needs_input -> ready -> running -> waiting_gate -> ready
                    |          |             |
                    |          v             v
                    |       blocked <----- changes_requested
                    |          |
                    |          v retry
                    +------- running

final_review approved -> ready_to_deliver -> delivering -> delivered
                                              |
                                              v
                                           failed
```

### 禁止的状态跳转

- 无 approved ContentItem 不得创建 Storyboard 正式任务。
- 无 locked Storyboard 不得提交托管视频 Job。
- 无成功 take Artifact 不得完成 `video_generate`。
- 无通过的 final MediaReview 不得进入 `ready_to_deliver`。
- 无 final video Artifact 和 ready DeliveryPackage 不得进入 `delivered`。
- Provider 返回“成功”但下载、MIME、大小或摘要校验失败时，Job 必须进入 `output_invalid`，不能视为完成。

## 6. 人工 Gate

V7 默认提供以下 Gate 模板：

1. `evidence_review`：处理 OCR 低置信、来源矛盾和不可定位片段。
2. `knowledge_approval`：批准候选事实、主张、权利和品牌规则。
3. `direction_selection`：从策略候选中选择一个生产方向。
4. `content_approval`：批准精确 ContentItem Revision。
5. `storyboard_lock`：确认商品真实性、连续性、rights 和 locked digest。
6. `generation_cost`：在预估费用或重试次数超过租户阈值时确认。
7. `take_selection`：在多个生成结果中选择一个，或只重生成失败 segment。
8. `final_approval`：批准最终 MP4 的 SHA-256、字幕、声音、CTA 和 Offer。

Gate 输入必须显示真正的业务内容。只显示对象 ID、数量或 digest 不构成可审查界面。

## 7. 失败恢复

### 局部重试

- Source 解析失败只重跑 SourceRevision。
- 单个知识候选被拒绝不重跑全部 Evidence。
- 单个 shot 失败只创建新的 Storyboard Revision 或 shot generation attempt。
- 单个 segment 生成失败只重试该 segment，不重复提交已成功且锁定的 segment。
- 后期字幕错误只重跑 deterministic render，不重复调用视频 Provider。

### 幂等

每个有副作用的动作都使用稳定 idempotency key：

```text
source ingest:       tenant/project/source-revision/digest
stage output:        task/stage-run/object-type/object-id/digest
provider submit:     generation-job/attempt/input-digest/profile-digest
provider callback:   provider/external-job-id/event-id
final render:        selected-take/overlay-manifest-digest/renderer-digest
delivery package:    final-artifact/approved-review/manifest-digest
```

## 8. 金陵古都香路径

`/Users/coso/Documents/dev/goodvision/marketing/jinling-gudu` 是 V7 的真实候选验收资料，但不是已绑定 Workspace，也不是已批准知识库。V7 验收按以下规则执行：

1. 用户在本地明确选择允许披露的来源文件，不扫描或上传整个目录。
2. 通过 Workspace 初始化或受控导入创建 SourceRevision，保留原始 SHA-256 和文件名。
3. Source Worker 生成 Evidence；知识 Agent 只能创建 candidate/blocked 对象。
4. `wiki/home.md` 和现有 blocked 脚本中的状态保持原样，不因导入而变成 approved。
5. 缺产品实物图、素材权利、标签安全或产品主张证据时，允许完成结构预览，但不得提交真实产品成片 Job。
6. 正式成片必须引用已批准的产品资产、rights、内容和分镜快照。

## 9. 产品验收

- 新用户从任务页能判断当前缺的是资料、批准、预算、Provider、生成结果还是质检，不依赖日志或聊天记录。
- 任务页显示完整剧本文字和镜头，不要求用户手工粘贴 JSON 才能查看。
- 视频生成期间显示队列、提交、生成、下载、校验、失败和重试状态。
- 用户能播放代理视频，选择 take，查看失败 segment，并只重试局部。
- 交付页显示文件名、时长、分辨率、编码、大小、SHA-256、批准人和下载动作。
- 历史空交付明确显示“旧版不完整交付”，不显示成片成功状态。
