import { describe, expect, it } from 'vitest';
import { matchRoutes } from 'react-router-dom';
import { appRoutes } from '../router';
import { consolePath } from '../consoleRoutes';
import { adminPath } from './routes';

describe('admin routes',()=>{
  it('maps every admin section to a stable deep link',()=>{
    expect(adminPath('dashboard')).toBe('/admin/dashboard');
    expect(adminPath('tenants')).toBe('/admin/tenants');
    expect(adminPath('users')).toBe('/admin/users');
  });

  it('mounts admin pages below the independent admin parent route',()=>{
    const tenants=matchRoutes(appRoutes,'/admin/tenants');
    expect(tenants?.map(item=>item.route.path)).toEqual(['/admin',undefined,'tenants']);
    const unknown=matchRoutes(appRoutes,'/admin/unknown');
    expect(unknown?.map(item=>item.route.path)).toEqual(['/admin',undefined,'*']);
    expect(matchRoutes(appRoutes,'/administer')?.map(item=>item.route.path)).toEqual(['/',undefined,'*']);
  });

  it('keeps the tenant console and public surfaces outside the admin route tree',()=>{
    expect(matchRoutes(appRoutes,'/projects/project-1/creative')?.map(item=>item.route.path)).toEqual(['/',undefined,'projects/:projectID','creative']);
    expect(matchRoutes(appRoutes,'/login')?.map(item=>item.route.path)).toEqual(['/login']);
    expect(matchRoutes(appRoutes,'/review/token-1')?.map(item=>item.route.path)).toEqual(['/review/:token']);
  });

  it('uses V3 root-level console URLs without legacy route aliases',()=>{
    expect(consolePath.dashboard).toBe('/');
    expect(consolePath.team).toBe('/team');
    expect(consolePath.project('project-1')).toBe('/projects/project-1/setup');
    expect(consolePath.project('project-1','creative')).toBe('/projects/project-1/creative');
    expect(matchRoutes(appRoutes,'/workspace/projects/project-1/scripts')?.map(item=>item.route.path)).toEqual(['/',undefined,'*']);
  });
});
