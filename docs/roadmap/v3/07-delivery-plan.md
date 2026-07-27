# V3 零兼容实施与验收计划

## 1. 总原则

当前处于开发期，V3 直接替换旧演示、旧命令路径和旧页面。不能建设：

- V1/V2 与 V3 双写。
- 旧 `KnowledgeItem` 到 V3 类型对象的在线适配器。
- 旧页面路由与 V3 页面长期并存。
- 根据旧目录自动猜测 V3 对象的后台兼容逻辑。
- 原型与实际 React 页面分离维护。

需要保留的仅是 Git 历史和文档证据；运行时、数据库和 demo fixture 不承诺历史兼容。

## 2. 实施阶段

### P0：冻结 V3 契约与 UI 基线

输出：本目录文档、V3 `prototype.html`、Fixture Schema、原型追踪矩阵。

验收：

- 目录、服务端对象、Web 页面、API 和测试名称使用相同 V3 术语。
- 设计明确 `.codex`、`.agents`、`.codex-plugin` 与 `.contentcloud` 的职责。
- 每个 V2 原型主入口都有 V3 页面归属。

### P1：重建 V3 Workspace Template

输出：`workspace_marketing_agent@3.0.0`、V3 Schema、AGENTS、Workflow、Lint、Run/Handoff、Fixture Workspace。

删除：

- `work/current-run.json`。
- `../service/` 作为客户 Workspace 运行时依赖。
- 双实例源模型（instances YAML 与 Wiki Markdown 同时可编辑）。

验收：

- 空目录 bootstrap 后得到 V3 文件树。
- Workspace doctor 拒绝非 V3 布局而不在旧目录中写入。
- 两个 Codex 对话可对一个 Run 竞争 claim，只有一个成功。
- 金陵 Fixture 可以完整 lint、构建 index、生成 blocked 内容批次。

### P2：收敛本地 CLI/MCP/Plugin 业务接口

输出：V3 `workspace_context`、source ingest、knowledge query、content batch、publish 和 handoff typed tools。

删除：

- 直接围绕 V1 `KnowledgeItem`、SellingPoint、VisualizationPlan、Brief 进行编辑的 MCP 命令。
- 需要 API 实体 ID 才能进行本地候选创作的前置条件。

验收：

- 无网络时能完成资料登记、候选知识、query、blocked 批次和 handoff。
- 每个写入工具都受 RunClaim/CAS 和 Schema 约束。
- Plugin Skill 只引用 V3 Tool/Schema，客户数据不进入 Plugin。

### P3：重建服务端治理与投影

输出：V3 ProjectContext、Submission 类型、类型化 Decision、ApprovedSnapshot、WorkAssignment、Projection builder 和 API。

删除：

- `seedDemo` 三 TXT 和 handler 内硬编码业务链。
- 直接 Web 写正式 Source/Knowledge/SellingPoint/VisualizationPlan/Brief 的写 API。
- `KnowledgeItem.status=approved` 的通用资格判断。
- ScriptVersion 与 SubmissionRevision 的平行审批轨。

数据库策略：开发数据库只允许从空库应用 `00001_v3_baseline.sql` 直接进入 V3；不写生产数据迁移、backfill 或兼容读层。检测到旧 migration 历史时明确拒绝，已有本地 fixture 通过 V3 importer 重新生成。

当前进度：Artifact 已只允许由 ApprovedSnapshot 生成和查询；旧 Extension Envelope、ScriptVersion、Strategy/Brief 直写链、Presentation/local-open、设备轮询与本地 Artifact 索引均已删除。TaskRun 已收缩为 Automation 通用执行骨架，只保存 capability、冻结输入、输出契约、租约和 Attempt，当前唯一真实消费链是知识提取。DeliveryPackage 与 ContentBatch 已使用 `content_item_id/content_item_refs`，不再通过 script 别名引用 ContentItem。唯一 PostgreSQL V3 基线已经真实空库验证，RLS、不可变触发器和 Submission -> Decision -> ApprovedSnapshot -> Delivery/Result 主链通过。V4 Web 已能把现有 Project 与 review feedback 安全交给 Codex，但 WorkAssignment 尚未建立，仍属于 V3 W3-01。

验收：

- `context`、`knowledge`、`brief`、`content_batch`、`asset_batch`、`delivery`、`result` 都只能以 Revision 进入审核。
- Fact/Claim/Rights 分别产生正确 eligible 状态。
- 任何 Projection 写请求都被拒绝。
- 上游变更生成 ImpactAction，历史 Snapshot 不被改写。
- migration 目录只含单一 V3 基线，旧 migration 历史被拒绝而不是自动升级。

### P4：按 V3 原型重建 Web

输出：V3 路由、BFF query、项目导航、初始化、九域业务页、Automation、浏览器测试。

删除：

- 当前实体导向的 `sources/assets/knowledge/strategy/briefs/scripts/submissions/results/lineage/audit` 主导航。
- 当前 React 页面直接 POST 业务正文并立即生成云端对象的表单。
- V2 原型和实际页面不一致的状态。

验收：

- 侧栏、总览、Gate、下一动作和页面分组与 `prototype.html` 一致。
- 每个原型动作链接到真实 BFF 或明确的 disabled/未实施状态；没有假按钮。
- 页面全部使用 V3 Fixture 和真实 Projection API。
- 可访问性、桌面/移动截图和状态转换测试通过。

### P5：端到端 Fixture 验收

使用金陵古都香 V3 Fixture：

```text
bootstrap
  -> 20 来源登记
  -> 15 维诊断
  -> candidate knowledge + conflicts
  -> knowledge publish / type decisions / pull
  -> intent content batch
  -> blocked creative review
  -> 解除部分门禁后发布 content_batch
  -> review feedback -> 新对话修订
  -> delivery -> observation -> learning
```

验收同时覆盖 Codex CLI 与 Desktop。Desktop 只在已发布、真实安装、真实新会话环境下声明支持。

## 3. 原型追踪门禁

每个页面必须在 `web/src/v3/page-contracts.ts` 声明：

```ts
{
  prototypeView: 'knowledge',
  route: '/projects/:id/knowledge',
  query: 'projectKnowledgeProjection',
  commands: ['createAssignment', 'reviewDecision'],
  tests: ['knowledge-page.spec.ts']
}
```

CI 要求：

- 原型主视图、路由、query、command 和 Playwright 用例一一对应。
- 新增/删除原型入口必须同步更新该表。
- 禁止一个页面直接调用未在契约中声明的写 API。

## 4. 验收指标

| 类别 | 完成条件 |
| --- | --- |
| 目录 | V3 Workspace 可 bootstrap、lint、升级、reset，且没有旧结构兼容旁路 |
| 多对话 | RunClaim、CAS、Handoff、digest 竞争与恢复通过 |
| 知识 | Source/Evidence/Fact/Claim/Asset/Rights/Conflict 分离，状态门禁正确 |
| 内容 | Intent -> eligible/blocked -> Batch -> lint -> Submission 闭环可跑 |
| Web | 与 V3 原型页面和状态一致，真实 API 无 fake seed |
| 审核 | Decision 绑定 Revision digest，反馈可被新对话 pull 和消费 |
| 环境 | `.codex` 只保存 Adapter 配置，`.contentcloud` 可写状态健康，Plugin/Skill 版本可诊断 |
| 安全 | 无 token、绝对路径、transcript 或未授权原件泄露 |
| Automation | 隔离 Attempt、能力门禁、Submission 输出和人工审核完整 |

Codex Workspace 额外验收：非 Git 客户目录必须打开 `project-root/` 本身，用户已信任项目，并由 doctor 使用真实原子写探针证明 `.contentcloud/` 可写；不能从配置文件存在或 UI 文案推断成功。

## 5. 删除授权与协作边界

用户已明确授权开发期直接删除 V1/V2 兼容层、旧运行时文件、迁移和页面，不保留双写或历史数据迁移。执行时仍需逐层盘点入口、服务、存储和旁路，并用测试证明 V3 current owner 已接管能力。

`docs/roadmap/v4/` 由并行工作流开发，V3 收口过程不得修改、格式化、删除或纳入批量操作。
