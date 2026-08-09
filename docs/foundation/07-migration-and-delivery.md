# 渐进迁移与交付计划

状态：`执行切流已完成；Studio、知识治理、资产首切片、Runtime 内核和运营 Explorer 首版已落地，生产验证与兼容 DTO 退场仍待完成`。

更新时间：2026-08-09。

## 1. 迁移策略

```text
长期架构：平台基线重构
交付路径：产品层适配
客户验证：简单 Studio 纵向切片
迁移方法：Strangler + 旁路比较 + Canary + 分阶段切流
```

不执行停机式大重写。服务端按模块渐进拆分，但客户 Web 旧 Shell 已物理删除；仍保留的 BFF 只服务明确的 CLI、Agent 或本地工作区契约，只有一个权威写入方。

## 2. 双轨交付

```text
价值轨
Studio Shell -> Experience -> 灵感采集 -> 客户选择 -> 下游固定输入
     |              使用当前线性 SOP / WorkTask / Runtime 业务投影
     |              尽早验证客户完成率和运营成本
     |
     +---------------------------------------------------+
                                                         |
平台轨                                                   |
事实地图 -> 模块边界 -> Job/Plan -> Shadow Runtime（首版已具备） -> 切流 -> 清理
```

价值轨不能绕过平台规范；平台轨不能以长期正确为由推迟客户验证。

## 3. 实施阶段

### F0：事实与决策冻结

交付：

- 核心对象生命周期和事实所有权矩阵。
- WorkTask、StageRun、TaskRun 只读投影与 JobRun/NodeRun/RuntimeAttempt 的处置记录。
- Studio-first 定位和旧客户入口迁移 ADR。
- 当前 API、表、契约、路由、投影和执行路径事实地图。
- 基线指标、Feature Flag 和回退责任人。

退出条件：任何新 Runtime 表或新 Studio 权威状态出现前，上述 ADR 已批准；每个现有对象标记 `current / compat / deprecated / dead`，并单独记录目标动作、所有者和退出门槛。

### F1：首个客户价值切片

使用现有线性内核交付：

- 独立 Studio Shell、路由和权限边界。
- 一个已发布的 `IP 人设营销视频` ExperienceTemplate。
- 六步客户流程骨架，但只有“灵感采集”是首个真实可执行阶段。
- 搜索/受控来源/本地资料统一为 Source、Evidence 和 CandidateInsight。
- 客户选择 Gate 和固定下游输入。
- 运营关联诊断、预算、租户开关和失败恢复。

本地 Codex MCP 若 POC 尚未通过，可以标为后续能力，不能阻塞基本云端或确定性路径。

退出条件：真实试点客户能完成灵感采集并进入下一阶段；记录首结果时间、完成率、候选保留率、运营介入和单任务成本。

### F1A：创作资产复用切片

在 F1 的灵感采集闭环上增加跨任务资产沉淀：

- 使用现有 Source、Asset、RightsRecord、ApprovedSnapshot、Artifact、DeliveryPackage 和 lineage 事实。
- 旁路构建只收录人物原型、剧本、分镜、图片和视频结果的 `CreativeAssetCatalogItem` 只读投影，不改变拥有域写路径。
- 客户入口先增加独立“创作结果”视图，只展示五类生成结果；旧品牌资料和输入型数据仍由资料或任务参考路径读取，不进入结果状态机。
- 完成“结果生成 -> 客户确认 -> 资产目录出现 -> 固定引用进入新任务 -> 新结果再次沉淀”的闭环。
- 运营侧处理权利待办、失效、重复组、投影延迟和影响范围。

退出条件：真实试点客户完成一次跨任务复用；权利过期、来源失效、跨租户和陈旧投影均不能创建不安全的新引用；目录重建结果确定且无外部副作用。

### F1B：客户“我的资产”工作区切片

在 F1A 已验证的结果复用边界旁增加客户资料工作区，不修改结果目录契约：

- 冻结 `WorkspaceFolderItem`、`WorkspaceMaterialItem`、上传/导入命令和固定资料引用。
- 支持文件夹、文档、图片、视频、音频、表格和其他文件的最小管理能力。
- 对上传/导入、预览、OCR、转写和摘要使用独立处理状态；不复用结果确认状态。
- 客户 BFF 组合“我的资产 / 创作结果 / 最近使用”，但不创建超级 `Asset` 写模型。
- 搜索候选、来源证据、知识和权利记录只有在客户明确导入时才建立工作区资料引用。

退出条件：真实试点客户完成“上传或导入 -> 整理 -> 预览 -> 加入创作”；关闭工作区开关不会影响已有创作结果复用。

### F2：逻辑模块和窄接口

交付：

- 新代码停止扩大全局 `store.Store`、`app.Service` 和 `web/src/types.ts`。
- 先为 Work、Catalog、Studio、Experience Projection 和 Runtime 建立窄 Repository / Command / Query 端口。
- Studio BFF 和 Operations BFF 的 DTO 与权限分离。
- 架构依赖检查进入 CI。
- Memory 与 PostgreSQL 对同一模块运行 Repository 契约测试。

退出条件：首个切片的新行为只依赖窄模块端口；现有宽接口通过 Adapter 兼容，新增调用量为零或持续下降。

### F3：Runtime 旁路内核（已完成）

对应 V8 的基础工作包：JobRun、JobEvent、JobPlanRevision 和线性调度。

交付：

- 现有 SOP 确定性编译为不可变 JobPlanRevision。
- 新调度器先旁路计算 ready 状态，不接管生产写入。
- 对冻结的阶段基线比较 Node 判断、输入摘要和客户投影。
- FakeHarness 覆盖租约、失败、重放和乱序事件。

退出条件：代表性内置 SOP 的旁路摘要一致；任何差异可定位；重放不产生执行或外部副作用。

### F4：Runtime 分阶段接管（执行事实已切流）

顺序：

1. 新 JobRun 成为一次完整执行记录。
2. NodeRun 和独立 RuntimeAttempt 成为唯一执行事实；V7 执行表、RunAttempt 和 daemon 命令链已删除。
3. StageRun 和 TaskRun 只通过单向投影表达业务阶段与运行摘要，不拥有租约或终态。
4. 状态、ContextView、预算和 Effect 台账上线。
5. 先对试点租户、单一低风险能力切流。
6. 观测完整窗口后扩大能力和租户。

退出条件：所有执行读取和写入只使用 Runtime；中断、重复、外部 unknown 和人工 Gate 故障测试通过。回退只允许停止新准入并前向修复 Runtime，不恢复旧权威路径。

### F5：运营产品化

交付体验模板完整生命周期：

```text
draft -> linted -> preview -> canary -> published -> deprecated -> retired
                         |          |
                         +-> failed +-> rolled_back
```

包括租户启用、版本固定、有限覆盖项、能力健康、成本、运行诊断和回退。运营不能编辑任意脚本或构造无限流程。

退出条件：运营人员不修改代码即可发布一个使用已批准原语和业务包的新场景版本，并完成 Canary 与回退演练。

### F6：第二业务流与动态能力

- 使用结构不同的文章或复盘流程验证 Runtime 无业务硬编码。
- 在共享状态、Effect、检查点和恢复通过后，再开放受限动态图。
- 验证 fan-out、汇聚、取消、迟到节点、预算和节点规模上限。

退出条件：第二业务流不增加专用调度表和内容类型分支；容量、安全和恢复门禁通过。

### F7：兼容清理与基线完成（执行链清理已完成）

- 保持 StageRun/TaskRun 为只读业务投影，下一版公开 API 统一 Runtime 术语后删除兼容 DTO。
- V7 执行 Adapter、双读、写路由、Store 和物理表已删除；继续清理其他达到门槛的兼容面。
- 继续按模块拆除全局 Store/Service 中已迁移接口。
- 更新对外文档、架构图、运行手册和能力登记。

退出条件：公开运行 API 不再暴露 TaskRun 兼容命名，代码搜索无 V7 执行依赖，生产故障与回退演练通过。

### 3.1 运营控制面专项阶段

FND-07 进一步拆为 O0-O7，详细页面和验收见
[《运营后台工作流、上线与清理计划》](../product/operations-control-plane/03-workflows-and-migration.md)：

```text
O0 统一说法与边界
O1 运营后台外壳与总览
O2 创作产品发布中心
O3 能力、执行方式和技能包目录
O4 绑定规则与模拟器
O5 Runtime Explorer 产品化
O6 创作结果治理
O7 兼容清理
```

这组阶段不替换 F0-F7 的 Runtime 迁移顺序，而是规定运营产品何时接入对应能力。任何运营页面都必须先有明确的事实来源、权限和回退方式，不能为了“先做一个页面”创建第二套权威状态。

## 4. 工作包

| ID | 工作包 | 依赖 | 主要产物 |
| --- | --- | --- | --- |
| FND-00 | 事实地图与 ADR | - | 对象、表、API、路由、状态和所有权清单 |
| FND-01 | Studio 客户切片 | FND-00 | Shell、Experience、灵感采集、客户 Gate |
| FND-01A | 创作资产目录闭环 | FND-00、FND-01 | 统一目录投影、资产库、固定引用、失效治理 |
| FND-01B | 我的资产工作区 | FND-00、FND-01A | Folder/Material 契约、上传/导入、处理状态和组合入口 |
| FND-02 | 产品目录与发布 | FND-00 | Experience 生命周期、租户启用、版本固定 |
| FND-03 | 模块边界与窄端口 | FND-00 | Work/Catalog/Studio/Runtime 端口与架构检查 |
| FND-04 | Runtime 旁路编译 | FND-00、V8 W8-02/03 | JobRun、JobPlanRevision、对账报告 |
| FND-05 | 线性 Runtime 切流 | FND-03、FND-04、V8 W8-04 | Node/Attempt 映射、调度与回退 |
| FND-06 | Context 与 Effect | FND-05、V8 W8-05/07 | 最小上下文、状态、外部操作与对账 |
| FND-07 | [运营控制面](../product/operations-control-plane/README.md) | FND-01A/01B、FND-02、FND-05/06 | 发布、资产治理、Canary、诊断、费用和运行手册；按 O0-O7 交付 |
| FND-08 | 第二业务流 | FND-05/06/07 | 无业务硬编码的通用性证据 |
| FND-09 | 动态图与容量 | FND-06、FND-08、V8 W8-09/10 | 受限动态 DAG、检查点和规模门禁 |
| FND-10 | 兼容退场 | 所有前置 | 删除旧路径、物理目录收敛、文档更新 |

## 5. 依赖图

```text
FND-00
├── FND-01 -> FND-01A -> FND-01B ------┐
├── FND-02 ------------------------------┤
├── FND-03 -----------┐                  │
└── FND-04 -----------+-> FND-05 -> FND-06
                                         |
                                         v
                                      FND-07
                                         |
                                         v
                                      FND-08 -> FND-09 -> FND-10
```

FND-01 可以与 Runtime 基础并行，但只能使用已冻结契约，不能创建临时任务状态作为捷径。

## 6. 并行开发线

| Lane | 顺序 | 模块 | 冲突规则 |
| --- | --- | --- | --- |
| A 客户价值 | FND-01 -> FND-01A -> 后续客户阶段 | `web/studio`、Studio BFF、体验投影 | 不修改 Runtime 内部状态机和业务事实所有权 |
| B 产品控制面 | FND-02 -> FND-07 | Catalog、Admin、租户启用 | 与 A 共享 Experience 契约，先冻结再并行 |
| C Runtime | FND-04 -> FND-05 -> FND-06 -> FND-09 | Runtime、scheduler、state、effect | 严格顺序合并公共状态接口 |
| D 模块迁移 | FND-03 -> FND-10 | domain/app/store/httpapi | 一次只迁移一个业务模块 |
| E 质量运行 | 横切所有阶段 | tests、observability、runbooks | 每个阶段同步交付，不留到最后 |

同时修改同一全局旧文件时不得并行迁移；先合并窄端口，再让不同 Lane 依赖它。

## 7. 现有对象与模块处置矩阵

| 对象或模块 | 状态 | 目标动作 | 目标所有者 | 删除/完成条件 |
| --- | --- | --- | --- | --- |
| `WorkTask` | `current` | 保持业务语义，增加体验和 Job 引用 | Work | 不吸收 Runtime 节点状态 |
| `StageRun` | `compat` | 收敛为客户阶段投影 | Work projection / Runtime adapter | Node 投影等价并停止旧权威写入 |
| `TaskRun` | `compat` | 仅保留 Runtime 只读业务 DTO，不对应执行表 | Experience / Runtime projection | 下一版公开 API 统一 Runtime 术语 |
| `RunAttempt` | `dead` | 表、领域对象、Store 和 API 已删除 | Runtime | 禁止恢复；历史仅保留在迁移 evidence |
| `JobRun` / `NodeRun` / `RuntimeAttempt` | `current` | 继续完善 V8 执行、租约和恢复 | Runtime | 生产门禁通过；不与 V7 执行状态双写 |
| `SOPVersion` / Stage / Gate | `current` | 增量扩展并由编译器直接消费 | Catalog | 不创建平行业务流水线定义 |
| `Capability` / catalog | `current` | 统一数据分类、副作用和执行模式 | Catalog | 发布和准入使用同一版本化契约 |
| `ProjectTemplate` | `compat` | 与 ExperienceTemplate 消除命名和职责冲突 | Workspace | 新项目入口不再依赖重叠语义 |
| `ProjectProjection` | `current` | 扩展 CustomerJourneyProjection | Experience | 可从权威对象确定性重建 |
| `Source` / `SourceRevision` | `current` | 保持来源事实，仅通过 lineage 影响结果 | Source | 不直接写客户结果状态 |
| `Asset` / `RightsRecord` | `current` | 保持参考素材与权利治理语义 | Source | 上传资料可进入工作区投影，但不进入结果状态机 |
| `ApprovedSnapshot` | `current` | 保持批准事实并驱动结果投影 | Review | 不新增重复批准状态 |
| `Artifact` / `DeliveryPackage` | `current` | 保持交付事实；分别投影媒体结果和交付视图 | Delivery | 不复制资产或交付正文 |
| `CreativeAssetCatalogItem` | `current` | 仅投影五类生成结果 | Experience | 可重建，禁止成为统一写模型 |
| `WorkspaceFolderItem` / `WorkspaceMaterialItem` | `current` | 独立表达资料组织、摘要和处理状态 | Experience / Workspace | 不扩张结果目录契约 |
| `store.Store` | `deprecated` | 迁移到各模块窄端口 | 各模块 ports | 所有方法迁移且新增依赖为零后删除 |
| `app.Service` | `deprecated` | 拆分为各模块 application service | 各模块 application | 全局依赖清零后删除 |
| `internal/domain` | `compat` | 按聚合迁移到业务模块 | 各业务模块 | 无反向导入且旧包引用归零 |
| `internal/httpapi` | `compat` | 拆分 Studio/Admin/Agent/Public transport | Transport | 旧聚合 handler 引用归零 |
| `agentadapter` | `current` | 收敛命名并支持 Node contract 与恢复 | Integration Agent | 通过适配器一致性测试 |
| `environment` | `compat` | 拆分配置、绑定与信任职责 | Catalog / Runtime / Integration | 旧聚合入口引用归零 |
| `mediapipeline` | `compat` | 拆分业务状态与 Provider SDK 调用 | Delivery / Provider | 新旧服务商路径完成对账并切流 |
| Workspace Shell | `dead` | 已迁移知识库、任务、资产和团队入口后删除 | Studio | 旧 Web 路由不再注册 |
| Admin Shell | `current` | 扩展 Experience 发布和 Runtime Explorer | Operations | 权限、审计和诊断门禁通过 |
| `REDESIGN_PLAN.md` | `dead` | 删除 | Documentation | 无引用，内容已被 `DESIGN.md` 和现有实现取代 |
| `docs/roadmap/content-work-os`、`plugin`、`v1-v7` | `dead` | 删除旧主动路线图 | Documentation | 当前文档无链接、实现与迁移证据已由代码、ADR、Foundation、V8 和变更记录承接 |

## 8. Feature Flag 与权威切换

Runtime 执行事实已经切流，不再用 Feature Flag 在新旧执行模型之间切换。剩余开关只允许控制增量能力和产品发布范围：

- Studio 场景是否可见和可创建。
- 动态图、真实 Provider/SDK 和实验性执行器是否准入。
- 新客户/运营投影是否可见；投影关闭不改变 Runtime 权威写入。
- 创作资产目录是否旁路构建、客户可见和允许创建新任务引用。

禁止用开关恢复 V7 表、命令或调度写入，也禁止用一个全局开关同时切换 UI、数据库写入、调度和投影。回退只能停止新准入、关闭增量能力或切换读投影版本，不能隐藏已经产生的 Runtime 历史。

## 9. 迁移数据流

```text
Runtime 权威写入
  -> JobEvent + runtime_outbox
  -> current projector
  -> TaskRun / RunProgressEvent compat DTO（仍有公开调用时）
  -> 客户与运营读模型

剩余产品面迁移
  -> 新读模型 shadow rebuild
  -> digest comparison
       ├── equal -> metric success
       └── diff  -> alert + sample + block cohort expansion
```

Runtime 状态禁止双写。其他业务域若无法用事件或投影桥接，必须通过 ADR 定义原子性、失败补偿和明确窗口。

## 10. 回退顺序

```text
发现异常
  -> 停止扩大 cohort
  -> 停止新 JobRun 准入或关闭增量能力
  -> 暂停危险外部 Effect
  -> 确认 Runtime 事件、outbox 和投影游标
  -> 切换到已兼容当前 Runtime schema 的服务或投影版本
  -> 保留 Runtime 历史，不删除
  -> 对账运行中节点和外部结果
  -> 验证客户下一动作
  -> 复盘后重新 Canary
```

`00034` 已删除 V7 执行表，因此回退不依赖逆向删除迁移，也不允许重建旧执行链。数据库变更必须保持前向兼容读取，事故恢复采用停止准入、排空、暂停和前向修复。

## 11. 里程碑与退出条件

| 里程碑 | 范围 | 退出条件 |
| --- | --- | --- |
| M0 基线冻结 | FND-00 | ADR、事实地图、指标和处置矩阵完成 |
| M1 客户验证 | FND-01 | 灵感采集真实闭环和价值指标可用 |
| M1A 资产复用 | FND-01A | 跨任务复用闭环、失效治理和目录重建通过 |
| M1B 资料工作区 | FND-01B | 上传/导入、文件夹、资料处理和加入创作闭环通过 |
| M2 模块边界 | FND-02/03 | 新代码不扩大旧宽接口，契约测试通过 |
| M3 Shadow Runtime | FND-04 | SOP 旁路编译和投影摘要一致 |
| M4 Runtime 接管 | FND-05/06 | 线性切流、Effect、恢复和前向修复通过 |
| M5 运营产品化 | FND-07 | 发布、Canary、租户启用和回退演练通过 |
| M6 通用性 | FND-08/09 | 第二业务流和受限动态图通过 |
| M7 基线完成 | FND-10 | V7 执行链保持 dead、兼容 DTO 退场、目录收敛、Runtime 历史可读 |

## 12. 风险登记

| 风险 | 级别 | 缓解 |
| --- | --- | --- |
| 客户价值等 Runtime 完成才出现 | P0 | FND-01 与基础工作并行，使用现有线性内核 |
| Run 对象形成第三套状态 | P0 | FND-00 ADR 先于数据库实现 |
| Runtime 吞并业务事实 | P0 | 事实所有权、引用契约和模块依赖检查 |
| 资产目录成为第二套业务事实 | P0 | 只读投影、固定 subject ref、回源校验和重建测试 |
| 原始候选污染资产库 | P1 | 搜索候选只保留为任务参考；进入我的资产必须有明确导入动作 |
| 工作区资料与结果 DTO 合并 | P0 | 冻结 Folder/Material/Result 窄契约，由客户 BFF 组合 |
| 新旧长期共存 | P0 | compat owner、指标、最晚阶段和退场门槛 |
| 目录搬迁导致大范围冲突 | P1 | 先端口后移动，一次迁移一个模块 |
| 运营配置空间不可测试 | P1 | 受限体验原语、批准组合、lint 和 Canary |
| Agent 宿主恢复能力不稳定 | P1 | 可选执行者，Fake/Worker 基本路径，不阻塞 Studio |
| 外部任务重复收费 | P0 | Effect、unknown、对账和费用预留 |
| 单体继续膨胀 | P1 | 模块依赖规则和按指标拆服务决策 |
| 文档成为新一套虚假事实 | P0 | 目标/当前标记、对外文档只发布已验证能力 |

## 13. 完成定义

- 客户通过简单 Studio 完成至少两条结构不同的创作流程。
- 运营可以不改代码发布、Canary、启用、回退和停用已批准场景版本。
- Runtime 执行事实、业务域事实和产品投影各有唯一所有者。
- 客户可以把已保存或已批准内容固定引用到新任务，每次完成创作都能形成可治理的下一次输入。
- 新流水线不修改 Runtime 核心状态和内容类型专用调度表。
- 旧宽 Store/Service 不再接受新依赖，已迁移接口完成退场。
- 安全、契约、故障、容量、可访问性和回退门禁全部通过。
- 对外文档、能力登记和产品页面只宣称真实可用能力。
