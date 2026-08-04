import { describe, expect, it } from 'vitest';
import { knowledgeObjectAllows, taskPath, workOSStatusTone } from './workOS';

describe('work OS task navigation', () => {
  it('keeps task links scoped to their Project', () => {
    expect(taskPath({project_id: 'project/demo', id: 'task/one'})).toBe('/projects/project%2Fdemo/tasks/task%2Fone');
  });
});

describe('knowledge object governance actions', () => {
  it('renders decisions only when the server explicitly allows them', () => {
    expect(knowledgeObjectAllows({allowed_actions: ['approve', 'reject']}, 'approve')).toBe(true);
    expect(knowledgeObjectAllows({allowed_actions: []}, 'approve')).toBe(false);
    expect(knowledgeObjectAllows({}, 'approve')).toBe(false);
  });

  it('shows every governed knowledge status as successful', () => {
    for (const status of ['verified', 'approved', 'valid', 'active']) {
      expect(workOSStatusTone(status)).toBe('is-success');
    }
  });
});
