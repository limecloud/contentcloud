import { describe, expect, it } from 'vitest';
import { taskPath } from './workOS';

describe('work OS task navigation', () => {
  it('keeps task links scoped to their Project', () => {
    expect(taskPath({project_id: 'project/demo', id: 'task/one'})).toBe('/projects/project%2Fdemo/tasks/task%2Fone');
  });
});
