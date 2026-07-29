# V5 领域模型与契约

## 1. 建模原则

V5 采用“规范内容与厂商交付分离”的最小演进：

- `ContentItem` 描述要表达什么，不包含厂商编号和临时上传顺序。
- `StoryboardPackage` 描述经过审核的视觉实现与可使用媒体。
- `SeedancePromptPackage` 把锁定分镜投影成 Seedance 的操作说明和提示词。
- `PublishedCreativeBinding` 记录实际发布了哪条成片以及投给哪个实验臂。
- 已存在的 ApprovedSnapshot、Artifact 和 DeliveryPackage 继续承担不可变快照与交付清单职责。

```text
AudienceTaxonomySnapshot
            |
            v
AudienceStrategyVersion ---- CommerceOfferSnapshot (optional)
            |                         |
            v                         v
          Brief -> ContentBatch -> ContentItem -> ApprovedSnapshot
                                                |
                                                v
                                       StoryboardPackage
                                                |
                                                v
                                      SeedancePromptPackage
                                                |
                                                v
                                        DeliveryPackage
                                                |
                                                v
                                  PublishedCreativeBinding
                                                |
                                                v
                                    PerformanceObservation
```

箭头表示引用和派生，不表示拥有关系。任何派生对象都不能修改上游不可变快照。

## 2. 对象定义

### 2.1 AudienceTaxonomySnapshot

记录某个时点采纳的平台人群分类，不持有项目创意。

| 字段 | 说明 |
| --- | --- |
| `id` | 内部稳定 ID |
| `provider` | 例如 `oceanengine_yuntu` |
| `taxonomy_id` | 平台分类标识 |
| `taxonomy_version` | 平台版本；没有公开版本时使用捕获日期并声明来源 |
| `segments[]` | `code`、`label`、平台原始定义摘要 |
| `source_url` | 来源页面 |
| `captured_at` | 采集时间 |
| `effective_from` / `expires_at` | 本地使用窗口 |
| `verification_status` | `unverified`、`human_verified`、`expired` |
| `source_sha256` | 允许保存内容时记录来源摘要；不能保存时记录标准化元数据摘要 |

这是参考数据，不应被修改为项目自定义人群。自定义策略进入 AudienceStrategyVersion。

### 2.2 AudienceStrategyVersion

人群策略是版本化、可审批的项目对象。

```json
{
  "id": "asv_...",
  "project_id": "project_...",
  "taxonomy_snapshot_id": "ats_...",
  "audience_code": "refined_mothers",
  "audience_label": "精致妈妈",
  "segment_definition": "当前项目希望验证的需求状态和场景",
  "objective": "conversion",
  "demand_moment": "工作日晚间准备次日早餐",
  "insight_statement": "有证据与假设边界的洞察",
  "hook_hypotheses": [],
  "proof_order": [],
  "objections": [],
  "cta_strategy": "",
  "evidence_refs": [],
  "confidence": "medium",
  "status": "candidate",
  "based_on_version_id": "",
  "content_hash": "sha256:..."
}
```

状态：`candidate -> review_ready -> approved -> deprecated`。批准后内容不可改；修订必须产生新 ID 和 `based_on_version_id`。

### 2.3 CommerceOfferSnapshot

可选对象，用于高时效商品权益。首版只在价格、优惠、赠品或活动会进入交付时要求创建。

| 字段组 | 必需内容 |
| --- | --- |
| 商品 | `sku_id`、`product_version_id`、商品事实与 approved claim 引用 |
| 权益 | 展示价格、券、赠品、活动范围、资格与互斥条件 |
| 时间 | `captured_at`、`valid_from`、`valid_until` |
| 证据 | 后台截图/接口记录等 Evidence 引用与摘要 |
| 决定 | `candidate`、`verified`、`expired`、`revoked` |

OfferSnapshot 不应保存账户秘密或不可披露的后台原件。V5 默认只把已批准摘要和证据引用发布到服务端。

### 2.4 StoryboardPackage

StoryboardPackage 是一个 ApprovedSnapshot 的视觉生产派生包，不替代 ContentItem。

| 字段 | 说明 |
| --- | --- |
| `id` / `project_id` | 标识与租户边界 |
| `approved_snapshot_id` | 规范剧本的不可变输入 |
| `content_item_id` | 目标 ContentItem |
| `generator_capability` | 图片生成能力 ID、版本和 digest |
| `status` | 本地 manifest 只允许 `candidate`、`review_ready`、`superseded`；正式 lock 由服务端 storyboard ApprovedSnapshot 表示 |
| `shots[]` | 每个 shot 的时间、角色、首帧、尾帧、运动和连续性 |
| `review_sheet_artifact_id` | 仅供审核的分镜接触图 |
| `asset_artifact_ids[]` | 可作为模型输入的独立图片和参考素材 |
| `rights_refs[]` | 素材与生成物权利引用 |
| `source_digest` | 上游剧本及资产清单摘要 |
| `locked_digest` | 锁定时对全部模型输入文件和 manifest 求摘要 |

每个 `shots[]` 元素至少包含：

- `shot_id`、`start_ms`、`end_ms` 和叙事作用。
- `first_frame_artifact_id`、可选 `end_frame_artifact_id`。
- 图片生成提示词、生成种子或可得的复现参数、模型版本。
- 主体、商品、场景、构图、光线、镜头和动作。
- `incoming_state`、`outgoing_state`、运动轴、光线锁、商品锁。
- asset、rights、knowledge、claim 引用和禁止项。
- 镜头级验收条件；人工审核结论、评语和锁定者保存在服务端 ReviewCycle、Decision 和 ApprovedSnapshot，不回写本地 candidate。

### 2.5 SeedancePromptPackage

SeedancePromptPackage 是厂商适配投影，可以丢弃并从锁定分镜重建。

| 字段 | 说明 |
| --- | --- |
| `id` | 提示词包 ID |
| `storyboard_snapshot_id` / `storyboard_package_id` / `storyboard_locked_digest` | 服务端锁定快照、唯一输入对象和摘要 |
| `provider` | `seedance` |
| `provider_profile_version` | 经验证的能力快照版本，不直接写“永远是 Seedance 2.0” |
| `adapter_capability` | ContentCloud 适配 Skill 的 ID、版本和 digest |
| `upstream_reference` | 上游仓库、固定 commit、许可证/授权记录 |
| `mode` | `first_last_frame`、`all_reference`、`extend` 等经验证模式 |
| `settings` | 比例、每段时长、分辨率或平台可用设置 |
| `upload_manifest[]` | 本地 Artifact 到 `@图片N/@视频N/@音频N` 的映射 |
| `segments[]` | 顺序、时间范围、复制文本、衔接点、输入/输出状态 |
| `post_production_plan` | 字幕、价格、优惠、LOGO、CTA 和免责声明合成 |
| `validation` | 引用、时长、素材上限、权利、Offer 和摘要校验 |
| `status` | `draft`、`validated`、`exported`、`stale`、`superseded` |

当 locked digest、Offer、权利状态或 provider profile 发生不兼容变化时，旧包标记 `stale`，但不覆盖或删除历史文件。

### 2.6 PublishedCreativeBinding

发布绑定把“规范内容”收敛到“实际投放的二进制成片”：

```json
{
  "id": "pcb_...",
  "project_id": "project_...",
  "delivery_package_id": "dp_...",
  "rendered_creative_artifact_id": "artifact_...",
  "platform": "douyin",
  "account_alias": "brand-main",
  "platform_creative_id": "...",
  "platform_post_id": "...",
  "audience_strategy_version_id": "asv_...",
  "experiment_id": "exp_...",
  "experiment_arm_id": "arm_...",
  "offer_snapshot_id": "offer_...",
  "published_at": "...",
  "binding_hash": "sha256:..."
}
```

一次重新剪辑、换字幕、换价格或重发都应创建新的 rendered artifact 或 binding，不复用旧记录伪装成同一成片。

## 3. 兼容现有契约

### 3.1 Brief 不立即升级

首个纵向切片沿用 `contracts/brief-3.0.schema.json`：

- `strategy_version_id` 指向已批准的 AudienceStrategyVersion。
- `audience` 保存人类可读摘要。
- `primary_variable`、`controlled_variables` 和 `measurement_window` 继续描述实验。

只有同时存在多种策略类型、且 ID 无法可靠消歧时，才评估新增显式 `audience_strategy_ref`。没有真实需求前不发布 Brief 3.1。

### 3.2 ContentItem 不写厂商字段

`contracts/content-item-3.0.schema.json` 已覆盖镜头的首帧、运动、尾帧、素材、权利、商品真实性、连续性和验收标准。V5 的 storyboard builder 从这些字段派生生产任务，不能向 ContentItem 写入：

- `@图片1` 等临时编号。
- Seedance 模式或平台按钮名称。
- 上传顺序和本机路径。
- 厂商特定提示词模板。
- 某次生成的图片和视频二进制位置。

### 3.3 复用 Artifact 与 DeliveryPackage

每张分镜图、审核接触图、提示词 Markdown/JSON、生成底片和最终成片都使用 Artifact 记录摘要、媒体类型、来源能力和可见性。DeliveryPackage 的 manifest 组合这些 Artifact，不新建重复的文件元数据系统。

## 4. 状态与门禁

```text
AudienceStrategyVersion
candidate -> review_ready -> approved -> deprecated

StoryboardPackage（Codex 本机）
candidate -> review_ready -> superseded
                  |
                  | publish
                  v
SubmissionRevision（服务端） -> ReviewCycle -> ApprovedSnapshot（locked）

SeedancePromptPackage
draft -> validated -> exported
  |          |           |
  +----------+-----------+-> stale -> superseded
```

硬门禁：

1. AudienceStrategyVersion 必须引用未过期的 taxonomy ApprovedSnapshot，且人群代码、名称和定义必须与该基线一致。
2. 未批准的人群策略不能进入可交付 Brief。
3. blocked ContentItem 不能生成 `review_ready` 分镜。
4. StoryboardPackage 的 `content_item_id` 必须是所引用 content_batch ApprovedSnapshot 的 eligible object，且 `source_digest` 必须一致。
5. 分镜素材或权利不完整时，服务端不能批准该 Revision。
6. 只有服务端批准后 pull 回本机的 storyboard ApprovedSnapshot 才代表 locked；只有本地媒体仍匹配其中的 `locked_digest` 才能导出厂商包。
7. Seedance 包中出现未映射 `@引用`、超限素材、失效 Offer 或摘要漂移时不能 `validated`。
8. 没有最终成片摘要和平台标识时不能建立 PublishedCreativeBinding。
9. 没有 binding 的投放数据可暂存隔离区，但不能进入正式归因。

## 5. Workspace 路径

V5 不增加新的顶级目录，沿用 V3 Workspace：

```text
50-production/
  media/
    storyboards/<storyboard_package_id>/
      manifest.json
      review-sheet.jpg
      shots/<shot_id>/first-frame.png
      shots/<shot_id>/end-frame.png

60-delivery/
  packages/<delivery_package_id>/
    manifest.json
    providers/seedance/
      package.json
      README.md
      prompts/
        segment-01.txt
      media/
        image-01.png
  exports/
    <delivery_package_id>.zip
```

`README.md` 是给操作者的导入说明，不是 Skill 本身；`package.json` 是机器可检验契约。文件名可以稳定，正确性以 Artifact ID、SHA-256 和 manifest 为准。

## 6. 本地候选与服务端正式事实

V5 沿用 V3 的边界，不将本地目录当作服务端数据库的镜像：

| 对象阶段 | Codex 本机 Workspace | ContentCloud 服务端 |
| --- | --- | --- |
| 人群目录 | pull 后只读使用 | 管理 taxonomy snapshot、来源和有效期 |
| 人群策略 | 生成 candidate、写入 LocalRunContext | 接收 publish 的 Revision，审核后批准/废弃 |
| Brief/ContentItem | 生成、lint、修订 candidate | 保存 SubmissionRevision、Decision、ApprovedSnapshot |
| 分镜与图片 | 调用获授权能力生成候选，保存本地媒体和 manifest | 接收明确披露的审核副本/摘要，保存评审、批准和 lock 决定 |
| Seedance 包 | 编译、lint、写入 `60-delivery`，不上传平台 | 可在 publish 后保存不可变 package manifest 与 DeliveryPackage 元数据，不保存绝对路径 |
| Seedance take / 后期工程 | 用户导回、选择、剪辑并登记候选 Artifact | 仅在显式 publish 后接收允许披露的最终 Artifact/元数据 |
| 发布与结果 | 可发起绑定/导入命令 | 校验 binding、持久化结果、RatingDecision 和 Learning |

服务端的 storyboard ApprovedSnapshot 是正式 `locked` 事实，锁定对象是所批准 Revision 中的 `locked_digest`。Codex 在导出前必须 `pull` 对应快照，并重新校验本地媒体；不得依据本地 `review_ready` 或聊天声明导出为正式交付。

## 7. 不变量

- ContentBatch 冻结 Brief、知识、人群策略和 Offer 引用，不拥有可变人群目录。
- 厂商包只从 storyboard ApprovedSnapshot 及与其 `locked_digest` 一致的本地 StoryboardPackage 媒体派生。
- review sheet 不能作为默认 Seedance 输入，除非某个经验证模式明确需要漫画/分镜板演绎。
- 任意 `@引用` 必须对应 upload manifest 中恰好一个 Artifact 和 SHA-256。
- 同一个 Artifact 在同一包中使用稳定编号；分段包必须说明编号是全局还是分段作用域。
- 商品真实素材优先级高于生成想象；无法保证真实性的镜头 blocked 或切换 Plan B。
- 上游 Skill 的更新不会静默改变已导出包，适配器升级必须产生新 capability digest。
- PerformanceObservation、RatingDecision 和 Learning 继续追加式记录，不覆盖历史。

## 8. 权利、安全与披露

- 生成前校验所有参考图、视频、音乐、人物形象和字体的使用范围及有效期。
- 平台不接受或合规不允许的真人素材必须在 export 前 blocked，不能靠提示词规避。
- 上游 Skill 纳入仓库前需记录固定 commit、LICENSE 或作者书面授权的内部 Evidence；“已沟通”不替代可审计记录。
- 本机绝对路径、登录凭据、平台 token、隐藏推理和未授权原件不得写入 Seedance 包或服务端投影。
- 厂商上传是一次明确的外部披露动作。首版由用户手工上传；未来自动上传必须单独设计授权、数据保留和审计。
