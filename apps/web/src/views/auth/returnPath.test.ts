import { describe, expect, it } from 'vitest';
import { loginPath, safeReturnPath } from './returnPath';

describe('safeReturnPath',()=>{
  it('keeps only the current Studio and admin surfaces',()=>{
    expect(safeReturnPath('/')).toBe('/studio');
    expect(safeReturnPath('/studio')).toBe('/studio');
    expect(safeReturnPath('/studio/connect')).toBe('/studio/connect');
    expect(safeReturnPath('/studio/connect?session=11111111-1111-4111-8111-111111111111')).toBe('/studio/connect?session=11111111-1111-4111-8111-111111111111');
    expect(safeReturnPath('/studio/connect?project=project-1')).toBe('/studio/connect?project=project-1');
    expect(safeReturnPath('/studio/tasks/task%3A1')).toBe('/studio/tasks/task%3A1');
    expect(safeReturnPath('/studio/tasks/new?experience=marketing-video')).toBe('/studio/tasks/new?experience=marketing-video');
    expect(safeReturnPath('/studio/tasks/new?project=project-1&asset_ref=result%3A1&material_ref=material%3A1')).toBe('/studio/tasks/new?project=project-1&asset_ref=result%3A1&material_ref=material%3A1');
    expect(safeReturnPath('/studio/assets?task_id=task-1')).toBe('/studio/assets?task_id=task-1');
    expect(safeReturnPath('/studio/knowledge?project=project-1')).toBe('/studio/knowledge?project=project-1');
    expect(safeReturnPath('/studio/team')).toBe('/studio/team');
    expect(safeReturnPath('/admin/dashboard')).toBe('/admin/dashboard');
    expect(safeReturnPath('/login')).toBe('/studio');
    expect(safeReturnPath('/unknown')).toBe('/studio');
  });

  it('rejects retired workspace/project URLs and malformed Studio targets',()=>{
    const rejected=[
      '/team',
      '/team?mode=admin',
      '/workspace',
      '/projects/project-1/setup?bootstrap_attempt=attempt-1',
      '/projects/project-1/planning',
      'https://evil.example/projects/project-1/review',
      '//evil.example/projects/project-1/review',
      '/\\evil.example/projects/project-1/review',
      '/studio/connect?session=bad/value',
      '/studio/connect?session=session-1&project=project-1',
      '/studio/tasks/new?project=project-1&unknown=value',
      '/studio/knowledge?project=../project',
      '/studio/knowledge?project=project-1&project=project-2',
      '/studio/team?mode=admin',
      '/studio/knowledge?project=project-1#fragment'
    ];
    for(const value of rejected)expect(safeReturnPath(value),value).toBe('/studio');
  });

  it('constructs a login URL from the canonical Studio return target',()=>{
    expect(loginPath('/studio/team')).toBe('/login?next=%2Fstudio%2Fteam');
    expect(loginPath('https://evil.example')).toBe('/login?next=%2Fstudio');
    expect(loginPath('/')).toBe('/login?next=%2Fstudio');
  });
});
