# ContentCloud

ContentCloud 是面向 AI 内容营销团队的本地优先创作与云端治理系统。客户在本机 Codex、Claude Code 等成熟 Agent 中完成资料整理、知识工程、策略、Brief 和剧本；云端负责项目、不可变 Submission、人工审批、审计和可选 Automation 调度。

V2 交付 AI 视频就绪剧本，不生成图片、视频或成片。Hosted Preview 是低优先级能力。

## 核心边界

- 云端 zero-exec：不调用、代理、编排 LLM，不保存模型凭据，不执行客户上传代码。
- Agent、Skill、Renderer、脚本和 CI 的所有程序化服务通讯只经过 `contentcloud` CLI。
- Web 只访问同源 `/api/bff`；内部 HTTP、token 和对象存储协议不是公共 SDK。
- 客户先在 Web 创建项目，再用一次性连接码初始化本地工作区、项目级 Skills 和 MCP。
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
make build
CONTENTCLOUD_DEV_MODE=1 ./bin/contentcloud-server
```

打开 `http://localhost:8080`。开发模式使用 Memory Store、本地 Blob，并自动创建金陵古法线香演示项目；来源由内置确定性 Worker 处理。

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
export CONTENTCLOUD_S3_BUCKET='contentcloud'
export CONTENTCLOUD_S3_REGION='us-east-1'
export CONTENTCLOUD_S3_ENDPOINT='https://s3.example.com' # AWS S3 可省略
export CONTENTCLOUD_S3_ACCESS_KEY_ID='...'
export CONTENTCLOUD_S3_SECRET_ACCESS_KEY='...'
./bin/contentcloud-server
```

Worker 使用相同数据库与对象存储配置，并要求 ClamAV：

```bash
CONTENTCLOUD_REQUIRE_MALWARE_SCAN=1 ./bin/contentcloud-worker
```

可用 `docker compose up --build` 启动 Server、Worker、PostgreSQL 和 MinIO 的完整本地拓扑。Compose 中的凭据只用于本机开发。

## 首次项目连接

1. 用户在 Web 创建项目。
2. 项目总览生成 10 分钟有效、单次使用的 `cck_`。
3. 用户在自己的 Mac 运行页面给出的命令：

```bash
npx --yes @goodvision/contentcloud@latest init \
  --server-url https://content.example.com \
  --connect cck_xxx \
  --target all \
  --accept-project-config \
  ./contentcloud-project
```

4. npm 安装器校验 GitHub Release 的 `checksums.txt`，原子安装 Go binary。
5. CLI 把 `wt_` Workspace Credential 和兼容用 `dt_` Device Credential 写入 macOS Keychain。
6. CLI 初始化本地模板、Skills/MCP，并通过 `workspace.register` 确认绑定；默认不注册 LaunchAgent、不启动 Daemon、不上传文件。

用户 CLI 登录与设备连接凭据分离：`contentcloud auth login --no-wait --json` 发起浏览器确认，之后用 `--device-code` 完成并把 `ct_` 写入 Keychain。

## 验收

```bash
make check
go test -race ./...
pnpm --dir web test
python /Users/coso/.codex/skills/.system/skill-creator/scripts/quick_validate.py \
  skills/contentcloud-marketing-video-script
```

设置 `CONTENTCLOUD_TEST_DATABASE_URL` 后，`go test ./...` 会额外执行真实 PostgreSQL migration、runtime-role RLS 隔离和来源处理生命周期测试。

完整范围和当前实现事实见 [V2 路线图](docs/roadmap/v2/README.md) 与 [V2 实现状态](docs/roadmap/v2/14-implementation-status.md)。
