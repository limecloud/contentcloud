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
    expect(markup).toContain('任务运行');
    expect(markup).toContain('客户创作台');
    expect(markup).toContain('运营工作台');
    expect(markup).toContain('执行客户端');
    expect(markup).toContain('项目任务');
    expect(markup).not.toContain('所有任务');
    expect(markup).not.toContain('我的任务');
    expect(markup).not.toContain('新建任务');
  });

  it('keeps operations navigation available before a Project has an execution client',()=>{
    const markup=renderToStaticMarkup(<StaticRouter location="/projects/project-1/setup"><Layout session={session} tenants={tenants} projects={[unconnectedProject]} project={unconnectedProject} onTenant={()=>{}} onProject={()=>{}} onCreateProject={()=>{}} onAdmin={()=>{}} onLogout={()=>{}}>{null}</Layout></StaticRouter>);
    expect(markup).toContain('执行客户端');
    expect(markup).toContain('客户创作台');
    expect(markup).toContain('运营概览');
    expect(markup).toContain('任务运行');
    expect(markup).toContain('项目任务');
    expect(markup).toContain('知识库');
  });
});
