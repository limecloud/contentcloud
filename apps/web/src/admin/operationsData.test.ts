import { describe, expect, it } from 'vitest';
import type { ProjectSOPView } from '../types';
import { normalizeAdminWorkOSView, normalizeOperationsExecutorDirectory, normalizeOperationsSkillDirectory, normalizeProjectSOPView, normalizeWorkbenchRegistry } from './operationsData';

describe('work OS API collection normalization', () => {
  it('turns nullable SOP collections into iterable arrays', () => {
    const value = {
      binding: {},
      sop: {
        content_types: null,
        stages: [{owner_roles: null, input_refs: null, required_capabilities: null, execution_modes: null, checks: null, gate_ids: null}],
        gates: null
      }
    } as unknown as ProjectSOPView;

    const normalized = normalizeProjectSOPView(value);

    expect(normalized.sop.content_types).toEqual([]);
    expect(normalized.sop.gates).toEqual([]);
    expect(normalized.sop.stages[0].owner_roles).toEqual([]);
    expect(normalized.sop.stages[0].gate_ids).toEqual([]);
  });

  it('normalizes nullable collections in the admin view', () => {
    const value = {environments: [{capabilities: null}], sops: [], gates: null, capabilities: [{presentation_profiles: null}], audit: null, usage: null} as any;
    const normalized = normalizeAdminWorkOSView(value);

    expect(normalized.environments[0].capabilities).toEqual([]);
    expect(normalized.gates).toEqual([]);
    expect(normalized.capabilities[0].presentation_profiles).toEqual([]);
    expect(normalized.audit).toEqual([]);
    expect(normalized.usage.by_execution_mode).toEqual({});
  });

  it('normalizes nullable executor facts from the operations BFF', () => {
    const value = {executors: [{capabilities: null, projects: null}], online_window_seconds: 0} as any;
    const normalized = normalizeOperationsExecutorDirectory(value);

    expect(normalized.executors[0].capabilities).toEqual([]);
    expect(normalized.executors[0].projects).toEqual([]);
    expect(normalized.executors[0].active_attempt_ids).toEqual([]);
    expect(normalized.executors[0].presence_status).toBe('unknown');
    expect(normalized.executors[0].environment_status).toBe('unknown');
    expect(normalized.executors[0].runtime_status).toBe('unknown');
    expect(normalized.online_window_seconds).toBe(45);
  });

  it('keeps legacy executor presence compatible without inventing readiness', () => {
    const value = {executors: [{status: 'online', capabilities: [], projects: []}], online_window_seconds: 120} as any;
    const normalized = normalizeOperationsExecutorDirectory(value);

    expect(normalized.executors[0].presence_status).toBe('online');
    expect(normalized.executors[0].environment_status).toBe('unknown');
    expect(normalized.executors[0].runtime_status).toBe('unknown');
  });

  it('normalizes nullable skill facts from the operations BFF', () => {
    const value = {configured: true, skills: [{compatible_profiles: null, permissions: null, data_flow: {cloud_actions: null}, output_schemas: null, evaluation: {evidence: null}}]} as any;
    const normalized = normalizeOperationsSkillDirectory(value);

    expect(normalized.skills[0].compatible_profiles).toEqual([]);
    expect(normalized.skills[0].permissions).toEqual([]);
    expect(normalized.skills[0].data_flow.cloud_actions).toEqual([]);
    expect(normalized.skills[0].output_schemas).toEqual([]);
    expect(normalized.skills[0].evaluation.evidence).toEqual([]);
  });

  it('normalizes missing workbench registry collections from older BFF responses', () => {
    const normalized = normalizeWorkbenchRegistry({generated_at: '2026-08-08T02:30:00Z'} as any);

    expect(normalized.entries).toEqual([]);
  });

  it('normalizes empty workbench entry scopes before the admin page renders', () => {
    const normalized = normalizeWorkbenchRegistry({entries: [{manifest: {content_types: ['article'], ui: {navigation: [], stages: []}}}]} as any);

    expect(normalized.entries[0].tenant_ids).toEqual([]);
    expect(normalized.entries[0].template_aliases).toEqual([]);
    expect(normalized.entries[0].manifest.content_types).toEqual(['article']);
    expect(normalized.entries[0].manifest.ui.navigation).toEqual([]);
    expect(normalized.entries[0].manifest.ui.stages).toEqual([]);
  });
});
