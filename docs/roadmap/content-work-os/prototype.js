(() => {
  'use strict';

  const $ = (selector, root = document) => root.querySelector(selector);
  const $$ = (selector, root = document) => [...root.querySelectorAll(selector)];

  const templates = [
    {
      id: 'research', name: '资料与知识建设', type: '知识建设', description: '从需求和来源登记到可复用知识快照，适合先补齐事实再进入生产的内容任务。', tags: [['知识治理', 'blue'], ['Evidence', 'success']],
      steps: [['需求 Brief', '目标、受众和边界'], ['资料与来源', '登记输入和定位'], ['知识整理', 'Evidence 和事实'], ['情报摘要', '竞争和市场线索'], ['策略输入', '可执行的内容方向']]
    },
    {
      id: 'video', name: '短视频生产', type: '视频脚本', description: '覆盖 Brief、知识、策略、脚本、检查和交付，审批按风险配置。', tags: [['Production', 'production'], ['Gate 可选', 'review']],
      steps: [['需求 Brief', '目标、渠道和验收'], ['知识与证据', '可用事实和引用'], ['受众与策略', '角度、钩子和假设'], ['脚本创作', 'ContentBatch / Script'], ['品牌与权利检查', 'Schema、Claim、Rights'], ['Accepted 与交付', '快照和交付包']]
    },
    {
      id: 'article', name: '文章协作', type: '文章', description: '适合编辑、本地 CLI 和业务负责人协作完成有引用的文章交付。', tags: [['多人协作', 'blue'], ['引用检查', 'success']],
      steps: [['Article Brief', '主题、受众和结构'], ['知识引用', '事实、来源和权利'], ['文章写作', '段落、标题和 CTA'], ['检查与交付', '可选 Gate 和交付']]
    },
    {
      id: 'retro', name: '活动结果复盘', type: '复盘', description: '把投放结果、内容版本和下一轮改进串成可复用的复盘任务。', tags: [['Learning', 'blue'], ['指标', 'success']],
      steps: [['结果导入', '平台观察和数据质量'], ['版本绑定', '关联 AcceptedSnapshot'], ['问题归因', '事实和可观测范围'], ['改进建议', '下一轮假设'], ['复盘交接', '沉淀到 SOP 或 Task']]
    },
    {
      id: 'blank', name: '空白 SOP', type: '自定义', description: '从一个空白 Stage 开始，按企业自己的内容方法论设计流程。', tags: [['自定义', '']],
      steps: [['新 Stage', '配置输入、输出和 Gate']]
    }
  ];

  const tasks = [
    { id: 1, title: '生成 10 条新品短视频脚本', meta: '视频脚本 · 2 个输入快照', project: '新品内容生产', sop: '短视频生产 · v1.0', stage: '脚本创作', status: '待补资料', tone: 'warning', owner: '林舟', executor: '本地 Workspace', updated: '8 分钟前' },
    { id: 2, title: '整理新品参数和引用证据', meta: '资料与知识 · 4 个来源', project: '新品内容生产', sop: '资料与知识建设 · v1.2', stage: '知识与证据', status: '执行中', tone: 'blue', owner: '林舟', executor: 'Claude Code CLI', updated: '24 分钟前' },
    { id: 3, title: '完成新品公众号首稿', meta: '文章 · 需引用当前知识', project: '新品内容生产', sop: '文章协作 · v1.0', stage: '文章写作', status: '待 Gate', tone: 'review', owner: '周宁', executor: 'Codex CLI', updated: '1 小时前' },
    { id: 4, title: '导入上周内容结果', meta: '复盘 · 3 条有效观察', project: '新品内容生产', sop: '活动结果复盘 · v1.0', stage: '结果导入', status: '已交付', tone: 'success', owner: '林舟', executor: '本地 Workspace', updated: '昨天' },
    { id: 5, title: '设计夏季活动内容方向', meta: '策略 · 目标和受众待确认', project: '新品内容生产', sop: '短视频生产 · v1.0', stage: '受众与策略', status: '待处理', tone: 'muted', owner: '陈璐', executor: 'Claude Code CLI', updated: '昨天' },
    { id: 6, title: '更新品牌禁用词知识', meta: '知识 · 12 条规则', project: '品牌知识建设', sop: '资料与知识建设 · v1.2', stage: '知识整理', status: '执行中', tone: 'blue', owner: '林舟', executor: 'Claude Code CLI', updated: '2 天前' },
    { id: 7, title: '确认达人素材使用权', meta: 'Rights · 6 个素材', project: '新品内容生产', sop: '短视频生产 · v1.0', stage: '品牌与权利检查', status: '待 Gate', tone: 'review', owner: '周宁', executor: '本地检查器', updated: '2 天前' },
    { id: 8, title: '交付夏季主视觉文案', meta: '文案 · AcceptedSnapshot #28', project: '新品内容生产', sop: '文章协作 · v1.0', stage: 'Accepted 与交付', status: '已交付', tone: 'success', owner: '陈璐', executor: '本地 Workspace', updated: '3 天前' }
  ];

  const inboxItems = [
    { id: 1, title: '客户希望增加“轻量便携”角度', source: '聊天消息', detail: '来自新品项目讨论，尚未判断是否改变当前脚本 Brief。', next: '建议：补充到任务 #1 的输入说明', icon: 'message-square-text', tone: 'blue', status: 'open' },
    { id: 2, title: '新品规格表 v3.xlsx', source: '本地 Workspace 文件', detail: '检测到参数更新，可补充到现有知识任务或新建任务。', next: '建议：更新知识任务的输入快照', icon: 'file-spreadsheet', tone: 'success', status: 'open' },
    { id: 3, title: '3 条竞品短视频链接', source: '采集结果', detail: '来源和抓取时间已登记，等待确认研究范围。', next: '建议：创建一次资料与知识建设任务', icon: 'link', tone: 'warning', status: 'open' }
  ];

  const clientAdapters = [
    {
      id: 'codex-cli', name: 'Codex CLI', version: '0.61.0', status: 'connected', statusLabel: '已连接',
      format: 'Codex session JSONL', icon: 'terminal-square', description: '由 Codex CLI 在本地选择会话、轮次和导出范围。',
      supports: { summary: true, selectedTurns: true, fullTranscript: true }, redactionHandledLocally: true
    },
    {
      id: 'claude-code', name: 'Claude Code CLI', version: '1.0.82', status: 'connected', statusLabel: '已连接',
      format: 'Claude Code transcript JSONL', icon: 'terminal-square', description: '由 Claude Code CLI 在本地解析 transcript 并生成导出包。',
      supports: { summary: true, selectedTurns: true, fullTranscript: true }, redactionHandledLocally: true
    },
    {
      id: 'workspace-bundle', name: '本地 Workspace Bundle', version: 'bundle/1.0', status: 'ready', statusLabel: '已安装',
      format: 'ConversationBundle JSON', icon: 'package-open', description: '适合没有专用 CLI 适配器的本地脚本或编辑器插件。',
      supports: { summary: true, selectedTurns: true, fullTranscript: false }, redactionHandledLocally: true
    }
  ];

  const knowledgeObjects = [
    { id: 'product:new-product', title: '新品产品聚合页', type: 'Entity', category: '产品实体', layer: 'product', status: 'active', statusLabel: '有效', tone: 'success', usable: true, summary: '聚合产品身份、规格断言、场景、素材、主张、生产与质检关系。', source: '6 个登记来源', evidence: '18 个关系', project: '新品内容生产', owner: '周宁', updated: '昨天', version: 'v1.3', relations: '规格 6 · 场景 4 · 素材 8', usedBy: '3 个 TaskRun', approvalStatus: 'active', approvalLabel: '保持有效' },
    { id: 'assertion:base-parameters', title: '当前基础参数来自已签批规格', type: 'FactAssertion', category: '事实断言', layer: 'product', status: 'verified', statusLabel: '已验证', tone: 'success', usable: true, summary: '当前可用于内容的产品参数集合，值和适用批次均绑定精确来源定位。', source: '新品规格表 v2.xlsx#Sheet1!B3:B12', evidence: '4 条已验证 Evidence', project: '新品内容生产', owner: '周宁', updated: '昨天', version: 'v1.3', relations: '产品实体 · 2 条 Claim', usedBy: '知识快照 #42', approvalStatus: 'verified', approvalLabel: '保持已验证' },
    { id: 'rule:brand-language', title: '品牌禁用词与替代表达', type: 'BrandRule', category: '品牌规则', layer: 'identity', status: 'active', statusLabel: '有效', tone: 'success', usable: true, summary: '对外内容不可使用的表达、推荐替代项和适用渠道。', source: '品牌语言规范 2026#page=4-9', evidence: '12 条确定性规则', project: '品牌知识建设', owner: '林舟', updated: '2 天前', version: 'v2.1', relations: '5 个 Intent · 2 个 Gate', usedBy: '18 个 TaskRun', approvalStatus: 'active', approvalLabel: '保持有效' },
    { id: 'constraint:short-video-expression', title: '短视频渠道表达约束', type: 'ConstraintRecord', category: '渠道约束', layer: 'content_engine', status: 'active', statusLabel: '有效', tone: 'success', usable: true, summary: '短视频首屏信息、Claim 边界和 CTA 使用规则。', source: '渠道规范与历史 Decision', evidence: '7 条 Evidence', project: '新品内容生产', owner: '陈璐', updated: '3 天前', version: 'v1.0', relations: '短视频 Intent · SOP Stage 4', usedBy: '9 个 TaskRun', approvalStatus: 'active', approvalLabel: '保持有效' },
    { id: 'rights:product-assets', title: '当前产品素材使用权', type: 'RightsRecord', category: '权利记录', layer: 'compliance', status: 'valid', statusLabel: '权利有效', tone: 'success', usable: true, summary: '记录 8 项产品素材的使用范围、渠道和有效期，2 项即将到期。', source: '素材授权清单 2026', evidence: '8 个 Asset', project: '新品内容生产', owner: '周宁', updated: '昨天', version: 'v1.1', relations: '8 个 Asset · 5 个 Delivery', usedBy: '3 个 TaskRun', approvalStatus: 'valid', approvalLabel: '保持有效' },
    { id: 'claim:light-portable', title: '“轻量便携”卖点适用范围', type: 'Claim', category: '外部主张', layer: 'expression', status: 'needs_review', statusLabel: '待批准', tone: 'warning', usable: false, summary: '来自客户补充意见，需要与新版规格和可公开口径交叉确认。', source: 'ConversationImport ci_24731018', evidence: '1 项支持 · 1 项待补', project: '新品内容生产', owner: '林舟', updated: '8 分钟前', version: '候选 #18', relations: '新品产品 · 2 个渠道', usedBy: '未进入快照', approvalStatus: 'approved', approvalLabel: '批准主张' },
    { id: 'assertion:spec-v3-delta', title: '新品规格表 v3 变化项', type: 'FactAssertion', category: '事实断言', layer: 'product', status: 'needs_review', statusLabel: '待验证', tone: 'blue', usable: false, summary: '检测到 6 项参数变化，需确认生效批次和对外使用范围。', source: '新品规格表 v3.xlsx#Sheet1!B3:B12', evidence: '6 条候选 Evidence', project: '新品内容生产', owner: 'Claude Code CLI', updated: '24 分钟前', version: '候选 #17', relations: '可能替代 assertion:base-parameters', usedBy: '未进入快照', approvalStatus: 'verified', approvalLabel: '验证事实' },
    { id: 'insight:competitor-structure', title: '公开竞品表达结构观察', type: 'Insight', category: '研究洞察', layer: 'market', status: 'candidate', statusLabel: '候选', tone: 'blue', usable: false, summary: '只保留公开表达结构观察，不包含原文复用；来源范围尚待负责人确认。', source: '3 条公开链接', evidence: '定位范围待确认', project: '新品内容生产', owner: 'Codex CLI', updated: '1 小时前', version: '候选 #16', relations: '策略任务 #5', usedBy: '仅研究输入', approvalStatus: 'approved', approvalLabel: '接受洞察' },
    { id: 'conflict:spec-version', title: '规格表 v2 与 v3 生效批次冲突', type: 'ConflictRecord', category: '冲突记录', layer: 'product', status: 'open', statusLabel: '未解决', tone: 'review', usable: false, summary: '同一参数存在两个候选值，缺当前签批批次；解决前相关对外表述被阻断。', source: '规格表 v2 / v3', evidence: '2 组相互冲突 Evidence', project: '新品内容生产', owner: '流程负责人', updated: '24 分钟前', version: '冲突 #4', relations: '2 条 Assertion · 3 个 Task', usedBy: '阻断 1 个 Stage', approvalStatus: 'resolved', approvalLabel: '记录解决方案' },
    { id: 'gap:public-claim-scope', title: '缺少“轻量便携”的公开口径依据', type: 'KnowledgeGap', category: '知识缺口', layer: 'expression', status: 'source_missing', statusLabel: '缺少来源', tone: 'warning', usable: false, summary: '需要产品负责人提供当前卖点口径或可比较的规格依据。', source: '尚未提供', evidence: '0 条', project: '新品内容生产', owner: '产品负责人', updated: '8 分钟前', version: '缺口 #9', relations: 'claim:light-portable', usedBy: '阻断 Claim 批准', approvalStatus: 'candidate', approvalLabel: '已补齐来源' },
    { id: 'claim:absolute-lightest', title: '“同类产品中最轻”', type: 'Claim', category: '高风险主张', layer: 'compliance', status: 'prohibited', statusLabel: '禁止使用', tone: 'review', usable: false, summary: '绝对化比较缺少完整样本和可持续证明，默认不得进入营销内容。', source: 'Claim Policy', evidence: '规则命中：绝对化比较', project: '品牌知识建设', owner: '合规规则', updated: '2 天前', version: 'policy/1.0', relations: '3 个渠道 Gate', usedBy: '阻断 0 次', approvalStatus: 'prohibited', approvalLabel: '保持禁止' }
  ];

  const knowledgeSources = [
    { id: 'source:spec-v3', title: '新品规格表 v3.xlsx', type: 'Workspace 文件', detail: '6 条候选 Evidence · 2 个受影响断言', locator: 'Sheet1!B3:B12', digest: 'sha256:7bd4…a319', objects: 6, status: '已登记', tone: 'blue', updated: '24 分钟前' },
    { id: 'source:brand-language', title: '品牌语言规范 2026', type: '受控文档', detail: '12 条规则 · 5 个 Intent 引用', locator: 'page=4-9', digest: 'sha256:028e…931a', objects: 12, status: '可引用', tone: 'success', updated: '2 天前' },
    { id: 'source:conversation-import', title: '客户补充意见', type: 'ConversationImport', detail: '选择性片段 · 已脱敏 · 不含完整 Transcript', locator: 'bundle:block-2', digest: 'sha256:319c…fa20', objects: 1, status: '候选输入', tone: 'warning', updated: '8 分钟前' },
    { id: 'source:competitor-links', title: '3 条公开竞品链接', type: '外部来源', detail: '范围待确认 · 不复用原文', locator: 'URL + captured_at', digest: 'sha256:c19b…e531', objects: 1, status: '待确认', tone: 'review', updated: '1 小时前' },
    { id: 'source:rights-list', title: '素材授权清单 2026', type: '权利文档', detail: '8 个素材 · 2 项即将到期', locator: 'rows 2-9', digest: 'sha256:6a31…027f', objects: 8, status: '可引用', tone: 'success', updated: '昨天' },
    { id: 'source:channel-policy', title: '短视频渠道规范', type: '规则文档', detail: '7 条约束 · 2 个 Gate', locator: 'sections 1-4', digest: 'sha256:1f81…c2d0', objects: 7, status: '可引用', tone: 'success', updated: '3 天前' }
  ];

  const knowledgeLayers = [
    { id: 'identity', name: '身份与品牌', icon: 'badge-check', coverage: 88, objects: 24, gaps: 1, detail: '品牌、主体、视觉与语言规则' },
    { id: 'product', name: '产品与规格', icon: 'package-search', coverage: 72, objects: 31, gaps: 4, detail: '产品、参数、材质、生产与质检' },
    { id: 'market', name: '市场与受众', icon: 'users-round', coverage: 61, objects: 18, gaps: 5, detail: '受众、场景、痛点与竞争观察' },
    { id: 'expression', name: '表达与主张', icon: 'message-square-quote', coverage: 54, objects: 16, gaps: 6, detail: 'Claim、话术、风险与渠道边界' },
    { id: 'operations', name: '运营与渠道', icon: 'workflow', coverage: 69, objects: 22, gaps: 3, detail: '渠道、交付、生产与反馈闭环' },
    { id: 'content_engine', name: '内容引擎', icon: 'blocks', coverage: 83, objects: 12, gaps: 2, detail: 'Intent、模板、SOP 与输出约束' },
    { id: 'compliance', name: '合规与权利', icon: 'shield-check', coverage: 66, objects: 19, gaps: 4, detail: '权利、禁用项、证据与发布规则' }
  ];

  const knowledgePacks = [
    { id: 'pack:new-product-v4', name: '新品品牌与产品知识包', version: 'v4.0', status: 'published', statusLabel: '已发布', tone: 'success', snapshot: '#42', layers: '7/7 层', objects: 142, tasks: 3, updated: '昨天 18:40' },
    { id: 'pack:brand-expression-v2', name: '品牌表达治理包', version: 'v2.1', status: 'published', statusLabel: '已发布', tone: 'success', snapshot: '#18', layers: '3/7 层', objects: 47, tasks: 18, updated: '2 天前' },
    { id: 'pack:summer-campaign-draft', name: '夏季活动知识包', version: '草稿', status: 'draft', statusLabel: '草稿', tone: 'warning', snapshot: '未生成', layers: '4/7 层', objects: 28, tasks: 0, updated: '1 小时前' }
  ];

  const resources = {
    automation: [
      { id: 'auto-brief', name: 'Brief 完整性 Hook', detail: '本地创建任务时检查目标、受众、渠道和验收条件。', meta: '触发：Task 创建', enabled: true },
      { id: 'auto-rights', name: '素材权利到期提醒', detail: '本地按素材 Rights 有效期生成提醒，不自动阻断任务。', meta: '触发：每天 09:00', enabled: true },
      { id: 'auto-delivery', name: '交付包组装规则', detail: 'AcceptedSnapshot 形成后在 Workspace 组装交付物和治理摘要。', meta: '触发：Accepted', enabled: false }
    ],
    agents: [
      { id: 'agent-research', name: 'Claude Code CLI · 研究配置', detail: '在本地 Workspace 中整理 Evidence 和知识候选，不直接写入已接受知识。', meta: '最近运行：24 分钟前', enabled: true },
      { id: 'agent-script', name: 'Codex CLI · 脚本配置', detail: '依据 Brief、知识快照和策略生成带引用的脚本批次。', meta: '最近运行：8 分钟前', enabled: true },
      { id: 'agent-quality', name: '本地检查器', detail: '在 Workspace 内执行 Schema、Claim、品牌规则和 Rights 确定性检查。', meta: '最近运行：1 小时前', enabled: true }
    ],
    capabilities: [
      { id: 'cap-video', name: '短视频脚本', detail: '默认内容生产能力，支持批次、Revision 和交付。', meta: '默认启用', enabled: true },
      { id: 'cap-article', name: '公众号文章', detail: '按环境启用的文章生产能力，绑定知识引用检查。', meta: '环境能力', enabled: true },
      { id: 'cap-external', name: '外部发布连接器', detail: '仅生成发布候选，实际发布需要显式授权。', meta: '高风险能力', enabled: false }
    ]
  };

  const projectConfig = {
    name: '新品内容生产',
    environment: '南京澄观内容科技',
    sop: '短视频生产',
    sopVersion: 'v1.0',
    owner: '林舟',
    risk: '外部营销',
    deliveryProfile: 'Workspace 交付包'
  };

  const sopRegistry = [
    { id: 'knowledge', name: '资料与知识建设', version: 'v1.2', status: 'published', statusLabel: '已发布', contentType: '知识建设', tasks: 9, builtin: true, default: false },
    { id: 'video', name: '短视频生产', version: 'v1.0', status: 'published', statusLabel: '已发布', contentType: '视频脚本', tasks: 18, builtin: true, default: true },
    { id: 'article', name: '文章协作', version: 'v1.0', status: 'published', statusLabel: '已发布', contentType: '文章', tasks: 4, builtin: true, default: false },
    { id: 'retro', name: '活动结果复盘', version: 'v1.0', status: 'published', statusLabel: '已发布', contentType: '复盘', tasks: 5, builtin: true, default: false }
  ];

  const gatePolicies = [
    { id: 'rights', name: '外部营销内容权利检查', detail: '确定性检查失败时阻断，负责人：流程负责人。', enabled: true },
    { id: 'claim-review', name: '高风险 Claim 内部确认', detail: '仅当规则命中高风险主张时创建人工决定。', enabled: true },
    { id: 'blanket-approval', name: '所有内容统一审批', detail: '不作为环境默认，可由特定 SOP 显式启用。', enabled: false }
  ];

  const tenantRoles = [
    { id: 'process-owner', name: '流程负责人', members: 2, permissions: '发布 SOP、接受 Revision、处理 Gate', scope: 'Environment' },
    { id: 'editor', name: '内容编辑', members: 5, permissions: '创建 Task、提交 Revision', scope: 'Project' },
    { id: 'client-approver', name: '客户决定人', members: 3, permissions: '处理被指派的客户决定', scope: 'Assigned Task' },
    { id: 'auditor', name: '审计员', members: 1, permissions: '只读事实与审计事件', scope: 'Environment' }
  ];

  const workspaceNodes = [
    { id: 'node-macbook', name: 'MacBook Pro · coso', type: 'Workspace', heartbeat: '2 分钟前', status: 'online', statusLabel: '在线', slots: 3 },
    { id: 'node-codex', name: 'Codex CLI', type: 'CLI 配置', heartbeat: '8 分钟前', status: 'ready', statusLabel: '可用', slots: 1 },
    { id: 'node-claude', name: 'Claude Code CLI', type: 'CLI 配置', heartbeat: '24 分钟前', status: 'ready', statusLabel: '可用', slots: 1 }
  ];

  const auditEvents = [
    { id: 'evt-106', time: '11:42:18', actor: '陈璐', actorId: 'user_04', category: 'delivery', action: 'delivery.created', object: 'Delivery #28', result: '成功', tone: 'success', reason: '从已接受 Revision #46 生成交付包' },
    { id: 'evt-105', time: '11:41:55', actor: 'Content Runtime', actorId: 'service', category: 'revision', action: 'snapshot.accepted', object: 'Revision #46', result: '成功', tone: 'success', reason: 'Schema、Claim 引用和 Rights 检查均通过' },
    { id: 'evt-104', time: '10:18:03', actor: '周宁', actorId: 'user_02', category: 'decision', action: 'gate.decision', object: 'Task #3', result: '要求修改', tone: 'blue', reason: '文章需要补充规格来源定位' },
    { id: 'evt-103', time: '09:02:31', actor: 'Rights Checker', actorId: 'automation', category: 'rights', action: 'rights.blocked', object: 'Asset #103', result: '已阻断', tone: 'review', reason: '素材授权范围不包含当前渠道' },
    { id: 'evt-102', time: '昨天 18:22', actor: '林舟', actorId: 'user_01', category: 'sop', action: 'sop.published', object: '资料与知识建设 v1.2', result: '成功', tone: 'success', reason: '样例运行通过并完成影响分析' }
  ];

  const contextScopes = [
    { id: 'project', name: '新品内容生产', short: '新', description: '项目范围的业务记录' },
    { id: 'research', name: '内容研究协作', short: '研', description: '研究输入与决定' },
    { id: 'delivery', name: '本周交付', short: '交', description: '交付事实和客户反馈' }
  ];

  const contextRecords = {
    project: [
      { kind: '输入补充', title: '规格表已更新', detail: '变化项已进入输入收集，尚未改变当前 Task 的输入快照。', author: '周宁', time: '10:14', tone: 'blue', icon: 'file-input' },
      { kind: '运行摘要', title: '识别 4 个可引用来源', detail: '1 项需要人工确认；原始 CLI Transcript 仍保留在本地。', author: 'Claude Code CLI', time: '10:09', tone: 'success', icon: 'terminal-square' },
      { kind: '业务决定', title: '保持脚本任务等待', detail: '产品参数确认后再恢复运行，不创建临时 Revision。', author: '林舟', time: '10:18', tone: 'review', icon: 'gavel' }
    ],
    research: [
      { kind: '运行摘要', title: '竞品链接已登记来源', detail: '等待确定研究问题和可用范围，未抓取受限正文。', author: 'Codex CLI', time: '昨天 16:42', tone: 'success', icon: 'terminal-square' },
      { kind: '业务边界', title: '只提炼公开卖点和表达结构', detail: '不复用原文，不把未授权内容沉淀为 Project Knowledge。', author: '林舟', time: '昨天 16:48', tone: 'review', icon: 'shield-check' }
    ],
    delivery: [
      { kind: '交付事实', title: '本周 5 个交付包已生成', detail: '均已绑定有效 AcceptedSnapshot。', author: '陈璐', time: '昨天 18:10', tone: 'production', icon: 'package-check' },
      { kind: '业务决定', title: '客户修改意见进入 Decision', detail: '不覆盖已接受 Revision，后续修改通过新 TaskRun 完成。', author: '林舟', time: '昨天 18:22', tone: 'review', icon: 'gavel' }
    ]
  };

  const pageMeta = {
    workspace: ['工作区', '工作台', '今天需要你推进的内容工作、运行状态和业务结果。'],
    inbox: ['工作区', '输入收集', '先收集本地文件、聊天消息和采集结果，再决定转为任务、补充证据或归档。'],
    knowledge: ['知识治理', '知识库', '管理知识候选、已接受知识、来源与证据，并为 Task 提供可追溯知识快照。'],
    'my-tasks': ['任务中心', '我的任务', '只显示当前由你负责或等待你决定的任务。'],
    'all-tasks': ['任务中心', '所有任务', '跨 Project 查看任务、负责人和当前 Stage。'],
    chat: ['协作', '任务上下文', '只记录明确的业务事项、运行摘要和交付事实；Codex、Claude Code 等对话仍留在本地 Workspace。'],
    automation: ['本地执行', '本地规则', '管理 Workspace Hook、计划任务和本地批处理，运行仍写入 TaskRun。'],
    agents: ['本地执行', 'CLI 执行器', '管理 Codex、Claude Code 等本地 CLI 配置，以及它们可使用的能力边界。'],
    swarm: ['本地执行', '工作区节点', '查看本地 Workspace、并行会话和连接状态，不把远程 Agent 当作业务对象。'],
    usage: ['运行治理', '用量', '查看能力调用、运行耗时、失败和成本分布。'],
    tasks: ['Project Tasks', '任务', '围绕当前 Project 查看任务、下一动作和运行状态。'],
    sop: ['SOP 设计', 'SOP 设计', '为当前 Project 选择并调整可复用方法论，Gate 按业务风险配置。'],
    admin: ['Environment Admin', '管理后台', '集中管理环境级 SOP、Gate、能力、本地执行器和角色权限。'],
    audit: ['Governance', '审计', '查询影响事实、决策、版本、权利和交付的完整事件。']
  };

  const state = {
    route: 'workspace',
    selectedTemplate: 'video',
    gateOn: true,
    gateMode: 'required_check',
    gateRole: '流程负责人',
    sopNote: '',
    taskFilter: '全部',
    inboxFilter: 'all',
    knowledgeTab: 'overview',
    knowledgeType: 'all',
    knowledgeLayer: 'all',
    knowledgeSearch: '',
    knowledgeQuery: '',
    knowledgeQueryRan: false,
    selectedKnowledgeId: null,
    selectedSourceId: null,
    activeContextScope: 'project',
    adminTab: 'environment',
    usageRange: '7d',
    pendingInboxId: null,
    savedSops: 1,
    importAdapterId: 'claude-code',
    importPurpose: 'handoff',
    importScope: 'selected',
    importRequestId: null,
    importRequested: false,
    taskCenterFilter: '全部',
    auditFilter: 'all',
    auditSearch: '',
    operationKind: null,
    operationContext: null,
    inputLastRefresh: '尚未刷新',
    legacyUpgradeStatus: 'available',
    lastExportName: ''
  };

  const escapeHtml = (value) => String(value).replace(/[&<>'"]/g, (character) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' })[character]);

  function iconRefresh() {
    if (window.lucide) window.lucide.createIcons({ attrs: { 'aria-hidden': 'true' } });
  }

  function showToast(message) {
    const toast = $('#toast');
    $('span', toast).textContent = message;
    toast.classList.add('is-visible');
    window.clearTimeout(showToast.timer);
    showToast.timer = window.setTimeout(() => toast.classList.remove('is-visible'), 2400);
  }

  function addAudit(action, object, result, tone = 'success', reason = '原型内操作') {
    auditEvents.unshift({
      id: `evt-${Date.now()}`,
      time: '刚刚',
      actor: '林舟',
      actorId: 'user_01',
      category: action.split('.')[0],
      action,
      object,
      result,
      tone,
      reason
    });
  }

  function setHeader(route) {
    const [eyebrow, title, description] = pageMeta[route];
    $('#page-eyebrow').textContent = eyebrow;
    $('#page-title').innerHTML = route === 'sop' ? `${title} <span class="count-badge">${state.savedSops}</span>` : escapeHtml(title);
    $('#page-description').textContent = description;
    $('.breadcrumb').innerHTML = route === 'tasks' || route === 'sop'
      ? '<span>Project</span><i data-lucide="chevron-right" class="icon"></i><strong>新品内容生产</strong>'
      : `<span>南京澄观内容科技</span><i data-lucide="chevron-right" class="icon"></i><strong>${escapeHtml(title)}</strong>`;

    const actions = {
      workspace: '<button class="button button-primary" data-action="new-task"><i data-lucide="plus" class="icon"></i><span>新建任务</span></button>',
      inbox: '<button class="button button-secondary" data-action="refresh-inputs"><i data-lucide="refresh-cw" class="icon"></i>刷新输入</button>',
      knowledge: '<button class="button button-primary" data-action="new-knowledge-task"><i data-lucide="list-plus" class="icon"></i>新建知识任务</button>',
      'my-tasks': '<button class="button button-primary" data-action="new-task"><i data-lucide="plus" class="icon"></i>新建任务</button>',
      'all-tasks': '<button class="button button-primary" data-action="new-task"><i data-lucide="plus" class="icon"></i>新建任务</button>',
      chat: '<button class="button button-secondary" data-action="open-context-record"><i data-lucide="notebook-pen" class="icon"></i>记录业务事项</button>',
      automation: '<button class="button button-primary" data-action="open-operation" data-operation="create-rule"><i data-lucide="plus" class="icon"></i>新建本地规则</button>',
      agents: '<button class="button button-primary" data-action="open-operation" data-operation="create-executor"><i data-lucide="plus" class="icon"></i>添加 CLI 配置</button>',
      swarm: '<button class="button button-primary" data-action="open-operation" data-operation="connect-workspace"><i data-lucide="plus" class="icon"></i>连接 Workspace</button>',
      usage: '<button class="button button-secondary" data-action="export-data" data-export="usage"><i data-lucide="download" class="icon"></i>导出明细</button>',
      tasks: '<button class="button button-secondary" data-action="open-operation" data-operation="project-settings"><i data-lucide="sliders-horizontal" class="icon"></i><span>项目设置</span></button><button class="button button-primary" data-action="new-task"><i data-lucide="plus" class="icon"></i><span>新建任务</span></button>',
      sop: '<button class="button button-secondary" data-action="open-operation" data-operation="project-settings"><i data-lucide="sliders-horizontal" class="icon"></i><span>项目设置</span></button><button class="button button-primary" data-action="create-sop"><i data-lucide="plus" class="icon"></i><span>创建 SOP</span></button>',
      admin: '<button class="button button-secondary" data-action="admin-save"><i data-lucide="save" class="icon"></i>保存配置</button>',
      audit: '<button class="button button-secondary" data-action="export-data" data-export="audit"><i data-lucide="download" class="icon"></i>导出事件</button>'
    };
    $('.header-actions').innerHTML = actions[route] || '';
  }

  function renderTemplates() {
    $('#template-grid').innerHTML = templates.map((template) => `
      <button class="template-card ${template.id === state.selectedTemplate ? 'is-selected' : ''} ${template.id === 'blank' ? 'is-blank' : ''}" data-template="${template.id}">
        <span class="template-icon"><i data-lucide="${template.id === 'blank' ? 'plus' : 'git-branch'}" class="icon"></i></span>
        <span class="template-copy">
          <span class="template-title"><strong>${template.name}</strong><span>${template.steps.length} 个 Stage</span></span>
          <p>${template.description}</p>
          <span class="template-tags">${template.tags.map(([label, tone]) => `<span class="tag ${tone ? `is-${tone}` : ''}">${label}</span>`).join('')}</span>
        </span>
      </button>`).join('');
    iconRefresh();
  }

  function renderBuilder() {
    const template = templates.find((item) => item.id === state.selectedTemplate) || templates[0];
    $('#builder-title').textContent = template.name;
    $('#builder-subtitle').textContent = `${template.steps.length} 个 Stage · ${template.type} · ${template.id === 'blank' ? '草稿' : 'v1.0'}`;
    $('#stage-list').innerHTML = template.steps.map(([name, detail], index) => {
      const isGate = index === template.steps.length - 1 && state.gateMode !== 'none';
      const isCheck = /检查|校验/.test(name);
      const stateClass = isGate ? 'is-gate' : isCheck ? 'is-check' : '';
      const stateIcon = isGate ? 'shield-check' : isCheck ? 'badge-check' : 'circle-dot';
      const stateLabel = isGate ? '可配置 Gate' : isCheck ? '确定性检查' : '业务 Stage';
      return `<div class="stage-row"><span class="stage-number">${index + 1}</span><span class="stage-copy"><strong>${name}</strong><small>${detail}</small></span><span class="stage-state ${stateClass}"><i data-lucide="${stateIcon}" class="icon"></i>${stateLabel}</span></div>`;
    }).join('');
    $('#gate-mode').value = state.gateMode;
    $('#gate-role').value = state.gateRole;
    $('#gate-role').disabled = state.gateMode === 'none';
    $('#gate-toggle').dataset.action = 'toggle-gate';
    $('#gate-toggle').disabled = state.gateMode === 'none';
    $('#gate-toggle').classList.toggle('is-on', state.gateMode !== 'none' && state.gateOn);
    $('#gate-toggle').setAttribute('aria-checked', String(state.gateMode !== 'none' && state.gateOn));
    if (!state.sopNote) state.sopNote = template.id === 'video' ? '从 Brief 到可交付内容，保留证据、权利和 Revision 摘要。' : `围绕${template.type}任务配置输入、输出和可选 Gate。`;
    $('#sop-note').value = state.sopNote;
    iconRefresh();
  }

  function taskRows(items) {
    if (!items.length) return '<div class="empty-state"><span class="card-icon"><i data-lucide="list-checks" class="icon"></i></span><strong>当前筛选没有任务</strong><p>调整状态筛选，或新建一个具体的内容任务。</p></div>';
    return items.map((task) => `
      <button class="task-row" data-action="open-task" data-task-id="${task.id}">
        <span class="task-name"><strong>${escapeHtml(task.title)}</strong><small>${escapeHtml(task.project)} · ${escapeHtml(task.meta)}</small></span>
        <span class="task-stage"><i data-lucide="arrow-right" class="icon"></i>${escapeHtml(task.stage)}</span>
        <span class="status is-${task.tone}">${escapeHtml(task.status)}</span>
        <span class="task-meta"><small>${escapeHtml(task.owner)}</small><small>${escapeHtml(task.executor || '本地 Workspace')} · ${escapeHtml(task.updated)}</small></span>
        <span class="task-open"><i data-lucide="chevron-right" class="icon"></i></span>
      </button>`).join('');
  }

  function renderProjectTasks() {
    const filtered = state.taskFilter === '全部' ? tasks.filter((task) => task.project === '新品内容生产') : tasks.filter((task) => task.project === '新品内容生产' && task.status === state.taskFilter);
    $('#task-list').innerHTML = taskRows(filtered);
    $$('.task-filters .filter').forEach((button) => button.classList.toggle('is-active', button.textContent.trim() === state.taskFilter));
    iconRefresh();
  }

  function metric(label, value, detail) {
    return `<div class="metric-card"><span>${label}</span><strong>${value}</strong><small>${detail}</small></div>`;
  }

  function listCard(title, subtitle, icon, body, tone = '') {
    return `<section class="generic-card"><header><div><span class="eyebrow">${subtitle}</span><h3>${title}</h3></div><span class="card-icon ${tone ? `is-${tone}` : ''}"><i data-lucide="${icon}" class="icon"></i></span></header><div class="generic-card-body">${body}</div></section>`;
  }

  function renderWorkspace() {
    const queue = tasks.filter((task) => ['待补资料', '待 Gate', '待处理'].includes(task.status)).slice(0, 4).map((task) => `
      <div class="list-row"><span class="row-icon is-${task.tone}"><i data-lucide="circle-dot" class="icon"></i></span><span class="row-copy"><strong>${task.title}</strong><small>${task.stage} · ${task.status}</small></span><span class="row-end"><button class="button button-secondary" data-action="open-task" data-task-id="${task.id}">处理</button></span></div>`).join('');
    const projects = `
      <div class="list-row"><span class="row-icon is-blue"><i data-lucide="folder-kanban" class="icon"></i></span><span class="row-copy"><strong>新品内容生产</strong><small>8 个任务 · 2 个待 Gate · SOP v1.0</small></span><span class="row-end"><button class="button button-secondary" data-action="quick-nav" data-route="tasks">打开</button></span></div>
      <div class="list-row"><span class="row-icon"><i data-lucide="folder-kanban" class="icon"></i></span><span class="row-copy"><strong>品牌知识建设</strong><small>3 个任务 · 12 条知识候选</small></span><span class="row-end"><button class="button button-secondary" data-action="quick-nav" data-route="knowledge">打开知识库</button></span></div>`;
    $('#generic-view').innerHTML = `
      <div class="metric-grid">${metric('等待我处理', '5', '2 个 Gate，3 个任务')}${metric('本地运行中', '2', 'Codex 1 · Claude 1')}${metric('本周已交付', '5', '100% 绑定接受快照')}${metric('待分流输入', inboxItems.filter((item) => item.status === 'open').length, '来自本地文件、消息和采集')}</div>
      <div class="generic-grid is-two">
        ${listCard('下一动作', 'Action Queue', 'list-todo', `<div class="list">${queue}</div>`, 'production')}
        ${listCard('活跃 Project', 'Business Context', 'folder-kanban', `<div class="list">${projects}</div>`, 'blue')}
        ${listCard('运行状态', 'Infrastructure', 'activity', '<div class="check-list"><div class="check-item"><i data-lucide="check-circle-2" class="icon"></i><div><strong>Workspace 已连接</strong><small>本地执行节点 2 分钟前心跳正常。</small></div></div><div class="check-item"><i data-lucide="check-circle-2" class="icon"></i><div><strong>事实存储正常</strong><small>Revision、Evidence、Rights 与 Decision 可写入。</small></div></div><div class="check-item is-warning"><i data-lucide="alert-triangle" class="icon"></i><div><strong>1 个来源需要重新授权</strong><small>只影响外部采集，不影响当前任务运行。</small></div></div></div>')}
        ${listCard('最近活动', 'Governed Events', 'scroll-text', '<div class="list"><div class="list-row"><span class="row-icon is-success"><i data-lucide="package-check" class="icon"></i></span><span class="row-copy"><strong>交付包 #28 已生成</strong><small>关联 AcceptedSnapshot · 陈璐</small></span></div><div class="list-row"><span class="row-icon is-blue"><i data-lucide="terminal-square" class="icon"></i></span><span class="row-copy"><strong>Claude Code 完成一次本地运行</strong><small>产生 Revision #46 · 未自动接受</small></span></div><div class="list-row"><span class="row-icon is-warning"><i data-lucide="shield-alert" class="icon"></i></span><span class="row-copy"><strong>素材 Rights 即将到期</strong><small>7 天后到期 · 已创建提醒</small></span></div></div>', 'review')}
      </div>`;
    iconRefresh();
  }

  function renderInbox() {
    const statusMap = { all: 'open', converted: 'converted', archived: 'archived' };
    const items = inboxItems.filter((item) => item.status === statusMap[state.inboxFilter]);
    const tabLabel = state.inboxFilter === 'all' ? '待处理' : state.inboxFilter === 'converted' ? '已转任务' : '已归档';
    $('#generic-view').innerHTML = `
      <div class="generic-tabs"><button class="generic-tab ${state.inboxFilter === 'all' ? 'is-active' : ''}" data-action="inbox-filter" data-inbox-filter="all">待处理 <span class="count-badge">${inboxItems.filter((item) => item.status === 'open').length}</span></button><button class="generic-tab ${state.inboxFilter === 'converted' ? 'is-active' : ''}" data-action="inbox-filter" data-inbox-filter="converted">已转任务</button><button class="generic-tab ${state.inboxFilter === 'archived' ? 'is-active' : ''}" data-action="inbox-filter" data-inbox-filter="archived">已归档</button></div>
      <section class="generic-card"><header><div><span class="eyebrow">输入分流</span><h3>${tabLabel}输入</h3><p>本地文件、聊天消息和采集结果先停在这里，由人明确转为 Task、补充到已有 Task 或归档。最近刷新：${escapeHtml(state.inputLastRefresh)}</p></div><span class="card-icon"><i data-lucide="inbox" class="icon"></i></span></header><div class="generic-card-body"><div class="triage-list">${items.length ? items.map((item) => `
        <div class="triage-row"><span class="row-icon is-${item.tone}"><i data-lucide="${item.icon}" class="icon"></i></span><div class="triage-copy"><div class="resource-meta"><span class="tag">${item.source}</span><span class="tag">${item.status === 'open' ? '待分流' : item.status === 'converted' ? '已生成 Task' : '已归档'}</span></div><strong>${item.title}</strong><p>${item.detail}</p><small>${item.next || '已记录处理结果，可从任务中心继续查看。'}</small></div><div class="triage-actions">${item.status === 'open' ? `<button class="button button-primary" data-action="convert-inbox" data-inbox-id="${item.id}">转为任务</button><button class="button button-ghost" data-action="archive-inbox" data-inbox-id="${item.id}">归档</button>` : `<button class="button button-secondary" data-action="quick-nav" data-route="${item.status === 'converted' ? 'tasks' : 'workspace'}">${item.status === 'converted' ? '查看任务' : '回到工作台'}</button>`}</div></div>`).join('') : '<div class="empty-state"><span class="card-icon"><i data-lucide="inbox" class="icon"></i></span><strong>没有待处理输入</strong><p>新的本地文件、聊天消息和采集结果会显示在这里。</p></div>'}</div></div></section>`;
    iconRefresh();
  }

  function renderKnowledgeObject(item) {
    return `<button class="knowledge-row" data-action="open-knowledge" data-knowledge-id="${escapeHtml(item.id)}"><span class="row-icon is-${escapeHtml(item.tone)}"><i data-lucide="${item.type === 'ConflictRecord' ? 'git-compare-arrows' : item.type === 'KnowledgeGap' ? 'circle-help' : item.type === 'Claim' ? 'message-square-quote' : item.type === 'RightsRecord' ? 'copyright' : 'book-open-check'}" class="icon"></i></span><span class="knowledge-copy"><span class="resource-meta"><span class="tag">${escapeHtml(item.type)}</span><span>${escapeHtml(item.category)}</span><span>${escapeHtml(item.version)}</span></span><strong>${escapeHtml(item.title)}</strong><small>${escapeHtml(item.summary)}</small><span class="knowledge-provenance"><i data-lucide="link-2" class="icon"></i>${escapeHtml(item.source)} · ${escapeHtml(item.evidence)} · ${escapeHtml(item.relations)}</span></span><span class="knowledge-end"><span class="status is-${escapeHtml(item.tone)}">${escapeHtml(item.statusLabel)}</span><small>${escapeHtml(item.updated)}</small><i data-lucide="chevron-right" class="icon"></i></span></button>`;
  }

  function renderKnowledgeSource(source) {
    return `<button class="knowledge-row" data-action="source-detail" data-source-id="${escapeHtml(source.id)}"><span class="row-icon is-${escapeHtml(source.tone)}"><i data-lucide="file-search" class="icon"></i></span><span class="knowledge-copy"><span class="resource-meta"><span class="tag">${escapeHtml(source.type)}</span><span>${escapeHtml(source.locator)}</span></span><strong>${escapeHtml(source.title)}</strong><small>${escapeHtml(source.detail)}</small><span class="knowledge-provenance"><i data-lucide="fingerprint" class="icon"></i>${escapeHtml(source.digest)} · ${source.objects} 个对象</span></span><span class="knowledge-end"><span class="status is-${escapeHtml(source.tone)}">${escapeHtml(source.status)}</span><small>${escapeHtml(source.updated)}</small><i data-lucide="chevron-right" class="icon"></i></span></button>`;
  }

  function knowledgeTabs() {
    const reviewCount = knowledgeObjects.filter((item) => ['needs_review', 'candidate', 'open', 'source_missing'].includes(item.status)).length;
    return `<div class="generic-tabs knowledge-tabs"><button class="generic-tab ${state.knowledgeTab === 'overview' ? 'is-active' : ''}" data-action="knowledge-tab" data-knowledge-tab="overview">概览</button><button class="generic-tab ${state.knowledgeTab === 'objects' ? 'is-active' : ''}" data-action="knowledge-tab" data-knowledge-tab="objects">对象浏览 <span class="count-badge">${knowledgeObjects.length}</span></button><button class="generic-tab ${state.knowledgeTab === 'review' ? 'is-active' : ''}" data-action="knowledge-tab" data-knowledge-tab="review">待审与冲突 <span class="count-badge">${reviewCount}</span></button><button class="generic-tab ${state.knowledgeTab === 'sources' ? 'is-active' : ''}" data-action="knowledge-tab" data-knowledge-tab="sources">来源与证据 <span class="count-badge">${knowledgeSources.length}</span></button><button class="generic-tab ${state.knowledgeTab === 'packs' ? 'is-active' : ''}" data-action="knowledge-tab" data-knowledge-tab="packs">知识包与快照</button><button class="generic-tab ${state.knowledgeTab === 'query' ? 'is-active' : ''}" data-action="knowledge-tab" data-knowledge-tab="query"><i data-lucide="search-check" class="icon"></i>查询</button></div>`;
  }

  function renderKnowledgeOverview() {
    const reviewItems = knowledgeObjects.filter((item) => ['needs_review', 'candidate', 'open', 'source_missing'].includes(item.status)).slice(0, 4);
    return `<div class="knowledge-overview"><div class="knowledge-hero"><div><span class="eyebrow">Knowledge Operating Surface</span><h2>知识不是一张文档列表，而是可追溯的内容基础设施。</h2><p>从来源摄取候选对象，经过 Evidence、冲突、权利和人工决策，形成可被 SOP 绑定的知识快照。每个对象都有类型、状态、来源、关系和影响范围。</p></div><div class="knowledge-hero-actions"><button class="button button-secondary" data-action="knowledge-lint"><i data-lucide="badge-check" class="icon"></i>运行知识校验</button><button class="button button-primary" data-action="new-knowledge-task"><i data-lucide="list-plus" class="icon"></i>新建知识任务</button></div></div><div class="knowledge-layer-grid">${knowledgeLayers.map((layer) => `<button class="knowledge-layer" data-action="knowledge-layer" data-knowledge-layer="${escapeHtml(layer.id)}"><span class="layer-icon"><i data-lucide="${escapeHtml(layer.icon)}" class="icon"></i></span><span><strong>${escapeHtml(layer.name)}</strong><small>${escapeHtml(layer.detail)}</small></span><span class="layer-metric"><b>${layer.coverage}%</b><small>${layer.objects} 个对象 · ${layer.gaps} 个缺口</small></span><span class="progress"><span style="width:${layer.coverage}%"></span></span></button>`).join('')}</div><div class="generic-grid is-two"><section class="generic-card"><header><div><span class="eyebrow">Review Queue</span><h3>当前需要人处理的知识</h3><p>状态决策必须针对具体对象，不会因为运行成功自动变成正式知识。</p></div><span class="card-icon is-review"><i data-lucide="list-todo" class="icon"></i></span></header><div class="knowledge-list">${reviewItems.map(renderKnowledgeObject).join('')}</div><div class="generic-card-body"><button class="button button-secondary" data-action="knowledge-tab" data-knowledge-tab="review"><i data-lucide="arrow-up-right" class="icon"></i>查看完整待审队列</button></div></section><section class="generic-card"><header><div><span class="eyebrow">Knowledge Health</span><h3>知识库健康</h3><p>健康度来自来源覆盖、状态决策和可复用快照，不是条目数量。</p></div><span class="status is-warning">需补料</span></header><div class="generic-card-body"><div class="health-score"><strong>68</strong><span>/ 100</span><small>本周 +6 · 仍有 4 个高影响缺口</small></div><div class="check-list"><div class="check-item"><i data-lucide="check-circle-2" class="icon"></i><div><strong>6 个来源已登记并生成 digest</strong><small>可定位到页码、表格单元格或 Bundle block。</small></div></div><div class="check-item is-warning"><i data-lucide="alert-triangle" class="icon"></i><div><strong>1 个规格冲突正在阻断 Claim</strong><small>解决前不能进入新的营销知识快照。</small></div></div><div class="check-item is-warning"><i data-lucide="circle-help" class="icon"></i><div><strong>3 个知识缺口需要客户补料</strong><small>缺口会被同步到 Task 的下一动作。</small></div></div></div></div></section></div></div>`;
  }

  function renderKnowledgeObjects() {
    const query = state.knowledgeSearch.trim().toLowerCase();
    const items = knowledgeObjects.filter((item) => (state.knowledgeType === 'all' || item.type === state.knowledgeType) && (state.knowledgeLayer === 'all' || item.layer === state.knowledgeLayer) && (!query || `${item.title} ${item.summary} ${item.source}`.toLowerCase().includes(query)));
    const typeOptions = ['all', ...new Set(knowledgeObjects.map((item) => item.type))];
    return `<div class="knowledge-toolbar"><div><span class="eyebrow">Object Browser</span><h2>对象浏览</h2><p>按层级、类型、状态和来源检查知识对象，不把不同状态混成一张“已确认列表”。</p></div><div class="knowledge-filters"><select id="knowledge-layer-filter" aria-label="按知识层级筛选"><option value="all" ${state.knowledgeLayer === 'all' ? 'selected' : ''}>全部层级</option>${knowledgeLayers.map((layer) => `<option value="${escapeHtml(layer.id)}" ${state.knowledgeLayer === layer.id ? 'selected' : ''}>${escapeHtml(layer.name)}</option>`).join('')}</select><select id="knowledge-type-filter" aria-label="按知识类型筛选">${typeOptions.map((type) => `<option value="${escapeHtml(type)}" ${state.knowledgeType === type ? 'selected' : ''}>${type === 'all' ? '全部类型' : escapeHtml(type)}</option>`).join('')}</select><input id="knowledge-search" value="${escapeHtml(state.knowledgeSearch)}" placeholder="搜索对象、来源或摘要" aria-label="搜索知识对象"><button class="button button-secondary" data-action="knowledge-search"><i data-lucide="search" class="icon"></i>搜索</button></div></div><section class="generic-card"><header><div><span class="eyebrow">Typed Registry</span><h3>${items.length} 个知识对象</h3><p>每一行都有稳定 ID、类型、状态、来源、Evidence、关系和使用影响。</p></div><span class="card-icon"><i data-lucide="database-zap" class="icon"></i></span></header><div class="knowledge-list">${items.length ? items.map(renderKnowledgeObject).join('') : '<div class="empty-state"><span class="card-icon"><i data-lucide="search-x" class="icon"></i></span><strong>没有匹配的知识对象</strong><p>调整层级、类型或搜索词，保留候选和缺口的可见性。</p></div>'}</div></section>`;
  }

  function renderKnowledgeReview() {
    const reviewItems = knowledgeObjects.filter((item) => ['needs_review', 'candidate', 'open', 'source_missing'].includes(item.status));
    const conflicts = knowledgeObjects.filter((item) => ['ConflictRecord', 'KnowledgeGap'].includes(item.type));
    return `<div class="knowledge-toolbar"><div><span class="eyebrow">Review And Gaps</span><h2>待审与冲突</h2><p>把需要客户或流程负责人的明确决定集中在这里；拒绝、补料和解决冲突都会留下理由。</p></div><span class="status is-warning">${reviewItems.length} 项待处理</span></div><div class="generic-grid is-two"><section class="generic-card"><header><div><span class="eyebrow">Review Queue</span><h3>状态决策队列</h3><p>FactAssertion、Claim、Insight 和权利记录分别遵循自己的状态机。</p></div><span class="card-icon is-review"><i data-lucide="gavel" class="icon"></i></span></header><div class="knowledge-list">${reviewItems.map(renderKnowledgeObject).join('')}</div></section><section class="generic-card"><header><div><span class="eyebrow">Conflicts And Gaps</span><h3>冲突与知识缺口</h3><p>缺口不是空白说明，而是可指派、可追踪的补料工作。</p></div><span class="card-icon is-warning"><i data-lucide="triangle-alert" class="icon"></i></span></header><div class="knowledge-list">${conflicts.map(renderKnowledgeObject).join('')}</div><div class="generic-card-body"><button class="button button-primary" data-action="new-knowledge-task"><i data-lucide="list-plus" class="icon"></i>创建补料任务</button></div></section></div>`;
  }

  function renderKnowledgeSources() {
    return `<div class="knowledge-toolbar"><div><span class="eyebrow">Source Registry</span><h2>来源与 Evidence</h2><p>来源保存原件身份和 digest，Evidence 保存可复核定位；知识对象不能只引用一段无定位的文本。</p></div><div class="knowledge-filters"><button class="button button-secondary" data-action="open-operation" data-operation="register-source"><i data-lucide="file-plus-2" class="icon"></i>登记来源</button><button class="button button-primary" data-action="open-operation" data-operation="create-ingest"><i data-lucide="scan-search" class="icon"></i>创建摄取任务</button></div></div><section class="generic-card"><header><div><span class="eyebrow">Registered Sources</span><h3>${knowledgeSources.length} 个来源</h3><p>支持 Workspace 文件、受控文档、外部来源和客户端导出的 ConversationBundle。</p></div><span class="card-icon is-blue"><i data-lucide="files" class="icon"></i></span></header><div class="knowledge-list">${knowledgeSources.map(renderKnowledgeSource).join('')}</div></section><div class="generic-grid is-two" style="margin-top:12px"><section class="generic-card"><header><div><span class="eyebrow">Ingest Pipeline</span><h3>最近摄取运行</h3><p>摄取只生成候选对象，不直接进入可引用快照。</p></div><span class="status is-success">lint 通过</span></header><div class="generic-card-body"><div class="pipeline-steps"><div class="pipeline-step is-done"><span>1</span><strong>登记 Source</strong><small>hash / mime / owner</small></div><div class="pipeline-step is-done"><span>2</span><strong>定位 Evidence</strong><small>page / cell / block</small></div><div class="pipeline-step is-done"><span>3</span><strong>生成候选对象</strong><small>Fact / Claim / Rights</small></div><div class="pipeline-step is-active"><span>4</span><strong>进入待审队列</strong><small>需要人类决策</small></div></div></div></section><section class="generic-card"><header><div><span class="eyebrow">Source Health</span><h3>来源质量</h3><p>校验原件、定位、更新和权利状态。</p></div><span class="status is-warning">2 项提醒</span></header><div class="generic-card-body"><div class="check-list"><div class="check-item"><i data-lucide="check-circle-2" class="icon"></i><div><strong>${knowledgeSources.length}/${knowledgeSources.length} 来源有 digest</strong><small>可以检测来源替换和重复摄取。</small></div></div><div class="check-item is-warning"><i data-lucide="alert-triangle" class="icon"></i><div><strong>2 项素材权利即将到期</strong><small>影响 5 个可交付内容引用。</small></div></div><div class="check-item is-warning"><i data-lucide="circle-help" class="icon"></i><div><strong>1 个外部来源定位范围未确认</strong><small>不会进入营销 Claim 的确定性查询。</small></div></div></div></div></section></div>`;
  }

  function renderKnowledgePacks() {
    return `<div class="knowledge-toolbar"><div><span class="eyebrow">Knowledge Packs</span><h2>知识包与快照</h2><p>知识包是按业务用途组织的对象集合；快照是绑定到 TaskRun 的不可变版本。</p></div><div class="knowledge-filters"><button class="button button-secondary" data-action="open-operation" data-operation="create-pack"><i data-lucide="package-plus" class="icon"></i>新建知识包</button><button class="button button-primary" data-action="open-operation" data-operation="impact-analysis"><i data-lucide="git-compare" class="icon"></i>运行影响分析</button></div></div><div class="knowledge-pack-grid">${knowledgePacks.map((pack) => `<article class="knowledge-pack"><header><div><span class="eyebrow">${escapeHtml(pack.id)}</span><h3>${escapeHtml(pack.name)}</h3></div><span class="status is-${escapeHtml(pack.tone)}">${escapeHtml(pack.statusLabel)}</span></header><div class="knowledge-pack-body"><div class="pack-meta"><span><b>${escapeHtml(pack.version)}</b><small>版本</small></span><span><b>${pack.objects}</b><small>对象</small></span><span><b>${escapeHtml(pack.layers)}</b><small>覆盖</small></span><span><b>${pack.tasks}</b><small>使用 Task</small></span></div><div class="pack-snapshot"><span>当前绑定快照</span><strong>${escapeHtml(pack.snapshot)}</strong><small>更新于 ${escapeHtml(pack.updated)}</small></div><div class="builder-actions"><button class="button button-secondary" data-action="open-operation" data-operation="pack-version" data-entity-id="${escapeHtml(pack.id)}"><i data-lucide="fingerprint" class="icon"></i>查看版本</button><button class="button button-primary" data-action="open-operation" data-operation="pack-usage" data-entity-id="${escapeHtml(pack.id)}"><i data-lucide="arrow-up-right" class="icon"></i>使用范围</button></div></div></article>`).join('')}</div><section class="generic-card" style="margin-top:12px"><header><div><span class="eyebrow">Snapshot History</span><h3>不可变快照历史</h3><p>新版本只影响新 TaskRun，历史运行保留原有对象集合和 digest。</p></div><span class="card-icon"><i data-lucide="history" class="icon"></i></span></header><div class="generic-card-body"><table class="mini-table"><thead><tr><th>快照</th><th>知识包</th><th>对象变化</th><th>使用 TaskRun</th><th>状态</th></tr></thead><tbody><tr><td><strong>#42</strong><small>昨天 18:40</small></td><td>新品品牌与产品知识包 v4</td><td>+6 / -1 / 1 冲突未纳入</td><td>3</td><td><span class="status is-success">有效</span></td></tr><tr><td><strong>#41</strong><small>3 天前</small></td><td>新品品牌与产品知识包 v4</td><td>+2 / 0 / 0</td><td>11</td><td><span class="status is-muted">历史</span></td></tr><tr><td><strong>#18</strong><small>2 天前</small></td><td>品牌表达治理包 v2.1</td><td>+4 / -2 / 0</td><td>18</td><td><span class="status is-success">有效</span></td></tr></tbody></table></div></section>`;
  }

  function renderKnowledgeQueryObject(item, blocked = false) {
    const icon = blocked
      ? item.type === 'KnowledgeGap' ? 'circle-help' : item.type === 'ConflictRecord' ? 'triangle-alert' : 'shield-alert'
      : 'check-circle-2';
    return `<button class="query-object ${blocked ? 'is-blocked' : ''}" data-action="open-knowledge" data-knowledge-id="${escapeHtml(item.id)}"><i data-lucide="${icon}" class="icon"></i><span><strong>${escapeHtml(item.title)}</strong><small>${escapeHtml(item.type)} · ${escapeHtml(item.status)} · ${escapeHtml(item.evidence)}</small></span></button>`;
  }

  function renderKnowledgeQuery() {
    const relevantIds = ['assertion:base-parameters', 'rule:brand-language', 'rights:product-assets', 'claim:light-portable', 'conflict:spec-version', 'gap:public-claim-scope'];
    const relevant = relevantIds.map((id) => knowledgeObjects.find((item) => item.id === id)).filter(Boolean);
    const eligible = relevant.filter((item) => item.usable);
    const blocked = relevant.filter((item) => !item.usable);
    const gaps = blocked.filter((item) => item.type === 'KnowledgeGap');
    const hardBlocked = blocked.filter((item) => item.type !== 'KnowledgeGap');
    const result = state.knowledgeQueryRan ? `<section class="query-result"><div class="query-result-summary"><span class="status is-success">查询完成</span><strong>可引用 ${eligible.length} 个对象，阻断 ${hardBlocked.length} 个对象，发现 ${gaps.length} 个知识缺口。</strong><small>结果只使用 verified / approved / valid / active 状态，并记录 eligible / blocked ID。</small></div><div class="generic-grid is-two"><div><span class="builder-label">Eligible</span><div class="query-object-list">${eligible.map((item) => renderKnowledgeQueryObject(item)).join('')}</div></div><div><span class="builder-label">Blocked / Gaps</span><div class="query-object-list">${blocked.map((item) => renderKnowledgeQueryObject(item, true)).join('')}</div></div></div></section>` : '';
    return `<div class="knowledge-query"><div><span class="eyebrow">Knowledge Query</span><h2>查询知识库</h2><p>查询不是自由聊天，而是带范围、状态和引用结果的业务查询。缺少证据时明确返回阻断和知识缺口。</p></div><div class="query-form"><label class="field"><span>业务问题</span><textarea id="knowledge-query-input" placeholder="例如：当前短视频任务可以使用哪些产品事实和素材？">${escapeHtml(state.knowledgeQuery || '')}</textarea></label><div class="query-options"><label class="field"><span>查询范围</span><select id="knowledge-query-scope"><option>新品内容生产 · 当前快照 #42</option><option>品牌知识建设 · 当前快照 #18</option><option>Environment 全部可用知识</option></select></label><label class="field"><span>允许状态</span><select id="knowledge-query-status"><option>仅可确定性使用</option><option>包含候选并标记风险</option></select></label><button class="button button-primary" data-action="submit-knowledge-query"><i data-lucide="search-check" class="icon"></i>执行查询</button></div></div>${result}</div>`;
  }

  function renderKnowledge() {
    const usable = knowledgeObjects.filter((item) => item.usable).length;
    const review = knowledgeObjects.filter((item) => ['needs_review', 'candidate', 'open', 'source_missing'].includes(item.status)).length;
    let body = '';
    if (state.knowledgeTab === 'overview') body = renderKnowledgeOverview();
    if (state.knowledgeTab === 'objects') body = renderKnowledgeObjects();
    if (state.knowledgeTab === 'review') body = renderKnowledgeReview();
    if (state.knowledgeTab === 'sources') body = renderKnowledgeSources();
    if (state.knowledgeTab === 'packs') body = renderKnowledgePacks();
    if (state.knowledgeTab === 'query') body = renderKnowledgeQuery();
    $('#generic-view').innerHTML = `<div class="metric-grid">${metric('可复用对象', usable, '已绑定状态和来源')}${metric('待决对象', review, '候选、冲突和缺口')}${metric('来源 / Evidence', knowledgeSources.length, '全部有 digest')}${metric('有效快照', '2', '3 个 TaskRun 正在使用')}</div>${knowledgeTabs()}${body}`;
    iconRefresh();
  }

  function renderTaskCenter(mode) {
    const mine = mode === 'my-tasks';
    const scoped = mine ? tasks.filter((task) => task.owner === '林舟' || task.status === '待 Gate') : tasks;
    const items = state.taskCenterFilter === '全部' ? scoped : scoped.filter((task) => task.status === state.taskCenterFilter);
    const filters = ['全部', '执行中', '待 Gate', '已交付'];
    $('#generic-view').innerHTML = `
      <div class="task-toolbar"><div class="task-filters">${filters.map((filter) => `<button class="filter ${state.taskCenterFilter === filter ? 'is-active' : ''}" data-action="task-center-filter" data-task-filter="${filter}">${filter}</button>`).join('')}</div><span class="section-meta">${items.length} 个任务</span></div>
      <div class="task-table"><div class="task-table-head"><span>任务</span><span>当前 Stage</span><span>状态</span><span>负责人</span><span></span></div><div>${taskRows(items)}</div></div>`;
    iconRefresh();
  }

  function renderContextScope(item) {
    return `<button class="thread ${item.id === state.activeContextScope ? 'is-active' : ''}" data-action="select-context-scope" data-context-scope-id="${escapeHtml(item.id)}"><span class="thread-avatar">${escapeHtml(item.short)}</span><span class="thread-copy"><strong>${escapeHtml(item.name)}</strong><small>${escapeHtml(item.description)}</small></span></button>`;
  }

  function renderContextRecord(record) {
    return `<article class="context-record"><span class="row-icon is-${escapeHtml(record.tone)}"><i data-lucide="${escapeHtml(record.icon)}" class="icon"></i></span><div class="context-record-copy"><span class="resource-meta"><span class="tag">${escapeHtml(record.kind)}</span><span>${escapeHtml(record.author)} · ${escapeHtml(record.time)}</span></span><strong>${escapeHtml(record.title)}</strong><p>${escapeHtml(record.detail)}</p></div></article>`;
  }

  function renderChat() {
    const scope = contextScopes.find((item) => item.id === state.activeContextScope);
    const records = contextRecords[state.activeContextScope];
    $('#generic-view').innerHTML = `<div class="notice"><i data-lucide="info" class="icon"></i><div><strong>这里是业务上下文账本，不是聊天窗口。</strong> 只显示明确提交的输入补充、业务决定、运行摘要和交付事实；本地 CLI 对话不会逐轮同步。</div></div><div class="chat-shell context-shell"><aside class="chat-threads"><header><h3>上下文范围</h3><small style="display:block;margin-top:5px;color:var(--muted);font-size:10px">按 Project 和交付批次组织</small></header>${contextScopes.map(renderContextScope).join('')}</aside><section class="chat-main"><header><span class="thread-avatar">${escapeHtml(scope.short)}</span><strong>${escapeHtml(scope.name)}</strong><small>结构化业务记录 · 不自动升级为正式事实</small></header><div class="context-feed">${records.map(renderContextRecord).join('')}</div><div class="local-run-strip"><div><span class="eyebrow">Local Run</span><strong>当前 Task 的本地执行</strong><small>Claude Code CLI · 知识与证据 Stage · 运行中 · 只上报结构化摘要</small></div><button class="button button-secondary" data-action="quick-nav" data-route="tasks"><i data-lucide="arrow-up-right" class="icon"></i>打开任务</button></div><div class="context-actions"><span>需要新增内容时，请选择明确的业务动作。</span><div><button class="button button-secondary" data-action="open-import"><i data-lucide="upload" class="icon"></i>请求客户端导出</button><button class="button button-primary" data-action="open-context-record"><i data-lucide="notebook-pen" class="icon"></i>记录业务事项</button></div></div></section></div>`;
    iconRefresh();
  }

  function renderResourcePage(type) {
    const isAutomation = type === 'automation';
    const title = isAutomation ? '本地规则' : 'CLI 执行器';
    const subtitle = isAutomation ? 'Local Hooks And Schedules' : 'Codex / Claude Code';
    const icon = isAutomation ? 'workflow' : 'terminal-square';
    const body = resources[type].map((item) => `<div class="resource-card"><span class="card-icon ${isAutomation ? '' : 'is-blue'}"><i data-lucide="${icon}" class="icon"></i></span><div class="resource-copy"><strong>${item.name}</strong><p>${item.detail}</p><div class="resource-meta"><span class="tag">${item.meta}</span><span class="status ${item.enabled ? 'is-success' : 'is-muted'}">${item.enabled ? '已启用' : '已停用'}</span></div></div><div class="row-end"><button class="button button-secondary" data-action="run-resource" data-resource-name="${item.name}"><i data-lucide="play" class="icon"></i>试运行</button><button class="toggle ${item.enabled ? 'is-on' : ''}" data-action="toggle-resource" data-resource-type="${type}" data-resource-id="${item.id}" role="switch" aria-checked="${item.enabled}" title="${item.enabled ? '停用' : '启用'}${item.name}"></button></div></div>`).join('');
    $('#generic-view').innerHTML = `${listCard(title, subtitle, icon, body, isAutomation ? '' : 'blue')}<div class="notice" style="margin-top:14px"><i data-lucide="shield-check" class="icon"></i><div><strong>所有执行都发生在本地 Workspace。</strong> 页面只管理可用配置和边界，不上传未发布正文，也不把 CLI transcript 当作正式内容事实。</div></div>`;
    iconRefresh();
  }

  function renderSwarm() {
    const nodes = workspaceNodes.map((node) => `<div class="check-item"><i data-lucide="${node.type === 'Workspace' ? 'laptop-minimal-check' : 'terminal-square'}" class="icon"></i><div><strong>${escapeHtml(node.name)}</strong><small>${escapeHtml(node.type)} · ${escapeHtml(node.statusLabel)} · 最近心跳 ${escapeHtml(node.heartbeat)}</small></div></div>`).join('');
    $('#generic-view').innerHTML = `<div class="generic-grid is-two">
      ${listCard('我的本地 Workspace', 'Connected Node', 'laptop-minimal-check', `<div class="check-list">${nodes}</div><div class="builder-actions"><button class="button button-secondary" data-action="open-operation" data-operation="workspace-settings"><i data-lucide="settings-2" class="icon"></i>配置</button><button class="button button-primary" data-action="run-resource" data-resource-name="本地 Workspace"><i data-lucide="play" class="icon"></i>领取下一任务</button></div>`, 'blue')}
      ${listCard('并行执行槽', 'Local Sessions', 'layers-3', '<div class="metric-grid" style="grid-template-columns:repeat(3,minmax(0,1fr));margin-bottom:0"><div class="metric-card"><span>已占用</span><strong>2</strong><small>Codex 1 · Claude 1</small></div><div class="metric-card"><span>可用</span><strong>3</strong><small>等待任务领取</small></div><div class="metric-card"><span>异常</span><strong>0</strong><small>最近 24 小时</small></div></div><div class="notice" style="margin:14px 0 0"><i data-lucide="info" class="icon"></i><div>并行槽只是本地执行能力，不改变 Task 的业务状态。只有提交的 Revision、检查结果和接受快照会进入正式链路。</div></div>', 'production')}
    </div>`;
    iconRefresh();
  }

  function renderUsage() {
    const data = state.usageRange === '7d' ? [42, 58, 46, 72, 63, 81, 69] : state.usageRange === '30d' ? [36, 48, 64, 55, 77, 68, 84] : [25, 43, 38, 59, 72, 66, 81];
    const labels = state.usageRange === '7d' ? ['一', '二', '三', '四', '五', '六', '日'] : ['W1', 'W2', 'W3', 'W4', 'W5', 'W6', 'W7'];
    $('#generic-view').innerHTML = `<div class="generic-heading"><div><h2>运行与成本</h2><p>按环境统计，不把 Token 数量当作业务价值。</p></div><div class="segmented"><button class="${state.usageRange === '7d' ? 'is-active' : ''}" data-action="usage-range" data-range="7d">7 天</button><button class="${state.usageRange === '30d' ? 'is-active' : ''}" data-action="usage-range" data-range="30d">30 天</button><button class="${state.usageRange === '90d' ? 'is-active' : ''}" data-action="usage-range" data-range="90d">90 天</button></div></div><div class="metric-grid">${metric('任务运行', '126', '成功 119，失败 7')}${metric('能力调用', '842', '环比 +12%')}${metric('平均耗时', '2m 18s', 'P95 6m 42s')}${metric('估算成本', '¥ 386', '每个接受交付 ¥ 7.72')}</div><div class="generic-grid is-two">${listCard('每日能力调用', 'Observed Usage', 'chart-no-axes-combined', `<div class="chart">${data.map((height, index) => `<div class="bar"><span style="height:${height}%"></span><small>${labels[index]}</small></div>`).join('')}</div><div class="generic-card-body"><small style="color:var(--muted)">脚本创作占 46%，知识提取占 31%，检查与其他占 23%。</small></div>`, 'blue')}${listCard('质量与失败', 'Operational Quality', 'activity', '<div class="check-list"><div class="check-item"><i data-lucide="check-circle-2" class="icon"></i><div><strong>94.4% 运行成功</strong><small>7 次失败均保留可重试上下文。</small></div></div><div class="check-item"><i data-lucide="check-circle-2" class="icon"></i><div><strong>100% 交付绑定接受快照</strong><small>没有从未接受 Revision 直接交付。</small></div></div><div class="check-item is-warning"><i data-lucide="alert-triangle" class="icon"></i><div><strong>2 次 Rights 检查阻断</strong><small>属于有效业务保护，不计为系统失败。</small></div></div></div>', 'review')}</div>`;
    iconRefresh();
  }

  const adminMenu = [
    ['environment', 'building-2', 'Environment'], ['sops', 'git-branch', 'SOP Registry'], ['gates', 'shield-check', 'Gate Policy'], ['capabilities', 'blocks', '能力'], ['execution', 'terminal-square', '本地执行'], ['roles', 'users', '角色权限'], ['audit', 'scroll-text', '审计']
  ];

  function adminPanel() {
    const sopRows = sopRegistry.map((sop) => `<tr><td><strong>${escapeHtml(sop.name)}</strong><small>${sop.builtin ? '平台内置' : '企业自定义'}${sop.default ? ' · 环境默认' : ''}</small></td><td>${escapeHtml(sop.version)}</td><td><span class="status is-${sop.status === 'published' ? 'success' : 'warning'}">${escapeHtml(sop.statusLabel)}</span></td><td>${sop.tasks}</td><td>${sop.status === 'draft' ? `<button class="button button-primary" data-action="publish-sop" data-sop-id="${sop.id}">发布</button>` : `<button class="button button-secondary" data-action="open-operation" data-operation="new-sop-version" data-entity-id="${sop.id}">新版本</button>`}</td></tr>`).join('');
    const gateRows = gatePolicies.map((policy) => `<div class="switch-row"><div><strong>${escapeHtml(policy.name)}</strong><small>${escapeHtml(policy.detail)}</small></div><button class="toggle ${policy.enabled ? 'is-on' : ''}" data-action="toggle-gate-policy" data-entity-id="${policy.id}" role="switch" aria-checked="${policy.enabled}"></button></div>`).join('');
    const roleRows = tenantRoles.map((role) => `<tr><td><strong>${escapeHtml(role.name)}</strong></td><td>${role.members}</td><td>${escapeHtml(role.permissions)}</td><td>${escapeHtml(role.scope)}</td><td><button class="button button-secondary" data-action="open-operation" data-operation="role-detail" data-entity-id="${role.id}">查看</button></td></tr>`).join('');
    const nodeRows = workspaceNodes.map((node) => `<tr><td><strong>${escapeHtml(node.name)}</strong><small>${node.slots} 个执行槽</small></td><td>${escapeHtml(node.type)}</td><td>${escapeHtml(node.heartbeat)}</td><td><span class="status is-success">${escapeHtml(node.statusLabel)}</span></td><td><button class="button button-secondary" data-action="open-operation" data-operation="node-detail" data-entity-id="${node.id}">配置</button></td></tr>`).join('');
    const upgradeNotice = state.legacyUpgradeStatus === 'available'
      ? '<div class="notice"><i data-lucide="arrow-up-circle" class="icon"></i><div><strong>检测到 1 条旧版流程可以兼容升级。</strong> 只有结构精确匹配的流程才会生成新版本；原版本、digest、历史任务和运行绑定保持不变。 <button class="button button-secondary" data-action="open-operation" data-operation="legacy-upgrade" style="margin-left:8px">查看升级预览</button></div></div>'
      : '<div class="notice"><i data-lucide="check-circle-2" class="icon"></i><div><strong>旧版流程已生成兼容新版本。</strong> 历史运行仍固定旧 digest，新任务可显式选择升级后的版本。</div></div>';
    const panels = {
      environment: `<section class="admin-panel"><header><div><span class="eyebrow">Environment</span><h3>${escapeHtml(projectConfig.environment)}</h3><p>环境是 SOP、能力、权限和事实治理的生效边界。</p></div><span class="status is-success">稳定</span></header><div class="admin-body"><div class="form-row"><label class="field"><span>环境名称</span><input id="admin-environment-name" value="${escapeHtml(projectConfig.environment)}"></label><label class="field"><span>默认时区</span><select id="admin-timezone"><option>Asia/Shanghai</option><option>Asia/Hong_Kong</option></select></label></div><div class="builder-divider"></div><div class="switch-row"><div><strong>本地 Workspace 执行</strong><small>允许已注册节点领取 TaskRun。</small></div><button class="toggle is-on" data-action="simple-toggle" role="switch" aria-checked="true"></button></div><div class="switch-row"><div><strong>外部发布</strong><small>关闭时只允许生成发布候选和交付包。</small></div><button class="toggle" data-action="simple-toggle" role="switch" aria-checked="false"></button></div><div class="builder-actions"><button class="button button-primary" data-action="admin-save"><i data-lucide="save" class="icon"></i>保存环境</button></div></div></section>`,
      sops: `<section class="admin-panel"><header><div><span class="eyebrow">SOP Registry</span><h3>环境方法论</h3><p>默认提供四条业务闭环 SOP；空白入口用于企业自定义，不计为内置 SOP。</p></div><button class="button button-primary" data-action="quick-nav" data-route="sop"><i data-lucide="plus" class="icon"></i>创建 SOP</button></header><div class="admin-body">${upgradeNotice}<table class="mini-table"><thead><tr><th>SOP</th><th>版本</th><th>状态</th><th>使用任务</th><th>操作</th></tr></thead><tbody>${sopRows}</tbody></table></div></section>`,
      gates: `<section class="admin-panel"><header><div><span class="eyebrow">Gate Policy</span><h3>决定点策略</h3><p>Gate 由 SOP 和风险条件触发，不是所有任务固定审批。</p></div><button class="button button-secondary" data-action="open-operation" data-operation="create-gate"><i data-lucide="plus" class="icon"></i>新建策略</button></header><div class="admin-body">${gateRows}</div></section>`,
      capabilities: `<section class="admin-panel"><header><div><span class="eyebrow">Capability Registry</span><h3>环境能力</h3><p>控制哪些能力可以被 SOP、本地 CLI 和规则引用。</p></div><button class="button button-secondary" data-action="open-operation" data-operation="import-capability"><i data-lucide="file-input" class="icon"></i>导入契约</button></header><div class="admin-body">${resources.capabilities.map((item) => `<div class="switch-row"><div><strong>${item.name}</strong><small>${item.detail} · ${item.meta}</small></div><button class="toggle ${item.enabled ? 'is-on' : ''}" data-action="toggle-resource" data-resource-type="capabilities" data-resource-id="${item.id}" role="switch" aria-checked="${item.enabled}"></button></div>`).join('')}</div></section>`,
      execution: `<section class="admin-panel"><header><div><span class="eyebrow">Local Execution</span><h3>本地 Workspace、CLI 与导入适配器</h3><p>注册本机 Workspace、CLI 配置和本地规则；对话导入由客户端适配器完成。</p></div><span class="status is-success">${workspaceNodes.length} 个配置可用</span></header><div class="admin-body"><table class="mini-table"><thead><tr><th>配置</th><th>类型</th><th>最近心跳</th><th>状态</th><th>操作</th></tr></thead><tbody>${nodeRows}</tbody></table><div class="builder-divider"></div><span class="builder-label">Conversation 导入适配器</span><table class="mini-table"><thead><tr><th>客户端</th><th>格式</th><th>能力</th><th>状态</th></tr></thead><tbody>${clientAdapters.map((adapter) => `<tr><td><strong>${adapter.name}</strong><small>${adapter.version}</small></td><td>${adapter.format}</td><td>${adapter.supports.summary ? '摘要' : ''}${adapter.supports.selectedTurns ? ' · 选择性片段' : ''}${adapter.supports.fullTranscript ? ' · 完整 Transcript' : ' · 无完整 Transcript'}</td><td><span class="status ${adapter.status === 'connected' ? 'is-success' : 'is-blue'}">${adapter.statusLabel}</span></td></tr>`).join('')}</tbody></table><div class="notice" style="margin-top:12px"><i data-lucide="shield-check" class="icon"></i><div>适配器在本地选择会话、解析客户端格式并完成脱敏；Web 只接收 ConversationBundle 和生命周期状态。</div></div><div class="builder-actions"><button class="button button-secondary" data-action="open-operation" data-operation="connect-workspace"><i data-lucide="plug-zap" class="icon"></i>连接 Workspace</button><button class="button button-primary" data-action="open-operation" data-operation="create-executor"><i data-lucide="plus" class="icon"></i>添加 CLI 配置</button></div></div></section>`,
      roles: `<section class="admin-panel"><header><div><span class="eyebrow">Access Control</span><h3>角色权限</h3><p>权限绑定事实动作和能力，不只控制页面是否可见。</p></div><button class="button button-secondary" data-action="open-operation" data-operation="create-role"><i data-lucide="user-plus" class="icon"></i>新建角色</button></header><div class="admin-body"><table class="mini-table"><thead><tr><th>角色</th><th>成员</th><th>关键权限</th><th>范围</th><th>操作</th></tr></thead><tbody>${roleRows}</tbody></table></div></section>`,
      audit: `<section class="admin-panel"><header><div><span class="eyebrow">Audit Policy</span><h3>审计与保留</h3><p>定义事件保留、导出和敏感字段查看范围。</p></div><span class="status is-success">持续记录</span></header><div class="admin-body"><div class="form-row"><label class="field"><span>事件保留期</span><select><option>永久保留</option><option>3 年</option><option>1 年</option></select></label><label class="field"><span>导出格式</span><select><option>JSON Lines</option><option>CSV</option></select></label></div><div class="builder-divider"></div><div class="switch-row"><div><strong>记录配置变更前后值</strong><small>敏感字段仅记录摘要，不写入明文。</small></div><button class="toggle is-on" data-action="simple-toggle" role="switch" aria-checked="true"></button></div><div class="switch-row"><div><strong>允许审计员导出</strong><small>导出动作本身也会写入审计事件。</small></div><button class="toggle is-on" data-action="simple-toggle" role="switch" aria-checked="true"></button></div></div></section>`
    };
    return panels[state.adminTab];
  }

  function renderAdmin() {
    $('#generic-view').innerHTML = `<div class="admin-layout"><nav class="admin-menu" aria-label="管理后台配置">${adminMenu.map(([id, icon, label]) => `<button class="${id === state.adminTab ? 'is-active' : ''}" data-action="admin-tab" data-admin-tab="${id}"><i data-lucide="${icon}" class="icon"></i>${label}</button>`).join('')}</nav><div id="admin-panel-slot">${adminPanel()}</div></div>`;
    iconRefresh();
  }

  function renderAudit() {
    const query = state.auditSearch.trim().toLowerCase();
    const events = auditEvents.filter((event) => (state.auditFilter === 'all' || event.category === state.auditFilter) && (!query || `${event.actor} ${event.actorId} ${event.action} ${event.object} ${event.reason}`.toLowerCase().includes(query)));
    const rows = events.map((event) => `<tr class="audit-row" data-action="open-operation" data-operation="audit-detail" data-entity-id="${event.id}"><td>${escapeHtml(event.time)}</td><td><strong>${escapeHtml(event.actor)}</strong><small>${escapeHtml(event.actorId)}</small></td><td>${escapeHtml(event.action)}</td><td>${escapeHtml(event.object)}</td><td><span class="status is-${escapeHtml(event.tone)}">${escapeHtml(event.result)}</span></td></tr>`).join('');
    $('#generic-view').innerHTML = `<div class="generic-heading"><div><h2>事件查询</h2><p>所有影响业务事实的动作都带主体、对象、原因和时间。点击事件可查看完整上下文。</p></div><div class="audit-filters"><input id="audit-search" value="${escapeHtml(state.auditSearch)}" placeholder="搜索主体、动作或对象" aria-label="搜索审计事件"><div class="segmented"><button class="${state.auditFilter === 'all' ? 'is-active' : ''}" data-action="audit-filter" data-audit-filter="all">全部</button><button class="${state.auditFilter === 'decision' ? 'is-active' : ''}" data-action="audit-filter" data-audit-filter="decision">决定</button><button class="${state.auditFilter === 'delivery' ? 'is-active' : ''}" data-action="audit-filter" data-audit-filter="delivery">交付</button></div></div></div><section class="generic-card"><div class="generic-card-body" style="overflow-x:auto"><table class="mini-table"><thead><tr><th>时间</th><th>主体</th><th>动作</th><th>对象</th><th>结果</th></tr></thead><tbody>${rows || '<tr><td colspan="5"><div class="empty-state"><strong>没有匹配的审计事件</strong><p>调整筛选或搜索词，事件本身不会被删除。</p></div></td></tr>'}</tbody></table></div></section>`;
    iconRefresh();
  }

  function renderGeneric(route) {
    if (route === 'workspace') renderWorkspace();
    if (route === 'inbox') renderInbox();
    if (route === 'knowledge') renderKnowledge();
    if (route === 'my-tasks' || route === 'all-tasks') renderTaskCenter(route);
    if (route === 'chat') renderChat();
    if (route === 'automation' || route === 'agents') renderResourcePage(route);
    if (route === 'swarm') renderSwarm();
    if (route === 'usage') renderUsage();
    if (route === 'admin') renderAdmin();
    if (route === 'audit') renderAudit();
    iconRefresh();
  }

  function setView(route, options = {}) {
    const normalized = route === 'project-tasks' ? 'tasks' : route === 'project-sop' ? 'sop' : route;
    if (!pageMeta[normalized]) return;
    state.route = normalized;
    const projectView = normalized === 'tasks' || normalized === 'sop';
    $('#sop-view').style.display = normalized === 'sop' ? 'block' : 'none';
    $('#task-view').classList.toggle('is-visible', normalized === 'tasks');
    $('#generic-view').classList.toggle('is-visible', !projectView);
    $('.project-tabs').style.display = projectView ? 'flex' : 'none';
    $$('.tab').forEach((tab) => tab.classList.toggle('is-active', tab.dataset.tab === normalized));
    $$('.nav-item').forEach((item) => {
      const navRoute = item.dataset.nav === 'project-tasks' ? 'tasks' : item.dataset.nav === 'project-sop' ? 'sop' : item.dataset.nav;
      item.classList.toggle('is-active', navRoute === normalized);
    });
    setHeader(normalized);
    if (normalized === 'sop') {
      renderTemplates();
      renderBuilder();
    } else if (normalized === 'tasks') {
      renderProjectTasks();
    } else {
      renderGeneric(normalized);
    }
    document.body.classList.remove('nav-open');
    $('.mobile-menu').setAttribute('aria-expanded', 'false');
    if (!options.skipHistory) history.replaceState(null, '', `#${normalized}`);
    iconRefresh();
  }

  function openTask(taskId) {
    const task = tasks.find((item) => item.id === Number(taskId)) || tasks[0];
    const statusProfiles = {
      '待补资料': { next: '补齐任务输入', detail: '补齐来源定位和可用素材权利后，继续执行当前 Stage。', evidence: '1 项待补', rights: '2 项有效 · 1 项待确认', revision: '尚未提交', action: '创建补料任务' },
      '执行中': { next: '等待本地运行提交', detail: '本地执行器完成后只提交结构化结果和 Revision，不上传完整 Transcript。', evidence: '已绑定', rights: '当前范围有效', revision: '生成中', action: '查看任务上下文' },
      '待 Gate': { next: '处理当前决定点', detail: '检查变更摘要、Evidence 和 Rights 后，由被指派角色作出明确决定。', evidence: '检查通过', rights: '当前范围有效', revision: '已提交', action: '查看决定上下文' },
      '已交付': { next: '查看交付与结果', detail: '交付已固定 Accepted Revision 和知识快照，可继续记录结果观察。', evidence: '完整', rights: '交付时有效', revision: '已接受', action: '查看交付事实' },
      '待处理': { next: '确认 Brief 和输入', detail: '确认业务目标、输入范围和执行方式后开始第一个 Stage。', evidence: '待绑定', rights: '待检查', revision: '尚未创建', action: '创建补料任务' }
    };
    const profile = statusProfiles[task.status] || statusProfiles['待处理'];
    $('#drawer-title').textContent = task.title;
    $('#drawer-project').textContent = task.project;
    $('#drawer-sop').textContent = task.sop;
    $('#drawer-stage').textContent = task.stage;
    $('#drawer-status').textContent = task.status;
    $('#drawer-objective').textContent = `${task.title}。${task.meta}，结果需要满足当前 SOP 的输入、检查和交付约束。`;
    $('#drawer-next-action').textContent = profile.next;
    $('#drawer-next-detail').textContent = profile.detail;
    $('#drawer-snapshot').textContent = task.status === '待处理' ? '创建 Run 时固定' : '#42 · 有效';
    $('#drawer-knowledge-count').textContent = task.status === '待处理' ? '待绑定' : '3 条';
    $('#drawer-evidence-count').textContent = task.status === '待处理' ? '待绑定' : '23 条';
    $('#drawer-candidate-count').textContent = task.status === '待补资料' ? '2 条' : '0 条';
    $('#drawer-evidence-status').textContent = profile.evidence;
    $('#drawer-rights-status').textContent = profile.rights;
    $('#drawer-revision-status').textContent = profile.revision;
    $('#drawer-executor').textContent = task.executor || '本地 Workspace';
    const primary = $('#drawer-primary-action');
    primary.dataset.action = task.status === '待补资料' || task.status === '待处理' ? 'create-supply-task' : 'quick-nav';
    primary.dataset.route = task.status === '执行中' || task.status === '待 Gate' ? 'chat' : task.status === '已交付' ? 'audit' : '';
    $('span', primary).textContent = profile.action;
    $('#drawer-backdrop').classList.add('is-open');
    iconRefresh();
  }

  function closeDrawer() {
    $('#drawer-backdrop').classList.remove('is-open');
  }

  function setKnowledgeText(id, value) {
    const target = $(`#${id}`);
    if (target) target.textContent = value || '未记录';
  }

  function renderKnowledgeDrawerActions(item) {
    const target = $('#knowledge-drawer-actions');
    if (!target) return;
    if (!item) {
      target.innerHTML = '<button class="button button-secondary" data-action="close-knowledge">关闭</button>';
      return;
    }
    const canAccept = ['Claim', 'FactAssertion', 'Insight'].includes(item.type) && !item.usable && item.status !== 'prohibited';
    const needsEvidence = ['needs_review', 'candidate', 'open', 'source_missing'].includes(item.status);
    const requestButton = needsEvidence ? '<button class="button button-secondary" data-action="request-knowledge-evidence"><i data-lucide="file-plus-2" class="icon"></i>要求补证据</button>' : '';
    const acceptButton = canAccept ? '<button class="button button-primary" data-action="accept-knowledge"><i data-lucide="check" class="icon"></i>接受为知识</button>' : '';
    const taskButton = !canAccept && ['ConflictRecord', 'KnowledgeGap'].includes(item.type) ? '<button class="button button-primary" data-action="new-knowledge-task"><i data-lucide="list-plus" class="icon"></i>创建补料任务</button>' : '';
    target.innerHTML = `${requestButton}${acceptButton}${taskButton}<button class="button button-ghost" data-action="close-knowledge">关闭</button>`;
    iconRefresh();
  }

  function openKnowledge(knowledgeId) {
    const item = knowledgeObjects.find((entry) => entry.id === knowledgeId);
    if (!item) return showToast('找不到该知识对象');
    state.selectedKnowledgeId = item.id;
    state.selectedSourceId = null;
    $('#knowledge-backdrop').classList.add('is-open');
    setKnowledgeText('knowledge-drawer-eyebrow', `${item.type} · ${item.category}`);
    setKnowledgeText('knowledge-drawer-title', item.title);
    setKnowledgeText('knowledge-detail-status', item.statusLabel);
    setKnowledgeText('knowledge-detail-version', item.version);
    setKnowledgeText('knowledge-detail-project', item.project);
    setKnowledgeText('knowledge-detail-owner', item.owner);
    setKnowledgeText('knowledge-detail-type', item.type);
    setKnowledgeText('knowledge-detail-layer', knowledgeLayers.find((layer) => layer.id === item.layer)?.name || item.layer);
    setKnowledgeText('knowledge-detail-summary', item.summary);
    setKnowledgeText('knowledge-detail-source', item.source);
    setKnowledgeText('knowledge-detail-evidence', item.evidence);
    setKnowledgeText('knowledge-detail-relations', item.relations);
    setKnowledgeText('knowledge-detail-usage', item.usedBy);
    renderKnowledgeDrawerActions(item);
    iconRefresh();
  }

  function openSource(sourceId) {
    const source = knowledgeSources.find((entry) => entry.id === sourceId);
    if (!source) return showToast('找不到该来源');
    state.selectedKnowledgeId = null;
    state.selectedSourceId = source.id;
    $('#knowledge-backdrop').classList.add('is-open');
    setKnowledgeText('knowledge-drawer-eyebrow', `${source.type} · Source Record`);
    setKnowledgeText('knowledge-drawer-title', source.title);
    setKnowledgeText('knowledge-detail-status', source.status);
    setKnowledgeText('knowledge-detail-version', source.locator);
    setKnowledgeText('knowledge-detail-project', '来源登记');
    setKnowledgeText('knowledge-detail-owner', '来源负责人');
    setKnowledgeText('knowledge-detail-type', source.type);
    setKnowledgeText('knowledge-detail-layer', '来源与 Evidence');
    setKnowledgeText('knowledge-detail-summary', source.detail);
    setKnowledgeText('knowledge-detail-source', source.digest);
    setKnowledgeText('knowledge-detail-evidence', `${source.objects} 个知识对象`);
    setKnowledgeText('knowledge-detail-relations', 'Source → Evidence → Object');
    setKnowledgeText('knowledge-detail-usage', '可被摄取任务和知识快照引用');
    renderKnowledgeDrawerActions(null);
    iconRefresh();
  }

  function closeKnowledge() {
    $('#knowledge-backdrop').classList.remove('is-open');
    state.selectedKnowledgeId = null;
    state.selectedSourceId = null;
  }

  function acceptKnowledge() {
    const item = knowledgeObjects.find((entry) => entry.id === state.selectedKnowledgeId);
    if (!item) return showToast('没有选中的知识对象');
    if (!['Claim', 'FactAssertion', 'Insight'].includes(item.type) || item.status === 'prohibited') return showToast('该对象不能直接接受为可引用知识');
    item.status = item.type === 'Claim' ? 'approved' : 'verified';
    item.statusLabel = item.type === 'Claim' ? '已批准' : '已验证';
    item.approvalStatus = item.status;
    item.approvalLabel = item.type === 'Claim' ? '保持已批准' : '保持已验证';
    item.tone = 'success';
    item.usable = true;
    item.updated = '刚刚';
    item.evidence = item.evidence.replace('待补', '已确认');
    item.usedBy = '待生成新快照';
    addAudit('knowledge.accepted', item.title, '成功', 'success', `对象状态更新为 ${item.statusLabel}`);
    closeKnowledge();
    if (state.route === 'knowledge') renderKnowledge();
    showToast(`“${item.title}”已接受，生成新知识版本`);
  }

  function requestKnowledgeEvidence() {
    const item = knowledgeObjects.find((entry) => entry.id === state.selectedKnowledgeId);
    if (!item) return showToast('没有选中的知识对象');
    item.evidence = '补证请求已进入输入收集';
    item.updated = '刚刚';
    inboxItems.unshift({ id: Math.max(...inboxItems.map((entry) => entry.id)) + 1, title: `补充“${item.title}”的来源依据`, source: '知识缺口', detail: `${item.summary} 当前证据状态：${item.evidence}。`, next: `关联知识对象：${item.id}`, icon: 'file-plus-2', tone: 'warning', status: 'open' });
    addAudit('knowledge.evidence_requested', item.title, '待补充', 'blue', '请求已进入输入收集，未自动接受对象');
    closeKnowledge();
    if (state.route === 'knowledge') renderKnowledge();
    showToast(`已为“${item.title}”创建补证任务`);
  }

  function submitKnowledgeQuery() {
    const input = $('#knowledge-query-input');
    state.knowledgeQuery = input?.value.trim() || '';
    if (!state.knowledgeQuery) {
      showToast('请先填写业务问题');
      input?.focus();
      return;
    }
    state.knowledgeQueryRan = true;
    renderKnowledge();
    showToast('知识查询完成，已区分可引用、阻断和缺口');
  }

  function syncCreateTaskSOP(preferredType = '') {
    const sopId = $('#create-sop').value;
    const contentTypes = {
      video: ['视频脚本', '短视频文案'],
      knowledge: ['资料与知识', '研究摘要'],
      article: ['公众号文章', '长文章'],
      retro: ['活动复盘', '结果洞察']
    }[sopId] || ['自定义内容'];
    $('#create-content-type').innerHTML = contentTypes.map((type) => `<option ${type === preferredType ? 'selected' : ''}>${type}</option>`).join('');
  }

  function openCreate(prefill = {}) {
    $('#create-task-title').value = prefill.title || '';
    $('#create-inputs').value = prefill.detail || '使用当前 Project 的 Brief、知识快照和有效素材。';
    const preferredSOP = prefill.sop || (((prefill.title || '').includes('知识') || (prefill.title || '').includes('资料')) ? 'knowledge' : 'video');
    $('#create-sop').value = preferredSOP;
    syncCreateTaskSOP(prefill.contentType || '');
    $('#create-backdrop').classList.add('is-open');
    window.setTimeout(() => $('#create-task-title').focus(), 30);
  }

  function closeCreate() {
    $('#create-backdrop').classList.remove('is-open');
    state.pendingInboxId = null;
  }

  function openContextRecord() {
    $('#context-record-summary').value = '';
    $('#context-backdrop').classList.add('is-open');
    window.setTimeout(() => $('#context-record-summary').focus(), 30);
  }

  function closeContextRecord() {
    $('#context-backdrop').classList.remove('is-open');
  }

  function submitContextRecord() {
    const summary = $('#context-record-summary').value.trim();
    if (!summary) {
      showToast('请先填写业务事项摘要');
      $('#context-record-summary').focus();
      return;
    }
    const kind = $('#context-record-type').value;
    const title = summary.length > 28 ? `${summary.slice(0, 28)}…` : summary;
    contextRecords[state.activeContextScope].unshift({ kind, title, detail: summary, author: '林舟', time: '刚刚', tone: kind === '输入补充' ? 'blue' : 'review', icon: kind === '输入补充' ? 'file-input' : 'notebook-pen' });
    closeContextRecord();
    renderChat();
    showToast('业务事项已记录到上下文账本');
  }

  function selectedAdapter() {
    return clientAdapters.find((adapter) => adapter.id === state.importAdapterId) || clientAdapters[0];
  }

  function importScopeLabel(scope = state.importScope) {
    return scope === 'summary' ? '当前 Stage 摘要' : scope === 'full' ? '完整 Transcript' : '客户端选择的片段';
  }

  function importPurposeLabel(purpose = state.importPurpose) {
    return ({ handoff: '任务交接', diagnosis: '失败诊断', knowledge: '知识提取候选', audit: '审计说明' })[purpose] || '任务交接';
  }

  function renderImportAdapters() {
    const target = $('#adapter-list');
    if (!target) return;
    target.innerHTML = clientAdapters.map((adapter) => {
      const selected = adapter.id === state.importAdapterId;
      const capabilityLabels = [
        adapter.supports.summary ? 'Stage 摘要' : '',
        adapter.supports.selectedTurns ? '选择性片段' : '',
        adapter.supports.fullTranscript ? '完整 Transcript' : ''
      ].filter(Boolean);
      return `<button class="adapter-card ${selected ? 'is-selected' : ''}" data-action="select-import-adapter" data-adapter-id="${adapter.id}" aria-pressed="${selected}">
        <span class="adapter-icon"><i data-lucide="${adapter.icon}" class="icon"></i></span>
        <span class="adapter-copy"><strong>${adapter.name}</strong><small>${adapter.description}</small><span class="adapter-meta"><span class="status ${adapter.status === 'connected' ? 'is-success' : 'is-blue'}">${adapter.statusLabel}</span><span>v${adapter.version}</span></span><span class="capability-list">${capabilityLabels.map((label) => `<span class="tag">${label}</span>`).join('')}</span></span>
        <span class="adapter-check"><i data-lucide="${selected ? 'circle-check' : 'circle'}" class="icon"></i></span>
      </button>`;
    }).join('');
    iconRefresh();
  }

  function updateImportPreview() {
    const adapter = selectedAdapter();
    const scope = state.importScope;
    const fullSelected = scope === 'full';
    const fullSupported = adapter.supports.fullTranscript;
    const consent = $('#import-full-consent');
    const redact = $('#import-redact');
    const submit = $('[data-action="submit-import"]');
    const preview = $('.import-preview');
    if (!preview) return;
    if (consent) {
      consent.disabled = !fullSelected;
      consent.closest('.check-item').classList.toggle('is-disabled', !fullSelected);
      if (!fullSelected) consent.checked = false;
    }
    if (submit) {
      const invalid = !fullSupported && fullSelected || !redact?.checked || (fullSelected && !consent?.checked);
      submit.disabled = Boolean(invalid);
      submit.title = !fullSupported && fullSelected ? '当前客户端不支持完整 Transcript' : !redact?.checked ? '必须由客户端先完成脱敏' : (fullSelected && !consent?.checked ? '完整 Transcript 需要明确授权' : '请求客户端导出');
    }
    const capability = fullSelected && !fullSupported ? '当前适配器不支持完整 Transcript，请改选摘要或片段。' : `客户端能力：${adapter.supports.summary ? '摘要' : ''}${adapter.supports.selectedTurns ? ' · 选择性片段' : ''}${adapter.supports.fullTranscript ? ' · 完整 Transcript' : ''}`;
    preview.innerHTML = `<span class="eyebrow">客户端导出请求</span><strong>${state.importRequested ? 'ConversationImport 已创建' : 'Web 不解析本地对话'}</strong><p>${state.importRequested ? `请求 ${state.importRequestId} · ${adapter.name} · ${importScopeLabel()} · ${importPurposeLabel()} · 等待客户端确认` : `${adapter.format} · ${capability}`}</p><small>${state.importRequested ? '原始 Transcript 仍保留在本地；只有客户端导出的 ConversationBundle 会进入任务上下文。' : '提交后由本地客户端选择会话、完成脱敏并生成 ConversationBundle，Web 只接收结构化导入包。'}</small>`;
  }

  function openImport() {
    state.importRequested = false;
    state.importRequestId = null;
    $('#import-backdrop').classList.add('is-open');
    renderImportAdapters();
    updateImportPreview();
    window.setTimeout(() => $('.adapter-card.is-selected')?.focus(), 30);
  }

  function closeImport() {
    $('#import-backdrop').classList.remove('is-open');
  }

  function operationDefinition(kind, entityId) {
    const pack = knowledgePacks.find((item) => item.id === entityId);
    const role = tenantRoles.find((item) => item.id === entityId);
    const node = workspaceNodes.find((item) => item.id === entityId);
    const sop = sopRegistry.find((item) => item.id === entityId);
    const auditEvent = auditEvents.find((item) => item.id === entityId);
    const sourceOptions = knowledgeSources.map((source) => `<option value="${escapeHtml(source.id)}">${escapeHtml(source.title)}</option>`).join('');
    const definitions = {
      'project-settings': {
        eyebrow: 'Project Settings', title: '项目设置', description: '项目显式绑定 Environment、SOP 和交付约束。修改只影响未来任务，历史 TaskRun 保持原 digest。', submit: '保存项目',
        body: `<div class="form-row"><label class="field"><span>项目名称</span><input id="operation-project-name" value="${escapeHtml(projectConfig.name)}"></label><label class="field"><span>负责人</span><select id="operation-project-owner"><option ${projectConfig.owner === '林舟' ? 'selected' : ''}>林舟</option><option ${projectConfig.owner === '周宁' ? 'selected' : ''}>周宁</option><option ${projectConfig.owner === '陈璐' ? 'selected' : ''}>陈璐</option></select></label></div><div class="form-row"><label class="field"><span>Environment</span><select id="operation-project-environment"><option>${escapeHtml(projectConfig.environment)}</option><option>试验环境</option></select></label><label class="field"><span>未来任务使用的 SOP</span><select id="operation-project-sop">${sopRegistry.filter((item) => item.status === 'published').map((item) => `<option value="${item.id}" ${item.name === projectConfig.sop ? 'selected' : ''}>${escapeHtml(item.name)} · ${escapeHtml(item.version)}</option>`).join('')}</select></label></div><div class="form-row"><label class="field"><span>默认风险级别</span><select id="operation-project-risk"><option ${projectConfig.risk === '内部低风险' ? 'selected' : ''}>内部低风险</option><option ${projectConfig.risk === '外部营销' ? 'selected' : ''}>外部营销</option><option ${projectConfig.risk === '需要客户确认' ? 'selected' : ''}>需要客户确认</option></select></label><label class="field"><span>交付配置</span><select id="operation-project-delivery"><option>Workspace 交付包</option><option>客户确认后交付</option></select></label></div><div class="notice" style="margin:0"><i data-lucide="history" class="icon"></i><div>当前 8 个历史任务继续使用原 SOP Version 和 digest；保存后只改变新任务默认绑定。</div></div>`
      },
      'create-rule': {
        eyebrow: 'Local Rule', title: '新建本地规则', description: '规则在本地 Workspace 触发，只提交结构化运行结果。', submit: '创建规则',
        body: '<div class="form-row"><label class="field"><span>规则名称</span><input id="operation-rule-name" value="新内容 Brief 完整性检查"></label><label class="field"><span>触发时机</span><select id="operation-rule-trigger"><option>Task 创建</option><option>Stage 开始</option><option>Revision 提交</option><option>每日计划</option></select></label></div><div class="form-row"><label class="field"><span>关联能力</span><select id="operation-rule-capability"><option>Brief Schema 检查</option><option>品牌规则检查</option><option>Rights 检查</option></select></label><label class="field"><span>失败行为</span><select id="operation-rule-failure"><option>记录并阻断</option><option>记录并提醒</option></select></label></div><div class="notice" style="margin:0"><i data-lucide="lock" class="icon"></i><div>原型不采集本机路径或命令正文。真实产品由客户端保存执行细节，Web 只管理可引用能力和策略。</div></div>'
      },
      'create-executor': {
        eyebrow: 'CLI Executor', title: '添加 CLI 执行器', description: '为本地 CLI 配置可领取的 Stage 和能力边界。', submit: '添加配置',
        body: '<div class="form-row"><label class="field"><span>配置名称</span><input id="operation-executor-name" value="内容生产 CLI"></label><label class="field"><span>客户端类型</span><select id="operation-executor-type"><option>Codex CLI</option><option>Claude Code CLI</option><option>自定义 Workspace Adapter</option></select></label></div><div class="operation-checks"><label class="check-item"><input type="checkbox" checked><span><strong>知识整理</strong><small>生成 Evidence 与知识候选。</small></span></label><label class="check-item"><input type="checkbox" checked><span><strong>内容生产</strong><small>生成带引用的 Revision。</small></span></label><label class="check-item"><input type="checkbox"><span><strong>确定性检查</strong><small>执行 Schema、Claim 和 Rights。</small></span></label><label class="check-item"><input type="checkbox"><span><strong>交付组装</strong><small>只处理已接受快照。</small></span></label></div>'
      },
      'connect-workspace': {
        eyebrow: 'Workspace Node', title: '连接本地 Workspace', description: '生成一次性连接请求，本地客户端确认后才会注册节点。', submit: '生成连接请求',
        body: '<div class="form-row"><label class="field"><span>节点名称</span><input id="operation-workspace-name" value="内容团队工作站"></label><label class="field"><span>并行执行槽</span><input id="operation-workspace-slots" type="number" min="1" max="8" value="2"></label></div><div class="form-row"><label class="field"><span>领取范围</span><select id="operation-workspace-scope"><option>当前 Environment</option><option>当前 Project</option></select></label><label class="field"><span>确认方式</span><select><option>本地客户端逐次确认</option><option>受信规则自动领取</option></select></label></div><div class="notice" style="margin:0"><i data-lucide="shield-check" class="icon"></i><div>连接请求不会让 Web 浏览本机目录。客户端只上报节点身份、能力、心跳和结构化运行状态。</div></div>'
      },
      'workspace-settings': {
        eyebrow: 'Workspace Settings', title: '本地 Workspace 配置', description: '调整节点容量和领取范围，不改变任务业务状态。', submit: '保存节点',
        body: `<div class="form-row"><label class="field"><span>节点名称</span><input id="operation-node-name" value="${escapeHtml(workspaceNodes[0].name)}"></label><label class="field"><span>并行执行槽</span><input id="operation-node-slots" type="number" min="1" max="8" value="${workspaceNodes[0].slots}"></label></div><div class="operation-checks"><label class="check-item"><input type="checkbox" checked><span><strong>允许领取任务</strong><small>仅领取当前 Environment 已发布 SOP 的 Stage。</small></span></label><label class="check-item"><input type="checkbox" checked><span><strong>保留本地 Transcript</strong><small>除非显式请求客户端导出，否则不上传。</small></span></label></div>`
      },
      'register-source': {
        eyebrow: 'Source Registry', title: '登记知识来源', description: '先登记来源身份和定位，再由本地摄取任务生成候选对象。', submit: '登记来源',
        body: '<div class="form-row"><label class="field"><span>来源名称</span><input id="operation-source-name" value="新品产品说明书"></label><label class="field"><span>来源类型</span><select id="operation-source-type"><option>Workspace 文件</option><option>受控文档</option><option>外部来源</option><option>ConversationBundle</option></select></label></div><div class="form-row"><label class="field"><span>可复核定位</span><input id="operation-source-locator" value="page=1-8"></label><label class="field"><span>权利状态</span><select id="operation-source-rights"><option>已确认可用于内部知识</option><option>仅研究可用</option><option>待确认</option></select></label></div><label class="field"><span>来源说明</span><textarea id="operation-source-detail">由产品负责人提供，需提取规格、场景和使用边界。</textarea></label>'
      },
      'create-ingest': {
        eyebrow: 'Ingest Task', title: '创建本地摄取任务', description: '摄取只产生 Evidence 和候选对象，不直接写入可引用知识。', submit: '创建摄取任务',
        body: `<div class="form-row"><label class="field"><span>选择来源</span><select id="operation-ingest-source">${sourceOptions}</select></label><label class="field"><span>候选对象类型</span><select id="operation-ingest-type"><option>自动识别并分类</option><option>FactAssertion</option><option>Claim</option><option>RightsRecord</option></select></label></div><div class="operation-checks"><label class="check-item"><input type="checkbox" checked><span><strong>生成精确 Evidence 定位</strong><small>保留页码、单元格或 Bundle block。</small></span></label><label class="check-item"><input type="checkbox" checked><span><strong>完成后进入待审队列</strong><small>不自动接受为正式知识。</small></span></label></div>`
      },
      'create-pack': {
        eyebrow: 'Knowledge Pack', title: '新建知识包', description: '显式选择用途和对象范围，发布时才生成不可变快照。', submit: '创建草稿',
        body: '<div class="form-row"><label class="field"><span>知识包名称</span><input id="operation-pack-name" value="新品营销知识包"></label><label class="field"><span>业务用途</span><select id="operation-pack-purpose"><option>短视频生产</option><option>文章协作</option><option>资料与知识建设</option></select></label></div><div class="operation-checks"><label class="check-item"><input type="checkbox" checked><span><strong>产品与规格</strong><small>31 个对象，4 个缺口。</small></span></label><label class="check-item"><input type="checkbox" checked><span><strong>身份与品牌</strong><small>24 个对象，1 个缺口。</small></span></label><label class="check-item"><input type="checkbox" checked><span><strong>表达与主张</strong><small>16 个对象，6 个缺口。</small></span></label><label class="check-item"><input type="checkbox"><span><strong>市场与受众</strong><small>18 个对象，5 个缺口。</small></span></label></div>'
      },
      'impact-analysis': {
        eyebrow: 'Impact Analysis', title: '知识变更影响分析', description: '预览候选对象进入新快照后会影响哪些未来运行。', submit: '关闭', readonly: true,
        body: '<div class="impact-summary"><div><strong>6</strong><small>候选新增</small></div><div><strong>3</strong><small>未来 Task 受益</small></div><div><strong>0</strong><small>历史 TaskRun 被改写</small></div></div><div class="operation-list"><div class="operation-row"><strong>新品规格表 v3</strong><span>新增 6 个 FactAssertion 候选</span><small>需要验证</small></div><div class="operation-row"><strong>当前快照 #42</strong><span>保持不可变，不自动替换</span><small>无影响</small></div><div class="operation-row"><strong>下一个 TaskRun</strong><span>发布新快照后可显式选择</span><small>可升级</small></div></div>'
      },
      'pack-version': {
        eyebrow: 'Pack Version', title: pack?.name || '知识包版本', description: '查看当前对象集合、快照和 digest。', submit: '关闭', readonly: true,
        body: `<div class="impact-summary"><div><strong>${escapeHtml(pack?.version || '-')}</strong><small>当前版本</small></div><div><strong>${pack?.objects || 0}</strong><small>对象数量</small></div><div><strong>${escapeHtml(pack?.snapshot || '-')}</strong><small>绑定快照</small></div></div><div class="operation-list"><div class="operation-row"><strong>对象集合</strong><span>${escapeHtml(pack?.layers || '未记录')}，状态必须为可用</span><small>已校验</small></div><div class="operation-row"><strong>版本 digest</strong><span>sha256:9c24…bf18</span><small>不可变</small></div></div>`
      },
      'pack-usage': {
        eyebrow: 'Usage Scope', title: `${pack?.name || '知识包'}的使用范围`, description: '历史运行固定旧快照，新运行显式绑定当前快照。', submit: '关闭', readonly: true,
        body: `<div class="impact-summary"><div><strong>${pack?.tasks || 0}</strong><small>使用中的 Task</small></div><div><strong>${escapeHtml(pack?.snapshot || '-')}</strong><small>当前快照</small></div><div><strong>0</strong><small>隐式替换</small></div></div><div class="operation-list"><div class="operation-row"><strong>新品内容生产</strong><span>Task #1、#2、#5</span><small>显式绑定</small></div><div class="operation-row"><strong>品牌知识建设</strong><span>未来任务可选</span><small>未绑定</small></div></div>`
      },
      'create-gate': {
        eyebrow: 'Gate Policy', title: '新建 Gate 策略', description: 'Gate 是条件触发的决定点，不是每条 SOP 固定审批。', submit: '创建策略',
        body: '<div class="form-row"><label class="field"><span>策略名称</span><input id="operation-gate-name" value="高风险外部主张客户确认"></label><label class="field"><span>Gate 类型</span><select id="operation-gate-type"><option>客户确认</option><option>内部审核</option><option>确定性检查</option></select></label></div><div class="form-row"><label class="field"><span>触发条件</span><select id="operation-gate-condition"><option>命中高风险 Claim</option><option>Rights 状态不完整</option><option>指定内容类型</option></select></label><label class="field"><span>负责角色</span><select id="operation-gate-role"><option>客户决定人</option><option>流程负责人</option><option>审核人</option></select></label></div><div class="switch-row"><div><strong>未通过时阻断下一 Stage</strong><small>关闭时只生成提醒和审计事件。</small></div><button class="toggle is-on" data-action="simple-toggle" role="switch" aria-checked="true"></button></div>'
      },
      'import-capability': {
        eyebrow: 'Capability Contract', title: '导入能力契约', description: '注册 SOP 和本地执行器可以引用的能力边界。', submit: '导入契约',
        body: '<div class="form-row"><label class="field"><span>能力名称</span><input id="operation-capability-name" value="内容事实核验"></label><label class="field"><span>契约版本</span><input id="operation-capability-version" value="1.0.0"></label></div><div class="form-row"><label class="field"><span>执行边界</span><select id="operation-capability-boundary"><option>本地 Workspace</option><option>受控服务端</option></select></label><label class="field"><span>输出类型</span><select><option>结构化检查结果</option><option>Revision 候选</option><option>Knowledge Candidate</option></select></label></div>'
      },
      'create-role': {
        eyebrow: 'Access Control', title: '新建自定义角色', description: '角色按动作和范围授权，不能仅靠页面可见性代替权限检查。', submit: '创建角色',
        body: '<div class="form-row"><label class="field"><span>角色名称</span><input id="operation-role-name" value="内容质检负责人"></label><label class="field"><span>作用范围</span><select id="operation-role-scope"><option>Project</option><option>Environment</option><option>Assigned Task</option></select></label></div><div class="operation-checks"><label class="check-item"><input type="checkbox" checked><span><strong>查看 Task 与 Revision</strong><small>只读业务内容和治理摘要。</small></span></label><label class="check-item"><input type="checkbox" checked><span><strong>提交检查结果</strong><small>不能接受 Revision 或发布 SOP。</small></span></label><label class="check-item"><input type="checkbox"><span><strong>处理内部 Gate</strong><small>仅处理明确指派的决定。</small></span></label><label class="check-item"><input type="checkbox"><span><strong>导出审计事件</strong><small>导出动作本身也被记录。</small></span></label></div>'
      },
      'role-detail': {
        eyebrow: 'Role Detail', title: role?.name || '角色详情', description: '查看成员数量、权限和作用范围。', submit: '关闭', readonly: true,
        body: `<div class="impact-summary"><div><strong>${role?.members || 0}</strong><small>成员</small></div><div><strong>${escapeHtml(role?.scope || '-')}</strong><small>作用范围</small></div><div><strong>已审计</strong><small>权限变更</small></div></div><div class="operation-list"><div class="operation-row"><strong>关键权限</strong><span>${escapeHtml(role?.permissions || '未记录')}</span><small>服务端校验</small></div></div>`
      },
      'node-detail': {
        eyebrow: 'Execution Node', title: node?.name || '执行节点', description: '查看节点能力、心跳和执行槽。', submit: '保存节点',
        body: `<div class="form-row"><label class="field"><span>显示名称</span><input id="operation-detail-node-name" value="${escapeHtml(node?.name || '')}"></label><label class="field"><span>执行槽</span><input id="operation-detail-node-slots" type="number" min="1" max="8" value="${node?.slots || 1}"></label></div><div class="operation-list"><div class="operation-row"><strong>节点类型</strong><span>${escapeHtml(node?.type || '-')}</span><small>${escapeHtml(node?.statusLabel || '-')}</small></div><div class="operation-row"><strong>最近心跳</strong><span>${escapeHtml(node?.heartbeat || '-')}</span><small>连接正常</small></div></div>`
      },
      'new-sop-version': {
        eyebrow: 'SOP Version', title: `基于 ${sop?.name || 'SOP'} 创建新版本`, description: '复制当前已发布版本为草稿。历史任务继续使用原版本和 digest。', submit: '创建草稿',
        body: `<div class="impact-summary"><div><strong>${escapeHtml(sop?.version || '-')}</strong><small>来源版本</small></div><div><strong>${sop?.tasks || 0}</strong><small>历史任务</small></div><div><strong>0</strong><small>被自动迁移</small></div></div><div class="form-row"><label class="field"><span>变更类型</span><select id="operation-sop-change"><option>小版本：调整配置和检查</option><option>大版本：改变输入输出契约</option></select></label><label class="field"><span>变更原因</span><input id="operation-sop-reason" value="适配当前 Environment 的业务实践"></label></div>`
      },
      'legacy-upgrade': {
        eyebrow: 'Compatibility Upgrade', title: '旧版流程升级预览', description: '扫描只读旧定义并生成候选新版本，不覆盖历史事实。', submit: '生成兼容新版本',
        body: '<div class="impact-summary"><div><strong>1</strong><small>精确匹配</small></div><div><strong>0</strong><small>字段冲突</small></div><div><strong>18</strong><small>历史任务保持不变</small></div></div><div class="operation-section"><header><div><h3>识别结果</h3><p>旧版短视频流程的 Stage、检查和输出契约可确定性映射。</p></div><span class="status is-success">可升级</span></header><div class="operation-list"><div class="operation-row"><strong>Brief / Context</strong><span>需求 Brief + 知识与证据</span><small>完整映射</small></div><div class="operation-row"><strong>Strategy / Compile</strong><span>受众与策略 + 脚本创作</span><small>完整映射</small></div><div class="operation-row"><strong>Review</strong><span>转为可配置 Gate，不强制审批</span><small>语义保持</small></div><div class="operation-row"><strong>Delivery</strong><span>Accepted Revision + 交付包</span><small>完整映射</small></div></div></div><div class="notice" style="margin:0"><i data-lucide="shield-check" class="icon"></i><div>同名但结构不同的企业自定义流程不会被覆盖，只会列为需要人工处理的候选。</div></div>'
      },
      'audit-detail': {
        eyebrow: 'Audit Event', title: auditEvent?.action || '审计事件', description: '事件记录主体、对象、结果、原因和关联事实。', submit: '关闭', readonly: true,
        body: `<div class="impact-summary"><div><strong>${escapeHtml(auditEvent?.time || '-')}</strong><small>发生时间</small></div><div><strong>${escapeHtml(auditEvent?.result || '-')}</strong><small>结果</small></div><div><strong>${escapeHtml(auditEvent?.category || '-')}</strong><small>事件类别</small></div></div><div class="operation-list"><div class="operation-row"><strong>主体</strong><span>${escapeHtml(auditEvent?.actor || '-')}</span><small>${escapeHtml(auditEvent?.actorId || '-')}</small></div><div class="operation-row"><strong>对象</strong><span>${escapeHtml(auditEvent?.object || '-')}</span><small>稳定 ID</small></div><div class="operation-row"><strong>原因</strong><span>${escapeHtml(auditEvent?.reason || '-')}</span><small>不可变记录</small></div></div>`
      }
    };
    return definitions[kind] || { eyebrow: 'Operation', title: '未识别操作', description: '该操作尚未注册。', submit: '关闭', readonly: true, body: '<div class="empty-state"><strong>无法打开此操作</strong><p>请返回上一页重试。</p></div>' };
  }

  function openOperation(kind, entityId = null) {
    const definition = operationDefinition(kind, entityId);
    state.operationKind = kind;
    state.operationContext = entityId;
    $('#operation-eyebrow').textContent = definition.eyebrow;
    $('#operation-title').textContent = definition.title;
    $('#operation-description').textContent = definition.description;
    $('#operation-body').innerHTML = definition.body;
    $('#operation-submit-label').textContent = definition.submit;
    const cancel = $('#operation-footer [data-action="close-operation"]');
    if (cancel) cancel.style.display = definition.readonly ? 'none' : 'inline-flex';
    $('#operation-backdrop').classList.add('is-open');
    iconRefresh();
    window.setTimeout(() => $('#operation-body input, #operation-body select, #operation-footer .button-primary')?.focus(), 30);
  }

  function closeOperation() {
    $('#operation-backdrop').classList.remove('is-open');
    state.operationKind = null;
    state.operationContext = null;
  }

  function requiredOperationValue(selector, message) {
    const input = $(selector);
    const value = input?.value.trim() || '';
    if (!value) {
      showToast(message);
      input?.focus();
      return null;
    }
    return value;
  }

  function rerenderCurrentView() {
    if (state.route === 'sop') {
      renderTemplates();
      renderBuilder();
    } else if (state.route === 'tasks') {
      renderProjectTasks();
    } else {
      renderGeneric(state.route);
    }
    setHeader(state.route);
  }

  function submitOperation() {
    const kind = state.operationKind;
    const entityId = state.operationContext;
    if (['impact-analysis', 'pack-version', 'pack-usage', 'role-detail', 'audit-detail'].includes(kind)) {
      closeOperation();
      return;
    }
    if (kind === 'project-settings') {
      const name = requiredOperationValue('#operation-project-name', '请填写项目名称');
      if (!name) return;
      const selectedSOP = sopRegistry.find((item) => item.id === $('#operation-project-sop').value);
      projectConfig.name = name;
      projectConfig.owner = $('#operation-project-owner').value;
      projectConfig.environment = $('#operation-project-environment').value;
      projectConfig.sop = selectedSOP?.name || projectConfig.sop;
      projectConfig.sopVersion = selectedSOP?.version || projectConfig.sopVersion;
      projectConfig.risk = $('#operation-project-risk').value;
      projectConfig.deliveryProfile = $('#operation-project-delivery').value;
      $('.project-title strong').textContent = projectConfig.name;
      addAudit('project.sop_bound', projectConfig.name, '成功', 'success', `未来任务改绑为 ${projectConfig.sop} ${projectConfig.sopVersion}`);
    }
    if (kind === 'create-rule') {
      const name = requiredOperationValue('#operation-rule-name', '请填写规则名称');
      if (!name) return;
      resources.automation.push({ id: `auto-${Date.now()}`, name, detail: `${$('#operation-rule-capability').value}；失败时${$('#operation-rule-failure').value}。`, meta: `触发：${$('#operation-rule-trigger').value}`, enabled: true });
      addAudit('local_rule.created', name, '成功');
    }
    if (kind === 'create-executor') {
      const name = requiredOperationValue('#operation-executor-name', '请填写配置名称');
      if (!name) return;
      const type = $('#operation-executor-type').value;
      resources.agents.push({ id: `agent-${Date.now()}`, name, detail: `${type} 的本地执行配置，仅领取已授权 Stage。`, meta: '最近运行：尚未运行', enabled: true });
      workspaceNodes.push({ id: `node-${Date.now()}`, name, type: 'CLI 配置', heartbeat: '刚刚', status: 'ready', statusLabel: '可用', slots: 1 });
      addAudit('executor.created', name, '成功');
    }
    if (kind === 'connect-workspace') {
      const name = requiredOperationValue('#operation-workspace-name', '请填写节点名称');
      if (!name) return;
      workspaceNodes.push({ id: `node-${Date.now()}`, name, type: 'Workspace', heartbeat: '等待客户端确认', status: 'pending', statusLabel: '待确认', slots: Number($('#operation-workspace-slots').value) || 1 });
      addAudit('workspace.connection_requested', name, '待确认', 'blue', '已生成一次性连接请求');
    }
    if (kind === 'workspace-settings') {
      const name = requiredOperationValue('#operation-node-name', '请填写节点名称');
      if (!name) return;
      workspaceNodes[0].name = name;
      workspaceNodes[0].slots = Number($('#operation-node-slots').value) || 1;
      addAudit('workspace.updated', name, '成功');
    }
    if (kind === 'register-source') {
      const name = requiredOperationValue('#operation-source-name', '请填写来源名称');
      const locator = requiredOperationValue('#operation-source-locator', '请填写可复核定位');
      if (!name || !locator) return;
      knowledgeSources.unshift({ id: `source:${Date.now()}`, title: name, type: $('#operation-source-type').value, detail: $('#operation-source-detail').value.trim() || '等待本地摄取', locator, digest: `sha256:${String(Date.now()).slice(-6)}…local`, objects: 0, status: $('#operation-source-rights').value.includes('待确认') ? '待确认' : '已登记', tone: $('#operation-source-rights').value.includes('待确认') ? 'warning' : 'blue', updated: '刚刚' });
      addAudit('source.registered', name, '成功');
    }
    if (kind === 'create-ingest') {
      const source = knowledgeSources.find((item) => item.id === $('#operation-ingest-source').value);
      tasks.unshift({ id: Math.max(...tasks.map((task) => task.id)) + 1, title: `摄取：${source?.title || '知识来源'}`, meta: `${$('#operation-ingest-type').value} · 生成候选`, project: projectConfig.name, sop: '资料与知识建设 · v1.2', stage: '资料与来源', status: '待处理', tone: 'muted', owner: '林舟', executor: '本地 Workspace', updated: '刚刚' });
      addAudit('ingest_task.created', source?.title || '知识来源', '成功');
    }
    if (kind === 'create-pack') {
      const name = requiredOperationValue('#operation-pack-name', '请填写知识包名称');
      if (!name) return;
      knowledgePacks.unshift({ id: `pack:${Date.now()}`, name, version: '草稿', status: 'draft', statusLabel: '草稿', tone: 'warning', snapshot: '未生成', layers: '3/7 层', objects: 71, tasks: 0, updated: '刚刚' });
      addAudit('knowledge_pack.created', name, '成功');
    }
    if (kind === 'create-gate') {
      const name = requiredOperationValue('#operation-gate-name', '请填写 Gate 策略名称');
      if (!name) return;
      gatePolicies.push({ id: `gate-${Date.now()}`, name, detail: `${$('#operation-gate-condition').value}时触发 ${$('#operation-gate-type').value}，负责人：${$('#operation-gate-role').value}。`, enabled: true });
      addAudit('gate_policy.created', name, '成功');
    }
    if (kind === 'import-capability') {
      const name = requiredOperationValue('#operation-capability-name', '请填写能力名称');
      if (!name) return;
      resources.capabilities.push({ id: `cap-${Date.now()}`, name, detail: `${$('#operation-capability-boundary').value}执行，契约 v${$('#operation-capability-version').value}`, meta: '已导入契约', enabled: false });
      addAudit('capability.imported', name, '成功');
    }
    if (kind === 'create-role') {
      const name = requiredOperationValue('#operation-role-name', '请填写角色名称');
      if (!name) return;
      tenantRoles.push({ id: `role-${Date.now()}`, name, members: 0, permissions: '查看 Task 与 Revision、提交检查结果', scope: $('#operation-role-scope').value });
      addAudit('role.created', name, '成功');
    }
    if (kind === 'node-detail') {
      const node = workspaceNodes.find((item) => item.id === entityId);
      const name = requiredOperationValue('#operation-detail-node-name', '请填写节点名称');
      if (!node || !name) return;
      node.name = name;
      node.slots = Number($('#operation-detail-node-slots').value) || 1;
      addAudit('execution_node.updated', name, '成功');
    }
    if (kind === 'new-sop-version') {
      const sop = sopRegistry.find((item) => item.id === entityId);
      if (!sop) return;
      const parts = sop.version.replace('v', '').split('.').map(Number);
      sop.version = `v${parts[0] || 1}.${(parts[1] || 0) + 1}`;
      sop.status = 'draft';
      sop.statusLabel = '草稿';
      addAudit('sop.version_created', `${sop.name} ${sop.version}`, '成功', 'success', $('#operation-sop-reason').value);
    }
    if (kind === 'legacy-upgrade') {
      const video = sopRegistry.find((item) => item.id === 'video');
      if (video) video.version = 'v1.1';
      state.legacyUpgradeStatus = 'completed';
      addAudit('sop.compatibility_version_created', '短视频生产 v1.1', '成功', 'success', '精确结构匹配；历史任务保持原 digest');
    }
    closeOperation();
    rerenderCurrentView();
    showToast('操作已保存，并写入当前原型状态');
  }

  function exportData(kind) {
    const rows = kind === 'audit'
      ? auditEvents.map((event) => [event.time, event.actor, event.action, event.object, event.result, event.reason])
      : [['范围', state.usageRange], ['任务运行', 126], ['能力调用', 842], ['平均耗时', '2m 18s'], ['估算成本', '386 CNY']];
    const header = kind === 'audit' ? ['time', 'actor', 'action', 'object', 'result', 'reason'] : ['metric', 'value'];
    const csv = [header, ...rows].map((row) => row.map((value) => `"${String(value).replace(/"/g, '""')}"`).join(',')).join('\n');
    const blob = new Blob([`\ufeff${csv}`], { type: 'text/csv;charset=utf-8' });
    const link = document.createElement('a');
    link.href = URL.createObjectURL(blob);
    link.download = `${kind === 'audit' ? 'audit-events' : 'usage-detail'}.csv`;
    link.click();
    URL.revokeObjectURL(link.href);
    state.lastExportName = link.download;
    addAudit(`${kind}.exported`, link.download, '成功');
    showToast(`${link.download} 已生成`);
  }

  function submitImport() {
    const adapter = selectedAdapter();
    const scope = state.importScope;
    const redact = $('#import-redact');
    const consent = $('#import-full-consent');
    if (!adapter) return showToast('请先选择本地客户端');
    if (!redact?.checked) return showToast('必须由客户端先完成脱敏');
    if (scope === 'full' && !adapter.supports.fullTranscript) return showToast('当前客户端不支持完整 Transcript');
    if (scope === 'full' && !consent?.checked) return showToast('完整 Transcript 需要明确授权');
    state.importRequestId = `ci_${String(Date.now()).slice(-8)}`;
    state.importRequested = true;
    updateImportPreview();
    showToast(`已请求 ${adapter.name} 导出，等待客户端确认`);
  }

  function submitTask() {
    const title = $('#create-task-title').value.trim();
    if (!title) {
      showToast('请先填写任务目标');
      $('#create-task-title').focus();
      return;
    }
    const selectedSOP = sopRegistry.find((item) => item.id === $('#create-sop').value);
    if (!selectedSOP || selectedSOP.status !== 'published') {
      showToast('只能使用当前 Environment 中已发布的 SOP Version');
      return;
    }
    tasks.unshift({
      id: Math.max(...tasks.map((task) => task.id)) + 1,
      title,
      meta: `${$('#create-content-type').value} · 输入待确认`,
      project: $('#create-project').value,
      sop: `${selectedSOP.name} · ${selectedSOP.version}`,
      stage: '需求 Brief',
      status: '待处理',
      tone: 'muted',
      owner: $('#create-owner').value,
      executor: $('#create-executor').value,
      updated: '刚刚'
    });
    selectedSOP.tasks += 1;
    addAudit('task.created', title, '成功', 'success', `${$('#create-environment').value} · ${selectedSOP.name} ${selectedSOP.version}`);
    if (state.pendingInboxId) {
      const inboxItem = inboxItems.find((item) => item.id === state.pendingInboxId);
      if (inboxItem) inboxItem.status = 'converted';
    }
    closeCreate();
    setView('tasks');
    showToast(`任务“${title}”已创建`);
  }

  function toggleResource(type, id) {
    const item = resources[type].find((resource) => resource.id === id);
    if (!item) return;
    item.enabled = !item.enabled;
    addAudit('capability.toggled', item.name, item.enabled ? '已启用' : '已停用', item.enabled ? 'success' : 'blue');
    if (state.route === 'admin') renderAdmin();
    else renderResourcePage(type);
    showToast(`${item.name}已${item.enabled ? '启用' : '停用'}`);
  }

  function handleAction(button) {
    const action = button.dataset.action;
    if (action === 'open-operation') openOperation(button.dataset.operation, button.dataset.entityId || null);
    if (action === 'project-settings') openOperation('project-settings');
    if (action === 'close-operation') closeOperation();
    if (action === 'submit-operation') submitOperation();
    if (action === 'export-data') exportData(button.dataset.export);
    if (action === 'new-task') openCreate({ title: '生成 10 条新品短视频脚本' });
    if (action === 'close-create') closeCreate();
    if (action === 'submit-task') submitTask();
    if (action === 'close-drawer') closeDrawer();
    if (action === 'open-knowledge') openKnowledge(button.dataset.knowledgeId);
    if (action === 'close-knowledge') closeKnowledge();
    if (action === 'accept-knowledge') acceptKnowledge();
    if (action === 'request-knowledge-evidence') requestKnowledgeEvidence();
    if (action === 'source-detail') openSource(button.dataset.sourceId);
    if (action === 'knowledge-tab') {
      state.knowledgeTab = button.dataset.knowledgeTab;
      renderKnowledge();
    }
    if (action === 'knowledge-layer') {
      state.knowledgeLayer = button.dataset.knowledgeLayer || 'all';
      state.knowledgeTab = 'objects';
      renderKnowledge();
    }
    if (action === 'knowledge-search') renderKnowledge();
    if (action === 'submit-knowledge-query') submitKnowledgeQuery();
    if (action === 'knowledge-lint') {
      state.knowledgeTab = 'review';
      renderKnowledge();
      addAudit('knowledge.lint_completed', '当前知识库', '发现待处理项', 'blue', '1 个冲突、3 个缺口；未自动改变状态');
      showToast('知识校验完成，已打开待审与冲突队列');
    }
    if (action === 'new-knowledge-task') {
      const item = knowledgeObjects.find((entry) => entry.id === state.selectedKnowledgeId);
      closeKnowledge();
      openCreate({ title: item ? `补齐：${item.title}` : '整理知识候选与证据', detail: item ? `${item.summary}\n关联对象：${item.id}\n当前证据：${item.evidence}` : '围绕待审候选、冲突和知识缺口补齐来源、Evidence 与业务决定。', sop: 'knowledge', contentType: '资料与知识' });
    }
    if (action === 'open-context-record') openContextRecord();
    if (action === 'close-context-record') closeContextRecord();
    if (action === 'submit-context-record') submitContextRecord();
    if (action === 'open-import') openImport();
    if (action === 'close-import') closeImport();
    if (action === 'submit-import') submitImport();
    if (action === 'select-import-adapter') {
      state.importAdapterId = button.dataset.adapterId;
      renderImportAdapters();
      updateImportPreview();
    }
    if (action === 'open-task') openTask(button.dataset.taskId);
    if (action === 'create-supply-task') {
      closeDrawer();
      openCreate({ title: '补齐任务所需资料与权利', detail: '补齐产品参数、来源定位和可用素材权利，完成后恢复原任务。' });
    }
    if (action === 'refresh-inputs') {
      state.inputLastRefresh = '刚刚';
      if (!inboxItems.some((item) => item.id === 4)) inboxItems.unshift({ id: 4, title: '品牌语气规范更新说明', source: '本地 Workspace 文件', detail: '检测到规范文件更新，等待确认是否创建知识维护任务。', next: '建议：补充到品牌知识建设 Project', icon: 'file-text', tone: 'blue', status: 'open' });
      addAudit('input_source.refreshed', '本地输入来源', '成功');
      renderInbox();
      showToast('输入来源已刷新，发现 1 项新输入');
    }
    if (action === 'inbox-filter') {
      state.inboxFilter = button.dataset.inboxFilter;
      renderInbox();
    }
    if (action === 'task-center-filter') {
      state.taskCenterFilter = button.dataset.taskFilter;
      renderTaskCenter(state.route);
    }
    if (action === 'audit-filter') {
      state.auditFilter = button.dataset.auditFilter;
      renderAudit();
    }
    if (action === 'quick-nav') {
      closeDrawer();
      closeKnowledge();
      setView(button.dataset.route);
    }
    if (action === 'create-sop') {
      state.selectedTemplate = 'blank';
      setView('sop');
      showToast('已创建一个空白 SOP 草稿');
    }
    if (action === 'save-sop') {
      state.savedSops = Math.max(1, state.savedSops);
      const template = templates.find((item) => item.id === state.selectedTemplate);
      const draftName = template?.id === 'blank' ? '企业自定义内容流程' : `${template?.name || 'SOP'}副本`;
      if (!sopRegistry.some((item) => item.name === draftName)) sopRegistry.push({ id: `custom-${Date.now()}`, name: draftName, version: 'v1.0', status: 'draft', statusLabel: '草稿', contentType: template?.type || '自定义', tasks: 0, builtin: false, default: false });
      addAudit('sop.draft_saved', draftName, '成功');
      setHeader('sop');
      showToast(`已保存“${draftName}”草稿，可在管理后台发布`);
    }
    if (action === 'sample-run') {
      tasks.unshift({ id: Math.max(...tasks.map((task) => task.id)) + 1, title: `${$('#builder-title').textContent}样例运行`, meta: '样例任务 · 不计入正式交付', project: '新品内容生产', sop: `${$('#builder-title').textContent} · 草稿`, stage: '需求 Brief', status: '执行中', tone: 'blue', owner: '林舟', executor: '本地 Workspace', updated: '刚刚' });
      addAudit('sop.sample_run_started', $('#builder-title').textContent, '执行中', 'blue');
      showToast('样例运行已加入当前 Project 的任务队列');
    }
    if (action === 'toggle-gate') {
      if (state.gateMode === 'none') return;
      state.gateOn = !state.gateOn;
      renderBuilder();
      showToast(state.gateOn ? 'Gate 将阻断未完成的任务' : 'Gate 改为只提醒，不阻断任务');
    }
    if (action === 'publish-sop') {
      const sop = sopRegistry.find((item) => item.id === button.dataset.sopId) || sopRegistry.find((item) => item.status === 'draft');
      if (sop) {
        sop.status = 'published';
        sop.statusLabel = '已发布';
        addAudit('sop.published', `${sop.name} ${sop.version}`, '成功');
      }
      state.savedSops += 1;
      showToast(`${sop?.name || 'SOP'}已发布，新任务可选择使用`);
      renderAdmin();
    }
    if (action === 'archive-inbox') {
      const item = inboxItems.find((inboxItem) => inboxItem.id === Number(button.dataset.inboxId));
      if (item) item.status = 'archived';
      renderInbox();
      showToast('输入已归档，未创建业务事实');
    }
    if (action === 'convert-inbox') {
      const item = inboxItems.find((inboxItem) => inboxItem.id === Number(button.dataset.inboxId));
      if (item) {
        state.pendingInboxId = item.id;
        const knowledgeInput = item.source.includes('文件') || item.source.includes('知识') || item.source.includes('采集');
        openCreate({ title: item.title, detail: `${item.source}：${item.detail}`, sop: knowledgeInput ? 'knowledge' : 'video', contentType: knowledgeInput ? '资料与知识' : '视频脚本' });
      }
    }
    if (action === 'select-context-scope') {
      state.activeContextScope = button.dataset.contextScopeId;
      renderChat();
    }
    if (action === 'toggle-gate-policy') {
      const policy = gatePolicies.find((item) => item.id === button.dataset.entityId);
      if (policy) {
        policy.enabled = !policy.enabled;
        addAudit('gate_policy.toggled', policy.name, policy.enabled ? '已启用' : '已停用', policy.enabled ? 'success' : 'blue');
        renderAdmin();
      }
    }
    if (action === 'toggle-resource') toggleResource(button.dataset.resourceType, button.dataset.resourceId);
    if (action === 'run-resource') {
      const name = button.dataset.resourceName;
      tasks.unshift({ id: Math.max(...tasks.map((task) => task.id)) + 1, title: `试运行：${name}`, meta: '本地试运行 · 不生成正式交付', project: projectConfig.name, sop: `${projectConfig.sop} · ${projectConfig.sopVersion}`, stage: '需求 Brief', status: '执行中', tone: 'blue', owner: '林舟', executor: name, updated: '刚刚' });
      addAudit('local_run.started', name, '执行中', 'blue');
      showToast(`${name}已开始试运行，可在任务中心查看`);
    }
    if (action === 'simple-toggle') {
      const enabled = !button.classList.contains('is-on');
      button.classList.toggle('is-on', enabled);
      button.setAttribute('aria-checked', String(enabled));
      showToast(`配置已${enabled ? '启用' : '停用'}，保存后生效`);
    }
    if (action === 'admin-tab') {
      state.adminTab = button.dataset.adminTab;
      renderAdmin();
    }
    if (action === 'admin-save') {
      const environmentName = $('#admin-environment-name')?.value.trim();
      if (environmentName) projectConfig.environment = environmentName;
      addAudit('environment.updated', projectConfig.environment, '成功');
      renderAdmin();
      showToast('环境配置已保存，并写入审计事件');
    }
    if (action === 'usage-range') {
      state.usageRange = button.dataset.range;
      renderUsage();
    }
  }

  document.addEventListener('click', (event) => {
    const template = event.target.closest('[data-template]');
    if (template) {
      state.selectedTemplate = template.dataset.template;
      renderTemplates();
      renderBuilder();
      return;
    }
    const tab = event.target.closest('[data-tab]');
    if (tab) {
      setView(tab.dataset.tab);
      return;
    }
    const nav = event.target.closest('[data-nav]');
    if (nav) {
      if (nav.dataset.nav === 'new-task') openCreate({ title: '生成 10 条新品短视频脚本' });
      else setView(nav.dataset.nav);
      return;
    }
    const filter = event.target.closest('.task-filters .filter');
    if (filter && state.route === 'tasks' && !filter.dataset.action) {
      state.taskFilter = filter.textContent.trim();
      renderProjectTasks();
      return;
    }
    const action = event.target.closest('[data-action]');
    if (action) handleAction(action);
  });

  $('#gate-mode').addEventListener('change', (event) => {
    state.gateMode = event.target.value;
    renderBuilder();
    showToast(state.gateMode === 'none' ? '当前 SOP 不设置人工 Gate' : 'Gate 模式已更新');
  });
  $('#gate-role').addEventListener('change', (event) => {
    state.gateRole = event.target.value;
    showToast(`Gate 负责角色已改为${state.gateRole}`);
  });
  $('#sop-note').addEventListener('input', (event) => { state.sopNote = event.target.value; });
  $('.mobile-menu').addEventListener('click', () => {
    const open = !document.body.classList.contains('nav-open');
    document.body.classList.toggle('nav-open', open);
    $('.mobile-menu').setAttribute('aria-expanded', String(open));
  });
  $('.workspace-switch select').addEventListener('change', (event) => showToast(`已切换到${event.target.value}`));
  $('.sidebar-action').addEventListener('click', () => setView('admin'));
  $('#drawer-backdrop').addEventListener('click', (event) => { if (event.target === $('#drawer-backdrop')) closeDrawer(); });
  $('#create-backdrop').addEventListener('click', (event) => { if (event.target === $('#create-backdrop')) closeCreate(); });
  $('#context-backdrop').addEventListener('click', (event) => { if (event.target === $('#context-backdrop')) closeContextRecord(); });
  $('#import-backdrop').addEventListener('click', (event) => { if (event.target === $('#import-backdrop')) closeImport(); });
  $('#operation-backdrop').addEventListener('click', (event) => { if (event.target === $('#operation-backdrop')) closeOperation(); });
  $('#import-purpose').addEventListener('change', (event) => { state.importPurpose = event.target.value; updateImportPreview(); });
  $('#import-scope').addEventListener('change', (event) => { state.importScope = event.target.value; updateImportPreview(); });
  $('#import-redact').addEventListener('change', updateImportPreview);
  $('#import-full-consent').addEventListener('change', updateImportPreview);
  document.addEventListener('input', (event) => {
    if (event.target.id === 'knowledge-search') state.knowledgeSearch = event.target.value;
    if (event.target.id === 'knowledge-query-input') state.knowledgeQuery = event.target.value;
    if (event.target.id === 'audit-search') {
      state.auditSearch = event.target.value;
      renderAudit();
      $('#audit-search')?.focus();
      $('#audit-search')?.setSelectionRange(state.auditSearch.length, state.auditSearch.length);
    }
  });
  document.addEventListener('change', (event) => {
    if (event.target.id === 'create-sop') syncCreateTaskSOP();
    if (event.target.id === 'knowledge-type-filter') {
      state.knowledgeType = event.target.value;
      renderKnowledge();
    }
    if (event.target.id === 'knowledge-layer-filter') {
      state.knowledgeLayer = event.target.value;
      renderKnowledge();
    }
  });
  $('#knowledge-backdrop').addEventListener('click', (event) => { if (event.target === $('#knowledge-backdrop')) closeKnowledge(); });
  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape') {
      closeDrawer();
      closeKnowledge();
      closeCreate();
      closeContextRecord();
      closeImport();
      closeOperation();
      document.body.classList.remove('nav-open');
    }
  });

  const initialRoute = location.hash.replace('#', '');
  setView(pageMeta[initialRoute] ? initialRoute : 'workspace', { skipHistory: Boolean(initialRoute) });
})();
