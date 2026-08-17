import { describe, expect, it } from 'vitest';
import { matchRoutes } from 'react-router-dom';
import { appRoutes } from '../router';
import { consolePath } from '../consoleRoutes';
import { adminCapabilityPath, adminCustomerPath, adminCustomersForProductPath, adminExecutorPath, adminPath, adminProductPath, adminProductVersionPath, adminReleaseResultPath, adminSkillPath } from './routes';

describe('admin routes',()=>{
  it('maps every admin section to a stable deep link',()=>{
    expect(adminPath('dashboard')).toBe('/admin/dashboard');
    expect(adminPath('products')).toBe('/admin/products');
    expect(adminPath('capabilities')).toBe('/admin/capabilities');
    expect(adminPath('jobs')).toBe('/admin/jobs');
    expect(adminPath('providers')).toBe('/admin/providers');
    expect(adminPath('tenants')).toBe('/admin/tenants');
    expect(adminPath('audit')).toBe('/admin/audit');
    expect(adminPath('costs')).toBe('/admin/costs');
    expect(adminProductPath('ip/video')).toBe('/admin/products/ip%2Fvideo');
    expect(adminProductVersionPath('ip/video',2)).toBe('/admin/products/ip%2Fvideo/versions/2');
    expect(adminCustomerPath('customer/environment')).toBe('/admin/customers/customer%2Fenvironment');
    expect(adminCustomersForProductPath('ip/video')).toBe('/admin/customers?product=ip%2Fvideo');
    expect(adminReleaseResultPath('ip/video',2)).toBe('/admin/releases/ip%2Fvideo/versions/2');
    expect(adminCapabilityPath('content/search','1.0.0')).toBe('/admin/capabilities/content%2Fsearch/versions/1.0.0');
    expect(adminExecutorPath('device/local')).toBe('/admin/executors/device%2Flocal');
    expect(adminSkillPath('content/skill','1.2.0')).toBe('/admin/skills/content%2Fskill/versions/1.2.0');
  });

  it('mounts admin pages below the independent admin parent route',()=>{
    const tenants=matchRoutes(appRoutes,'/admin/tenants');
    expect(tenants?.map(item=>item.route.path)).toEqual(['/admin',undefined,'tenants']);
    const unknown=matchRoutes(appRoutes,'/admin/unknown');
    expect(unknown?.map(item=>item.route.path)).toEqual(['/admin',undefined,'*']);
    expect(matchRoutes(appRoutes,'/administer')?.map(item=>item.route.path)).toEqual(['*']);
  });

  it('mounts the new operations workspaces as independent pages',()=>{
    const paths=['products','releases','customers','capabilities','skills','executors','providers','jobs','alerts','tenants','audit','costs'];
    for(const path of paths){
      const matches=matchRoutes(appRoutes,`/admin/${path}`);
      expect(matches?.map(item=>item.route.path)).toEqual(['/admin',undefined,path]);
      expect(matches?.at(-1)?.route.lazy).toBeTypeOf('function');
      expect(matches?.at(-1)?.route.element).toBeUndefined();
    }
  });

  it('mounts product and version details as stable deep links',()=>{
    const product=matchRoutes(appRoutes,'/admin/products/ip-video');
    expect(product?.map(item=>item.route.path)).toEqual(['/admin',undefined,'products/:productID']);
    const version=matchRoutes(appRoutes,'/admin/products/ip-video/versions/2');
    expect(version?.map(item=>item.route.path)).toEqual(['/admin',undefined,'products/:productID/versions/:version']);
  });

  it('mounts customer enrollment details as stable deep links',()=>{
    const customer=matchRoutes(appRoutes,'/admin/customers/environment-1');
    expect(customer?.map(item=>item.route.path)).toEqual(['/admin',undefined,'customers/:environmentID']);
    expect(customer?.at(-1)?.route.lazy).toBeTypeOf('function');
  });

  it('mounts release results as stable deep links',()=>{
    const release=matchRoutes(appRoutes,'/admin/releases/ip-video/versions/2');
    expect(release?.map(item=>item.route.path)).toEqual(['/admin',undefined,'releases/:productID/versions/:version']);
    expect(release?.at(-1)?.route.lazy).toBeTypeOf('function');
  });

  it('mounts capability versions as stable deep links',()=>{
    const capability=matchRoutes(appRoutes,'/admin/capabilities/inspiration_collection/versions/1.0.0');
    expect(capability?.map(item=>item.route.path)).toEqual(['/admin',undefined,'capabilities/:capabilityID/versions/:capabilityVersion']);
    expect(capability?.at(-1)?.route.lazy).toBeTypeOf('function');
  });

  it('mounts executor details as stable deep links',()=>{
    const executor=matchRoutes(appRoutes,'/admin/executors/executor-1');
    expect(executor?.map(item=>item.route.path)).toEqual(['/admin',undefined,'executors/:executorID']);
    expect(executor?.at(-1)?.route.lazy).toBeTypeOf('function');
  });

  it('mounts skill versions as stable deep links',()=>{
    const skill=matchRoutes(appRoutes,'/admin/skills/contentcloud-script-writing/versions/1.2.0');
    expect(skill?.map(item=>item.route.path)).toEqual(['/admin',undefined,'skills/:skillID/versions/:skillVersion']);
    expect(skill?.at(-1)?.route.lazy).toBeTypeOf('function');
  });

  it('does not expose retired or contract-free admin routes',()=>{
    for(const path of ['environments','sops','gates','runtime','usage','connectors','policies','effects','results','rights','duplicates','projections','users','support']){
      const matches=matchRoutes(appRoutes,`/admin/${path}`);
      expect(matches?.map(item=>item.route.path)).toEqual(['/admin',undefined,'*']);
      expect(matches?.at(-1)?.route.lazy).toBeUndefined();
    }
  });

  it('keeps the tenant console and public surfaces outside the admin route tree',()=>{
    expect(matchRoutes(appRoutes,'/')?.map(item=>item.route.path)).toEqual(['/']);
    expect(matchRoutes(appRoutes,'/studio')?.map(item=>item.route.path)).toEqual(['/studio',undefined,undefined]);
    expect(matchRoutes(appRoutes,'/studio/knowledge')?.map(item=>item.route.path)).toEqual(['/studio',undefined,'knowledge']);
    expect(matchRoutes(appRoutes,'/workspace')?.map(item=>item.route.path)).toEqual(['*']);
    expect(matchRoutes(appRoutes,'/projects/project-1/creative')?.map(item=>item.route.path)).toEqual(['*']);
    expect(matchRoutes(appRoutes,'/login')?.map(item=>item.route.path)).toEqual(['/login']);
    expect(matchRoutes(appRoutes,'/review/token-1')?.map(item=>item.route.path)).toEqual(['/review/:token']);
    expect(matchRoutes(appRoutes,'/docs/clients/codex')?.map(item=>item.route.path)).toEqual(['/docs','clients/:clientID']);
  });

  it('keeps retired workspace paths outside the application route tree',()=>{
    expect(consolePath.dashboard).toBe('/studio');
    expect(consolePath.team).toBe('/studio/team');
    expect(matchRoutes(appRoutes,'/workspace/projects/project-1/scripts')?.map(item=>item.route.path)).toEqual(['*']);
  });
});
