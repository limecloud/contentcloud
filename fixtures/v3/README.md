# ContentCloud V3 验收数据包

`jinling-gudu.json` 是脱敏、固定、可重复导入的开发验收数据，不是服务端默认 seed，也不包含客户原件。

## 本地 Workspace

只允许写入空目录，不提供覆盖或 `force`：

```bash
contentcloud workspace fixture apply fixtures/v3/jinling-gudu.json \
  --directory /path/to/empty-workspace \
  --project-id project-fixture \
  --workspace-id workspace-fixture \
  --target codex
```

命令会实际执行 Source 登记与 ingest、Knowledge lint、十五维诊断、七层 Pack、ApprovedSnapshot、Brief、十条 blocked ContentItem 和 completed LocalRun。任何一步未通过确定性门禁时，命令返回失败。

## 服务端

开发模式下可把同一 JSON 作为 `POST /api/v1/dev/bootstrap` 的请求体。Handler 只负责严格解码；项目正文和验收状态全部来自数据包。重复导入不会重复创建 Project、Workspace、Submission 或 ApprovedSnapshot。

## 验收基线

- 20 个可校验 Source 与 EvidenceBundle。
- 15 个方法论维度、4 个研发节点和七层 KnowledgePack。
- Fact、Claim、Asset、Rights、Conflict 各至少两个状态。
- 1 个 completed LocalRun。
- 10 个结构有效但因授权素材不足而 blocked 的 ContentItem。
- 服务端 1 个待审 Submission、1 个 changes requested Submission 和 1 个 ApprovedSnapshot。
