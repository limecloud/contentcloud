---
name: contentcloud-wechat-delivery
description: 从已批准的 ContentCloud ArticleItem 导出并校验操作员可用的本地微信公众号交付包。用于安全语义 HTML、Markdown、JSON、资产映射、本地预览、操作员说明，或在 pull 精确 ArticleItem ApprovedSnapshot 后进行人工交接。
---

# ContentCloud 微信交付

将已验证的 ApprovedSnapshot 编译为本地交付文件。首个版本只支持人工交付：绝不登录微信、创建平台草稿、上传资产或执行外部发布。

## 前置条件

1. 调用 `workspace_context`；签名 Manifest 必须包含 `wechat_article`，并且存在匹配环境锁。
2. 必须有显式 pull 且经过 digest 验证的 `content_batch` ApprovedSnapshot，其中包含请求的 `review_ready` ArticleItem。
3. 已批准对象、内容 digest、资产权利或渠道配置缺失或过期时停止。不得导出未发布的本地候选。

## 导出与校验

1. 使用精确的已批准 ArticleItem ID 调用 `wechat_package_export`。输出保持在 `60-delivery/packages/` 下。
2. 对返回的 `providers/wechat-official-account/package.json` 调用 `wechat_package_lint`。
3. 验证交付包包含受治理 JSON、Markdown、安全语义 HTML、操作员说明、本地预览、文件 digest、资产映射和来源 ApprovedSnapshot ID。
4. 用户要求检查时，只打开生成的本地预览。预览是派生输出，不是规范内容。
5. 交接前报告未解决的 `manual_asset_upload` 映射。

## 人工交接

告知操作员按照生成的 `README.md`，手工执行以下外部动作：

```text
manual_login
manual_asset_upload
manual_preview
manual_publish
record_external_binding
```

不要自动化这些步骤，不要索要账户凭据，不要声称草稿已创建，也不要声称发布成功。交付包生成和 lint 不会产生外部微信副作用。只有操作员确认实际结果后，才能通过单独授权的 ContentCloud 流程记录外部绑定或结果。
