# ADR-0019：Local Workspace 与 Cloud Revision 同步权威

状态：`Accepted`

日期：2026-08-17。

决策者：产品与平台工程。

关联：

- [Desktop 同步、审批与上传](../../product/content-work-os-desktop/03-sync-review-upload.md)
- [ADR-0018 Desktop 产品面、Electron 技术栈与仓库拓扑](./ADR-0018-desktop-surface-and-repository-topology.md)
- [客户资产入口](./ADR-0014-customer-asset-surface.md)

## 背景

Desktop 将长期观察本地项目目录，并与 ContentCloud 服务端进行上传、下载、审批和交付。如果 Electron、SQLite、本地文件和服务端数据库都能独立决定同一对象状态，系统会出现静默覆盖、过期审批、重复上传和无法解释的冲突。

现有本地工作台已经使用 revision、digest、Claim、Proposal 和 CAS 保护本地修改；服务端已经使用不可变 Revision、ApprovedSnapshot、Gate、Artifact 和 DeliveryPackage 保存云端治理事实。需要冻结两类事实源之间的同步协议，而不是增加第三套 Desktop 状态机。

## 决策

### 1. 两个事实源

- **Local Workspace**：未提交文件、草稿、项目目录结构和本地生成内容的权威事实源。
- **Cloud Revision**：已提交版本、审批、团队协作、正式资产、运行状态和交付的权威事实源。

Go 本地服务中的 SQLite 只保存可重建索引、服务器投影缓存、同步游标、上传分片记录和 outbound outbox。Electron Renderer 不保存业务事实。

### 2. 四条独立状态轴

禁止用一个 `status` 同时表达文件、传输、审批和业务生命周期。

```text
local_state      clean | modified | deleted | conflict
transfer_state   idle | queued | hashing | uploading | downloading | synced | failed
review_state     unsubmitted | pending | changes_requested | approved | rejected | expired
lifecycle_state  draft | ready | delivered | archived
```

Runtime 的 `queued/running/waiting/failed/succeeded` 是第五条独立执行状态轴，不能覆盖上述任何状态。

### 3. 本地到云端

1. 文件监听形成 Workspace-relative 变更和内容 digest。
2. Local Kernel 验证允许目录、文件类型、大小、symlink、revision 和当前 Claim。
3. Sync Engine 写入持久 outbox，命令包含 `client_mutation_id`、`base_revision`、`content_digest` 和幂等键。
4. 大文件先申请上传会话，按分片上传并可恢复；服务端 finalize 时重新验证总摘要。
5. 服务端只在 base revision 仍有效时创建不可变 Cloud Revision。
6. 收到确认事件后，本地记录 server revision 和同步摘要；不得以“请求已发送”推导 synced。

### 4. 云端到本地

1. Daemon 通过带单调游标的 WebSocket/SSE 接收失效事件。
2. 事件只通知对象和 revision 变化，正文通过认证 Query 拉取。
3. 游标缺口、断线过久或事件摘要不连续时执行范围化 resync。
4. 云端内容不能直接覆盖 modified 本地文件；必须形成冲突对象或新的受控 Proposal。
5. 批准快照以只读缓存进入 Workspace，保留来源、版本、摘要和审批时间。

### 5. 冲突规则

- 文本对象可以生成 base/local/remote 三方差异，但合并结果仍需 Proposal/Apply。
- 二进制和大媒体不自动合并；保留本地与云端两个版本，由用户选择或另存。
- 目录重命名与删除使用对象身份和 tombstone，不只比较路径字符串。
- 同一个幂等键和相同摘要返回原结果；同一个键和不同摘要返回冲突。
- 服务端拒绝 stale base 时，本地 outbox 进入 `conflict`，禁止无条件重试覆盖。

### 6. 审批规则

- 审批必须绑定精确 `subject_id + revision + digest`。
- 上游内容变化后，旧审批进入 expired，不能批准新内容。
- Desktop 可以展示、评论、批准、驳回和要求修改，但命令由服务端重新校验负责人、权限、输入版本和 Gate 状态。
- Codex 可以解释反馈和生成修订，不能代表用户批准。

### 7. 删除与归档

- 本地删除先形成 tombstone 和待同步命令；服务端存在已批准或交付引用时可以拒绝物理删除，只允许归档。
- 云端归档不会静默删除本地未提交内容。
- 本地缓存和缩略图可以清理；原始文件、Cloud Revision、ApprovedSnapshot 和审计记录遵循各自保留策略。

## 备选方案

### 方案 A：服务端始终覆盖本地

实现简单，但会丢失离线创作和 Codex 刚生成但尚未同步的内容。不采用。

### 方案 B：本地始终覆盖服务端

会绕过团队审批、权限和正式交付版本，也无法处理多设备修改。不采用。

### 方案 C：所有对象采用 CRDT

首版主要对象是文档、结构化清单和大媒体，不存在已验证的实时多人逐字符协作需求。CRDT 增加协议、存储和调试复杂度，不采用。

### 方案 D：Electron SQLite 成为离线业务数据库

会形成第三个事实源，并把 Go、Web 和 Codex 的规则复制到 Node 侧。不采用。

## 安全与运行影响

- Upload slot、下载 URL 和事件 capability 必须短期、限对象、限设备、可撤销。
- 路径、原文、设备 Token 和上传凭据不得出现在事件、通知或 Codex 可见 metadata。
- Outbox 重试必须区分可重试网络错误、认证终止错误、策略拒绝和摘要冲突。
- 诊断包只包含错误码、哈希化对象引用、游标、摘要和受限日志，不默认上传。

## 验证

1. 断网创建、修改、删除、重命名后恢复连接，不丢事件且不重复创建 Revision。
2. 两台设备基于同一 base 修改文本和二进制，均得到可解释冲突，不发生静默覆盖。
3. 分片上传中断、应用重启、Daemon 重启后从已确认分片恢复。
4. 审批后上游内容变化，旧审批失效并阻断交付。
5. 事件游标 gap 触发范围化 resync，最终状态与服务端一致。
6. 删除本地 SQLite 后可以从 Workspace 和 Cloud 重建，不丢业务事实。

## 回退

同步协议出现风险时，停止自动 push 和自动 pull，只保留显式 `publish`、`pull` 和只读状态查询。不得通过关闭 CAS 或忽略 digest 恢复可用性。

## 后果

正面：本地创作、云端治理和离线恢复都有明确权威，Desktop 可以持续工作而不制造第三套状态。

代价：必须实现 outbox、事件游标、上传恢复、冲突对象和多状态轴，不能用简单“最后写入覆盖”完成同步。
