# Content Work OS：Task-first 内容工作系统

状态：`重构方案，待产品、工程、内容运营和真实租户评审`

更新时间：2026-08-01。

## 一句话

ContentCloud 以 Task 为入口、以 SOP 为企业方法论、以 Project 为业务上下文、以可审计内容事实为底层托底。

用户不需要先理解 Submission、Decision、Snapshot 或 Runtime 才能开始工作；管理员可以在后台配置企业自己的 SOP，普通用户从一个具体任务开始，系统再把任务映射到可验证的内容生产、审核、交付和学习链路。

## 这次重构解决什么

此前方案把 Project、Content Production Unit、Stage、Gate、Assignment 和交接放到了产品主心智中。模型在治理上完整，但用户必须先理解系统，才能完成一个实际工作。

参考交互样本提供的不是内容业务答案，而是三个产品组织方式：

1. 用户先处理 Task，而不是先浏览领域对象。
2. SOP 是可以复用、运行、版本化和配置的业务流程。
3. Project 是上下文容器，复杂能力进入任务详情、SOP 设计器和后台。

ContentCloud 需要吸收这三个组织方式，同时保留自己的内容治理底线：来源、证据、权利、版本、人工决定和交付事实不能被一个看板状态替代。

## 目标产品模型

```text
Workspace
  -> Environment
    -> Project
      -> SOP binding
        -> Task
          -> TaskRun
            -> StageRun
              -> LocalRun / CliRun / AutomationAttempt / ParallelRun
              -> SubmissionRevision
              -> optional GateEvaluation / Decision
              -> AcceptedSnapshot
              -> DeliveryPackage
              -> PerformanceObservation / Learning
```

这里的 Task 和 TaskRun 是用户工作流与编排对象，不取代底层正式事实。正式事实仍由 Revision、Evidence、Rights、Decision、AcceptedSnapshot、DeliveryPackage 和 Observation 提供。

## 关键决策

| 决策 | 当前规则 |
| --- | --- |
| 基础建设 | 先建环境、能力、SOP Registry、版本、权限、审计和运行契约，再扩展内容类型 |
| 业务入口 | Task 是普通用户的第一工作对象；Project 不再是每天必须打开的首页 |
| SOP | 每个环境都必须有可管理的 SOP 配置；企业可以定义自己的 Stage、角色、输出和 Gate |
| 审批 | 审批不是固定步骤；通过 SOP 的 Gate 配置为 `none`、`required_check`、`internal_review` 或 `client_decision` |
| Project | 承载品牌、产品、客户、渠道、知识、资产、SOP 绑定、Task 和交付上下文 |
| 本地执行 | Codex、Claude Code 和 Workspace 是 Task 的执行入口；CLI 配置和本地规则不是普通用户必须理解的领域对象 |
| 底层治理 | Evidence、Rights、Revision、Decision、AcceptedSnapshot 和 Delivery 继续保持不可变或可追溯边界 |
| 复杂能力 | SOP Builder、Gate Policy、CLI 配置、本地规则、权限、审计和用量进入管理后台 |
| 历史包袱 | 允许重构和删除旧主路径；迁移期间只保留一个正式事实源，不并行维护两套状态机 |

## 首批内容范围

首批只验证两类高频内容工作：

- 短视频脚本与分镜：从需求、资料、知识、策略到脚本和交付。
- 长文/公众号文章：从 Brief、知识引用、写作、检查到交付。

两者共享 Task、SOP、Stage、Gate、Revision、Evidence、Rights 和 Delivery 能力，只通过内容 Schema 和 SOP 配置表达差异。

## 平台内置 SOP

新租户默认获得四条可以直接开始使用的基础模板：资料与知识建设、短视频生产、文章协作、活动结果复盘。四条模板分别对应事实底座、首批两种内容生产和结果学习，不把每个风险等级或审批组合拆成新的平台 SOP。模板默认只阻断确定性检查，不把人工审批写死在产品里；企业可以在后台复制模板并配置自己的 Gate、角色和执行方式。

内置模板按 `template_key` 管理并拥有自己的版本和 digest。已有 Environment、Project 和 Task 永远保留具体版本绑定。旧短视频流程只有在精确匹配旧 Stage 结构时才升级为新版本，原版本继续可读且不自动换绑；V3 基线中的 Project、Source、Evidence、Rights 和旧 Run 通过连续迁移保留，Project 首次进入新工作区时再显式建立 SOP binding；无法精确识别的历史或自定义流程不会被猜测式改写。

## 用户看到的主导航

```text
工作区
  新建任务
  输入收集
  知识库
  我的任务
  所有任务
  任务上下文
  本地规则
  CLI 执行器
  工作区节点
  用量

项目
  任务
  SOP
```

知识库是一等基础设施工作面，提供对象、来源、Evidence、冲突、缺口、知识包、快照和确定性查询。审核、交付、结果和运行仍然从 Task 详情、SOP Stage 和后台进入，不要求普通用户先学习完整领域树。

## 文档目录

| 文件 | 内容 |
| --- | --- |
| `PLAN.md` | 目标、范围、里程碑、依赖、风险和执行顺序 |
| `01-architecture.md` | 对象模型、数据边界、运行时和投影架构 |
| `02-contracts-and-security.md` | API、Schema、权限、版本、Gate 和安全约束 |
| `03-product-workflows.md` | Task、SOP、内容生产、审核、交付和异常流程 |
| `04-web-console.md` | 工作区、Project、SOP 设计器、Task 详情和后台信息架构 |
| `05-pilot-metrics-and-handoff.md` | 首批试点、指标、运营节奏和交接包 |
| `06-delivery-and-acceptance.md` | 分阶段交付、迁移、测试、上线和验收门禁 |
| `07-sop-catalog-and-migration.md` | 内置 SOP 数量、模板边界和历史升级规则 |

## 不做的事情

- 不把 ContentCloud 做成通用项目管理软件。
- 不把 Agent 聊天记录作为内容正式事实。
- 不把所有工作强制塞进固定审批链。
- 不为每种内容类型复制一套 Project、Task、Review、Delivery 和指标系统。
- 不让 Web 读取或上传未明确 publish 的本地正文。
- 不因为有模板就自动开启租户内容能力。

成功标准不是增加多少页面，而是让企业能配置自己的方法论，让普通用户低成本创建任务，并让至少一条真实内容链路从任务创建跑到可交付结果。
