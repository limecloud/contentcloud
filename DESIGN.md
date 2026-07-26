# ContentCloud Design System

## Product Context

- **产品**：面向 AI 内容营销团队的本地优先创作与云端治理系统。
- **用户**：内容策略、编辑、审核、项目管理与平台运营人员。
- **界面类型**：高频工作台、内容生产管线、审核界面与独立系统后台。
- **设计目标**：像一张可信、克制但有创作能量的编辑制作台，而不是通用云平台控制台。

## Aesthetic Direction

- **方向**：Editorial Studio（编辑制作台）。
- **装饰强度**：克制且有意图。中性工作面承担大部分内容，颜色只标记品牌、当前动作、内容分类和状态。
- **布局**：严格网格。生产工具优先扫描效率、对齐和稳定尺寸，不采用营销页式卡片堆叠。
- **基调**：石墨、稿纸灰与荧光标记构成主界面；资料、知识、策略、制作和审核拥有稳定分类色。

### Research References

- [Adobe Spectrum color system](https://spectrum.adobe.com/page/color-system/)：用颜色关系与语义角色建立可扩展系统。
- [Sanity UI theme](https://www.sanity.io/ui/docs/theme)：中性表面配合多组 hue token，而非单色覆盖全部功能。
- [Contentful Forma 36 colors](https://f36.contentful.com/tokens/colors/)：内容后台以清晰中性层级和独立交互色为基础。
- [Frame.io](https://frame.io/)：深色制作台、媒体内容优先、颜色稀疏且用于状态与选择。
- [Descript](https://www.descript.com/)：高对比编辑气质与鲜明的制作信号色。
- [Craft](https://www.craft.do/) 与 [Airtable](https://www.airtable.com/)：中性工作面承载多种内容分类色。

## Color

### Brand And Commands

- **Brand marker** `#d5f34a`：荧光稿纸标记，只用于品牌、当前导航和生产管线基线。
- **Brand ink** `#202810`：品牌亮色上的文字与图形。
- **Command** `#1c1f1b`：主要按钮和高权重命令。
- **Command hover** `#30342e`。
- **Link** `#4c6500`：白色和稿纸灰表面上的可访问文本链接。

### Surfaces

- **Page** `#f2f3ef`
- **Panel** `#ffffff`
- **Raised** `#eef0ea`
- **Neutral** `#e7eae3`
- **Ink** `#1b1d1a`
- **Muted** `#5b6258`
- **Border** `#d9dcd3`

### Content Categories

- **Sources** `#3567d8` / `#e9f0ff`
- **Knowledge** `#117a61` / `#e5f5ef`
- **Strategy** `#a46200` / `#fbf0db`
- **Production** `#c74632` / `#fbeae6`
- **Review** `#b33c62` / `#f8e9ef`

分类色表达对象类型和生产阶段，不表达成功或失败。

### Semantic States

- **Info** `#3567d8`
- **Success** `#117a61`
- **Warning** `#a46200`
- **Danger** `#b8273d`

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

- **安全选择**：中性表面、严格网格、可预测导航和高对比命令符合内容后台的高频操作预期。
- **设计风险**：荧光品牌色比传统 SaaS 蓝更醒目，但限定在小面积标记中，避免娱乐化。
- **设计风险**：生产阶段采用分类色，提升定位速度；代价是必须严格区分分类色与语义状态色。

## Decisions Log

| Date | Decision | Rationale |
| --- | --- | --- |
| 2026-07-26 | 从 Rubik 蓝平台主题改为 Editorial Studio | 单一蓝色更像基础设施控制台，无法表达内容生产阶段。 |
| 2026-07-26 | 品牌、命令、链接、语义和分类 token 分离 | 防止一个 accent 同时承担多个互相冲突的职责。 |
| 2026-07-26 | 使用本地系统字体栈 | 保持本地优先、离线可用和零第三方字体请求。 |
