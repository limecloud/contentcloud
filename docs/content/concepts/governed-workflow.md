# 受治理的内容工作流

ContentCloud 把创作和治理分在两个平面：本地 Workspace 保存创作候选和跨会话状态，云端保存项目、不可变提交、人工决定、批准快照与审计。

## 核心对象

| 对象 | 所在位置 | 含义 |
| --- | --- | --- |
| Local candidate | 本地 Workspace | 尚未提交的候选内容，可以继续修订 |
| LocalRun | 本地 Workspace | 一次受约束工作的状态、输入、检查和输出 |
| Handoff | 本地 Workspace | 跨会话或跨人员恢复工作的确定性入口 |
| SubmissionRevision | 云端 | 用户显式提交的不可变版本 |
| Decision | 云端 | 审核人的批准或退回决定 |
| ApprovedSnapshot | 云端 | 绑定精确 Revision 摘要的正式批准事实 |
| DeliveryPackage | 本地与云端投影 | 基于批准快照生成并校验的交付物 |

## 标准流程

```text
读取 Workspace 状态
  -> 选择或创建 LocalRun
  -> 冻结输入并取得单写者 claim
  -> 生成候选并执行确定性 lint
  -> publish preflight
  -> 用户确认精确计划
  -> 创建不可变 SubmissionRevision
  -> Web 人工审核
  -> ApprovedSnapshot 或 changes_requested
  -> pull 明确版本
  -> 继续修订或生成交付包
```

## 四条不能混淆的边界

1. **本地保存不等于云端提交。** 只有显式 publish 才创建 SubmissionRevision。
2. **提交不等于批准。** 只有服务端人工 Decision 才能产生 ApprovedSnapshot。
3. **批准不等于交付。** 交付包必须从精确批准快照生成并通过类型化检查。
4. **交付不等于外部发布。** 外部平台的草稿、预览、提交和发布成功是不同状态。

## 为什么每次新对话都读取 Workspace

聊天记录不是项目事实源。`workspace_context` 会读取持久化状态、当前 revision、活跃 Run、Handoff 和修复要求。安装 Plugin、修改环境或转交工作后，应在同一 Workspace Root 新建对话并重新读取上下文。

## 服务端边界

ContentCloud 服务端保持 zero-exec：不替用户运行 Agent、Skill、Renderer 或客户上传代码，也不扫描本地 Workspace。服务端只能看到用户明确提交的范围。
