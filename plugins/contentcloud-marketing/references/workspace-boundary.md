# 营销 Skill 工作区边界

## 先解析 Workspace

任何读写前调用 `workspace_context` 和 `workspace_status`，确认当前项目、Workspace Root 和模板状态。Skill 包根目录不是客户项目目录；不得通过当前工作目录猜测客户身份。

## 数据分层

- Workspace 数据：来源、证据、知识、主张、素材、权利、客户 profile、意图配置、Run、审核队列和候选输出。
- Plugin 能力：中文流程、提示边界、MCP 工具编排和确定性门禁顺序。
- Core 能力：文件安全、摘要、Run Claim、状态迁移、知识 lint/query/pack、内容 lint、发布预检和云端审批。

## 状态规则

- `FactAssertion` 只有人工依据证据确认后才能是 `verified`。
- `Claim` 只有人工批准后才能是 `approved`。
- `RightsRecord` 只有权利依据有效后才能是 `valid`。
- 候选知识不足时输出 blocked 结果，不能用模型常识补齐事实。

## 写入规则

所有本地写入都通过 `contentcloud-local` stdio MCP 或受管 CLI 完成。先取得对应 Run Claim，写入后记录 `changed_ids`、`output_refs` 和检查结果；不要直接修改插件包，也不要创建第二套 RunContext 文件。

## 云端规则

`publish_preflight` 只生成披露范围、摘要和 `plan_id`。未得到用户对准确 `plan_id` 的明确确认前，不调用 `publish_apply`，不称内容已经批准或发布。
