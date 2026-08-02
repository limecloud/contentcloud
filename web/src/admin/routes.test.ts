import { describe, expect, it } from 'vitest';
import { isValidElement } from 'react';
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
    expect(matchRoutes(appRoutes,'/administer')?.map(item=>item.route.path)).toEqual(['*']);
  });

  it('redirects retired platform directory links to the configuration control plane',()=>{
    const tenants=matchRoutes(appRoutes,'/admin/tenants');
    const users=matchRoutes(appRoutes,'/admin/users');
    const tenantRedirect=tenants?.at(-1)?.route.element;
    const userRedirect=users?.at(-1)?.route.element;
    expect(isValidElement(tenantRedirect)?tenantRedirect.props.to:undefined).toBe('/admin/dashboard');
    expect(isValidElement(userRedirect)?userRedirect.props.to:undefined).toBe('/admin/dashboard');
  });

  it('keeps the tenant console and public surfaces outside the admin route tree',()=>{
    expect(matchRoutes(appRoutes,'/')?.map(item=>item.route.path)).toEqual(['/']);
    expect(matchRoutes(appRoutes,'/workspace')?.map(item=>item.route.path)).toEqual(['/workspace',undefined,undefined]);
    expect(matchRoutes(appRoutes,'/projects/project-1/creative')?.map(item=>item.route.path)).toEqual(['/projects/:projectID',undefined,'creative']);
    expect(matchRoutes(appRoutes,'/login')?.map(item=>item.route.path)).toEqual(['/login']);
    expect(matchRoutes(appRoutes,'/review/token-1')?.map(item=>item.route.path)).toEqual(['/review/:token']);
    expect(matchRoutes(appRoutes,'/docs/clients/codex')?.map(item=>item.route.path)).toEqual(['/docs','clients/:clientID']);
  });

  it('uses a dedicated workspace URL while preserving existing project deep links',()=>{
    expect(consolePath.dashboard).toBe('/workspace');
    expect(consolePath.team).toBe('/team');
    expect(consolePath.project('project-1')).toBe('/projects/project-1/setup');
    expect(consolePath.project('project-1','creative')).toBe('/projects/project-1/creative');
    expect(matchRoutes(appRoutes,'/workspace/projects/project-1/scripts')?.map(item=>item.route.path)).toEqual(['/workspace',undefined,'*']);
  });
});
