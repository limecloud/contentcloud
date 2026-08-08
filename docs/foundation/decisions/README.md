# 架构决策记录规范

状态：`规范已定义；候选 ADR 待联合评审后逐项接受`。

更新时间：2026-08-07。

## 1. 目的

ADR 记录会长期影响产品、领域、数据和工程边界的决策及其理由。它不是会议纪要，也不重复实现细节。新工程师应该能通过 ADR 理解“为什么这样设计、放弃了什么、怎样回退”。

## 2. 状态

```text
Proposed -> Accepted -> Superseded
    |           |
    v           v
Rejected     Deprecated -> Retired
```

Accepted 决策只能由新的 ADR 明确替代，不能静默修改旧文档抹去历史。

## 3. 命名

```text
ADR-0001-studio-first-product-positioning.md
ADR-0002-product-plane-separation.md
```

编号一旦分配不复用。标题描述决策，不使用“架构优化”一类空泛名称。

## 4. 模板

```markdown
# ADR-NNNN：标题

状态：Proposed / Accepted / Rejected / Superseded / Deprecated / Retired
日期：YYYY-MM-DD
决策者：角色或小组
关联：需求、路线图、PR、前序 ADR

## 背景
当前事实、业务问题、约束和为什么现在必须决定。

## 决策
用可验证语言说明采用什么，不采用什么。

## 备选方案
每个方案的收益、代价、风险和为什么未采用。

## 事实所有权与边界
谁拥有状态，谁只能读取或引用。

## 兼容与迁移
旧对象、API、数据和调用怎样共存与退场。

## 安全与运行影响
权限、数据披露、故障、监控和运行手册变化。

## 验证
测试、指标、Canary 和退出条件。

## 回退
触发条件、回退步骤和不可逆影响。

## 后果
正面、负面和新增约束。
```

## 5. 必须写 ADR 的变更

- Studio-first 产品定位和本地客户端角色变化。
- 客户、租户管理、平台运营和 Runtime Explorer 产品面变化。
- 业务事实所有权或聚合边界变化。
- WorkTask、StageRun、V7 TaskRun/RunAttempt 与 V8 JobRun/NodeRun/RuntimeAttempt 的处置。
- 模块化单体与独立服务边界变化。
- Schema major、事件排序、摘要或幂等语义变化。
- 新执行模式、能力绑定、数据披露和外部副作用类型。
- 兼容路径超期保留或绕过平台固定不变量。

## 6. 首批候选 ADR

| 编号 | 候选决策 | 当前建议 | 阻断内容 |
| --- | --- | --- | --- |
| ADR-0001 | Studio-first 产品定位 | Web Studio 为默认入口，本地 Agent 为高级执行面 | 面向用户的导航与定位迁移 |
| ADR-0002 | 产品面分离 | Studio 与 Admin 独立 Shell、路由、权限和 BFF | 客户创作台实现 |
| ADR-0003 | 事实所有权 | Runtime 只拥有执行事实，业务域拥有业务事实 | Runtime 数据模型 |
| ADR-0004 | 运行对象收敛 | 冻结 WorkTask/Job/Node/Attempt 与 StageRun/TaskRun 关系 | V8 W8-02 及数据库迁移 |
| ADR-0005 | 模块化单体 | 先用包、端口和权限隔离，暂不拆微服务 | 目标代码目录与部署 |
| ADR-0006 | 能力与执行绑定 | 能力声明与具体执行者分离，运行前固定绑定 | Catalog 与 Runtime 编译 |
| ADR-0007 | 双投影单状态 | CustomerJourney 与 Operations 从同一权威状态生成 | BFF 与投影实现 |
| ADR-0008 | 外部副作用 | Effect 台账、unknown 对账、补偿不删除历史 | Provider 生产切流 |
| ADR-0009 | 契约版本 | 显式 major/minor、摘要固定和兼容窗口 | 新 Schema 与业务包 |
| ADR-0010 | 受限体验原语 | 运营组合批准原语，不建设任意低代码页面 | ExperienceTemplate |
| [ADR-0011](./ADR-0011-creative-asset-catalog-projection.md) | 统一创作资产目录 | 使用只读投影引用生成结果，不扩张 `Asset` 语义；收录范围由 ADR-0013 修订 | 资产目录投影与复用契约 |
| [ADR-0012](./ADR-0012-customer-asset-library-information-architecture.md) | 客户资产库信息架构（历史） | “品牌资料”演进为跨任务资产库，已由 ADR-0013/0014 取代分类与入口设计 | 旧路由兼容与历史迁移 |
| [ADR-0013](./ADR-0013-customer-result-asset-boundary.md) | 客户创作结果边界 | 冻结五类生成结果、结果状态与复用门禁；客户入口范围由 ADR-0014 扩展 | 结果 DTO、目录收录与复用门禁 |
| [ADR-0014](./ADR-0014-customer-asset-surface.md) | 客户资产入口 | 一个“资产”入口组合工作区资料与创作结果两个专用投影，不创建超级 Asset | 客户信息架构、资料契约与 BFF 组合 |
| [ADR-0015](./ADR-0015-operations-control-plane.md) | 独立的平台运营后台 | 按创作产品、能力与执行、运行诊断、创作结果治理分区；客户面与运营面完全分离 | Operations BFF、发布中心、绑定规则、Runtime Explorer 和资产治理 |
| [ADR-0016](./ADR-0016-runtime-command-kernel.md) | Runtime 统一事务命令内核 | 快照、JobEvent 和 outbox 在同一命令提交边界内完成；Service 不再组合宽写方法 | RuntimeCommandStore、outbox、故障注入和旧写路径退场 |

候选编号在正式创建 ADR 文件时确认。若已有项目 ADR 编号体系，应迁入现有体系而不是并行编号。

## 7. 评审规则

一个 ADR 只有同时回答以下问题才可 Accepted：

1. 决策解决了客户或平台的哪个真实问题？
2. 当前代码和数据事实是什么？
3. 谁拥有状态，哪些模块受到影响？
4. 兼容、迁移、回退和删除条件是什么？
5. 安全、成本、运行和开发体验代价是什么？
6. 哪些测试和指标证明决策有效？
7. 如果六个月后判断错误，如何改回或替代？
