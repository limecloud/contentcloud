import { describe, expect, it } from 'vitest';
import { capabilityStatus, normalizeDigest, validateAgentHandoff, type AgentClient, type AgentHandoff } from './agentHandoff';

const capabilities: AgentClient['capabilities'] = [
  {id:'local_automation',status:'available'},
  {id:'workspace_registration',status:'available'},
  {id:'workspace_bootstrap',status:'available'},
  {id:'interactive_handoff',status:'available'},
  {id:'creative_environment',status:'available'},
];
const codex:AgentClient={id:'codex',display_name:'Codex',capabilities};
const digest=`sha256:${'a'.repeat(64)}`;

function handoff(overrides:Partial<AgentHandoff>={}):AgentHandoff {
  const prompt='[@ContentCloud](plugin://contentcloud-video-production@contentcloud) project project-1; workspace_context';
  const value:AgentHandoff={
    schema_version:'contentcloud.agent-handoff/1.0',client:codex,kind:'project',project_id:'project-1',
    target:{kind:'project',id:'project-1'},integration:{kind:'plugin',id:'contentcloud-video-production@contentcloud',version:'0.12.0'},
    requires_new_session:true,requires_workspace_selection:true,launch:{mode:'deep_link',url:`codex://new?prompt=${encodeURIComponent(prompt)}`},
    prompt,steps:['select workspace'],fallback_url:'/codex',...overrides,
  };
  if(overrides.prompt&&!overrides.launch)value.launch={mode:'deep_link',url:`codex://new?prompt=${encodeURIComponent(overrides.prompt)}`};
  return value;
}

describe('Agent handoff contract',()=>{
  it('accepts the active Codex strategy',()=>{
    const value=validateAgentHandoff(handoff(),{client:codex,projectID:'project-1',targetKind:'project',targetID:'project-1'});
    expect(value.client.id).toBe('codex');
  });

  it('rejects unsafe or drifting launch contracts',()=>{
    for(const url of ['codex://new?path=%2FUsers%2Fprivate&prompt=x','codex://new?prompt=x&extra=y','https://contentcloud.test/codex']){
      expect(()=>validateAgentHandoff(handoff({prompt:'x',launch:{mode:'deep_link',url}}),{client:codex,projectID:'project-1',targetKind:'project',targetID:'project-1'})).toThrow();
    }
    expect(()=>validateAgentHandoff(handoff({project_id:'other'}),{client:codex,projectID:'project-1',targetKind:'project',targetID:'project-1'})).toThrow();
    expect(()=>validateAgentHandoff(handoff({prompt:'workspace_context for a different project'}),{client:codex,projectID:'project-1',targetKind:'project',targetID:'project-1'})).toThrow('Codex 恢复适配器契约无效');
  });

  it('requires review prompts to bind the exact revision and digest',()=>{
    const prompt=`[@ContentCloud](plugin://contentcloud-video-production@contentcloud) workspace_context project-1 revision-1 ${digest} review_feedback_list`;
    const value=handoff({kind:'review_feedback',target:{kind:'submission_revision',id:'revision-1',digest},prompt});
    expect(validateAgentHandoff(value,{client:codex,projectID:'project-1',targetKind:'submission_revision',targetID:'revision-1',digest}).target.id).toBe('revision-1');
    expect(()=>validateAgentHandoff(handoff({kind:'review_feedback',target:{kind:'submission_revision',id:'revision-1',digest},prompt:'workspace_context project-1 revision-1 review_feedback_list'}),{client:codex,projectID:'project-1',targetKind:'submission_revision',targetID:'revision-1',digest})).toThrow();
  });

  it('keeps planned capabilities non-operational',()=>{
    const cursor:AgentClient={id:'cursor',display_name:'Cursor',capabilities:capabilities.map(item=>({...item,status:'planned'}))};
    expect(capabilityStatus(cursor,'interactive_handoff')).toBe('planned');
    expect(()=>validateAgentHandoff(handoff({client:cursor}),{client:cursor,projectID:'project-1',targetKind:'project',targetID:'project-1'})).toThrow('尚未实现');
  });

  it('normalizes immutable digests',()=>{
    expect(normalizeDigest('A'.repeat(64))).toBe(digest);
  });
});
