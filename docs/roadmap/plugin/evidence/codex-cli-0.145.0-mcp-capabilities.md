# Codex CLI 0.145.0 MCP 能力探针

日期：2026-07-27。

目标：验证 ContentCloud 是否可以在 Codex CLI 中依赖 MCP Roots 和 Resources 定位、恢复 Workspace，而不是从 Maker 的实现类推。

## 环境

- Codex：`codex-cli 0.145.0`
- MCP transport：stdio
- MCP protocol：客户端选择 `2025-06-18`
- 探针：`scripts/probes/codex-mcp-probe.mjs`
- 会话参数：`--ephemeral`、`--ignore-user-config`、`--ignore-rules`、只读 sandbox
- 用例：单工作目录；工作目录加一个 `--add-dir`

探针只记录 MCP 初始化元数据、方法名和 Roots URI。它不记录 Prompt、凭据、文件内容或业务数据。

## 结果

单目录与 `--add-dir` 用例得到相同初始化能力：

```json
{
  "protocolVersion": "2025-06-18",
  "clientInfo": {
    "name": "codex-mcp-client",
    "title": "Codex",
    "version": "0.145.0"
  },
  "capabilities": {
    "elicitation": {
      "form": {},
      "url": {}
    }
  }
}
```

随后 Codex 发送：

```text
notifications/initialized
tools/list
```

Codex 没有声明 `roots` capability，因此探针按 MCP 协议没有发送 `roots/list`。即使 CLI 使用 `--add-dir`，额外目录也没有通过 MCP Roots 传给 Server。

Server 声明了 `resources` capability，但启动阶段没有收到 `resources/list`。当前官方 Codex Manual 的“Supported MCP features”明确列出 stdio、streamable HTTP、认证和 server instructions，没有把 Resources 列为可依赖的 Codex 能力。因此本次证据不能证明 Codex 对模型暴露 MCP Resources。

模型请求阶段因本机已有 API key 失效而返回 401，但 MCP initialize、initialized 和 tools/list 均已在请求模型前完成。该认证失败不影响上述协议结果，也没有为测试修改凭据。

## 实施结论

1. Codex CLI `0.145.0` 不使用 MCP Roots 作为 ContentCloud Workspace 定位机制。
2. `--add-dir` 不是 MCP Roots 的替代接口，不能让 MCP Server 自动知道多个 Codex 工作目录。
3. Codex MVP 的 Resolver 顺序改为：Tool 显式 `directory`，否则从插件 MCP 进程 `cwd` 向上唯一查找 `.contentcloud/project.yaml`；无法唯一识别时 fail closed。
4. conversation context 和 status 在 Codex 中采用 typed Tool-first。Resource 可以复用同一 handler 作为其他宿主兼容层，但不作为 Codex 门禁。
5. Codex Desktop 必须独立重复探针，不能从 CLI 结果推断。
