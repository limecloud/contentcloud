import {renderToStaticMarkup} from 'react-dom/server';
import {StaticRouter} from 'react-router-dom';
import {describe,expect,it} from 'vitest';
import type {Project, Session, Tenant} from '../types';
import {Layout} from './Layout';

const session={
  role:'tenant_admin',
  tenant:{id:'tenant-1',name:'演示租户'},
  user:{display_name:'管理员',email:'admin@example.com'}
} as unknown as Session;
const tenants=[session.tenant] as unknown as Tenant[];
const project={id:'project-1',brand_name:'Demo',product_name:'内容项目',status:'draft',connected_devices:1} as unknown as Project;
const unconnectedProject={...project,connected_devices:0};

describe('Layout task navigation',()=>{
  it('keeps task scope in the page instead of duplicating sidebar entries',()=>{
    const markup=renderToStaticMarkup(<StaticRouter location="/workspace/tasks"><Layout session={session} tenants={tenants} projects={[project]} project={project} onTenant={()=>{}} onProject={()=>{}} onCreateProject={()=>{}} onAdmin={()=>{}} onLogout={()=>{}}>{null}</Layout></StaticRouter>);
    expect(markup).toContain('任务中心');
    expect(markup).toContain('项目任务');
    expect(markup).not.toContain('所有任务');
    expect(markup).not.toContain('我的任务');
    expect(markup).not.toContain('新建任务');
  });

  it('shows only connection setup before a Project has a Workspace binding',()=>{
    const markup=renderToStaticMarkup(<StaticRouter location="/projects/project-1/setup"><Layout session={session} tenants={tenants} projects={[unconnectedProject]} project={unconnectedProject} onTenant={()=>{}} onProject={()=>{}} onCreateProject={()=>{}} onAdmin={()=>{}} onLogout={()=>{}}>{null}</Layout></StaticRouter>);
    expect(markup).toContain('接入与初始化');
    expect(markup).not.toContain('今天');
    expect(markup).not.toContain('任务中心');
    expect(markup).not.toContain('项目任务');
    expect(markup).not.toContain('知识库');
  });
});
