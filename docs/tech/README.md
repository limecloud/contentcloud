# 技术图源

状态：`Mermaid 源文件是事实源；SVG/PNG 仅为渲染产物`。

更新时间：2026-08-17。

## 图谱

| 图 | Mermaid 源 | 渲染产物 | 可编辑场景 | 状态 |
| --- | --- | --- | --- | --- |
| Agentic Job Runtime 总架构 | [contentcloud-agentic-job-runtime-architecture.mmd](./contentcloud-agentic-job-runtime-architecture.mmd) | [SVG](./contentcloud-agentic-job-runtime-architecture.svg) / [PNG](./contentcloud-agentic-job-runtime-architecture.png) | [Excalidraw](./contentcloud-agentic-job-runtime-architecture.excalidraw) | current |
| Desktop、Codex、Web 与本地/云端边界 | [contentcloud-desktop-architecture.mmd](./contentcloud-desktop-architecture.mmd) | [SVG](./contentcloud-desktop-architecture.svg) / [PNG](./contentcloud-desktop-architecture.png) | [Excalidraw](./contentcloud-desktop-architecture.excalidraw) | target；Electron 尚未实现 |

架构语义以 `.mmd` 与对应 ADR/产品文档为准。修改架构时先改源文件，再用离线 Mermaid 渲染器同时生成 SVG、PNG 和 Excalidraw；没有渲染工具时不得手工伪造图片。

Desktop 的启动时序、同步时序、大文件上传、审批回流、冲突决策和离线状态机维护在：

- [Desktop 架构与技术](../product/content-work-os-desktop/02-architecture-and-technology.md)
- [Desktop 同步、审批与上传](../product/content-work-os-desktop/03-sync-review-upload.md)
- [Desktop 一次性交付计划](../product/content-work-os-desktop/04-delivery-plan.md)
