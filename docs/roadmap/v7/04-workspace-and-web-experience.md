# V7 Workspace 与服务端工作面

## 1. 原则

本地 Workspace 继续承担候选创作和受控披露；服务端承担团队可见的规范事实、审核、托管媒体运行和交付。V7 的改进不是把本地整个目录同步到云端，而是让每次显式提交的业务对象都能在 Web 中被理解和操作。

```text
Local Workspace                         Server
---------------                         ------
selected source files   -- publish -->  SourceRevision / Evidence
knowledge candidates    -- publish -->  Knowledge Submission / Review
strategy + brief        -- publish -->  SubmissionRevision
content batch           -- publish -->  Content review / ApprovedSnapshot
storyboard + frames     -- publish -->  Artifact / lock review
                                       |
                                       v
                                  managed video generation
                                       |
final/result cache      <-- pull ----  Artifact / Delivery / Result
```

## 2. Workspace 连接要求

- 一个任务只能绑定一个验证通过的 Workspace 和精确 context revision。
- `.contentcloud/workspace.yaml` 是绑定入口；没有该文件的普通资料目录不能直接 publish。
- 初始化、安装 Plugin、修改环境或 Provider 能力后必须重新运行 doctor，并在新 Agent 会话读取 context。
- Workspace 只上传 publish plan 中列出的相对路径；绝对路径、凭据、Transcript 和未披露正文不能进入服务端对象。
- 本地 candidate、云端 SubmissionRevision、ApprovedSnapshot 和 DeliveryPackage 始终分别表达。

## 3. 金陵古都香导入

来源目录：`/Users/coso/Documents/dev/goodvision/marketing/jinling-gudu`。

该目录当前不是 V3 Workspace，且现有知识和脚本包含 candidate/blocked 状态。V7 提供“受控资料导入”而不是隐式升级：

1. 在 Web 项目页创建 import plan，显示允许文件类型、总字节、目标 Project 和披露说明。
2. 用户在本地选择具体文件，CLI 计算 digest、MIME 和清单，不扫描未选择文件。
3. 用户确认精确 plan 后上传 SourceRevision；服务端保持原文件名、digest 和来源说明。
4. Source Worker 解析 Evidence，低置信或不可解析内容进入 review，不自动失败整个来源。
5. Knowledge Extraction 创建 candidate/blocked KnowledgeObject，并引用 Evidence ID。
6. 人工接受 Evidence 和知识后才能发布 KnowledgePack/Snapshot。
7. 现有 `outputs/scripts/jinling-douyin-20260803/11-drawer-reversal.md` 只能作为 creative candidate；在缺产品、rights、标签安全或素材批准时不可生成正式产品成片。

## 4. 信息架构

### 项目导航

- 概览
- 输入与来源
- 知识库
- 任务
- 内容审核
- 分镜与媒体
- 交付
- 结果
- 设置与能力

### 任务详情

任务详情使用可扫描的标签页，不把所有对象堆在一个长页面：

1. `概览`：目标、SOP、当前 Stage、唯一下一动作、阻断原因。
2. `输入与知识`：Source、Evidence、KnowledgeSnapshot、缺口和冲突。
3. `策略与剧本`：策略、Brief、ContentItem 正文、镜头和 Revision diff。
4. `分镜`：shot 列表、首尾帧、review sheet、锁定状态和 rights。
5. `视频生成`：Job、Provider/Profile、预算、进度、Attempts、takes 和局部重试。
6. `质检与后期`：技术检查、人工评论、take 选择、字幕/音频/CTA 配置和 final render。
7. `交付与血缘`：最终视频播放器、manifest、下载、批准决定和完整 lineage。

标签页使用 URL 子路由，刷新和分享后保持精确上下文。

## 5. 业务内容渲染

### Knowledge

- Source 显示文件名、版本、MIME、字节数、摘要、解析状态和 Evidence 数量。
- Evidence 显示定位、引用文本、OCR 置信度和审核状态。
- KnowledgeObject 显示陈述、类型、层级、引用、rights、冲突、有效期和决定历史。
- 空知识库显示“尚未导入来源”和精确动作，不显示笼统空白页。

### Script

- 显示标题、受众、核心主张、时长、逐镜头画面、旁白、声音、屏幕文案和引用。
- Revision 可切换并显示字段级 diff；正文不能只以 JSON textarea 呈现。
- 未知 Schema 使用安全只读 JSON tree fallback，禁止执行 HTML 或脚本。

### Storyboard

- 使用稳定 9:16 缩略图轨道，显示 first/end frame、shot 时间、动作、连续性和风险。
- 图片加载失败、digest 漂移或 rights 失效时在精确 shot 上阻断。
- review sheet 是人工审核材料，不作为默认 Provider 输入。

### Video

- 视频播放器显示代理文件，不直接暴露对象存储 key。
- take 列表尺寸稳定，显示 segment、时长、分辨率、声音、生成时间和技术检查。
- 只允许选择一个 active take；选择改变需要 expected version，避免双标签页覆盖。
- 下载按钮仅对授权角色和 ready Artifact 可用。

## 6. Stage 操作

工作台不再提供“Stage 输出引用（每行一条）”和固定 `passed: true` 的上报表单。

每个 Stage 的操作由服务端 `allowed_actions` 返回：

- 上传来源
- 运行/继续本地 Agent
- 打开审核
- 批准/退回
- 确认生成费用
- 提交视频 Job
- 取消/重试 Job
- 选择 take
- 运行后期
- 批准最终成片
- 创建交付包

按钮提交后立即禁用并显示确定状态；重复点击由 idempotency key 保证不会重复创建外部 Job。

## 7. 生成进度

```text
排队 -> 提交中 -> Provider 已接收 -> 生成中 -> 下载中 -> 校验中 -> 可审核
```

- 页面通过 SSE 接收任务和 Job 事件，断线后用 cursor 恢复；SSE 失败时退化为带 ETag 的轮询。
- 进度是状态事实，不伪造百分比。Provider 只返回状态时显示阶段，不显示模型猜测的进度。
- 超过预期时长显示“仍在 Provider 生成”和最近确认时间，不直接标成失败。
- 取消是 request 状态；只有 Provider 确认或本地终止完成后才显示 cancelled。

## 8. 错误体验

每个错误至少包含：

- 用户可读说明。
- 稳定 `reason_code`。
- 影响的 Stage/Job/Artifact。
- 是否会产生费用。
- 可执行的下一动作。
- 最近一次尝试和可重试时间。

Provider 原始错误、内部 URL、对象 key、secret ref 和堆栈不显示给普通用户。

## 9. 管理面

租户设置增加“媒体生成能力”：

- enabled/disabled/unavailable/misconfigured。
- 当前 Provider/Profile、验证时间和过期时间。
- 月预算、单 Job 上限、并发和最大重试。
- 数据披露摘要和保留政策。
- 最近健康检查，不显示 Secret 明文。

平台后台管理 allowlisted Provider Adapter、Profile 发布/撤回、区域、模型和证据。租户不能自行填入任意 endpoint。

## 10. 设计要求

- 遵循根目录 `DESIGN.md`；工作台保持冷白、高密度、严格网格和多阶段分类色。
- 来源/知识/策略/生产/审核继续使用既有对象色；Provider 错误使用语义 danger，不新增整页单色主题。
- 使用 Lucide 图标和 tooltip；播放器、镜头轨道和 Job 进度定义稳定尺寸。
- 不使用卡片嵌套；标签页内容为无框架区块，重复 take 和 Artifact 才使用卡片/行。
- 桌面 1440x1000 和移动 390x844 必须无横向溢出、按钮文字截断或内容遮挡。
- 视频、图片和正文都有 loading、empty、error、blocked、stale 和 unauthorized 状态。

## 11. 页面验收

- 任务有 Revision 时正文必然可见；只有数量不可接受。
- 任务有 Delivery 时 manifest 文件列表和最终 Artifact 必然可见；只有 digest 不可接受。
- 知识库为空时用户能直接进入来源导入，不会误以为后台正在自动生成。
- 所有 Gate 页面显示做决定所需正文、媒体和引用。
- 视频 Job 在两标签页同时操作时，较旧页面得到冲突提示并可刷新恢复。
- 320px 最小宽度仍能完成费用确认、take 选择、最终批准和下载。
