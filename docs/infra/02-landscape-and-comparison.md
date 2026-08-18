# AI Infra 参考版图：vLLM、SGLang 与 ContentCloud

状态：`研究快照 + ContentCloud 当前模块对账，不是供应商选型结论`。

更新时间：2026-08-11。

## 1. 搜索口径

本次检索使用官方项目 README、官方文档、官方 GitHub API 和 Camunda 官方平台页。检索时间为 2026-08-10（Asia/Shanghai）。GitHub Stars 只作为公开社区关注度的粗略信号，不等同于生产采用量、性能或商业价值；SGLang README 中的“400,000 GPUs”等采用声明也属于项目方自述，不能替代 ContentCloud 的试点数据。

公开活动快照：

| 项目 | GitHub Stars | Forks | 最近更新（API） | 公开定位 |
| --- | ---: | ---: | --- | --- |
| vLLM | 88,653 | 20,499 | 2026-08-10 | LLM inference and serving |
| SGLang | 31,611 | 7,776 | 2026-08-10 | LLM/multimodal serving framework |
| LiteLLM | 56,021 | 10,465 | 2026-08-10 | 100+ LLM 的统一 Gateway |
| LangGraph | 39,356 | 6,610 | 2026-08-10 | Stateful agent orchestration |
| Temporal | 22,211 | 1,802 | 2026-08-10 | Durable execution platform |
| Langfuse | 32,818 | 3,529 | 2026-08-10 | LLM/Agent observability and evals |
| LlamaIndex | 51,528 | 7,904 | 2026-08-10 | Data framework for LLM applications |

这组数据支持的判断是：当前“AI Infra”并不是一个单一品类，而是从推理运行时、模型网关、Agent 编排、持久执行、数据接入到观测评测的一组相邻层。ContentCloud 应站在这些能力之上，做内容工作特有的闭环和事实治理。但这份对比不能脱离仓库当前已经存在的 LocalRun、V3 内容包、Plugin Host 和 V8 Runtime。

## 2. vLLM 与 SGLang 对比

### 2.1 能力对照

| 维度 | vLLM | SGLang | 对 ContentCloud 的启示 |
| --- | --- | --- | --- |
| 核心问题 | 高吞吐、低成本的 LLM serving | 高性能 LLM/多模态 serving 和复杂生成程序 | 两者都是底层推理执行者，不是内容任务平台 |
| 关键优化 | PagedAttention、连续批处理、Prefix Cache、Chunked Prefill、CUDA/HIP Graph、量化、Speculative Decoding | RadixAttention、Prefix Cache、连续批处理、Prefill/Decode Disaggregation、结构化输出、量化、并行和 Speculative Decoding | ContentCloud 应通过 Provider/Model Capability 接口消费这些性能，而不是复制内核 |
| 模型范围 | Hugging Face 200+ 架构，包含文本、多模态、嵌入、重排和分类 | 文本、多模态、嵌入、奖励模型以及部分 diffusion 模型 | ContentCloud 需要统一文本、图片、视频、音频的异步产物契约 |
| API | OpenAI-compatible，另有 Anthropic Messages 和 gRPC | OpenAI-compatible API，并有前端语言和多模态能力 | 统一 API 适合做适配层，不足以表达版本、权利、审批和血缘 |
| 分布式 | Tensor、Pipeline、Data、Expert、Context Parallelism；支持多种硬件插件 | 单 GPU 到大集群；Tensor/Pipeline/Expert/Data Parallelism、PD disaggregation | 运行时仍需租户公平、预算、区域和数据策略；推理框架不会替我们完成 |
| 结构化输出 | xgrammar/guidance、tool calling 和 reasoning parser | 结构化输出、工具调用和推理相关能力 | 仅解决“返回格式”，不解决跨节点 Schema 版本和业务事实归属 |
| 主要差异 | 生态广、部署入口简单、OpenAI API 兼容和硬件覆盖突出 | 对 RadixAttention、复杂程序、多模态和最新模型 day-0 优化投入更明显 | 首期保持 Provider-neutral，按任务实测再绑定 vLLM 或 SGLang |
| 不负责的事情 | 搜索、网页采集、内容权利、人工审核、产物目录、发布渠道 | 同左 | 这些是 ContentCloud 的产品差异化空间 |

### 2.2 选择建议

不要在架构层做“vLLM versus SGLang 永久二选一”。建议建立以下能力绑定：

```text
Capability: model.text.generate
  -> Provider: cloud/openai-compatible/sglang/vllm
  -> ModelProfile: model + region + precision + data policy
  -> ExecutionProfile: timeout + quota + tool policy + cost ceiling
```

选择原则：

- 以 ContentCloud 真实内容任务的首 token、完整结果时间、结构化输出通过率、失败率和单位正式产物成本做基准。
- 同一模型、同一输入、同一提示和同一硬件条件下比较；不能把不同模型结果当作引擎优劣。
- 需要自托管、硬件可控和广泛模型兼容时优先评估 vLLM。
- 需要复杂结构化程序、多模态或最新模型快速支持时评估 SGLang。
- 两者都只能通过发布后的 `ExecutionProfile` 接入，不能让 Agent 在运行时自行切换端点。
- 结果不明时，先对账或人工处理；不要因为某个推理端点超时就无条件重复生成和重复收费。

## 3. 邻近项目各自解决什么

| 项目/类别 | 它提供的价值 | ContentCloud 应复用 | ContentCloud 不能外包给它 |
| --- | --- | --- | --- |
| LiteLLM | 统一多供应商 API、虚拟密钥、花费追踪、路由和负载均衡 | 模型/Agent Provider Gateway 的适配思路；或作为可选组件 | 任务图、内容 Schema、来源证据、资产和发布事实 |
| LangGraph | 长时、带状态 Agent，持久执行、人工介入、记忆和调试 | Agent 节点内的编排模式、开发体验 | 跨租户权威 Job、RLS、外部 Effect 对账、内容业务状态 |
| Temporal | 持久 Workflow、失败恢复和重试 | Durable execution 的成熟语义作为参考 | ContentCloud 的内容对象、权限和客户投影 |
| Camunda | 设计工具、连接器、统一编排、Tasklist、Operate、Identity、审计和 Optimize | 三层平台叙事、人工任务、治理和运营产品 | BPMN 不替代 ContentCloud 的内容事实和创作 Schema |
| LlamaIndex 等数据框架 | 文档接入、索引、检索和数据连接 | 数据处理/检索节点的实现组件 | 权利、版本固定、证据等级和客户资产复用门禁 |
| Langfuse 等观测平台 | LLM/Agent trace、成本、评测和反馈 | LLM 调用层 trace/eval 的可选集成 | 任务、产物、发布 Effect 和运营故障的权威审计 |
| 对象存储/数据库/队列 | 文件、元数据、事件和索引的基础存储 | 作为 Runtime 和业务域实现依赖 | 不把原始存储表直接暴露为客户资产 API |

### 3.1 Agent 与 SaaS 也必须是可替换执行端

Codex/Claude Code 不是 ContentCloud 的控制面。它们只是当前已有实现切片的本地宿主和 Runtime Harness，且客户侧发布层级并不相同。Pi Agent、其他本地/远程 Agent、Agent 工作流 SaaS、研究 SaaS、图片/视频/音频/排版 SaaS 均应按能力接入：

| 外部执行生态 | 可以承担 | 不能拥有 |
| --- | --- | --- |
| 本地通用 Agent（Codex、Claude Code、Pi Agent 等） | 读取授权工作区、调用 MCP/CLI、生成候选、执行 Lint 与 Handoff | WorkTask、批准事实、外部发布事实 |
| 远程/托管 Agent | 长时研究、结构化生成、异步协作 | ContentCloud 的任务状态机和客户资产权威版本 |
| Agent 工作流 SaaS | 多 Agent 分工、研究/生成/校对流水线 | 未经验证的业务正文写入和跨租户上下文 |
| 垂直创作 SaaS | 图片、视频、剪辑、配音、翻译、排版、发布 | Canonical content、权利结论和 ContentCloud 审批 |
| 确定性 Worker | 解析、转码、排版编译、打包、校验 | 需要编辑判断的质量决策 |
| 人工 | 创意判断、编辑、法务、审核、账号发布 | 系统自动化执行日志和机器回执 |

Pi Agent 在本仓库中没有已验证 Harness/Host 实现，因此只能写成候选接入方式。是否接入不取决于品牌热度，而取决于它能否满足结构化输出、能力握手、工作区隔离、会话恢复/幂等、取消、费用和审计要求。

## 4. ContentCloud 与这些项目的真正差异

### 4.1 单个推理请求 vs. 一项内容工作

```text
vLLM/SGLang:
  request -> tokens/structured response

ContentCloud:
  brief -> search -> source/evidence -> input snapshot
        -> plan -> agent/worker/provider/human nodes
        -> revisions -> approval -> artifact
        -> channel preflight -> publish effect -> receipt
        -> performance feedback -> reusable asset
```

内容任务的终态不只是“HTTP 200”或“模型返回文本”，而是一个经过权利、质量、审批和交付验证的版本化结果。

### 4.2 对话记忆 vs. 可审计内容事实

Agent 会话可以帮助完成推理，但以下对象必须脱离会话独立存在：

- 来源、证据和查询时间。
- 输入资料的固定版本和摘要。
- Brief、脚本、分镜和媒体的修订链。
- 人工选择、修改、批准和发布授权。
- 外部模型、媒体服务和渠道调用的请求、结果、费用与对账。
- 哪个正式产物被哪些后续任务复用。

### 4.3 连接器数量 vs. 数据互通质量

“有 500 个连接器”不代表可用。每个连接器至少要有：

```text
授权范围 -> 可读取对象 -> 增量游标 -> 版本/删除语义
         -> 规范化 Schema -> 错误/限流 -> 权利与数据区域
         -> 预览/导出 -> 观测指标 -> 退役策略
```

首期只做一个公开搜索源、一个指定网站采集器、一个本地资料入口和一个发布渠道，用真实闭环证明契约，再扩展连接器。

## 5. Camunda 图到 ContentCloud 图的映射

| Camunda 图中的位置 | ContentCloud 对应 | 需要保留的差异 |
| --- | --- | --- |
| Web Modeler / BPMN / Forms | Experience、SOP、Schema、Prompt、评测集和渠道规格 | 面向内容任务，不让客户直接编辑底层执行图 |
| Console / Identity | 运营控制台、租户、成员、角色、能力和 Secret 管理 | 客户、运营和执行者是不同产品面 |
| Connectors / MCP / REST | Search、Fetch、Provider、Agent、Channel Connector Registry | 每个连接器有数据、权利、删除和回执契约 |
| Optimize / Analytics | Runtime Explorer、成本、质量、发布表现和复盘 | 不能只分析流程时间，还要分析内容质量和复用 |
| Zeebe orchestration engine | ContentCloud Agentic Job Runtime | Runtime 不拥有内容正文和客户资产 |
| Operate | 任务记录、运行诊断、Effect 对账和投影重建 | 技术原因保留在运营面，客户看到可行动语言 |
| Tasklist | 客户审核、运营处理队列、发布前确认 | 人工 Gate 是执行图中的正式节点 |
| Enterprise systems | 来源、企业知识库、资产、模型、媒体服务和渠道 | 增加证据、权利、版本和产物血缘 |

## 6. 外部能力与当前模块的落点

对比结论必须能落到当前代码所有权，而不是只停留在“ContentCloud 应该有一个 Gateway/Registry”。

| 外部能力类别 | ContentCloud 当前落点 | 当前状态 | 复用/建设决策 |
| --- | --- | --- | --- |
| vLLM/SGLang 推理服务 | `internal/integration/provider/model`、Provider/Capability Binding、`model-generation` receipt | `current-server` / `external-dependency` | Adapter、OpenAI-compatible 请求和回执已实现；真实集群、模型和基准仍需外部接通 |
| LiteLLM 类模型 Gateway | `internal/integration/provider/model` 的 Provider-neutral 接口 | `partial` / `external-dependency` | 可作为上游路由，不新增 Gateway 事实模型；账单和路由由现有 ModelGeneration receipt 记录 |
| LangGraph 类 Agent 编排 | `internal/integration/agent`、Plugin Skill、MCP Gateway、V8 Runtime | `current-local` / `current-server` | 复用 Agent Harness 和 Runtime；编排 SaaS 只提供可替换执行能力 |
| Temporal 类 durable execution | `internal/runtime`、`internal/persistence/postgres`、outbox/effect | `current-server` | 现有 JobRun/NodeRun/Attempt/Effect 是唯一执行事实，不引入第二套 Workflow 状态 |
| Camunda Tasklist/Operate | Studio 审核、运营控制台、Runtime Explorer、ReviewGrant | `current-server` / `partial` | 借鉴人工任务和运营诊断面；不用 BPMN 替代 Experience/SOP/V3 Schema |
| LlamaIndex 类数据接入 | `internal/integration/provider/source`、`internal/integration/connector`、`internal/local/workspace/source.go` | `current-server` / `external-dependency` | Search/Fetch/Connector 已落到 SourceRevision/Evidence；企业数据授权和字段映射仍由外部系统提供 |
| Langfuse 类观测评测 | Runtime events、usage、PerformanceObservation、RatingDecision | `partial` | 可接 trace/eval；任务、Artifact、发布 Effect 仍由 ContentCloud 拥有 |
| Agent Plugin 市场/包管理 | `internal/catalog/environment`、`internal/integration/plugin*`、`contracts/marketplace-registry-1.0.schema.json` | `current` / `partial` | 复用签名 Registry 和 Environment lock，不重新设计 Capability SDK |

## 7. ContentCloud 当前能力的选择原则

| 决策 | 选择 |
| --- | --- |
| 任务和内容事实 | 保留 WorkTask、V3 ContentBatch/ContentItem、SubmissionRevision、ApprovedSnapshot、Artifact、DeliveryPackage |
| 本地 Agent 交互 | 保留 Plugin/Skill/MCP、LocalRun、Claim、Handoff；不强制每次交互创建云端 Automation Run |
| 长时自动化 | 保留 JobRun、NodeRun、RuntimeAttempt、ContextView、Effect 和 Outbox |
| 推理执行 | vLLM/SGLang/云 API 都是可替换 Provider，不成为客户内容模型 |
| 搜索和采集 | 先补一个受治理的真实 source.search/source.fetch 执行器，再抽象联邦搜索 |
| 发布渠道 | 将微信人工 Runbook 作为 current 交付模式，再验证一个真实自动渠道 Adapter 和回执；不新增没有消费者的兼容层 |
| 能力分发 | 复用签名 Plugin Registry、Environment Manifest 和 Host Adapter，不另起市场模型 |
| Agent/SaaS 执行 | 业务节点只绑定 Capability 和 Schema；Codex、Claude、Pi 或 SaaS 由 ExecutionProfile/Adapter 决定 |

## 8. 研究来源

以下链接是本次检索直接使用的来源，访问日期均为 2026-08-10：

- [vLLM README](https://github.com/vllm-project/vllm)：定位、PagedAttention、连续批处理、分布式并行、结构化输出、OpenAI-compatible API 和模型支持。
- [vLLM 官方文档](https://docs.vllm.ai/en/latest/)：安装、模型、使用和开发文档入口；页面显示的文档更新日期为 2026-04-09。
- [SGLang README](https://github.com/sgl-project/sglang)：RadixAttention、前缀缓存、PD disaggregation、多模态、扩散模型和硬件支持等定位。
- [SGLang 官方文档](https://docs.sglang.io/)：生产 serving、低延迟、高吞吐和 OpenAI-compatible API 入口。
- [LiteLLM README](https://github.com/BerriAI/litellm)：100+ LLM 的统一 API、Gateway、虚拟密钥、花费追踪和负载均衡。
- [LangGraph README](https://github.com/langchain-ai/langgraph)：长时、有状态 Agent、持久执行、人工介入、记忆和调试。
- [Temporal README](https://github.com/temporalio/temporal)：durable execution、Workflow、失败重试和恢复。
- [Langfuse README](https://github.com/langfuse/langfuse)：LLM/Agent trace、成本、评测和反馈入口。
- [LlamaIndex README](https://github.com/run-llama/llama_index)：数据接入、索引和检索框架定位。
- [Camunda Platform](https://camunda.com/platform/)：三层架构、统一编排、Agent/人/系统、连接器、Operate、Tasklist、Identity、审计和 Optimize。

## 9. 证据限制

- GitHub Stars、Forks 和更新时间会变化，只能作为公开热度快照。
- 项目 README 的性能和采用声明没有在 ContentCloud 的硬件、模型、数据和任务上复现，不能直接写成产品承诺。
- “AI Infra”搜索热度没有单一权威排名；本文件把它拆成可验证的能力层，而不是声称某个词在所有市场都排名第一。
- 供应商和开源项目的许可证、数据处理条款、云区域和商用限制必须在真实接入前单独评审。
