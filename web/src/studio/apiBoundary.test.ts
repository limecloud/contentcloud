// @ts-expect-error Vitest 在 Node 中运行；Web 应用的类型边界不引入 Node globals。
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const studioSources=['CustomerStudioShell.tsx','StudioPages.tsx','StudioContext.tsx','studioApi.ts','studioTypes.ts']
  .map(file=>readFileSync(new URL(`./${file}`,import.meta.url),'utf8'))
  .join('\n');

describe('customer studio boundary',()=>{
  it('uses the dedicated customer API instead of internal BFF resources',()=>{
    expect(studioSources).toContain('/api/studio');
    expect(studioSources).not.toContain('/api/bff');
  });

  it.each(['WorkTaskView','StageRun','GateEvaluation','SOPVersion','executor_kind','capability_id'])('does not import or expose %s',(term)=>{
    expect(studioSources).not.toContain(term);
  });
});
