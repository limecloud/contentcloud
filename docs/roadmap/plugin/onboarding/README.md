# ContentCloud 客户初始化与排障方案组

状态：`方案待实施`。

更新时间：2026-07-27。

本目录把 Codex 客户初始化拆成一组可以独立实施和验收的方案。目标不是让客户阅读一篇很长的安装教程，而是让服务端、Web、Bootstrap CLI 和 Codex 按同一套检查编号协作：正常情况走最短路径，出现问题时只展开当前失败步骤，最终仍能生成可交给客服的脱敏诊断结果。

架构总方案仍以 [插件路线图](../README.md) 为事实源，实施状态仍只在 [PLAN.md](../PLAN.md) 跟踪。本目录只负责客户初始化、诊断和支持体验，不重复定义 Creative Environment、Marketplace Registry、Environment Manifest、多对话 Handoff 或 Automation 协议。

## 1. 已确认的产品前提

首版可以明确要求客户具备：

- Codex Desktop。
- Codex CLI，并与 Desktop 使用同一系统用户和默认 `CODEX_HOME`。
- Node.js 20 或更高版本，以及可用的 npm/npx。
- 当前 macOS 用户存在可用的默认 Keychain，用于保存设备和 Workspace 凭据。
- 能访问 ContentCloud、npm、Git Marketplace 来源和 OpenAI 登录服务的网络。
- 对目标工作区和用户级 Codex 配置有写权限。

这组前提把问题从“完全无本地运行环境的 Desktop 用户如何静默安装”收敛为“如何让客户在已有受支持环境中完成一次可验证初始化”。教程可以解释前提，但真正的成功依赖自动预检、结构化错误和可恢复事务，不能依赖客户判断日志。

## 2. 核心决策

1. Web 项目页仍是唯一初始化入口，不新增独立的插件管理产品。
2. 正常路径只要求客户复制一次 Prompt、允许 Codex 运行固定版本 CLI、确认一次安装计划。
3. Marketplace 和 Plugin 操作优先调用官方 `codex plugin` 命令，不通过第三方安装器手工模拟 Codex 状态。
4. 服务端决定允许的 Scene Plugin、Skill Pack、版本、来源和 digest，但服务端不在客户机器执行命令。
5. CLI 负责本机 Detect、Plan、Apply、Validate、Rollback 和 Resume；Web 负责展示服务端能够证明的进度。
6. `ConnectSession` 继续是唯一连接生命周期。初始化阶段和诊断结果作为它的子投影，不创建第二套互相竞争的连接状态机。
7. 安装、MCP、Skill 或 `AGENTS.md` 发生变化后，固定进入新 Codex 对话，不假设当前对话可以热加载。
8. 客户只看到业务步骤和下一动作，不需要选择底层插件 ID，也不需要手工编辑 TOML/JSON。
9. 所有支持内容按稳定 `check_id` 和 `action_id` 分发。服务端不能向客户端动态下发任意 shell 命令。
10. 诊断默认最小披露，不上传 Prompt、对话、连接密钥、环境变量值、客户文件正文或全部插件清单。

## 3. 四级解决路径

| 级别 | 适用场景 | 客户动作 | 系统责任 | 成功出口 |
| --- | --- | --- | --- | --- |
| L0 快速初始化 | 环境满足前提 | 复制 Prompt、发送、确认计划 | 自动预检、安装、doctor、注册并打开新对话 | `connected` |
| L1 分步引导 | 缺少 Node、CLI、登录或权限 | 每次只完成一个明确步骤并点“重新检查” | 精确定位失败检查，返回对应教程与检测方法 | 回到 L0 当前阶段 |
| L2 自动恢复 | 授权后安装中断、doctor 或打开 Desktop 失败 | 确认恢复已有环境 | 幂等 `resume`，不重复绑定、不覆盖冲突配置 | 新对话可打开 |
| L3 客服协助 | 未知错误、策略限制、重复失败 | 提交支持码或脱敏诊断包 | 服务端关联 attempt、版本、失败检查和审计事件 | 明确修复或产品缺陷 |

升级顺序必须固定为 `L0 -> L1 -> L2 -> L3`。客户不应该在第一个错误出现时就被要求重装所有软件，也不应该在设备/Workspace 已创建后重新发起授权破坏恢复现场。

## 4. 组件职责

| 组件 | 负责 | 不负责 |
| --- | --- | --- |
| ContentCloud Web | 创建 ConnectSession、复制 Prompt、打开 Codex、展示进度和下一动作 | 猜测本机插件状态、执行本地命令 |
| Control Plane | 签发受控初始化描述、Environment Manifest、支持内容和版本策略 | 下发任意命令、读取客户目录 |
| Bootstrap CLI | 本机预检、官方 Codex 安装事务、Workspace 初始化、doctor、resume、诊断包 | 自由选择市场、上传客户内容 |
| Codex Desktop | 承载引导对话、用户确认、Plugin 使用和后续创作对话 | 代替服务端授权、跨对话保存业务状态 |
| Scene Plugin | 新对话中的 Workspace 探测、业务路由和 MCP 工具入口 | 安装自身、修改 Marketplace |
| 客服控制台 | 按支持码查看脱敏检查结果和版本事实 | 查看 Prompt、token、客户文件正文 |

## 5. 文档索引

- [初始化流程](./INITIALIZATION_FLOWS.md)：定义快速初始化、分步引导、恢复和手工路径，以及服务端交互时机。
- [诊断协议](./DIAGNOSTIC_PROTOCOL.md)：定义 stage、check、action、进度事件、隐私边界和诊断包。
- [客服运行手册](./SUPPORT_RUNBOOK.md)：定义客户与客服如何按检查编号逐步排查和升级。

## 6. 参考项目的取舍

### `wshobson/agents`

采用它的单一源、多宿主 Adapter、项目级 Marketplace 和小插件组合思想。首版不采用其 `npx codex-marketplace` 安装路径，也不接受只做结构校验的 Codex 兼容声明。ContentCloud 必须使用官方 Codex CLI 完成真实 Marketplace/Plugin 安装，并在 Desktop 做端到端验收。

### `@taptap/maker`

采用浏览器授权、CLI doctor、多客户端检测和升级后 reconnect 的产品经验。不采用直接把 MCP 配置当作完整插件安装、未锁定 npx 运行版本或依赖客户手工判断 PATH 的方式。

## 7. 成功指标

首版上线前至少收集并达到：

- 支持环境中的首次初始化成功率。
- 从点击初始化到 `connected` 的 P50/P95 时间。
- 各 `check_id` 的失败率、自动恢复率和人工支持率。
- ConnectSession/attempt 过期，以及授权完成后错误创建新会话的比例。
- 初始化后新 Codex 对话成功打开并读到 Workspace Context 的比例。
- 诊断包秘密扫描零泄露。
- 客服无需索取截图或完整日志即可定位的工单比例。

不得用虚假的线性百分比表达初始化进度。Web 只展示已开始、已完成或需要客户动作的真实阶段。

## 8. 官方能力依据

- [Package your plugin](https://developers.openai.com/plugins/build/plugins)：Plugin 结构、repo/personal Marketplace、Desktop 安装和官方 Marketplace CLI。
- [Plugins](https://learn.chatgpt.com/docs/plugins)：插件安装、连接和 Workspace 策略边界。
- [ChatGPT desktop app commands and deep links](https://learn.chatgpt.com/docs/reference/commands)：打开新对话、Workspace 和 Plugin 安装界面的能力边界。
