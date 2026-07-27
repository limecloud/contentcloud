# ContentCloud Design System

## Product Context

- **产品**：面向 AI 内容营销团队的本地优先创作与云端治理系统。
- **用户**：内容策略、编辑、审核、项目管理与平台运营人员。
- **界面类型**：高频工作台、内容生产管线、审核界面与独立系统后台。
- **设计目标**：像一张明快、可信且有创作能量的内容制作台，而不是通用云平台控制台。

## Aesthetic Direction

- **方向**：TapTap Maker-inspired Content Studio（制造型内容工作台）。
- **装饰强度**：克制且有意图。冷白工作面承担大部分内容，TapTap 青与制造蓝紫只标记品牌、当前选择和创作对象。
- **布局**：严格网格。生产工具优先扫描效率、对齐和稳定尺寸，不采用营销页式卡片堆叠。
- **基调**：冷白、近黑和清透青色构成主界面；蓝紫补充 AI 创作感，资料、知识、策略、制作和审核拥有稳定分类色。

### Research References

- [TapTap 制造](https://maker.taptap.cn/)：白色与冷灰工作面、`#111827` 近黑命令色、`#00c0b7` / `#00d9c5` 青色品牌信号，以及 `#4d4dad` 制造蓝紫。研究同时核验了桌面、移动端 computed styles 与站点 CSS。
- [Adobe Spectrum color system](https://spectrum.adobe.com/page/color-system/)：用颜色关系与语义角色建立可扩展系统。
- [Sanity UI theme](https://www.sanity.io/ui/docs/theme)：中性表面配合多组 hue token，而非单色覆盖全部功能。
- [Contentful Forma 36 colors](https://f36.contentful.com/tokens/colors/)：内容后台以清晰中性层级和独立交互色为基础。
- [Frame.io](https://frame.io/)：深色制作台、媒体内容优先、颜色稀疏且用于状态与选择。
- [Descript](https://www.descript.com/)：高对比编辑气质与鲜明的制作信号色。
- [Craft](https://www.craft.do/) 与 [Airtable](https://www.airtable.com/)：中性工作面承载多种内容分类色。

## Color

### Brand And Commands

- **Brand marker** `#00c0b7`：TapTap 青，用于品牌标记、当前导航与生产管线基线。
- **Brand hover** `#00a99d`；**Brand soft** `#e9fffd`。
- **Brand ink** `#072f2f`：品牌亮色上的文字与图形。
- **Companion** `#4d4dad` / `#eeeeff`：制造蓝紫，用于知识与 AI 创作对象，不承担主命令。
- **Command** `#111827`：主要按钮和高权重命令。
- **Command hover** `#20283a`。
- **Link** `#007b74`：白色和浅灰表面上的可访问文本链接。

### Surfaces

- **Page** `#f5f7f8`
- **Panel** `#ffffff`
- **Raised** `#eef1f4`
- **Neutral** `#e6eaee`
- **Ink** `#111827`
- **Muted** `#657082`
- **Border** `#e2e6ea`

### Content Categories

- **Sources** `#007f78` / `#e9fffd`
- **Knowledge** `#4d4dad` / `#eeeeff`
- **Strategy** `#2563eb` / `#eff6ff`
- **Production** `#c2410c` / `#fff3e8`
- **Review** `#d92d4e` / `#fff1f2`

分类色表达对象类型和生产阶段，不表达成功或失败。

### Semantic States

- **Info** `#2563eb`
- **Success** `#0b7f53`
- **Warning** `#b45309`
- **Danger** `#d92d4e`

语义色只表达系统状态，不承担品牌装饰。

## Typography

- **UI / Body**：平台原生 humanist sans 栈，优先 Avenir Next、Segoe UI Variable、苹方与微软雅黑。
- **Data / Code**：系统等宽字体栈；数字列启用 tabular nums。
- **策略**：不加载远程字体，保证本地优先环境、离线开发和隐私边界。
- **正文尺度**：`11 / 11.5 / 12 / 13 / 14px`，页面标题按容器使用 `21-27px`。
- **字距**：统一为 `0`。

## Spacing And Layout

- **基础单位**：4px。
- **密度**：紧凑但可扫描；表格和管线保持固定节奏，表单保留 8-16px 组间距。
- **内容宽度**：工作台最大 1440px，页面使用响应式网格。
- **圆角**：小元素 6px、控件与卡片 8px、弹层 12px、状态 pill 使用完全圆角。
- **阴影**：卡片仅使用 1px 级轻阴影，弹层使用独立高程；边框负责主要分层。

## Motion

- **方向**：minimal-functional。
- **时长**：微交互 150-200ms。
- **属性**：仅过渡颜色、透明度和 transform，不使用 `transition: all`。
- **无障碍**：必须尊重 `prefers-reduced-motion`，所有键盘交互保留 `focus-visible`。

## Safe Choices And Risks

- **安全选择**：冷白表面、严格网格、可预测导航和近黑命令符合内容后台的高频操作预期。
- **设计风险**：TapTap 青会带来更轻快的创作者气质，因此只用于品牌与选择态，避免把全部后台染成青色。
- **设计风险**：制造蓝紫与五段分类色提升对象定位速度；代价是必须严格区分分类色、品牌色与语义状态色。

## Decisions Log

| Date | Decision | Rationale |
| --- | --- | --- |
| 2026-07-26 | 从 Rubik 蓝平台主题改为 Editorial Studio | 单一蓝色更像基础设施控制台，无法表达内容生产阶段。 |
| 2026-07-26 | 品牌、命令、链接、语义和分类 token 分离 | 防止一个 accent 同时承担多个互相冲突的职责。 |
| 2026-07-26 | 使用本地系统字体栈 | 保持本地优先、离线可用和零第三方字体请求。 |
| 2026-07-27 | 采用 TapTap 制造的冷白、青色与蓝紫配色关系 | 让工作台更接近创作者工具，同时保留近黑命令和多阶段生产语义。 |
