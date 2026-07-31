import { describe, expect, it } from 'vitest';
import { normalizeDigest, validateCodexHandoff, type CodexHandoff } from './codexHandoff';

const digest = `sha256:${'a'.repeat(64)}`;

function handoff(overrides: Partial<CodexHandoff> = {}): CodexHandoff {
  const prompt = '[plugin://contentcloud-video-production@contentcloud] project project-1; workspace_context';
  return {
    schema_version: 'contentcloud.codex-handoff/1.0', kind: 'project', project_id: 'project-1',
    target: { kind: 'project', id: 'project-1' }, plugin_id: 'contentcloud-video-production@contentcloud', plugin_version: '0.13.0',
    requires_new_chat: true, requires_workspace_selection: true,
    launch_url: `codex://new?prompt=${encodeURIComponent(prompt)}`, prompt, steps: ['select workspace'], fallback_url: '/codex', ...overrides,
  };
}

describe('Codex handoff validation', () => {
  it('accepts the canonical project deep link', () => {
    expect(validateCodexHandoff(handoff(), { projectID: 'project-1', targetKind: 'project', targetID: 'project-1' }).launch_url).toContain('codex://new?prompt=');
  });

  it('rejects path, originUrl, extra query, and non-new links', () => {
    for (const launch_url of [
      'codex://new?path=%2FUsers%2Fprivate&prompt=x',
      'codex://new?originUrl=https%3A%2F%2Fprivate&prompt=x',
      'codex://new?prompt=x&extra=y',
      'https://contentcloud.test/codex',
    ]) {
      const value = handoff({ prompt: 'x', launch_url });
      expect(() => validateCodexHandoff(value, { projectID: 'project-1', targetKind: 'project', targetID: 'project-1' })).toThrow();
    }
  });

  it('rejects target drift, missing gates, and digest drift', () => {
    expect(() => validateCodexHandoff(handoff({ project_id: 'other' }), { projectID: 'project-1', targetKind: 'project', targetID: 'project-1' })).toThrow();
    expect(() => validateCodexHandoff(handoff({ requires_new_chat: false as unknown as true }), { projectID: 'project-1', targetKind: 'project', targetID: 'project-1' })).toThrow();
    const review = handoff({ kind: 'review_feedback', target: { kind: 'submission_revision', id: 'revision-1', digest } });
    expect(() => validateCodexHandoff(review, { projectID: 'project-1', targetKind: 'submission_revision', targetID: 'revision-1', digest: `sha256:${'b'.repeat(64)}` })).toThrow();
  });

  it('normalizes bare digests before comparing them', () => {
    expect(normalizeDigest('A'.repeat(64))).toBe(digest);
  });
});
