# V4 本地与云端部署拓扑决策

## 1. 结论

ContentCloud 应学习 Codex Slides 的 Browser-first 交互，但首版不复制它的本地部署拓扑：

```text
Codex / Workspace（本地创作）
  -> Local MCP + CLI Gateway（解析绑定、构造链接、publish/pull）
  -> ContentCloud Web（云端治理工作台、协作、审核、决定）
```

推荐形态是“云端治理 Web + 本地轻伴随层”，不是“云端版和本地版两套 ContentCloud”。本地轻伴随层已经由 Workspace、CLI、MCP 和 Scene Plugin 构成；V4 只补充精确页面导航，不增加第二套业务数据库。

## 2. 为什么 Slides 适合本地服务

Codex Slides 的核心产品对象就是本机演示文稿。其本地 Next.js 服务同时承担：

- 读取和保存项目 JSON、生成文件与逐页图片。
- 驱动研究、渲染、编辑和播放等长流程。
- 为 Browser 提供可视化工作区。
- 在 Electron 打包版中绑定 `127.0.0.1`，数据保存在应用用户目录。

因此 Slides 的 localhost 不是单纯为了打开右侧栏，而是它的产品运行时和本地数据层。Browser 只是打开这个运行时返回的链接。

ContentCloud 的正式对象不同：SubmissionRevision、Decision、ApprovedSnapshot、WorkAssignment、成员权限和审计必须被多人共享，并由服务端持有。把这些对象搬到本地会立即产生同步、冲突和授权双轨问题。

ChatCut 的公开实现进一步验证了这个方向：它也没有为 Codex 启动完整 localhost 编辑器，而是用本地 Plugin 连接远程 MCP，并在 Browser 打开云端编辑器。ContentCloud 与它的区别是本地候选仍是创作事实源，不能把所有修改直接写入云端。完整对标见 [06-chatcut-benchmark.md](./06-chatcut-benchmark.md)。

## 3. 三种方案比较

| 维度 | 全云端 Web | Slides 式全本地 Web | 推荐的混合形态 |
| --- | --- | --- | --- |
| 未发布内容 | 不可见，隐私边界清楚 | 可直接展示 | 留在 Workspace，由 Codex 展示 |
| 多人审核与协作 | 单一事实源 | 需要另建同步协议 | 云端单一事实源 |
| 权限、审计、客户链接 | 统一实施 | 容易形成两套实现 | 统一由服务端实施 |
| 离线使用 | 只能继续本地创作，不能治理 | 本地页面可用 | 本地创作可用，治理明确离线 |
| 安装与升级 | Web 无本地 UI 安装 | 要处理运行时、端口、进程和升级 | 只维护已有 Plugin/CLI/MCP |
| ChatGPT Web Browser | 可访问 | 不能访问用户 localhost | 可访问云端治理页 |
| 数据泄露面 | publish 时按披露策略上传 | 本地 HTTP/file API 成为新攻击面 | 未发布正文不进入 Web |
| 实现成本 | 主要是 Web 与服务端 | 重做 UI、存储、同步和安全 | 增加导航契约，复用 V3 |

全本地方案并不会消除服务端：一旦需要客户审核、团队协作和跨设备使用，仍要建设云端。结果通常是两个 UI、两个状态模型和一套高风险同步协议。

## 4. 服务端 Web 的真实弊端

### 4.1 网络与可用性

- Browser 页面加载依赖网络、DNS、服务端和登录系统。
- 弱网时页面打开和刷新比 localhost 慢。
- 服务故障时仍可在 Workspace 创作，但无法查看最新决定或执行审核。

控制方式：本地创作不依赖 Browser；导航失败不改变 LocalRun；页面使用 Projection 缓存和明确的离线/过期时间；publish 网络状态不明时按幂等键查询，而不是重复提交。

### 4.2 登录摩擦

内置 Browser 使用独立 profile，首次使用可能需要重新登录。登录后还要重新验证 tenant、project 和 object 权限。

控制方式：使用正常会话认证和受控相对 return path；登录后恢复精确目标；不在 URL 中携带 token，也不由 MCP 搬运 cookie。

### 4.3 隐私与披露压力

云端页面只能看到已上传内容。为了“右侧能看见”而自动上传本地草稿，会破坏 V3 最重要的隐私边界。

控制方式：未 publish 的正文继续只在 Codex/Workspace 可见；上传必须经过 preflight、披露预览和用户确认；Evidence 使用 `metadata_only`、`evidence_pack`、`full_source` 分级。

### 4.4 延迟与成本

服务端要承担 Projection 查询、文件分发、认证、审计、监控、备份和多租户隔离；长期还有带宽、存储和运维成本。

控制方式：Browser 复用现有 ProjectProjection，不建立 Browser 专用副本或轮询通道；资源按 digest 缓存；首版不实时同步 LocalRun。

### 4.5 安全责任集中

服务端一旦出现 IDOR、跨租户授权或披露错误，影响可能超过单台设备。

控制方式：授权顺序固定为 user -> tenant -> project -> view -> object -> disclosure -> command；深链不改变授权；所有决定绑定不可变 revision digest；安全测试作为发布门禁。

## 5. 应该向 Slides 与 ChatCut 学什么

应该复制的模式：

1. 一个 Tool 返回标准 `resource_link` 和精确业务焦点。
2. Skill 要求 Agent 实际导航并验证页面，不能只打印 URL。
3. Browser 打不开时保留链接和上下文，明确降级。
4. 长流程结果有稳定 ID，可重新打开，不依赖一次对话存活。
5. 本地程序只监听和访问自己负责的数据边界。
6. 云端 Web 应是持续可见、可完成自身职责的工作台，而不是被动状态截图。
7. 使用统一 `/codex` 入口降低 Plugin 安装、OAuth、验证和新对话交接成本。

不应该复制的实现：

- 本地 Next.js 项目数据库和 Electron 壳。
- 在 localhost 复刻审核、权限、Assignment 和 ApprovedSnapshot。
- 让 Browser 页面成为本地/云端同步协议。
- 为每个页面或对象创建一个 MCP Tool。
- 为视觉相似而上传未发布 Workspace 正文。

## 6. 是否保留本地页面的可能性

保留，但不进入 V4 首版。只有出现以下可验证需求时，才评估一个窄范围的 `Local Preview`：

- 用户频繁需要在 Browser 中比较未发布的复杂视觉产物，Codex 文本展示明显不足。
- 目标客户有稳定的离线或内网隔离需求，云端治理页面无法满足。
- 真实使用数据证明云端往返延迟阻断主要创作流程，而不是偶发登录摩擦。

即使触发，也只能是只读或本地候选编辑器：绑定 `127.0.0.1`、只允许已验证 Workspace Root、随机端口、无任意文件读取、无云端 Decision、无第二套数据库，并随宿主进程退出。它不能被命名或实现为“本地 ContentCloud 服务端”。

## 7. 分阶段选择

### V4 首版

- 右侧 Browser 打开服务端 ContentCloud Web。
- 本地导航 Tool 解析 WorkspaceBinding、构造 allowlisted URL、返回链接；其他本地 Tool 继续负责 Workspace 和显式交换。
- Web 可以执行授权的 Comment、Decision、Assignment、Context Revision 和 Automation Plan 命令。
- publish/pull 仍是唯一跨边界交换。
- 没网或 Browser 不可用时，用户仍可继续本地候选创作。

### V4 验收后

- 记录 Browser 打开耗时、登录失败、不可用宿主和用户回退频率。
- 访谈确认用户缺的是“云端页面更快”，还是“未发布内容需要可视化预览”。
- 只有达到上一节的触发条件，才为 `Local Preview` 新建立项和威胁模型。

## 8. 决策摘要

当前决策不是“只要云端，不要本地”，而是：

```text
本地拥有创作连续性和未发布候选
云端拥有协作治理和正式事实
Browser 提供精确、可操作的云端治理工作台
CLI Gateway 提供显式数据交换
```

这保留了 Slides 最有价值的交互灵感，同时避免为 ContentCloud 引入不必要的双端状态和同步复杂度。
