import { describe, expect, it } from 'vitest';
import { consolePath } from './consoleRoutes';

describe('console paths',()=>{
  it('builds only relative project navigation from the shared page contract',()=>{
    const digest='sha256:'+'c'.repeat(64);
    expect(consolePath.projectNavigation('project:1',{view:'review',focus:{kind:'submission_revision',id:'revision-1',digest}}))
      .toBe(`/projects/project%3A1/review?focus_kind=submission_revision&focus_id=revision-1&expected_digest=${encodeURIComponent(digest)}`);
    expect(consolePath.projectNavigation('project:1',{view:'https://evil.example'})).toBeUndefined();
    expect(consolePath.projectNavigation('project:1',{view:'review',focus:{kind:'submission_revision',id:'revision-1'}})).toBeUndefined();
    expect(consolePath.projectNavigation('project:1',undefined)).toBeUndefined();
  });
});
