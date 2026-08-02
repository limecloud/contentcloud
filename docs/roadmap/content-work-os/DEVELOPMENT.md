# 开发跟踪

## 当前结论

当前阶段以基础设施托底，同时把最小业务链路接通。知识库不再只是文件列表或“已审核条目”列表，而是由类型化对象、来源与 Evidence、权利、冲突、缺口、知识包、不可变快照和确定性查询组成的事实底座；Task、Environment 和 SOP 负责把这些事实绑定到可执行的业务目标。

早期本地 MVP 中已经验证的七层知识、类型化对象、15 维诊断和 `eligible / blocked / gaps` 查询会被保留为能力；文件格式、客户端会话格式和云端事实写入继续保持边界，不把本地页面直接当作服务端数据库。

### 内置 SOP 与旧版本升级

平台内置四条可直接运行的通用 SOP。四条是当前基础模板的上限：它们覆盖事实底座、首批两种内容生产和结果学习；风险等级、审批组合、渠道差异和客户角色不再各自复制成一条平台 SOP。它们是基础建设的起点，不是客户业务事实；每个租户都可以在管理后台复制为自定义 SOP，再按自己的方法论调整 Stage、角色、输出、执行方式和 Gate。

| 模板 | 用途 | 默认 Gate |
| --- | --- | --- |
| `资料与知识建设` | 把 Brief、来源、Evidence、权利和知识候选整理成可引用的事实底座 | `required_check`，只检查确定性条件 |
| `短视频生产` | 从 Brief、知识、策略到脚本、质量检查和交付 | `required_check`，不预置人工审批 |
| `文章协作` | 从 Brief、知识引用到文章写作、检查和交付 | `required_check`，人工协作可由租户添加 |
| `活动结果复盘` | 绑定内容版本与渠道观察，形成归因、学习候选和下一动作 | 默认无 Gate |

内置模板的升级规则：

1. 新租户首次进入后台、打开 Project SOP 或创建 Task 时安装四条模板的已发布 v1；重复初始化只补齐缺失模板和缺失版本，必须幂等。
2. Environment、Project 和 Task 继续绑定具体 SOP Version 与 digest；模板升级不自动改写已有绑定。
3. 早期短视频流程只有在精确匹配旧结构（`brief -> knowledge -> draft -> delivery`）时才被标记为内置模板；原 v1 保留不变，新结构作为下一版本发布。
4. 已有 Environment 即使使用旧 v1，也不被静默切换到新版本；管理员确认后才能改绑。其他自定义 SOP 不做猜测式迁移。
5. 已发布内置版本不可直接编辑。管理员只能“复制为自定义 SOP”后修改；自定义 SOP 的已发布版本通过创建新草稿演进。

升级链路如下：

```text
启动/打开后台
  -> 读取当前租户 SOP Registry
  -> 按 template_key 补齐缺失模板
  -> 精确识别可迁移的旧短视频 v1
  -> 保留旧版本，发布新版本
  -> 保留 Environment/Project/Task 原绑定
```

### 旧版本升级边界

“之前版本”分为三类处理，不能用一个自动迁移脚本全部覆盖：

| 旧内容 | 处理方式 | 是否自动改写事实 |
| --- | --- | --- |
| 已经写入当前 SOP Registry 的旧短视频流程 | 精确匹配 `brief -> knowledge -> draft -> delivery` 后，保留旧 v1，追加当前模板版本 | 否；Environment、Project、Task 仍指向原版本 |
| V3 基线数据库中的 Project、Source、Evidence、Rights、ContextSnapshot 和旧 TaskRun | 通过连续数据库迁移保留原记录；第一次进入新工作区时再创建 Project 的 SOP binding | 否；旧 Run 只读，不伪造新的 Revision 或 Gate |
| 早期方案中的页面、Stage 名称、聊天记录或客户自定义方法 | 作为导入材料或人工配置参考，由管理员复制模板后确认 | 否；没有可验证的结构就不自动识别 |

因此，升级是“事实保留 + 新 SOP 增量接入”，不是把旧界面翻译成新界面，也不是把旧审批路径强行塞进所有任务。当前迁移只会自动识别有明确身份或明确结构的对象；同名但结构不同的自定义流程会继续保持自定义。旧数据库如果带有不在当前连续迁移集合中的历史记录，启动时会明确拒绝并要求导出/重建，不执行不可审计的在线回填。

## 里程碑

| 里程碑 | 状态 | 说明 |
| --- | --- | --- |
| 契约冻结 | 已完成 | Task-first、SOP 可配置、Gate 可选、事实层不可被投影覆盖。 |
| 基础设施与 Registry | 进行中 | Knowledge、Environment、四条内置 SOP、Gate、审计和 Source/Evidence 显式上传已接入；PostgreSQL 真实环境和试点数据仍待验收。 |
| Task 与 Run | 已接入纵向切片 | WorkTask 已支持领取、开始、暂停、恢复、取消、重试、Stage 上报和关联 TaskRun；状态机和租户隔离由服务端校验。 |
| 内容生产链 | 已接入低风险闭环 | Stage/Gate/Revision/Delivery 已形成正式事实链；视频脚本与文章 Revision 使用统一任务契约。 |
| SOP Builder 与管理后台 | 已接入真实管理页 | Environment、SOP Stage、Gate 模式、能力、审计和用量均可配置；发布检查会校验人工 Gate 阻断语义。 |
| 主工作区 | 已接入真实 API | Task-first 页面、真实动作、Gate 决定、Revision、Delivery、输入收集和知识来源/快照均已读写 BFF。 |
| 试点与交接 | 未开始 | 需要真实租户和完整审计链。 |

## 当前开发切片

### 知识基础设施

目标：让一个 Project 可以把多个类型化知识对象固定为 KnowledgePack，发布后生成不可变 KnowledgeSnapshot，并对快照执行无副作用的确定性查询。

当前输出：

- `domain.KnowledgeObject`：类型、七层、版本、状态、Evidence/权利/冲突引用、digest；
- `domain.KnowledgePack`：对象版本引用和查询策略；
- `domain.KnowledgeSnapshot`：完整对象副本和 pack digest，生成后只读；
- `domain.EvaluateKnowledgeSnapshot`：按时间、渠道、状态、Evidence、冲突和权利返回 `eligible / blocked / gaps`；
- Memory Store、PostgreSQL Store 和迁移；
- Service 层的创建、发布、快照和查询命令；
- 领域、服务和存储契约测试。

## 工程工作包状态

| 工作包 | 状态 | 下一验收 |
| --- | --- | --- |
| 契约和不变量 | 已完成 | 继续补充 Schema/命令契约测试。 |
| Environment 与 SOP Registry | 已接入基础能力 | Environment 保存、四条内置 SOP、旧短视频增量升级、默认 SOP、SOP 草稿复制/保存/发布和历史 digest 已有服务端契约。 |
| Task 与 TaskRun | 已完成首个纵向切片 | 创建后生成首个 StageRun；开始、暂停、恢复、取消、领取、重试和报告均有状态机、审计和回归测试。 |
| Stage 与 Gate 引擎 | 已接入可配置语义 | `none`、`required_check`、`internal_review`、`client_decision` 以及旧兼容别名可配置；Gate 决定后会推进或阻断 Stage。 |
| Revision、Decision 与 Delivery | 已接入 Task 正式事实 | Revision 不可覆盖、Gate 记录决定上下文，Delivery 只接受已接受 Revision 并保存 digest。 |
| 视频与文章 Schema | 已接入基础契约 | 同一 Task 支持 `contentcloud.video_script/1.0` 和 `contentcloud.article/1.0`，提交时校验标题及脚本/文章结构。 |
| Task-first Web | 已接入真实 API | 首页、任务列表、任务详情、新建任务和 Project SOP 已不再依赖静态演示数据。 |
| SOP Builder 与 Admin | 已接入真实管理页 | 后台可管理 Environment、SOP 版本、Gate 模式、能力声明、审计和用量；lint、版本 diff、影响分析、回滚和退休均有服务端契约。 |
| 本地 Workspace、CLI 与 Rules | 已有基础 | 适配器继续保持本地解析和显式 publish。 |
| 迁移和删除旧主路径 | 已完成主路径收口 | 旧云端知识入口、表和冲突决策旁路已删除；旧 Project 领域路由统一进入 Task-first 工作区，历史事实只读保留。 |
| 试点和交接 | 未开始 | 低风险与高风险任务各跑通一条链。 |

## 依赖和阻塞

- 知识基础设施 依赖现有 Source、Evidence、Rights 和 Project 作用域；不新增跨租户读取路径。
- Task 与 TaskRun 依赖 KnowledgeSnapshot 的不可变 ID 和 digest，TaskRun 只能绑定具体快照。
- PostgreSQL 集成测试依赖本地可用的 PostgreSQL；没有数据库时先由 Memory Store 和 SQL 迁移静态校验覆盖。
- 本地 Workspace 只负责解析客户端文件并显式发布候选；服务端事实统一写入 `KnowledgeObject`，旧云端知识链直接删除，不保留双写。

## 验收标准

1. 同一 Project 的两个知识对象版本可以被 Pack 显式固定。
2. Pack 发布后生成 Snapshot，后续对象版本变化不会改变旧 Snapshot。
3. 查询只从 Snapshot 返回确定性结果，候选、冲突、权利失败和缺口不会进入 `eligible`。
4. 查询包含稳定的 `query_digest`，相同 Snapshot、时间、渠道和策略得到相同结果。
5. Tenant、Project 和 Object 作用域不匹配时，服务端拒绝写入或查询。
6. 低风险内容可以使用不含人工审核 Gate 的 Snapshot；高风险对象仍由状态、Evidence、权利和冲突规则阻断。

## 测试证据

2026-08-02 已完成：

- `go test ./...`；
- `pnpm --dir web typecheck`、`pnpm test:web`（67 个测试）、`pnpm --dir web build`；
- OpenAPI YAML 解析、原型 JavaScript 语法检查和 `git diff --check`；
- Memory/PostgreSQL Store、服务层、HTTP BFF 和领域查询回归测试。
- 后台入口权限验证：租户管理员和项目经理可从工作台进入运行配置；5173 前端开发服务和 8080 API 服务均可访问，文档 API 返回有效 JSON。

浏览器截图回归和 PostgreSQL 集成环境仍待补充；本轮 `CONTENTCLOUD_TEST_DATABASE_URL` 未设置，相关测试按约定跳过，本机 5432 监听但没有可用测试角色。

## 决策记录

| 日期 | 决策 | 原因 |
| --- | --- | --- |
| 2026-08-01 | 知识事实使用独立 `KnowledgeObject`，不继续扩张旧知识条目模型 | 旧模型承担候选流程，直接加字段会混合两套状态语义。 |
| 2026-08-01 | Snapshot 保存对象完整副本 | 新对象版本不能静默改变历史 Task/Run 的输入。 |
| 2026-08-01 | 查询结果固定为 `eligible / blocked / gaps` | 业务生成只能使用可证明对象，缺口必须转为下一动作。 |
| 2026-08-01 | 查询不改变知识状态 | 查询是纯函数，状态变化必须走独立命令和审计。 |

## 后续迁移和清理

- 知识抽取直接生成 `KnowledgeObject` candidate，并绑定稳定 Evidence 引用；不再新增旧知识条目云端记录。
- 将本地 15 维诊断映射到 Project 的七层覆盖投影，不把诊断分数写成事实状态。
- Task/Run 接入 Snapshot 后，删除前端静态知识数据和旧知识主导航的重复写入。
- 旧云端知识 API、表和冲突决策旁路已删除，后续只扩展 `KnowledgeObject`、`KnowledgePack` 和 `KnowledgeSnapshot`。
