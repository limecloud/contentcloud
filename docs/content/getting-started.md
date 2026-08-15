# 开始使用

Content Work OS 的使用路径由两个维度决定：你使用的智能体客户端，以及你要生产的内容形态。客户端可用不代表所有内容形态都可用，内容形态可用也不代表每个客户端都已完成接入。

## 1. 选择客户端

当前推荐使用 Codex。Codex 已支持本地工作区注册、初始化、创作环境和交互式任务交接，可以完成完整的本地创作与云端治理流程。

| 客户端 | 当前可用范围 | 尚未开放 |
| --- | --- | --- |
| Codex CLI/Desktop | 完整客户流程；Skills + stdio MCP 控制面；MCP Apps 最小协议闭环 | Codex Desktop App/Bridge 真实宿主验收 |
| Claude Code CLI | 本地自动化、工作区注册、Plugin/Skills/stdio MCP 控制面 | Web bootstrap、交互式交接、内联富 UI |
| Claude Desktop/Web、Cursor、VS Code GitHub Copilot | 上游具备部分 MCP Apps 或 Agent Plugins 能力 | ContentCloud 安装投影、项目绑定、生命周期和真实 UI 验收 |
| GitHub Copilot 其他 Surface、Kiro、Gemini CLI、Cline、Windsurf、Continue | 协议候选 | ContentCloud 正式 Adapter 与完整验收 |
| Hermes、OpenClaw、WorkBuddy、Grok Bot、NanoClaw | 规划或非首发 | 客户侧完整接入 |

“上游支持 Agent Plugins、Agent Skills、MCP 或 MCP Apps”不等于 ContentCloud 已支持该客户端。正式开放必须同时通过安装、工作区绑定、MCP 生命周期、呈现降级和安全测试；详细工程矩阵见[本地工作台技术方案](../product/customer-creation-studio/05-local-workbench-browser.md)。

## 2. 选择内容形态

营销视频默认对所有租户开放。微信公众号文章也已支持，但必须由平台管理员针对租户显式开通；开通后需要刷新签名执行环境清单（`Environment Manifest`），旧清单不会自动获得该能力。

其他内容形态仍处于规划状态。不要根据规划页面自行推断命令、数据格式或发布步骤。

## 3. 连接执行客户端

1. 登录 Content Work OS。
2. 创建或选择一个项目。
3. 打开“执行客户端”。
4. 选择具备 `workspace_bootstrap` 能力的客户端。
5. 按页面生成的固定操作指令或确定性计划完成初始化。
6. 安装或环境变更后，在相同的本地工作区根目录（`Workspace Root`）中新建智能体会话。

初始化不会自动上传已有文件、启动后台进程或替你提交内容。每次安装、授权、写入、拉取（`pull`）、提交（`publish`）和人工决定都有独立边界。

客户端的本地呈现可能是 MCP App、受控 Browser/WebView 或纯 Tool/Resource。缺少富 UI 时仍应使用类型化 Headless 流程，不能要求模型复制带 token 的 localhost URL，也不能临时启动长期 Node 服务。

## 4. 开始第一条工作流

选择[使用 Codex 制作营销视频内容](guides/marketing-video/codex.md)或[使用 Codex 制作微信公众号文章](guides/wechat-article/codex.md)。进入新对话后先调用 `workspace_context`，从持久化的本地工作区状态恢复工作，不要依赖旧聊天记录重建项目事实。

## 5. 在 Web 中协作

- 本地智能体负责候选资料、知识、创作简报和内容的生成与修订。
- Web 工作台只展示已经显式提交的不可变内容版本。
- 审核人批准后形成已批准快照（`ApprovedSnapshot`）；本地文件中的 `approved` 文案不能替代它。
- 交付必须基于明确拉取的批准快照，不能把“生成成功”推断为“已发布”。

开始前建议阅读[受治理的内容工作流](concepts/governed-workflow.md)。
