import { describe, expect, it } from 'vitest';
import { executionModeLabel, gateModeBlockingLabel, gateModeLabel, statusLabel, workOSTerm } from './workOSLabels';

describe('后台业务术语映射', () => {
  it('用中文业务语言表达核心运行对象', () => {
    expect(workOSTerm.environment).toBe('执行环境');
    expect(workOSTerm.sop).toBe('流程规范');
    expect(workOSTerm.gate).toBe('检查与审批');
    expect(workOSTerm.stage).toBe('流程阶段');
    expect(workOSTerm.task).toBe('任务');
  });

  it('把检查与审批模式翻译为可理解的业务表达', () => {
    expect(gateModeLabel('none')).toBe('不设置审批');
    expect(gateModeLabel('required_check')).toBe('必做检查');
    expect(gateModeLabel('internal_review')).toBe('内部审核');
    expect(gateModeLabel('client_decision')).toBe('客户确认');
    expect(gateModeLabel('unknown_mode')).toContain('unknown_mode');
    expect(gateModeBlockingLabel(true)).toBe('会阻断后续阶段');
    expect(gateModeBlockingLabel(false)).toBe('不阻断后续阶段');
  });

  it('保留技术枚举作为辅助信息，而不让它成为主标签', () => {
    expect(executionModeLabel('local')).toBe('本地客户端');
    expect(executionModeLabel('agent')).toBe('自动执行适配器');
    expect(statusLabel('published')).toBe('已发布');
    expect(statusLabel('unknown_status')).toBe('未识别状态（unknown_status）');
  });
});
