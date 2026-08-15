# ContentCloud Marketing Skill Pack 支持手册

## 运行前提

1. 使用 ContentCloud Agent Plugins Loader 加载标准包。
2. 确认 `contentcloud-local` stdio MCP 由 Core Scene Plugin 提供；本 Skill Pack 不启动第二个 MCP 或 Node 服务。
3. 确认当前对话绑定了准确的 Workspace Root，并通过 `workspace_context` 校验项目身份。
4. 安装、升级、修复或移除后新建宿主对话。

## Environment 与业务编排

- 运行前请求 `environment_execution_plan`，确认 `contentcloud.marketing.knowledge-governance` 或 `contentcloud.marketing.content-orchestration` 已由签名 Environment 允许。
- 缺少营销 Pack 时只调用 `environment_prepare_plan` 展示版本、摘要、权限、数据流、费用和新会话影响；用户确认同一个计划后才 apply。
- 营销编排通过 Core stdio MCP 的 `workspace_context`、`local_run_*`、知识工具和内容工具完成；视频和文章分别交接给对应形态 Pack。
- 所有阶段复用同一个 Run；失败后 `local_run_resume`，不得创建第二个状态源或用新 Run 隐藏失败历史。

## 数据边界

- 客户资料、品牌规则、素材、来源、知识页、运行记录和输出只存在于客户 Workspace。
- 包目录只读，不能写入客户事实、凭据、绝对路径或原始素材。
- `PLUGIN_DATA` 只能保存插件自身的非业务缓存，不能替代 Workspace 事实源。

## 故障恢复

- 缺少 Core MCP、Workspace Root 或能力包时停止，并报告稳定错误和下一步；不得扫描其他目录。
- 运行占用过期时只能在用户确认后 takeover；旧 owner 的写入必须被拒绝。
- 知识、内容或发布检查失败时保留当前 Run，修复后 resume；不得新建 Run 伪装成功。
- digest 不一致必须使用新的不可变包；不得原地修复已安装包。

## 云端边界

本包只生成本地候选、检查和预检计划。只有用户明确确认准确的 `plan_id` 后，才允许调用 `publish_apply`；本包不代替人工审批、渠道登录或外部平台发布。

## 上报信息

支持诊断只报告 Plugin ID、版本、包摘要、宿主、Run 摘要、稳定错误码和脱敏信息；不得输出客户原文、Token、Cookie、绝对路径或宿主原始配置。
