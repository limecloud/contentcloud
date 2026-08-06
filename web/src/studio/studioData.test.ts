import { describe, expect, it } from 'vitest';
import { customerTaskTone, studioStepStateLabel } from './studioData';

describe('customer studio presentation state',()=>{
  it('uses only customer-facing step states returned by the studio API',()=>{
    expect(studioStepStateLabel('completed')).toBe('已确认');
    expect(studioStepStateLabel('needs_decision')).toBe('等待确认');
    expect(studioStepStateLabel('not_started')).toBe('待开始');
  });

  it('maps task status to restrained semantic tones',()=>{
    expect(customerTaskTone('running')).toBe('is-working');
    expect(customerTaskTone('waiting_gate')).toBe('is-attention');
    expect(customerTaskTone('delivered')).toBe('is-success');
  });
});
