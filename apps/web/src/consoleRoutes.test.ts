import { describe, expect, it } from 'vitest';
import { consolePath } from './consoleRoutes';

describe('console paths',()=>{
  it('uses Studio as the only customer workspace surface',()=>{
    expect(consolePath.dashboard).toBe('/studio');
    expect(consolePath.studio).toBe('/studio');
    expect(consolePath.team).toBe('/studio/team');
  });
});
