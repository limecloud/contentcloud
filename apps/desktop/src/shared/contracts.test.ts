import { describe, expect, it } from 'vitest';

import { isCommandResponse, isCreateServerProjectInput, isDesktopAssetSurface, isDesktopDaemonStatus, isDesktopDeliveries, isDesktopFileSelectionResult, isDesktopLoginInput, isEventStream, isPublishWorkspaceInput, isReviewCommentRequest, isReviewDecisionRequest, isReviewRevisionRequest, isServerBootstrap, isServerConnectSession, isSnapshot } from './contracts';

describe('desktop snapshot contract', () => {
  it('accepts only the current schema with a project collection', () => {
    expect(isSnapshot({ schema_version: 'contentcloud.desktop-snapshot/1.0', projects: [] })).toBe(true);
    expect(isSnapshot({ schema_version: 'contentcloud.desktop-snapshot/0.9', projects: [] })).toBe(false);
    expect(isSnapshot({ schema_version: 'contentcloud.desktop-snapshot/1.0', projects: null })).toBe(false);
    expect(isSnapshot({ schema_version: 'contentcloud.desktop-snapshot/1.0', projects: [{ experience: { schema_version: 'contentcloud.desktop-experience/0.9', event_count: 0, success_count: 0, failure_count: 0, recovery_count: 0, pattern_count: 0, patterns: [] } }] })).toBe(false);
    expect(isSnapshot({ schema_version: 'contentcloud.desktop-snapshot/1.0', projects: [{ experience: { schema_version: 'contentcloud.desktop-experience/1.0', event_count: 1, success_count: 1, failure_count: 0, recovery_count: 0, pattern_count: 1, patterns: [{ id: 'publish-confirmed', kind: 'success', title: 'ok', detail: 'detail', status: 'validated', evidence_count: 1, evidence: [{ event_id: 'event-1', cursor: 1, event_type: 'workspace.publish.synced', created_at: new Date().toISOString() }] }] } }] })).toBe(true);
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
    expect(isEventStream({ schema_version: 'contentcloud.desktop-events/1.0', project_id: 'project-1', events: [{ id: 'event-1', project_id: 'project-1', cursor: 1, type: 'project.observed', payload: {}, created_at: 'not-a-date' }], next_cursor: 1, gap: false, resync_required: false })).toBe(false);
  });

  it('validates daemon lifecycle status', () => {
    expect(isDesktopDaemonStatus({ state: 'running', managed: true, pid: 42, version: '0.29.3' })).toBe(true);
    expect(isDesktopDaemonStatus({ state: 'failed', managed: false, message: 'failed' })).toBe(true);
    expect(isDesktopDaemonStatus({ state: 'running', managed: true, pid: 0 })).toBe(false);
  });

  it('validates delivery projection paths', () => {
    expect(isDesktopDeliveries({ packages: [{ id: 'delivery-1', project_name: 'Brand', status: 'ready', files: [{ id: 'artifact-1', file_name: 'script.md', media_type: 'text/markdown', byte_size: 10, href: '/api/studio/artifacts/artifact-1/download' }], created_at: new Date().toISOString() }], publications: [], generated_at: new Date().toISOString() })).toBe(true);
    expect(isDesktopDeliveries({ packages: [], publications: [], generated_at: new Date().toISOString() })).toBe(true);
    expect(isDesktopDeliveries({ packages: [{ id: 'delivery-1', project_name: 'Brand', status: 'ready', files: [{ id: 'artifact-1', file_name: 'script.md', media_type: 'text/markdown', byte_size: 10, href: 'https://evil.example/download' }], created_at: new Date().toISOString() }], publications: [], generated_at: new Date().toISOString() })).toBe(false);
  });
});

describe('server bootstrap contracts', () => {
  it('accepts the authenticated project bootstrap and connect session', () => {
    const bootstrap = {
      session: { user: { id: 'user-1', display_name: 'Owner' }, tenant: { id: 'tenant-1', name: 'Team' }, role: 'tenant_admin', can_create: true, can_connect_execution_client: true, can_manage_team: true },
      tenants: [{ id: 'tenant-1', name: 'Team' }],
      projects: [{ id: 'project-1', brand_name: 'Brand', product_name: 'Product', content_type: 'video_script', channel: 'douyin', status: 'active', execution_client_connected: false, connected_client_count: 0 }],
      generated_at: new Date().toISOString(),
    };
    expect(isServerBootstrap(bootstrap)).toBe(true);
    expect(isServerBootstrap({ ...bootstrap, projects: [{ ...bootstrap.projects[0], connected_client_count: -1 }] })).toBe(false);
    expect(isServerConnectSession({ id: 'session-1', project_id: 'project-1', status: 'waiting_for_computer', message: 'waiting', requires_confirmation: false, expires_at: new Date().toISOString() })).toBe(true);
    expect(isServerConnectSession({ id: 'session-1', project_id: 'project-1', status: 'waiting_for_computer', message: 'waiting', requires_confirmation: 'yes', expires_at: new Date().toISOString() })).toBe(false);
    expect(isCreateServerProjectInput({ brand_name: 'Brand', product_name: 'Product', content_type: 'video_script', channel: 'douyin' })).toBe(true);
    expect(isCreateServerProjectInput({ brand_name: 'Brand', product_name: 'Product', unknown: true })).toBe(false);
  });
});

describe('desktop authentication contracts', () => {
  it('rejects malformed login payloads before IPC reaches the gateway', () => {
    expect(isDesktopLoginInput({ server_url: 'https://content.example.com', email: 'owner@example.com', password: 'password' })).toBe(true);
    expect(isDesktopLoginInput({ server_url: 'http://localhost:8080', email: 'owner@example.com', password: 'password' })).toBe(true);
    expect(isDesktopLoginInput({ server_url: 'http://example.com', email: 'owner@example.com', password: 'password' })).toBe(false);
    expect(isDesktopLoginInput({ server_url: 'https://content.example.com?token=secret', email: 'owner@example.com', password: 'password' })).toBe(false);
    expect(isDesktopLoginInput({ server_url: 'https://content.example.com', email: 'owner@example.com', password: '' })).toBe(false);
    expect(isDesktopLoginInput({ server_url: 'https://content.example.com', email: 'owner@example.com', password: 'password', extra: true })).toBe(false);
  });
});

describe('desktop material contracts', () => {
  it('accepts safe file selections and asset projections', () => {
    expect(isDesktopFileSelectionResult({ status: 'selected', files: [{ id: 'file-1', name: 'brief.md', size: 12, mime_type: 'text/markdown' }] })).toBe(true);
    expect(isDesktopFileSelectionResult({ status: 'selected', files: [{ id: '', name: 'brief.md', size: 12, mime_type: 'text/markdown' }] })).toBe(false);
    expect(isDesktopAssetSurface({
      workspace: { folders: [], materials: [{ ref: 'material-1', project_id: 'project-1', title: 'brief', file_name: 'brief.md', mime_type: 'text/markdown', byte_size: 12, processing_state: 'ready' }], counts: { all: 1 }, generated_at: new Date().toISOString() },
      creative_results: { items: [], counts: {}, generated_at: new Date().toISOString() },
      recent: { materials: [], results: [] }, generated_at: new Date().toISOString(),
    })).toBe(true);
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
