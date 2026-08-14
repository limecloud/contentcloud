---
name: contentcloud-storyboard-production
description: 在已绑定的 ContentCloud 工作区中，根据已批准 ContentItem 构建、生成、校验、发布和修订本地分镜图片包。适用于 ContentItem 到镜头规划、首尾帧制作、审核表准备、分镜审核交接或 digest 漂移诊断；必须确保 Codex 只生成候选，唯一权威锁定快照由 ContentCloud 服务端批准产生。
---

# ContentCloud 分镜制作

根据已批准且供应商中立的 ContentItem 制作可独立审核的首帧/尾帧。保留产品事实、连续性、权利、引用和实验意图。

## 执行边界

| 平面 | 允许工作 |
| --- | --- |
| `Codex local` | 拉取已批准内容、创建镜头任务、调用已授权图片能力、写入本地媒体、计算 digest、生成审核表，并 lint `review_ready` 候选。 |
| `ContentCloud server` | 接收明确的分镜发布、校验提交修订版本、承载审核、创建 ApprovedSnapshot，并记录锁定决策和审计轨迹。 |
| `Human` | 选择生成帧、判断产品事实与连续性、授权披露、确认发布，并批准或要求修改。 |

`StoryboardPackage.status=review_ready` 只表示本地就绪，不是 `approved` 或 `locked`。只有从 ContentCloud 拉取 `storyboard` ApprovedSnapshot，且包内 `locked_digest` 仍与本地媒体匹配时，分镜才被锁定。

## 工作流

1. 必须使用已拉取且包含可交付 ContentItem 的 `content_batch` ApprovedSnapshot。不得从未批准的 `50-production/batches` 候选开始。

2. 创建本地分镜包：

   ```bash
   contentcloud local storyboard create \
     --snapshot <content-approved-snapshot-id> \
     --content-item <content-item-id> \
     --capability-id <image-capability-id> \
     --capability-version <version> \
     --capability-digest sha256:<digest>
   ```

3. 每个生成镜头目录准确写入一张 `first-frame` 图片和一张可选 `end-frame` 图片。文件扩展名必须受有效能力支持。在包根目录生成 `review-sheet` 供人工审核。

4. 只要外观重要，就使用已批准真实产品资产。不得重新生成 SKU 形状、包装文字、接口、配件、比例、认证、价格、折扣或产品结果。无法保留产品事实时，切换到已声明 Plan B。

5. 保留每个镜头的传入/传出状态、运动轴、光照锁定、产品锁定、锚点、权利、知识、断言引用、负向约束和验收标准。审核表不得作为默认视频模型参考。

6. 发现媒体并准备审核：

   ```bash
   contentcloud local storyboard prepare <manifest.json>
   contentcloud local storyboard lint <manifest.json>
   ```

7. 运行分镜发布预检。向服务端发送任何内容前，确认准确披露列表和计划：

   ```bash
   contentcloud publish storyboard --file <manifest.json> --dry-run
   ```

8. 发布后停止。由用户或已授权审核员在 ContentCloud 服务端完成审核。不得将本地清单修改为 `approved` 或 `locked`，也不得创建伪造 ApprovedSnapshot。

9. 服务端批准后，拉取准确快照：

   ```bash
   contentcloud pull approved --type storyboard
   ```

10. 交接给 Seedance 导出前，对照已拉取快照校验所有本地文件 SHA-256 和包 `locked_digest`。任何被修改、替换、重新压缩、裁剪或重命名的文件都必须创建新的本地候选和审核修订版本。

## 审核要求

要求人工审核叙事一致性、受众策略、产品外观与使用方式、首尾状态连续性、运动轴、光照、身份锚点、权利、9:16 安全构图、字幕空间和可观察验收标准。

任何首帧、审核表、权利、Plan B、能力 digest 或已批准上游引用缺失时停止。报告准确镜头和下一项本地或服务端动作，不得跨越执行边界。
