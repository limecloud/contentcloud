# V6 Runtime 与 Content Pack 需求

## 1. Runtime 统一视图

Runtime 是“谁在本机执行”的产品对象，不等同于某一个模型或 Skill。每个 Runtime 需要公开以下非敏感信息：

- `client_id`、显示名称和宿主版本。
- `device_id`、Workspace 绑定数量和最近在线时间。
- 已声明的 Capability、版本和 digest。
- Daemon 状态：installed、running、stopped、outdated、blocked。
- 最近一次 Attempt、进度 cursor、待重报和 dead-letter 数量。
- 当前 Environment Manifest 的签发时间、过期时间和验证结果。

敏感 token、模型凭据、本机绝对路径和 Agent 对话内容不能进入 Web 展示、官网、审计事件或公开文档。

## 2. Tenant Capability Matrix

租户管理员需要能够按租户查看和修改平台允许的能力。修改动作必须生成审计事件，并在下一次 Manifest 投影中生效。

```text
Tenant
  -> Content Capability
  -> Agent Client Capability
  -> Delivery Capability
  -> Automation Policy
```

### 最小字段

| 字段 | 说明 |
| --- | --- |
| `tenant_id` | 所属租户 |
| `capability_id` | 稳定能力 ID，如 `video_script`、`wechat_article` |
| `state` | enabled / disabled / unavailable / misconfigured |
| `source` | platform_registry / tenant_override / environment_check |
| `effective_at` | 本次状态生效时间 |
| `reason_code` | 可机器读取的阻断原因 |
| `manifest_digest` | 当前投影所依据的 Manifest 摘要 |
| `updated_by` | 操作人或系统身份 |

### 门禁要求

- Web 创建、CLI `local`、MCP、Skill、`publish`、审批和交付全部复验租户能力。
- 关闭能力不能删除已有的批准快照和审计历史。
- 关闭能力后，已有本地候选可以保留，但不能创建新的正式 Revision 或 DeliveryPackage。
- `misconfigured` 必须提供诊断入口，不允许用户用 `--force` 或隐藏参数绕过。

## 3. Content Pack 契约

Content Pack 是内容形态和其本地工作流的可发布单元，包含：

- 内容类型 ID 与 Schema 版本。
- Brief/Batch/Item 的输入输出契约。
- 本地 Skill、MCP 工具和 Agent handoff 说明。
- lint、diff、finalize、publish/pull 和交付包命令。
- 能力要求、外部平台边界、权利/事实/敏感信息门禁。
- Pack 版本、digest、签名、撤回状态和兼容的 CLI 版本范围。

Content Pack 不能把领域事实藏在 prompt 中。Prompt 只能编排已声明的契约和本地候选，服务端仍然负责身份、权限、Submission、审批和审计。

## 4. V6 首批 Pack

### `video_script`

- 默认启用。
- 继承 V5 的人群策略、StoryboardPackage、Seedance Prompt Package、成片绑定和结果学习。
- 官网展示为“完整生产闭环”，但明确 Seedance/抖音登录、上传和发布仍由用户执行。

### `wechat_article`

- 平台可用，租户默认关闭。
- 使用 ArticleBrief、ArticleItem、文章 lint/diff/finalize、Submission/Review 和公众号本地交付包。
- 不自动登录公众号后台、不代建草稿、不自动发布。
- 官网必须同时显示“可用”和“需租户开通”，避免把平台能力误解为租户已开通。

## 5. Environment Manifest 投影

签名 Manifest 是本地 Workspace 和官网/工作台的共同状态来源。V6 要求它可投影为：

```json
{
  "tenant_id": "tenant_x",
  "workspace_id": "workspace_x",
  "runtime": {"client": "codex", "version": "0.16.0", "status": "ready"},
  "content_packs": [
    {"id": "video_script", "state": "enabled", "digest": "sha256:..."},
    {"id": "wechat_article", "state": "disabled", "reason_code": "tenant_not_enabled"}
  ],
  "automation": {"state": "enabled", "max_concurrent_tasks": 2},
  "manifest_digest": "sha256:..."
}
```

示例只表达结构，不构成新的公共 Schema；实施时优先复用现有 Environment Manifest/Registry 契约。

## 6. 运行诊断

工作台需要按故障层级呈现：

1. 身份：登录、租户、项目和角色。
2. Workspace：根目录、模板锁、sync-state、受管目录和写入探针。
3. Runtime：客户端探测、Daemon、版本和进程健康。
4. Pack：签名、digest、能力和本地安装状态。
5. 业务：Brief/Batch/Item lint、权利、事实、审批和 Delivery。

每个失败项至少包含 `status`、`reason_code`、最近检查时间、影响对象和推荐动作。推荐动作只能导航到受支持的页面或命令，不生成任意 Shell。

## 7. 兼容策略

- 保留现有视频插件暂时承载通用 Scene/MCP 的兼容职责，直到独立 Runtime/Pack 入口完成迁移。
- 不立即删除旧命令；先在文档和 CLI 输出中标记为 compat，并通过治理检查阻止新的硬编码入口。
- 新增 Pack 必须通过 Schema、Plugin、Skill、Manifest、Tenant Capability 和官网 Registry 校验。
- 每个 Pack 至少覆盖：正常路径、未开通、Manifest 过期、digest 漂移、权限不足、Schema 不匹配和交付边界阻断。
