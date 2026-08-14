# ContentItem

运行时提供的 Schema 始终具有权威性。使用选定 V3 ContentBatch 或不可变 Automation Contract 中冻结的 Schema；不得添加旧 Contract 的兼容字段。

## 顶层意图

- `deliverability`：只有所有阻断规则通过时才为 `review_ready`，否则为 `blocked`。
- `project_id`、`content_batch_id`、`brief_ref` 和 `context_snapshot_id`：冻结的本地血缘。
- `direction`：选中的角度、钩子、母题、叙事、语气、情绪和风险。
- `cover`：首屏产品或品牌信号、视觉意图、资产、权利、安全区域和遮挡保护。
- `narrative_structure`：映射到时间范围和镜头 ID 的有序决策功能。
- `shots`：完整、连续的时间线。
- `citations`：知识 ID 到镜头和用途的明确映射。
- `asset_requirements`：事实等级、权利、用途和降级方案。
- `experiment`：一个主变量、控制维度、假设、测量窗口和指标。
- `global_constraints`：禁止断言、品牌规则、产品事实、连续性和安全区域。

## 镜头契约

为每个镜头提供：

- 稳定的 `shot_id`、连续的 `start_ms`/`end_ms`，以及一个叙事 `role`。
- 面向决策的 `narrative_purpose` 和可观察的 `visual_intent`。
- 主体、物理动作、构图、镜头运动、声音、可选旁白和画面文字。
- 作为三个兼容状态的 `first_frame`、`motion_spec` 和 `end_frame`。
- 仅使用合格 `knowledge_refs`、已批准断言、资产和有效权利。
- 一种制作模式：`real_asset`、`asset_guided_generation`、`generated_non_product`、`composite` 或 `external_capture`。
- 负向约束、带锚点的连续性传入/传出、产品事实策略、可测量验收标准，以及可执行 Plan B。

本地 review-ready 必需角色是 `hook`、`proof`、`cta`，以及 `product_intro|product_solution` 之一。镜头时间码必须从零开始、保持连续，并在 `duration_ms` 结束。

## 产品事实策略

- `real_asset_composite`：使用真实包装、标志、标签、认证或可读产品材料。
- `generated_environment`：只生成周围环境；使用真实资产保护产品事实。
- `no_product_detail`：保持生成的类产品形态通用，不得暗示其为准确产品呈现。

## 引用用途

使用 `spoken_claim`、`on_screen_text`、`visual_fact` 或 `style_rule`。引用只能指向冻结本地上下文或 Automation Contract 中的合格知识 ID。
