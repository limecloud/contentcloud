# Desktop 同步、审批与上传

状态：`目标协议`。

更新时间：2026-08-17。

## 1. 状态模型

Desktop 对每个对象同时展示独立状态轴：

| 轴 | 状态 | 权威 |
| --- | --- | --- |
| 本地文件 | clean / modified / deleted / conflict | Local Workspace |
| 传输 | idle / queued / hashing / uploading / downloading / synced / failed | Local Sync Engine |
| 审批 | unsubmitted / pending / changes_requested / approved / rejected / expired | Cloud Review |
| 生命周期 | draft / ready / delivered / archived | 业务拥有域 |
| 执行 | queued / running / waiting / failed / succeeded | Runtime |

UI 可以组合显示，但 API、持久化和测试不能合并这些状态。

## 2. 本地修改到云端 Revision

```mermaid
sequenceDiagram
    actor User as 用户或 Codex
    participant Kernel as Local Workspace Kernel
    participant Sync as Sync Engine
    participant Outbox as SQLite Outbox
    participant API as Cloud API
    participant DB as Cloud Revision
    participant UI as Desktop

    User->>Kernel: Proposal / Apply 或外部文件修改
    Kernel->>Kernel: 路径、Claim、revision、digest 校验
    Kernel-->>UI: local view invalidated
    Kernel->>Sync: normalized change + content digest
    Sync->>Outbox: append(client_mutation_id, base_revision, digest)
    Sync-->>UI: transfer_state=queued
    Sync->>API: publish preflight
    API-->>Sync: accepted / conflict / policy denied
    alt accepted
        Sync->>API: upload/finalize + idempotency key
        API->>DB: create immutable revision
        DB-->>Sync: revision + digest
        Sync->>Outbox: ack exact command
        Sync-->>UI: synced + cloud revision
    else stale base
        Sync->>Outbox: mark conflict, stop automatic retry
        Sync-->>UI: conflict with local/base/remote refs
    else transient network error
        Sync->>Outbox: retry_at + bounded backoff
        Sync-->>UI: queued/failed with recovery action
    end
```

`synced` 只在服务端返回并重新核对 revision/digest 后成立。上传完成、HTTP 2xx 或 WebSocket 在线都不能单独推导同步成功。

## 3. 云端事件回流

```mermaid
sequenceDiagram
    participant Server as Cloud Event Stream
    participant Sync as Sync Engine
    participant Cache as Local Projection Cache
    participant UI as Desktop
    participant MCP as Codex MCP

    Server-->>Sync: event(id, subject_ref, revision, type)
    Sync->>Sync: verify cursor and device/project scope
    alt cursor continuous
        Sync->>Server: query changed projection
    else event gap
        Sync->>Server: scoped resync from checkpoint
    end
    Server-->>Sync: typed projection + digest
    Sync->>Cache: atomic replace + cursor
    Sync-->>UI: projection invalidated
    Sync-->>MCP: workspace/cloud context invalidated
    UI->>Sync: reload affected view
```

事件不携带完整客户正文和短期凭据。事件丢失通过 cursor gap 与范围化 resync 修复，不靠客户端猜测。

## 4. 大文件上传

```mermaid
flowchart TD
    A[用户拖入文件] --> B[Workspace containment 和类型检查]
    B --> C[流式计算 SHA-256 / size / MIME]
    C --> D{服务端已有同摘要对象?}
    D -->|是| E[绑定已有 Blob 引用]
    D -->|否| F[申请 scoped upload session]
    F --> G[分片并发上传]
    G --> H{中断或进程重启?}
    H -->|是| I[从 SQLite 读取已确认分片]
    I --> G
    H -->|否| J[finalize 总摘要]
    J --> K{摘要和权限有效?}
    K -->|否| L[失败，不创建 Revision]
    K -->|是| M[创建资料/Artifact Revision]
    E --> M
    M --> N[服务端异步 OCR/转写/转码/扫描]
    N --> O[事件回流 Desktop]
```

约束：

- 分片大小和并发由服务端能力协商，不写死在 Renderer。
- 上传 URL 绑定 tenant、project、object、part、device 和绝对 TTL。
- 重试只发送服务端未确认分片。
- finalize 必须核对完整 digest 和预期 size。
- 本地文件变化后旧上传会话作废，不能把混合分片合成为新对象。
- 处理状态与审批状态分开；OCR 完成不表示资料已批准。

## 5. 审批与修改回流

```mermaid
sequenceDiagram
    actor Reviewer as 审批人
    participant UI as Desktop
    participant Review as Cloud Review API
    participant Runtime as Job Runtime
    participant Codex as Codex
    participant Workspace as Local Workspace

    Review-->>UI: gate pending(subject, revision, digest)
    Reviewer->>UI: 打开精确版本、diff、来源和风险
    alt 批准
        UI->>Review: approve(revision, digest, reason, idempotency)
        Review->>Review: 权限、Gate、revision 再校验
        Review-->>Runtime: gate satisfied event
        Review-->>UI: ApprovedSnapshot
    else 要求修改
        UI->>Review: request changes(comment, revision, digest)
        Review-->>UI: changes_requested
        Reviewer->>Codex: 通过对象 Handoff 请求修订
        Codex->>Workspace: Claim -> Proposal -> Apply
        Workspace-->>UI: new local revision
        UI->>Review: submit new cloud revision
        Review-->>UI: old review expired, new review pending
    else 驳回
        UI->>Review: reject(revision, digest, reason)
        Review-->>Runtime: branch rejected/cancelled per plan
    end
```

审批不允许：

- Codex、Worker、Provider 或 Electron 自动点击替代人工决定。
- 上游内容变化后沿用旧批准。
- 使用文件名、路径或 UI 当前选中项代替 revision/digest。
- Renderer 离线时先显示“已批准”再等待服务端确认。

## 6. 冲突决策图

```mermaid
flowchart TD
    A[收到 stale base / remote changed] --> B{本地是否 modified?}
    B -->|否| C[安全拉取 Cloud Revision]
    B -->|是| D{对象类型}
    D -->|结构化文本| E[生成 base/local/remote diff]
    E --> F{确定性无冲突合并?}
    F -->|是| G[生成 Proposal，用户确认 Apply]
    F -->|否| H[用户选择、编辑或交给 Codex 修订]
    D -->|图片/音频/视频/PDF| I[保留 local 和 remote 两个版本]
    I --> J[用户选择保留、另存或创建新版本]
    G --> K[以最新 base 重新提交]
    H --> K
    J --> K
```

禁止 last-write-wins。自动三方合并也只能产生 Proposal，不能绕过 Apply。

## 7. 离线与恢复状态机

```mermaid
stateDiagram-v2
    [*] --> Online
    Online --> Offline: network unavailable
    Offline --> Queued: local changes
    Queued --> Reconnecting: network restored
    Reconnecting --> Resyncing: cursor gap or stale projection
    Reconnecting --> Uploading: cursor continuous
    Resyncing --> Conflict: local and remote changed
    Resyncing --> Uploading: no conflict
    Uploading --> Online: server ack + digest verified
    Uploading --> Queued: transient failure
    Uploading --> AuthRequired: credential revoked/expired
    Conflict --> Uploading: user-approved resolution
    AuthRequired --> Reconnecting: device reauthorized
```

## 8. 删除、归档和空间管理

- 本地删除形成 tombstone，并等待服务端确认。
- 已批准、被任务固定引用或已交付的对象不能直接物理删除；进入 archived 或新建撤销事实。
- 云端归档不删除本地 modified 内容。
- 缩略图、转码缓存、下载缓存和本地索引可按空间策略清理并重建。
- 用户执行“释放本地空间”时必须预览将清理的缓存与仍保留的原始文件，不能混用。

## 9. 必测故障矩阵

| 故障 | 预期 |
| --- | --- |
| 文件写入一半时被监听 | 等待稳定窗口，不上传部分内容 |
| Daemon 在 outbox append 后崩溃 | 重启继续同一幂等命令 |
| 服务端创建 Revision 后响应丢失 | 重试返回原 Revision，不重复创建 |
| 分片上传到 70% 后断网 | 从服务端已确认分片恢复 |
| 两台设备同时修改文本 | 出现三方 diff，不静默覆盖 |
| 本地修改与远端二进制更新冲突 | 两版本均保留 |
| 审批后内容改变 | 旧审批 expired，交付被阻断 |
| 事件 ring/cursor gap | scoped resync 后与服务端一致 |
| 设备被撤销 | 停止上传和新任务，保留本地内容 |
| SQLite 损坏或删除 | 从 Workspace 与 Cloud 重建 |
