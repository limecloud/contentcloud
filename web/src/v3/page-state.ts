import type { SubmissionRevision } from '../types';

export interface ProjectPageIssue {
  kind: 'auth' | 'access' | 'unavailable';
  code: 'PROJECT_AUTH_REQUIRED' | 'PROJECT_TARGET_UNAVAILABLE' | 'PROJECT_PAGE_UNAVAILABLE';
  title: string;
  detail: string;
}

export interface DisclosureSummary {
  total: number;
  metadataOnly: number;
  evidencePack: number;
  fullSource: number;
  unknown: number;
  limited: boolean;
}

const inaccessibleIssue: ProjectPageIssue = {
  kind: 'access',
  code: 'PROJECT_TARGET_UNAVAILABLE',
  title: '无法访问此项目内容',
  detail: '目标不存在，或当前账号、租户没有访问权限。请返回项目列表重新选择。',
};

const authIssue: ProjectPageIssue = {
  kind: 'auth',
  code: 'PROJECT_AUTH_REQUIRED',
  title: '登录状态已失效',
  detail: '重新登录后将返回当前项目位置，并再次验证租户、项目和目标对象权限。',
};

const unavailableIssue: ProjectPageIssue = {
  kind: 'unavailable',
  code: 'PROJECT_PAGE_UNAVAILABLE',
  title: '页面暂时无法加载',
  detail: '服务端没有返回可验证的项目状态。当前页面不会执行任何写操作，请稍后重试。',
};

export function inaccessibleProjectIssue(): ProjectPageIssue {
  return inaccessibleIssue;
}

export function projectPageIssueFromError(value: unknown): ProjectPageIssue {
  if (isRecord(value) && value.status === 401) return authIssue;
  if (isRecord(value) && (value.status === 403 || value.status === 404)) return inaccessibleIssue;
  return unavailableIssue;
}

export function summarizeDisclosure(revision: Pick<SubmissionRevision, 'evidence_limited' | 'source_disclosures'>): DisclosureSummary {
  const summary: DisclosureSummary = {
    total: revision.source_disclosures.length,
    metadataOnly: 0,
    evidencePack: 0,
    fullSource: 0,
    unknown: 0,
    limited: revision.evidence_limited,
  };
  for (const disclosure of revision.source_disclosures) {
    if (disclosure.level === 'metadata_only') summary.metadataOnly += 1;
    else if (disclosure.level === 'evidence_pack') summary.evidencePack += 1;
    else if (disclosure.level === 'full_source') summary.fullSource += 1;
    else summary.unknown += 1;
  }
  if (summary.unknown > 0) summary.limited = true;
  return summary;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}
