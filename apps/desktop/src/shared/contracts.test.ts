import { describe, expect, it } from 'vitest';

import { isCommandResponse, isEventStream, isPublishWorkspaceInput, isReviewCommentRequest, isReviewDecisionRequest, isReviewRevisionRequest, isSnapshot } from './contracts';

describe('desktop snapshot contract', () => {
  it('accepts only the current schema with a project collection', () => {
    expect(isSnapshot({ schema_version: 'contentcloud.desktop-snapshot/1.0', projects: [] })).toBe(true);
    expect(isSnapshot({ schema_version: 'contentcloud.desktop-snapshot/0.9', projects: [] })).toBe(false);
    expect(isSnapshot({ schema_version: 'contentcloud.desktop-snapshot/1.0', projects: null })).toBe(false);
  });
});

describe('desktop command and event contracts', () => {
  it('validates the narrow publish input', () => {
    expect(isPublishWorkspaceInput({ workspace_id: 'workspace-1', project_id: 'project-1', base_revision: '0', observed_digest: `sha256:${'a'.repeat(64)}` })).toBe(true);
    expect(isPublishWorkspaceInput({ workspace_id: 'workspace-1', project_id: 'project-1', base_revision: '0', observed_digest: 'invalid' })).toBe(false);
  });

  it('rejects stale command and event schema versions', () => {
    expect(isCommandResponse({ schema_version: 'contentcloud.desktop-command-result/0.9', state: 'queued' })).toBe(false);
    expect(isEventStream({ schema_version: 'contentcloud.desktop-events/1.0', project_id: 'project-1', events: [], next_cursor: 2, gap: false, resync_required: false })).toBe(true);
    expect(isEventStream({ schema_version: 'contentcloud.desktop-events/0.9', project_id: 'project-1', events: [], next_cursor: 2, gap: false, resync_required: false })).toBe(false);
  });
});

describe('desktop main IPC request contracts', () => {
  it('accepts scoped review requests and rejects malformed or unknown actions', () => {
    expect(isReviewRevisionRequest({ projectID: 'project-1', revisionID: 'revision-1' })).toBe(true);
    expect(isReviewRevisionRequest({ projectID: 'project-1', revisionID: '' })).toBe(false);
    expect(isReviewRevisionRequest({ projectID: 'project-1', revisionID: 'revision-1', extra: true })).toBe(false);
    expect(isReviewCommentRequest({ projectID: 'project-1', payload: { revision_id: 'revision-1', body: 'comment' } })).toBe(true);
    expect(isReviewCommentRequest({ projectID: 'project-1', payload: { revision_id: 'revision-1', body: 'comment', extra: true } })).toBe(false);
    expect(isReviewDecisionRequest({ projectID: 'project-1', revisionID: 'revision-1', action: 'reject', payload: { reason: 'reason' } })).toBe(true);
    expect(isReviewDecisionRequest({ projectID: 'project-1', revisionID: 'revision-1', action: 'delete', payload: { reason: 'reason' } })).toBe(false);
  });
});
