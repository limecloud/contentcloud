import type { StudioStepStatus } from './studioTypes';

export function customerTaskTone(status:string):string {
  if(status==='delivered'||status==='accepted')return'is-success';
  if(status==='blocked'||status==='cancelled'||status==='canceled')return'is-danger';
  if(status==='waiting_gate'||status==='needs_input')return'is-attention';
  if(status==='running')return'is-working';
  return'is-muted';
}

export function studioStepStateLabel(status:StudioStepStatus):string {
  return {completed:'已确认',working:'进行中',ready:'可以开始',needs_input:'待补资料',needs_decision:'等待确认',blocked:'需要协助',not_started:'待开始'}[status];
}
