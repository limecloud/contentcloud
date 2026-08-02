# SOP 目录与历史升级

## 1. 结论

平台内置四条 SOP，当前不再继续增加平台级模板：

| SOP | 解决的问题 | 默认是否阻断 | 适合谁使用 |
| --- | --- | --- | --- |
| 资料与知识建设 | 把 Brief、来源、Evidence、权利和知识候选整理成可引用的事实底座 | 确定性检查阻断 | 策略、项目负责人、资料负责人 |
| 短视频生产 | 从 Brief、知识和策略到脚本、质量检查和交付 | 确定性检查阻断；人工审核不预置 | 内容编辑、策略、项目负责人 |
| 文章协作 | 从 Brief、知识引用到文章写作、检查和交付 | 确定性检查阻断；人工协作可配置 | 编辑、策略、业务负责人 |
| 活动结果复盘 | 把内容版本、渠道观察和指标整理为归因、学习候选和下一动作 | 默认不阻断 | 策略、项目负责人 |

这四条是平台提供的起点，不是客户方法论。每个 Environment 都必须绑定一条已发布 SOP Version；企业可以复制模板，按自己的 Stage、角色、输出、执行方式和 Gate 发布新版本。

不能再为以下差异创建新的平台 SOP：

- 低风险与高风险；
- 不同渠道；
- 是否需要内部审核或客户确认；
- Codex、Claude Code、Workspace 或自动化执行器；
- 批量与单条内容。

这些差异分别由 Gate Policy、Content Schema、Capability、Execution Mode 和 Task 输入表达。这样平台只有一套 Task、Run、Revision、Delivery 和审计事实链，复杂度留在后台配置，不转嫁给普通用户。

## 2. 为什么是四条

四条刚好覆盖当前基础建设和最小业务闭环：

```text
资料与知识建设
        |
        +--> 短视频生产 --> 交付
        |
        +--> 文章协作 --> 交付
                         |
                         +--> 活动结果复盘 --> 下一轮假设
```

资料与知识建设托底事实；短视频和文章验证两种首批业务；复盘把交付结果重新带回下一轮任务。平台不把“图片生成”“发布平台上传”“统一审批”“客户管理”等能力伪装成新的 SOP，它们应当作为能力、交付适配器或可选 Gate 接入。

没有真实客户时，先用四条模板验证基础设施和一条低风险业务链。复盘模板可以先保持可用但低曝光，等有真实结果数据后再扩展指标适配器；不能因为尚未接入数据平台就删除底层契约。

## 3. 版本和绑定规则

每个模板由稳定的 `template_key` 标识，并拥有独立的 SOP Definition、SOP Version 和 digest。

```text
Environment  --绑定-->  SOP Version + digest
Project      --绑定-->  SOP Version + digest
TaskRun      --固定-->  SOP Version + digest
```

规则如下：

1. 新租户第一次进入工作区、后台或创建任务时，幂等安装四条模板的已发布 v1。
2. 重复初始化只补齐缺失模板或缺失的内置版本，不重复创建 Definition、Version 或 Environment。
3. 已发布内置版本不可直接编辑。管理员必须复制为自定义草稿，再保存和发布新版本。
4. 发布新版本不会修改已经创建的 Task、TaskRun、Revision、Gate 或 Delivery。
5. Environment 和 Project 的改绑是显式操作；后台必须先显示影响范围和历史任务数量。
6. 退休版本仍可被历史任务读取，但不能用于创建新的 Task。

## 4. 历史流程如何升级

“升级之前版本”不是一个动作，而是三类输入的不同处理方式。

### 4.1 已存在的旧 SOP Registry

当前服务端已经支持这一类的增量升级：

1. 读取租户当前 SOP Registry；
2. 通过 `template_key` 或平台保留 ID 识别已知内置模板；
3. 对早期短视频流程只接受精确 Stage 结构：`brief -> knowledge -> draft -> delivery`；
4. 保留旧 v1、原 digest 和所有绑定；
5. 在同一个 SOP Definition 下追加当前模板的新版本并发布；
6. Environment、Project 和历史 Task 不自动改绑，管理员确认后才切换后续任务。

同名但结构不同的自定义 SOP 不会被覆盖，也不会因为名称相同就被标记为内置。升级失败时应保留旧 Registry，不执行部分改写。

### 4.2 早期数据库中的业务事实

早期 Project、Source、Source Revision、Evidence、Rights、Context Snapshot 和旧运行记录先通过连续数据库迁移保留。第一次进入新工作区时：

- 为 Project 建立一个明确的 SOP Binding；
- 旧 Run 以只读历史显示；
- 不凭旧状态伪造新的 Revision、Gate Decision 或 Accepted Snapshot；
- 只有经过新的来源、Evidence、权利和对象状态检查，才允许进入新的 Knowledge Snapshot 或交付链。

这保证“事实保留”和“新方法论接入”互不混淆。

### 4.3 早期本地 MVP 文件和对话

本地 MVP 的页面、Markdown、YAML、聊天记录和客户端私有事件不直接写成正式事实。升级流程应由客户端 Adapter 完成格式解析，再生成迁移预览：

| 旧内容 | 新对象候选 | 自动动作 |
| --- | --- | --- |
| Project 基本信息 | Project | 可保留并校验字段 |
| 来源文件与元数据 | Source / Source Revision | 可登记，正文需按披露范围上传 |
| 引用片段 | Evidence | 需要稳定 locator 和 quote digest |
| 七层/多维知识条目 | KnowledgeObject | 先以 candidate 导入，不自动批准 |
| 知识包 | KnowledgePack | 只导入对象引用，发布 Snapshot 需重新检查 |
| Brief、方向、脚本候选 | Task Input / Revision candidate | 绑定新 Task，不能覆盖正式 Revision |
| 本地聊天 transcript | ConversationBundle / InputItem | 只在客户端选择范围、脱敏并确认后导入 |
| 旧审批记录 | 迁移说明或历史 Evidence | 不转换成新的人工 Gate 决定 |

迁移预览必须显示：来源文件、对象候选、缺失字段、冲突、权利缺口、预计影响和需要人工确认的项目。用户确认前不得创建正式 Knowledge、Decision、Snapshot 或 Delivery。

## 5. 升级后的恢复和回滚

升级后出现问题时，恢复入口按影响范围区分：

- 模板版本不合适：从历史已发布版本复制新版本，显式回绑未来 Environment/Project；
- 某个 Project 需要保留旧方法：继续使用原 SOP Version，不要求迁移历史 Task；
- 导入资料不完整：删除候选导入或标记缺口，不删除原 Source；
- Gate 配错：发布修正版 SOP，已有运行按原 digest 继续，运行中的人工决定不被重写；
- 迁移预览与确认不一致：取消本次导入，保留预览审计和原始本地文件摘要。

任何回滚都必须记录操作者、目标版本、影响的 Environment/Project/Task 数量和新 digest。不能通过“重新安装模板”覆盖客户自定义配置。

## 6. 验收标准

- 新租户最终拥有四条且仅四条平台内置模板，重复初始化结果不变；
- 已知旧短视频四 Stage 流程能追加新版本，旧版本、digest 和绑定保持不变；
- 同名但结构不同的自定义 SOP 不被误识别；
- 新版本发布后，历史 TaskRun 仍能按原 SOP 和 digest 重建；
- 早期事实可以读取，但没有未经确认的正式 Knowledge、Decision、Snapshot 或 Delivery；
- 客户可以在后台复制模板、调整 Stage 和 Gate 并发布，不需要修改代码；
- 每个迁移动作都有审计事件和明确恢复入口。
