import { describe, expect, it } from 'vitest';
import { executionModeLabel, gateModeBlockingLabel, gateModeLabel, statusLabel, workOSTerm } from './workOSLabels';

describe('后台业务术语映射', () => {
  it('用中文业务语言表达核心运行对象', () => {
    expect(workOSTerm.environment).toBe('客户设置');
    expect(workOSTerm.sop).toBe('创作流程');
    expect(workOSTerm.gate).toBe('检查与确认');
    expect(workOSTerm.stage).toBe('创作步骤');
    expect(workOSTerm.task).toBe('任务');
  });

  it('把检查与审批模式翻译为可理解的业务表达', () => {
    expect(gateModeLabel('none')).toBe('无需确认');
    expect(gateModeLabel('required_check')).toBe('必须检查');
    expect(gateModeLabel('internal_review')).toBe('内部审核');
    expect(gateModeLabel('client_decision')).toBe('客户确认');
    expect(gateModeLabel('unknown_mode')).toBe('其他处理方式');
    expect(gateModeBlockingLabel(true)).toBe('会让后面的步骤暂停');
    expect(gateModeBlockingLabel(false)).toBe('不会影响后面的步骤');
  });

  it('保留技术枚举作为辅助信息，而不让它成为主标签', () => {
    expect(executionModeLabel('local')).toBe('工作电脑处理');
    expect(executionModeLabel('agent')).toBe('系统自动处理');
    expect(statusLabel('published')).toBe('已发布');
    expect(statusLabel('unknown_status')).toBe('当前状态');
  });
});
