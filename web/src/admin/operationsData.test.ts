import { describe, expect, it } from 'vitest';
import type { ProjectSOPView } from '../types';
import { normalizeAdminWorkOSView, normalizeOperationsExecutorDirectory, normalizeOperationsSkillDirectory, normalizeProjectSOPView } from './operationsData';

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
    expect(normalized.online_window_seconds).toBe(120);
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
});
