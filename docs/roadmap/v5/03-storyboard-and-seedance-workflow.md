# 分镜图与 Seedance 可复制交付流程

## 1. 设计目标

V5 要交付的是一条可执行链路，而不是在剧本末尾追加一大段提示词：

```text
规范剧本 -> 独立分镜图片 -> 人工审核并锁定 -> 厂商适配 -> 按清单上传 -> 复制生成 -> 后期合成
```

其中分镜图片解决“视觉是否正确”，Seedance 提示词解决“如何让画面动起来”，后期方案解决“文字和权益如何准确呈现”。三个责任不能混在同一生成步骤。

本文件中的“系统”不是单一进程。执行标签如下：

- `Codex`：在已绑定的本机 Workspace 生成和校验候选。
- `服务端`：接收显式 publish，执行权限、审核、批准、lock 和正式存证。
- `外部平台`：用户在 Seedance 或抖音界面执行上传、生成或发布。
- `人工`：作出选择、批准、外部披露和发布决定。

## 2. 端到端步骤

### 2.1 冻结生产输入

执行方：`Codex + 服务端 pull`。服务端保存 ApprovedSnapshot；Codex 通过 pull 得到本地只读生产输入。

输入必须是包含 ContentItem 的 ApprovedSnapshot，并同时固定：

- AudienceStrategyVersion。
- Brief 与 ContentBatch context snapshot。
- 知识、approved claims 和禁止声明。
- 商品与场景资产、RightsRecord。
- 需要时固定 CommerceOfferSnapshot。

ContentItem 为 blocked 或任一核心引用失效时停止，不自动补写商品事实。

### 2.2 构建分镜任务

执行方：`Codex 本机`。Storyboard builder 是本地 Skill/能力，不在服务端同步请求中生成媒体。

Storyboard builder 对每个 shot 生成结构化任务：

```text
叙事目的 + 主体/商品 + 场景 + 构图/景别 + 首帧状态
+ 动作意图 + 尾帧状态 + 连续性锁 + 资产引用 + 禁止项
```

同一人物、商品或场景使用公共 identity anchors。提示词不能只写“保持一致”，而要明确哪些可观察属性必须相同，例如包装颜色、瓶盖形状、人物服装、光线方向和运动轴。

### 2.3 生成独立图片

执行方：`Codex 本机 + 用户授权的媒体生成能力`。输出先进入本地 `50-production/media`，未 publish 前服务端不可见。

每个 shot 默认生成一张首帧，动作复杂或跨场景时增加尾帧。生成规则：

- 商品为视觉中心时优先使用已授权真实商品资产进行引导或合成。
- 不允许模型重新设计 SKU、包装文字、接口、配件或规格。
- 生成图保持干净，不烘焙字幕、价格、LOGO、镜头编号和安全区辅助线。
- 保存模型、版本、能力 digest、提示词、引用素材摘要以及可得的 seed。
- 若模型不能满足商品真实性，使用 `real_asset`、`composite` 或 `external_capture` Plan B。

### 2.4 生成审核接触图

执行方：`Codex 本机生成 + 服务端/人工审核`。Codex 生成 review sheet；用户显式 publish 审核副本、摘要和允许披露的媒体后，服务端才创建 ReviewCycle。

系统把独立图片排成 review sheet，并叠加 shot ID、时间、旁白摘要、首尾状态和风险提示。review sheet 只用于人类审核，不是默认视频生成输入。

审核至少覆盖：

1. 剧情和人群策略是否一致。
2. 商品外观、使用方式和结果是否真实。
3. 首尾状态、视线、运动轴、光线和道具是否连续。
4. 权利、人物形象、场景和品牌规则是否允许。
5. 9:16 构图、主体可见区和后期字幕空间是否合理。
6. 每个镜头是否有可验证的画面验收条件。

### 2.5 批准与锁定

执行方：`ContentCloud 服务端 + 人工审核者`。服务端对已发布 manifest digest 作出 approved/locked 决定；Codex 只能 pull 该决定，不能在本机自行批准。

本地 `review_ready` 只表示已具备审核材料，不是批准。服务端批准该 storyboard SubmissionRevision 后创建的 ApprovedSnapshot 同时表示：内容决定通过，并锁定 Revision 内的 `locked_digest`。Codex 在 publish 前对 manifest、全部独立图片和参考素材计算摘要；服务端还要确认 `content_item_id` 属于所引用 content_batch ApprovedSnapshot，并复算 `source_digest` 与 `locked_digest` 后才可批准。任何换图、裁切、压缩或重命名导致 manifest 变化时都必须派生新版并重新审核。

## 3. 上游 Seedance Skill 的引用方式

上游 `songguoxs/seedance-prompt-skill` 提供了有价值的中文提示词模式、时间戳分镜、`@图片N/@视频N/@音频N` 引用、长视频分段以及操作型输出格式。ContentCloud 可以参考，但不应直接把它放在核心剧本生成入口无条件执行。

### 3.1 推荐落点

执行方：`Codex 本机`。该 Skill 随 ContentCloud Plugin/Workspace 能力运行，不部署为服务端视频生成任务。

当前实现嵌入 ContentCloud Plugin：

```text
plugins/contentcloud-video-production/skills/contentcloud-seedance-export/
  SKILL.md
  agents/openai.yaml
```

确定性编译和校验由 `internal/localworkspace/seedance_v5.go` 承担，输出契约由 `contracts/seedance-prompt-package-1.0.schema.json` 固定。这样 Skill 只负责编排，命令和服务端可以复用同一组领域门禁。

职责划分：

- `SKILL.md` 只保留读取 storyboard ApprovedSnapshot、核对 `locked_digest`、执行门禁、选择模式、导出和验证的核心流程。
- 当前 profile 通过 CLI 显式输入版本、时长和素材上限；未提供经人工验证的值时拒绝导出。
- 完成人工平台验证并归档 Evidence 后，再增加版本化 `references/seedance-provider-profile.md`；当前仓库不得先写猜测参数。
- Go validator 校验素材数量、引用完整性、时长、SHA-256、Offer 和 rights 状态。

这符合 Skill 的渐进式披露原则：核心流程保持短小，频繁变化的厂商知识独立版本化。上游调研固定在 commit `57d1e2f273747c238dd892698a05137ab2f10d4a`。2026-07-29 查询时仓库未声明 GitHub license 且根目录没有 LICENSE，因此 V5 不复制上游文件，也不从 `master` 静默拉取生产规则；作者沟通或书面授权必须另行归档为内部 Evidence。

### 3.2 触发边界

适配 Skill 只在用户表达“导出/生成 Seedance 包”，且 Codex 已从服务端 pull 到 storyboard ApprovedSnapshot，并确认本地媒体仍匹配其中 `locked_digest` 时触发。它不负责：

- 自己决定目标人群或批准策略。
- 修改 ContentItem、商品声明或分镜图。
- 绕过人工审核、RightsRecord 或 Offer 有效期。
- 直接登录平台或上传素材。
- 把模型自评当作 QA 通过。

缺输入时返回结构化 blocked reason 和下一动作，不根据聊天上下文猜路径或引用编号。

## 4. Provider Profile 与能力漂移

执行方：`Codex/人工采集候选 + 服务端批准版本 + Codex pull 使用`。真实产品验证发生在用户的 Seedance 界面，正式 profile 作为治理事实发布到服务端。

上游 Skill 当前描述的能力包括多模态引用、图片/视频/音频上限、4 至 15 秒生成、首尾帧/全能参考、视频延长和原生声音等。这些是适配器的候选基线，不是永久契约。

每个 provider profile 至少记录：

| 字段 | 示例 |
| --- | --- |
| `provider` | `seedance` |
| `profile_version` | `2026-07-28-manual-verified` |
| `verified_surface` | 实际使用的即梦/Seedance 产品入口和区域 |
| `model_label` | UI 当时展示的模型名称 |
| `supported_modes` | 首尾帧、全能参考、延长等 |
| `limits` | 素材格式、数量、大小、总时长和生成时长 |
| `face_policy` / `rights_policy` | 当时入口的实际拦截与合规要求 |
| `verified_at` / `expires_at` | 验证和复核时间 |
| `evidence_refs` | 截图、官方帮助或人工测试记录 |

profile 过期时仍可查看旧交付，但不能静默为新交付盖章。验证器应以 profile 数据判断，不把数字散落在提示模板中。

## 5. 分段与素材编号

### 5.1 分段原则

若成片时长超过单次生成能力，按叙事和连续性切段：

- 每段只承载一个清晰动作或情绪转折。
- 上一段 `outgoing_state` 必须能成为下一段 `incoming_state`。
- 优先使用已批准尾帧作为下一段参考，或使用上一段成片执行经验证的延长模式。
- 不能把一条 30 秒剧本机械切成两个 15 秒片段而不检查动作和台词。
- 分段时长取自当前 provider profile，不能写死为永远 15 秒。

### 5.2 编号算法

编号必须由 manifest 确定性生成：

1. 公共商品/人物/场景 identity anchors 优先。
2. 再按 segment 顺序和 shot 顺序排列首帧、尾帧。
3. 同一 Artifact 只分配一次编号。
4. 编号超过 provider profile 上限时，拆成独立分段作用域并生成新的上传清单。
5. Prompt 中的每个 `@引用` 必须反向解析到一个 Artifact，未使用素材也应被 lint 提示。

## 6. 提示词编译

执行方：`Codex 本机`。服务端可以复验 package schema 和摘要，但不运行 Seedance Prompt，也不调用 Seedance 生成。

适配器将规范字段编译为中文自然语言，建议顺序：

```text
模式与技术设置
+ @素材用途
+ 本段输入状态
+ 按时间的主体动作和环境变化
+ 景别、角度、运镜、焦点和节奏
+ 台词、音效和声音意图
+ 本段输出状态与衔接点
+ 商品真实性和连续性约束
+ 禁止项
```

提示词必须描述可观察动作，避免堆叠互相冲突的“电影感、8K、大片感”等形容词。对于 9 至 15 秒或动作较多的段落使用相对时间段；短单动作镜头不必为了格式强行切碎。

字幕、价格、优惠、LOGO、水印和法律说明默认写入禁止项，并在 `post_production_plan` 中恢复。若真实平台证明可以稳定生成某类文字，也只能作为可选能力，不得牺牲权益准确性。

## 7. 最终可复制交付格式

执行方：`Codex 本机生成交付包 + 用户在 Seedance 手工执行`。ContentCloud 服务端只在显式 publish 后保存 DeliveryPackage manifest 和允许披露的 Artifact。

每个 Seedance 目录同时生成机器可检验的 `package.json` 和人类可操作的 `README.md`。README 必须能独立完成操作，不要求用户回到聊天记录寻找编号。

对应的命令闭环如下，前三组命令由 Codex 在本机执行，`publish` 的 apply 阶段和 `submission approve` 的决定写在服务端，最后一步由用户在 Seedance 界面执行：

```bash
contentcloud local storyboard create --snapshot <content-snapshot> --content-item <content-item> ...
contentcloud local storyboard prepare <manifest.json>
contentcloud publish storyboard --file <manifest.json> --dry-run
contentcloud publish storyboard --file <manifest.json> --plan-id <confirmed-plan> --review
contentcloud pull approved --type storyboard
contentcloud local seedance export --snapshot <storyboard-snapshot> --storyboard <storyboard-id> --profile-version <verified-profile> ...
contentcloud local seedance lint <package.json>
```

`submission approve` 不应被 Codex 自动串入上述脚本；它是服务端授权用户在查看审核材料后作出的独立决定。导出完成后 CLI 只返回本地目录、上传顺序和提示词文件，不调用 Seedance。

```markdown
# Seedance 生成包：15 秒便携榨汁杯视频

输入版本：StoryboardPackage sbp_123，locked digest sha256:...
适配版本：contentcloud-seedance-export/1.0.0
能力快照：seedance/2026-07-28-manual-verified

## 平台设置

- 模式：全能参考
- 比例：9:16
- 本段生成时长：12 秒
- 声音：生成环境音；旁白在后期合成

## 上传顺序

1. `media/image-01.png` -> `@图片1`，真实商品正面参考，SHA-256: ...
2. `media/image-02.png` -> `@图片2`，S01 首帧，SHA-256: ...
3. `media/image-03.png` -> `@图片3`，S02 尾帧，SHA-256: ...

上传后逐项核对缩略图与编号，再复制提示词。

## 第 1 段

生成时长：12 秒

### 可复制提示词

9:16 竖屏商品短视频。@图片1仅用于锁定榨汁杯的真实外观、颜色、
杯盖和按钮位置，@图片2为开场构图，@图片3为结尾状态。0-2秒……
3-7秒……8-12秒……结尾保持……。不得改变商品结构、配件数量和包装，
禁止任何字幕、价格、优惠、LOGO或水印，不生成未经批准的功效结果。

### 衔接与验收

- 输入状态：……
- 输出状态：……
- 必须满足：商品按钮位置与 @图片1 一致；动作轴不反转。
- 失败时 Plan B：改用真实商品实拍并合成环境背景。

## 后期合成

- 旁白：使用批准文案 VO-01。
- 字幕：读取 `captions.srt`，应用 9:16 安全区。
- 价格与优惠：仅使用发布前仍有效的 OfferSnapshot offer_123。
- LOGO 与 CTA：使用已授权品牌资产，不交给生成模型绘制。
```

实际交付中的“可复制提示词”必须是一个连续纯文本块，不能插入解释、引用脚注或 Markdown 标题。解释和验收放在文本块之外。

## 8. 生成后与后期

执行方：`用户在外部平台生成 + Codex/本地工具导回和后期 + 服务端接收最终 publish`。

Seedance 结果先作为 `generated_plate` Artifact 导入并记录文件摘要、生成时间、包 ID、segment ID 和人工选择。人工 QA 检查商品、动作、连续性、异常画面和合规后，才进入后期。

后期输出 `rendered_creative` Artifact，并记录：

- 使用的生成底片和真实素材。
- 剪辑时间线或工程摘要。
- 旁白、音乐、字体和权利引用。
- 字幕、价格、优惠、LOGO、CTA 和 OfferSnapshot。
- 画幅、时长、编码、音量与平台安全区检查。

最终成片不是 SeedancePromptPackage 本身。只有 rendered creative 被放入 DeliveryPackage 并建立 PublishedCreativeBinding 后，生产链路才算进入可归因状态。

## 9. 失败恢复

| 失败 | 处理 |
| --- | --- |
| 商品外观漂移 | 拒绝该 take；增强真实资产约束或切换 composite/实拍 Plan B |
| 人物/场景不连续 | 定位违反的 anchor，重生成相关 shot，不重做整条剧本 |
| `@引用` 错位 | 验证器阻止导出，重新按 manifest 生成文本 |
| 素材超平台上限 | 按叙事拆分作用域，不随机丢弃参考图 |
| 平台能力与 profile 不一致 | 标记 profile 过期，记录真实界面证据并创建新版 |
| Offer 已过期 | 不必重生成干净底片；更新批准 Offer 后重新合成和绑定 |
| 分镜被修改 | 旧 prompt package 标记 stale，从新 locked digest 重建 |
| 生成结果不可用 | 保存失败分类和 take 记录，人工选择重试、Plan B 或回到分镜修订 |
