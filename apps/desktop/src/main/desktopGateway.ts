import { app } from 'electron';
import { createHash, randomUUID } from 'node:crypto';
import { readFile } from 'node:fs/promises';
import { join } from 'node:path';

import type {
	DesktopReviewComment,
	DesktopReviewCommentInput,
	DesktopReviewDecisionInput,
	DesktopReviewInbox,
	DesktopReviewResult,
	DesktopReviewRevisionDetail,
  DesktopCommandResult,
  DesktopEventStream,
  DesktopEventStreamResult,
  DesktopSnapshot,
  DesktopSnapshotResult,
  PublishWorkspaceInput,
} from '../shared/contracts';
import { isCommandResponse, isEventStream, isPublishWorkspaceInput, isReviewInbox, isReviewRevision, isSnapshot } from '../shared/contracts';

const apiVersion = '1.0';
const apiVersionHeader = 'X-ContentCloud-Desktop-API-Version';

interface DiscoveryFile {
  schema_version: 'contentcloud.desktop-api-discovery/1.0';
  endpoint: string;
  capability: string;
  api_versions: string[];
}

interface Capabilities {
  schema_version: 'contentcloud.desktop-api-capabilities/1.0';
  api_versions: string[];
  snapshot_schema: string;
  command_schema: string;
  event_schema: string;
  commands: string[];
}

class DaemonResponseError extends Error {
  constructor(readonly status: number, readonly code: string) {
    super(`Daemon returned ${status}: ${code}`);
  }
}

function discoveryPath(): string {
  return join(app.getPath('appData'), 'contentcloud', 'desktop-api.json');
}

async function loadDiscovery(): Promise<DiscoveryFile> {
  const body = await readFile(discoveryPath(), 'utf8');
  const value: unknown = JSON.parse(body);
  if (!value || typeof value !== 'object') throw new Error('Daemon discovery is invalid');
  const discovery = value as Partial<DiscoveryFile>;
  if (discovery.schema_version !== 'contentcloud.desktop-api-discovery/1.0') throw new Error('Daemon discovery schema is unsupported');
  if (!discovery.endpoint?.startsWith('http://127.0.0.1:')) throw new Error('Daemon endpoint is not loopback');
  if (!discovery.capability || discovery.capability.length < 32) throw new Error('Daemon capability is missing');
  if (!Array.isArray(discovery.api_versions) || !discovery.api_versions.includes(apiVersion)) throw new Error('Daemon API version is unsupported');
  return discovery as DiscoveryFile;
}

async function connect(): Promise<DiscoveryFile> {
  const discovery = await loadDiscovery();
  const capabilities = await daemonFetch<Capabilities>(discovery, '/v1/capabilities');
  if (capabilities.schema_version !== 'contentcloud.desktop-api-capabilities/1.0'
    || !capabilities.api_versions.includes(apiVersion)
    || capabilities.snapshot_schema !== 'contentcloud.desktop-snapshot/1.0'
    || capabilities.command_schema !== 'contentcloud.desktop-command/1.0'
    || capabilities.event_schema !== 'contentcloud.desktop-events/1.0') {
    throw new Error('Daemon capabilities are incompatible');
  }
  return discovery;
}

async function daemonFetch<T>(discovery: DiscoveryFile, path: string, init: RequestInit = {}): Promise<T> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 4000);
  try {
    const headers = new Headers(init.headers);
    headers.set('Authorization', `Bearer ${discovery.capability}`);
    if (path !== '/v1/capabilities') headers.set(apiVersionHeader, apiVersion);
    const response = await fetch(`${discovery.endpoint}${path}`, { ...init, headers, signal: controller.signal });
    const value: unknown = await response.json();
    if (!response.ok) {
      const code = value && typeof value === 'object'
        ? String((value as { error?: { code?: unknown } }).error?.code ?? 'DESKTOP_API_ERROR')
        : 'DESKTOP_API_ERROR';
      throw new DaemonResponseError(response.status, code);
    }
    return value as T;
  } finally {
    clearTimeout(timeout);
  }
}

export async function requestSnapshot(): Promise<DesktopSnapshotResult> {
  try {
    const discovery = await connect();
    const value: unknown = await daemonFetch<DesktopSnapshot>(discovery, '/v1/snapshot');
    if (!isSnapshot(value)) throw new Error('Daemon snapshot schema is unsupported');
    return { status: 'ready', snapshot: value };
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Daemon is unavailable';
    return { status: 'offline', message };
  }
}

export async function publishWorkspace(input: PublishWorkspaceInput): Promise<DesktopCommandResult> {
  if (!isPublishWorkspaceInput(input)) return { status: 'rejected', code: 'DESKTOP_COMMAND_INPUT_INVALID' };
  try {
    const discovery = await connect();
    const identity = [input.workspace_id, input.project_id, input.base_revision, input.observed_digest].join('\n');
    const value: unknown = await daemonFetch(discovery, '/v1/commands/workspace-publish', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        schema_version: 'contentcloud.desktop-command/1.0',
        request_id: `dreq_${randomUUID()}`,
        workspace_id: input.workspace_id,
        project_id: input.project_id,
        subject_ref: 'workspace',
        base_revision: input.base_revision,
        observed_digest: input.observed_digest,
        idempotency_key: `dpk_${createHash('sha256').update(identity).digest('hex')}`,
      }),
    });
    if (!isCommandResponse(value)) throw new Error('Daemon command response schema is unsupported');
    return { status: 'accepted', command: value };
  } catch (error) {
    if (error instanceof DaemonResponseError && error.status >= 400 && error.status < 500) {
      return { status: 'rejected', code: error.code };
    }
    return { status: 'offline', message: error instanceof Error ? error.message : 'Daemon is unavailable' };
  }
}

export async function requestProjectEvents(projectID: string, after: number): Promise<DesktopEventStreamResult> {
  try {
    const discovery = await connect();
    const value: unknown = await daemonFetch<DesktopEventStream>(discovery, `/v1/projects/${encodeURIComponent(projectID)}/events?after=${after}&limit=100`);
    if (!isEventStream(value)) throw new Error('Daemon event schema is unsupported');
    return { status: 'ready', stream: value };
  } catch (error) {
    return { status: 'offline', message: error instanceof Error ? error.message : 'Daemon is unavailable' };
  }
}

async function requestReview<T>(path: string, validator: (value: unknown) => value is T, init?: RequestInit): Promise<DesktopReviewResult<T>> {
  try {
    const discovery = await connect();
    const value: unknown = await daemonFetch(discovery, path, init);
    if (!validator(value)) throw new Error('Daemon review schema is unsupported');
    return { status: 'ready', value };
  } catch (error) {
    if (error instanceof DaemonResponseError && error.status >= 400 && error.status < 500) return { status: 'rejected', code: error.code };
    return { status: 'offline', message: error instanceof Error ? error.message : 'Daemon is unavailable' };
  }
}

export function requestReviewInbox(projectID: string): Promise<DesktopReviewResult<DesktopReviewInbox>> {
  return requestReview(`/v1/projects/${encodeURIComponent(projectID)}/review/inbox`, isReviewInbox);
}

export function requestReviewRevision(projectID: string, revisionID: string): Promise<DesktopReviewResult<DesktopReviewRevisionDetail>> {
  return requestReview(`/v1/projects/${encodeURIComponent(projectID)}/review/revisions/${encodeURIComponent(revisionID)}`, isReviewRevision);
}

export function addReviewComment(projectID: string, input: DesktopReviewCommentInput): Promise<DesktopReviewResult<DesktopReviewComment>> {
  return requestReview(`/v1/projects/${encodeURIComponent(projectID)}/review/comments`, (value): value is DesktopReviewComment => Boolean(value && typeof value === 'object' && typeof (value as DesktopReviewComment).id === 'string'), {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input),
  });
}

export function decideReview(projectID: string, revisionID: string, action: 'approve' | 'reject' | 'request-changes', input: Omit<DesktopReviewDecisionInput, 'revision_id'>): Promise<DesktopReviewResult<unknown>> {
  return requestReview(`/v1/projects/${encodeURIComponent(projectID)}/review/revisions/${encodeURIComponent(revisionID)}/${action}`, (_value: unknown): _value is unknown => true, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input),
  });
}
