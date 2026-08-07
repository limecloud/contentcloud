# 渐进迁移与交付计划

状态：`目标实施计划；Runtime 第一批内核、存储、脱敏 BFF 和运营 Explorer 首版已落地，接管、动态执行图和生产排期仍需 ADR/POC`。

更新时间：2026-08-07。

## 1. 迁移策略

```text
长期架构：平台基线重构
交付路径：产品层适配
客户验证：简单 Studio 纵向切片
迁移方法：Strangler + 旁路比较 + Canary + 分阶段切流
```

不执行停机式大重写。新旧路径在有限窗口内共存，但只有一个权威写入方；旁路结果使用摘要比较，不悄悄形成第二套状态。

## 2. 双轨交付

```text
价值轨
Studio Shell -> Experience -> 灵感采集 -> 客户选择 -> 下游固定输入
     |              使用当前线性 SOP / WorkTask / StageRun / TaskRun
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
- WorkTask、StageRun、TaskRun、JobRun、NodeRun、RunAttempt 处置 ADR。
- Studio-first 定位和旧客户入口迁移 ADR。
- 当前 API、表、契约、路由、投影和执行路径事实地图。
- 基线指标、Feature Flag 和回退责任人。

退出条件：任何新 Runtime 表或新 Studio 权威状态出现前，上述 ADR 已批准；每个现有对象标记 `Reuse / Extend / Rename / Compat / Deprecate / Retire`。

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
- 客户“品牌资料”入口在试点租户演进为“资产库”，但旧品牌资料和输入型数据迁移到任务输入或项目参考，旧路由保持兼容读取。
- 完成“结果生成 -> 客户确认 -> 资产目录出现 -> 固定引用进入新任务 -> 新结果再次沉淀”的闭环。
- 运营侧处理权利待办、失效、重复组、投影延迟和影响范围。

退出条件：真实试点客户完成一次跨任务复用；权利过期、来源失效、跨租户和陈旧投影均不能创建不安全的新引用；目录重建结果确定且无外部副作用。

### F2：逻辑模块和窄接口

交付：

- 新代码停止扩大全局 `store.Store`、`app.Service` 和 `web/src/types.ts`。
- 先为 Work、Catalog、Studio、Experience Projection 和 Runtime 建立窄 Repository / Command / Query 端口。
- Studio BFF 和 Operations BFF 的 DTO 与权限分离。
- 架构依赖检查进入 CI。
- Memory 与 PostgreSQL 对同一模块运行 Repository 契约测试。

退出条件：首个切片的新行为只依赖窄模块端口；现有宽接口通过 Adapter 兼容，新增调用量为零或持续下降。

### F3：Runtime 旁路内核

对应 V8 的基础工作包：JobRun、JobEvent、JobPlanRevision 和线性调度。

交付：

- 现有 SOP 确定性编译为不可变 JobPlanRevision。
- 新调度器只旁路计算 ready 状态，不接管生产写入。
- 对现有任务比较旧阶段判断、新 Node 判断、输入摘要和客户投影。
- FakeHarness 覆盖租约、失败、重放和乱序事件。

退出条件：代表性内置 SOP 的旁路摘要一致；任何差异可定位；重放不产生执行或外部副作用。

### F4：Runtime 分阶段接管

顺序：

1. 新 JobRun 成为一次完整执行记录。
2. 根据 ADR 将现有 TaskRun 扩展/重命名为 NodeRun，或通过有限兼容映射迁移。
3. RunAttempt 关联 NodeRun，并保留原租约和心跳能力。
4. 状态、ContextView、预算和 Effect 台账上线。
5. 先对试点租户、单一低风险能力切流。
6. 观测完整窗口后扩大能力和租户。

退出条件：旧权威路径可一键恢复；新旧状态持续对账；中断、重复、外部 unknown 和人工 Gate 故障测试通过。

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

### F7：兼容清理与基线完成

- 停止旧 StageRun/TaskRun 权威写入或按 ADR 保留其最终角色。
- 删除已达到退场门槛的 Adapter、双读和兼容路由。
- 完成目标模块物理迁移，拆除全局 Store/Service 中已迁移接口。
- 更新对外文档、架构图、运行手册和能力登记。

退出条件：零活跃旧绑定、历史可读、回退演练通过、兼容指标为零且代码搜索无新依赖。

## 4. 工作包

| ID | 工作包 | 依赖 | 主要产物 |
| --- | --- | --- | --- |
| FND-00 | 事实地图与 ADR | - | 对象、表、API、路由、状态和所有权清单 |
| FND-01 | Studio 客户切片 | FND-00 | Shell、Experience、灵感采集、客户 Gate |
| FND-01A | 创作资产目录闭环 | FND-00、FND-01 | 统一目录投影、资产库、固定引用、失效治理 |
| FND-02 | 产品目录与发布 | FND-00 | Experience 生命周期、租户启用、版本固定 |
| FND-03 | 模块边界与窄端口 | FND-00 | Work/Catalog/Studio/Runtime 端口与架构检查 |
| FND-04 | Runtime 旁路编译 | FND-00、V8 W8-02/03 | JobRun、JobPlanRevision、对账报告 |
| FND-05 | 线性 Runtime 切流 | FND-03、FND-04、V8 W8-04 | Node/Attempt 映射、调度与回退 |
| FND-06 | Context 与 Effect | FND-05、V8 W8-05/07 | 最小上下文、状态、外部操作与对账 |
| FND-07 | 运营控制面 | FND-01A、FND-02、FND-05/06 | 发布、资产治理、Canary、诊断、费用和运行手册 |
| FND-08 | 第二业务流 | FND-05/06/07 | 无业务硬编码的通用性证据 |
| FND-09 | 动态图与容量 | FND-06、FND-08、V8 W8-09/10 | 受限动态 DAG、检查点和规模门禁 |
| FND-10 | 兼容退场 | 所有前置 | 删除旧路径、物理目录收敛、文档更新 |

## 5. 依赖图

```text
FND-00
├── FND-01 -> FND-01A -------------------┐
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

| 对象或模块 | 状态 | 目标所有者 | 删除/完成条件 |
| --- | --- | --- | --- |
| `WorkTask` | Extend | Work | 增加体验和 Job 引用，保持用户工作语义 |
| `StageRun` | Compat | Work projection / Runtime adapter | Node 投影等价并停止旧写入 |
| `TaskRun` | Proposed Rename/Extend or Compat | Runtime | ADR 冻结，不允许永久双状态 |
| `RunAttempt` | Extend | Runtime | 关联 NodeRun，原历史可读 |
| `SOPVersion` / Stage / Gate | Reuse/Extend | Catalog | 编译器可直接消费 |
| `Capability` / catalog | Extend | Catalog | 统一数据分类、副作用和执行模式 |
| `ProjectTemplate` | Rename or Deprecate | Workspace | 与 ExperienceTemplate 消除语义冲突 |
| `ProjectProjection` | Extend | Experience | CustomerJourneyProjection 可从权威对象重建 |
| `Source` / `SourceRevision` | Reuse | Source | 进入任务输入或项目参考投影；只通过内部 lineage 影响结果可复用状态 |
| `Asset` / `RightsRecord` | Reuse/Extend | Source | 保持参考素材和权利治理语义，不生成客户结果资产目录项 |
| `ApprovedSnapshot` | Reuse | Review | 创建后自动进入目录，不新增批准状态 |
| `Artifact` / `DeliveryPackage` | Reuse | Delivery | Artifact 可形成图片/视频结果；DeliveryPackage 只进入交付视图并推导交付状态 |
| `CreativeAssetCatalogItem` | New Projection | Experience | 迁移期技术契约名，只投影生成结果；可重建，禁止成为统一写模型 |
| `store.Store` | Deprecate | 各模块 ports | 所有方法迁移到窄接口后删除 |
| `app.Service` | Deprecate | 各模块 application | 全局依赖清零后删除 |
| `internal/domain` | Compat | 各业务模块 | 按聚合迁移且无反向导入 |
| `internal/httpapi` | Compat | Transport | Studio/Admin/Agent/Public handler 拆分 |
| `agentadapter` | Extend/Rename | Integration Agent | 支持 Node contract 和恢复能力 |
| `environment` | Split | Catalog/Runtime/Integration | 配置、绑定、信任各归其主 |
| `mediapipeline` | Split | Delivery/Provider | 业务状态与 SDK 调用分离 |
| Workspace Shell | Compat/Deprecate | Studio | 关键流程迁移且旧路由访问量归零 |
| Admin Shell | Reuse/Extend | Operations | 增加 Experience 发布和 Runtime Explorer |

## 8. Feature Flag 与权威切换

具体名称由实现确定，但必须覆盖五个独立维度：

- Studio 场景是否可见和可创建。
- 新计划编译是否只旁路、双读或权威。
- 新 Runtime 是否接管调度写入。
- 新客户/运营投影是否权威展示。
- 创作资产目录是否旁路构建、客户可见和允许创建新任务引用。

禁止用一个全局开关同时切换 UI、数据库写入、调度和投影。回退需要可以单独恢复旧权威路径，而不隐藏已经产生的新历史。

## 9. 迁移数据流

```text
旧权威写入
  -> Compat event/ref
  -> 新模型 shadow apply
  -> digest comparison
       ├── equal -> metric success
       └── diff  -> alert + sample + block cohort expansion

切流后
新权威写入
  -> legacy compatibility projection when required
  -> old reader comparison
```

双写只在无法用事件或投影桥接时使用，并且需要原子性、失败补偿和明确窗口。

## 10. 回退顺序

```text
发现异常
  -> 停止扩大 cohort
  -> 暂停危险外部 Effect
  -> 确认新旧写入和投影游标
  -> 切回旧权威调度/投影 Flag
  -> 保留新历史，不删除
  -> 对账进行中任务和外部结果
  -> 验证客户下一动作
  -> 复盘后重新 Canary
```

数据库破坏性清理与切流分开发版，所以回退不依赖逆向删除迁移。

## 11. 里程碑与退出条件

| 里程碑 | 范围 | 退出条件 |
| --- | --- | --- |
| M0 基线冻结 | FND-00 | ADR、事实地图、指标和处置矩阵完成 |
| M1 客户验证 | FND-01 | 灵感采集真实闭环和价值指标可用 |
| M1A 资产复用 | FND-01A | 跨任务复用闭环、失效治理和目录重建通过 |
| M2 模块边界 | FND-02/03 | 新代码不扩大旧宽接口，契约测试通过 |
| M3 Shadow Runtime | FND-04 | SOP 旁路编译和投影摘要一致 |
| M4 Runtime 接管 | FND-05/06 | 线性切流、Effect、恢复、回退通过 |
| M5 运营产品化 | FND-07 | 发布、Canary、租户启用和回退演练通过 |
| M6 通用性 | FND-08/09 | 第二业务流和受限动态图通过 |
| M7 基线完成 | FND-10 | 旧权威路径退场、目录收敛、历史可读 |

## 12. 风险登记

| 风险 | 级别 | 缓解 |
| --- | --- | --- |
| 客户价值等 Runtime 完成才出现 | P0 | FND-01 与基础工作并行，使用现有线性内核 |
| Run 对象形成第三套状态 | P0 | FND-00 ADR 先于数据库实现 |
| Runtime 吞并业务事实 | P0 | 事实所有权、引用契约和模块依赖检查 |
| 资产目录成为第二套业务事实 | P0 | 只读投影、固定 subject ref、回源校验和重建测试 |
| 原始候选污染资产库 | P1 | 只收录保存、选择、批准和正式交付内容 |
| 新旧长期共存 | P0 | Compat owner、指标、最晚阶段和退场门槛 |
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
