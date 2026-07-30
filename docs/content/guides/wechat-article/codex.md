# 使用 Codex 制作微信公众号文章

状态：**可用，需租户显式开通**。

本教程适用于已连接 ContentCloud Workspace、具备项目权限，并已由平台管理员开通微信公众号文章的租户。

## 1. 开通并刷新环境

1. 平台管理员在“系统后台 / 租户 / 内容能力”开启“公众号”。
2. 在项目现有接入入口执行受控 Workspace 恢复，刷新签名 Environment Manifest 和 lock。
3. 确认 ContentCloud Marketplace 中的 `contentcloud-wechat-article` Skill Pack 已安装；安装或变更后新建 Codex 对话。
4. 在 Workspace Root 调用 `workspace_context`，确认 `content_types` 包含 `wechat_article`。

不要编辑 Manifest、lock 或本地配置来模拟开通。CLI 和服务端都会重新验证租户能力。

## 2. 准备批准知识

1. 选择或创建 LocalRun，并取得单写者 claim。
2. 从已接受 Evidence 中提取事实、Claim、素材和 Rights 候选。
3. 运行知识 lint，提交 Knowledge Revision 并在 Web 完成审核。
4. 显式拉取 Knowledge ApprovedSnapshot。

未经批准的资料可以帮助发现缺口，但不能作为正式事实或商业主张依据。

## 3. 创建 ArticleBrief

1. 使用 `$contentcloud-article-planning` 定义主题、读者收益、结构、语气、CTA 和唯一实验变量。
2. 把必需事实与批准商业主张分别写入 ArticleBrief。
3. 缺少事实、Claim、素材或权利时保持 `blocked`，列出 `blocked_reasons` 和 `missing_inputs`。
4. 调用 `article_brief_lint`。
5. 通过 `publish_preflight` 展示精确计划，用户确认同一 `plan_id` 后再 `publish_apply`。
6. 在 Web 批准 Brief，并显式拉取 ArticleBrief ApprovedSnapshot。

## 4. 生成和修订文章

1. 使用 `$contentcloud-longform-writing` 调用 `article_batch_create`，冻结批准 Brief 和知识快照。
2. 在返回的 batch 目录中生成 ArticleItem，使用受控 blocks 和稳定 assertion ID。
3. 为事实、商业主张和引语绑定批准引用；观点、个人经历和假设必须清晰分类。
4. 需要配图时使用 `$contentcloud-article-visuals`，只绑定冻结上下文中具备 Rights 的素材。
5. 依次调用 `article_item_lint`、`article_batch_lint` 和 `article_batch_finalize`。
6. 通过精确 preflight 与确认发布 `content_batch` Revision。

Web 退回后，从原 Revision 派生新 ArticleItem，填写 `based_on_version_id`、已解决评论和 change summary。调用 `article_item_diff`，只允许用户确认的 JSON Pointer 发生变化。

## 5. 审核与批准

公共审核页会安全展示文章标题、摘要、作者、正文 blocks、assertion 和引用，不执行 ArticleItem 中的 HTML。审核人可以批准或退回；批准后服务端创建不可变 ApprovedSnapshot。

本地候选、已提交 Revision 和 ApprovedSnapshot 是三种不同状态。文件中出现 `approved` 文案不能替代服务端批准。

## 6. 生成本地交付包

1. 显式拉取包含目标 ArticleItem 的 ApprovedSnapshot。
2. 使用 `$contentcloud-wechat-delivery` 调用 `wechat_package_export`。
3. 对返回的 `providers/wechat-official-account/package.json` 调用 `wechat_package_lint`。
4. 检查本地预览、素材映射、文件 digest 和操作说明。
5. 由操作人员登录公众号后台，上传素材、粘贴内容、预览并发布。

Skill 不会登录、创建公众号草稿、上传素材或发布文章。外部发布完成后，另行登记外部绑定和结果，不要把本地 package 状态当作平台发布状态。
