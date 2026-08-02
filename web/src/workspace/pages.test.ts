import { describe, expect, it } from 'vitest';
// @ts-expect-error Vitest 在 Node 中运行；Web 应用的类型边界不引入 Node globals。
import { readFileSync } from 'node:fs';

const source = readFileSync(new URL('./pages.tsx', import.meta.url), 'utf8');

describe('project console routing', () => {
  it('routes every project view through the V3 console so setup can create a connect session', () => {
    expect(source).toContain("import { V3ProjectPage } from '../v3/ProjectPage';");
    expect(source).toContain('return <V3ProjectPage view={view}/>;');
    expect(source).not.toContain("if(view==='setup')return <WorkOSSOPPage");
  });
});
