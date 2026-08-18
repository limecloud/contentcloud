export type DesktopSnapshotResult =
  | { status: 'ready'; snapshot: DesktopSnapshot }
  | { status: 'offline'; message: string };

export interface DesktopSnapshot {
  schema_version: 'contentcloud.desktop-snapshot/1.0';
  daemon: { connected: boolean; version: string };
  projects: ProjectSnapshot[];
  generated_at: string;
}

export interface ProjectSnapshot {
  project_id: string;
  workspace_id: string;
  name: string;
  local_state: 'clean' | 'modified' | 'deleted' | 'conflict';
  transfer_state: 'idle' | 'queued' | 'hashing' | 'uploading' | 'downloading' | 'synced' | 'failed';
  review_state: 'unsubmitted' | 'pending' | 'changes_requested' | 'approved' | 'rejected' | 'expired';
  lifecycle_state: 'draft' | 'ready' | 'delivered' | 'archived';
  runtime_state: 'queued' | 'running' | 'waiting' | 'paused' | 'cancelled' | 'failed' | 'succeeded';
  content: ContentSection[];
  pending_feedback: number;
  pending_decision: number;
  source_count: number;
  last_synced_at?: string;
  local_revision: number;
  observed_digest?: string;
  cloud_revision: string;
  cloud_event_cursor: number;
  synced_digest?: string;
  event_cursor: number;
  allowed_actions: DesktopAllowedAction[];
  error_code?: string;
}

export type DesktopAllowedAction = 'workspace.publish';

export interface ContentSection {
  ref: string;
  label: string;
  items: ContentDirectoryEntry[];
}

export interface ContentDirectoryEntry {
  ref: string;
  kind: string;
  byte_size?: number;
  mime_type?: string;
}

export interface DesktopAppInfo {
  name: 'Content Work OS Desktop';
  version: string;
  platform: string;
  electron: string;
}

export interface PublishWorkspaceInput {
  workspace_id: string;
  project_id: string;
  base_revision: string;
  observed_digest: string;
}

export interface DesktopCommandResponse {
  schema_version: 'contentcloud.desktop-command-result/1.0';
  request_id: string;
  command_id: string;
  project_id: string;
  state: 'queued';
  event_cursor: number;
  accepted_at: string;
}

export type DesktopCommandResult =
  | { status: 'accepted'; command: DesktopCommandResponse }
  | { status: 'rejected'; code: string }
  | { status: 'offline'; message: string };

export interface DesktopEvent {
  id: string;
  project_id: string;
  cursor: number;
  type: string;
  payload: Record<string, unknown>;
  created_at: string;
}

export interface DesktopEventStream {
  schema_version: 'contentcloud.desktop-events/1.0';
  project_id: string;
  events: DesktopEvent[];
  next_cursor: number;
  gap: boolean;
  resync_required: boolean;
}

export type DesktopEventStreamResult =
  | { status: 'ready'; stream: DesktopEventStream }
  | { status: 'offline'; message: string };

export interface DesktopReviewComment {
  id: string;
  project_id: string;
  subject_id: string;
  json_pointer?: string;
  body: string;
  visibility: string;
  author_id: string;
  resolved_at?: string;
  created_at: string;
}

export interface DesktopReviewObject {
  id: string;
  type: string;
  version: number;
  digest: string;
  path: string;
  content: unknown;
}

export interface DesktopReviewSubmission {
  id: string;
  project_id: string;
  workspace_id: string;
  submission_type: string;
  status: string;
  current_revision_id: string;
  updated_at: string;
}

export interface DesktopReviewRevision {
  id: string;
  project_id: string;
  submission_id: string;
  revision_no: number;
  schema_version: string;
  content_hash: string;
  objects: DesktopReviewObject[];
  message?: string;
  evidence_limited: boolean;
  created_at: string;
}

export interface DesktopReviewInboxItem {
  submission: DesktopReviewSubmission;
  revision: DesktopReviewRevision;
  pending_comments: number;
  allowed_actions: DesktopReviewAction[];
}

export interface DesktopReviewInbox {
  project_id: string;
  items: DesktopReviewInboxItem[];
}

export type DesktopReviewAction = 'comment' | 'approve' | 'reject' | 'request_changes';

export interface DesktopReviewObjectDiff {
  object_id: string;
  object_type: string;
  path: string;
  change: 'added' | 'modified' | 'unchanged' | 'removed';
  base_digest?: string;
  current_digest?: string;
  base_content?: string;
  current_content?: string;
}

export interface DesktopReviewRevisionDetail {
  submission: DesktopReviewSubmission;
  revision: DesktopReviewRevision;
  previous_revision?: DesktopReviewRevision;
  comments: DesktopReviewComment[];
  diffs: DesktopReviewObjectDiff[];
  allowed_actions: DesktopReviewAction[];
}

export type DesktopReviewResult<T> =
  | { status: 'ready'; value: T }
  | { status: 'rejected'; code: string }
  | { status: 'offline'; message: string };

export interface DesktopReviewCommentInput {
  revision_id: string;
  body: string;
  json_pointer?: string;
}

export interface DesktopReviewDecisionInput {
  revision_id: string;
  reason: string;
  json_pointer?: string;
}

export interface DesktopReviewRevisionRequest {
  projectID: string;
  revisionID: string;
}

export interface DesktopReviewCommentRequest {
  projectID: string;
  payload: DesktopReviewCommentInput;
}

export interface DesktopReviewDecisionRequest {
  projectID: string;
  revisionID: string;
  action: 'approve' | 'reject' | 'request-changes';
  payload?: Omit<DesktopReviewDecisionInput, 'revision_id'>;
}

export interface DesktopApi {
  getSnapshot(): Promise<DesktopSnapshotResult>;
  publishWorkspace(input: PublishWorkspaceInput): Promise<DesktopCommandResult>;
  getAppInfo(): Promise<DesktopAppInfo>;
  onSnapshotChanged(listener: (result: DesktopSnapshotResult) => void): () => void;
  getReviewInbox(projectID: string): Promise<DesktopReviewResult<DesktopReviewInbox>>;
  getReviewRevision(projectID: string, revisionID: string): Promise<DesktopReviewResult<DesktopReviewRevisionDetail>>;
  addReviewComment(projectID: string, input: DesktopReviewCommentInput): Promise<DesktopReviewResult<DesktopReviewComment>>;
  decideReview(projectID: string, revisionID: string, action: 'approve' | 'reject' | 'request-changes', input: Omit<DesktopReviewDecisionInput, 'revision_id'>): Promise<DesktopReviewResult<unknown>>;
}

declare global {
  interface Window {
    contentcloudDesktop: DesktopApi;
  }
}

export function isSnapshot(value: unknown): value is DesktopSnapshot {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<DesktopSnapshot>;
  return candidate.schema_version === 'contentcloud.desktop-snapshot/1.0' && Array.isArray(candidate.projects);
}

export function isPublishWorkspaceInput(value: unknown): value is PublishWorkspaceInput {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<PublishWorkspaceInput>;
  return [candidate.workspace_id, candidate.project_id, candidate.base_revision].every(isBoundedString)
    && typeof candidate.observed_digest === 'string'
    && /^sha256:[0-9a-f]{64}$/.test(candidate.observed_digest);
}

export function isCommandResponse(value: unknown): value is DesktopCommandResponse {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<DesktopCommandResponse>;
  return candidate.schema_version === 'contentcloud.desktop-command-result/1.0'
    && candidate.state === 'queued'
    && [candidate.request_id, candidate.command_id, candidate.project_id, candidate.accepted_at].every(isBoundedString)
    && Number.isSafeInteger(candidate.event_cursor) && Number(candidate.event_cursor) >= 0;
}

export function isEventStream(value: unknown): value is DesktopEventStream {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<DesktopEventStream>;
  return candidate.schema_version === 'contentcloud.desktop-events/1.0'
    && isBoundedString(candidate.project_id)
    && Array.isArray(candidate.events)
    && candidate.events.every((event) => Boolean(event) && typeof event === 'object' && Number.isSafeInteger((event as DesktopEvent).cursor))
    && Number.isSafeInteger(candidate.next_cursor)
    && typeof candidate.gap === 'boolean'
    && typeof candidate.resync_required === 'boolean';
}

export function isReviewInbox(value: unknown): value is DesktopReviewInbox {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<DesktopReviewInbox>;
  return isBoundedString(candidate.project_id) && Array.isArray(candidate.items);
}

export function isReviewRevision(value: unknown): value is DesktopReviewRevisionDetail {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<DesktopReviewRevisionDetail>;
  return Boolean(candidate.submission && candidate.revision && Array.isArray(candidate.comments) && Array.isArray(candidate.diffs));
}

export function isReviewRevisionRequest(value: unknown): value is DesktopReviewRevisionRequest {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<DesktopReviewRevisionRequest>;
  return hasOnlyKeys(candidate, ['projectID', 'revisionID'])
    && isBoundedString(candidate.projectID) && isBoundedString(candidate.revisionID);
}

export function isReviewCommentRequest(value: unknown): value is DesktopReviewCommentRequest {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<DesktopReviewCommentRequest>;
  const payload = candidate.payload as Partial<DesktopReviewCommentInput> | undefined;
  return hasOnlyKeys(candidate, ['projectID', 'payload'])
    && isBoundedString(candidate.projectID)
    && Boolean(payload && typeof payload === 'object')
    && payload !== undefined
    && hasOnlyKeys(payload, ['revision_id', 'body', 'json_pointer'])
    && isBoundedString(payload?.revision_id)
    && typeof payload?.body === 'string';
}

export function isReviewDecisionRequest(value: unknown): value is DesktopReviewDecisionRequest {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<DesktopReviewDecisionRequest>;
  if (!hasOnlyKeys(candidate, ['projectID', 'revisionID', 'action', 'payload']) || !isBoundedString(candidate.projectID) || !isBoundedString(candidate.revisionID)) return false;
  if (candidate.action !== 'approve' && candidate.action !== 'reject' && candidate.action !== 'request-changes') return false;
  if (candidate.payload === undefined) return true;
  const payload = candidate.payload as Partial<DesktopReviewDecisionInput> | undefined;
  return Boolean(payload && typeof payload === 'object')
    && payload !== undefined
    && hasOnlyKeys(payload, ['reason', 'json_pointer'])
    && typeof payload?.reason === 'string';
}

function isBoundedString(value: unknown): value is string {
  return typeof value === 'string' && value.length > 0 && value.length <= 256;
}

function hasOnlyKeys(value: object, allowed: string[]): boolean {
  const keys = Object.keys(value);
  return keys.every((key) => allowed.includes(key));
}
