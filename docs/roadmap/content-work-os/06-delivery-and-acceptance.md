# 交付与验收

## 1. 交付策略

方案分阶段交付，每一阶段都必须有可运行的基础设施和可验证的业务结果。不能先堆完整 UI，再补数据契约。

```text
契约和 Registry
  -> Task/Run 纵向切片
    -> 内容生产和治理
      -> SOP Builder/Admin
        -> Task-first 主工作区
          -> 迁移、试点和交接
```

每阶段都保留可回退的旧读路径，但不允许新旧路径双写同一正式事实。

## 2. P0：冻结契约

### 交付内容

- Task、TaskRun、StageRun、SOP、Stage、Gate Schema；
- Environment、Tenant、Project 作用域；
- Task、Run、Revision、Decision、Snapshot、Delivery 状态关系；
- Gate 模式和不可关闭安全门禁；
- 权限和审计事件；
- Task Projection、Task Detail Projection、SOP Projection。

### 验收

- 契约测试覆盖合法和非法样例；
- 能解释每个用户状态对应的事实来源；
- 不再存在“禁止通用 Task”或“审批固定必选”的规则；
- 所有未解决的对象命名和范围决策记录在案。

## 3. P1：基础设施

### 交付内容

- SOP Registry、版本和 digest；
- Environment Manifest；
- Capability、Agent、Automation、Swarm Registry；
- Role Policy、审计和用量事件；
- SOP lint、影响分析、发布和退休；
- Task 创建前的作用域、能力和输入检查。

### 验收

- 两个 Environment 可配置不同 SOP；
- 同一 Project 可以绑定已发布 SOP Version；
- 新 SOP 发布不改变历史 TaskRun；
- 能力关闭会阻止新 Run，并显示恢复入口；
- 版本和 digest 可以离线校验。

## 4. P2：Task/Run 纵向切片

### 交付内容

- 新建 Task；
- Task 列表、筛选和分页；
- 领取、开始、暂停、恢复、取消；
- StageRun、ExecutionAttempt 和 next action；
- 本地 Workspace、Agent 和 Automation 至少两种执行方式；
- Revision publish 和检查摘要；
- claim 冲突、lease 过期、失败和恢复。

### 验收

- 新建 Task 后能稳定生成 TaskRun 和第一条下一动作；
- 同一 StageRun 不能被两个执行者同时写入；
- 本地未 publish 内容不出现在 Web；
- 失败后可以补料、重试或转人工；
- 运行摘要、Revision 和 Task 状态不互相矛盾。

## 5. P3：内容生产和可选 Gate

### 交付内容

- 视频脚本 Schema 和 SOP；
- 文章 Schema 和 SOP；
- Evidence、Rights、Claim、引用和内容检查；
- `none`、`required_check`、`internal_review`、`client_decision` Gate；
- AcceptedSnapshot 和 DeliveryPackage；
- 结果观察导入入口。

### 验收

- 低风险流程可以没有人工审批并完成交付；
- 高风险流程可以配置内部或客户 Gate；
- Gate 退回生成新 Revision，不覆盖旧版本；
- Rights 或 Evidence 硬失败始终阻断；
- Delivery 只能使用有效 AcceptedSnapshot。

## 6. P4：SOP Builder 和后台

### 交付内容

- 模板选择和空白 SOP；
- Stage 添加、删除、排序和详情；
- 输入、输出、角色、能力、检查、Gate、指标和升级配置；
- 草稿、发布、退休、回滚和版本 diff；
- 样例 Task 和样例失败路径；
- Agent、Automation、Swarm、权限、审计和用量页面。

### 验收

- 流程负责人无需改代码即可复制并调整 SOP；
- 发布前能发现 Schema、能力、Gate 和权限错误；
- 发布结果可追踪到操作者、版本、digest 和影响 Project；
- 回滚不影响已运行 TaskRun；
- 复杂配置只在后台出现，普通用户界面保持 Task-first。

### 4.1 内置模板和升级验收

- 新租户恰好安装四条平台基础 SOP：资料与知识建设、短视频生产、文章协作、活动结果复盘。
- 旧短视频 SOP 只有在结构精确匹配时才追加新版本；原版本、digest、Environment/Project/Task 绑定保持不变。
- 同名但结构不同的自定义 SOP 不被标记为内置，也不被平台模板覆盖。
- 从现有 V3 基线升级时，Project、Source、Evidence、Rights、ContextSnapshot 和旧 TaskRun 可继续读取；新工作区只新增 SOP binding 和新 WorkTask，不覆盖旧事实。
- 检测到不连续或未知的数据库迁移历史时，启动明确失败并提示导出/重建，不执行不可审计的自动回填。

## 7. P5：主工作区和迁移

### 交付内容

- 工作区：新建任务、收件箱、聊天、我的任务、所有任务、自动化、智能体、协作群组、用量；
- Project：任务、SOP；
- Task Detail 深链到审核、Evidence、Rights、Delivery 和 Learning；
- 旧 Project/Stage 主导航的迁移提示和删除开关；
- 旧 ProjectProjection 到新 Task Projection 的对账工具；
- 旧模板和旧 Assignment 的只读迁移视图。

### 验收

- 普通用户默认从 Task 进入工作；
- 旧页面不会与新页面写入不同状态；
- 所有旧记录都能找到对应的新上下文或明确标记为历史；
- 删除旧主导航前完成真实用户试用和数据对账；
- 失败时可以回到旧读路径，不产生双写。

## 8. 测试矩阵

### 8.1 单元和纯函数

- SOP Schema、Stage 顺序和 Gate 组合校验；
- Task/TaskRun/StageRun 状态计算；
- next action 选择；
- digest、版本和时间窗口；
- 权限和作用域规则；
- Revision/Gate/Decision/Snapshot/Delivery 绑定。

### 8.2 集成测试

- Environment 发布不同 SOP；
- Task 创建和 Project/SOP 绑定；
- Local/Agent/Automation 执行；
- claim、lease、取消、恢复和重试；
- Revision publish、Gate、Decision 和 Delivery；
- Capability 关闭、SOP 退休和权限变更。

### 8.3 端到端测试

```text
创建低风险脚本 Task
  -> 选择 SOP
  -> LocalRun
  -> publish Revision
  -> checks
  -> AcceptedSnapshot
  -> DeliveryPackage
```

```text
创建高风险文章 Task
  -> 选择 SOP
  -> AgentRun
  -> Rights blocker
  -> 补料 Task
  -> 新 Revision
  -> Internal Review
  -> Client Decision
  -> DeliveryPackage
```

### 8.4 安全和隐私

- 跨租户、跨 Project 和越权角色；
- 未 publish 本地数据披露；
- 恶意 URL、路径、focus 和 resource link；
- 伪造 digest、重复 Decision 和旧 Revision 写入；
- 能力关闭、成员撤销和 Session 失效；
- 审计完整性和敏感字段脱敏。

### 8.5 前端视觉和交互

- 1440×1000、1280×720、768×1024、390×844；
- `scrollWidth <= clientWidth`；
- 键盘访问、focus-visible、读屏顺序；
- Loading、Stale、Forbidden、Not Found、Capability Blocked、Projection Failed；
- SOP Builder Stage 轨道稳定，不因文字或状态改变尺寸；
- 状态文本和图标不依赖颜色单独表达。

## 9. 迁移和删除规则

### 9.1 允许保留

- 历史 ProjectProjection 只读；
- 已完成 Revision、Decision、Snapshot、Delivery；
- 旧深链的安全重定向；
- CLI/MCP 的兼容参数，直到新 Task API 覆盖同一能力。

### 9.2 必须删除或合并

- 禁止 Task 的产品规则；
- 把每个领域页面作为主入口的导航；
- Project 上堆叠 Charter、Stage、Gate 和指标字段；
- 前端单独维护的状态机；
- 与 Revision、Decision、Delivery 重复的“完成记录”；
- 只为兼容旧页面而保留的第二套投影事实。

### 9.3 删除前检查

1. 新 Task Projection 和旧 ProjectProjection 对账；
2. 真实租户完成至少一轮低风险和一轮高风险 Task；
3. 所有旧记录都有历史映射或明确未迁移原因；
4. 兼容重定向和 API 监控已部署；
5. 删除动作有回滚窗口和备份；
6. 业务负责人确认旧主路径不再承担日常工作。

## 10. 发布门禁

进入真实试点前必须满足：

- `go test ./...` 通过；
- `pnpm --dir web typecheck` 和 `pnpm --dir web build` 通过；
- 契约、安全、运行和端到端测试全部通过；
- 无跨租户或未 publish 数据泄露；
- 两个 Environment 的 SOP 配置已验证；
- 低风险和高风险 Task 各有一条完整录屏或审计链；
- 试点 Runbook、支持码和回滚步骤可执行；
- 交接包能被非研发成员按文档完成一次新 Task。

## 11. 回滚

- Web 主导航可以通过 Feature Flag 切回旧读路径；
- 新 Task 写入失败时不自动创建旧 Assignment，避免双写；
- 已创建 TaskRun 保留，只停止新执行和交付；
- SOP 新版本可以退休，历史 Run 继续只读；
- Projection 错误显示明确支持码，不拼接新旧数据形成假成功状态。
