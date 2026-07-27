# ContentCloud Codex 插件实施跟踪计划

状态：`v0.5.0 已签名（等待发布与宿主验收）`。W1 最小插件包、W2 Bootstrap、W3 本地上下文、W4 多对话交接、W5 Environment/Pack 准备及 W6 publish/审批/Automation 闭环均已落地；版本、生产可信公钥、Registry 签名与部署 Profile 已就绪，当前剩余 Git/npm 发布、生产部署和 Codex Desktop 实机验收。

更新时间：2026-07-27。

架构方案：[README.md](./README.md)。

首个目标宿主：Codex Desktop 与 Codex CLI。Codex IDE Extension 当前不作为 Plugin 验收表面，因为官方文档未将 Plugins 列为其支持能力。

## 0. 当前进展

完成度口径：本地可实施的代码与自动化验证为 `100%`；包含 Git/npm 发布、生产部署和 Codex Desktop 实机验收的首版交付约为 `95%`。`v0.5.0` 已统一全部版本事实源，Plugin Registry 已用仓库外发布 key 签名，独立 Environment key 与生产 Profile 已准备完成。W0 Desktop 门禁未通过前，不对外承诺 Codex Desktop 与 CLI 行为完全一致；新版本产物发布前，不对外宣称远程安装闭环可用。

| 项目 | 当前状态 | 已有证据 | 下一检查点 |
| --- | --- | --- | --- |
| 方案与边界 | 已完成 | `README.md` 已覆盖创作环境、精选市场、服务端交互、多对话交接和 Maker 取舍 | 实施过程中发现假设不成立时同步修订 |
| Codex 官方能力核验 | 进行中 | CLI `0.145.0` 单目录/`--add-dir` 探针证明未声明 Roots，仅启动 `tools/list` | 完成 Desktop 探针与安装后新会话实测 |
| Marketplace | 发布就绪 | repo Marketplace、签名 Registry 和可信公钥均通过 source 门禁 | 发布包含 Marketplace 的 `v0.5.0` 不可变 Git tag |
| Scene Plugin | 发布就绪 | `0.5.0` manifest、三个 Skills、MCP 和 8 场景评测已绑定同一 digest | 发布 npm `0.5.0` 后完成远程安装验收 |
| canonical Skills | 已完成 | 三个 Skills 位于插件单一事实源；metadata、Go embed 和 Skill 校验通过 | 后续新增能力继续只进入插件目录 |
| Bootstrap/Codex Adapter | 代码完成，宿主验收待完成 | Detect/Plan/Apply/Validate/Rollback、`plan_id`、ConnectSession、doctor 门禁、resume、bootstrap handoff、新对话入口及失败路径均有测试 | 发布新版本后做 Codex Desktop 真实安装、新会话和回退验收 |
| Workspace Context/Routing | 已完成 | Tool-first context/status、可选 Resource、受限 cwd Resolver、canonical routing 与 doctor 已有测试 | 在 Codex Desktop 验证完整启动链 |
| 多对话交接 | 已完成 | revision CAS、TTL RunClaim、Handoff 生命周期、digest 校验与双对话竞争测试已通过 | 在 Codex Desktop 验证真实双对话恢复流程 |
| 服务端闭环 | 本地完成，宿主验收待完成 | publish 精确确认、反馈不可变 inbox、ApprovedSnapshot verified cache 及多对话离线读取已有测试 | 在 Codex Desktop 新会话验收真实 publish/review 恢复链 |
| Environment/Automation 控制面 | 配置就绪，生产待部署 | 两类生产公钥、签名 Registry、Profile、Pack preparation、Run Bundle 原子租约前门禁及 Attempt 隔离工作区均已就绪 | 部署 Environment signer 与配置并执行公网 bootstrap 验收 |

下一检查点按以下顺序推进：

1. `v0.5.0` 已统一全部版本事实源，并完成确定性评测与 Plugin/报告 digest 绑定。
2. 两类可信公钥已登记；Registry 已使用仓库外发布私钥签署 `published` payload。
3. 生产 Environment Profile、独立 signer 和 systemd 配置已准备完成。
4. 获得 Git/npm 发布授权后创建不可变产物，并在干净隔离环境验证远程 Marketplace、Plugin 和 npm MCP 启动。
5. 获得生产部署授权后启用 Environment Control Plane，验证公网 bootstrap、环境落盘、required doctor、Manifest 重拉和 Automation policy。
6. 重复 Desktop 能力探针，补齐 Codex 表面兼容矩阵和真实新会话验收。

### 0.1 当前发布状态

截至 2026-07-27，`v0.5.0` 的本地发布事实已准备完成：

- `VERSION`、根/CLI npm 包、Go CLI、Plugin manifest、`.mcp.json`、Web 固定命令和 Marketplace ref 已统一为 `0.5.0`。
- Plugin digest 为 `sha256:025e6f31b491f8cd529d39dbbae31a464743dba653234be2012426631950b26a`，8 场景评测报告 digest 为 `sha256:8dbecc50bb45fee5904889d4a29b19b7ae49a10c7cb7242afb4251eb6913c93e`。
- Registry 的 `published` payload 已由 `contentcloud-plugin-release-2026-07` 签名，内置 Go trust store 与 Marketplace PEM trust store 均可验证。
- Environment signer `contentcloud-environment-2026-07`、内置公钥和生产 Profile 已准备；私钥始终位于仓库外受限目录。

当前外部阻塞是 `v0.5.0` Git tag、GitHub Release 和 npm `0.5.0` 尚未创建，因此 tagged 门禁和远程安装验收必须在 Git/npm 发布后完成。

### 0.2 2026-07-27 验证基线

- 两次真实只读 `bootstrap plan` 探针均返回 `bp_d63218c67c8c99d7655dd0eed52608dfaa13f849eb1a1628bcbd39852da7b9ce`，目标目录保持不存在，连接码未进入输出。
- Plugin 通过 Codex `plugin-creator` 的 `validate_plugin.py`。
- `contentcloud-workspace`、`contentcloud-knowledge-extraction`、`contentcloud-marketing-video-script` 均通过 `skill-creator` 的 `quick_validate.py`。
- `go test ./...` 通过。
- `pnpm --dir web test --run` 通过，共 3 个测试文件、19 个测试。
- `pnpm --dir web typecheck` 与 `git diff --check` 通过。
- `go test -race ./...`、`go vet ./...` 和 Web production build 通过。
- Marketplace Registry 1.0 Schema 已通过 Draft 2020-12 校验；`pnpm check:plugin` source 模式通过并生成固定 Plugin digest。
- 确定性 Scene Plugin 评测已通过 8/8 场景；Environment 控制面场景覆盖 Manifest、Execution Bundle、Resolver、Lock、Pack preparation、撤回、Automation 零租约门禁、Attempt 隔离及心跳续租；Registry 绑定的报告 digest 为 `sha256:8dbecc50bb45fee5904889d4a29b19b7ae49a10c7cb7242afb4251eb6913c93e`。
- Ed25519 canonical payload、仓库外私钥门禁、可信公钥清单和 fail-closed 验证已实现；生产 Plugin 与 Environment 公钥均已登记，私钥保持仓库外 `0600`。
- Registry lifecycle/revocation 已纳入发布签名；Node/Go 共享固定 canonical SHA-256 向量，Go Resolver 只接受 cryptographically verified Registry，不能伪造 `status: verified` 绕过撤回。
- W5 环境契约、项目绑定 Manifest、ControlPlane、Workspace verified state、Manifest 重拉接口及确定性 LocalExecutionPlan 已通过 race 测试；bootstrap 已 fail closed 接入，生产 Environment trust key 与 Profile 已准备完成。
- CreativeExecutionBundle 1.0 Schema、确定性 `ceb_` ID、Ed25519 签发/验签、subject/capability/Pack 绑定和本地 Resolver 已实现；篡改、过期、撤销、Registry 撤回及 digest 漂移均 fail closed。
- Automation Poll 在 Store 租约事务前验证 Run Bundle、ContextSnapshot、设备 Environment Claim 和 capability/Pack digest；三类环境失败路径均保持 Run queued 且不创建 RunAttempt，匹配后 Lease 并行返回 TaskContract 与 Bundle。
- `check:plugin` source 模式已在 verified 签名下零 warning 通过；`check:plugin --tagged` 仅等待尚未创建的 `v0.5.0` Git ref。
- W6 合入后重新执行 `go test -race ./...`、`go vet ./...`、Web 19 项测试/typecheck/production build 和 `git diff --check`，全部通过；新增用例覆盖 publish `plan_id` 稳定/失效、CLI/MCP 未确认零云端写入、确认后单次 SubmissionRevision 写入、反馈多版本不可变保存和新对话离线读取。
- W6-03 增加 ApprovedSnapshot `0400` cache + digest sidecar、纯本地 CLI/MCP list/show、显式 MCP pull 和 verified conversation context；同一 Submission 的两个 revision、多对话无凭据读取、覆盖/篡改/旧缓存拒绝均有测试。
- 最新 Plugin source 校验通过，固定 digest 为 `sha256:025e6f31b491f8cd529d39dbbae31a464743dba653234be2012426631950b26a`；Plugin 官方校验器与三个 Skill 官方校验器均通过临时隔离 `PYTHONPATH` 复跑，未修改全局 Python 环境。

本文件是 Codex 插件实施的唯一进度台账。状态更新遵循以下规则：

- `进行中` 表示已有实际文件或测试工作，但尚未满足全部验收标准。
- `已完成` 必须附可复查的代码、测试、日志或兼容矩阵证据，不能仅凭代码已写入。
- 发现宿主能力不成立时，将任务标为 `阻塞` 或 `不采用`，同时记录替代方案，不保留无效兼容层。
- 每次阶段验收后更新顶部“当前进展”、对应工作项状态、更新时间和末尾变更记录。

## 1. 目标与完成定义

本计划只跟踪 Codex 首版闭环，不提前实现 Claude Code、OpenClaw 或 WorkBuddy Adapter。

完成必须同时满足：

1. 用户在 ContentCloud Web 复制一次连接 Prompt。
2. 插件未安装时，bootstrap 对话通过固定版本 CLI 展示计划，经确认安装 ContentCloud Marketplace 和 Scene Plugin。
3. CLI 完成项目绑定、环境锁、受管 `AGENTS.md`、offline doctor 和 Workspace 注册。
4. 安装后进入最终 Workspace Root 的新 Codex 项目对话；不依赖旧会话热加载。
5. 新对话能从本地状态回答当前进度、可继续任务和 ready handoff，不读取旧 transcript，也不自动访问服务端。
6. 对话 A 可以创建 checkpoint/Handoff，对话 B 能校验 digest、原子 claim 并继续。
7. ScriptPackage 可以 publish、进入人工审核，并通过新对话拉取反馈继续修订。
8. 插件、CLI、MCP、Skills、Schema、Environment 和 Pack 均有确定版本、digest 与诊断结果。

## 2. 当前方案判断

以下判断是当前实施基线。带“待实测”的能力仍必须通过 W0 门禁；实测结论不成立时应更新本表和对应实现，不以方案假设覆盖宿主事实。

| ID | 决定 | 状态 | 依据 |
| --- | --- | --- | --- |
| D-01 | 产品对象是 Creative Environment，Codex Plugin 是交付机制 | 实施基线 | 业务需要完整创作链与生命周期，不是插件浏览器 |
| D-02 | ContentCloud 维护少量精选 Scene/Skill/Provider Pack | 实施基线 | 用户不做底层插件选型，Resolver 按场景解析 |
| D-03 | 首版一个必装 Scene Plugin，真实任务证明需要后再增加 Pack | 实施基线 | KISS、YAGNI，避免细碎依赖 |
| D-04 | 首次安装采用 bootstrap 对话 -> 新项目对话 | 实施基线 | Codex 官方要求安装 Plugin 后在新 chat/session 使用 bundled Skills/MCP |
| D-05 | 多对话共享 Workspace 文件，不共享模型上下文 | 实施基线 | Codex 对话相互独立，连续性落到版本化状态 |
| D-06 | RunClaim + revision CAS + HandoffRecord 是跨对话写入与交接边界 | 实施基线 | 文件可见性不能解决并发覆盖和版本校验 |
| D-07 | CLI/MCP/Skill 分别负责低频执行、高频 typed tools、意图路由 | 实施基线 | 减少工具面，保持确定性边界 |
| D-08 | Codex Workspace 状态采用 typed Tool-first、同 Schema Resource 可选兼容 | CLI 已实测 | CLI 启动只发现 Tools；官方 Manual 未将 Resources 列为可依赖能力 |
| D-09 | canonical capability routing 生成 MCP instructions 与受管 AGENTS 块 | 实施基线 | 避免路由规则多份手写漂移 |
| D-10 | Codex CLI 使用显式 `directory` -> 受限 `cwd` 定位；不依赖 Roots | CLI 已实测 | `0.145.0` 单目录和 `--add-dir` 均未声明 Roots；Desktop 仍待实测 |
| D-11 | 普通本地创作默认离线，服务端只参与明确节点 | 实施基线 | 保持云端 zero-exec 和本地草稿事实源 |
| D-12 | Automation 不复用可见 Codex 对话或 Handoff | 实施基线 | 无人值守任务需要独立租约、凭据和隔离工作区 |

## 3. `@taptap/maker` 参考结论

分析基线：

- npm：`@taptap/maker@0.0.26`，发布于 2026-07-24，MIT。
- npm integrity 已核验：`sha512-mFFw3vTNYaqbcGcmjUL+nroVPx8IbWi5eqloSdHXJjmy+0BQNYQR35dek0BG9pR64LhrUHxzNVBgDA6W8DKQ0g==`。
- 源码：`taptap/instant-games-open-mcp`。
- 分析 commit：`482c5db5bd4428981731f2bdfc34618bf34b83ca`。
- 方法：只解包和静态阅读，没有执行第三方 CLI、登录或修改用户配置。

### 3.1 纳入计划

| 项目 | Maker 的有效做法 | ContentCloud 落点 |
| --- | --- | --- |
| Workspace 定位 | 用户级 MCP + `roots/list`，显式目录优先，多项目拒绝猜测 | CLI 不采纳 Roots；只保留显式目录和受限 cwd，其他宿主声明 Roots 后再接入 |
| 职责划分 | 安装/登录/诊断在 CLI，稳定业务操作在 MCP，路由在 Skill | W2 Bootstrap；W3 MCP；W4 Scene Skill |
| 能力路由 | 一份 routing 同时进入 MCP instructions 和受管 AGENTS 块 | W3 Routing Generator 与 doctor |
| 状态入口 | `maker://status` Resource + 轻量 status Tool | Codex 改为 Tool-first；Resource 只作为共用 handler 的可选兼容层 |
| 会话加载 | 更新后明确 reconnect/restart | W2 新项目对话恢复 |
| Harness 诊断 | 按目标 Detect、备份、写入、验证、恢复和报告 | W2 Codex Adapter；后续复用于其他 Harness |

### 3.2 明确不纳入

| Maker 做法 | 不采用原因 | ContentCloud 保持 |
| --- | --- | --- |
| 凭据写入用户目录 JSON | 缺少 OS secret store 边界，写入权限未显式收紧 | macOS Keychain；无安全存储时拒绝明文 fallback |
| 下载 ZIP 后不按元数据校验，执行包内 shell/PowerShell 脚本 | 供应链和远程代码执行风险过高 | 固定版本、checksum、digest/签名、内容 manifest 和脚本审核 |
| 通过正则修改 Codex TOML | 结构化配置可能被误改 | TOML parser + namespace 所有权 + backup/validate/rollback |
| MCP 启动解析未锁定的 npm 最新包 | 环境不可复现 | Environment Lock 固定 Plugin/CLI/MCP/Pack digest |
| build 合并 commit、push 和远程构建 | 副作用过大且难以审计 | preflight、显式确认、幂等调用、不可变 Submission |
| 只依赖文件和 AGENTS 做多对话连续性 | 无写锁、revision 和原子接管 | RunClaim + HandoffRecord |
| 没有精选市场、Execution Bundle、审批与 Automation 协议 | 不满足 ContentCloud 业务控制面 | 保留现有治理模型 |

若后续实现证明 Maker 某个局部做法并不优于现有代码，该项直接关闭，不为“参考过 Maker”保留兼容层。

## 4. 工作流与状态

状态值：`待确认`、`待实施`、`进行中`、`阻塞`、`已完成`、`后续`、`不采用`。

### W0 文档与能力证据

目标：先锁定 Codex 当前真实能力，避免围绕不存在的热加载或跨表面能力开发。

| ID | 任务 | 状态 | 验收证据 |
| --- | --- | --- | --- |
| W0-01 | 核对 Codex Plugin、Marketplace、Skills、MCP、AGENTS 和新会话加载边界 | 已完成 | README 中有官方来源和约束 |
| W0-02 | 核对本机 `codex plugin`、`plugin marketplace`、`codex app <path>` 命令面 | 已完成 | 已在 `codex-cli 0.145.0` 读取 help |
| W0-03 | 编写最小测试 MCP，记录 Codex CLI `initialize` client capabilities 和 `roots/list` | 已完成 | `evidence/codex-cli-0.145.0-mcp-capabilities.md`；单目录与 `--add-dir` 均无 Roots |
| W0-04 | 在 Codex Desktop 重复 Roots 测试 | 待实施 | Desktop 版本、结果与 CLI 差异 |
| W0-05 | 验证 Plugin 安装后当前会话不可用、新会话可用 | 待实施 | 可重复的安装测试和 session 边界记录 |
| W0-06 | 验证 Deep Link 与 `codex app <path>` 的路径、Prompt 和秘密处理 | 待实施 | macOS 实测；URL 中无 connect key |
| W0-07 | 形成 Codex 表面兼容矩阵 | 待实施 | Desktop/CLI/IDE 的 Plugin、MCP Roots、Resources、Deep Link 结论 |

门禁：W0-03 到 W0-07 未完成前，不删除 `target_dir` fallback，也不承诺自动打开/恢复在所有 Codex 表面可用。

### W1 Marketplace 与 Plugin 包装

目标：建立最小、可签名、可测试的 ContentCloud Codex 原生交付单元。

| ID | 任务 | 状态 | 验收标准 |
| --- | --- | --- | --- |
| W1-01 | 定义 Marketplace Registry 最小 Schema | 已完成 | `marketplace-registry-1.0.schema.json` 与 `registry.json` 包含 ID、类型、版本、来源、license、digest、签名状态、权限、数据流、兼容 Profile、输出 Schema、评测、生命周期和撤回状态 |
| W1-02 | 创建 Codex repo Marketplace | 已完成 | `marketplace.json` 已通过隔离 `CODEX_HOME` 的真实 `marketplace add/list` 验证 |
| W1-03 | 创建 `contentcloud-video-production` Scene Plugin | 已完成 | manifest、MCP 配置和三个 Skills 通过官方校验器及隔离 `plugin add/list --json` 验证 |
| W1-04 | 迁移现有创作 Skills 到 canonical 插件目录 | 已完成 | Go embed 从插件单一事实源读取，Skill metadata 与相关测试通过 |
| W1-05 | 建立插件与 Marketplace 发布校验 | 进行中 | source 门禁、8 场景评测、Ed25519 离线签名协议和可信公钥验证已接入 Make/CI；Registry 已进入 `evaluated`，tagged 模式继续阻止未完成生产签名或缺少 Plugin 的发布 |
| W1-06 | 定义 Pack 撤回和已安装环境处理 | 已完成 | Registry Schema 要求撤回原因/风险级别；新安装和新 Run fail closed；历史 Run 只读可审计；`high` 风险有独立阻断码和测试 |

门禁：首版只发布一个 Scene Plugin。Skill/Provider Pack 在真实内容任务证明独立价值、权限或发布周期后才新增。

W1-05 子项按以下状态继续跟踪：

| 子项 | 状态 | 完成定义 |
| --- | --- | --- |
| 确定性评测报告 | 已完成 | 8 个场景均执行指定 Go 测试，声明的 evidence 测试名实际出现在输出中 |
| Registry 评测绑定 | 已完成 | source 校验重新计算报告 SHA-256，并核对 Plugin ID、版本、digest、场景状态和 evidence |
| Ed25519 签名协议 | 已完成 | 签名固定 canonical payload；工具只从仓库外受限文件读取私钥并只输出签名结果；Registry 不保存私钥 |
| 可信公钥验证 | 已完成 | `key_id` 解析到受控 trust store；签名、payload、key、lifecycle 或撤销状态不匹配时 fail closed；Node/Go 固定向量和临时密钥测试通过 |
| 生产密钥登记与签名 | 已完成 | 两类生产公钥已登记；Registry `published` payload 已用仓库外发布私钥签名并通过 Node/Go 双重验证 |
| tagged 发布门禁 | 阻塞 | verified 签名已满足；创建包含 Plugin 的 `v0.5.0` Git ref 后执行最终 tagged 门禁 |

签名 payload 固定包含 Plugin ID/类型/版本、来源 ref、license、Plugin digest、兼容 Profile、权限、数据流、输出 Schema、评测绑定、生命周期和撤回状态。实际签名命令只输出可审核的 signature block，不自动修改 Registry：

```text
node scripts/sign-plugin-release.mjs --private-key <仓库外私钥路径> --key-id <trusted-key-id>
```

版本、Plugin 内容或评测报告发生变化后，旧签名必然失效，必须重新评测和签名。私钥路径位于仓库内、文件权限允许 group/other 访问、公私钥不匹配或 key 已撤销时，签名工具直接拒绝执行。

### W2 Bootstrap 与 Codex Adapter

目标：从 Web 一次 Prompt 稳定进入已加载插件的项目会话。

| ID | 任务 | 状态 | 验收标准 |
| --- | --- | --- | --- |
| W2-01 | 增加固定版本 bootstrap 命令 | 已完成 | `bootstrap plan/apply/resume` 已实现；`plan` 只读，`apply` 要求 `--accept` 和匹配的确定性 SHA-256 `plan_id` |
| W2-02 | 检测 Marketplace/Plugin 实际状态 | 已完成 | Adapter 使用 `marketplace list --json`、`plugin list --json`，结构化分类 absent/current/outdated/broken |
| W2-03 | 实现 Codex Adapter 安装事务 | 已完成 | Detect/Plan/Apply/Validate/局部 Rollback 已实现；身份错配回滚实际新增对象、Deep Link 回退、双失败报告和固定恢复 Prompt 均有测试 |
| W2-04 | 接入现有 ConnectSession | 已完成 | bootstrap 复用 `connectDevice`；Plugin 失败不消费 key，连接失败回滚本次新增 Plugin/Marketplace |
| W2-05 | 写入 Workspace、Environment Lock 和受管 AGENTS 块 | 已完成 | bootstrap apply/resume 验证签名 Manifest 与 Registry，写入 `0400/0600` verified state、精确 Plugin Lock 和 canonical routing；required doctor 失败时拒绝注册；生产 trust key 属于发布配置门禁 |
| W2-06 | 完成 offline doctor 后注册 Workspace | 已完成 | apply/resume 均先 doctor，`requireHealthyWorkspace` 未通过时禁止 `workspace.register`；旧 init 路径同样设门禁 |
| W2-07 | 生成 bootstrap handoff 并打开新项目会话 | 进行中 | `.contentcloud/bootstrap-handoff.json`、Plugin mention、Deep Link、恢复 Prompt 已实现；待 Codex Desktop 真实打开与回退验收 |
| W2-08 | 安装失败恢复 | 已完成 | 只回滚本次新增项；身份错配回滚实际返回对象；取消后用独立 context 回滚；服务端已返回绑定时保留 Plugin；后续失败支持 `bootstrap resume` |

安全门禁：Agent 不能修改服务端给出的 bootstrap 包名、版本、Marketplace 来源或 digest；所有实际写入和安装必须先展示计划并获得确认。

### W3 MCP、项目指导与本地状态

目标：让多个新对话可靠识别同一 Workspace，并用最小工具面恢复业务状态。

| ID | 任务 | 状态 | 验收标准 |
| --- | --- | --- | --- |
| W3-01 | 实现 Workspace Context Resolver | 已完成 | Codex 使用显式 `directory -> 受限 cwd`；Workspace 边界与 symlink 校验 fail closed |
| W3-02 | 增加 status/context typed Tools | 已完成 | `contentcloud_workspace_conversation_context` 与 `contentcloud_workspace_status` 只读、离线，已有 MCP 测试 |
| W3-03 | 增加同 Schema 可选 Resources | 已完成 | `contentcloud://workspace/*` 与 Tool 共用 handler/Schema，不作为 Codex 门禁 |
| W3-04 | 收敛 MCP tools | 已完成 | 安装、登录和配置修复保留在 CLI；MCP 暴露稳定的 Workspace 与业务操作 |
| W3-05 | 建立 canonical capability routing | 已完成 | `internal/capabilityrouting` 同时生成 MCP instructions 与 AGENTS 受管块 |
| W3-06 | 实现 AGENTS 路由块 inspect/update | 已完成 | routing version + SHA-256；missing/outdated/current；仅替换 ContentCloud block |
| W3-07 | 扩展 environment doctor | 已完成 | doctor 按 `codex-plugin` target 检查实际交付模式并检测 routing 漂移 |

约束：`cwd` fallback 只有在能唯一向上识别 `.contentcloud/project.yaml` 且路径位于允许 Workspace 边界时成立。

### W4 多对话 Run 与 Handoff

目标：对话是交互入口，文件夹中的结构化业务状态才是连续性事实源。

| ID | 任务 | 状态 | 验收标准 |
| --- | --- | --- | --- |
| W4-01 | 将活动 Run 从全局 current 指针迁移到 `work/runs/<run-id>` | 已完成 | conversation context 扫描全部 `work/runs/*.json`，多个 Run 可并存，不依赖 current 指针恢复 |
| W4-02 | 实现 RunClaim | 已完成 | 单写者、TTL、续期、释放、显式过期接管和冲突报告已有连续并发测试 |
| W4-03 | 实现 context revision CAS | 已完成 | claimed 写操作要求 token + expected revision，旧 revision 写入失败 |
| W4-04 | 实现 HandoffRecord Schema 与生命周期 | 已完成 | ready/claimed/completed/superseded 已实现；引用文件 SHA-256，不保存 transcript |
| W4-05 | 原子实现 handoff accept + Run claim | 已完成 | 两个并发接管者只有一个成功；digest 变化拒绝且不遗留 claim |
| W4-06 | 实现 `WorkspaceConversationContext` | 已完成 | 离线返回活动 Run、claim 摘要和 ready handoff，Tool/CLI 均可读取 |
| W4-07 | 场景 Skill 接入 bootstrap/恢复/接管状态机 | 已完成 | Workspace Skill 要求先读 context、写前 claim、交接时 checkpoint + Handoff + release |

隐私门禁：Handoff 不保存完整 transcript、隐藏推理、token、安装 URL 或未版本化的大段正文。

### W5 服务端控制面与精选 Pack

目标：服务端控制允许的创作能力，但不直接操控用户本机 Codex。

| ID | 任务 | 状态 | 验收标准 |
| --- | --- | --- | --- |
| W5-01 | 实现 CreativeEnvironmentManifest 签发与验证 | 已完成 | project/profile/version/digest/expiry/Ed25519、`device.connect`、Workspace Credential 重拉、bootstrap 验签落盘和服务启动配置均已实现；生产 signer/trust key 属于部署门禁 |
| W5-02 | 实现 Environment Resolver | 已完成 | Profile allowlist、Ed25519 verified Registry、签名 Manifest 与本地 Lock 交集、撤回保护、bootstrap Registry 拉取和 verified cache 均已实现 |
| W5-03 | 实现 LocalExecutionPlan | 已完成 | 确定性 `lep_` plan、capability 越权拒绝、ready/environment_prepare、缺失原因，以及 CLI `workspace execution-plan` 和 MCP `environment_execution_plan` 已实现 |
| W5-04 | 实现 CreativeExecutionBundle | 已完成 | 独立 1.0 Schema；确定性 Bundle ID/digest；Ed25519 签发验签；绑定项目、Profile/Environment、subject、capability 和任务级 Pack；Manifest/Registry/Lock/设备 digest 验证及篡改测试通过 |
| W5-05 | 实现任务缺失 Pack 准备流程 | 已完成 | 确定性 `epp_` plan 展示权限、数据流、费用和会话影响；CLI/MCP 精确确认后安装、Lock CAS、doctor、局部回滚，并返回新会话 Deep Link/Prompt |
| W5-06 | 市场审核与场景评测 | 待实施 | 首批 Pack 不超过 2-3 个，每个有输出 Schema、权限和评测基线 |

### W6 Publish、审批与 Automation

目标：复用已有业务治理，不让 Plugin 绕过 Submission 与人工决定。

| ID | 任务 | 状态 | 验收标准 |
| --- | --- | --- | --- |
| W6-01 | Plugin 流程接入 publish preflight/apply | 已完成 | `plan_id` 绑定文件、披露、message、幂等键和环境；MCP/CLI 在精确确认前零云端写入，apply 后记录 SubmissionRevision |
| W6-02 | 审核反馈进入本地 inbox | 已完成 | 显式 pull 后按内容 hash 保存 `0400` Bundle；同一 revision 的新增评论不覆盖旧版本；新对话可离线读取并看到 inbox count |
| W6-03 | ApprovedSnapshot 保持不可变共享 | 已完成 | pull 写入只读 snapshot + digest；inbox/show 纯本地验证；同一 Submission 的不同 revision 并存，双对话读取同一 digest，覆盖/篡改 fail closed |
| W6-04 | Automation 领取前验证 capability/Pack digest | 已完成 | Store 原子候选门禁、Run Bundle 持久化、HTTP/CLI Environment Claim 和零 Attempt 测试已完成；生产 Environment key/Profile/capability policy 属于部署门禁 |
| W6-05 | Automation 使用隔离工作区与租约 | 已完成 | 每个 Attempt 使用内容寻址私有目录和完整身份 lease；Contract/Schema/Skill/Bundle 冻结只读，心跳原子续租，run token 不落盘、不进 Prompt/Agent env；冲突/失败调用 `run.finish`，结束后只清理本 Attempt 受管目录 |

固定的 Plugin 业务流如下，任一输入变化都必须回到 preflight：

```text
本地 lint
  -> publish_preflight（只读，不访问私有 HTTP）
  -> 展示 plan_id / environment_digest / 披露 / review-visible 数据 / 云端副作用
  -> 用户明确确认同一个 plan_id
  -> publish_apply（只创建对应的不可变 SubmissionRevision）
  -> 用户要求检查审核状态
  -> review_feedback_pull（显式云端读，本地不可变落盘）
  -> 当前或新对话 review_feedback_inbox（纯本地读）
  -> 创建新候选版本并重新 preflight
  -> 人工批准后，用户明确要求刷新
  -> approved_snapshot_pull（显式云端读，verified cache）
  -> 当前或新对话 approved_snapshot_inbox/show（纯本地读）
```

## 5. 剩余实施顺序

W3/W4 基础设施已经先于 W2 完成，后续不再按原始编号机械推进。以真实依赖和当前发布状态为准：

```text
v0.5.0 版本、评测、可信公钥与 Registry 签名已完成
  -> Plugin/Skill/Go/Web 全量验证
  -> 获得授权后创建 Git tag、GitHub Release 与 npm 产物
  -> tagged 门禁与干净环境远程安装验收
  -> 获得授权后部署生产 Environment Control Plane
  -> Codex Desktop 实机验收
  -> W1-05 tagged 门禁
  -> 干净环境远程安装与 Codex Desktop 新会话验收
  -> 首版交付完成
  -> 真实任务证明需要后再启动 W5-06 可选 Pack
  -> 第二个 Harness Adapter
```

W5-06 的可选 Pack 与完整 Pack Console 不属于首版发布门禁；只在首个 Scene Plugin 远程闭环和真实创作任务证明独立价值后启动。

## 6. 测试矩阵

### 6.1 安装与会话

- 全新 Codex 用户，没有 ContentCloud Marketplace。
- Marketplace 已存在，Scene Plugin 未安装。
- Plugin 已安装但版本过旧。
- Plugin 已安装且在当前会话启动前加载。
- `plan_id` 在状态不变时稳定，Marketplace/Plugin/目录状态变化后失效。
- `apply` 缺少或携带错误 `plan_id` 时，不安装、不连接、不写 Workspace。
- 安装成功但新会话打开失败。
- Deep Link 打开失败后回退 `codex app <path>`；两者都失败时返回路径与恢复 Prompt。
- 安装中断、连接码过期、用户拒绝确认。
- 用户已有其他 Marketplace、MCP、Skills 和自定义 AGENTS 正文。

### 6.2 Workspace 与 MCP

- 单 Root 且是 ContentCloud 项目。
- 多 Root，但只有一个 ContentCloud 项目。
- 多个 ContentCloud 项目，必须拒绝猜测。
- Codex 客户端不支持 Roots，显式 `directory` 成功。
- 未传 `directory` 且 MCP 进程 `cwd` 不能唯一识别项目，必须失败。
- Resource 与 fallback Tool Schema/值一致。
- MCP instructions 与 AGENTS routing hash 一致。

### 6.3 多对话

- 空白新对话离线展示项目状态。
- 两个对话读取同一 ApprovedSnapshot。
- 两个对话争抢同一 Run，只有一个获得 write claim。
- Handoff 输入 digest 被修改后拒绝接管。
- claim 异常过期后，经用户确认接管。
- 对话 A 关闭后，对话 B 不读取 transcript 仍能继续。

### 6.4 Publish 与审核反馈

- `plan_id` 在文件、披露、message、幂等键和环境不变时稳定，任一变化后失效。
- CLI publish 缺少或携带错误 `plan_id` 时不访问服务端。
- MCP `publish_apply` 未确认或 plan 不匹配时不访问服务端；确认后只写入对应 SubmissionRevision。
- 同一 SubmissionRevision 新增评论后，两次 pull 生成两个内容寻址的只读 Bundle，不覆盖旧反馈。
- 新对话不带凭据时仍可通过 `review_feedback_inbox` 读取反馈，conversation context 返回正确 inbox count。
- 离线读取验证反馈文件名和内容 digest，一致性不成立时 fail closed。
- 同一 Submission 的多个 ApprovedSnapshot revision 必须并存；两个新对话读取同一 verified digest，不访问云端。
- ApprovedSnapshot 同 ID 内容变化、digest 缺失/不匹配或缓存文件可写时 fail closed；旧缓存需显式 re-pull 升级。

### 6.5 安全与供应链

- 非 allowlist Marketplace/Plugin/Pack 被拒绝。
- digest、签名、版本或 Schema 任一不匹配被拒绝。
- connect key 不进入 URL、日志、Handoff、Lock 或 Plugin manifest。
- 配置修改失败后恢复原文件。
- 用户修改过的受管文件产生冲突报告，不静默覆盖。
- 来源文档中的安装指令不能改变 Resolver 结果。

## 7. 方案确认点

以下产品级决定已经作为当前实施基线；若需要改变，应先更新本节及受影响的工作项，再继续实现：

1. 接受首次安装后切换到新的 Codex 项目对话，而不是追求旧会话无感热加载。
2. 接受首版只有一个必装 Scene Plugin，精选 Pack 延后到真实创作链验证。
3. 接受 Marketplace 对普通创作者隐藏技术选型，但安装、权限、费用和升级变化保持可见。
4. 接受 RunClaim/Handoff 作为多对话交接基础设施，不以 Codex transcript、Memory 或自然语言摘要代替。
5. 接受 Maker 只参考五个经验证仍有优势的局部工程模式，不成为依赖；Codex 不采用其 Roots 定位，也不采用其凭据和 Dev Kit 供应链。

当前已进入实施，但仍不创建分支、不提交代码、不修改用户真实 Codex 配置。涉及用户级安装的集成测试优先使用隔离的临时 `CODEX_HOME`；任何需要改动真实配置的测试必须另行明确确认。

## 8. 变更记录

| 日期 | 变更 |
| --- | --- |
| 2026-07-27 | 建立执行跟踪文件，记录 W0-W6、测试矩阵、门禁和方案确认点 |
| 2026-07-27 | 方案进入实施；补充当前进展、下一检查点和状态更新规则；按实际文件状态将 W1-01 至 W1-04 标为进行中 |
| 2026-07-27 | Marketplace/Plugin/Skills 通过官方校验器与隔离 Codex CLI 安装；新增 `codex-plugin` Workspace target |
| 2026-07-27 | Codex CLI MCP 探针证明 `0.145.0` 未声明 Roots；Codex 路线调整为 Tool-first 与显式目录/受限 cwd Resolver |
| 2026-07-27 | 完成 Tool-first conversation context、可选 Resource、canonical routing 与按 target doctor；相关 Go 测试通过 |
| 2026-07-27 | 完成 revision CAS、RunClaim 与 Handoff 核心生命周期；双对话竞争、过期接管、digest 漂移和 MCP 生命周期测试通过 |
| 2026-07-27 | 校准计划实际状态；下一实施主线切换为 W2 Bootstrap/Codex Adapter，并保留 W0 Desktop 能力门禁 |
| 2026-07-27 | W2 主链已实现：增加结构化 Codex Adapter、bootstrap plan/apply/resume、确定性 `plan_id`、doctor 注册门禁、局部回滚、无秘密 bootstrap handoff 与新对话入口 |
| 2026-07-27 | 记录发布阻塞：既有 `v0.4.0` 不含 Plugin/Marketplace，npm 仅有 `0.2.0`；必须使用新不可变版本完成远程验收 |
| 2026-07-27 | 完成 W2 协议与失败路径：apply 强制匹配 `plan_id`，补齐 Web/Bootstrap 文案、身份错配回滚、Deep Link 回退、固定恢复 Prompt 与 origin URL 测试 |
| 2026-07-27 | Plugin/三个 Skills 官方校验、真实只读 plan 稳定性探针、全量 Go/Web 测试、typecheck 与 diff check 全部通过；W2 剩余发布和 Desktop 实机门禁 |
| 2026-07-27 | 修正连接失败恢复：请求取消不再阻断 Plugin rollback；服务端已消费连接码但本地凭据保存失败时不再误删 Plugin |
| 2026-07-27 | 新增 Marketplace Registry 1.0、确定性 Plugin digest 与 source/tagged 双模式发布检查，接入 Make/CI；tagged 模式保持对签名、评测和无效旧 tag 的硬阻塞 |
| 2026-07-27 | 完成 W6-01/W6-02：publish 使用精确 preflight/apply 确认协议；审核反馈按内容 hash 不可变落盘，新对话可离线读取；补齐 MCP/CLI 无副作用门禁测试与 Skill 固定流程 |
| 2026-07-27 | 完成 W6-03：ApprovedSnapshot 使用只读 snapshot + digest cache，新增显式 pull 与纯本地 inbox/show，双对话共享不同 revision 且篡改 fail closed；路由升级至 1.1.0 |
| 2026-07-27 | 完成 W1-05 确定性评测阶段：7/7 场景通过，Registry 绑定报告 digest 并进入 `evaluated`；tagged 门禁只剩生产签名和无效旧 tag 阻塞 |
| 2026-07-27 | 完成 W1-05 签名协议：固定 canonical payload、仓库外私钥门禁、Ed25519 签名工具、可信公钥清单和篡改/撤销测试已落地；生产 key 与签名等待新版本确认 |
| 2026-07-27 | 完成 W1-06：Registry 撤回原因/风险级别进入 Schema 和校验；撤回条目禁止新安装/新 Run，高风险返回独立阻断，历史 Run 保持只读审计 |
| 2026-07-27 | 推进 W5-01 至 W5-03：新增四个环境契约、Ed25519 项目 Manifest、ControlPlane、Profile/Registry/Lock Resolver、确定性 LocalExecutionPlan、Workspace verified state 与 `environment.manifest.get`；发布评测扩展至 8/8 |
| 2026-07-27 | 收紧 Registry trust boundary：lifecycle/revocation 进入签名 payload，Node/Go canonical 固定向量一致；Resolver 改为只接收 `VerifiedRegistry`，伪造状态、内容篡改或 revoked key 均 fail closed |
| 2026-07-27 | 完成 W5-04：新增 CreativeExecutionBundle 1.0 契约、确定性 ID/digest、Ed25519 签发验签和 Manifest/Registry/Lock/capability Resolver；业务正文与可执行 Pack 继续分离 |
| 2026-07-27 | 推进 W6-04：新增不可变 Run Bundle 存储、Daemon Environment Claim、Store 原子 eligible 候选租约；环境/Pack/capability 失败保持 queued 且零 Attempt，确定性评测证据同步更新 |
| 2026-07-27 | 完成 W2-05 与 W5-01 至 W5-03：bootstrap 和服务启动配置接入可信 Environment，required doctor、Registry verified cache、LocalExecutionPlan CLI/MCP 均通过测试；生产 key 保持发布门禁 |
| 2026-07-27 | 完成 W5-05：新增确定性 Pack preparation plan/apply、权限/费用披露、精确确认、Lock CAS、局部回滚、doctor 和新会话 handoff；CLI/MCP 共享同一实现 |
| 2026-07-27 | 完成 W6-04/W6-05：Automation 在租约前 fail closed，并使用 Attempt 级独占隔离目录、冻结输入、服务端心跳续租和最小 Agent 环境；run token 不落盘，失败 finish 与受限清理均有集成测试；评测保持 8/8 |
| 2026-07-27 | 确认并准备 `v0.5.0`：统一 CLI/Web/npm/Plugin/MCP 版本，重算 Plugin 与评测 digest，登记两类生产公钥，签署 `published` Registry，并增加可部署 Environment Profile |

## 9. 资料

- [OpenAI: Package your plugin](https://developers.openai.com/plugins/build/plugins)
- [OpenAI: Build skills](https://developers.openai.com/plugins/build/skills)
- [OpenAI: Build an MCP server](https://developers.openai.com/plugins/build/mcp-server)
- [OpenAI: Use plugins](https://learn.chatgpt.com/docs/plugins)
- [OpenAI: Codex app deep links](https://developers.openai.com/codex/app/deep-links)
- [OpenAI: Codex CLI reference](https://developers.openai.com/codex/cli/reference)
- [TapTap Maker npm package](https://www.npmjs.com/package/@taptap/maker)
- [TapTap Maker source](https://github.com/taptap/instant-games-open-mcp)
