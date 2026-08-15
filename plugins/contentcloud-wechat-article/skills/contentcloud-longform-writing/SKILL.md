---
name: contentcloud-longform-writing
description: 在受治理的微信文章 ContentBatch 中生成或修订带引用的 ContentCloud ArticleItem 候选。用于长文起草、标题变体、结构化文章块、断言、编辑评审、受控修订，或基于已批准 ArticleBrief 发布可评审的微信文章修订版。
---

# ContentCloud 长文写作

基于不可变的 ContentCloud 输入编写与供应商无关的 `contentcloud.article/1.0` 对象。规范文章内容是结构化数据，不是任意 HTML。

## 前置条件

1. 调用 `workspace_context`；必须具备已验证的 `wechat_article` 能力、匹配的环境锁、选定的 LocalRun 和有效 claim。
2. 使用 `article_batch_create` 冻结一个已批准的 ArticleBrief 及其合格 Knowledge ApprovedSnapshot。
3. 在新批次目录下只读取返回的 `manifest.yaml` 和 `context.json`。不要从聊天历史或较新的工作区文件重建冻结上下文。
4. 候选只能写入该批次内的 item 路径。

## 起草

1. 生成多个带明确策略和风险引用的标题候选，然后只选择一个标题 ID。
2. 使用受支持的语义块构建文章：`heading`、`paragraph`、`list`、`quote`、`image`、`callout`、`divider` 和 `cta`。
3. 每个块只承担一个编辑目的。使用稳定的块 ID 和断言 ID，确保评审评论和修订仍可定位。
4. 将断言分类为 `fact`、`commercial_claim`、`quotation`、`editorial_opinion`、`personal_experience` 或 `hypothesis`。
5. 每个事实、商业声明和引语都必须有引用。`commercial_claim` 只能引用 kind 为 `claim` 的已批准 Knowledge 项；引语必须保留归属信息。
6. 让编辑意见、个人经历和假设与已验证事实保持清晰区分。不得编造经历或归属。
7. 遵循 ArticleBrief 的字数范围、文风、结构、CTA、已批准声明和受控变量。必需门禁失败时使用 `blocked` 并填写可执行原因。

不得在 ArticleItem 中嵌入 HTML、脚本、跟踪代码、远程媒体、凭据或供应商专用草稿 ID。

## 校验与评审

1. 为每个候选调用 `article_item_lint`。
2. 使用精确 Manifest 和完整 item 列表调用 `article_batch_lint`。
3. 只有确定性检查通过后才能调用 `article_batch_finalize`。blocked 批次可以评审，但不可交付。
4. 只能通过 `publish_preflight`、对其 `plan_id` 的明确确认以及 `publish_apply` 发布精确的 `content_batch` 文件。

修订时设置 `based_on_version_id`、已解决的评论 ID 和 `change_summary`。使用明确允许的 JSON Pointer 前缀调用 `article_item_diff`。任何未声明路径的变更都视为错误，发布前重新执行 item 和 batch lint。
