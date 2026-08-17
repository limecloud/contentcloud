# 产品平面与总体架构

状态：`目标架构；客户与运营产品面边界已进入实现，完整模块迁移待完成`。

更新时间：2026-08-07。

上位规范：[ContentCloud 平台基线](../../foundation/README.md)。如本文与基线的产品平面、事实所有权或 Runtime 边界冲突，以基线和对应 ADR 为准。

## 1. 要解决的问题

当前内容系统容易把三类职责混在一起：客户完成创作、运营人员配置和治理流水线、执行系统调度智能体和程序。只在同一个工作台中更换文案或隐藏高级菜单，仍会让客户面对项目、知识、SOP、运行记录、审核门禁和执行环境等内部概念。

目标不是缩减 ContentCloud 的底层能力，而是把复杂度放在正确的位置：

- 客户使用简单、具体、以产出为中心的创作页面。
- 平台运营人员使用完整的运营控制台管理创作产品。
- Runtime 使用稳定的内部契约调度所有执行者并保存权威状态。
- 执行者只完成明确分配的步骤，不拥有任务全局状态和批准权。

### 1.1 客户叙事视角

客户不先看下面的 Runtime 架构，而先理解一个稳定闭环：

```text
任务输入与项目参考
          |
          v
Content Work OS 创作任务
场景选择 · 当前进度 · 结果预览 · 人工确认
          |
          v
人物原型、剧本、分镜、图片、视频、交付包与后续专业工具
          |
          v
已确认结果继续进入资产 / 创作结果
```

中心不能命名为“ContentCloud Agent”。Content Work OS 是客户产品品牌；ContentCloud Agentic Job Runtime 是中心背后的技术执行内核。完整表达规则见[产品叙事规范](../00-product-narrative.md)。

## 2. 五个边界清晰的部分

### 2.1 客户创作台

客户创作台按“我要完成什么”组织，不按内部领域或执行技术组织。

客户只需要理解：

- 选择哪个创作场景。
- 需要提供哪些资料。
- 当前做到哪一步。
- 现在需要补充或确认什么。
- 已经得到哪些可预览、可审核或可交付的结果。
- 已上传或导入的哪些文档、图片、视频、音频和表格可以加入本次创作。
- 已有的哪些人物原型、剧本、分镜、图片和视频结果可以直接复用。

工作区资料、任务参考、创作结果和交付必须在客户产品面分开：

| 客户概念 | 示例 | 入口 | 收录规则 |
| --- | --- | --- | --- |
| 我的资产 | 文件夹、文档、图片、视频、音频、表格 | 资产 | 客户明确上传、导入或登记 |
| 任务输入/项目参考 | 搜索候选、灵感、知识、来源证据、权利摘要 | 当前任务、项目参考 | 不自动进入资产 |
| 创作结果 | 人物原型、剧本、分镜、图片、视频 | 资产 | 流水线结果投影产生 |
| 交付作品 | 已整理的交付包和正式下载文件 | 交付 | 不复制为新的资产类型 |

客户默认不看到：

- SOP、JobRun、NodeRun、AgentInstance 和执行图。
- 模型、适配器、MCP、租约、重试策略和服务商配额。
- StateCollection、ContextView、事件游标和外部操作状态机。
- 平台级租户配置、能力白名单和完整运行记录。

### 2.2 ContentCloud 运营控制台

运营控制台是平台基座的管理表面，供平台运营、内容运营、支持和授权管理员使用。

它负责：

- 创建、测试、发布、停用和回退客户体验模板。
- 维护客户可见步骤、输入表单、文案和结果呈现。
- 绑定已发布 SOP、数据 Schema、审核节点和交付规则。
- 配置能力、执行者、模型、服务商、区域、预算和隔离要求。
- 按租户启用场景并固定可用版本。
- 查看运行诊断、费用、外部操作、失败恢复和审计。
- 治理创作资产的收录、权利、失效、重复、使用影响和目录重建。

运营控制台不能直接改写 Runtime 权威状态，不能“强制成功”，也不能绕过批准版本生成正式交付。

### 2.3 ContentCloud Agentic Job Runtime

Runtime 是内部执行内核，不是客户入口。它负责：

- 关联 WorkTask，并保存 JobRun、NodeRun、RuntimeAttempt 和事件。
- 编译并固定执行计划版本。
- 判断节点依赖、资源、权限、预算和人工门禁。
- 发放最小权限租约并回收执行权。
- 处理重试、取消、中断恢复、外部结果不明和对账。
- 关联正式 Artifact、Submission、ApprovedSnapshot 和 DeliveryPackage。
- 向客户侧和运营侧生成不同的只读投影。
- 接收已经过拥有域校验并固定版本摘要的创作资产引用。

Runtime 不理解客户页面布局，也不把“人物原型”“公众号文章”写死成调度逻辑。业务语义通过版本化流水线和类型化输入输出进入 Runtime。

### 2.4 执行者

执行者完成一个节点要求的具体能力：

| 执行者类型 | 适合工作 | 不负责 |
| --- | --- | --- |
| ContentCloud 确定性 Worker | Schema 校验、转换、渲染、去重、分页、打包、轮询和对账 | 创意判断、事实批准 |
| ContentCloud 托管智能体 | 经批准的云端推理任务，使用受限上下文、模型和工具 | 任意访问客户数据、扩大权限 |
| 本地 Codex / Claude Code | 本地资料研究、MCP 工具、策略、人物原型、剧本和语义修订 | 保存云端权威状态、自动批准 |
| 外部服务商 | 搜索、图片、视频、语音、转录和平台 API | 决定业务任务是否成功或交付 |
| 人工节点 | 事实、权利、方向、费用和最终内容决定 | 直接修改运行时历史 |

### 2.5 Content Work OS Desktop

Desktop 是持续项目工作面，不是客户 Web Studio 的壳，也不是 Codex 的聊天容器。它直接面向项目生命周期，负责：

- 展示上下文、来源、知识、工作、生产、结果和交付目录。
- 观察本地文件、Codex Apply、外部编辑、同步、上传和处理进度。
- 提供大媒体预览、版本差异、冲突处理、审批收件箱和交付状态。
- 通过 Go Local Service 与 Cloud API 交互，不让 Renderer 直接访问文件系统或服务端。
- 通过对象引用、revision 和意图发起 Codex Handoff；不在 Desktop 内复制 AI 对话。

Desktop 不拥有 Cloud Revision、Approval、Artifact 或 Runtime 事实；服务端命令和 Local Workspace Kernel 必须重新校验所有写入。

## 3. 总体架构

```text
┌────────────────────────────────────────────────────────────┐
│ 客户创作应用                                                │
│ IP 人设视频 / 营销视频 / 公众号文章 / 后续创作场景          │
│ 业务表单 · 资产复用 · 阶段进度 · 结果预览 · 确认 · 交付    │
└──────────────────────────┬─────────────────────────────────┘
                           │ Customer Journey BFF
                           v
┌────────────────────────────────────────────────────────────┐
│ 客户体验与流水线产品层                                      │
│ ExperienceTemplate · Published SOP · Input/Output Contract │
│ 客户步骤映射 · 结果资产投影 · 动作映射 · 租户可用性         │
└──────────────────────────┬─────────────────────────────────┘
                           │ JobPlanRevision / Commands
                           v
┌────────────────────────────────────────────────────────────┐
│ ContentCloud Agentic Job Runtime                            │
│ WorkTask · JobRun · NodeRun · Gate · State · ResultAssetRef  │
│ Scheduler · Lease · Budget · Effect · Audit · Projection    │
└─────────────┬────────────────┬────────────────┬─────────────┘
              │                │                │
              v                v                v
     ContentCloud Worker   Local Agent     External Provider
     校验/转换/渲染       Codex/Claude    搜索/图片/视频/API
              \                |                /
               \---------------+---------------/
                               |
                               v
                       Candidate / Artifact
                               |
                               v
                         Human Gate / Review

┌────────────────────────────────────────────────────────────┐
│ ContentCloud 运营控制台与 Runtime Explorer                  │
│ 配置 · 发布 · 租户 · 能力 · 运行 · 费用 · 故障 · 审计      │
└────────────────────────────────────────────────────────────┘
```

## 4. 产品层契约

以下是下一阶段建议冻结的逻辑契约。名称是目标设计，不表示当前已经存在同名数据库表。

### 4.1 ExperienceTemplate

客户体验模板定义一个客户可以直接使用的创作应用：

```text
ExperienceTemplate
├── id / version / status
├── customer_name / description / icon
├── content_type
├── input_form_schema
├── customer_steps[]
├── customer_actions[]
├── result_presentations[]
├── published_sop_version_ref
├── required_capabilities[]
├── tenant_eligibility
└── published_at / retired_at
```

客户步骤可以聚合多个内部节点，但不能独立保存运行状态。它的状态必须由 Runtime 投影计算。

### 4.2 PipelineDefinition

流水线定义描述业务阶段、输入输出、能力和人工门禁。首版应尽量复用现有 `SOPVersion` 和 `StageDefinition`，不急于创建第二套工作流模型。

```text
PipelineDefinition
├── stages[]
│   ├── stage_id
│   ├── input_schema_refs[]
│   ├── output_schema_refs[]
│   ├── required_capabilities[]
│   ├── allowed_execution_modes[]
│   ├── checks[]
│   └── gate_refs[]
└── compile -> JobPlanRevision
```

只有现有 SOP 无法表达客户流水线需要的稳定语义时，才将 `PipelineDefinition` 提升为独立持久化对象。

### 4.3 CapabilityDefinition

能力声明回答“这一步需要什么”，不直接回答“必须使用哪个客户端”。

```text
CapabilityDefinition
├── capability_id
├── input_schema_refs[]
├── output_schema_refs[]
├── supported_execution_modes[]
├── data_classification
├── side_effect_class
├── cost_model
└── health / availability
```

示例：

```text
capability_id: source.search
supported_execution_modes:
  - deterministic_worker
  - trusted_local_agent
  - managed_agent
  - external_provider
```

### 4.4 ExecutionBinding

流水线编译或任务准入时，根据以下条件把能力绑定到具体执行者：

- 租户和项目策略。
- 数据是否允许离开本地。
- 设备、Worker 和服务商当前能力。
- 模型、区域和服务商批准状态。
- 隔离等级、网络出口和工具白名单。
- 预算、并发、费用和任务优先级。

绑定结果必须进入不可变执行计划，运行中不得因为一个执行者不可用而静默切换到另一个执行者。回退必须来自已发布策略或人工决定。

## 5. 多种投影，一个事实体系

```text
业务域事实 + Runtime 执行事实
      |
      +--> CustomerJourneyProjection
      |      客户步骤、业务阻断、客户动作、结果预览
      |
      +--> WorkspaceMaterialProjection
      |      客户上传/导入资料、文件夹、处理状态、允许动作
      |
      +--> CreativeResultAssetProjection
      |      跨任务生成结果、来源任务、版本、复用状态
      |
      +--> RuntimeOperationsProjection
             节点、尝试、执行者、事件、费用、外部操作、诊断
```

`WorkspaceMaterialProjection`、`CreativeResultAssetProjection` 与客户任务投影都不是万能写模型。前者投影客户明确上传或导入的资料与组织关系，后者投影人物原型、剧本、分镜、图片和视频等结果。Source、Knowledge、RightsRecord 等输入和治理对象只在任务输入或运营投影中显示；任何修复都必须调用事实拥有域命令。Runtime 只消费固定底层对象版本和摘要，不把目录项或文件夹当作业务正文。

同一个底层事实或状态应翻译成不同语言：

| 底层事实或状态 | 客户创作台 | 运营控制台 |
| --- | --- | --- |
| `waiting(resource)` | 正在等待可用处理能力 | 服务商配额已满，预计继续等待 |
| `waiting(human_gate)` | 有一项结果等待你确认 | Gate `persona_approval` 等待客户审批人 |
| `blocked` + rights check | 任务输入缺少使用授权 | `rights.references` 检查失败 |
| local agent offline | 需要连接本地创作工具 | Codex workspace binding offline |
| external effect unknown | 正在核对外部处理结果 | Provider 请求结果不明，禁止自动重试 |
| result reuse blocked | 该结果当前不可用于新创作 | 输入权利或批准条件失效，显示影响范围和修复入口 |

客户错误必须说明用户能做什么；运营错误必须保留可定位、可审计的技术原因。

## 6. 版本和发布关系

一项任务开始时必须固定：

```text
WorkTask
├── experience_template_id
├── experience_template_version
├── sop_id / sop_version
├── job_plan_revision_id
├── capability_binding_digest
└── input_snapshot_refs[]
```

运营人员发布新模板或 SOP 后：

- 新任务使用新版本。
- 进行中的任务继续使用原版本。
- 停用版本不删除历史任务和结果。
- 紧急停用能力时，受影响节点进入明确阻断，不自动改用更宽松执行者。
- 回退发布只改变后续新任务的默认绑定，除非用户显式创建新的执行实例。

## 7. API 和部署边界

产品面完全分离不要求首版拆成多个微服务。推荐继续使用模块化单体和独立权限边界：

```text
同一部署单元
├── runtime/                  执行内核
├── pipeline/                 SOP 编译与能力绑定
├── customer-bff/             客户业务投影和业务命令
├── experience-projection/    CustomerJourney 和 CreativeResultAsset
├── admin-control-plane/      运营配置、发布和诊断
└── apps/web/
    ├── studio/               客户创作台
    └── admin/                运营控制台
```

边界要求：

- 客户创作台只访问客户 BFF，不读取 Runtime 内部 API。
- 运营控制台使用管理员 API，并保留平台角色和审计。
- Runtime 数据库只能通过领域服务写入，前端不能直接拼接状态变更。
- 长任务在 Worker、Daemon 或受控执行器中运行，不阻塞同步 HTTP 请求。
- 后续是否拆服务由吞吐、故障隔离和团队所有权决定，不提前引入分布式事务。

## 8. 安全和数据边界

- 本地私有资料默认留在用户工作区；只有用户明确选择的摘要、候选或正式提交进入云端。
- 搜索结果、网页、文件、服务商响应和智能体输出都视为不可信数据，不能改变服务端策略。
- ContentCloud 不向执行者暴露数据库凭据、租户令牌、设备令牌或服务商密钥。
- 执行者只获得当前节点的 ContextView、短期凭据、允许工具和预算。
- 外部写操作必须登记、幂等并处理结果不明，不能因网络超时直接重复提交。
- 完整智能体对话和本地文件不是权威业务状态，默认不进入客户页面或普通运营日志。
- 结果资产预览继承底层数据分类；正式复用时服务端重新校验租户、版本、输入权利和用途。

## 9. 非目标

- 不建设客户可见的通用工作流编辑器。
- 不允许运营人员上传任意脚本并直接注册为执行能力。
- 不把所有内容类型塞进一个包含大量可选字段的万能 Schema。
- 不要求所有流水线阶段都由智能体执行。
- 不把 Codex 或 Claude Code 会话当作业务数据库。
- 不让客户页面直接显示 Runtime Explorer。
- 不建设吞并所有事实对象的超级 Asset、企业级 DAM 或全盘同步网盘；客户需要的文件夹、上传/导入和基础预览属于资产工作区的明确范围。
- 不在首版因产品面分离而拆分为多个独立部署服务。

## 10. 架构验收标准

1. 同一 Runtime 能执行至少两种结构不同的流水线，而不增加内容类型专用调度表。
2. 一个业务阶段可以在不同租户策略下绑定不同执行模式，但任务开始后绑定保持可追溯。
3. 客户创作台无法直接访问节点事件、服务商密钥、完整上下文或平台配置。
4. 运营人员可以定位客户看到的每个步骤对应的 WorkTask、JobRun、节点、门禁和产物。
5. 客户页面和运营控制台对同一权威状态显示一致但角色适配的文案。
6. 新模板发布、停用和回退不会改写进行中的任务。
7. Runtime、客户 BFF 和运营控制台各自有独立的契约测试和权限测试。
8. 客户能在同一资产入口管理工作区资料并复用已确认结果；两个投影可重建，且不会形成第二套正文、权利或批准状态。
