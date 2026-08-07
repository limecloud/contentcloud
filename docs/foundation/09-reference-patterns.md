# 外部参考架构与 ContentCloud 边界

状态：`参考基线，不引入外部依赖`。

更新时间：2026-08-07。

本文记录对 ContentCloud Agentic Job Runtime、客户创作台、运营控制台和客户资产入口有直接启发的公开系统。它不是竞品清单，也不要求 ContentCloud 采用其中任何一个产品。

## 1. 结论

没有发现一个系统完整复制 ContentCloud 的组合，但各层已有成熟先例：

```text
Camunda 8              产品面和执行者分层
Dify                   流水线定义与客户应用发布
Temporal / LangGraph   可恢复执行、检查点和人工中断
Adobe GenStudio        创作、模板、资产和审批治理
Runway / Frame.io      生成结果血缘、版本和审核体验
```

ContentCloud 的新颖性不在于发明这些单点能力，而在于把它们组合到内容创作领域：客户看到简单的创作任务和资产工作区，运营维护流水线，Runtime 统一调度本地 Agent、托管 Agent、确定性 Worker、外部服务商和人工，客户资料与生成结果通过不同投影继续被使用。

## 2. 参考系统对照

| 系统 | 已验证的设计 | 可借鉴部分 | 不应直接复制 |
| --- | --- | --- | --- |
| Camunda 8 | Web Modeler、Zeebe、Job Workers、Tasklist、Operate 分开 | 运营编排、持久 Runtime、执行者领取任务、人工任务和独立运维 | BPMN 和通用流程对象不能直接成为客户创作语言 |
| Dify | Workflow/Chatflow 在画布中编排，再发布为 Web App、API 或 MCP | 定义、发布、运行、结果管理和 Human Input 分离 | 通用表单和聊天输出不足以表达剧本、分镜、版本和交付 |
| Temporal | Workflow Execution、Activity、事件历史、重试和长时间运行 | 把可恢复执行放在服务端，外部调用通过活动和幂等策略治理 | 不把 Temporal 的 Workflow History 当作 ContentCloud 业务正文 |
| LangGraph | Checkpointer、Thread、Interrupt、状态恢复和 Agent Server | Agent 内部状态可以恢复，人工中断可以回到明确节点 | Thread、消息和模型状态不能替代 WorkTask、Gate 和批准事实 |
| Adobe GenStudio | Create、Assets、Experiences、Templates、Reviews/Approvals 分层 | 生成上下文、模板、品牌规则、草稿、批准结果和搜索分开 | 企业级营销套件的复杂导航不适合直接暴露给客户 |
| Runway | 生成结果进入 All Generations，上传输入进入 Private，结果拥有 lineage | 结果库、输入隔离、生成血缘和从结果继续变体 | Runway 的通用 Asset 术语不直接决定 ContentCloud 的客户命名 |
| Frame.io | Version Stack、比较、评论和审核沿版本管理 | 版本和审批是不同维度，历史版本不覆盖当前版本 | 它是审阅与媒体协作工具，不是内容任务 Runtime |

## 3. ContentCloud 的目标映射

```text
平台运营人员
    |
    v
ExperienceTemplate / Published SOP / Capability Registry
    |                         |
    |                         +--> 运营控制台：配置、发布、租户、诊断
    v
JobPlanRevision -> JobRun -> NodeRun -> RuntimeAttempt
    |
    +--> ContentCloud Worker
    +--> 本地 Codex / Claude Code
    +--> 托管 Agent
    +--> 搜索、图片、视频和其他 Provider
    +--> 人工 Gate
    |
    v
CustomerJourneyProjection            Customer Asset Surface
    |                             /                           \
    v                            v                             v
客户创作台：输入、进度、确认、交付  WorkspaceMaterialProjection  CreativeResultAssetProjection
                                 文件夹与导入资料               人物、剧本、分镜、图片、视频
```

### 3.1 与 Camunda 的关系

Camunda 的 Job Worker 证明了“能力类型”和“执行者实例”可以分离：Runtime 创建工作项，符合能力的 Worker 领取并完成，超时或失败后由 Runtime 重试或产生事件。ContentCloud 需要在此基础上补充内容输入版本、人工批准、外部费用、权利和结果资产。

### 3.2 与 Dify 的关系

Dify 证明了运营人员可以在复杂画布中设计 Workflow，再把它发布成面向终端用户的 Web App。ContentCloud 应复用这种发布关系，但用 `ExperienceTemplate` 定义更强的客户体验契约：客户步骤、输入表单、结果展示和唯一主要动作，而不是把内部节点画布直接开放给客户。

### 3.3 与持久 Runtime 的关系

Temporal 和 LangGraph 的共同启发是：模型会话、线程和进程内存都不应成为唯一事实源。ContentCloud 仍应由服务端保存 JobRun、NodeRun、Attempt、Gate、Effect、Checkpoint 和业务结果引用；Agent 只领取最小上下文并提交结构化结果。

### 3.4 与生成式内容产品的关系

Adobe、Runway 和 Frame.io 共同说明，内容产品至少需要把以下维度分开：

```text
输入参考       生成结果       内容类型       版本       审批状态       使用血缘
```

ContentCloud 在客户面把“入口统一”和“领域统一”分开：客户只看到一个“资产”入口，但“我的资产”承接明确上传/导入的工作区资料，“创作结果”承接流水线结果，搜索候选和来源证据仍留在任务参考。这样符合客户对文件工作区的直觉，又不要求客户判断底层治理对象。

## 4. 不采用的方案

### 4.1 不把 Dify 或 ComfyUI 画布直接给客户

画布适合运营配置、调试和能力组合，不适合客户完成一次具体创作。客户需要看到“现在得到什么、下一步做什么”，而不是节点、变量和边。

### 4.2 不直接把 Camunda 或 Temporal 作为客户领域模型

它们适合解决运行可靠性和流程执行，但不会替 ContentCloud 定义人物原型、剧本、分镜、审批和资产复用。首阶段继续保持模块化单体，用边界和契约借鉴这些系统；是否引入外部 Runtime 由真实吞吐、恢复和运维成本决定。

### 4.3 不把 Agent Client 当作连接后的全能主控

Codex、Claude Code 和其他客户端可以是执行者之一。它们不能获得任务全局状态、批准权、任意工具、平台密钥或无限预算。客户只需对需要本地能力的项目完成一次连接和健康检查，具体节点由 Runtime 按能力绑定执行者；当前客户连接协议只发布 Codex，其他客户端必须先通过完整 bootstrap 契约才能进入客户面。

## 5. 对 ContentCloud 文档和代码的约束

1. 客户、运营和 Runtime 必须拥有独立的路由、权限、BFF、DTO 和信息架构。
2. `ExperienceTemplate`、`Published SOP` 和 `JobPlanRevision` 必须按版本发布，任务开始后不可静默改写。
3. 能力先声明，执行者后绑定；Codex、Claude Code、Worker、Provider 和人工都实现执行契约。
4. 工作区资料与创作结果使用独立 DTO；客户 BFF 可以组合查询，但不增加万能可空字段集合。
5. `confirmed` 和 `delivered` 是可复用门槛，不能与 `persona`、`script`、`storyboard`、`image`、`video` 等结果类型混为一轴。
6. 客户资产入口、当前任务输入和正式交付必须保持边界；资产入口内部也要清楚区分“我的资产”和“创作结果”。
7. Runtime、目录投影和客户投影都必须可重建、可对账并覆盖空、失败、陈旧和权限状态。

## 6. 官方参考

- [Camunda Web Modeler](https://docs.camunda.io/docs/components/modeler/web-modeler/)
- [Camunda Job Workers](https://docs.camunda.io/docs/components/concepts/job-workers/)
- [Camunda Tasklist](https://docs.camunda.io/docs/components/tasklist/introduction-to-tasklist/)
- [Camunda Operate](https://docs.camunda.io/docs/components/operate/operate-introduction/)
- [Dify Workflow Web Apps](https://docs.dify.ai/en/cloud/use-dify/publish/webapp/workflow-webapp)
- [Dify Workflow & Chatflow](https://docs.dify.ai/en/cloud/use-dify/build/workflow-chatflow)
- [Dify Human Input](https://docs.dify.ai/en/cloud/use-dify/nodes/human-input)
- [Temporal Workflow Execution](https://docs.temporal.io/workflow-execution)
- [LangGraph Persistence](https://docs.langchain.com/oss/python/langgraph/durable-execution)
- [Adobe GenStudio Create](https://experienceleague.adobe.com/en/docs/genstudio-for-performance-marketing/user-guide/create/overview)
- [Adobe GenStudio Assets and Experiences](https://experienceleague.adobe.com/en/docs/genstudio-for-performance-marketing/user-guide/content/manage-assets)
- [Adobe GenStudio Asset Details](https://experienceleague.adobe.com/en/docs/genstudio-for-performance-marketing/user-guide/content/asset-details)
- [Runway Asset Organization](https://help.runwayml.com/hc/en-us/articles/23998498329107-How-to-organize-assets)
- [Runway Asset Lineage](https://help.runwayml.com/hc/en-us/articles/53718574533395-Viewing-and-downloading-an-asset-s-lineage)
- [Frame.io Version Stacking](https://help.frame.io/en/articles/9101068-version-stacking)
