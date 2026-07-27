import { describe, expect, it } from 'vitest';
import type { SubmissionRevision } from '../types';
import { inaccessibleProjectIssue, projectPageIssueFromError, summarizeDisclosure } from './page-state';

describe('V4 project page states', () => {
  it('uses the same public state for missing and forbidden targets', () => {
    const missing = projectPageIssueFromError({status: 404, api: {code: 'RESOURCE_NOT_FOUND'}});
    const forbidden = projectPageIssueFromError({status: 403, api: {code: 'ROLE_DENIED'}});
    expect(missing).toEqual(forbidden);
    expect(missing).toEqual(inaccessibleProjectIssue());
    expect(missing.detail).not.toContain('ROLE_DENIED');
  });

  it('uses a retryable state for transport and server failures', () => {
    expect(projectPageIssueFromError(new Error('private upstream detail'))).toMatchObject({
      kind: 'unavailable',
      code: 'PROJECT_PAGE_UNAVAILABLE',
    });
  });

  it('maps an expired session to the login recovery state', () => {
    expect(projectPageIssueFromError({status: 401})).toMatchObject({
      kind: 'auth',
      code: 'PROJECT_AUTH_REQUIRED',
    });
  });

  it('summarizes disclosure levels without returning evidence bodies', () => {
    const revision = {
      evidence_limited: false,
      source_disclosures: [
        {id: 'd1', source_ref: 's1', level: 'metadata_only', sha256: 'a'.repeat(64), byte_size: 1},
        {id: 'd2', source_ref: 's2', level: 'evidence_pack', sha256: 'b'.repeat(64), byte_size: 2, evidence_pack: {quote: 'private'}},
        {id: 'd3', source_ref: 's3', level: 'full_source', sha256: 'c'.repeat(64), byte_size: 3},
      ],
    } as Pick<SubmissionRevision, 'evidence_limited' | 'source_disclosures'>;
    const summary = summarizeDisclosure(revision);
    expect(summary).toEqual({total: 3, metadataOnly: 1, evidencePack: 1, fullSource: 1, unknown: 0, limited: false});
    expect(JSON.stringify(summary)).not.toContain('private');
  });

  it('treats unknown disclosure levels as limited', () => {
    const revision = {
      evidence_limited: false,
      source_disclosures: [{id: 'd1', source_ref: 's1', level: 'future_level', sha256: 'a'.repeat(64), byte_size: 1}],
    } as unknown as Pick<SubmissionRevision, 'evidence_limited' | 'source_disclosures'>;
    expect(summarizeDisclosure(revision)).toMatchObject({unknown: 1, limited: true});
  });
});
