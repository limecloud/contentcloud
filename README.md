# ContentCloud

ContentCloud 是面向 AI 内容营销团队的本地优先创作与云端治理系统。客户在本机 Codex、Claude Code 等成熟 Agent 中完成资料整理、知识工程、策略、Brief 和剧本；云端负责项目、不可变 Submission、人工审批、审计和可选 Automation 调度。

V2 交付 AI 视频就绪剧本，不生成图片、视频或成片。Hosted Preview 是低优先级能力。

## 核心边界

- 云端 zero-exec：不调用、代理、编排 LLM，不保存模型凭据，不执行客户上传代码。
- Agent、Skill、Renderer、脚本和 CI 的所有程序化服务通讯只经过 `contentcloud` CLI。
- 使用者工作台只访问同源 `/api/bff`，独立系统后台只访问 `/api/v1/admin`；这些内部 HTTP、token 和对象存储协议都不是公共 SDK。
- 客户先在 Web 创建项目，再把一次性 Agent Prompt 粘贴到 Codex 或 Claude，由 Agent 初始化本地工作区、项目级 Skills 和 MCP。
- 普通本地操作不创建 TaskRun；只有显式启用的远程、事件或定时 Automation 使用 Daemon。
- 客户审批绑定不可变 SubmissionRevision 内容哈希，不跟随“最新版本”。

## 仓库结构

```text
cmd/                    Go Server、Worker、CLI/Daemon
internal/               Domain、Application Service、Transport 与 Adapter
contracts/              OpenAPI 3.1 与 JSON Schema
migrations/             PostgreSQL schema、RLS 与 runtime role
skills/                  随 CLI 内嵌的本地 Agent Skill
packages/contentcloud/   零依赖 npm 校验安装器
web/                     React + TypeScript 工作台与客户审批页
docs/roadmap/v2/         V2 产品、架构、流程、安全、计划与实现状态
```

## 本地开发

要求 Go 1.24+、Node.js 22+、pnpm 10+。

```bash
pnpm install
make dev
```

开发脚本会同时启动 Go API（`http://localhost:8080`）和 Vite（`http://localhost:5173`），自动加载根目录 `.env`，并在退出时清理自己启动的子进程。打开 `http://localhost:5173` 使用租户工作台，或打开 `http://localhost:5173/admin/dashboard` 使用独立系统后台。

默认使用隔离的 Memory Store 和 `var/dev-data` 本地 Blob，不会连接 `.env` 中的数据库或 S3。运行 `./scripts/dev.sh --help` 可查看端口覆盖、真实数据库联调和显式清理占用端口等选项。需要验证构建后的前后端单体服务时使用 `make preview`。

开发模式下，演示账号默认具备平台管理员权限，并自动创建金陵古法线香演示项目；来源由内置确定性 Worker 处理。

CLI 示例：

```bash
./bin/contentcloud --json doctor --offline
./bin/contentcloud --json schema
./bin/contentcloud init --connect cck_xxx --target all --accept-project-config ./contentcloud-project
./bin/contentcloud workspace doctor ./contentcloud-project
./bin/contentcloud publish script --dry-run
./bin/contentcloud submission list
./bin/contentcloud pull approved --type script
./bin/contentcloud team invite editor@example.com --role editor --dry-run --json
./bin/contentcloud tenant switch "$TENANT_ID" --dry-run --json
./bin/contentcloud project templates --json
./bin/contentcloud project update "$PROJECT_ID" --row-version 1 --owner "项目负责人" --dry-run --json
./bin/contentcloud device connect-create "$PROJECT_ID" --dry-run --json
./bin/contentcloud device connect-cancel "$CONNECT_SESSION_ID" --dry-run --json
./bin/contentcloud skills list --json
./bin/contentcloud result import ./results.csv --project "$PROJECT_ID" --dry-run --json
./bin/contentcloud result batches --project "$PROJECT_ID" --json
./bin/contentcloud lineage show --project "$PROJECT_ID" --direction both --json
./bin/contentcloud lineage impact --project "$PROJECT_ID" --type script_version --id "$SCRIPT_VERSION_ID" --json
./bin/contentcloud audit list --project "$PROJECT_ID" --limit 50 --json
CONTENTCLOUD_BINARY_PATH=./bin/contentcloud node packages/contentcloud/bin/contentcloud.js --help
```

默认输出面向人类；Agent 与脚本必须使用 `--json`。成功 envelope 固定为 `{ok,command,request_id,data,meta}`，结构化错误只写 stderr。

## PostgreSQL 与 S3

生产模式至少配置：

```bash
export CONTENTCLOUD_DATABASE_URL='postgres://...'
export CONTENTCLOUD_AUTO_MIGRATE=1
export CONTENTCLOUD_PLATFORM_ADMIN_EMAILS='admin@example.com' # 多个邮箱用逗号分隔
export CONTENTCLOUD_S3_BUCKET='contentcloud'
export CONTENTCLOUD_S3_REGION='us-east-1'
export CONTENTCLOUD_S3_ENDPOINT='https://s3.example.com' # AWS S3 可省略
export CONTENTCLOUD_S3_ACCESS_KEY_ID='...'
export CONTENTCLOUD_S3_SECRET_ACCESS_KEY='...'
./bin/contentcloud-server
```

Worker 使用相同数据库与对象存储配置。v0.2 默认不安装或调用 ClamAV；开放不可信文件上传前，可显式启用恶意文件扫描：

```bash
CONTENTCLOUD_REQUIRE_MALWARE_SCAN=1 ./bin/contentcloud-worker
```

生产环境可按 [systemd 部署说明](deploy/systemd/README.md) 在无 Docker 模式下运行 Server 与 Worker，并由 Nginx 反向代理到 Server 的本地监听地址。

可用 `docker compose up --build` 启动 Server、Worker、PostgreSQL 和 MinIO 的完整本地拓扑。Compose 中的凭据只用于本机开发。

## 首次项目连接

1. 用户在 Web 创建项目。
2. 项目总览生成 10 分钟有效、单次使用的 `cck_`，并拼成不含登录态的 Agent Prompt：

```text
Fetch https://content.example.com/api/bootstrap and follow it to connect this ContentCloud project to Codex.

server-url: https://content.example.com
connect-key: cck_xxx
contentcloud-cli: npx --yes @limecloud/contentcloud@<pinned-version>
project: "品牌 / 单品"
```

3. 用户把 Prompt 粘贴到 Codex 安装会话。Agent 获取公开的 `/api/bootstrap` 协议并执行只读 `bootstrap plan`。
4. CLI 返回固定 Marketplace、Plugin、目标目录变化和确定性 `plan_id`；用户确认该计划后，Agent 才能把同一个 `plan_id` 传给 `bootstrap apply`。
5. CLI 安装并验证固定 Plugin，然后消费连接码、初始化 `codex-plugin` Workspace、执行 offline doctor；只有 `workspace.register` 成功后 Web 才显示 `connected`。
6. 新安装的 bundled Skills/MCP 在新的 Codex chat/session 生效。CLI 用不含连接码的 Plugin mention 和恢复 Prompt 打开 Workspace 新对话。
7. npm 安装器校验 GitHub Release 的 `checksums.txt`，原子安装 Go binary；Workspace/Device Credential 写入 macOS Keychain。
8. 初始化默认不注册 LaunchAgent、不启动 Daemon、不上传文件，也不写项目级 `.codex/config.toml` 或重复的 `.agents/skills`。

无法使用 Prompt 流程时，在空目录使用 Web 提供的固定 `contentcloud-cli`，不能替换为 `@latest`。先运行只读计划：

```bash
<contentcloud-cli> bootstrap plan . --server-url https://content.example.com --connect cck_xxx --json
```

检查返回的 `plan_id` 后再确认执行：

```bash
<contentcloud-cli> bootstrap apply . --server-url https://content.example.com --connect cck_xxx --plan-id bp_xxx --accept --json
```

用户 CLI 登录与设备连接凭据分离：`contentcloud auth login --no-wait --json` 发起浏览器确认，之后用 `--device-code` 完成并把 `ct_` 写入 Keychain。

## 验收

```bash
make check
go test -race ./...
pnpm --dir web test
pnpm check:plugin
```

设置 `CONTENTCLOUD_TEST_DATABASE_URL` 后，`go test ./...` 会额外执行真实 PostgreSQL migration、runtime-role RLS 隔离和来源处理生命周期测试。

完整范围和当前实现事实见 [V2 路线图](docs/roadmap/v2/README.md) 与 [V2 实现状态](docs/roadmap/v2/14-implementation-status.md)。
