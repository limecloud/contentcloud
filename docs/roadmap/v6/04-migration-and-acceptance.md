# V6 迁移、风险与验收

## 1. 迁移原则

V6 是产品语义和控制面升级，不是一次性重写。所有迁移按“先投影、再收敛、后删除”执行。

1. 先保留 V3/V4/V5 契约和现有 API，新增 Work OS 投影层。
2. 让 Web、CLI、MCP、Skill 和官网读取同一组 Registry/Manifest/Capability 数据。
3. 旧入口标记为 `compat`，新入口稳定后再决定是否删除。
4. 任何删除、数据库迁移或外部平台动作都单独评审，不由官网或 Content Pack 隐式触发。

## 2. 现有能力映射

| 现有能力 | V6 名称 | 迁移动作 |
| --- | --- | --- |
| Workspace / Bootstrap | Workspace Plane | 增加更清晰的连接状态和运行时诊断投影 |
| CLI / MCP / Skill | Runtime Plane | 统一显示版本、能力和 Pack 来源 |
| Tenant Content Capability | Capability Control | 扩展四态语义和管理审计 |
| Environment Manifest | Runtime Contract | 作为 Web/CLI/官网的共享投影 |
| ContentBatch | Content Plane | 保留通用批次，按 Pack 路由内容类型 |
| Video/Article Item | Content Pack | 保持领域 Schema，收敛入口和状态显示 |
| Daemon / TaskRun / RunAttempt | Automation Runtime | 增加 Work OS 首页的进度、恢复和异常入口 |
| Submission / Approval | Governance Plane | 在官网和工作台解释不可变 Revision 与 ApprovedSnapshot |
| Delivery Package | Delivery Plane | 继续坚持用户负责外部平台登录、上传和发布 |

## 3. 发布前阻断项

以下问题未解决前，不能宣称 V6 生产就绪：

- Bootstrap 计划的 `would_enable_daemon` 与测试和产品预期一致。
- 固定 Plugin 规格、Marketplace Registry、Profile allowlist 和内置信任数据使用同一版本。
- `go test ./...`、`go test -race ./...`、`go vet ./...` 全部通过。
- Web 64 个现有测试和生产构建保持通过；新增测试后以当次主干总数为准，不允许通过删测试满足门禁。
- Plugin/Skill/Content Governance 校验与新官网 Registry 校验通过。
- 真实 macOS 设备完成 Daemon 安装、升级、停止、重启、断网重报和多 Workspace 验收。

## 4. Golden Journey

1. 租户管理员创建租户并确认默认只开启 `video_script`。
2. 管理员开启 `wechat_article`，生成新的 Manifest digest。
3. 内容生产者从官网进入工作台，选择项目后创建 ConnectSession，完成 CLI bootstrap 和 Workspace doctor。
4. Codex 读取 Manifest，发现两个 Content Pack 状态与 Web 一致。
5. 在本地分别生成视频 ContentBatch 和公众号 ArticleItem。
6. 对两个类型运行 lint、diff、finalize 和 publish dry-run。
7. 用户确认 publish，服务端创建不可变 SubmissionRevision。
8. 审核人员在 Web 查看安全渲染、评论并批准一个 Revision。
9. 本地 pull ApprovedSnapshot，生成视频交付包或公众号本地交付包。
10. Automation Plan 触发受租约控制的 Daemon Attempt，断网后恢复进度并重报结果。
11. Work OS 首页显示待审、运行中、阻断项和最近交付。
12. 审计日志能从结果反查租户、Workspace、Runtime、Pack、Revision 和 Manifest。

## 5. 风险登记

| 风险 | 影响 | 应对 |
| --- | --- | --- |
| 官网静态状态与 Registry 漂移 | 用户看到不可用或错误的能力 | 发布前运行 Registry 校验，生产改为受控数据源 |
| `disabled` 与 `unavailable` 混淆 | 租户误以为可用，或运营误开未实现功能 | 四态状态与 reason_code 强制统一 |
| Daemon 全权限宣传过度 | 用户误解安全边界 | 官网明确“仅租约内 Automation”，工作台显示当前租约 |
| V5 兼容入口过多 | 维护成本和事实源分裂 | 标记 compat，禁止新功能继续增加旧硬编码 |
| Content Pack 复制业务逻辑 | 新类型扩展变成条件分支 | 以 Pack manifest、Schema 和 capability routing 扩展 |
| 真实设备验收滞后 | 线上安装/升级失败 | M6-5 前完成 macOS/Windows 设备矩阵和发布包检查 |

## 6. 完成定义

V6 只有在以下条件同时满足时才从 Beta 进入生产：

- 产品定位、官网、工作台和文档统一使用 Content Work OS 词汇。
- Tenant Capability 是租户开关唯一事实源，Web/CLI/MCP/Skill/审批/交付均复验。
- 视频默认可用，公众号仅在租户显式开通后可用。
- Runtime、Workspace、Pack、Manifest 和 Automation 状态可诊断且不泄露秘密。
- V5 的本地 Daemon 闭环和现有内容生产链无回归。
- 官网首屏、导航、状态文案和 CTA 经过真实用户评审，并通过响应式与可访问性验收。
