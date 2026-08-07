const statusLabels: Record<string, string> = {
  accepted: '已接受', accepted_risk: '已接受风险', active: '运行中', approved: '已批准', archived: '已归档',
  awaiting_cost_approval: '待确认费用', blocked: '已阻断', canceled: '已取消', cancelled: '已取消', cancelled_job: '已取消',
  candidate: '候选', changes_requested: '待修改', client_review: '客户审核中', collecting: '补充资料中', complete: '完整',
  completed: '已完成', conflicted: '有冲突', connected: '已接入', delivered: '已交付', discarded: '不采用',
  consumed: '已使用', denied: '已拒绝', downloading: '下载中', draft: '草稿', empty: '暂无记录', expired: '已过期', failed: '失败', generating: '生成中',
  imported: '已导入', in_review: '审核中', insufficient_sample: '样本不足', internal_review: '内部审核中',
  internally_approved: '内部审核通过', leased: '执行中', legacy_incomplete: '历史数据不完整', needs_info: '待补信息',
  needs_input: '待补输入', needs_review: '待审核', open: '待解决', output_invalid: '输出不符合要求', paused: '已暂停',
  passed: '已通过', pending: '待处理', prohibited: '已禁止', project_material: '已归档为项目资料', published: '已发布', queued: '排队中',
  ready: '可开始', ready_package: '可交付', rejected: '已拒绝', repairable: '可修复', resolved: '已解决', retired: '已停用',
  retry_wait: '等待重试', retryable_failed: '失败，可重试', review_ready: '可审核', review_required: '待复核',
  revision_requested: '待修订', revoked: '已撤销', routed: '已转负责人', running: '运行中', seed_candidate: '小范围测试候选',
  script_only: '仅剧本', source_missing: '缺少来源', submitted: '已提交', submitting: '提交中', succeeded: '已完成', superseded: '已替代',
  suspended: '已停用', planned: '即将支持', task_created: '已创建任务', task_merged: '已并入任务', untriaged: '待分流', valid: '有效',
  validated: '已核验', validating: '校验中', verified: '已核验', verifying: '接入初始化中',
  waiting_for_computer: '等待执行客户端', waiting_gate: '待审核决定', waived: '已豁免'
};

const roleLabels: Record<string, string> = {
  tenant_admin: '租户管理员', project_manager: '项目经理', strategist: '内容策略', editor: '内容编辑', reviewer: '内容审核',
  viewer: '只读成员', client_approver: '客户审批人', workspace: '本地工作区', agent: '自动执行适配器'
};

const contentTypeLabels: Record<string, string> = {
  marketing_video: '营销视频', video_script: '短视频剧本', wechat_article: '微信公众号文章'
};

const submissionTypeLabels: Record<string, string> = {
  context: '项目上下文', knowledge: '知识库', brief: '创作简报', content_batch: '内容批次', asset_batch: '素材批次',
  delivery: '交付包', result: '投放结果', storyboard: '分镜包'
};

const knowledgeObjectTypeLabels: Record<string, string> = {
  Asset: '素材', Audience: '目标受众', BrandRule: '品牌规则', Campaign: '营销活动', Claim: '营销主张',
  ConflictRecord: '冲突记录', ConstraintRecord: '约束记录', DomainObject: '业务对象', FactAssertion: '事实陈述',
  Insight: '业务洞察', KnowledgeGap: '知识缺口', Learning: '复盘结论', Process: '业务流程',
  RightsRecord: '权利记录', Scenario: '使用场景', knowledge_object: '知识对象'
};

const knowledgeLayerLabels: Record<string, string> = {
  identity: '品牌身份', product: '产品信息', market: '市场洞察', expression: '内容表达',
  operations: '业务运营', content_engine: '内容方法', compliance: '合规要求'
};

const sourceTypeLabels: Record<string, string> = {
  brief: '创作简报', manual_inspiration: '人工灵感', workspace_file: '本地工作区文件', comment: '审核评论', external_request: '外部需求',
  trigger: '触发事件', conversation_bundle: '对话摘要包', local_import: '本地上传', other: '其他'
};

const evidenceLocatorLabels: Record<string, string> = {
  paragraph: '段落', slide: '幻灯片', sheet_cell: '表格单元格', page: '页面', image_region: '图片区域', markdown: 'Markdown 段落'
};

const stageLabels: Record<string, string> = {
  onboarding: '接入初始化', methodology: '方法论', context: '项目上下文', sources: '来源', knowledge: '知识',
  intelligence: '情报', strategy: '策略', planning: '策划', creative: '创意', script: '剧本', storyboard: '分镜',
  generation: '视频生成', review: '审核', postproduction: '后期制作', delivery: '交付', learning: '复盘', handoff: '交接', automation: '自动化'
};

const gateModeLabels: Record<string, string> = {
  none: '无审批', required_check: '必做检查', advisory: '可选建议',
  internal_review: '内部审核', client_decision: '客户确认', required: '必选审批'
};

const outputRoleLabels: Record<string, string> = {
  primary: '主要输出', preview: '预览文件', selected_take: '选定的候选成片', final: '最终成果'
};

const artifactKindLabels: Record<string, string> = {
  delivery: '交付文件', storyboard_image: '分镜图片', storyboard_media: '分镜媒体',
  generated_video: '候选成片', final_render: '最终成片'
};

const shotRoleLabels: Record<string, string> = {
  hook: '开场钩子', context: '背景铺垫', pain: '痛点呈现', friction: '矛盾冲突', bridge: '情节转折',
  product_intro: '产品引入', product_solution: '产品方案', usage: '使用场景', proof: '事实佐证',
  resolution: '问题解决', payoff: '价值收束', cta: '行动引导'
};

const storyboardAssetRoleLabels: Record<string, string> = {
  first_frame: '首帧图片', end_frame: '尾帧图片', reference: '参考素材', review_sheet: '分镜审核总览',
  audio: '音频素材', soundtrack: '配乐素材', voiceover: '口播音频'
};

const articleCalloutKindLabels: Record<string, string> = {
  note: '补充说明', conclusion: '小结', warning: '注意事项'
};

const workflowCheckLabels: Record<string, string> = {
  'brief.required': '需求简报完整', 'source.registered': '来源已登记', 'claim.references': '营销主张引用完整',
  'rights.references': '权利引用完整', 'knowledge.lint': '知识规则检查通过', 'strategy.complete': '策略内容完整',
  'content.schema': '内容结构符合要求', 'storyboard.locked': '分镜内容已锁定', 'media.technical': '媒体技术检查通过',
  'cost.confirmed': '生成费用已确认', 'media.content': '成片内容检查通过', 'media.final': '最终成片检查通过',
  'offer.valid': '营销信息有效', 'delivery.integrity': '交付文件完整', 'metrics.source': '指标来源明确',
  'metrics.window': '统计周期明确', 'content.id': '内容标识有效', 'observation.complete': '结果观察完整',
  'hypothesis.scoped': '改进假设范围明确', 'next_action.required': '下一步行动明确',
  'script.alignment': '画面与剧本一致'
};

const knowledgeNextActionLabels: Record<string, string> = {
  REQUEST_SOURCE: '补充可信来源', REVIEW_SOURCE: '复核来源', RESOLVE_CONFLICT: '解决知识冲突',
  REVIEW_RIGHTS: '复核使用权利', UPDATE_KNOWLEDGE: '更新知识内容'
};

const deliveryDestinationLabels: Record<string, string> = {
  workspace: '本地工作区', download: '浏览器下载', client: '客户交付区'
};

const knowledgePackPurposeLabels: Record<string, string> = {
  content_production: '内容生产', marketing_video: '营销视频', video_script: '短视频剧本', wechat_article: '微信公众号文章'
};

const taskTypeLabels: Record<string, string> = {
  knowledge_extract: '品牌知识提取', marketing_video: '营销视频生产', video_script: '短视频剧本生产', wechat_article: '微信公众号文章生产'
};

const mediaProviderLabels: Record<string, string> = {
  fake: '本地演示生成服务'
};

const mediaModelLabels: Record<string, string> = {
  'fixture-video': '演示视频模型'
};

const rendererLabels: Record<string, string> = {
  passthrough: '原样封装'
};

const auditActionLabels: Record<string, string> = {
  'approved_snapshot.exported': '导出已批准快照', 'asset.created': '创建素材',
  'bootstrap.authorization.approved': '批准接入授权', 'bootstrap.authorization.denied': '拒绝接入授权',
  'bootstrap.diagnostic.uploaded': '上传接入诊断信息', 'cli_login.approved': '批准命令行登录',
  'cli_token.tenant_switched': '切换命令行租户', 'connect_session.canceled': '取消接入会话',
  'connect_session.created': '创建接入会话', 'conversation_import.cancelled': '取消对话导入',
  'conversation_import.created': '创建对话导入', 'conversation_import.uploaded': '上传对话内容',
  'delivery.package_built': '构建交付包', 'delivery_package.created': '创建交付包',
  'device.attached': '关联本地设备', 'device.connected': '连接本地设备', 'device.detached': '解除设备关联',
  'device.revoked': '撤销本地设备', 'environment.created': '创建执行环境', 'environment.updated': '更新执行环境',
  'evidence.reviewed': '审核证据', 'gate.created': '创建检查与审批记录', 'gate.decided': '完成检查与审批',
  'input_item.created': '创建输入记录', 'input_item.triaged': '分流输入记录',
  'knowledge_extraction_run.created': '创建知识提取任务', 'knowledge_extraction_run.reported': '上报知识提取结果',
  'knowledge_object.created': '创建知识对象', 'knowledge_object.decided': '完成知识审核',
  'knowledge_pack.created': '创建知识包', 'knowledge_pack.published': '发布知识包',
  'media.cost_approved': '确认视频生成费用', 'media.final_render_created': '生成最终成片',
  'media.job_cancelled': '取消视频生成任务', 'media.job_created': '创建视频生成任务', 'media.review_decided': '完成媒体审核',
  'membership.invite_accepted': '接受成员邀请', 'membership.invite_revoked': '撤销成员邀请',
  'membership.invited': '邀请成员', 'membership.revoked': '移除成员', 'membership.role_changed': '修改成员角色',
  'performance_import.created': '导入投放表现数据', 'platform.tenant_content_capability_changed': '修改租户内容能力',
  'platform.tenant_status_changed': '修改租户状态', 'project.archived': '归档项目', 'project.created': '创建项目',
  'project.restored': '恢复项目', 'project.sop_bound': '绑定项目流程规范', 'project.updated': '更新项目',
  'project_template.created': '创建项目模板', 'rating_decision.created': '创建表现评级',
  'review_comment.resolved': '解决审核评论', 'review_grant.created': '创建外部审核授权',
  'review_grant.revoked': '撤销外部审核授权', 'rights.created': '创建素材权利记录', 'rights.reviewed': '审核素材权利',
  'run.attempt_canceled': '取消执行尝试', 'run.cancel_requested': '请求取消执行任务', 'session.tenant_switched': '切换当前租户',
  'sop.created': '创建流程规范', 'sop.version_created': '创建流程规范草稿', 'sop.version_published': '发布流程规范',
  'sop.version_retired': '停用流程规范版本', 'sop.version_rolled_back': '回滚流程规范版本', 'sop.version_saved': '保存流程规范草稿',
  'source.created': '登记来源', 'source.impact_propagated': '更新来源影响范围', 'source.processed': '完成来源解析',
  'source.revision_created': '上传来源新版本', 'stage.reported_failed': '上报流程阶段失败',
  'storyboard.artifact_uploaded': '上传分镜素材', 'submission.approved': '批准内容版本',
  'submission.changes_requested': '退回内容修改', 'submission.client_reviewed': '完成客户审核',
  'submission.internally_approved': '完成内部审核', 'submission.published': '提交内容版本',
  'task.cancelled': '取消任务', 'task.claimed': '认领任务', 'task.created': '创建任务',
  'task.delivery_created': '创建任务交付记录', 'task.paused': '暂停任务', 'task.retry_scheduled': '安排任务重试',
  'task.revision_submitted': '提交任务内容版本', 'task.script_approved': '批准任务剧本', 'task.started': '开始任务',
  'tenant.created': '创建租户', 'workspace.registered': '登记本地工作区'
};

const auditSubjectLabels: Record<string, string> = {
  artifact: '成果文件', asset: '素材', bootstrap_attempt: '接入初始化尝试', bootstrap_diagnostic: '接入诊断信息',
  cli_token: '命令行访问凭证', connect_session: '接入会话', conversation_import: '对话导入', delivery_package: '交付包',
  device: '本地设备', environment: '执行环境', evidence_span: '证据片段', gate_evaluation: '检查与审批记录',
  input_item: '输入记录', knowledge_object: '知识对象', knowledge_pack: '知识包', media_generation_job: '视频生成任务',
  media_review: '媒体审核', membership: '成员关系', membership_invite: '成员邀请', performance_import_batch: '表现数据导入批次',
  project: '项目', project_sop_binding: '项目流程规范绑定', project_template: '项目模板', rating_decision: '表现评级',
  review_comment: '审核评论', review_grant: '外部审核授权', rights_record: '素材权利记录', run_attempt: '执行尝试',
  session: '登录会话', sop: '流程规范', sop_version: '流程规范版本', source: '来源', source_revision: '来源版本',
  stage_run: '流程阶段记录', submission_revision: '提交内容版本', task: '任务', task_delivery: '任务交付',
  task_revision: '任务内容版本', task_run: '执行任务', tenant: '租户', user_device_flow: '命令行登录授权',
  workspace_binding: '本地工作区连接'
};

export function statusLabel(value: string): string { return statusLabels[value] || `未识别状态（${value}）`; }
export function roleLabel(value: string): string { return roleLabels[value] || `未识别角色（${value}）`; }
export function contentTypeLabel(value: string): string { return contentTypeLabels[value] || `未识别内容类型（${value}）`; }
export function submissionTypeLabel(value: string): string { return submissionTypeLabels[value] || `未识别提交类型（${value}）`; }
export function knowledgeObjectTypeLabel(value: string): string { return knowledgeObjectTypeLabels[value] || `未识别对象类型（${value}）`; }
export function knowledgeObjectTypeValue(value: string): string {
  const normalized = value.trim();
  if (knowledgeObjectTypeLabels[normalized]) return normalized;
  return Object.entries(knowledgeObjectTypeLabels).find(([, label]) => label === normalized)?.[0] || normalized;
}
export function knowledgeLayerLabel(value: string): string { return knowledgeLayerLabels[value] || `未识别知识层级（${value}）`; }
export function sourceTypeLabel(value: string): string { return sourceTypeLabels[value] || `未识别来源类型（${value}）`; }
export function evidenceLocatorLabel(value: string): string { return evidenceLocatorLabels[value] || `未识别证据位置（${value}）`; }
export function stageLabel(value: string): string { return stageLabels[value] || `未识别流程阶段（${value}）`; }
export function gateModeLabel(value: string): string { return gateModeLabels[value] || `未识别审核方式（${value}）`; }
export function outputRoleLabel(value: string): string { return outputRoleLabels[value] || `未识别输出角色（${value}）`; }
export function artifactKindLabel(value: string): string { return artifactKindLabels[value] || `未识别成果类型（${value}）`; }
export function shotRoleLabel(value: string): string { return shotRoleLabels[value] || `其他镜头作用（${value}）`; }
export function storyboardAssetRoleLabel(value: string): string { return storyboardAssetRoleLabels[value] || `其他素材用途（${value}）`; }
export function articleCalloutKindLabel(value: string): string { return articleCalloutKindLabels[value] || '提示'; }
export function workflowCheckLabel(value: string): string { return workflowCheckLabels[value] || `其他检查（${value}）`; }
export function knowledgeNextActionLabel(value: string): string { return knowledgeNextActionLabels[value] || `按要求补充信息（${value}）`; }
export function deliveryDestinationLabel(value: string): string { return deliveryDestinationLabels[value] || `其他交付位置（${value}）`; }
export function knowledgePackPurposeLabel(value: string): string { return knowledgePackPurposeLabels[value] || value || '未填写用途'; }
export function taskTypeLabel(value: string): string { return taskTypeLabels[value] || `其他任务（${value}）`; }
export function mediaProviderLabel(value: string): string { return mediaProviderLabels[value] || value || '未指定生成服务'; }
export function mediaModelLabel(value: string): string { return mediaModelLabels[value] || value || '未指定模型'; }
export function mediaRequestIDLabel(providerID: string, requestID?: string): string {
  if (!requestID) return '未返回请求编号';
  if (providerID === 'fake' && requestID.startsWith('fake-request-')) return `本地演示请求 ${requestID.slice('fake-request-'.length)}`;
  return requestID;
}
export function rendererLabel(value: string): string { return rendererLabels[value] || value || '固定渲染器'; }
export function knowledgeBlockReasonLabel(value: string): string {
  if (value.startsWith('STATUS_')) return `当前状态不符合要求：${statusLabel(value.slice(7).toLowerCase())}`;
  const separator = value.indexOf(':');
  const code = separator < 0 ? value : value.slice(0, separator);
  const reference = separator < 0 ? '' : value.slice(separator + 1);
  const labels: Record<string, string> = {
    OBJECT_TYPE_NOT_ALLOWED: '当前知识包不允许此对象类型', EVIDENCE_REQUIRED: '缺少可核验的证据',
    CHANNEL_NOT_ALLOWED: '当前渠道不可使用', NOT_YET_VALID: '尚未到生效时间', VALIDITY_ENDED: '有效期已结束',
    EXPIRED: '内容已过期', CONFLICT_REFERENCE_MISSING: '引用的冲突记录不存在', CONFLICT_OPEN: '仍有知识冲突未解决',
    RIGHTS_REFERENCE_MISSING: '引用的权利记录不存在', RIGHTS_NOT_USABLE: '当前权利记录不可用'
  };
  const label = labels[code] || `其他阻断原因（${code}）`;
  return reference ? `${label}：${reference}` : label;
}
export function auditActionLabel(value: string): string { return auditActionLabels[value] || `系统操作（${value}）`; }
export function auditSubjectLabel(value: string): string { return auditSubjectLabels[value] || `系统对象（${value}）`; }
