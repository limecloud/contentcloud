# ContentCloud Agent Plugin 架构

本文是 ContentCloud 插件化的架构和运行手册。它定义标准插件包、ContentCloud 控制面、设备本地存储以及 Codex/Claude Code 宿主之间的边界。目标不是保留旧插件安装方式，而是让一个不可变的 Agent Plugins 包在正式宿主中可验证、可安装、可升级、可诊断和可撤回。

## 1. 结论

ContentCloud 的插件发布物只有一种：Agent Plugins 1.0.0 标准包。标准包的入口固定为根目录 `plugin.json`，MCP 文件固定为根目录 `mcp.json`，Skill 固定发现于 `skills/<skill-name>/SKILL.md`。包以内容摘要作为不可变身份，不以 Git 仓库、分支或 Marketplace 文件作为身份。

宿主兼容层保留为 `Plugin Host Adapter`，但它是一个窄的设备安装端口，不是第二套插件模型，也不是 `AgentHarnessAdapter` 的扩展。共享核心负责解析、计划、确认、CAS 锁、回执、状态机和回滚；宿主实现只负责调用真实 CLI、生成宿主私有投影、检查原生状态。

当前宿主：

| 宿主 | 已验证版本 | 宿主私有投影 | 真实安装入口 |
| --- | --- | --- | --- |
| Codex | `0.147.0` 及以上 | Store 根目录内 `.agents/plugins/marketplace.json` | `codex plugin marketplace add` + `codex plugin add` |
| Claude Code | `2.1.220` 及以上 | `.claude-plugin/plugin.json`、`.claude-plugin/marketplace.json`、`.mcp.json` | `claude plugin marketplace add` + `claude plugin install` |

标准包不包含 `.codex-plugin/`、`.claude-plugin/`、`.mcp.json` 或宿主专属 `agents/openai.yaml`。这些文件属于投影层；把它们放进发布包会让宿主私有格式污染跨宿主 Artifact。

## 2. 边界和所有权

```text
release source
    -> Agent Plugins package (plugin.json + skills/ + mcp.json + ContentCloud extension)
    -> verifier: schema + safety + claims + signature + digest
    -> local CAS Store: packages / data / receipts / hosts / locks
    -> Plugin Host Adapter
         -> Codex NativeHost: local Marketplace projection + real Codex CLI
         -> Claude NativeHost: local Marketplace projection + real Claude CLI
    -> receipt + environment lock + diagnostics
```

| 层 | 唯一职责 | 不拥有的职责 |
| --- | --- | --- |
| Agent Plugins Loader | 读取标准 manifest，发现 Skill/MCP，计算摘要 | 宿主安装、租户权限、登录 |
| Claims/Policy | 校验 ContentCloud 扩展、权限、数据流、费用和支持信息 | 改写标准组件 |
| Registry/Control Plane | 发布经过签名的 Artifact 元数据、生命周期和撤回状态 | 设备 Marketplace 配置 |
| Local CAS Store | 保存不可变包、插件数据、安装回执、锁和宿主投影 | 云端业务数据 |
| `pluginhost.Adapter` | 计划、确认、串行化安装、回滚、回执 | 宿主原始命令语义 |
| Codex/Claude `NativeHost` | 调用真实 CLI，物化和检查宿主投影 | ContentCloud 业务规则 |
| `AgentHarnessAdapter` | 启动/恢复 Agent Session 和事件 | 插件安装事务 |
| Workspace Bootstrap | 将已安装宿主和已授权工作区绑定起来 | 自行选择插件来源或绕过确认 |

Adapter 不能接收 Registry 的 repository/ref 决策，也不能接收租户 token；它只接收已经验证的 `plugin.Package` 和固定的 `ReleaseRef`。

## 3. 标准包格式

### 3.1 目录

```text
contentcloud-video-production/
├── plugin.json
├── mcp.json
├── skills/
│   └── contentcloud-workspace/
│       └── SKILL.md
├── run.zhongcao.contentcloud/
│   ├── claims.json
│   └── RUNBOOK.md
└── contracts/
    └── ...schema.json
```

`contracts/` 中的引用必须是包内、以 `contracts/` 开头的相对路径。包不能包含符号链接、绝对路径、越界路径或非普通文件。Loader 默认限制：4096 个文件、单文件 8 MiB、总包 64 MiB、路径深度 32。

### 3.2 `plugin.json`

最低可互操作 manifest：

```json
{
  "$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
  "name": "contentcloud-video-production",
  "version": "0.21.0",
  "description": "Governed local-first content production workflows.",
  "author": {"name": "GoodVision"},
  "license": "Apache-2.0",
  "extensions": {
    "run.zhongcao.contentcloud": {
      "claims": "./run.zhongcao.contentcloud/claims.json"
    }
  }
}
```

标准 schema 允许的顶层字段是 `$schema`、`name`、`version`、`description`、`author`、`homepage`、`repository`、`license`、`keywords` 和 `extensions`。`skills`、`mcpServers`、`hooks`、`apps` 等是特定客户端扩展语义，不应写进跨宿主标准 manifest；ContentCloud 依照 Agent Plugins 1.0.0 默认发现 `skills/` 和 `mcp.json`。

### 3.3 `mcp.json`

```json
{
  "$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
  "mcpServers": {
    "contentcloud-local": {
      "type": "stdio",
      "command": "npx",
      "args": ["--yes", "@limecloud/contentcloud@0.21.0", "mcp", "serve"],
      "cwd": "${PLUGIN_ROOT}"
    }
  }
}
```

标准 MCP 支持 `stdio`、`streamable-http` 和 `sse` 三种 transport。当前 ContentCloud 首发只允许本地 stdio；HTTP/SSE 条目可以被读取并标记为 `unsupported`，不能被宿主安装事务静默启用。`PLUGIN_ROOT` 和 `PLUGIN_DATA` 是标准变量，不能出现在 `env` 的键名中，路径必须是包内相对路径或以这两个变量为根。

### 3.4 ContentCloud 扩展

标准协议不解释 `extensions` 的命名空间。ContentCloud 使用反向域名命名空间 `run.zhongcao.contentcloud`，仅声明一个包内 claims 路径。Claims 固定声明：插件身份、请求能力、权限、数据流、费用、宿主要求和支持 Runbook。它不能授权自己，也不能改变标准 Skill/MCP 位置。

## 4. 身份、摘要和可信发布

一个 Release 的身份是三元组：

```text
(plugin_id, version, package_digest)
```

摘要由排序后的包内文件路径、文件类型和内容计算 SHA-256。文件权限中的 executable 位参与摘要。相同版本对应不同摘要必须阻断，不能通过改 Marketplace、Git ref 或缓存目录“修复”。要发布不同内容，必须提升版本或重新生成合法 Release。

控制面至少保存：

| 字段 | 用途 |
| --- | --- |
| `id/kind/version` | 发布身份和能力类型 |
| `digest` | 不可变包内容摘要 |
| `signature` | 发布者签名和信任根验证 |
| `compatible_profiles` | 环境 Profile 匹配 |
| `permissions/data_flow/cost` | 用户确认和租户策略 |
| `lifecycle/revocation` | 发布、弃用、撤回 |
| `evaluation` | 可复核的评测报告 |

Registry 是云端可信元数据目录，不是设备安装源。设备安装必须拿到包 Artifact，并在本地重新计算摘要；服务端返回的摘要、包 manifest、claims 和签名必须全部一致。

撤回规则：新安装和新任务拒绝 `revoked`；历史回执和审计仍可读取。高风险撤回阻止新任务；普通撤回可以继续展示历史结果，但不能升级或重新激活。

## 5. Local Store 与回执

默认 Store 位于本机 ContentCloud 配置目录下的 `plugins/`，也可由 `CONTENTCLOUD_PLUGIN_STORE` 指定：

```text
plugins/
├── packages/<plugin>/<version>/<digest>/     # 不可变 CAS 包
├── bundles/<plugin>/<version>/               # 正式二进制内嵌包的物化缓存
├── data/<host>/<plugin>/<version>/<digest>/ # PLUGIN_DATA
├── hosts/<host>/                             # 宿主投影和宿主状态
├── receipts/<host>/<plugin>.json             # 安装回执
├── locks/<host>.lock                         # 宿主级 CAS 锁
└── staging/<installation-id>/                # 未提交暂存区
```

回执记录 schema、安装 ID、宿主、Release 三元组、Plan Digest、已安装组件、NativeChange、时间和前一份回执。回执不是信任源；每次 `Detect` 仍必须读取真实宿主状态。回执用于恢复、审计和只影响本次安装的回滚。

`PLUGIN_DATA` 永远指向宿主/插件/版本/摘要隔离目录，不能指向工作区或包目录。包目录只读；宿主执行需要写数据时使用 `PLUGIN_DATA`。

## 6. Plugin Host Adapter

### 6.1 为什么保留

Codex 与 Claude Code 的真实插件生命周期已经不同：命令名、JSON 输出、Marketplace schema、安装 scope、缓存字段和私有配置位置都不同。把它们硬塞进一个“通用 CLI”会把差异泄漏到 Loader、Runtime 和业务层；保留一个窄 Adapter 端口可以隔离变化，同时让共享事务逻辑只有一份。

### 6.2 端口

```go
type NativeHost interface {
    ID() HostID
    Capabilities(context.Context) (Capabilities, error)
    Detect(context.Context, HostTarget) (State, error)
    Apply(context.Context, NativeApply) (NativeChange, []InstalledComponent, error)
    Remove(context.Context, NativeRemove) (NativeChange, error)
    Rollback(context.Context, NativeChange) error
    Commit(context.Context, NativeChange) error
}
```

共享 `Adapter` 只做六件事：验证包、生成计划、校验确认、取得宿主锁、调用 NativeHost、保存/恢复回执。它不能按宿主 ID 分支，也不能返回宿主原始 JSON 作为公共模型。

## 7. Codex NativeHost：真实运行约束

以下事实来自 `/Users/coso/Documents/dev/rust/codex` 当前源码和本机真实 CLI 冒烟，不是猜测：

1. Marketplace 命令是 `codex plugin marketplace add|list|upgrade|remove`。
2. 插件命令是 `codex plugin add|list|remove`；安装选择器为 `PLUGIN@MARKETPLACE` 或 `--marketplace`。
3. `--json` 输出字段使用 camelCase，例如 `pluginId`、`marketplaceName`、`installedPath`、`marketplaces`、`installed`、`available`。
4. 本地 Marketplace 必须在 `<root>/.agents/plugins/marketplace.json`；Codex 的 Marketplace Add 返回实际 `installedRoot`，它必须位于 ContentCloud Store 根目录。
5. Codex Agent Plugins 解析根 `plugin.json`，默认从 `./skills` 和 `./mcp.json` 发现组件；标准包不需要 `.codex-plugin/plugin.json`。

ContentCloud 的 Codex 流程：

```text
Store packages/<id>/<version>/<digest>
  -> Store 根目录生成 .agents/plugins/marketplace.json
  -> codex plugin marketplace add <StoreRoot> --json
  -> codex plugin add <id>@contentcloud --json
  -> codex plugin list --marketplace contentcloud --json
  -> codex mcp list --json
```

投影中的 source 必须是 `local`，path 必须是 `./packages/<id>/<version>/<digest>` 这类 Store 内相对路径。发现同名但不是该 Store 根、不是 local 或来源路径漂移时，状态为 `blocked/repair_required`，不得接管用户已有 Marketplace。

当前已验证最低版本为 `0.147.0`。安装完成必须新建对话；Codex 不会把新 Skill/MCP 热加载到旧会话。

## 8. Claude Code NativeHost：真实运行约束

当前验证版本为 `2.1.220`。Claude Code 的私有投影不进入标准包：

```text
<Store>/hosts/claude/marketplace/
├── .claude-plugin/marketplace.json
└── plugins/<plugin>/<digest>/
    ├── plugin.json                 # 从标准 manifest 生成的 Claude 私有 manifest
    ├── .claude-plugin/plugin.json  # 私有入口
    ├── .mcp.json                   # 从标准 mcp.json 翻译
    └── .contentcloud-plugin-host.json
```

真实命令是：

```text
claude plugin validate <projection> --strict
claude plugin marketplace add <projection-root> --scope user
claude plugin install <plugin>@contentcloud --scope user
claude plugin update <plugin>@contentcloud --scope user
claude plugin uninstall <plugin>@contentcloud --scope user --keep-data
claude plugin enable|disable <plugin>@contentcloud --scope user
claude plugin list --json
```

Adapter 只把标准包内容复制到 digest 目录，再写 Claude 私有 manifest、Marketplace 和 `.mcp.json`，然后调用 `validate --strict`。`PLUGIN_ROOT` 映射为 `CLAUDE_PLUGIN_ROOT`，`PLUGIN_DATA` 映射为 `CLAUDE_PLUGIN_DATA`。安装 scope 固定为 user；检测到同名非 ContentCloud Marketplace 时阻断。

## 9. 安装状态机

```text
absent
  -> staged       (生成本地计划，尚未改宿主)
  -> ready        (Apply 完成且 Detect 复核)
  -> repair_required / installed (版本、投影或组件漂移)
  -> blocked      (宿主不支持、来源不受管、签名/摘要冲突)
  -> removed      (Remove 完成且 Detect 复核)
```

标准流程：

1. `Load`：读取根 manifest、Skill、MCP、Claims，拒绝路径和文件安全问题。
2. `Verify`：验证 Registry 条目、签名、撤回状态和包摘要。
3. `Detect`：读取真实宿主 JSON/命令结果，并生成 generation。
4. `Plan`：计算确定性 Plan Digest，展示动作、权限、数据流、费用和新会话要求。
5. `Confirm`：只接受用户确认的同一个 Plan Digest。
6. `Stage`：复制到 staging，重新 Load 并核对摘要。
7. `Apply`：取得宿主级锁，提交 CAS 包，调用 NativeHost 生成投影并安装。
8. `Verify`：再次 Detect，所有声明的 Skill/MCP 必须达到 ready。
9. `Receipt`：原子保存回执；随后写 Environment Lock 和 workspace handoff。

Apply 失败时，只回滚本次 NativeChange；NativeChange 为空时不执行回滚。回执保存失败或 Commit 失败也必须恢复上一份回执。宿主锁按 host，而不是按 plugin ID，避免两个插件并发改写同一个宿主配置。

## 10. 升级、删除和撤回

升级不是覆盖目录：新摘要先进入 CAS，生成新 Plan，确认后由 NativeHost 原子替换投影。旧包和旧 `PLUGIN_DATA` 在回执仍可恢复前保留。相同版本不同摘要直接阻断。

删除按回执中的 Release 操作，不接受只给插件名的模糊删除。NativeHost 必须先卸载宿主插件，再移除由本次安装创建的 Marketplace 条目；如果 Marketplace 还包含其他插件，只更新投影，不删除整个 Marketplace。

撤回由服务端策略阻断新安装/新运行；本机不能通过删除回执、改 manifest 或切换 Marketplace 绕过撤回。诊断输出只包含版本、状态、摘要和错误码，不上传工作区文件、token、绝对路径或宿主原始配置。

## 11. Bootstrap 与会话边界

`contentcloud bootstrap plan` 是只读的，输出包括宿主、标准包 Release、Host Plan、工作区状态和将发生的云端动作。`bootstrap apply` 必须同时满足浏览器设备授权、同一个 `plan_id`、`--accept`、插件安装成功、Workspace Doctor 通过和 `workspace.register` 成功。

安装或升级后必须开启新宿主会话。Codex 支持 `codex://new` 深链或 `codex app <workspace>`；Claude 当前只返回恢复提示，不假装已经打开桌面。新会话第一步调用 `workspace_context`，校验 `project_id`，不能从旧聊天重建状态或扫描其他目录。

## 12. 并发、故障和诊断

同一宿主所有安装、升级、删除和回滚共享一个 CAS 锁。计划包含 `observed_generation`；锁内 generation 变化会返回 `PLUGIN_HOST_PLAN_STALE`，调用方必须重新 Plan。网络、CLI 不存在、JSON 结构变化、版本过低和私有配置污染分别使用可诊断错误码，不能统一吞成“插件安装失败”。

最小诊断字段：

```json
{
  "host": "codex",
  "plugin_id": "contentcloud-video-production",
  "version": "0.21.0",
  "package_digest": "sha256:...",
  "state": "repair_required",
  "error_code": "CODEX_PLUGIN_VERIFY_FAILED",
  "new_session_required": true
}
```

禁止字段：`Authorization`、device/workspace token、Cookie、工作区绝对路径、MCP env 的敏感值、宿主原始 stderr 中的秘密。

## 13. 发布和验证门禁

发布流水线顺序固定：

```text
build package -> validate standard schema -> validate claims -> digest
  -> sign release metadata -> run host smoke -> publish Artifact + registry entry
```

必须验证：

- 根 `plugin.json` 的 Agent Plugins 1.0.0 schema 和名称规则。
- `mcp.json` 的 schema、transport、命令、路径和 reserved env。
- Skill frontmatter 与目录名一致，包内无符号链接。
- Package digest 可重复；签名 payload 包含 id/version/digest。
- Codex/Claude 最低版本检测和真实 CLI lifecycle smoke。
- 安装计划只读、确认防重放、宿主级锁、回滚和删除后 Detect。
- 标准包中不存在 `.codex-plugin`、`.claude-plugin`、`.mcp.json`、Git Marketplace 文件。
- `go test ./...`、`npm test`、发布脚本和治理脚本通过。

正式二进制通过 `agent_plugins.go` 内嵌标准包，运行时物化到 Store 后再走相同 Loader、摘要和 Host Adapter；不能依赖源码仓库路径、当前工作目录或开发机 Marketplace。

## 14. 实现索引

| 能力 | 代码 |
| --- | --- |
| 标准包 Loader | `internal/integration/plugin` |
| 内嵌发布包 | `agent_plugins.go`、`internal/integration/pluginbuiltin` |
| 共享安装事务 | `internal/integration/pluginhost/adapter.go` |
| 本地 CAS/回执/锁 | `internal/integration/pluginhost/store.go` |
| Codex NativeHost | `internal/integration/pluginhost/codex` |
| Claude NativeHost | `internal/integration/pluginhost/claude` |
| Bootstrap CLI | `internal/cli/bootstrap_commands.go`、`internal/cli/plugin_host.go` |
| Workspace Lock/Doctor | `internal/localworkspace` |
| Agent Session/Handoff | `internal/agentadapter` |
| 标准 schema 固定副本 | `contracts/agent-plugins/1.0.0` |

## 15. 设计决策

### D1：标准包优先于宿主格式

标准包只表达跨宿主能力；Codex/Claude 私有文件由投影器生成。这样删除或替换一个宿主不会改变 Artifact 摘要，也不会让另一个宿主解析无关字段。

### D2：Adapter 保留，但不扩大

Adapter 隔离真实 CLI 差异，避免共享核心出现 `host == codex/claude` 分支。它不承担认证、会话、业务状态或 Marketplace 信任决策。

### D3：本地投影是设备状态，不是发布事实

Marketplace 只存在于宿主需要的本地投影中。任何仓库内 Marketplace 文件都不能成为安装权威；安装事实以标准包摘要、宿主 Detect 和 Receipt 三者共同确认。

### D4：从当前状态重构，不保留旧 facade

仓库没有既有插件用户，因此旧 Git Marketplace 安装器、旧 `.codex-plugin` 发布格式和旧兼容分支在标准链路稳定后直接清理。历史评测报告可以保留为审计数据，但不得继续作为运行时入口。

## 16. 当前验收命令

```bash
go test ./internal/integration/plugin ./internal/integration/pluginhost ./internal/cli ./internal/localworkspace

CONTENTCLOUD_CODEX_PLUGIN_SMOKE=1 \
  go test ./internal/integration/pluginhost/codex \
  -run TestRealCodexAgentPluginLifecycle -count=1 -v

CONTENTCLOUD_CLAUDE_PLUGIN_SMOKE=1 \
  go test ./internal/integration/pluginhost/claude \
  -run TestRealClaudeAgentPluginLifecycle -count=1 -v

go test ./...
npm test
```

真实宿主冒烟测试默认跳过，只有显式设置对应环境变量才会执行。冒烟测试必须使用临时 `CODEX_HOME`/`CLAUDE_CONFIG_DIR` 和临时 Store，不能碰开发者现有宿主配置。

## 17. 不支持的事情

- 不把 Claude 私有 manifest 当成 Agent Plugins 标准。
- 不把 Codex Marketplace repository/ref 当成包身份。
- 不在工作区根目录直接写 Skill 或 MCP 配置。
- 不让插件自行修改租户 Content Capability、权限或注册表。
- 不在旧会话中声称新组件已加载。
- 不对不支持的 HTTP MCP、低版本宿主或被撤回 Release 自动降级。

当新增宿主时，先证明它有稳定的真实 CLI/协议和可测试的投影边界，再实现一个薄 NativeHost；不要修改标准包 schema，也不要把新宿主的私有字段加入共享 Adapter。
