import { describe, expect, it } from 'vitest';
import { projectFocusFromSearch, projectNavigationSuffix, projectPageContracts, projectRoute, projectViewIDs } from './page-contracts';

describe('V3 project page contracts',()=>{
  it('defines every route exactly once',()=>{
    expect(Object.keys(projectPageContracts)).toEqual([...projectViewIDs]);
    expect(new Set(projectViewIDs).size).toBe(projectViewIDs.length);
  });

  it('maps every domain page to a projection section',()=>{
    const domainViews=projectViewIDs.filter(view=>view!=='overview');
    expect(domainViews.every(view=>Boolean(projectPageContracts[view].section))).toBe(true);
  });

  it('uses only V3 submission types',()=>{
    const allowed=new Set(['context','knowledge','brief','content_batch','asset_batch','delivery','result']);
    for(const contract of Object.values(projectPageContracts)){
      expect(contract.submissionTypes.every(type=>allowed.has(type))).toBe(true);
      expect(contract.snapshotTypes.every(type=>allowed.has(type))).toBe(true);
    }
  });

  it('uses unique route segments and view-specific focus allowlists',()=>{
    expect(new Set(projectViewIDs.map(projectRoute)).size).toBe(projectViewIDs.length);
    expect(projectPageContracts.review.focusKinds).toContain('submission_revision');
    expect(projectPageContracts.setup.focusKinds).toContain('environment_health');
    expect(projectPageContracts.overview.focusKinds).not.toContain('submission_revision');
  });

  it('parses only view-allowed focus parameters',()=>{
    const digest='sha256:'+'a'.repeat(64);
    expect(projectFocusFromSearch('review',`?focus_kind=submission_revision&focus_id=revision-1&expected_digest=${digest}`).focus).toEqual({kind:'submission_revision',id:'revision-1',digest});
    expect(projectFocusFromSearch('setup','?bootstrap_attempt=attempt-1').focus).toEqual({kind:'bootstrap_attempt',id:'attempt-1'});
    expect(projectFocusFromSearch('overview','?focus_kind=submission_revision&focus_id=revision-1').error?.code).toBe('PROJECT_FOCUS_INVALID');
    expect(projectFocusFromSearch('review','?focus_kind=submission_revision&focus_id=revision-1').error?.code).toBe('PROJECT_FOCUS_DIGEST_REQUIRED');
    expect(projectFocusFromSearch('review','?focus_kind=submission_revision&focus_id=../revision&expected_digest='+digest).error?.code).toBe('PROJECT_FOCUS_INVALID');
    expect(projectFocusFromSearch('setup','?bootstrap_attempt=attempt-1&focus_kind=environment_health&focus_id=doctor-1').error?.code).toBe('PROJECT_FOCUS_INVALID');
    expect(projectFocusFromSearch('review',`?focus_kind=submission_revision&focus_kind=review_cycle&focus_id=revision-1&expected_digest=${digest}`).error?.code).toBe('PROJECT_FOCUS_INVALID');
  });

  it('builds navigation suffixes only from allowlisted targets',()=>{
    const digest='sha256:'+'b'.repeat(64);
    expect(projectNavigationSuffix({view:'planning'})).toBe('planning');
    expect(projectNavigationSuffix({view:'review',focus:{kind:'submission_revision',id:'revision-1',digest}})).toBe(`review?focus_kind=submission_revision&focus_id=revision-1&expected_digest=${encodeURIComponent(digest)}`);
    expect(projectNavigationSuffix({view:'unknown'})).toBeUndefined();
    expect(projectNavigationSuffix({view:'review',focus:{kind:'bootstrap_attempt',id:'attempt-1'}})).toBeUndefined();
    expect(projectNavigationSuffix({view:'review',focus:{kind:'submission_revision',id:'revision-1'}})).toBeUndefined();
    expect(projectNavigationSuffix(undefined)).toBeUndefined();
    expect(projectNavigationSuffix({view:'review',focus:'revision-1'})).toBeUndefined();
  });
});
