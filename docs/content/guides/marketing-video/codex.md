# 使用 Codex 制作营销视频内容

状态：**可用**。

本教程适用于已登录 ContentCloud、拥有项目权限，并能在本机运行 Codex Desktop 或 Codex CLI 的用户。

## 1. 连接 Workspace

1. 在 Web 创建或选择项目。
2. 打开“接入与初始化”。
3. 创建初始化会话并复制页面生成的 Prompt。
4. 在具备本机权限的 Codex 会话中执行该 Prompt 所描述的公开 bootstrap 流程。
5. 核对固定 Plugin 身份、目标目录变化和 `plan_id`，确认后再应用。
6. 等待 Web 显示 Workspace 已连接。

安装身份和手动接入命令以 [`/codex`](/codex) 为准。不要替换 Marketplace source、ref、Plugin ID 或版本。

## 2. 在新对话恢复项目

安装或环境变更后，在已验证的 Workspace Root 新建 Codex 对话。先调用：

```text
workspace_context
```

如果结果是 `repair_required`，再调用 `workspace_doctor`。如果存在多个活跃 Run，先让用户选择准确的 `run_id`；如果存在 ready Handoff，接受后重新校验摘要。

## 3. 准备可信输入

1. 摄取项目资料并冻结当前输入范围。
2. 使用 Knowledge Extraction Skill 形成带 Evidence 的候选知识。
3. 本地检查后显式提交 Knowledge Revision。
4. 在 Web 完成人工审核，拉取精确 ApprovedSnapshot。

模型常识、历史聊天和未经批准的客户材料不能替代可信知识。

## 4. 形成策略与 Brief

1. 选择明确受众、渠道、场景和营销目标。
2. 使用受众策略能力形成候选策略。
3. 创建绑定知识快照的 Brief。
4. 明确批准主张、禁用表达、CTA、实验变量和内容数量。
5. 运行本地 lint；缺失输入时保留 blocked 状态并列出原因。

## 5. 生成并审核营销内容

1. 为当前任务创建或选择 LocalRun。
2. 取得单写者 claim，并冻结 Brief 与知识快照。
3. 使用 Marketing Video Script Skill 生成 ContentBatch candidate。
4. 检查引用、主张、素材权利、时长、画幅和完整性。
5. 执行 `publish_preflight`，向用户展示精确范围、摘要和云端影响。
6. 用户确认同一计划后再 publish，创建不可变 SubmissionRevision。
7. 在 Web 审核；批准会产生 ApprovedSnapshot，退回会产生可恢复反馈。

## 6. 处理退回意见

在 Web 点击“在 Agent 中修订”，选择 Codex。系统只会打开带预填 Prompt 的新对话，不会自动发送，也不会自动选择 Workspace。

新对话重新调用 `workspace_context`，读取目标 Revision、digest 和审核评论。修订时保留 `based_on_version_id`、已解决评论和 change summary，再提交新 Revision。

## 7. 生成分镜与交付

1. 拉取明确的 ApprovedSnapshot。
2. 使用 Storyboard Production 生成分镜；不得从未批准候选直接生成正式下游。
3. 通过分镜 lint 后生成 Seedance 交付包。
4. 核对素材、权利、镜头连续性、提示词和文件摘要。
5. 将“交付包已生成”和“外部平台已发布”分别记录。

遇到连接或恢复问题时，查看[Workspace 与 Handoff 故障排查](../../troubleshooting/workspace-and-handoff.md)。
