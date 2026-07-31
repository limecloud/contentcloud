import { api } from './api';

export interface CodexHandoff {
  schema_version: string;
  kind: 'project' | 'review_feedback';
  project_id: string;
  target: { kind: 'project' | 'submission_revision'; id: string; digest?: string };
  plugin_id: string;
  plugin_version: string;
  requires_new_chat: true;
  requires_workspace_selection: true;
  launch_url: string;
  prompt: string;
  steps: string[];
  fallback_url: '/codex';
}

export interface CodexHandoffExpectation {
  projectID: string;
  targetKind: CodexHandoff['target']['kind'];
  targetID: string;
  digest?: string;
}

const expectedKeys = new Set([
  'schema_version', 'kind', 'project_id', 'target', 'plugin_id', 'plugin_version',
  'requires_new_chat', 'requires_workspace_selection', 'launch_url', 'prompt', 'steps', 'fallback_url',
]);

export async function loadProjectCodexHandoff(projectID: string): Promise<CodexHandoff> {
  const value = await api<unknown>(`/api/bff/projects/${encodeURIComponent(projectID)}/codex-handoff`);
  return validateCodexHandoff(value, { projectID, targetKind: 'project', targetID: projectID });
}

export async function loadReviewFeedbackCodexHandoff(projectID: string, revisionID: string, digest: string): Promise<CodexHandoff> {
  const value = await api<unknown>(`/api/bff/projects/${encodeURIComponent(projectID)}/submission-revisions/${encodeURIComponent(revisionID)}/codex-handoff`);
  return validateCodexHandoff(value, { projectID, targetKind: 'submission_revision', targetID: revisionID, digest });
}

export function validateCodexHandoff(value: unknown, expectation: CodexHandoffExpectation): CodexHandoff {
  if (!isRecord(value) || Object.keys(value).some(key => !expectedKeys.has(key))) {
    throw new Error('Codex 恢复响应结构不受支持');
  }
  const target = value.target;
  if (!isRecord(target) || !isSafeID(value.project_id) || !isSafeID(target.id) || typeof target.kind !== 'string') {
    throw new Error('Codex 恢复目标无效');
  }
  if (value.schema_version !== 'contentcloud.codex-handoff/1.0' || value.project_id !== expectation.projectID || value.kind !== (expectation.targetKind === 'project' ? 'project' : 'review_feedback') || target.kind !== expectation.targetKind || target.id !== expectation.targetID) {
    throw new Error('Codex 恢复目标与当前页面不一致');
  }
  if (expectation.digest && target.digest !== normalizeDigest(expectation.digest)) {
    throw new Error('Codex 恢复摘要与当前页面不一致');
  }
  if (target.digest !== undefined && !isDigest(target.digest)) {
    throw new Error('Codex 恢复摘要无效');
  }
  if (value.plugin_id !== 'contentcloud-video-production@contentcloud' || value.plugin_version !== '0.13.0' || value.requires_new_chat !== true || value.requires_workspace_selection !== true || value.fallback_url !== '/codex' || typeof value.prompt !== 'string' || !Array.isArray(value.steps) || value.steps.some(step => typeof step !== 'string' || step.length === 0)) {
    throw new Error('Codex 恢复门禁或 Plugin 版本无效');
  }
  const launch = parseCodexLaunchURL(value.launch_url, value.prompt);
  if (!launch) {
    throw new Error('Codex 恢复链接不是受支持的 new-chat deep link');
  }
  return value as unknown as CodexHandoff;
}

export function normalizeDigest(value: string): string {
  const normalized = value.trim().toLowerCase();
  return normalized.startsWith('sha256:') ? normalized : `sha256:${normalized}`;
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

function isRecord(value: unknown): value is Record<string, any> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isSafeID(value: unknown): value is string {
  return typeof value === 'string' && /^[A-Za-z0-9][A-Za-z0-9._:-]{0,199}$/.test(value);
}

function isDigest(value: unknown): value is string {
  return typeof value === 'string' && /^sha256:[0-9a-f]{64}$/.test(value);
}
