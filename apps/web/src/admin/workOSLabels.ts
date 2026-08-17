export const workOSTerm = {
  environment: '客户设置',
  environmentBinding: '客户设置',
  workspaceBinding: '工作电脑连接',
  sop: '创作流程',
  sopWithCode: '创作流程',
  gate: '检查与确认',
  stage: '创作步骤',
  task: '任务',
  workTask: '任务',
  revision: '内容版本',
  digest: '设置说明',
  capability: '可用功能'
} as const;

const gateModeLabels: Record<string, string> = {
  none: '无需确认',
  advisory: '可选建议',
  required_check: '必须检查',
  internal_review: '内部审核',
  client_decision: '客户确认'
};

const executionModeLabels: Record<string, string> = {
  local: '工作电脑处理',
  agent: '系统自动处理'
};

const statusLabels: Record<string, string> = {
  active: '运行中',
  paused: '已暂停',
  draft: '草稿',
  published: '已发布',
  retired: '已停用',
  pending: '待处理',
  running: '运行中',
  completed: '已完成',
  failed: '失败'
};

export function gateModeLabel(mode: string): string {
  return gateModeLabels[mode] || '其他处理方式';
}

export function gateModeBlockingLabel(blocking: boolean): string {
  return blocking ? '会让后面的步骤暂停' : '不会影响后面的步骤';
}

export function executionModeLabel(mode: string): string {
  return executionModeLabels[mode] || '其他处理方式';
}

export function statusLabel(value: string): string {
  return statusLabels[value] || '当前状态';
}
