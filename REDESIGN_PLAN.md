# 页面视觉重构（Editorial Studio 内容生产体系）

> 独立于 `IMPLEMENTATION_PLAN.md`（后端 V2 审批收敛，进行中），本文件只覆盖 `web/src/styles.css` 的视觉层。

基础密度参考：`/Users/coso/Documents/dev/js/loopany-platform/packages/server/src/styles/app.css`

第二轮色彩研究参考 Adobe Spectrum、Sanity UI、Contentful Forma 36、Frame.io、Descript、Craft
与 Airtable。完整设计决策见 `DESIGN.md`。

## 问题诊断

在 `web/src/styles.css`（83KB 压缩单体）实测：

- **无色彩体系**：130+ 种硬编码 hex，几乎全是暖绿调灰（`#6f7774` / `#dfe3e1` / `#f6f8f7`）
- **红色语义错位**：`--accent:#d84b3e` 砖红同时承担品牌色、交互色和装饰性 eyebrow，
  而错误态另用一套红（`#a93d35` / `#fdebea`）——「红」因此不传达任何信息
- **字号失控**：21 个不同字号，其中 9px×79、8px×40、7px×4，正文级文字落在 8-10px
- **圆角失控**：4/5/6/7/8/9/14/16px 共 8 种规格混用

## 目标体系

| 维度 | 现状 | 目标（Editorial Studio） |
| --- | --- | --- |
| 页面底色 | `#f7f8f8` 暖灰 | `#f2f3ef` 稿纸灰 |
| 品牌标记 | 品牌色兼作按钮与状态 | `#d5f34a` 荧光稿纸标记 |
| 高权重命令 | 与品牌色共用 | `#1c1f1b` 石墨黑 |
| 内容分类 | 无稳定分类色 | 资料蓝 / 知识青绿 / 策略琥珀 / 制作珊瑚 / 审核玫红 |
| 错误态 | 散落 5 种红 | `#b8273d` 危险红 |
| 成功态 | `#17855f` | `#117a61` 成功青绿 |
| 警告态 | `#a6650b` | `#a46200` 警告琥珀 |
| 字号 | 7~13px 无级 | 11 / 11.5 / 12 / 13 / 14 五档 |
| 圆角 | 8 种 | 6 / 8 / 12 三档角色层级 |

## Stage 1: 色彩体系
**Goal**: 将品牌、命令、链接、语义状态和内容分类拆分为独立 token，建立编辑制作台配色
**Success Criteria**: 无单一 accent 跨职责复用；分类色与成功/错误等语义色严格分离
**Tests**: `pnpm test` + `pnpm typecheck` + 逐页截图回归
**Status**: Completed (2026-07-26)

## Stage 2: 字号体系
**Goal**: 21 档字号收敛到 5 档 token，最小字号从 7px 提到 11px
**Success Criteria**: 无 <11px 文字；固定宽度容器无溢出/截断
**Tests**: 逐页截图检查溢出，重点查 grid 固定列宽处
**Status**: Completed (2026-07-26)

## Stage 3: 圆角与描边
**Goal**: 圆角收敛到 3 档角色层级；卡片使用 1px 级轻阴影，弹层使用独立高程
**Success Criteria**: 卡片边界统一，视觉层次清晰
**Tests**: 截图回归
**Status**: Completed (2026-07-26)

## Stage 4: 关键组件精修
**Goal**: sidebar / topbar / eyebrow / button / stat 卡片对齐参考项目质感
**Success Criteria**: 全页面截图通过人工审阅
**Tests**: dashboard、项目总览、团队、登录、admin 全覆盖截图
**Status**: Completed (2026-07-26)

## 完成记录

- 色彩已集中到根 token：稿纸灰页面、石墨命令、荧光品牌标记，以及资料、知识、策略、制作、
  审核五组内容分类色；品牌色不再兼任按钮、链接和状态色。
- 正文字号已收敛为 `11 / 11.5 / 12 / 13 / 14px` 五档，样式中无小于 `11px` 的文字。
- 组件圆角按编辑工具密度调整为 `6 / 8 / 12px` 角色层级，卡片使用 1px 级轻阴影，
  弹层使用独立的浮层阴影 token。
- sidebar、生产管线、eyebrow、按钮、统计卡片和登录页已统一到 Editorial Studio；导航与生产阶段
  使用分类色定位，清除了登录页废弃装饰规则，并保留统一 `focus-visible` 与减弱动效支持。
- 移动端精修覆盖后台紧凑租户表和团队成员表，关键字段与操作在 `390px` 视口内完整显示。

## 验收结果

- 人工截图：工作台、项目总览、团队、登录、系统后台，均覆盖 `1440px` 桌面与 `390px` 移动端。
- 浏览器检查：上述关键路由在 `390px` 下均无页面级横向溢出，无 console error 或 page error。
- 自动验证：`pnpm --dir web test`（16 tests）、`pnpm --dir web typecheck`、
  `pnpm --dir web build`、`go test ./...`、`git diff --check` 全部通过。
