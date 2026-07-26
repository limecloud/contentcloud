# 技术选型 ADR：Go 控制面与单二进制客户端

> 状态：Accepted  
> 日期：2026-07-25  
> 范围：ContentCloud V1 控制面、CLI/Daemon、Web、Worker 与契约工具链

## 1. 决策

采用以下技术边界：

| 层 | 选择 | 责任 |
| --- | --- | --- |
| 云端控制面 | Go 1.24 模块化单体 | CLI Gateway、领域服务、设备租约、审批、审计、对象存储授权、队列协调 |
| CLI / Daemon | Go 1.24 单二进制 | 安装连接、安全凭据存储、long-poll、Agent/Renderer Adapter、Artifact 同步、稳定 JSON |
| Web | React + TypeScript；Vite 构建 SPA | 项目工作台、知识治理、Brief、审核与客户审批；只调用同源 BFF |
| Web BFF | Go 控制面同源路由 | session、CSRF、页面聚合 DTO；不是公开 SDK |
| 数据库 | PostgreSQL 17 + `pgx` + `sqlc` + `goose` | 在线事务事实源、RLS、显式 SQL、迁移 |
| 异步任务 | PostgreSQL 队列 + Go Worker | 摄取、OCR 编排、确定性策略校验、导出；V1 不引入 Redis/Kafka |
| 文件 | S3 兼容对象存储 | 来源、不可变快照、Artifact、Rendition 与 Hosted Preview blob |
| 契约 | OpenAPI 3.1 + JSON Schema 2020-12 | Go 服务端、Go CLI 与 TypeScript Web 的唯一传输契约源 |

服务端继续保持 zero-exec：Go 的选择不改变边界，云端仍不运行 LLM、用户代码、客户端插件或源码构建。

## 2. 为什么不是 TypeScript 全栈

TypeScript 全栈能更快复用 Loopany，但 ContentCloud 的长期难点不是页面开发，而是一个需要持续运行、跨平台分发并安全处理租约、文件 hash、进程和凭据的客户端。若 CLI/Daemon 用 Go、服务端仍用 TypeScript，会形成两套核心协议实现和两套运行时运维模型。

Go 统一控制面与客户端的收益：

- 编译为无 Node.js 运行时依赖的 macOS/Linux/Windows 单文件，适合非技术用户安装和后台运行。
- `net/http`、流式文件、SHA-256、并发上传、long-poll、信号和子进程管理成熟直接。
- 服务端和客户端可共享纯 Go 的协议 envelope、错误码、内容寻址和状态机测试夹具。
- 静态类型与显式错误适合 Agent 依赖的稳定机器接口。
- 单体部署和资源占用可控，符合 V1 的 KISS 原则。

代价是 Web 不再与服务端共享 TypeScript 类型。通过契约生成而非手写复制解决：OpenAPI 生成 TypeScript client，JSON Schema 生成/校验 Script Package 和 Task Contract；领域模型不跨语言共享。

## 3. 飞书 CLI 与 Loopany 的验证

### 3.1 从飞书官方 CLI 学习

基于 `larksuite/cli` v1.0.77（commit `a7865cd0a7416655535517a2a630848fde318761`）源码核对：它使用 Go 1.23+、Cobra 和 GoReleaser 生成跨平台单二进制，npm runner 负责下载、校验和执行。ContentCloud 采用以下模式，完整对标见 [11-feishu-cli-benchmark.md](11-feishu-cli-benchmark.md)：

1. npm 安装器只识别平台、下载并校验 Go 二进制后执行，不承担业务逻辑；V1 校验 SHA-256，正式发布再增加签名校验。
2. `contentcloud doctor --json` 即使未登录也能返回安装、版本、安全凭据存储、网络和 Agent capability 状态。
3. stdout 只输出稳定 JSON；进度、提示和诊断写 stderr；错误有 `type/subtype/code/retryable/hint`。
4. 写命令支持 `--dry-run`；命令标注 `read/write/high-risk-write`，高风险写入缺少 `--yes` 时以结构化错误和 exit 10 停止；分页有硬上限。
5. `contentcloud schema <command>` 暴露 CLI 命令契约，而不是暴露底层 HTTP API。
6. Skill 内容随二进制发布并可通过 `skills list/read/status` 自省；Agent 先使用产品级短命令，不依赖 raw API。
7. 凭据策略按平台验收：macOS Keychain、Windows Credential Manager/DPAPI、Linux Secret Service；不把飞书 CLI 在 Linux 上的本地加密文件方案误称为系统 Keychain，也不保存明文凭据。
8. 用户 CLI OAuth 支持 `--no-wait` 发起和 `--device-code` 恢复，避免 Agent 在同一轮阻塞；项目首次设备绑定仍使用 Web 签发的 `connect-key`，两种流程不混用。
9. 多项目 context 必须显式解析，不能以“最近使用项目”作为静默写入目标。

### 3.2 从 Loopany 学习

采用项目页生成 `connect-key`、本机 `up`、等待心跳、单二进制 daemon/callback、按凭据类型统一 dispatch、租约心跳，以及 `manifest -> need_hashes -> blob -> complete`。不采用持续目录 watcher、cron/evolve、自修改 Agent 或云端下发代码。

## 4. 安装与分发

Web 在项目创建成功后展示：

```bash
npx --yes @goodvision/contentcloud@latest up \
  --server-url https://app.contentcloud.cn \
  --connect-key cck_xxx
```

npm wrapper 的职责仅为：

1. 根据 OS/arch 下载 GitHub Release 或国内镜像的归档。
2. 校验嵌入 npm 包的 SHA-256；正式版再校验签名。
3. 安装到用户级应用目录，并调用真实 `contentcloud up`。
4. 注册用户级后台服务；不得要求管理员权限。

V1 试点正式支持 macOS arm64/x64；CI 同时构建 Linux x64/arm64。Windows arm64/x64 构建进入兼容测试，达到安全凭据存储、服务管理和 Agent Adapter 验收后再标记正式支持。安装失败必须给出手工下载路径，不能让项目卡在无法恢复的等待态。

## 5. 云端模块化单体

```text
cmd/
├── contentcloud-server/      # Web BFF + CLI Gateway + health
├── contentcloud-worker/      # 确定性异步任务
└── contentcloud/             # CLI / Daemon
internal/
├── identity/                 # session、RBAC、ReviewGrant、token
├── projects/                 # BrandProject 与 onboarding
├── knowledge/                # 来源、证据、知识与权利
├── production/               # Brief、Script、Review、Export
├── execution/                # Device、ConnectSession、Run、Lease
├── artifacts/                # manifest、blob、rendition、preview
├── cligateway/               # dispatch、scope、JSON envelope
├── web/                      # BFF handlers 与静态 Web 托管
└── platform/                 # Postgres、S3、queue、mail、telemetry
web/                          # React + TypeScript SPA
contracts/                    # OpenAPI、JSON Schema、golden fixtures
```

模块通过 Go 接口和领域方法协作，不拆微服务。HTTP handler、CLI dispatch 和 Worker 都调用同一 application service，不能复制授权与状态转换逻辑。

## 6. 服务端库选择

| 关注点 | 选择 | 理由 |
| --- | --- | --- |
| HTTP | Go `net/http` + `chi` | 小而透明，路由和中间件成熟，不引入全栈框架魔法 |
| SQL | `pgx` + `sqlc` | SQL/RLS 可审计，编译期结果类型，避免 ORM 隐式查询 |
| 迁移 | `goose` | 简单 SQL migration，支持 CI 与启动前检查 |
| 日志 | `slog` JSON | 标准库、结构化、少依赖 |
| 指标/追踪 | OpenTelemetry | CLI Gateway、Worker、对象存储可统一 trace |
| CLI | Cobra + 分平台 credential adapter | 命令树稳定；凭据实现与降级策略必须逐平台威胁建模，不能由单一库名代替验收 |
| 契约 | `kin-openapi` + JSON Schema validator | 请求/响应与产物 schema 可自动验证 |

不引入 gRPC、GraphQL、Kubernetes、服务网格、事件溯源或插件 VM。V1 的部署单元只有 Web/Control Plane、Worker、Postgres、对象存储和 Preview Edge。

## 7. 关键质量门禁

- `go test -race ./...`、`go vet`、`golangci-lint`、OpenAPI breaking-change 检查。
- 生成的 TypeScript client 必须无未提交差异，禁止手改生成文件。
- 服务端、CLI 和 Web 共享同一 golden fixtures；状态机与错误码必须 contract test。
- CLI 从 `/tmp` 等任意目录执行 `--help`、`doctor --json`、未登录错误和离线模式。
- macOS 安装、升级、重复 `up`、设备撤销、后台服务重启和 Keychain 读写做端到端测试；Linux 验证 Secret Service 存在与缺失两条路径。
- Go pprof 和数据库慢查询只进入受限运维面，不暴露客户正文或 token。

## 8. 重新评估触发器

仅在以下事实出现时重评，而不因团队偏好频繁换栈：

- Go 无法可靠驱动目标 Agent/Renderer，且外部进程 Adapter 不能解决。
- Web BFF 聚合复杂度显著超过领域服务，导致前端交付持续受阻。
- PostgreSQL 队列在真实试点负载下无法满足派发与恢复 SLO。
- Windows 成为首要试点平台且 Go 用户级服务/凭据方案无法通过验收。

在触发前不新增 Node.js 常驻 Daemon、独立 API 服务或第二套公共 SDK。
