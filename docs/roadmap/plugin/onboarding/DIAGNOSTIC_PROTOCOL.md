# 初始化诊断协议

状态：`方案待实施`。

本协议让 CLI、服务端、Web 和客服用相同的 stage、check 和 action 描述问题。它不允许服务端远程控制客户机器，也不允许客户端上传原始日志后让客服人工猜测。

## 1. 模型边界

`ConnectSession.state` 继续表示连接生命周期。每次本地尝试创建一个 `BootstrapAttempt` 子记录：

```text
ConnectSession
  -> BootstrapAttempt 1
       -> ProgressEvent 1..n
       -> DiagnosticSummary
  -> BootstrapAttempt 2（仅重试时）
```

一个 attempt 的 `stage` 可以失败或需要动作，但不能直接把 ConnectSession 标为 `connected`。只有 `workspace.register` 成功才能完成连接。

## 2. ProgressEvent

目标 JSON 结构：

```json
{
  "schema_version": "1.0",
  "attempt_id": "bat_...",
  "sequence": 7,
  "occurred_at": "2026-07-27T10:00:00Z",
  "stage": "plugin_installing",
  "status": "needs_action",
  "check_id": "codex.marketplace.source_conflict",
  "error_code": "CODEX_PLUGIN_INSTALL_BLOCKED",
  "action_id": "contact_support.marketplace_identity_conflict",
  "facts": {
    "platform": "darwin",
    "arch": "arm64",
    "cli_version": "0.6.0",
    "codex_version": "0.145.0"
  }
}
```

约束：

- `sequence` 在 attempt 内单调递增，服务端按 `(attempt_id, sequence)` 幂等接收。
- `status` 只能是 `started`、`passed`、`needs_action`、`failed`、`skipped`。
- `check_id`、`error_code` 和 `action_id` 必须来自版本化目录。
- `facts` 只能包含 Schema allowlist 中的非秘密标量或枚举。
- 客户拒绝确认属于 `needs_action` 或 `canceled`，不是系统错误。
- 离线或上报失败不能阻塞本地只读预检；授权 attempt 创建后可继续上报后续事件。

## 3. Stage 目录

| Stage | 含义 | 是否允许本地写入 |
| --- | --- | --- |
| `prerequisites` | Node、npx、macOS Keychain、平台和基本权限 | 否 |
| `codex_ready` | CLI、Desktop、登录和版本兼容性 | 否 |
| `network_ready` | 必要服务和来源可达 | 否 |
| `workspace_selected` | 目标路径经过冲突和权限检查 | 否 |
| `plan_ready` | 确定性 plan 和 `plan_id` 已生成 | 否 |
| `awaiting_confirmation` | 等待客户确认精确计划 | 否 |
| `plugin_installing` | 官方 Codex Marketplace/Plugin 事务 | 是，需确认 |
| `authorizing` | ContentCloud 浏览器设备授权 | 只写安全凭据存储 |
| `workspace_initializing` | 写 Workspace 受管文件和环境锁 | 是，需确认 |
| `doctor_running` | 验证 Plugin、Workspace、Environment | 只读 |
| `registering` | 向服务端登记已验证 Workspace | 云端写入 |
| `opening_desktop` | 打开新项目对话或生成恢复入口 | 否 |
| `complete` | 全部 required check 通过 | 否 |

## 4. Check 目录

首版至少定义以下稳定检查：

### 4.1 运行环境

| Check ID | 通过条件 | 默认 Action |
| --- | --- | --- |
| `runtime.platform.supported` | 平台位于已验收矩阵 | `guide.platform.requirements` |
| `runtime.node.available` | `node` 可执行 | `guide.node.install` |
| `runtime.node.version` | Node `>=20` | `guide.node.upgrade` |
| `runtime.npx.available` | `npx` 可执行且来自预期 Node 安装 | `guide.npx.repair` |
| `runtime.temp.writable` | 安全临时目录可写 | `guide.permissions.temp` |
| `runtime.credential_store.available` | 当前 macOS 用户的默认 Keychain 可用 | `guide.credentials.keychain` |
| `runtime.path.consistent` | Codex Desktop 子进程能看到 Node/Codex | `guide.path.desktop` |

### 4.2 Codex

| Check ID | 通过条件 | 默认 Action |
| --- | --- | --- |
| `codex.cli.available` | `codex` 可执行 | `guide.codex.cli_install` |
| `codex.cli.version` | 满足服务端兼容范围 | `guide.codex.upgrade` |
| `codex.desktop.available` | Desktop 可打开 | `guide.codex.desktop_install` |
| `codex.auth.ready` | 当前账号可使用目标 Plugin 能力 | `open.codex.login` |
| `codex.home.consistent` | CLI/Desktop 使用同一预期配置根 | `guide.codex.home_mismatch` |
| `codex.workspace.policy` | Workspace 管理策略允许 Plugin/MCP | `contact_admin.codex_policy` |

### 4.3 网络与来源

| Check ID | 通过条件 | 默认 Action |
| --- | --- | --- |
| `network.contentcloud.reachable` | Control Plane 健康检查通过 | `retry.network.contentcloud` |
| `network.npm.reachable` | 固定 npm 包可解析 | `guide.network.npm` |
| `network.marketplace.reachable` | 固定 Git source/ref 可读取 | `guide.network.marketplace` |
| `network.openai.reachable` | Codex 登录/Plugin 表面可用 | `guide.network.openai` |

### 4.4 Marketplace 与 Plugin

| Check ID | 通过条件 | 默认 Action |
| --- | --- | --- |
| `codex.marketplace.identity` | 名称、source、ref 精确匹配 | `repair.marketplace.install` |
| `codex.marketplace.source_conflict` | 不存在同名异源对象 | `contact_support.marketplace_identity_conflict` |
| `codex.plugin.identity` | Plugin ID、版本、来源匹配 | `repair.plugin.install` |
| `codex.plugin.source_conflict` | 不存在同名异源 Plugin | `contact_support.plugin_identity_conflict` |
| `codex.plugin.enabled` | Plugin 已启用 | `repair.plugin.enable` |
| `codex.plugin.new_session` | 变更后进入新会话 | `open.codex.new_workspace_chat` |

### 4.5 Workspace 与 Environment

| Check ID | 通过条件 | 默认 Action |
| --- | --- | --- |
| `workspace.path.safe` | 空目录或同项目受管目录 | `choose.workspace.directory` |
| `workspace.path.writable` | 目标目录可写 | `guide.permissions.workspace` |
| `workspace.binding` | 项目/Workspace 绑定可读且一致 | `repair.bootstrap.resume` |
| `workspace.template_lock` | 模板锁存在且可验证 | `repair.bootstrap.resume` |
| `workspace.managed_files` | 受管文件无缺失/漂移 | `review.workspace.managed_files` |
| `workspace.capability_routing` | 受管路由版本/hash 当前 | `repair.routing.update` |
| `environment.signature` | Manifest/Registry 签名可信 | `contact_support.environment_signature` |
| `environment.lock` | Lock 与签名环境完全一致 | `repair.environment.plan` |
| `workspace.registration` | 服务端已登记 verified Workspace | `retry.bootstrap.resume` |
| `desktop.new_chat` | 新对话打开并能读取 context | `open.codex.recovery_prompt` |

## 5. Action 目录

Action 是客户端已知处理器加服务端文案，不是服务端命令字符串：

```json
{
  "action_id": "guide.node.install",
  "kind": "open_guide",
  "title_key": "bootstrap.node.install.title",
  "body_key": "bootstrap.node.install.body",
  "doc_url": "https://contentcloud.example/help/bootstrap/node",
  "requires_confirmation": false,
  "recheck": ["runtime.node.available", "runtime.node.version", "runtime.npx.available"]
}
```

允许的 `kind`：

- `retry_check`：重跑只读检查。
- `open_guide`：打开 ContentCloud 受控教程。
- `open_browser_auth`：打开固定授权 origin。
- `open_codex`：打开官方 Codex 页面、设置或新对话。
- `choose_directory`：调用本地目录选择器，不接受服务端绝对路径。
- `run_managed_repair`：调用 CLI 内置、版本化的修复 handler，写入前必须展示 plan 并确认。
- `copy_fixed_command`：复制由当前 CLI 版本定义的固定命令模板。
- `create_diagnostic_bundle`：本地生成待预览的脱敏包。
- `contact_support`：提交支持码或已确认诊断包。

禁止 `shell`、`script`、任意 URL 下载执行和模型生成 action。

## 6. 服务端进度投影

ConnectSession API 可增加可选字段，不改变已有客户端：

```json
{
  "state": "waiting_for_computer",
  "progress": {
    "attempt_id": "bat_...",
    "stage": "prerequisites",
    "status": "needs_action",
    "step": 1,
    "step_count": 13,
    "check_id": "runtime.node.version",
    "action_id": "guide.node.upgrade",
    "support_code": "CC-7K4M-2Q9D",
    "updated_at": "2026-07-27T10:00:00Z"
  }
}
```

服务端只投影最新有效 sequence。Web 根据 `action_id` 读取本地化支持目录，不解析 CLI stderr。

CLI Gateway 能力：

- `bootstrap.authorization.start`：公开 session ID + 本地 PKCE challenge 创建 attempt。
- `bootstrap.authorization.complete`：Attempt Token + 本地 verifier 轮询并原子创建设备/Workspace。
- `bootstrap.progress.append`：幂等追加 allowlist 事件。
- `bootstrap.attempt.complete`：提交最终摘要；不能代替 `workspace.register`。
- `bootstrap.diagnostic.upload`：客户确认后上传脱敏诊断包。

浏览器批准/拒绝只通过登录态 BFF；CLI 持有 Attempt Token 和 verifier，Web 两者都不可见。服务端没有连接密钥、旧 `init/up` 或任意脚本下发入口。

## 7. 本地 DiagnosticSummary

CLI 无论能否联网都可以生成摘要：

```json
{
  "schema_version": "1.0",
  "attempt_id": "bat_local_...",
  "platform": "darwin",
  "arch": "arm64",
  "versions": {
    "node": "20.19.0",
    "contentcloud_cli": "0.6.0",
    "codex_cli": "0.145.0"
  },
  "checks": [
    {
      "check_id": "codex.plugin.identity",
      "status": "failed",
      "error_code": "CODEX_PLUGIN_VALIDATION_FAILED"
    }
  ],
  "managed_digests": {
    "environment_lock": "sha256:...",
    "plugin_spec": "sha256:..."
  }
}
```

允许收集：

- OS/架构和明确列出的组件版本。
- ContentCloud CLI 的退出码、稳定 error code 和 check 状态。
- ContentCloud 管理对象的 ID、版本、ref、digest 和签名状态。
- 网络目标的域名级结果和耗时分桶。
- 路径类型、权限布尔值、是否同一 Host/CODEX_HOME；绝对路径默认本地展示不上传。
- Rollback 是否完整以及 ContentCloud 管理对象名称。

禁止收集：

- Bootstrap Attempt Token、PKCE verifier、Workspace/Device Credential、OAuth token、Cookie。
- Prompt、完整对话、隐藏推理、剪贴板内容。
- 环境变量值、Shell 历史、完整 PATH。
- 客户文件名列表、正文、知识库、剧本、素材和输出。
- 非 ContentCloud 插件、Skills、MCP 的完整清单。
- 未脱敏 stdout/stderr。

## 8. 自动恢复边界

可以自动重试的动作：

- 短暂网络请求。
- 只读 Detect/Validate/doctor。
- 幂等进度上报。
- 已生成 handoff 的 Desktop 打开动作。

需要客户确认：

- 安装或升级 Marketplace/Plugin。
- 写入 Workspace、Environment Lock 或 `AGENTS.md` 受管块。
- 修复 ContentCloud 管理配置。
- 上传诊断包。

必须人工处理：

- 同名异源 Marketplace/Plugin。
- 签名无效、Registry 撤回或 Profile 不允许。
- 企业 Codex Workspace 策略阻断。
- 用户改动过的受管文件无法安全合并。
- Rollback 不完整。

## 9. 版本与兼容性

服务端维护只读兼容矩阵：

```text
platform
node_min/node_max_tested
contentcloud_cli exact/range
codex_cli min/max_tested
codex_desktop min/max_tested
plugin version/digest
marketplace source/ref
known_issue action_id
```

兼容矩阵可以改变“允许/阻止/警告”和教程内容，不能改变待执行命令、包来源或 digest。执行事实仍来自签名 Environment Manifest、Registry 和固定版本 CLI。
