# V3 架构基线与问题定义

## 1. 真实客户端已经验证了什么

`jinling-gudu` 的价值不在当前目录名，而在它已经验证了五组相互独立的责任。

### 1.1 约束与执行系统

| 责任 | 旧样本 | V3 结论 |
| --- | --- | --- |
| 永久约束 | `AGENTS.md` | Agent 权限、禁止事项和完成定义必须随 Workspace 生效 |
| 业务流程 | `workflows/*.md` | 每个流程必须声明输入、步骤、输出、失败和退回条件 |
| 执行能力 | `.agents/skills/` | Skill 负责方法和编排，不保存客户事实 |
| 确定性校验 | `scripts/` | Schema、引用、状态和内容门禁不能只靠 Prompt |
| 跨对话交接 | `work/runs/*.json` | 业务连续性必须落盘，不能依赖聊天历史 |

### 1.2 可信知识系统

```text
Source 原件登记
  -> Evidence 可复核位置
  -> FactAssertion 候选事实
  -> Claim 对外表达
  -> Asset 可使用对象
  -> RightsRecord 使用权利
  -> Conflict / DecisionRequest
  -> 人工决定后的 eligible / blocked 集合
```

这些对象不能合并成一个 `KnowledgeItem(status=approved)`：

- Fact 的合格状态是 `verified`。
- Claim 的合格状态是 `approved`。
- RightsRecord 的合格状态是 `valid`。
- Source 只表示原件登记，不证明其中内容正确。
- Asset 存在不代表拥有使用权。
- Conflict 必须共存并显式解决，不能用“最后写入值”覆盖。

### 1.3 客户 Agent 产品系统

```text
MethodologyVersion
  -> 15 维素材诊断
  -> ClientProfile / Project
  -> 七层 KnowledgePack
  -> IntentTemplate
  -> eligible / blocked 查询
  -> 候选内容与交付物
```

方法论回答“应该收集和验证什么”；知识包回答“Agent 当前知道什么”；意图回答“这次要生成什么”；本体规则回答“哪些内容允许进入结果”。

### 1.4 内容生产系统

样本中的首批十条脚本不是十个普通 Markdown 文件，而是一个受治理批次：

- 批次有 `batch_id`、渠道、目的和状态。
- 每条草稿有稳定 ID、内容类型、引用和阻断原因。
- 批次级声明禁止方向和缺失输入。
- 内容 lint 校验引用和可发布条件。
- `blocked` 不等于无价值，但明确禁止发布。

### 1.5 人类治理系统

Agent 不能把候选事实改成已验证，也不能把主张、权利或内容改成已批准。所有状态提升都需要：

```text
subject_id + subject_digest + decision + actor + basis + decided_at
```

本地记录候选和待决项，服务端负责跨团队决定、不可变版本和审计。

## 2. 当前 ContentCloud 的实际偏差

### 2.1 演示数据是假的业务骨架

`internal/httpapi/server.go` 的 `seedDemo` 创建三个 TXT 来源：产品说明、品牌边界和视觉规范，然后在服务端直接创建并批准知识、卖点、可视化方案和 Brief。

问题不是 TXT 扩展名不好看，而是这条演示链隐含了错误产品模型：

```text
云端上传文件 -> 云端直接建知识 -> 云端批准 -> 云端拼 Brief
```

它绕开了方法论诊断、知识包、本体类型、冲突、RunContext、eligible/blocked 查询和本地产物校验。

### 2.2 Web 页面没有呈现客户端主流程

现有页面能够显示来源、通用知识、素材权利、Brief、剧本和结果，但缺少：

- 15 维方法论覆盖和当前研发节点。
- 七层客户知识包及每层质量。
- IntentTemplate 与本次内容任务的输入门禁。
- Fact、Claim、Rights 的类型化状态和决策队列。
- LocalRunContext、Handoff、当前对话可继续任务。
- 本地候选、已提交 revision、已批准 snapshot 三者的明确区分。
- blocked 内容为什么仍可评审、为什么不可发布。

### 2.3 领域代码与 V3 文档不一致

当前代码仍保留直接创建/审核 `KnowledgeItem`、直接批准卖点和直接创建 Brief 的 V1 路径；方法论、知识包和意图主要存在于文档，没有成为可投影的 V3 契约。

已有 `SubmissionRevision`、`ApprovedSnapshot`、WorkspaceBinding、Environment 和 Handoff 基础设施可以复用，但它们必须成为唯一正式路径，不能继续与直接云端编辑业务对象并存。

### 2.4 当前 Web 为什么没有实现 V2 原型

这不是浏览器缓存或样式没有同步，而是开发路径发生了结构性偏离：

1. `docs/roadmap/v2/prototype.html` 定义的是“接入、项目总览、可信知识、市场情报、营销策略、内容策划、创意与剧本、审核、交付、学习、Automation”的目标工作台。
2. 当前 `web/src/components/Layout.tsx` 仍沿用最初版本的实体导航：资料、素材权利、可信知识、内容策略、Brief、剧本、提交审核、结果、追踪和审计。
3. `docs/roadmap/v2/13-acceptance-and-traceability.md` 明确把“九域、四层上下文与 Automation Plan”标为 `planned`；`14-implementation-status.md` 只把 Submission Web 审核切片列为已实现。
4. 后续 Plugin W0-W7 主要实现安装、环境、Handoff、Publish 和初始化诊断，没有回到九域 Web 的 P1 任务。
5. 工程中没有建立 `prototype view -> route -> BFF query -> domain projection -> browser test` 的追踪表，因此基础设施完成度被错误地等同于产品界面完成度。

V3 处理方式不是让当前页面换一套 CSS，而是以 V2 原型的导航层级、工作台密度、左侧项目上下文和“下一动作”设计为 UI 基线，重新接入 V3 领域投影。原型中的虚构数量、已批准状态和旧 Submission 类型不作为数据事实继承。

## 3. V3 继承什么，不继承什么

### 3.1 继承

- Source/Evidence/Fact/Claim/Asset/Rights 分离。
- 本体 Schema、稳定 ID、类型状态和来源引用。
- 方法论、知识包、意图、本体治理四层分离。
- RunContext 的阶段门禁和 history。
- deterministic lint 先于 publish。
- blocked 产物可以评审但不可交付。
- 人工决定和发布边界。

### 3.2 不继承

- `../service/` 依赖父目录的物理结构。
- `ontology/instances` 与 `wiki` 同时作为实例事实源的双重写法。
- 全局 `work/current-run.json`。
- 文件路径作为业务主键。
- 每个客户仓库复制一套 Skill 源码。
- 三个 TXT 和直接云端造知识的 demo seed。
- 为旧 API、旧数据库或旧前端保留兼容层。

## 4. V3 稳定不变量

1. 每个业务对象都有稳定 ID、类型、状态、版本和 digest。
2. 每个事实或主张都能追到一个或多个 Evidence locator。
3. 每个素材都能追到 Source 和 RightsRecord。
4. 每个生产输出都能追到 Intent、Knowledge Snapshot 和 Run。
5. 每个跨对话输入都通过 digest 和 Handoff 引用。
6. 每个云端决定都绑定不可变 SubmissionRevision。
7. 每个下发任务都绑定不可变业务输入和签名 Environment Bundle。
8. 本地目录可以随模板主版本演进，但服务端永远只理解契约，不解析任意文件树。
