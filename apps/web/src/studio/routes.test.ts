import { matchRoutes } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { appRoutes } from '../router';

describe('customer studio routes',()=>{
  it.each([
    ['/studio',undefined],
    ['/studio/connect','connect'],
    ['/studio/tasks','tasks'],
    ['/studio/tasks/new','tasks/new'],
    ['/studio/tasks/task-1','tasks/:taskID'],
    ['/studio/assets','assets'],
    ['/studio/assets/materials/material-1','assets/materials/:materialID'],
    ['/studio/assets/results/task-1/result-1','assets/results/:taskID/:resultID'],
    ['/studio/knowledge','knowledge'],
    ['/studio/team','team'],
    ['/studio/deliveries','deliveries'],
  ])('keeps %s under the independent customer shell',(pathname,leaf)=>{
    expect(matchRoutes(appRoutes,pathname)?.map(match=>match.route.path)).toEqual(['/studio',undefined,leaf]);
  });

  it('does not register retired project URLs',()=>{
    expect(matchRoutes(appRoutes,'/projects/project-1/overview')?.map(match=>match.route.path)).toEqual(['*']);
  });
});
