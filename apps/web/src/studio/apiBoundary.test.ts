import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const studioSources=[
  'CustomerStudioShell.tsx','StudioPages.tsx','StudioContext.tsx','studioApi.ts','studioTypes.ts','StudioKnowledgePage.tsx',
  'knowledge/GovernedKnowledgePage.tsx','knowledge/knowledgeApi.ts'
].map(file=>readFileSync(new URL(`./${file}`,import.meta.url),'utf8')).join('\n');

describe('customer studio boundary',()=>{
  it('uses the dedicated customer API instead of internal BFF resources',()=>{
    expect(studioSources).toContain('/api/studio');
    expect(studioSources).not.toContain('/api/bff');
  });

  it.each(['WorkTaskView','StageRun','GateEvaluation','SOPVersion','executor_kind','capability_id'])('does not import or expose %s',(term)=>{
    expect(studioSources).not.toContain(term);
  });

  it('keeps input references out of the customer result-asset language',()=>{
    expect(studioSources).not.toContain('从人物、灵感、历史剧本和已批准成果开始下一次创作');
    expect(studioSources).not.toContain('保存到资产库');
  });
});
