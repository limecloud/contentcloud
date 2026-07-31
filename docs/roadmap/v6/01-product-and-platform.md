# V6 产品与平台需求

## 1. 产品结构

Content Work OS 由五层组成。每一层必须有清晰的责任边界和可读状态。

| 层 | 责任 | 用户看到的对象 |
| --- | --- | --- |
| Control Plane | 租户、身份、权限、项目、能力、Registry、审核、审计 | Tenant、Workspace、Capability、Manifest、Audit |
| Workspace Plane | 本地文件、上下文、候选、缓存和 Handoff | Local Workspace、Sync State、Inbox |
| Runtime Plane | CLI、Daemon、Agent Adapter、能力探测和任务执行 | Device、Runtime Health、Attempt、Progress |
| Content Plane | 统一内容批次和类型专属结构 | ContentBatch、ContentItem、ArticleItem、Pack |
| Delivery Plane | 批准事实导出到用户负责的外部渠道 | ApprovedSnapshot、DeliveryPackage、Published Binding |

## 2. 目标用户

### 内容生产者

在 Codex 或 Claude Code 中处理资料、知识、策略和内容候选。需要快速恢复 Workspace，知道当前可用 Pack，并能在本地完成 lint 和 publish 预检。

### 审核与运营人员

在 Web 中比较不可变 Revision，处理评论、批准、交付和异常。不能被模型对话、最新草稿或本地路径混淆。

### 租户管理员

控制本租户的 Content Pack、Runtime 和外部交付能力；可以看到哪些能力由平台提供、哪些只是未开通、哪些配置异常。

### 平台运营与安全人员

维护签名 Registry、Profile、Pack 发布、撤回、版本兼容窗口、审计和生产验收证据。

## 3. Work OS 首页

登录后的首页不做营销 Hero，而是显示当前工作面：

1. 当前租户、Workspace 和 Agent Runtime。
2. 待处理的审核、评论、决策和交付。
3. 正在运行的 Automation、进度和恢复状态。
4. 已启用的 Content Pack 及其下一步动作。
5. 阻断项：Manifest 失效、租约过期、能力未开通、版本不兼容、权利或事实缺失。

首页需要使用统一的 `next_action`/页面目标，而不是让每个模块手写 URL。高优先动作在两次点击内可达；低权限用户只能看到可访问对象的摘要。

## 4. 状态语义

所有控制面和官网状态使用以下有限集合：

| 状态 | 含义 | 允许的用户动作 |
| --- | --- | --- |
| `enabled` | 平台实现且租户已开通 | 进入生产、提交、审核和交付 |
| `disabled` | 平台实现但租户未开通 | 查看开通要求，不能创建生产对象 |
| `unavailable` | 平台尚未实现或已撤回 | 查看 planned 信息，不能执行 |
| `misconfigured` | 已开通但 Runtime、Pack、Manifest 或版本异常 | 进入诊断，不允许绕过门禁 |

不要把 `disabled` 显示为“暂不支持”，也不要把 `misconfigured` 显示为“关闭”。这两个状态对应不同的运营动作。

## 5. 核心用户流程

### 5.1 首次连接

```text
登录 -> 选择租户/项目 -> 创建 ConnectSession -> 本地 bootstrap plan
     -> 用户确认计划 -> 浏览器设备授权 -> Workspace register
     -> 获取签名 Manifest -> doctor -> 打开 Agent 新会话
```

V6 在每一步都显示可验证的状态和失败原因。Bootstrap 不能因官网宣传或静态配置而启用租户未授权的 Pack。

### 5.2 本地生产到审核

```text
Workspace pull -> Agent 读取 Context/Knowledge -> Content Pack 生成 candidate
-> 本地 lint -> publish dry-run -> 用户确认 publish -> SubmissionRevision
-> Web review -> approve -> ApprovedSnapshot -> pull -> DeliveryPackage
```

### 5.3 Automation

```text
用户确认 Automation Plan -> 服务端签发 TaskContract/ExecutionBundle/租约
-> Daemon poll/lease -> Attempt 隔离目录 -> Agent 全权限执行
-> 心跳/进度/SSE -> Schema 复验 -> report/finish -> 审核或交付门禁
```

Automation 仍然不是普通本地创作的必经路径。官网只能描述“可选的受治理自动运行”，不能宣传为无限制后台代理。

## 6. 信息架构

### 公共官网

- Product：Content Work OS、工作面和生产闭环。
- Runtimes：Codex、Claude Code 及 Registry 中真实可用的客户端。
- Content Packs：视频剧本、微信公众号文章和规划中的 Pack。
- Governance：审批、审计、租户能力和外部平台边界。
- Docs：按客户端、内容形态和状态进入文档。
- Connect：创建 Workspace 连接和安装 CLI。

### 登录后工作台

- 工作台
- 项目
- 审核协作
- 交付
- Automation 与运行
- 团队
- 租户能力与设置（按权限显示）

### 平台后台

- Tenant Capability Matrix
- Agent Client Registry
- Content Pack Registry
- Environment Profile/Manifest
- Device/Daemon health
- Audit and Release evidence

## 7. 成功指标

### 激活

- 新租户从登录到完成 Workspace doctor 的中位时间小于 10 分钟。
- 首次连接失败时，90% 的用户能在页面上明确定位到客户端、权限、版本或 Manifest 原因。

### 生产

- 视频和公众号内容均可从统一 Work OS 首页进入，不产生隐藏的类型专用入口。
- 从本地候选到创建 Submission 的流程不超过一次重复输入。

### 治理

- `disabled`/`unavailable`/`misconfigured` 无法绕过 API、CLI、MCP、Skill 或 Handoff 门禁。
- 每个 ApprovedSnapshot 都能追溯到租户、Workspace、Content Pack、Runtime 和输入摘要。

### 运营

- Daemon 断网、重启、版本不兼容和 dead-letter 都有可读状态和下一步动作。
- 官网客户端、Content Pack 和状态文案通过 Registry 检查，避免静态漂移。
