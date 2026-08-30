export type DesktopSnapshotResult =
  | { status: 'ready'; snapshot: DesktopSnapshot }
  | { status: 'offline'; message: string };

export interface DesktopDaemonStatus {
  state: 'running' | 'starting' | 'stopped' | 'failed' | 'unsupported';
  managed: boolean;
  pid?: number;
  version?: string;
  message?: string;
}

export type DesktopAuthResult =
  | { status: 'authenticated'; session: DesktopAuthSession }
  | { status: 'unauthenticated' }
  | { status: 'rejected'; code: string; message: string }
  | { status: 'offline'; message: string };

export interface DesktopAuthSession {
  server_url: string;
  expires_at?: string;
  user: {
    id?: string;
    email: string;
    display_name: string;
  };
  tenant: {
    id?: string;
    name: string;
  };
  role: string;
  is_platform_admin: boolean;
}

export interface DesktopLoginInput {
  server_url: string;
  email: string;
  password: string;
}

export interface DesktopServerBootstrap {
  session: DesktopServerSession;
  tenants: DesktopServerTenant[];
  projects: DesktopServerProject[];
  generated_at: string;
}

export interface DesktopServerSession {
  user: { id: string; display_name: string };
  tenant: { id: string; name: string };
  role: string;
  can_create: boolean;
  can_connect_execution_client: boolean;
  can_manage_team: boolean;
}

export interface DesktopServerTenant {
  id: string;
  name: string;
}

export interface DesktopServerProject {
  id: string;
  brand_name: string;
  product_name: string;
  content_type: string;
  channel: string;
  status: string;
  execution_client_connected: boolean;
  connected_client_count: number;
}

export interface DesktopServerConnectSession {
  id: string;
  project_id: string;
  status: string;
  message: string;
  requires_confirmation: boolean;
  verification_code?: string;
  support_code?: string;
  expires_at: string;
}

export interface DesktopCreateServerProjectInput {
  brand_name: string;
  product_name: string;
  content_type?: string;
  channel?: string;
  stage_objective?: string;
}

export interface DesktopFileSelection {
  id: string;
  name: string;
  size: number;
  mime_type: string;
}

export interface DesktopFileSelectionResult {
  status: 'selected' | 'canceled' | 'rejected';
  files: DesktopFileSelection[];
  message?: string;
}

export interface DesktopMaterialUploadResult {
  file_id: string;
  file_name: string;
  status: 'uploaded' | 'rejected' | 'offline';
  material?: DesktopWorkspaceMaterial;
  message?: string;
}

export type DesktopMaterialPreviewResult =
  | { status: 'ready'; data_url: string; mime_type: string; file_name: string }
  | { status: 'rejected'; message: string }
  | { status: 'offline'; message: string };

export interface DesktopWorkspaceMaterial {
  ref: string;
  folder_ref?: string;
  project_id: string;
  project_name: string;
  material_kind: string;
  origin: string;
  usage: string;
  title: string;
  file_name: string;
  mime_type: string;
  byte_size: number;
  preview_ref?: string;
  processing_state: string;
  rights_summary: string;
  created_at: string;
  updated_at: string;
  last_used_at?: string;
}

export interface DesktopAssetSurface {
  workspace: { folders: Array<Record<string, unknown>>; materials: DesktopWorkspaceMaterial[]; counts: Record<string, number>; generated_at: string };
  creative_results: { items: Array<Record<string, unknown>>; counts: Record<string, number>; generated_at: string };
  recent: { materials: DesktopWorkspaceMaterial[]; results: Array<Record<string, unknown>> };
  generated_at: string;
}

export interface DesktopDeliveryDownload {
  id: string;
  file_name: string;
  media_type: string;
  byte_size: number;
  href: string;
}

export interface DesktopDeliveryPackage {
  id: string;
  project_name: string;
  status: string;
  files: DesktopDeliveryDownload[];
  created_at: string;
}

export interface DesktopPublication {
  id: string;
  project_name: string;
  destination: string;
  account_ref: string;
  status: string;
  external_url?: string;
  published_at?: string;
  updated_at: string;
}

export interface DesktopDeliveries {
  packages: DesktopDeliveryPackage[];
  publications: DesktopPublication[];
  generated_at: string;
}

export type DesktopDeliveryDownloadResult =
  | { status: 'saved'; file_name: string }
  | { status: 'canceled'; file_name: string }
  | { status: 'unauthenticated'; file_name: string }
  | { status: 'rejected'; file_name: string; message: string }
  | { status: 'offline'; file_name: string; message: string };

export type DesktopServerResult<T> =
  | { status: 'ready'; value: T }
  | { status: 'unauthenticated' }
  | { status: 'rejected'; code: string; message: string }
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

export type DesktopAllowedAction = 'workspace.publish' | 'workspace.retry';

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

export interface RetryWorkspaceInput {
  workspace_id: string;
  project_id: string;
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
  getDaemonStatus(): Promise<DesktopDaemonStatus>;
  startDaemon(): Promise<DesktopDaemonStatus>;
  stopDaemon(): Promise<DesktopDaemonStatus>;
  restartDaemon(): Promise<DesktopDaemonStatus>;
  getAuthSession(): Promise<DesktopAuthResult>;
  login(input: DesktopLoginInput): Promise<DesktopAuthResult>;
  logout(): Promise<DesktopAuthResult>;
  getServerBootstrap(): Promise<DesktopServerResult<DesktopServerBootstrap>>;
  createServerProject(input: DesktopCreateServerProjectInput): Promise<DesktopServerResult<DesktopServerBootstrap>>;
  chooseFiles(): Promise<DesktopFileSelectionResult>;
  uploadMaterials(projectID: string, fileIDs: string[]): Promise<DesktopMaterialUploadResult[]>;
  previewMaterial(materialRef: string): Promise<DesktopMaterialPreviewResult>;
  getAssets(projectID: string): Promise<DesktopServerResult<DesktopAssetSurface>>;
  getDeliveries(): Promise<DesktopServerResult<DesktopDeliveries>>;
  downloadDelivery(artifactID: string, fileName: string): Promise<DesktopDeliveryDownloadResult>;
  createConnectSession(projectID: string): Promise<DesktopServerResult<DesktopServerConnectSession>>;
  getConnectSession(sessionID: string): Promise<DesktopServerResult<DesktopServerConnectSession>>;
  cancelConnectSession(sessionID: string): Promise<DesktopServerResult<DesktopServerConnectSession>>;
  getProjectEvents(projectID: string, after: number): Promise<DesktopEventStreamResult>;
  publishWorkspace(input: PublishWorkspaceInput): Promise<DesktopCommandResult>;
  retryWorkspace(input: RetryWorkspaceInput): Promise<DesktopCommandResult>;
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

export function isDesktopDaemonStatus(value: unknown): value is DesktopDaemonStatus {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<DesktopDaemonStatus>;
  return ['running', 'starting', 'stopped', 'failed', 'unsupported'].includes(String(candidate.state))
    && typeof candidate.managed === 'boolean'
    && (candidate.pid === undefined || (Number.isSafeInteger(candidate.pid) && Number(candidate.pid) > 0))
    && (candidate.version === undefined || typeof candidate.version === 'string')
    && (candidate.message === undefined || typeof candidate.message === 'string');
}

export function isServerBootstrap(value: unknown): value is DesktopServerBootstrap {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<DesktopServerBootstrap>;
  return isServerSession(candidate.session)
    && Array.isArray(candidate.tenants) && candidate.tenants.every(isServerTenant)
    && Array.isArray(candidate.projects) && candidate.projects.every(isServerProject)
    && isBoundedString(candidate.generated_at);
}

export function isServerConnectSession(value: unknown): value is DesktopServerConnectSession {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<DesktopServerConnectSession>;
  return isBoundedString(candidate.id)
    && isBoundedString(candidate.project_id)
    && isBoundedString(candidate.status)
    && typeof candidate.message === 'string'
    && typeof candidate.requires_confirmation === 'boolean'
    && isBoundedString(candidate.expires_at)
    && (candidate.verification_code === undefined || isBoundedString(candidate.verification_code))
    && (candidate.support_code === undefined || isBoundedString(candidate.support_code));
}

export function isCreateServerProjectInput(value: unknown): value is DesktopCreateServerProjectInput {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<DesktopCreateServerProjectInput>;
  return hasOnlyKeys(candidate, ['brand_name', 'product_name', 'content_type', 'channel', 'stage_objective'])
    && isBoundedString(candidate.brand_name)
    && isBoundedString(candidate.product_name)
    && (candidate.content_type === undefined || isBoundedString(candidate.content_type))
    && (candidate.channel === undefined || isBoundedString(candidate.channel))
    && (candidate.stage_objective === undefined || typeof candidate.stage_objective === 'string');
}

export function isDesktopFileSelectionResult(value: unknown): value is DesktopFileSelectionResult {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<DesktopFileSelectionResult>;
  return (candidate.status === 'selected' || candidate.status === 'canceled' || candidate.status === 'rejected')
    && Array.isArray(candidate.files)
    && candidate.files.every((file) => Boolean(file && typeof file === 'object' && isBoundedString((file as DesktopFileSelection).id) && isBoundedString((file as DesktopFileSelection).name) && Number.isSafeInteger((file as DesktopFileSelection).size) && Number((file as DesktopFileSelection).size) >= 0 && typeof (file as DesktopFileSelection).mime_type === 'string'))
    && (candidate.message === undefined || typeof candidate.message === 'string');
}

export function isDesktopAssetSurface(value: unknown): value is DesktopAssetSurface {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<DesktopAssetSurface>;
  const workspace = candidate.workspace as DesktopAssetSurface['workspace'] | undefined;
  const creative = candidate.creative_results as DesktopAssetSurface['creative_results'] | undefined;
  return Boolean(workspace && Array.isArray(workspace.folders) && Array.isArray(workspace.materials) && workspace.materials.every(isDesktopWorkspaceMaterial) && workspace.counts && typeof workspace.counts === 'object')
    && Boolean(creative && Array.isArray(creative.items) && creative.counts && typeof creative.counts === 'object')
    && Boolean(candidate.recent && typeof candidate.recent === 'object' && Array.isArray(candidate.recent.materials) && Array.isArray(candidate.recent.results))
    && typeof candidate.generated_at === 'string';
}

export function isDesktopDeliveries(value: unknown): value is DesktopDeliveries {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<DesktopDeliveries>;
  return Array.isArray(candidate.packages) && candidate.packages.every(isDesktopDeliveryPackage)
    && Array.isArray(candidate.publications) && candidate.publications.every(isDesktopPublication)
    && isBoundedString(candidate.generated_at);
}

export function isDesktopDeliveryDownloadResult(value: unknown): value is DesktopDeliveryDownloadResult {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<Extract<DesktopDeliveryDownloadResult, { status: string }>>;
  if (typeof candidate.file_name !== 'string') return false;
  if (candidate.status === 'saved' || candidate.status === 'canceled' || candidate.status === 'unauthenticated') return true;
  return (candidate.status === 'rejected' || candidate.status === 'offline') && typeof candidate.message === 'string';
}

function isDesktopDeliveryPackage(value: unknown): value is DesktopDeliveryPackage {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<DesktopDeliveryPackage>;
  return isBoundedString(candidate.id) && typeof candidate.project_name === 'string' && typeof candidate.status === 'string'
    && isBoundedString(candidate.created_at) && Array.isArray(candidate.files) && candidate.files.every(isDesktopDeliveryDownload);
}

function isDesktopDeliveryDownload(value: unknown): value is DesktopDeliveryDownload {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<DesktopDeliveryDownload>;
  return isBoundedString(candidate.id) && typeof candidate.file_name === 'string' && typeof candidate.media_type === 'string'
    && Number.isSafeInteger(candidate.byte_size) && Number(candidate.byte_size) >= 0 && typeof candidate.href === 'string' && candidate.href.startsWith('/api/studio/artifacts/');
}

function isDesktopPublication(value: unknown): value is DesktopPublication {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<DesktopPublication>;
  return isBoundedString(candidate.id) && typeof candidate.project_name === 'string' && typeof candidate.destination === 'string'
    && typeof candidate.account_ref === 'string' && typeof candidate.status === 'string' && isBoundedString(candidate.updated_at)
    && (candidate.external_url === undefined || typeof candidate.external_url === 'string')
    && (candidate.published_at === undefined || typeof candidate.published_at === 'string');
}

function isDesktopWorkspaceMaterial(value: unknown): value is DesktopWorkspaceMaterial {
  if (!value || typeof value !== 'object') return false;
  const material = value as Partial<DesktopWorkspaceMaterial>;
  return isBoundedString(material.ref) && isBoundedString(material.project_id) && typeof material.title === 'string' && typeof material.file_name === 'string'
    && typeof material.mime_type === 'string' && Number.isSafeInteger(material.byte_size) && Number(material.byte_size) >= 0 && typeof material.processing_state === 'string';
}

export function isDesktopLoginInput(value: unknown): value is DesktopLoginInput {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<DesktopLoginInput>;
  return hasOnlyKeys(candidate, ['server_url', 'email', 'password'])
    && typeof candidate.server_url === 'string'
    && isSafeServerURL(candidate.server_url)
    && isBoundedString(candidate.email)
    && typeof candidate.password === 'string'
    && candidate.password.length > 0
    && candidate.password.length <= 4096;
}

function isServerSession(value: unknown): value is DesktopServerSession {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<DesktopServerSession>;
  return Boolean(candidate.user && typeof candidate.user === 'object' && isBoundedString(candidate.user.id) && isBoundedString(candidate.user.display_name))
    && Boolean(candidate.tenant && typeof candidate.tenant === 'object' && isBoundedString(candidate.tenant.id) && isBoundedString(candidate.tenant.name))
    && isBoundedString(candidate.role)
    && typeof candidate.can_create === 'boolean'
    && typeof candidate.can_connect_execution_client === 'boolean'
    && typeof candidate.can_manage_team === 'boolean';
}

function isServerTenant(value: unknown): value is DesktopServerTenant {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<DesktopServerTenant>;
  return isBoundedString(candidate.id) && isBoundedString(candidate.name);
}

function isServerProject(value: unknown): value is DesktopServerProject {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<DesktopServerProject>;
  return isBoundedString(candidate.id)
    && typeof candidate.brand_name === 'string'
    && typeof candidate.product_name === 'string'
    && isBoundedString(candidate.content_type)
    && typeof candidate.channel === 'string'
    && isBoundedString(candidate.status)
    && typeof candidate.execution_client_connected === 'boolean'
    && Number.isSafeInteger(candidate.connected_client_count)
    && Number(candidate.connected_client_count) >= 0;
}

export function isPublishWorkspaceInput(value: unknown): value is PublishWorkspaceInput {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<PublishWorkspaceInput>;
  return [candidate.workspace_id, candidate.project_id, candidate.base_revision].every(isBoundedString)
    && typeof candidate.observed_digest === 'string'
    && /^sha256:[0-9a-f]{64}$/.test(candidate.observed_digest);
}

export function isRetryWorkspaceInput(value: unknown): value is RetryWorkspaceInput {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<RetryWorkspaceInput>;
  return hasOnlyKeys(candidate, ['workspace_id', 'project_id'])
    && isBoundedString(candidate.workspace_id)
    && isBoundedString(candidate.project_id);
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
    && candidate.events.every(isDesktopEvent)
    && Number.isSafeInteger(candidate.next_cursor)
    && Number(candidate.next_cursor) >= 0
    && typeof candidate.gap === 'boolean'
    && typeof candidate.resync_required === 'boolean';
}

function isDesktopEvent(value: unknown): value is DesktopEvent {
  if (!value || typeof value !== 'object') return false;
  const event = value as Partial<DesktopEvent>;
  return isBoundedString(event.id)
    && isBoundedString(event.project_id)
    && Number.isSafeInteger(event.cursor)
    && Number(event.cursor) >= 0
    && isBoundedString(event.type)
    && typeof event.created_at === 'string'
    && !Number.isNaN(Date.parse(event.created_at))
    && Boolean(event.payload && typeof event.payload === 'object' && !Array.isArray(event.payload));
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

function isSafeServerURL(value: string): boolean {
  if (!isBoundedString(value)) return false;
  try {
    const parsed = new URL(value.trim());
    return (parsed.protocol === 'https:' || (parsed.protocol === 'http:' && ['127.0.0.1', 'localhost'].includes(parsed.hostname)))
      && !parsed.username && !parsed.password && !parsed.search && !parsed.hash;
  } catch {
    return false;
  }
}

function hasOnlyKeys(value: object, allowed: string[]): boolean {
  const keys = Object.keys(value);
  return keys.every((key) => allowed.includes(key));
}
