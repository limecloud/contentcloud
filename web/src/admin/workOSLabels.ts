export const workOSTerm = {
  environment: '执行环境',
  environmentBinding: '执行环境绑定',
  workspaceBinding: '本地工作区连接',
  sop: '流程规范',
  sopWithCode: '流程规范（SOP）',
  gate: '检查与审批',
  stage: '流程阶段',
  task: '任务',
  workTask: '工作任务',
  taskRun: '执行记录',
  revision: '内容版本',
  digest: '配置摘要',
  capability: '本地能力'
} as const;

const gateModeLabels: Record<string, string> = {
  none: '不设置审批',
  advisory: '可选建议',
  required_check: '必做检查',
  internal_review: '内部审核',
  client_decision: '客户确认',
  required: '必选审批（兼容模式）'
};

const executionModeLabels: Record<string, string> = {
  local: '本地客户端',
  agent: '自动执行适配器'
};

const statusLabels: Record<string, string> = {
  active: '运行中',
  paused: '已暂停',
  draft: '草稿',
  published: '已发布',
  retired: '已退役',
  pending: '待处理',
  running: '运行中',
  completed: '已完成',
  failed: '失败'
};

export function gateModeLabel(mode: string): string {
  return gateModeLabels[mode] || `未识别模式（${mode}）`;
}

export function gateModeBlockingLabel(blocking: boolean): string {
  return blocking ? '会阻断后续阶段' : '不阻断后续阶段';
}

export function executionModeLabel(mode: string): string {
  return executionModeLabels[mode] || `未识别方式（${mode}）`;
}

export function statusLabel(value: string): string {
  return statusLabels[value] || value;
}
