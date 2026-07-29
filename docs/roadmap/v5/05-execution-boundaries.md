# Codex、本地媒体、服务端与外部平台的执行边界

## 1. 结论

V5 使用三方执行模型，而不是让“ContentCloud”模糊地负责所有步骤：

```text
Codex + 本机 Workspace             ContentCloud 服务端              外部平台 + 用户
候选生成、媒体生产、导出            正式事实、审核、审计、归因        Seedance/抖音登录与实际操作
```

`publish` 将本地候选提交为可审核 Revision；`pull` 将服务端已批准/锁定事实带回本机。两者是唯一跨边界的数据通道。Codex 不直接写服务端正式对象；服务端不扫描、读取或执行本机 Workspace；Seedance 与抖音不由服务端代持账号或代上传。

本文中的 `Codex` 特指运行在用户机器、绑定本机 Workspace 的 Codex/Plugin Agent，不包括服务端 LLM worker。首版不设置服务端创意生成 worker。

执行位置按四条规则确定：

1. 需要未披露原始素材、频繁交互或可恢复生成的工作放在 Codex 本机。
2. 会形成多人共享正式事实、权限决定或结果归因的工作放在服务端。
3. 需要 Seedance/抖音登录态并产生外部副作用的动作由用户在外部平台确认。
4. Schema、摘要、引用、权利和 Offer 等确定性校验可以本地预检、服务端复验；两次执行使用同一契约，但只有服务端结果能支撑正式状态迁移。

## 2. 职责矩阵

| 步骤 | 唯一执行方 | 输入 | 输出 | 跨边界规则 |
| --- | --- | --- | --- | --- |
| 维护八大人群 taxonomy | 服务端治理管理员 | 官方/人工验证来源 | AudienceTaxonomySnapshot | 服务端记录来源、版本和有效期；Codex 只 pull |
| 生成人群策略候选 | Codex | pull 的 taxonomy、项目证据、商品知识 | 本地 AudienceStrategyVersion candidate | 本机文件和 LocalRunContext；不自动成为正式事实 |
| 审核人群策略 | 服务端 + 人工审核者 | publish 的 strategy Revision | Decision / ApprovedSnapshot | Web/Browser 只执行治理命令，不编辑本地候选 |
| 生成 Brief、剧本 | Codex | pull 的 approved strategy/知识/规则 | 本地 Brief、ContentItem candidate | 通过 publish 进入服务端审核 |
| 审批剧本 | 服务端 + 人工审核者 | SubmissionRevision | ApprovedSnapshot | Codex 必须 pull 后才作为正式生产输入 |
| 生成分镜和候选图 | Codex | approved 剧本、获授权素材 | `50-production/media` 本地 Artifact 与 manifest | 可调用用户已配置的生成能力；服务端不私自读取本机媒体 |
| 分镜审核与 lock | 服务端 + 人工审核者 | 显式 publish 的 review subset、摘要和元数据 | Decision、含 locked digest 的 storyboard ApprovedSnapshot | 原件披露遵循 SourceDisclosure；服务端不要求全部本机素材上云 |
| 生成 Seedance 交付包 | Codex | pull 的 storyboard ApprovedSnapshot + 摘要匹配的本地媒体 | `60-delivery` 的 package、prompts、上传清单 | 本地验证后可 publish manifest/DeliveryPackage；绝对路径不出本机 |
| 上传、生成和下载 Seedance take | 用户在 Seedance | 本地导出包 | 外部平台结果 | 首版手工执行；ContentCloud 服务端无账号、token 或上传动作 |
| 导入 take 与后期合成 | Codex / 用户的本地工具 | 下载的结果、真实素材、后期方案 | 本地 generated plate、rendered creative | 用户选择合格 take；最终成片需显式 publish |
| 在抖音发布 | 用户在抖音/千川 | 最终本地成片、当前 Offer | 平台 creative/post ID | 服务端不代发布；用户回填或导入平台 ID 建立 binding |
| 建立发布绑定与导入结果 | 服务端 | 显式 command、CSV/API 适配数据 | PublishedCreativeBinding、PerformanceObservation | 服务端校验 ID、arm、币种、窗口、去重和 ROI |
| 形成学习 | 服务端 + 人工决策者 | 可归因 observation | RatingDecision / Learning | 不能由 Codex 或服务端自动采纳为下一版策略 |

### 2.1 CLI 命令与实际执行位置

“在 Codex 终端输入命令”不代表业务动作都在 Codex 本机执行。CLI 是边界网关，命令前缀和副作用如下：

| 命令 | 发起位置 | 实际读写位置 | 权威性 |
| --- | --- | --- | --- |
| `contentcloud local ...` | Codex 本机 | 仅本机 Workspace | candidate 或 validated local delivery；不能产生正式批准 |
| `contentcloud publish ... --dry-run` | Codex 本机 | 仅本机读取和校验 | preflight，无云端写入 |
| `contentcloud publish ... --plan-id ... --review` | Codex 本机发起 | ContentCloud 服务端创建不可变 SubmissionRevision | 进入待审，不等于批准 |
| `contentcloud submission approve ...` | 用户 CLI/Web 发起 | ContentCloud 服务端写 Decision 和 ApprovedSnapshot | 需要用户会话、角色与明确理由；Codex 不自动执行 |
| `contentcloud pull approved ...` | Codex 本机发起 | 读服务端，写本机只读 cache | 把服务端正式事实带回本机 |
| Seedance 上传/生成、抖音发布 | 用户在外部平台 | 外部平台 | 必须人工确认，ContentCloud CLI 不代理 |

CLI JSON 输出必须携带执行平面：`local` 结果为 `execution_plane=codex_local`；publish preflight 同时标出 `preflight_execution_plane=codex_local` 和 `apply_execution_plane=contentcloud_server`。Skill 必须据此停在权限边界，不能因为命令由 Codex 调用就宣称已经完成服务端审核或外部平台操作。

## 3. 时序

```text
Codex                              服务端                         用户/外部平台
  |                                    |                                  |
  | pull taxonomy/knowledge ----------> |                                  |
  | <--------- approved facts ----------|                                  |
  | create strategy candidate            |                                  |
  | publish Revision ------------------> |                                  |
  |                                    | review / decision                |
  | pull ApprovedSnapshot ------------> |                                  |
  | <--------- approved snapshot --------|                                  |
  | build storyboard candidate           |                                  |
  | publish review subset ------------> |                                  |
  |                                    | approve + lock digest            |
  | pull storyboard snapshot ----------> |                                  |
  | <------ ApprovedSnapshot + digest ----|                                  |
  | compile + validate Seedance package  |                                  |
  |-----------------------------------------------> upload/copy/generate  |
  | <----------------------------------------------- download take        |
  | local QA + post-produce             |                                  |
  | publish final delivery -----------> |                                  |
  |-----------------------------------------------> publish on Douyin     |
  |                                    | <------ platform IDs/results --- |
  |                                    | binding/import/decision          |
```

箭头不代表服务端代替用户操作外部平台。只有用户明确上传、下载、发布或导入后，相关平台数据才会跨入 ContentCloud。

## 4. Codex 本机执行细则

Codex Skill 是本地编排器。它只能在已绑定、已验证的 Workspace 中：

1. 读取 LocalRunContext 和 pull 到本机的已批准对象。
2. 创建 candidate 文件、调用 lint、生成分镜任务和维护本地 manifest。
3. 调用用户配置且被允许的图片/视频/后期能力，记录 capability version/digest 与输入输出摘要。
4. 在 export 前执行无网络副作用的 package validator。
5. 通过 CLI Gateway 明确调用 publish/pull，不模拟服务端审批状态。
6. 生成给用户执行的 Seedance/抖音操作清单，不读取、保管或打印平台 token。

Codex 不得：

- 把未批准 candidate 标记为 approved/locked。
- 绕过 publish 直接写服务端 Artifact、Decision、DeliveryPackage 或结果。
- 因为本机找到了文件就推断有权上传到外部平台。
- 将本机绝对路径、对话内容或环境秘密打包进交付。

## 5. 服务端执行细则

服务端是治理层和正式事实层。它负责：

1. 租户/项目授权、Workspace 绑定、请求幂等、审计和 SourceDisclosure。
2. 接收 publish 的不可变 Revision，启动 ReviewCycle，写入 Decision 与 ApprovedSnapshot。
3. 对公开/审核所需的 Artifact 元数据或允许披露的副本进行摘要校验和保留策略管理。
4. 校验 lock digest、DeliveryPackage、PublishedCreativeBinding 和结果导入之间的血缘。
5. 在 Web/Browser 工作台展示证据、评论、阻断和下一个人工动作。
6. 以追加式规则存储 PerformanceObservation、RatingDecision 和 Learning。

服务端不得：

- 遍历本机 `50-production` 或 `60-delivery` 目录。
- 直接执行 Seedance 提示词、上传素材、下载视频或操作抖音账户。
- 从未发布聊天内容、local path 或内部推理构建创意事实。
- 自动批准分镜、自动选择 Seedance take、自动发布或自动采纳投放结论。

## 6. 外部平台与人工确认

Seedance 和抖音/千川属于外部系统。首版需人工明确执行四个不可隐含的动作：

1. 使用自己的合法账户登录平台。
2. 按 package manifest 上传已锁定且有权使用的素材。
3. 复制提示词，检查平台 UI 最终显示的编号、时长和模式后发起生成。
4. 发布前核对 Offer、字幕、价格、合规和平台 creative/post ID。

未来若要自动化上传或发布，必须单独立项，至少新增：OAuth/账户授权范围、素材披露确认、上传审计、幂等键、失败恢复、平台限流、保留/删除策略和紧急撤销。不能把它作为 V5 的隐含实现。

## 7. 断点恢复

| 中断点 | 所在平面 | 恢复方式 |
| --- | --- | --- |
| Codex 中断 | 本机 | 使用 LocalRunContext、HandoffRecord 和本地 manifest 恢复；未 publish 的候选仍是本地候选 |
| 审核中断 | 服务端 | 由 ReviewCycle、Revision digest 和 Assignment 恢复；不要求重新生成本地媒体 |
| lock 后本地文件丢失 | 本机 | 从允许披露的 Artifact 或备份恢复；摘要不匹配时禁止导出并重新 lock |
| Seedance 生成失败 | 外部平台 | 记录 segment/take 失败；本机选择重试或 Plan B，不修改锁定上游对象 |
| 后期或 Offer 变化 | 本机 + 服务端 | 重渲染本地成片，publish 新 Artifact/Delivery，建立新 binding |
| 结果导入失败 | 服务端 | 保留隔离错误报告；修复 CSV/ID 后以新请求重试，历史 Observation 不覆盖 |

## 8. 可测边界

实现必须至少证明：

- Codex 在离线状态只能编辑本地 candidate，不能伪造服务端批准。
- 服务端在没有 `publish` 时看不到本机候选和媒体。
- export Skill 在未 `pull` 到 storyboard ApprovedSnapshot，或本地媒体不匹配其 `locked_digest` 时拒绝执行。
- 服务端网络故障时，Codex 可以保存本地工作但不能标为可发布交付。
- 外部平台不可用时，服务端仍保持完整血缘，不把“未生成”记为失败成片。
- 结果导入只能引用已存在的 PublishedCreativeBinding，且不能跨 tenant/project。
