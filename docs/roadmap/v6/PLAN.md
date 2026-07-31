# ContentCloud V6 实施台账

状态：`需求草案、官网原型与 React 官网 Beta 已完成，待 M6-0 和发布评审`。

更新时间：2026-07-31。

本文件是 V6 的进度台账。V6 只承接已在 V3/V4/V5 验证的事实和边界，不因产品更名而改变服务端 zero-exec、本地优先、显式 publish/approve、外部平台人工操作和租户隔离原则。

## 1. 工作包

| ID | 工作包 | 产物 | 状态 |
| --- | --- | --- | --- |
| W6-00 | 产品定位和词汇 | Content Work OS 名称、对象词典、边界说明 | 已完成草案 |
| W6-01 | Control Plane 状态投影 | Runtime/Workspace/Pack/Manifest/Capability 统一投影 | 待实施 |
| W6-02 | Tenant Capability Matrix | 四态能力、审计、reason_code、租户后台 | 待实施 |
| W6-03 | Work OS 首页 | 待审、运行、交付、阻断项和下一步动作 | 待实施 |
| W6-04 | Content Pack 统一入口 | 视频/公众号路由、Pack 元数据和状态门禁 | 部分已有，待收敛 |
| W6-05 | Runtime 诊断 | Daemon、设备、版本、租约、进度和 dead-letter | 部分已有，待 Web 化 |
| W6-06 | 官网 Beta | 公共官网、连接入口、Registry 驱动状态和文档导流 | React Beta 已实现，待发布验收 |
| W6-07 | V5 兼容收敛 | Bootstrap、Plugin、Registry/Profile、兼容入口 | 阻断项待解决 |
| W6-08 | 真实设备与租户验收 | macOS/Windows、视频/公众号、权限和断网恢复 | 待实施 |
| W6-09 | 发布治理 | 官网静态检查、文档同步、版本和回滚预案 | 待实施 |

## 2. 里程碑

| 里程碑 | 目标 | 状态 |
| --- | --- | --- |
| M6-0 | 产品、工程、安全和内容运营确认定位与边界 | 待评审 |
| M6-1 | 控制面状态可读、可诊断、可审计 | 待开始 |
| M6-2 | Work OS 首页和统一 Content Pack 导航上线 | 待开始 |
| M6-3 | 官网 Beta 与文档/Connect 入口上线 | 实现完成，待发布评审 |
| M6-4 | V5 兼容项和真实设备问题收敛 | 待开始 |
| M6-5 | Content Work OS 生产发布 | 待开始 |

## 3. 当前检查记录

| 检查 | 结果 | 说明 |
| --- | --- | --- |
| `go test ./...` | 通过 | Go 全量包测试通过 |
| `go test -race ./...` | 通过 | Go 全量竞态检测通过 |
| `go vet ./...` | 通过 | Go 静态检查通过 |
| `pnpm --dir web test -- --run` | 通过 | 14 个测试文件，64 个测试通过 |
| `pnpm --dir web build` | 通过 | Vite production build 通过；官网路由独立懒加载 |
| 现有 Content Pack 文档 | 已核对 | 视频可用；公众号可用但需租户开通 |
| 官网原型 | 已完成 | `prototype.html`，无第三方脚本，可独立打开 |
| React 官网 Beta | 已完成 | `/` 公共官网，`/workspace` 登录后工作台，目录不可用时显示诊断状态 |
| 响应式视觉检查 | 通过 | 1440×1000、1280×720、768×1024、390×844 无横向溢出，首屏均露出产品段标题 |

## 4. 决策记录

| 日期 | 决策 | 原因 |
| --- | --- | --- |
| 2026-07-31 | V6 升级为 Content Work OS | 用统一工作面表达本地 Agent、云端治理、Pack 和交付关系 |
| 2026-07-31 | 官网与工作台分离设计 | 官网建立定位和信任，工作台保持高密度生产效率 |
| 2026-07-31 | 先做状态可见，再扩展内容形态 | 没有 Runtime/Capability/Manifest 可见性时，新增 Pack 只会增加排障成本 |
