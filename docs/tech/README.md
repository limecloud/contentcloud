# 技术图源

状态：`Mermaid 源文件是事实源；SVG/PNG 仅为渲染产物`。

更新时间：2026-08-17。

## 图谱

| 图 | Mermaid 源 | 渲染产物 | 状态 |
| --- | --- | --- | --- |
| Agentic Job Runtime 总架构 | [contentcloud-agentic-job-runtime-architecture.mmd](./contentcloud-agentic-job-runtime-architecture.mmd) | SVG / PNG | current |
| Desktop、Codex、Web 与本地/云端边界 | [contentcloud-desktop-architecture.mmd](./contentcloud-desktop-architecture.mmd) | SVG / PNG | target；Electron 尚未实现 |

架构语义以 `.mmd` 与对应 ADR/产品文档为准。修改架构时先改源文件，再使用项目固定 Mermaid CLI 渲染；没有渲染工具时不得手工伪造 SVG/PNG。

Desktop 的启动时序、同步时序、大文件上传、审批回流、冲突决策和离线状态机维护在：

- [Desktop 架构与技术](../product/content-work-os-desktop/02-architecture-and-technology.md)
- [Desktop 同步、审批与上传](../product/content-work-os-desktop/03-sync-review-upload.md)
- [Desktop 一次性交付计划](../product/content-work-os-desktop/04-delivery-plan.md)
