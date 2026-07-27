import { describe, expect, it } from 'vitest';
import { loginPath, safeReturnPath } from './returnPath';

describe('safeReturnPath',()=>{
  const digest='sha256:'+'d'.repeat(64);

  it('keeps only allowlisted console pages',()=>{
    expect(safeReturnPath('/')).toBe('/');
    expect(safeReturnPath('/team')).toBe('/team');
    expect(safeReturnPath('/admin/dashboard')).toBe('/admin/dashboard');
    expect(safeReturnPath('/login')).toBe('/');
    expect(safeReturnPath('/unknown')).toBe('/');
    expect(safeReturnPath('/team?mode=admin')).toBe('/');
  });

  it('canonicalizes Page Contract project targets and preserves exact focus',()=>{
    const value=`/projects/project%3A1/review?expected_digest=${encodeURIComponent(digest)}&focus_id=revision-1&focus_kind=submission_revision`;
    expect(safeReturnPath(value)).toBe(`/projects/project%3A1/review?focus_kind=submission_revision&focus_id=revision-1&expected_digest=${encodeURIComponent(digest)}`);
    expect(safeReturnPath('/projects/project-1/setup?bootstrap_attempt=attempt-1')).toBe('/projects/project-1/setup?bootstrap_attempt=attempt-1');
    expect(safeReturnPath('/projects/project-1/planning')).toBe('/projects/project-1/planning');
  });

  it('rejects external, ambiguous, unknown, and malformed targets',()=>{
    const rejected=[
      'https://evil.example/projects/project-1/review',
      '//evil.example/projects/project-1/review',
      '/\\evil.example/projects/project-1/review',
      '/projects/%2F%2Fevil/review',
      '/projects/../review',
      '/projects/project-1/unknown',
      '/projects/project-1/review?focus_kind=submission_revision&focus_id=revision-1',
      `/projects/project-1/review?focus_kind=submission_revision&focus_id=revision-1&expected_digest=${digest}&url=https://evil.example`,
      `/projects/project-1/review?focus_kind=submission_revision&focus_id=revision-1&focus_id=revision-2&expected_digest=${digest}`,
      `/projects/project-1/review?focus_kind=submission_revision&focus_id=../revision&expected_digest=${digest}`,
      `/projects/project-1/review?focus_kind=submission_revision&focus_id=revision-1&expected_digest=sha256:abc`,
      `/projects/project-1/review?focus_kind=submission_revision&focus_id=revision-1&expected_digest=${digest}#fragment`
    ];
    for(const value of rejected)expect(safeReturnPath(value),value).toBe('/');
  });

  it('constructs a login URL from the canonical return target',()=>{
    expect(loginPath('/team')).toBe('/login?next=%2Fteam');
    expect(loginPath('https://evil.example')).toBe('/login?next=%2F');
  });
});
