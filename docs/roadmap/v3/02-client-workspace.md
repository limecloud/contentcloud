# V3 客户端 Workspace 架构

## 1. 定位

一个 V3 Workspace 对应一个 ContentCloud `Project`，也是用户在 Codex Desktop 中打开的项目文件夹。多个对话可以围绕不同任务工作，但共享同一套业务文件、只读批准快照和本地运行状态。

V3 目录借鉴“收件箱、项目、资产、资料、归档”的人类整理思想，但不照搬 PARA。原因是客户 Agent 还需要表达来源证据、类型化知识、人工门禁和执行状态。

## 2. 标准目录

```text
project-root/
├── AGENTS.md
├── .codex/
│   └── config.toml
├── .contentcloud/
│   ├── workspace.yaml
│   ├── environment.lock
│   ├── sync-state.json
│   ├── inbox/
│   │   ├── assignments/
│   │   ├── review-feedback/
│   │   └── decisions/
│   ├── cache/
│   │   ├── approved/
│   │   └── schemas/
│   ├── locks/
│   │   ├── workspace.claim
│   │   └── runs/
│   └── tmp/
├── 00-inbox/
│   ├── ideas/
│   └── unregistered-sources/
├── 10-context/
│   ├── client.yaml
│   ├── project.yaml
│   ├── methodology.yaml
│   ├── service-plan.yaml
│   └── intents/
├── 20-sources/
│   ├── registry.yaml
│   ├── originals/
│   └── extracts/
├── 30-knowledge/
│   ├── schema/
│   ├── pages/
│   │   ├── sources/
│   │   ├── evidence/
│   │   ├── facts/
│   │   ├── claims/
│   │   ├── assets/
│   │   ├── rights/
│   │   ├── conflicts/
│   │   └── domain/
│   ├── imports/
│   ├── packs/
│   └── index.md
├── 40-work/
│   ├── focus.md
│   ├── queues/
│   │   ├── review.md
│   │   ├── decisions.md
│   │   └── gaps.md
│   ├── runs/
│   │   └── <run-id>/context.json
│   └── handoffs/
│       └── <handoff-id>.json
├── 50-production/
│   ├── plans/
│   ├── campaigns/
│   ├── briefs/
│   ├── batches/
│   ├── scripts/
│   └── media/
├── 60-delivery/
│   ├── packages/
│   └── exports/
├── 70-results/
│   ├── imports/
│   ├── observations/
│   └── learnings/
├── 90-archive/
├── workflows/
└── scripts/
```

数字只用于让普通用户在侧边栏中按业务顺序浏览。CLI 不依赖字符串排序；它根据 `.contentcloud/workspace.yaml` 的 `layout_version` 和固定 V3 角色解析目录。

`.codex/config.toml` 是可选的 Codex Adapter 文件。只有项目需要 Codex 专用 MCP、hook、sandbox 或其他受支持配置时才生成；Plugin 已经提供所需能力时保持为空缺，不为了“看起来像 Codex 项目”而创建无用配置。

客户 Workspace 不强制创建 Git 仓库。Codex 官方说明非版本控制目录可能默认以只读方式打开，因此 Bootstrap 完成后必须引导用户在 Desktop 中信任该文件夹，并由 `workspace doctor` 实测当前进程能原子写入 `.contentcloud/`。不能仅通过检查 `.codex/config.toml` 存在就宣称 Workspace 可写。

## 3. 各目录职责

| 目录 | 事实源内容 | 允许谁写 | 是否 publish |
| --- | --- | --- | --- |
| `.codex/` | Codex 项目级配置、规则或 hook | Bootstrap/升级流程；普通 Run 不写 | 否 |
| `.contentcloud/` | 绑定、环境、缓存、收件箱、锁和同步游标 | CLI/MCP | 只上传摘要，不上传凭据和本机路径 |
| `00-inbox/` | 尚未登记的灵感与资料 | 用户、Agent | 否；登记后移动或引用 |
| `10-context/` | 客户、项目、方法论快照、服务计划和意图 | CLI 拉取；Agent 创建候选修订 | 作为 `context` Submission |
| `20-sources/` | 不可变原件或外部引用、哈希、提取结果 | 用户放入；CLI 登记和提取 | 默认只 publish metadata/evidence pack |
| `30-knowledge/` | 类型化知识页面、Schema、导入记录和七层知识包 | Agent 候选；CLI 校验 | 作为 `knowledge` Submission |
| `40-work/` | 当前焦点、队列、RunContext 和 Handoff | CLI/MCP 原子写 | 只 publish LocalRunSummary |
| `50-production/` | Plan、Campaign、Brief、批次、剧本和媒体候选 | Agent + CLI/MCP | 按对象类型 publish |
| `60-delivery/` | 本地组装的交付包和导出件 | CLI/MCP | 作为 `delivery` Submission |
| `70-results/` | 结果导入、观察和候选学习 | 用户、CLI/MCP | 作为 `result` Submission |
| `90-archive/` | 本地已结束内容 | 用户显式归档 | 不自动 publish |
| `workflows/` | 项目级业务流程和 Gate | 模板升级或项目负责人 | 随 `context` 版本披露 hash |
| `scripts/` | 确定性校验和构建工具 | 插件模板 | 不作为业务正文 |

## 4. 单一事实源

V3 不再让 `ontology/instances/*.yaml` 和 `wiki/*.md` 同时承担实例事实源。

规则如下：

1. `30-knowledge/pages/**/*.md` 是本地知识对象的唯一可编辑事实源。
2. YAML frontmatter 保存稳定 ID、类型、状态、关系和来源；Markdown 正文保存供人审阅的说明。
3. `30-knowledge/imports/*.yaml` 只记录批量导入来源、输入 digest、创建的 ID 和错误，不再拥有对象当前值。
4. `30-knowledge/index.md`、搜索索引和向量索引都是可重建 projection。
5. 决定不直接改写旧对象历史；本地创建新版本并引用云端 Decision/ApprovedSnapshot。

知识页最小契约：

```yaml
---
id: claim:ancient-formula
type: Claim
version: 1
status: needs_review
text: 沸氏香铺沿袭金陵古方。
risk_level: red
about_refs: [sku:jinling-gudu-incense]
evidence_refs: []
source_refs: [source:product-copy#paragraph=P0014-P0015]
decision_refs: []
---
```

## 5. 来源与 Markdown 的关系

必须区分三种文件：

1. 原始来源：PDF、DOCX、XLSX、图片、音视频等，放在 `20-sources/originals/` 或由 `registry.yaml` 指向外部只读路径。
2. 提取结果：OCR、分页文本、表格结构和缩略图，放在 `20-sources/extracts/`，可以重新生成。
3. 知识页面：Agent 从来源中提炼的 Markdown，放在 `30-knowledge/pages/`，带来源和证据引用。

因此，Web 不需要把 `.md` 加入“原始资料上传”白名单来解决对齐问题。正确链路是：本地 Markdown 通过结构化 manifest publish，服务端显示其业务投影；原始来源只有在披露策略允许时单独上传。

## 6. 方法论、知识包与意图

`10-context/` 保存项目实际使用的解析后上下文，不依赖工作区外部的 `../service/`：

```text
methodology.yaml   平台方法论的项目冻结版本
service-plan.yaml  当前租户服务阶段、角色、Gate 和交付标准
client.yaml        客户、品牌、产品和负责人
intents/*.yaml     当前项目允许的内容意图及渠道覆盖
```

`30-knowledge/packs/<pack-id>.yaml` 保存七层知识包引用和质量，不复制底层事实正文：

```yaml
layers:
  identity: [brand:..., sku:...]
  product: [fact:..., claim:...]
  market: [audience:..., scenario:...]
  expression: [brand-rule:..., claim:...]
  operations: [process:..., quality:...]
  content_engine: [campaign:..., learning:...]
  compliance: [rights:..., conflict:...]
```

## 7. RunContext 与多对话

V3 删除全局 `current-run.json`。每个 Run 独立保存：

```text
40-work/runs/<run-id>/context.json
```

最小字段：

```json
{
  "run_id": "run_...",
  "intent_id": "intent:douyin-short-video",
  "stage": "query",
  "status": "active",
  "context_revision": 7,
  "source_refs": [],
  "changed_ids": [],
  "eligible_ids": [],
  "blocked_ids": [],
  "output_refs": [],
  "checks": [],
  "history": []
}
```

规则：

- 新对话先只读扫描所有活动 Run 和 ready Handoff。
- 准备写入时获取 `RunClaim`，每次保存使用 `context_revision` CAS。
- 交接前保存业务对象、运行 lint、写 Handoff、释放 claim。
- Handoff 只引用 ID、路径和 digest，不保存 transcript 或隐藏推理。
- 不同 Run 可并行；同一版本化输出路径不可并发写。

## 8. Skill、Workflow 与 Plugin

V3 不设置用户可见的 `05-skills/` 业务目录：

- 通用 Skill 由 ContentCloud Plugin/精选 Pack 提供并版本锁定。
- 客户数据永远不进入 Plugin 包。
- 项目特有的业务步骤写在 `workflows/`，由 Skill 读取。
- `scripts/` 只放确定性工具，不复制第三方 Skill 方法论。
- 首版不支持工作区自定义 Skill；真实需求出现后再增加受审扩展机制。

### 8.1 Codex 官方目录与 ContentCloud 目录的边界

根据当前 Codex 官方手册：

| 路径 | 官方用途 | V3 用法 |
| --- | --- | --- |
| `AGENTS.md` | 仓库或子目录的持久指导 | 保存项目约束、必读上下文、验证命令和人工权限边界 |
| `.codex/config.toml` | 可信项目的 Codex 配置 | 仅保存 Codex Adapter 配置，不保存业务状态 |
| `.agents/skills/` | 仓库级 Skills | 首版不用；通用 Skill 由 Plugin 提供 |
| `.codex-plugin/plugin.json` | Plugin 包 manifest | 只存在于 ContentCloud Plugin 包仓库，不复制到客户 Workspace |
| Plugin 的 `skills/` | Plugin 自带 Skills | ContentCloud canonical Skills 的交付位置 |
| `.contentcloud/` | 非 Codex 官方目录 | ContentCloud 自有绑定、缓存、Run、Handoff、锁和同步状态 |

不能把 `.contentcloud` 改成 `.codex`，原因有三点：

1. `.codex` 是 Codex 宿主配置命名空间，ContentCloud 还要支持其他 Harness，业务状态不应绑定宿主。
2. Codex 官方说明默认 `workspace-write` 沙箱可能把 `.codex/` 和 `.agents/` 作为递归只读保护路径；Run、inbox、sync 和 lock 必须在对话中可写。
3. 配置与业务状态生命周期不同：`.codex/config.toml` 变化通常需要信任、刷新或新会话；`.contentcloud/` 在每个本地 Run 中持续变化。

Codex 打开路径必须是 `project-root/` 本身。V3 首版不依赖自定义 `project_root_markers` 猜测父目录，也不从任意子目录启动；这样即使客户没有 Git，Codex 仍能从当前根读取 `AGENTS.md` 和项目配置。

官方依据：

- [Codex best practices](https://learn.chatgpt.com/guides/best-practices)：`AGENTS.md` 保存持久指导，`.codex/config.toml` 保存项目配置，重复流程进入 Skill。
- [Custom instructions with AGENTS.md](https://learn.chatgpt.com/docs/agent-configuration/agents-md)：项目根和子目录的指导发现与覆盖规则。
- [Codex configuration](https://learn.chatgpt.com/docs/config-file/config-basic)：用户配置与项目 `.codex/config.toml` 的职责。
- [Build skills](https://learn.chatgpt.com/docs/build-skills)：仓库 Skill 使用 `.agents/skills`。
- [Build plugins](https://developers.openai.com/plugins/build)：Plugin 使用 `.codex-plugin/plugin.json` 和包内 `skills/`。
- [Protected paths](https://learn.chatgpt.com/docs/agent-approvals-security#protected-paths-in-writable-roots)：默认 workspace-write 下 `.codex`/`.agents` 可能只读。

## 9. 初始化后的首个对话

首次打开 Workspace 时，Scene Plugin 返回：

```text
项目：金陵古都香内容 Agent
方法论：15 维产品研发 v1
当前节点：立项
来源：20 个，已登记 20 个
知识资格：verified Fact 0 / approved Claim 0 / valid Rights 0
活动 Run：0
可接管 Handoff：1
主要阻断：包装尺寸冲突、价格矩阵、商标与素材权利
建议动作：继续 handoff hnd_...，处理客户待决策项
```

这些信息来自本地文件和已拉取快照，默认不访问服务端。
