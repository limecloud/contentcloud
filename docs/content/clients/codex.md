# Content Work OS 与 Codex

状态：**可用**。

Codex 是当前最完整的 Content Work OS 连接工具。它可以连接项目文件夹、继续上次工作、整理资料并把结果提交到网页工作台审核。

这里的“可用”指完整的客户创作和网页审核流程。不同 Codex 版本的显示方式可能不同：有的会显示完整页面，有的会显示文字和按钮，但都可以完成同一条工作流。

## 可用能力

| 能力 | 状态 |
| --- | --- |
| 电脑上的资料整理和修改 | 可用 |
| 连接项目文件夹 | 可用 |
| 准备项目文件夹 | 可用 |
| 在网页和电脑之间继续工作 | 可用 |
| 创作所需工具 | 可用 |

## 接入方式

1. 在有本机配置权限的 Codex Desktop 或 Codex CLI 中运行。
2. 登录 Content Work OS，在项目的“连接工作电脑”页创建连接会话（`ConnectSession`）。
3. 使用页面提供的连接文字连接项目文件夹。
4. 核对要改动的目录和版本，确认后再应用操作（`apply`）。
5. 连接成功后，在同一个项目文件夹中新建 Codex 对话。
6. 新对话先调用 `workspace_context`；只有页面提示需要修复时，才调用 `workspace_doctor`。

固定的安装方式、命令和安全说明以兼容入口 [`/codex`](/codex) 为准。该入口同时提供网页说明和连接工具可读文本。

## 日常使用

- 使用 `workspace_context` 恢复当前项目、工作记录（`Run`）和继续工作状态。
- 写入前选择要继续的工作记录，并取得编辑许可（`claim`）。
- 使用对应内容形式的工具生成或修改草稿。
- 提交前运行本地检查（`lint`）和 `publish_preflight`。
- 只有在你确认具体修改范围后，才执行提交。
- 网页工作台退回后，通过“在连接工具中修改”打开绑定准确版本的新对话。

## 当前内容形式

- [营销视频](../content-types/marketing-video.md)：可用。
- [微信公众号文章](../content-types/wechat-article.md)：可用，需要管理员为客户开启。

完整场景流程见[使用 Codex 制作营销视频内容](../guides/marketing-video/codex.md)和[使用 Codex 制作微信公众号文章](../guides/wechat-article/codex.md)。
