# 开始使用

ContentCloud 的使用路径由两个维度决定：你使用的 Agent 客户端，以及你要生产的内容形态。客户端可用不代表所有内容形态都可用，内容形态可用也不代表每个客户端都已完成接入。

## 1. 选择客户端

当前推荐使用 Codex。Codex 已支持 Workspace 注册、初始化、创作环境和交互式 Handoff，可以完成完整的本地创作与云端治理流程。

Claude Code 当前只开放本地 Automation 与 Workspace 注册能力，尚不能替代 Codex 完成 Web 初始化、创作环境准备或交互式 Handoff。WorkBuddy、Cursor、Hermes 和 OpenClaw 已进入兼容目录，但仍是规划状态。

## 2. 选择内容形态

营销视频默认对所有租户开放。微信公众号文章也已支持，但必须由平台管理员针对租户显式开通；开通后需要刷新签名 Environment Manifest，旧 Manifest 不能自行获得该能力。

其他内容形态仍处于规划状态。不要根据规划页面自行推断命令、Schema 或发布步骤。

## 3. 连接项目

1. 登录 ContentCloud。
2. 创建或选择一个项目。
3. 打开“接入与初始化”。
4. 选择具备 `workspace_bootstrap` 能力的客户端。
5. 按页面生成的固定 Prompt 或确定性计划完成初始化。
6. 安装或环境变更后，在相同 Workspace Root 中新建 Agent 会话。

初始化不会自动上传已有文件、启动 Daemon 或替你提交内容。每次安装、授权、写入、pull、publish 和人工决定都有独立边界。

## 4. 开始第一条工作流

选择[使用 Codex 制作营销视频内容](guides/marketing-video/codex.md)或[使用 Codex 制作微信公众号文章](guides/wechat-article/codex.md)。进入新对话后先调用 `workspace_context`，从持久化 Workspace 状态恢复工作，不要依赖旧聊天记录重建项目事实。

## 5. 在 Web 中协作

- 本地 Agent 负责候选资料、知识、Brief 和内容的生成与修订。
- Web 只展示已经显式提交的不可变 Revision。
- 审核人批准后形成 ApprovedSnapshot；本地文件中的 `approved` 文案不能替代它。
- 交付必须基于明确拉取的批准快照，不能把“生成成功”推断为“已发布”。

开始前建议阅读[受治理的内容工作流](concepts/governed-workflow.md)。
