# 使用 Codex 制作营销视频内容

状态：**可用**。

本教程适用于已登录 Content Work OS、拥有项目权限，并能在本机运行 Codex Desktop 或 Codex CLI 的用户。

## 1. 连接执行客户端

1. 在 Web 工作台创建或选择项目。
2. 打开“执行客户端”。
3. 创建连接会话并复制页面生成的连接指令。
4. 在具备本机权限的 Codex 会话中执行该操作指令所描述的公开初始化流程。
5. 核对固定插件身份、目标目录变化和 `plan_id`，确认后再应用。
6. 等待 Web 工作台显示本地工作区已连接。

安装身份和手动接入命令以 [`/codex`](/codex) 为准。不要替换插件市场来源、引用（`ref`）、插件 ID 或版本。

## 2. 在新对话恢复项目

安装或环境变更后，在已验证的本地工作区根目录中新建 Codex 对话。先调用：

```text
workspace_context
```

如果结果是 `repair_required`，再调用 `workspace_doctor`。如果存在多个活跃执行记录，先让用户选择准确的 `run_id`；如果存在“可继续（`ready`）”的任务交接，接受后重新校验摘要。

## 3. 准备可信输入

1. 摄取项目资料并冻结当前输入范围。
2. 使用知识提取技能形成带证据的候选知识。
3. 本地检查后显式提交知识内容版本。
4. 在 Web 工作台完成人工审核，拉取精确的已批准快照。

模型常识、历史聊天和未经批准的客户材料不能替代可信知识。

## 4. 形成策略与创作简报

1. 选择明确受众、渠道、场景和营销目标。
2. 使用受众策略能力形成候选策略。
3. 创建绑定知识快照的创作简报。
4. 明确批准主张、禁用表达、行动引导、实验变量和内容数量。
5. 运行本地检查；缺失输入时保留“已阻断（`blocked`）”状态并列出原因。

## 5. 生成并审核营销内容

1. 为当前任务创建或选择本地执行记录。
2. 取得单写者声明，并固定创作简报与知识快照。
3. 使用营销视频剧本技能生成内容批次候选。
4. 检查引用、主张、素材权利、时长、画幅和完整性。
5. 执行 `publish_preflight`，向用户展示精确范围、摘要和云端影响。
6. 用户确认同一计划后再提交，创建不可变的提交内容版本。
7. 在 Web 工作台审核；批准会产生已批准快照，退回会产生可恢复反馈。

## 6. 处理退回意见

在 Web 工作台点击“在智能体客户端中修订”，选择 Codex。系统只会打开带预填操作指令的新对话，不会自动发送，也不会自动选择本地工作区。

新对话重新调用 `workspace_context`，读取目标内容版本、摘要和审核评论。修订时保留 `based_on_version_id`、已解决评论和变更摘要，再提交新内容版本。

## 7. 生成分镜与交付

1. 拉取明确的已批准快照。
2. 使用分镜生产技能生成分镜；不得从未批准候选直接生成正式下游。
3. 通过分镜 lint 后生成 Seedance 交付包。
4. 核对素材、权利、镜头连续性、提示词和文件摘要。
5. 将“交付包已生成”和“外部平台已发布”分别记录。

## 8. 使用 Seedance 2.5 服务端执行

当租户已经配置并批准 `modelark-seedance25` Provider Profile 和 Binding 时，优先通过 ContentCloud Media Job 执行单镜头生成：

1. 确认当前分镜是已批准快照，且 `SeedancePromptPackage` 的锁定摘要、Profile 版本和输入 Artifact 没有漂移。
2. 创建 `MediaGenerationJob`，只填写快照、模式、画幅、时长和 Artifact ID；不要填写本地绝对路径或长期视频 URL。
3. 等待费用审批。估算费用来自 Provider Profile 的时长价格，未配置价格的 Provider 会被阻断。
4. 由 Media Worker 提交、轮询、取消和下载。服务商超时或状态不明时不要重新创建任务，等待对账。
5. 生成 MP4 通过技术校验后会产生候选 Artifact 和待处理的内容审核；它不能直接作为最终成片或交付包。

第一阶段仅支持 `text_to_video` 与 `image_to_video` 单镜头。多镜头、续写、编辑、音频驱动和超长视频继续使用手动导出或保持未启用状态。

### HTTP 操作契约

服务端执行使用当前登录会话的 BFF API。上传提示包使用 `multipart/form-data`，字段为 `snapshot_id` 和 `file`；`file` 必须是已经校验过的 JSON `SeedancePromptPackage`：

```text
POST /api/bff/tasks/{taskID}/seedance-prompt-package
```

上传成功返回 `Artifact`，后续创建 Media Job 时将其 `id` 作为 `prompt_package_artifact_id`。不要把提示词正文、绝对路径或长期 URL 放进 Media Job。

提交超时或结果不明时，先通过状态对账确认外部任务 ID，再使用：

```json
POST /api/bff/media-jobs/{id}/reconcile-submit
{"expected_version": 3, "external_job_id": "外部任务标识"}
```

该接口只接受 `awaiting_external_result` 任务，不能覆盖已经绑定的外部 ID，也不会重新提交生成请求。`expected_version` 必须来自最新的 Media Job 投影。

部署方通过受控 BFF 配置 Provider，不要直接写数据库：平台管理员先 `POST /api/bff/admin/provider-profiles` 创建 `draft` Profile，再调用 `/api/bff/admin/provider-profiles/{providerID}/{version}/publish` 发布；租户管理员随后 `PUT /api/bff/provider-bindings/{providerID}` 配置 Binding。Binding 的 `profile_version` 必须与已发布 Profile 精确一致，active 非 fake Provider 的 `credential_ref` 只能是 `secret://`、`vault://` 或 `env://` 引用，不能提交 API Key。响应不会返回凭据字段。

Worker 只有在设置 `CONTENTCLOUD_SEEDANCE25_API_KEY` 和 `CONTENTCLOUD_SEEDANCE25_ALLOWED_HOSTS` 后才注册 Provider；这里的环境变量是 Worker 进程从受控 SecretRef 解析后的运行时注入，不应写入 Provider Profile 或 Binding。真实凭据、费用和输出域名必须在沙箱环境完成受控验收。

遇到连接或恢复问题时，查看[本地工作区与任务交接故障排查](../../troubleshooting/workspace-and-handoff.md)。
