# 本地工作区、初始化、Skills/MCP 与发布同步

## 1. 定位

本地工作区是 ContentCloud V2 的主要创作界面。用户继续在熟悉的 Codex、Claude Code 或其他本地 Agent 中工作，ContentCloud 提供受治理的项目模板、Skills、MCP、确定性校验和云端审批边界。

```text
本地：原始资料、未发布知识、LocalRunContext、策略和创作草稿
云端：项目治理、不可变 Submission、人工决定、ApprovedSnapshot、客户协作
```

本地和云端通过显式 publish/pull 交换不可变检查点，不持续同步目录，也不把每次 Agent 对话变成云端 TaskRun。

## 2. 首次初始化

### 2.1 服务端准备

项目负责人在 Web 完成：

1. 创建 Client、Brand、Product 和 BrandProject。
2. 选择 TenantServiceTemplateVersion 和适用方法论。
3. 指定内部角色和客户审批人。
4. 创建短期有效、项目绑定的公开 ConnectSession。

Web 显示不含凭据的 Prompt；手工路径使用固定版本 CLI：

```bash
npx --yes @limecloud/contentcloud@0.6.0 bootstrap preflight ./contentcloud-project --server-url https://content.example.com --json
npx --yes @limecloud/contentcloud@0.6.0 bootstrap plan ./contentcloud-project --server-url https://content.example.com --session <session-id> --json
```

### 2.2 CLI 初始化

```mermaid
sequenceDiagram
    autonumber
    actor U as 本地用户
    participant CLI as contentcloud CLI
    participant API as 云端Bootstrap API
    participant FS as 本地工作区
    participant Agent as Codex/Claude配置

    U->>CLI: bootstrap preflight/plan ./project
    CLI->>CLI: 检查目标目录和本机依赖
    U->>CLI: 确认plan_id并执行apply
    CLI->>API: PKCE challenge发起浏览器授权
    U->>API: 在登录态页面核对短码并批准
    CLI->>API: verifier完成授权
    API-->>CLI: Workspace/Device Credential + 项目绑定 + 签名环境
    CLI->>CLI: 读取CLI内置版本化模板并计算文件hash
    CLI->>FS: 创建目录和模板文件
    CLI->>FS: 写project.yaml/template.lock/sync-state
    CLI->>Agent: 经确认安装项目级Skills和MCP配置
    CLI->>FS: workspace doctor + 初始lint
    CLI->>API: 注册WorkspaceBinding和模板版本
    CLI-->>U: 初始化结果、下一步和未安装项
```

## 3. 初始化安全规则

- 目标不存在时创建；空目录可以初始化。
- 非空且不是 ContentCloud 工作区时默认拒绝，先输出文件冲突报告。
- 已有同项目工作区时只允许 `bootstrap resume`，不重复授权或绑定。
- 不覆盖未知文件、用户已修改模板、现有 AGENTS.md 或 Agent 配置。
- `bootstrap plan` 只读输出将创建、修改、跳过和冲突的文件，并返回确定性 `plan_id`。
- `bootstrap apply` 必须携带刚确认的同一 `plan_id` 和 `--accept`。
- 默认不上传任何本地文件，不注册后台 Automation Daemon。
- 公开 session ID 不是凭据；浏览器批准受用户角色、租户和项目约束。

## 4. 工作区结构

```text
project-root/
├── .contentcloud/
│   ├── project.yaml
│   ├── template.lock
│   ├── sync-state.json
│   ├── inbox/
│   │   ├── review-feedback/
│   │   └── decision-deltas/
│   ├── cache/
│   │   └── approved/
│   ├── skills/
│   └── mcp/
├── AGENTS.md
├── methodology/
├── ontology/
│   ├── classes.yaml
│   ├── properties.yaml
│   ├── rules/
│   └── vocabularies/
├── schemas/
├── knowledge/
│   ├── index/
│   ├── sources/
│   ├── evidence/
│   ├── facts/
│   ├── claims/
│   ├── assets/
│   ├── rights/
│   ├── conflicts/
│   └── packs/
├── raw/
│   ├── inbox/
│   └── source-registry.yaml
├── work/
│   ├── current-focus.md
│   ├── conflicts.md
│   ├── knowledge-gaps.md
│   ├── review-queue.md
│   └── runs/
├── workflows/
├── scripts/
└── outputs/
    ├── briefs/
    ├── scripts/
    ├── storyboards/
    ├── reports/
    └── delivery/
```

`raw/` 默认加入项目忽略规则；模板不强制创建 Git 仓库。用户选择版本控制时，原始资料和本地敏感缓存仍保持忽略。

## 5. 模板清单与锁文件

当前实现由 CLI 内嵌 `workspace_marketing_video@2.0.0`，初始化后生成 `template.lock`，记录每个受管文件 hash、内置 Skill、MCP 和目标 Agent。服务端通过 `workspace.register` 记录 template ID/version/targets，但不下发或执行模板。

远程签名 `WorkspaceTemplateManifest` 是后续租户定制模板的目标契约，不是当前已实现入口：

```json
{
  "manifest_version": "1.0",
  "template_id": "workspace_marketing_video",
  "template_version": "2.0.0",
  "methodology_version": "2.0.0",
  "schema_versions": {},
  "files": [{"path": "AGENTS.md", "sha256": "...", "mode": "managed_merge"}],
  "skills": [{"name": "knowledge-content-pipeline", "version": "2.0.0", "sha256": "..."}],
  "mcp_servers": [{"name": "contentcloud-local", "version": "2.0.0"}],
  "signature": "..."
}
```

文件模式：

- `managed_replace`：纯生成脚本/Schema，只有内容仍等于旧 hash 时自动替换。
- `managed_merge`：AGENTS.md、工作流等允许用户扩展，升级生成三方 diff。
- `seed_once`：客户内容模板只在首次创建，之后永不自动覆盖。
- `local_state`：运行和同步状态，不参与模板升级。

`template.lock` 当前保存安装版本、CLI 版本、每个受管文件的基线 hash 以及 Skill/MCP 版本；远程模板启用后再增加签名摘要。

## 6. Skills 安装

当前随 CLI 内嵌并由 `init` 安装两个项目级 Skills：

```text
contentcloud-knowledge-extraction
contentcloud-marketing-video-script
```

Skills 只描述业务步骤、输入输出、门禁和 CLI/MCP 使用，不内嵌客户事实、模型配置和云端私有 HTTP。

CLI 以 `.contentcloud/skills` 为受管副本，并在初始化时为目标 Agent 生成项目级入口。独立查看/安装命令为：

```bash
contentcloud skills list
contentcloud skills status
contentcloud skills install contentcloud-marketing-video-script --target codex
```

知识查询、市场研究、策略 Brief、修订和反馈应用等更细 Skill 仍属于后续波次；当前不得标记为已安装。

## 7. MCP

`contentcloud-local` MCP 通过 `contentcloud mcp serve` 在本机以 stdio 运行。当前已实现 24 个工具，与 `09-cli-mcp-and-contracts.md` §4 的清单一致：

- 工作区：`workspace_status`、`workspace_doctor`
- 本地来源：`source_register`、`source_list`、`source_ingest`、`source_verify`
- 本地运行与知识：`local_run_init`、`local_run_show`、`knowledge_import_candidates`、`knowledge_lint`、`knowledge_query`、`knowledge_diagnose`、`knowledge_pack`
- Brief 与剧本：`brief_lint`、`creative_batch_init`、`creative_batch_lint`、`creative_batch_finalize`、`script_lint`、`script_diff`、`script_export`
- 云端治理：`publish_preflight`、`submission_status`、`review_feedback_list`、`approved_snapshot_list`

研究、策略编译和反馈应用相关工具属于后续波次。MCP 本身不直接调用私有 HTTP；需要云端数据时复用 CLI 的 Workspace Credential 和统一 dispatch 客户端。它不返回 token、不自动上传资料、不启动后台 Daemon。

```bash
contentcloud mcp status
contentcloud mcp serve
```

修改全局 Agent 配置属于高风险范围，V2 默认只写项目级配置；目标 Agent 只支持全局配置时必须展示精确 diff 并要求明确确认。

## 8. LocalRunContext

LocalRunContext 延续金陵古都香已验证的阶段交接：

```mermaid
stateDiagram-v2
    [*] --> ingest
    ingest --> knowledge_lint
    knowledge_lint --> query: lint passed
    knowledge_lint --> failed: lint failed
    query --> compile: eligible/blocked已记录
    compile --> output_lint: outputs已记录
    output_lint --> done: lint passed
    output_lint --> failed
    failed --> previous: 修复后resume
```

LocalRunContext 保存 run ID、intent、stage、source refs、changed IDs、eligible/blocked IDs、output paths、checks、findings 和 history。它不上传完整 Agent transcript，也不对应云端 TaskRun。

Submission 可以附带精简的 LocalRunSummary：阶段、校验结果、版本、输入/输出 hash 和用户确认，不包含工具逐步轨迹。

## 9. 本地知识流程

```text
raw/inbox
-> source registry + hash
-> evidence locator
-> candidate facts/claims/assets/rights
-> conflict/gap/review queue
-> deterministic lint
-> eligible/blocked query
-> seven-layer knowledge pack
-> publish preflight
```

Agent 不得用模型常识补客户事实，不得改变 verified/approved/valid 云端资格。本地可以表达“建议批准”，真正决定只在云端 ReviewCycle 中产生。

## 10. Publish 检查点

支持类型：knowledge、research、strategy、brief、script、delivery、performance。

```bash
contentcloud publish knowledge --review
contentcloud publish brief --review
contentcloud publish script --review
```

当前 publish 固定执行：

1. 解析项目上下文和最近 ApprovedSnapshot。
2. 运行对应 lint 和 Schema 校验。
3. 计算 canonical content hash、输入 hash 和最近批准基线 ID。
4. 检查 JSON、类型必填字段、blocked 标记、文件边界、大小和来源披露。
5. 展示将上传的结构化对象、披露数量、总字节和审核可见范围；raw 文件始终不上传。
6. 用户通过 `--review` 或 `--yes` 确认后提交。
7. 服务端复核 hash、Schema、基线、权限和幂等键。
8. 创建 SubmissionRevision 和 ReviewCycle，返回 revision 与下一条状态查询命令。

完整 Schema registry、对象存储上传许可和基线字段级 diff 尚未实现，属于后续加固项。

相同 submission type + content hash + base snapshot + idempotency key 重试只返回已有 revision。

## 11. 来源分级披露

| 等级 | 上传内容 | 审核能力 |
| --- | --- | --- |
| `metadata_only` | 来源 ID、hash、类型、locator 描述 | 只能知道来源存在，不能独立核验正文 |
| `evidence_pack` | 以上 + 精确摘录/安全预览 | 默认；可审核普通事实，受租户风险策略限制 |
| `full_source` | 以上 + 加密原件 | 允许授权审核人检查完整上下文 |

产品规则是默认 `evidence_pack` 并支持逐来源交互选择。

> 实现现状：CLI 目前接受显式 disclosures JSON，未提供时不上传任何来源正文（等价于 `metadata_only`），交互式逐来源选择待实现。以 `14-implementation-status.md` 为准。

高风险 Claim、权利和合规事实如果证据等级不足：

- 云端显示 `evidence_limited`。
- 禁止普通远程批准。
- 可上传 full-source，或由具备 reviewer 权限的人完成本地核验并提交签名 attestation、依据和时间。

## 12. Pull 与反馈应用

```bash
contentcloud pull feedback
contentcloud pull decisions
contentcloud pull approved
```

- feedback/decision 先下载到 `.contentcloud/inbox`，不直接修改业务文件。
- ApprovedSnapshot 下载到只读 `.contentcloud/cache/approved/<snapshot-id>`。
- 当前 pull 只落盘不可变 bundle，不自动应用反馈，也不推进业务正文。
- 后续 `review-feedback-apply` Skill 才会读取指定基线和反馈，在新 LocalRunContext 中生成修订。
- 后续应用流程必须先比较本地文件 hash；冲突不得自动选择云端或本地版本，用户处理后重跑 lint。

## 13. 云端编辑边界

当前 Web 允许：

- 对指定对象/字段/镜头提出修改要求。
- 批准当前 SubmissionRevision 并创建 ApprovedSnapshot。
- 查看 revision 摘要、结构化正文、来源披露、hash 和审核记录。

独立批注编辑、责任人/截止时间和字段级 diff 属于后续审核协作增强。客户审批链接（ReviewGrant + OTP）目标是绑定具体 SubmissionRevision，属于波次一的单轨收敛改造，见 `03-domain-and-data-model.md` §2.1。

Web 不允许直接改 Submission 正文、知识值、Brief 内容、口播或镜头文本。所有内容修改回到本地，形成新的 SubmissionRevision。

## 14. 模板升级

```bash
contentcloud workspace upgrade --dry-run
contentcloud workspace upgrade
```

以上 upgrade 命令尚未实现。目标流程为：下载新签名 manifest -> 比较 template.lock -> 安全替换未修改 generated 文件 -> 对 managed_merge 生成三方 diff -> 保留 seed_once -> 用户确认 -> 运行 doctor/lint -> 更新 lock。

升级失败不改变旧 lock。平台强制安全升级可以阻止新的 publish/Automation，但不能静默修改客户工作区。

## 15. Automation 工作区隔离

以下是后续显式启用 Automation 的目标命令，当前尚未实现：

```bash
contentcloud automation device enable --project <id>
```

Daemon 注册 workspace ID、模板版本、可用来源 hash 和业务 capability，不上传路径。每个 TaskRun 创建隔离临时工作区，从 ApprovedSnapshot 和允许的本地来源读取，输出单独保存并自动 publish Submission。

Automation 不直接写用户当前工作目录。用户可通过 CLI 将通过审核的结果 pull 回主工作区。

## 16. 工作区状态与诊断

```bash
contentcloud workspace status
contentcloud workspace doctor
```

当前状态显示项目绑定、模板/Skill/MCP 版本、Agent 可用性、同步状态、披露默认策略和 Automation 是否启用。来源数量、LocalRunContext、待拉取反馈和 workspace diff 待对应本地领域能力完成后增加。

doctor 区分本地结构、Agent/MCP、云端连接和 Automation；普通创作所需检查通过时，不因后台 Daemon 未启用而整体失败。
