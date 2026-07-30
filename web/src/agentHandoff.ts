import { api } from './api';

export type AgentClientID = 'codex' | 'claude-code' | 'workbuddy' | 'cursor' | 'hermes' | 'openclaw';
export type AgentCapabilityID = 'local_automation' | 'workspace_registration' | 'workspace_bootstrap' | 'interactive_handoff' | 'creative_environment';
export type AgentSupportStatus = 'available' | 'planned';

export interface AgentClient {
  id: AgentClientID;
  display_name: string;
  capabilities: Array<{ id: AgentCapabilityID; status: AgentSupportStatus }>;
}

export interface AgentHandoff {
  schema_version: 'contentcloud.agent-handoff/1.0';
  client: AgentClient;
  kind: 'project' | 'review_feedback';
  project_id: string;
  target: { kind: 'project' | 'submission_revision'; id: string; digest?: string };
  integration: { kind: string; id: string; version: string };
  requires_new_session: true;
  requires_workspace_selection: true;
  launch: { mode: 'deep_link'; url: string };
  prompt: string;
  steps: string[];
  fallback_url: string;
}

export interface AgentHandoffExpectation {
  client: AgentClient;
  projectID: string;
  targetKind: AgentHandoff['target']['kind'];
  targetID: string;
  digest?: string;
}

const clientIDs: AgentClientID[] = ['codex', 'claude-code', 'workbuddy', 'cursor', 'hermes', 'openclaw'];
const capabilityIDs: AgentCapabilityID[] = ['local_automation', 'workspace_registration', 'workspace_bootstrap', 'interactive_handoff', 'creative_environment'];
const expectedHandoffKeys = new Set([
  'schema_version', 'client', 'kind', 'project_id', 'target', 'integration', 'requires_new_session',
  'requires_workspace_selection', 'launch', 'prompt', 'steps', 'fallback_url',
]);

export async function loadAgentClients(): Promise<AgentClient[]> {
  const value = await api<unknown>('/api/bff/agent-clients');
  if (!isRecord(value) || value.schema_version !== 'contentcloud.agent-client-catalog/1.0' || !Array.isArray(value.clients)) {
    throw new Error('Agent 客户端目录结构不受支持');
  }
  const clients = value.clients.map(validateAgentClient);
  if (clients.length !== clientIDs.length || new Set(clients.map(client => client.id)).size !== clientIDs.length || clientIDs.some(id => !clients.some(client => client.id === id))) {
    throw new Error('Agent 客户端目录不完整');
  }
  return clients;
}

export async function loadProjectAgentHandoff(projectID: string, client: AgentClient): Promise<AgentHandoff> {
  const value = await api<unknown>(`/api/bff/projects/${encodeURIComponent(projectID)}/agent-handoff?client=${encodeURIComponent(client.id)}`);
  return validateAgentHandoff(value, { client, projectID, targetKind: 'project', targetID: projectID });
}

export async function loadReviewFeedbackAgentHandoff(projectID: string, revisionID: string, digest: string, client: AgentClient): Promise<AgentHandoff> {
  const value = await api<unknown>(`/api/bff/projects/${encodeURIComponent(projectID)}/submission-revisions/${encodeURIComponent(revisionID)}/agent-handoff?client=${encodeURIComponent(client.id)}`);
  return validateAgentHandoff(value, { client, projectID, targetKind: 'submission_revision', targetID: revisionID, digest });
}

export function validateAgentHandoff(value: unknown, expectation: AgentHandoffExpectation): AgentHandoff {
  if (!isRecord(value) || Object.keys(value).some(key => !expectedHandoffKeys.has(key))) {
    throw new Error('Agent 恢复响应结构不受支持');
  }
  const client = validateAgentClient(value.client);
  const target = value.target;
  const integration = value.integration;
  const launch = value.launch;
  if (!sameClient(client, expectation.client) || !isRecord(target) || !isSafeID(value.project_id) || !isSafeID(target.id) || !isRecord(integration) || !isRecord(launch)) {
    throw new Error('Agent 恢复目标无效');
  }
  const expectedKind = expectation.targetKind === 'project' ? 'project' : 'review_feedback';
  if (value.schema_version !== 'contentcloud.agent-handoff/1.0' || value.project_id !== expectation.projectID || value.kind !== expectedKind || target.kind !== expectation.targetKind || target.id !== expectation.targetID) {
    throw new Error('Agent 恢复目标与当前页面不一致');
  }
  if (expectation.digest && target.digest !== normalizeDigest(expectation.digest)) {
    throw new Error('Agent 恢复摘要与当前页面不一致');
  }
  if (target.digest !== undefined && !isDigest(target.digest)) {
    throw new Error('Agent 恢复摘要无效');
  }
  if (value.requires_new_session !== true || value.requires_workspace_selection !== true || typeof value.prompt !== 'string' || !Array.isArray(value.steps) || value.steps.some(step => typeof step !== 'string' || step.length === 0) || typeof value.fallback_url !== 'string') {
    throw new Error('Agent 恢复门禁无效');
  }
  validateClientHandoff(client.id, integration, launch, value.prompt, value.fallback_url, expectation);
  return value as unknown as AgentHandoff;
}

export function capabilityStatus(client: AgentClient, capability: AgentCapabilityID): AgentSupportStatus {
  return client.capabilities.find(item => item.id === capability)?.status ?? 'planned';
}

export function normalizeDigest(value: string): string {
  const normalized = value.trim().toLowerCase();
  return normalized.startsWith('sha256:') ? normalized : `sha256:${normalized}`;
}

function validateAgentClient(value: unknown): AgentClient {
  if (!isRecord(value) || Object.keys(value).some(key => !['id', 'display_name', 'capabilities'].includes(key)) || !clientIDs.includes(value.id) || typeof value.display_name !== 'string' || !value.display_name.trim() || !Array.isArray(value.capabilities)) {
    throw new Error('Agent 客户端定义无效');
  }
  const capabilities = value.capabilities;
  if (capabilities.length !== capabilityIDs.length || new Set(capabilities.map(item => isRecord(item) ? item.id : '')).size !== capabilityIDs.length) {
    throw new Error('Agent 客户端能力目录不完整');
  }
  for (const capability of capabilities) {
    if (!isRecord(capability) || Object.keys(capability).some(key => !['id', 'status'].includes(key)) || !capabilityIDs.includes(capability.id) || (capability.status !== 'available' && capability.status !== 'planned')) {
      throw new Error('Agent 客户端能力定义无效');
    }
  }
  return value as unknown as AgentClient;
}

function validateClientHandoff(clientID: AgentClientID, integration: Record<string, unknown>, launch: Record<string, unknown>, prompt: string, fallbackURL: string, expectation: AgentHandoffExpectation): void {
  switch (clientID) {
    case 'codex':
      if (integration.kind !== 'plugin' || integration.id !== 'contentcloud-video-production@contentcloud' || integration.version !== '0.10.0' || launch.mode !== 'deep_link' || fallbackURL !== '/codex' || !parseCodexLaunchURL(launch.url, prompt) || !promptBindsTarget(prompt, integration.id, expectation)) {
        throw new Error('Codex 恢复适配器契约无效');
      }
      return;
    default:
      throw new Error(`${clientID} 恢复适配器尚未实现`);
  }
}

function promptBindsTarget(prompt: string, integrationID: unknown, expectation: AgentHandoffExpectation): boolean {
  if (typeof integrationID !== 'string') return false;
  const required = [`plugin://${integrationID}`, 'workspace_context', expectation.projectID];
  if (expectation.targetKind === 'submission_revision') {
    required.push(expectation.targetID, 'review_feedback_list');
    if (!expectation.digest) return false;
    required.push(normalizeDigest(expectation.digest));
  }
  return required.every(value => prompt.includes(value));
}

function parseCodexLaunchURL(value: unknown, prompt: string): URL | undefined {
  if (typeof value !== 'string') return undefined;
  let launch: URL;
  try { launch = new URL(value); } catch { return undefined; }
  if (launch.protocol !== 'codex:' || launch.hostname !== 'new' || launch.pathname !== '' || launch.hash || launch.username || launch.password || launch.port) return undefined;
  const keys = [...launch.searchParams.keys()];
  if (keys.length !== 1 || keys[0] !== 'prompt' || launch.searchParams.get('prompt') !== prompt) return undefined;
  return launch;
}

function sameClient(left: AgentClient, right: AgentClient): boolean {
  return left.id === right.id && left.display_name === right.display_name && JSON.stringify(left.capabilities) === JSON.stringify(right.capabilities);
}

function isRecord(value: unknown): value is Record<string, any> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isSafeID(value: unknown): value is string {
  return typeof value === 'string' && /^[A-Za-z0-9][A-Za-z0-9._:-]{0,199}$/.test(value);
}

function isDigest(value: unknown): value is string {
  return typeof value === 'string' && /^sha256:[0-9a-f]{64}$/.test(value);
}
