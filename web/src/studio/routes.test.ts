import { matchRoutes } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { appRoutes } from '../router';

describe('customer studio routes',()=>{
  it.each([
    ['/studio',undefined],
    ['/studio/tasks','tasks'],
    ['/studio/tasks/new','tasks/new'],
    ['/studio/tasks/task-1','tasks/:taskID'],
    ['/studio/assets','assets'],
    ['/studio/deliveries','deliveries'],
  ])('keeps %s under the independent customer shell',(pathname,leaf)=>{
    expect(matchRoutes(appRoutes,pathname)?.map(match=>match.route.path)).toEqual(['/studio',undefined,leaf]);
  });
});
