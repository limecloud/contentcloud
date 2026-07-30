# 微信公众号文章

状态：**可用，需租户显式开通**。

ContentCloud 支持从批准知识和 ArticleBrief 生成结构化 ArticleItem，在 Web 中审核精确 Revision，并从批准快照导出微信公众号本地交付包。平台管理员必须先为目标租户开启 `wechat_article`；视频剧本仍是所有租户唯一的默认内容能力。

## 当前范围

- ArticleBrief 表达选题、读者收益、结构、语气、CTA、知识和主张要求。
- ArticleItem 使用受控 blocks 表达标题、摘要、正文、列表、引语、图片、提示和 CTA，不保存任意 HTML。
- 事实、商业主张和引语通过 assertion 绑定批准知识或证据。
- 公共审核页按文章 Schema 显示正文 blocks、主张、引用和 JSON Pointer 评论。
- ApprovedSnapshot 可导出 JSON、Markdown、安全语义 HTML、素材清单、操作说明和本地预览。

## 开通与环境

1. 平台管理员在系统后台为租户开启“微信公众号文章”。
2. Workspace 操作者通过受控恢复流程刷新签名 Environment Manifest 和本地 lock。
3. 在同一 Workspace Root 新建 Codex 对话并调用 `workspace_context`。
4. 确认返回的 `content_types` 包含 `wechat_article`，再使用 ContentCloud WeChat Article Skill Pack。

CLI、MCP 和 Skill 只能消费签名 Manifest，不能自行开通能力。租户关闭能力后，服务端在创建、内审和最终批准阶段都会重新拒绝公众号内容。

## 交付边界

首版只生成本地交付包。以下动作必须由操作人员在微信公众号后台完成：

- 登录账号。
- 上传或替换素材。
- 检查预览和移动端排版。
- 创建草稿或正式发布。
- 把外部内容 ID、URL 和发布时间登记回 ContentCloud。

ContentCloud 不索取公众号凭据，也不声称“交付包已生成”等于“公众号已发布”。

完整流程见[使用 Codex 制作微信公众号文章](../guides/wechat-article/codex.md)。
