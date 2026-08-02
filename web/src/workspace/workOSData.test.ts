import { describe, expect, it } from 'vitest';
import type { ProjectSOPView } from '../types';
import { normalizeAdminWorkOSView, normalizeProjectSOPView } from './workOSData';

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
    const value = {environments: [{capabilities: null}], sops: [], gates: null, capabilities: null, audit: null, usage: null} as any;
    const normalized = normalizeAdminWorkOSView(value);

    expect(normalized.environments[0].capabilities).toEqual([]);
    expect(normalized.gates).toEqual([]);
    expect(normalized.audit).toEqual([]);
    expect(normalized.usage.by_execution_mode).toEqual({});
  });
});
