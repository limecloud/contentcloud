# 环境生命周期

## 检查

1. 解析一个已绑定 Workspace。
2. 读取 Environment Lock 和离线 doctor 报告。
3. 将已安装 Plugin、CLI、MCP、Skill、Schema 和路由 digest 与签名目标状态比较。
4. 将环境归类为 `ready`、`update_available`、`repair_required` 或 `blocked`。

## 变更

1. 生成 dry-run 计划，不消耗 connect key，也不修改文件。
2. 展示能力、版本、权限、网络、文件、供应商和费用变化。
3. 要求明确确认。
4. 只备份 ContentCloud 拥有的配置目标。
5. 应用 allowlist 中的固定版本和已验证 digest。
6. 校验生成文件并运行离线 doctor。
7. 校验失败时恢复之前的目标。
8. 分别报告每个目标。

不得修改其他 Marketplace、Plugin、MCP Server、Skill、用户指令或不归 ContentCloud 所有的配置块。

## 重新连接

Plugin、Skill、MCP 或项目路由变化后，必须开启新的 Codex 对话或 CLI 会话。创建不含秘密的 bootstrap Handoff，并打开已验证 Workspace Root。打开失败时，返回本地路径和恢复提示。

## 升级与重置

不得在活动的交互式或 Automation Run 期间升级。重置只恢复 ContentCloud 管理的环境文件，不得删除来源材料、知识、Brief、脚本、媒体、Submission、Approval 或无关 Agent 配置。
