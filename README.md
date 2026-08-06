# Content Work OS

Content Work OS 是面向 AI 内容营销团队的本地优先创作与云端治理系统。客户可在本机 Codex、Claude Code 等成熟智能体客户端中完成资料整理、知识工程、策略、创作简报和剧本；服务端负责项目、不可变提交、人工审核、审计和可选的自动化调度。

当前版本覆盖从输入收集、知识治理、剧本、分镜素材、视频生成、成片审核到正式交付的完整生产链。

## 核心边界

- 云端不执行客户上传的代码，也不保存本地智能体或模型凭据。
- 智能体、技能、渲染器、脚本和 CI 的所有程序化服务通信只经过 `contentcloud` 命令行工具。
- 使用者工作台只访问同源 `/api/bff`，独立系统后台只访问 `/api/v1/admin`；这些内部 HTTP、token 和对象存储协议都不是公共 SDK。
- 客户先在工作台创建项目，再把一次性智能体提示词粘贴到 Codex 或 Claude，由智能体初始化本地工作区、项目级技能和 MCP。
- 普通本地操作不创建自动化任务；只有显式启用的远程、事件或定时自动化使用后台服务。
- 客户审批绑定不可变提交版本的内容摘要，不会自动跟随“最新版本”。

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

## 使用文档

多客户端、多内容形态的用户指南以 [`docs/content/README.md`](docs/content/README.md) 为单一事实源。运行服务后可通过公开 `/docs` 文档中心查看；Agent 可使用 `Accept: text/markdown` 读取同源页面。内部扩展架构稿位于 `docs/content/internal/`，不会进入公开文档 API。

下一阶段的目标架构与产品需求分别位于 [平台基线](docs/foundation/README.md) 和 [产品需求索引](docs/product/README.md)，统一表达见[产品叙事规范](docs/product/00-product-narrative.md)。这些文档定义 Studio-first、独立运营控制台、Agentic Job Runtime、[创作资产库](docs/product/creative-asset-library/README.md)和创作流水线扩展方向，均为待评审/待实现目标，不改变本 README 所述当前能力边界；Runtime 专项实施路线见 [V8](docs/roadmap/v8/README.md)。

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
./bin/contentcloud bootstrap preflight ./contentcloud-project --offline --json
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
2. 项目总览创建 10 分钟有效的公开 ConnectSession ID，并拼成不含凭据的 Agent Prompt：

```text
请读取 https://content.example.com/api/bootstrap，并按照其中的步骤在 Codex 中初始化这个 Content Work OS 项目。

server-url: https://content.example.com
session-id: 11111111-1111-4111-8111-111111111111
contentcloud-cli: npx --yes @limecloud/contentcloud@<pinned-version>
project: "品牌 / 单品"
```

3. 用户把 Prompt 粘贴到 Codex 安装会话。Agent 获取公开的 `/api/bootstrap` 协议，先执行只读 `bootstrap preflight`，通过后再执行 `bootstrap plan`。
4. CLI 返回固定 Marketplace、Plugin、目标目录变化和确定性 `plan_id`；用户确认该计划后，Agent 才能把同一个 `plan_id` 传给 `bootstrap apply`。
5. 命令行工具在本机生成 PKCE 校验参数并打开浏览器。用户在已登录的 Content Work OS 页面核对短码并批准后，命令行工具才能换取设备和工作区凭据。
6. CLI 安装并验证固定 Plugin、初始化 `codex-plugin` Workspace、执行 doctor；只有 `workspace.register` 成功后 Web 才显示 `connected`。
7. 新安装的 bundled Skills/MCP 在新的 Codex chat/session 生效。CLI 用不含凭据的 Plugin mention 和恢复 Prompt 打开 Workspace 新对话。
8. npm 安装器校验 GitHub Release 的 `checksums.txt`，原子安装 Go binary；Workspace/Device Credential 写入 macOS Keychain。
9. 初始化默认不注册 LaunchAgent、不启动 Daemon、不上传文件，也不写项目级 `.codex/config.toml` 或重复的 `.agents/skills`。

无法使用 Prompt 流程时，在空目录使用 Web 提供的固定 `contentcloud-cli`，不能替换为 `@latest`。先运行环境检查：

```bash
<contentcloud-cli> bootstrap preflight . --server-url https://content.example.com --json
```

再使用 Web 显示的公开 ConnectSession ID 生成计划：

```bash
<contentcloud-cli> bootstrap plan . --server-url https://content.example.com --session 11111111-1111-4111-8111-111111111111 --json
```

检查返回的 `plan_id` 后再确认执行：

```bash
<contentcloud-cli> bootstrap apply . --server-url https://content.example.com --session 11111111-1111-4111-8111-111111111111 --plan-id bp_xxx --accept --json
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
