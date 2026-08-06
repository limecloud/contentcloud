# 使用 Codex 制作微信公众号文章

状态：**可用，需租户显式开通**。

本教程适用于已连接 Content Work OS 本地工作区、具备项目权限，并已由平台管理员开通微信公众号文章的租户。

## 1. 开通并刷新环境

1. 平台管理员在“系统后台 / 租户 / 内容能力”开启“公众号”。
2. 在项目现有接入入口执行受控工作区恢复，刷新签名执行环境清单和锁文件。
3. 确认 `contentcloud-wechat-article` 技能包已安装；安装或变更后新建 Codex 对话。
4. 在本地工作区根目录调用 `workspace_context`，确认 `content_types` 包含 `wechat_article`。

不要编辑执行环境清单、锁文件或本地配置来模拟开通。CLI 和服务端都会重新验证租户能力。

## 2. 准备批准知识

1. 选择或创建本地执行记录（`LocalRun`），并取得单写者声明（`claim`）。
2. 从已接受的证据中提取事实、营销主张、素材和权利记录候选。
3. 运行知识检查，提交知识内容版本并在 Web 工作台完成审核。
4. 显式拉取已批准知识快照。

未经批准的资料可以帮助发现缺口，但不能作为正式事实或商业主张依据。

## 3. 创建文章创作简报

1. 使用 `$contentcloud-article-planning` 定义主题、读者收益、结构、语气、行动引导和唯一实验变量。
2. 把必需事实与批准商业主张分别写入文章创作简报。
3. 缺少事实、营销主张、素材或权利时保持“已阻断（`blocked`）”，列出 `blocked_reasons` 和 `missing_inputs`。
4. 调用 `article_brief_lint`。
5. 通过 `publish_preflight` 展示精确计划，用户确认同一 `plan_id` 后再 `publish_apply`。
6. 在 Web 工作台批准创作简报，并显式拉取文章创作简报批准快照。

## 4. 生成和修订文章

1. 使用 `$contentcloud-longform-writing` 调用 `article_batch_create`，固定已批准创作简报和知识快照。
2. 在返回的批次目录中生成文章内容项，使用受控内容块和稳定的事实陈述标识。
3. 为事实、商业主张和引语绑定批准引用；观点、个人经历和假设必须清晰分类。
4. 需要配图时使用 `$contentcloud-article-visuals`，只绑定冻结上下文中具备 Rights 的素材。
5. 依次调用 `article_item_lint`、`article_batch_lint` 和 `article_batch_finalize` 完成检查与定稿。
6. 通过精确的提交前检查与确认，发布内容批次（`content_batch`）版本。

Web 工作台退回后，从原内容版本派生新的文章内容项，填写 `based_on_version_id`、已解决评论和变更摘要。调用 `article_item_diff`，只允许用户确认的 JSON 指针发生变化。

## 5. 审核与批准

公共审核页会安全展示文章标题、摘要、作者、正文内容块、事实陈述和引用，不执行文章内容项中的 HTML。审核人可以批准或退回；批准后服务端创建不可变的已批准快照。

本地候选、已提交内容版本和已批准快照是三种不同状态。文件中出现 `approved` 文案不能替代服务端批准。

## 6. 生成本地交付包

1. 显式拉取包含目标文章内容项的已批准快照。
2. 使用 `$contentcloud-wechat-delivery` 调用 `wechat_package_export`。
3. 对返回的 `providers/wechat-official-account/package.json` 调用 `wechat_package_lint`。
4. 检查本地预览、素材映射、文件摘要和操作说明。
5. 由操作人员登录公众号后台，上传素材、粘贴内容、预览并发布。

技能不会登录、创建公众号草稿、上传素材或发布文章。外部发布完成后，另行登记外部绑定和结果，不要把本地交付包状态当作平台发布状态。
