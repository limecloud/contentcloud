# 客户初始化支持手册

状态：`手册完成，macOS Desktop 试点待执行`。

本手册面向客户成功、实施和技术支持。处理初始化问题时优先使用支持码和稳定检查编号，不要求客户发送完整终端日志、Prompt 或工作区文件。

## 1. 支持原则

1. 先确认客户处于哪个 stage，再处理第一个失败的 required check。
2. 不让客户反复创建 ConnectSession；先判断设备/Workspace 是否已创建以及能否 resume。
3. 不建议删除 `~/.codex`、重装所有插件或清空客户工作区。
4. 不让客户手工编辑 `config.toml`、Marketplace JSON 或 Environment Lock。
5. 不索取 Attempt Token、PKCE verifier、Workspace/Device Credential、环境变量值、完整 PATH、Prompt、对话或客户文件。
6. 同名异源、签名错误、策略阻断和不完整 rollback 直接升级，不指导覆盖。
7. 每完成一个动作都重新运行对应 check，不凭“看起来好了”继续。

## 2. 五分钟分诊

按顺序询问：

1. Web 上的 ConnectSession 状态和支持码是什么。
2. 当前 stage、`check_id`、`error_code` 是什么。
3. 客户是否已经确认过安装计划。
4. ContentCloud 浏览器授权是否已经完成。
5. 是否已经创建 `.contentcloud/project.yaml` 或得到 Workspace path。
6. 是否是 CLI 成功但 Desktop 没打开。

判断恢复入口：

| 现场 | 入口 |
| --- | --- |
| 尚未生成 plan | 修复 prerequisite 后重新 plan |
| plan 已生成但未确认 | 保留现场，让客户检查并确认当前 `plan_id` |
| `plan_id` stale | 重新 plan，不能复用旧确认 |
| Plugin 安装前失败且连接未消费 | 修复后重新 apply |
| 授权/连接已完成，后续失败 | 使用 `bootstrap resume` |
| Workspace doctor 已通过，仅 Desktop 未打开 | 使用返回的 path 和 recovery Prompt |
| 同名异源、签名或策略问题 | 停止变更并升级支持 |

## 3. 客户自检命令

仅在 Web 引导无法继续时提供。命令必须从当前版本支持目录复制：

```bash
node --version
npx --version
codex --version
npx --yes @limecloud/contentcloud@0.6.0 doctor --json
```

已有 Workspace：

```bash
npx --yes @limecloud/contentcloud@0.6.0 workspace doctor <workspace> --offline --json
```

连接已消费后的恢复：

```bash
npx --yes @limecloud/contentcloud@0.6.0 bootstrap resume <workspace> --accept --json
```

不得让客户把上述命令中的包名和版本替换成 `latest`，也不得让客户把输出中的凭据字段发送给客服。正式实现诊断包后，优先用诊断包替代粘贴 JSON。

## 4. 常见检查处理

### `runtime.node.available` / `runtime.node.version`

客户动作：

1. 按系统教程安装受支持 Node 20 LTS 或升级现有 Node。
2. 完全退出并重新打开 Codex Desktop，使其重新读取 PATH。
3. 重新执行 Node/npx 检查。

若普通终端通过、Codex 内仍失败，转到 `runtime.path.consistent`，不要继续重装 npm 包。

### `runtime.npx.available` / `runtime.path.consistent`

客户动作：

1. 对比普通终端和 Codex 会话中的 Node/npx 检测结果。
2. 重启 Desktop 后重试。
3. 仍不一致时提交只包含命令路径类型和版本的诊断包。

禁止让客户发送完整 PATH，因为它可能包含用户名、内部目录和工具信息。

### `runtime.credential_store.available`

客户动作：

1. 确认当前 macOS 用户存在默认 Keychain，并已在系统中解锁。
2. 不要改用明文 token 文件或把凭据粘贴进 Prompt。
3. 重新执行只读 preflight；仍失败时只提交错误码和支持码。

这个检查必须在浏览器授权前通过，避免服务端创建设备后才发现本机无法保存一次性凭据。

### `codex.cli.available` / `codex.desktop.available`

客户动作：

1. 使用 OpenAI 官方入口安装缺失表面。
2. 登录同一预期 ChatGPT/Codex 账号。
3. 运行 `codex --version`，再从 Desktop 打开一个本地目录验证。

### `codex.cli.version`

客户动作：按官方升级方式升级 Codex CLI，重启 Desktop，重新开始只读 plan。不要在已经确认的旧 `plan_id` 上继续 apply。

### `codex.auth.ready` / `codex.workspace.policy`

客户动作：

1. 在 Codex Desktop 确认登录账号与 Workspace。
2. 在 Plugins 页面确认 Plugin 功能可用。
3. 若被组织策略禁止，联系客户自己的 Workspace 管理员。

ContentCloud 支持不能绕过 OpenAI Workspace 策略。

### `network.contentcloud.reachable`

客户动作：

1. 用浏览器打开 ContentCloud 项目页确认服务可访问。
2. 重试当前只读检查。
3. 若服务端健康检查异常，支持人员查询同时间段服务状态，不要求客户重装。

### `network.npm.reachable` / `network.marketplace.reachable`

客户动作：确认企业代理、防火墙和 DNS 是否允许固定 npm/Git 来源。服务端应展示实际域名列表，不建议客户关闭全部安全软件。

### `workspace.path.safe` / `workspace.path.writable`

客户动作：

1. 优先选择新的空目录。
2. 不把初始化目标设为主目录、系统目录或已有无关项目根。
3. 权限不足时选择客户有写权限的位置，而不是扩大整个目录权限。

### `codex.marketplace.identity` / `codex.plugin.identity`

缺失或版本过旧时：重新生成 plan，客户确认后由官方 Codex CLI 安装/升级。

同名异源时：立即停止。记录预期和实际对象的名称、source 类型、ref 和 digest，绝不自动移除或覆盖现有对象。

### `environment.signature` / `environment.lock`

签名无效直接升级为产品或发布事故。不要让客户重新下载任意文件、跳过验证或手工修改 Lock。

Lock 漂移但签名有效时，先生成 Environment Preparation plan，展示变化并确认后修复。

### `workspace.managed_files` / `workspace.capability_routing`

若文件缺失且未被客户修改，可以由受管 repair plan 恢复。检测到客户修改时，先展示 diff 范围并让客户决定保留或迁移，不能覆盖。

### `workspace.registration`

本地 doctor 已通过时使用 `bootstrap resume` 重试云端注册。不得重新初始化、重新安装 Plugin 或创建新的 ConnectSession。

### `desktop.new_chat`

本地环境已经成功，不属于安装失败。向客户展示：

- 已验证 Workspace path。
- CLI 生成的不含秘密的 recovery Prompt。
- 在 Desktop 中打开该目录并新建对话的步骤。

## 5. 诊断包流程

1. 客户点击“生成诊断包”。
2. CLI 在本地生成 DiagnosticSummary，并运行秘密扫描。
3. Web/Desktop 展示字段摘要，不展示客户内容。
4. 客户明确确认上传。
5. 服务端返回短支持码，并把包关联到 ConnectSession/attempt。
6. 客服只通过支持码查看，不让客户重复粘贴日志。

若秘密扫描发现禁止字段，拒绝上传并指出字段类型；不要把可疑内容先上传再服务端清洗。

## 6. 升级条件

满足任一条件直接升级二线/工程：

- 同一个 check 按标准动作重试两次仍失败。
- `CODEX_PLUGIN_INSTALL_BLOCKED` 且原因是同名异源。
- Environment/Registry 签名不可信、digest 不匹配或条目被撤回。
- Codex Workspace 策略行为与官方文档或兼容矩阵不一致。
- Rollback 返回任何错误。
- CLI 与 Desktop 在同一 Host/用户下读取到不同 Plugin 状态。
- 诊断协议出现未知 `check_id`、`action_id` 或 Schema 版本。
- 客户数据可能被读取、上传或覆盖。

工程工单至少包含：

- 支持码、attempt ID、ConnectSession ID。
- 平台/架构和允许上传的版本信息。
- stage/check/error/action。
- ContentCloud 管理对象的预期/实际版本与 digest。
- 是否已授权、连接是否消费、Workspace 是否存在、doctor 是否通过。
- 自动 rollback 结果。

## 7. 客户沟通模板

### 环境缺失

> 当前没有进入安装阶段。检查发现本机缺少受支持的运行环境，请先完成页面中的这一项安装，然后点击“重新检查”。现有项目和 Codex 配置尚未被修改。

### 等待确认

> 只读计划已经生成，尚未修改本机。请核对目标目录、ContentCloud Plugin 版本和权限范围；确认后系统只应用这个 `plan_id` 对应的变化。

### 可以恢复

> ContentCloud 授权已经完成，不需要重新发起授权。系统将从现有 Workspace 恢复 doctor 和注册，不会重复绑定或删除业务文件。

### 需要升级支持

> 检测到现有 Codex 对象与 ContentCloud 预期来源不一致。为避免覆盖您的配置，自动安装已停止。请提交页面显示的支持码，我们会根据脱敏诊断结果继续处理。

## 8. 支持验收

- 一线支持可以只凭支持码识别 stage 和第一个失败 check。
- 标准问题都有单一 action 和复检条件。
- 客户不会被要求提供秘密、完整日志或客户文件。
- 已创建设备/Workspace 后的恢复不会要求重新授权。
- 同名异源、签名和策略问题不会被自动修复。
- 每个支持操作都能关联到 attempt 和版本事实。
