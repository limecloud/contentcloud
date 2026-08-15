---
name: contentcloud-seedance-execution
description: 通过 ContentCloud Media Job 受治理地执行已批准的单镜头 Seedance 2.5 生成。适用于费用审批、任务提交、状态恢复、取消、下载和媒体审核；不得直接调用 ModelArk 或绕过 ContentCloud 控制面。
---

# ContentCloud Seedance 2.5 执行

## 执行边界

| 平面 | 允许工作 |
| --- | --- |
| `Codex local` | 绑定工作区、读取已批准快照、展示 Media Job 状态、生成受控操作参数。 |
| `ContentCloud server` | 校验快照和 Artifact、估算费用、登记 Effect、调用 Provider Worker、下载并校验 MP4、创建审核记录。 |
| `Provider worker` | 使用部署环境中的 SecretRef 调用 ModelArk Seedance 2.5；只接收服务端解析后的输入。 |
| `Human` | 批准费用、取消结果不明的外部任务、审核画面和选择最终成片。 |

不得在本 Skill 中注册或调用 `modelark-mcp`，不得把 Artifact ID 替换为本地绝对路径或长期 URL，也不得直接写入最终交付状态。

## 前置条件

1. 当前工作区已通过 `workspace_context`，并绑定正确的租户、项目和 `run_id`。
2. 输入是 `StoryboardSnapshot` 和 `SeedancePromptPackage` 的已批准版本，锁定摘要与 Artifact SHA-256 一致。
3. 租户已经启用 `modelark-seedance25` Provider Binding，并使用已发布、未过期的 Profile；部署已完成一次受控 Provider 健康检查。
4. Profile 只允许单镜头 `text_to_video` 或 `image_to_video`，且包含经核验的费用字段。

## 工作流

1. 读取批准快照，确认当前 Stage 是 `generation`，不要从候选分镜或可变本地文件创建 Job。
2. 创建 `MediaGenerationJob`，填写快照、PromptPackage Artifact、模式、画幅、时长和 Artifact ID；首阶段只创建一个片段。
3. 如果任务进入 `awaiting_cost_approval`，向用户展示估算费用和币种，等待有权限的项目负责人或租户管理员批准。
4. 费用批准后由 Worker 提交 ModelArk 任务。记录返回的外部任务 ID 后，重启或轮询只使用该 ID，不重新提交。
5. `queued`/`running` 等中间状态继续等待；`failed`/`expired` 进入失败处理；`succeeded` 才允许受控下载。
6. 取消时先调用 Provider。外部取消超时或结果不明时进入 `awaiting_external_result` 对账状态，不把本地 Job 伪装成 `cancelled`。
7. 下载结果必须通过域名白名单、MIME、大小、MP4 容器和 SHA-256 校验。成功后产生候选 Artifact、技术审核和待处理内容审核。

## 当前限制

- 只支持单镜头 `text_to_video` 与 `image_to_video`。
- 单任务最长 30 秒、最多 30 张图片，提示词最多 32,000 个 Unicode 字符；Provider Profile 只能进一步收紧限制。
- 暂不支持续写、视频编辑、首尾帧组合、音频驱动、超长视频和多镜头并行；这些能力必须等独立分段数据模型和真实接口验收完成后开放。
- Provider 超时或提交结果不明时禁止自动重试提交，必须先对账。
- 生成 Artifact 不是最终成片；必须经过内容审核、后期和最终审核。
