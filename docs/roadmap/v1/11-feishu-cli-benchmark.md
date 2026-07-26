# 飞书官方 CLI 源码对标与 ContentCloud 决策

> 调研快照：`larksuite/cli` v1.0.77，commit `a7865cd0a7416655535517a2a630848fde318761`，2026-07-24  
> 调研日期：2026-07-25  
> 用途：约束 ContentCloud CLI 的产品边界、命令契约、安装分发和 Agent 使用方式；飞书 CLI 不是运行时依赖

## 1. 结论

飞书 CLI 最值得学习的不是 200 多个 OpenAPI 命令，而是把 CLI 当作人类、脚本和 AI Agent 共同依赖的稳定产品接口。ContentCloud 采用其工程纪律，但保持更窄的领域边界：

1. Go + Cobra 单二进制承载全部业务逻辑，npm 只负责低门槛安装和启动。
2. 产品级短命令优先，`schema` 提供离线自省；Agent 不需要理解私有 HTTP。
3. JSON stdout、诊断 stderr、稳定错误 envelope 和稳定退出码共同构成机器契约。
4. 写操作支持 `--dry-run`，高风险写入必须经过结构化确认门禁。
5. `doctor`、更新、凭据安全存储和 Agent Skill 与主功能同等重要。
6. 不复制飞书的任意 OpenAPI 调用、超大命令树、bot/user 双身份或应用凭据初始化流程。

ContentCloud 的首次接入仍保持“服务端先创建项目，客户端后安装连接”。项目页签发的短期 `connect-key` 已经表达了用户选择的租户、项目和设备绑定意图，不需要再让非技术用户配置开发者应用或先做一次 CLI OAuth。

## 2. 源码事实

| 观察项 | 飞书 CLI 实现 | 对 ContentCloud 的含义 |
| --- | --- | --- |
| 运行时 | Go 1.23+、Cobra、跨平台单二进制 | 控制面与 CLI/Daemon 继续采用 Go 1.24 |
| 安装 | npm `bin` 指向 Node runner；postinstall 按 OS/arch 下载 release 归档 | `@goodvision/contentcloud` 只做平台选择、下载、校验和执行 |
| 供应链 | GoReleaser 生成 `checksums.txt`；安装器校验 SHA-256、限制初始下载 host 和重定向次数 | V1 必须校验 checksum；正式发布增加签名和最终下载来源校验 |
| 命令设计 | 快捷命令、类型化 API 命令、通用 API 三层；根 help 内置 Agent quickstart | 只采用产品级 noun/verb 命令和 `schema`，拒绝任意 raw write |
| 输出 | JSON 成功写 stdout，结构化错误写 stderr，退出码与错误类别绑定 | 固定 success/error envelope，禁止日志污染 stdout |
| 安全写入 | `read/write/high-risk-write` 风险标签，`--dry-run`，高风险操作需 `--yes` | ContentCloud 使用相同风险模型；确认缺失返回 exit 10 |
| 认证 | OAuth Device Flow，支持 `--no-wait` 发起和 `--device-code` 后续恢复 | 人工 CLI 登录采用 split flow；设备首次绑定继续使用 `connect-key` |
| 凭据 | macOS 使用 Keychain 保护主密钥；Linux 为本地 AES-GCM 文件；Windows 使用 DPAPI | 不把“飞书使用 Keychain”泛化为所有平台同一实现；逐平台做威胁模型和验收 |
| 诊断 | `doctor` 可关闭认证前置，并支持 `--offline` | 未登录、离线、网络受限时仍能输出完整分项诊断 |
| 自省 | `schema` 从命令目录返回参数、类型、权限、风险与示例 | ContentCloud schema 返回 CLI 契约，不返回底层路径/header |
| Skills | Skill 内容嵌入二进制，`skills list/read` 可由 Agent 读取；更新会同步 CLI 与 Skills | 内置 ContentCloud Skill 与 CLI 版本锁步，避免旧 Skill 调用新协议 |
| 更新 | CLI 自更新并处理 checksum、平台差异和失败回退 | Daemon 更新必须 staged、可校验、可回滚，不能原地破坏在线版本 |

源码入口：

- [larksuite/cli](https://github.com/larksuite/cli/tree/a7865cd0a7416655535517a2a630848fde318761)
- [npm 安装器](https://github.com/larksuite/cli/blob/a7865cd0a7416655535517a2a630848fde318761/scripts/install.js)
- [Doctor](https://github.com/larksuite/cli/blob/a7865cd0a7416655535517a2a630848fde318761/cmd/doctor/doctor.go)
- [Schema](https://github.com/larksuite/cli/blob/a7865cd0a7416655535517a2a630848fde318761/cmd/schema/schema.go)
- [错误契约](https://github.com/larksuite/cli/blob/a7865cd0a7416655535517a2a630848fde318761/errs/ERROR_CONTRACT.md)
- [内嵌 Skills](https://github.com/larksuite/cli/blob/a7865cd0a7416655535517a2a630848fde318761/cmd/skill/skill.go)

## 3. ContentCloud 命令分层

ContentCloud 不按 HTTP resource 自动生成几百个命令，只保留两层：

```text
产品命令：project / source / knowledge / brief / run / artifact / review / device
诊断出口：schema / doctor / request get（只读 allowlist）
```

典型 Agent 路径：

```bash
contentcloud doctor --json
contentcloud context show --json
contentcloud project resolve --name "金陵古都香" --json
contentcloud knowledge list --project prj_xxx --status approved --limit 20 --json
contentcloud run create --project prj_xxx --type script-generate --brief brv_xxx --dry-run --json
contentcloud schema run.create
```

禁止提供：

- `contentcloud api POST <path>` 之类的任意写入口。
- 允许 Agent 传入预签名 URL、Authorization header 或对象存储凭据的参数。
- 同时读取、修改、审批和发布的 `auto`、`fix`、`do-all` 命令。
- 云端下发 shell、prompt、插件正文、模型或 Renderer 实现。

## 4. 项目上下文解析

飞书 CLI 的 profile 思路适合身份和应用切换；ContentCloud 更需要避免跨品牌误操作。项目解析顺序固定为：

1. 显式 `--project prj_xxx`。
2. 当前进程的 `CONTENTCLOUD_PROJECT_ID`。
3. 当前目录向上查找 `.contentcloud/project.json` 中的非敏感绑定。
4. 当前身份只有一个已授权项目时自动选择。
5. 其余情况返回 `PROJECT_CONTEXT_REQUIRED`，列出可选项目的 ID 与名称，不使用“最近项目”静默兜底。

`contentcloud context use --project prj_xxx` 只写当前仓库的项目绑定，不保存 token；`context clear` 只移除绑定。设备 Daemon 领取任务时不使用 cwd context，而使用服务端签发的 `ProjectDeviceGrant`。

## 5. 两种认证流程不能混用

### 5.1 设备绑定

```text
Web 创建项目 -> Web 生成 cck_ -> npx ... up --connect-key cck_ -> 安全存储 dt_ -> 首次心跳
```

`cck_` 只能消费一次，只能创建设备和当前项目授权，不能列项目、读取知识或执行人工审批。

### 5.2 人工 CLI 登录

需要以用户 RBAC 执行 `project list`、`knowledge review` 等命令时使用 Device Flow：

```bash
contentcloud auth login --no-wait --json
contentcloud auth login --device-code dc_xxx --json
contentcloud auth status --verify --json
```

第一条命令必须立即返回 `verification_url`、`device_code`、`expires_at` 和轮询间隔。Agent 把链接交给用户后结束当前交互；用户完成授权后再恢复第二条命令，不能在同一轮阻塞等待。`ct_` 与 `dt_` 使用不同验证器、权限和撤销路径。

## 6. 输出与风险契约

成功：

```json
{
  "ok": true,
  "command": "run.create",
  "request_id": "req_xxx",
  "data": {"run_id": "run_xxx", "state": "queued"},
  "meta": {"dry_run": false}
}
```

确认缺失：

```json
{
  "ok": false,
  "command": "device.detach",
  "request_id": "req_xxx",
  "error": {
    "type": "confirmation",
    "subtype": "confirmation_required",
    "code": "CONFIRMATION_REQUIRED",
    "message": "撤销设备将中断该设备领取新任务",
    "retryable": false,
    "hint": "用户确认后使用原参数追加 --yes",
    "risk": "high-risk-write",
    "action": "device.detach"
  }
}
```

退出码冻结如下：

| Code | 含义 |
| --- | --- |
| 0 | 命令成功，包括空列表和 dry-run 成功 |
| 1 | 未分类内部错误 |
| 2 | 参数、Schema 或本地输入校验失败 |
| 3 | 未登录、凭据失效或安全存储不可用 |
| 4 | RBAC、项目授权或策略拒绝 |
| 5 | 网络、超时或服务暂不可达 |
| 6 | 版本、幂等或并发冲突 |
| 7 | 内容安全、Artifact 或 Task Contract 校验失败 |
| 10 | 高风险操作等待用户确认 |

Agent 只能在用户明确确认后，把 `--yes` 追加到原始 argv 重试 exit 10；不能重新拼接 shell 字符串，也不能把它当作网络错误自动重试。

## 7. 凭据与本地安全决策

- macOS：Keychain。
- Windows：Credential Manager 或 DPAPI 封装的用户级安全存储，完成威胁模型后锁定实现。
- Linux Desktop：Secret Service；没有可用安全存储时，设备 `up` 失败并给出修复提示，V1 不降级为明文 token 文件。
- CI：不复用设备 token。需要时单独设计短期、最小权限、仅环境注入的 automation session；不进入南京试点 V1 关键路径。
- 配置文件只保存 server URL、device ID、项目绑定和非敏感偏好；任何日志、doctor、错误和 telemetry 都不得输出 token。

飞书 CLI 在不同平台采用不同密钥保护策略，证明“跨平台 Keychain”不能只靠一个库名完成。ContentCloud 必须为 macOS 首发平台先做真实 Keychain、无 UI 会话、锁屏、拒绝授权和 sandbox 场景测试，再扩大支持矩阵。

## 8. 更新与 Skill 同步

`contentcloud update` 采用 staged update：

1. 下载 release manifest、归档、checksum 和签名。
2. 校验版本、OS/arch、host、SHA-256 和签名。
3. 在临时路径运行 `version --json` 与 `doctor --offline --json`。
4. 原子替换 CLI；Daemon 在当前 Attempt 结束后重启。
5. 首次启动失败时回滚上一版本。
6. 内嵌 Skill 与 binary 同版本发布；`skills status --json` 报告安装版本是否一致。

V1 不支持服务端向客户端强制推送未知二进制。服务端只能标记 `upgrade_available` 或 `upgrade_required`；安装与替换发生在本机并保留审计结果。

## 9. V1 验收清单

- 从空白 macOS 用户环境运行项目页命令，不要求预装 Go，不要求管理员权限。
- npm wrapper 对错误平台、未知 host、checksum 不匹配、归档穿越和中断下载 fail closed。
- `doctor --offline --json` 未登录可执行，所有检查都有 `pass/warn/fail/skip` 状态和修复提示。
- success 只写 stdout；error 只写 stderr；日志不能破坏 JSON；所有退出码有 golden tests。
- `schema` 无网络、无登录可读取内嵌命令契约。
- `--dry-run` 不产生服务端写入；exit 10 在未确认时不能绕过。
- 多项目无显式 context 时失败；任何命令不能因“最近项目”写入另一个品牌。
- `skills read` 的命令示例只使用当前版本存在的参数，CLI/Skill 不一致时给出可执行更新提示。
- 更新中断后旧 binary 和 Daemon 仍能启动，当前 Attempt 不丢失终态报告。
