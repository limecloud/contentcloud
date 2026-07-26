# V1 实现状态与验收入口

> 更新日期：2026-07-26。本文描述仓库当前事实，不用 feature flag 把 V1.1 能力伪装成完成。

## 1. 已实现闭环

| 领域 | 当前实现 |
| --- | --- |
| 身份与租户 | Argon2id 密码、12 小时会话、多租户切换/退出、固定角色、邀请创建/接受/撤销、角色变更、成员撤销后 session 即时失效、团队 Web/CLI/BFF、Postgres RLS 与双租户负测 |
| 项目先行接入 | Web/CLI 项目模板、乐观锁更新、归档只读/管理员恢复、一次性 `cck_` 创建/查询/取消、npm 校验安装器、macOS Keychain、LaunchAgent、首次 poll 后 connected |
| CLI 边界 | `ct_`/`dt_`/`rt_` 分权、统一 dispatch、稳定 envelope/退出码、风险确认、dry-run、schema、内嵌 Skills |
| 来源 | BFF/CLI 上传、不可变 hash、S3/本地 Blob、MIME 签名、ClamAV 生产门禁、确定性解析/OCR、EvidenceSpan |
| Gate 1 | `knowledge.extract` 本地任务、冻结 Evidence Contract、typed 候选、渠道、风险、人工批准/拒绝/冲突和审计；revision 同项目、ready、accepted span 与原文一致的双阶段证据强校验 |
| Gate 2 | 对标、画面/文案框架、镜头模式、卖点、可视化方案、Brief 硬引用门禁 |
| Gate 3 | ContextSnapshot、TaskContract、不可变 RunAttempt、租约回收/心跳/取消优先/最多 3 次 Attempt、capability 契约匹配、Codex/Claude Adapter、Script Package 校验、report 幂等 |
| Gate 4 | 逻辑 Script 与不可变修订/变体、真实字段 diff、ReviewCycle、整版结论/责任人、跨版本批注、版本绑定 ReviewGrant、OTP/撤销、客户移动页、最终依赖复核、客户决定与内容 hash |
| 导出 | canonical JSON、Markdown、XLSX；XLSX 一镜头一行、冻结首行、防公式注入 |
| Artifact 展示 | Extension Artifact Envelope 确定性校验、核心 Schema 原生展示、安全 rendition allowlist、设备在线实时降级、60 秒声明式 `local-open`；本机路径只保存在 CLI 本地索引 |
| Gate 5 | 单条/JSON/CSV/XLSX typed 导入、不可变 ImportBatch、整批原子性与行级错误、重复键、单一币种、服务端 ROI、Web 结果工作台和人工 RatingDecision；评级不自动改业务状态 |
| 追踪与审计 | Source 到 RatingDecision 的统一双向 LineageGraph、确定性 ImpactAnalysis、Web 阶段工作台、`lineage show/impact` 与 `audit list` CLI、BFF/OpenAPI 和租户隔离负测；只读投影不执行 Agent 或修改业务状态 |
| 部署 | Server/Worker 镜像、Postgres 自动迁移、S3 兼容存储、本地 compose 拓扑 |

## 2. 明确不属于 V1

- Hosted Preview、静态前端 bundle 托管和 iframe 展示：V1.1/P3。
- 图片、视频、数字人、配音或成片生成：外部生产系统职责。
- 服务端 LLM、Agent、prompt、模型路由或模型凭据：永久架构禁区。
- 自动连接抖音/小红书/视频号投放 API：V1.1 候选。
- Electron/Tauri 桌面壳：不实施；桌面形态为 CLI + 用户级 Daemon。
- 普通 HTML/SVG、工程文件和未知二进制在线执行：不实施；V1 只能 attachment、元数据或来源设备本机打开。

## 3. 环境相关限制

- macOS 是 V1 首发安全凭据与后台服务平台；Windows DPAPI 和 Linux Secret Service 需逐平台验收后开放。
- 本机 Tesseract 若缺少动态库，图片来源稳定进入 `OCR_UNAVAILABLE/needs_review`，不会猜测补字。生产 Worker 镜像包含 `chi_sim+eng`。
- PostgreSQL/S3 集成验收需要相应服务；纯开发模式可使用 Memory Store 与本地 Blob。
- npm 安装器只有在发布 GitHub Release 及 `checksums.txt` 后才从公网工作；仓库开发可设置 `CONTENTCLOUD_BINARY_PATH`。
- Extension Artifact V1 先登记 Envelope 与本机索引，不把大文件 base64 穿过控制面；安全 rendition 的对象存储直传仍需上传许可协议。
- `00012_performance_imports.sql` 与 PostgreSQL 事务/RLS 测试已进入仓库；在新的数据库环境执行前仍须按变更流程确认并保存迁移证据。
- Lineage/Impact 直接读取既有表，不需要新 migration；真实浏览器截图和南京角色 UAT 尚未完成，不能由 TypeScript build 替代。
- 邮箱验证、忘记密码与重置密码仍需事务邮件适配器、投递域名和生产回调地址；开发环境不返回伪造的“已发送邮件”令牌，因此 FR-01 仍是 `partial`。
- 官方 Skill 快速校验脚本依赖 Python `PyYAML`；缺少该依赖时可用现有安全 YAML 解析器执行同等 frontmatter 规则，不应为验收修改全局 Python 环境。

## 4. 验收命令

```bash
go fmt ./...
go vet ./...
go test -race ./...
pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web build
make build
python /Users/coso/.codex/skills/.system/skill-creator/scripts/quick_validate.py skills/contentcloud-marketing-video-script
```

本地产品闭环：

```bash
CONTENTCLOUD_DEV_MODE=1 ./bin/contentcloud-server
./bin/contentcloud --json doctor --offline
./bin/contentcloud team invite editor@example.com --role editor --dry-run --json
./bin/contentcloud project update "$PROJECT_ID" --row-version 1 --owner "项目负责人" --dry-run --json
./bin/contentcloud device connect-create "$PROJECT_ID" --dry-run --json
CONTENTCLOUD_BINARY_PATH=./bin/contentcloud node packages/contentcloud/bin/contentcloud.js --help
```

生产形态本地验证：

```bash
docker compose up --build
```

## 5. 真实闭环验收记录

2026-07-26 使用最新二进制、真实 PostgreSQL、独立 Server/Worker 和本地 fixture Adapter 完成一次产品 E2E：

```text
TXT 上传 -> Worker 解析 -> ready EvidenceSpan
-> 证据绑定知识 -> 人工批准
-> 对标/框架/卖点/可视化方案 -> Brief 批准
-> 设备连接 -> CLI 创建 Run -> 本地 Daemon 报告 Script Package
-> 内审 -> OTP 客户批准
-> CLI 导出 JSON/Markdown/XLSX -> CLI 原子导入投放观察 -> 人工评级
```

验收结果：Run `succeeded`、ScriptVersion `approved`、三种下载文件可解析、结果导入 1 条、项目审计 28 条。后续自动化已增加 RunAttempt、知识提取、逻辑 Script 修订/变体、ReviewCycle、批注门禁、Grant 撤销，以及 ImportBatch/混合币种/重复键/ROI/RatingDecision 单元与 PostgreSQL 测试；另有 Memory Store source-to-rating 双向 lineage、影响动作、跨租户负测和 BFF 结果审计断言。`00012` 的真实数据库执行证据尚未在本次实现会话中重跑；该记录也不替代南京试点团队 UAT。

## 6. 发布门禁

发布前必须同时满足：Go race test、Web typecheck/build、Postgres 迁移与 RLS 双租户测试、来源损坏/伪 MIME/恶意文件测试、Run 租约/取消/report 冲突测试、OTP/过期/跨版本/批注可见性测试、三格式导出语义测试、安装器 checksum 负测和 macOS LaunchAgent 真机测试。
