import { describe, expect, it } from 'vitest';
import { resolveStudioProjectID } from './StudioKnowledgePage';

describe('Studio knowledge navigation',()=>{
  const projects=[{id:'project-1',status:'active'},{id:'project-2',status:'active'},{id:'project-archived',status:'archived'}];

  it('accepts only an active project from the current Studio tenant',()=>{
    expect(resolveStudioProjectID(projects,'project-2')).toBe('project-2');
    expect(resolveStudioProjectID(projects,'project-archived')).toBe('project-1');
    expect(resolveStudioProjectID(projects,'project-other')).toBe('project-1');
    expect(resolveStudioProjectID([],undefined)).toBeUndefined();
  });
});
