import { BookOpen, ChevronDown, CircleHelp, ClipboardList, FileInput, LayoutDashboard, LogOut, Menu, Plus, Settings, Shield, Users, Workflow, Wrench, X } from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import type { Project, Session, Tenant } from '../types';
import { roleLabel } from '../uiLabels';
import { BrandLockup, BrandMark } from './Brand';
import { IconButton, Status } from './ui';

const workspaceItems = [
  {label:'任务中心',icon:ClipboardList,path:'/workspace/tasks'},
  {label:'输入收集',icon:FileInput,path:'/workspace/inbox'}
];

const projectItems: {label:string;icon:typeof LayoutDashboard;path:(id:string)=>string}[] = [
  {label:'接入与初始化',icon:Wrench,path:id=>`/projects/${encodeURIComponent(id)}/setup`},
  {label:'项目总览',icon:LayoutDashboard,path:id=>`/projects/${encodeURIComponent(id)}/overview`},
  {label:'项目任务',icon:ClipboardList,path:id=>`/projects/${encodeURIComponent(id)}/tasks`},
  {label:'知识库',icon:BookOpen,path:id=>`/projects/${encodeURIComponent(id)}/knowledge`},
  {label:'流程规范',icon:Workflow,path:id=>`/projects/${encodeURIComponent(id)}/sop`}
];

export function Layout({session,tenants,projects,project,onTenant,onProject,onCreateProject,onAdmin,onLogout,children}: {session:Session;tenants:Tenant[];projects:Project[];project?:Project;onTenant:(tenantID:string)=>void;onProject:(project:Project)=>void;onCreateProject:()=>void;onAdmin:()=>void;onLogout:()=>void;children:ReactNode}) {
  const [mobileOpen,setMobileOpen]=useState(false);
  const location=useLocation();
  const navigate=useNavigate();
  const canManage=session.role==='tenant_admin'||session.role==='project_manager';
  const needsConnection=Boolean(project&&project.status!=='archived'&&project.connected_devices===0);
  const go=(path:string)=>{const target=path==='/workspace/tasks/new'&&project?`/projects/${encodeURIComponent(project.id)}/tasks/new`:path;navigate(target);setMobileOpen(false)};
  const active=(path:string)=>location.pathname===path||location.pathname.startsWith(`${path}/`);
  return <div className="app-shell">
    <header className="mobile-header"><BrandMark/><strong>Content Work OS</strong><IconButton label="打开导航" onClick={()=>setMobileOpen(true)}><Menu size={20}/></IconButton></header>
    <aside className={`sidebar ${mobileOpen?'sidebar-open':''}`}>
      <div className="brand"><BrandLockup subtitle="内容工作台"/><IconButton label="关闭导航" className="mobile-close" onClick={()=>setMobileOpen(false)}><X size={18}/></IconButton></div>
      <div className="tenant-switcher"><div className="tenant-avatar">{session.tenant.name.slice(0,1)}</div><label><span className="visually-hidden">切换租户</span><select value={session.tenant.id} onChange={event=>onTenant(event.target.value)}>{tenants.map(tenant=><option key={tenant.id} value={tenant.id}>{tenant.name}</option>)}</select><small>{roleLabel(session.role)}</small></label><ChevronDown size={16}/></div>
      <nav>
        {!needsConnection&&<><NavItem id="today" label="今天" icon={LayoutDashboard} active={location.pathname==='/workspace'} onClick={()=>go('/workspace')}/><div className="nav-label"><span>工作区</span></div>{workspaceItems.map(item=><NavItem key={item.path} id={item.path} label={item.label} icon={item.icon} active={active(item.path)} onClick={()=>go(item.path)}/>)}</>}
        <div className="nav-label"><span>项目</span>{project&&<Status value={project.status}/>}</div>
        {project ? (needsConnection?projectItems.slice(0,1):projectItems).map(item=><NavItem key={item.path(project.id)} id={item.path(project.id)} label={item.label} icon={item.icon} active={active(item.path(project.id))} onClick={()=>go(item.path(project.id))}/>) : canManage ? <button className="nav-create" onClick={onCreateProject}><Plus size={15}/>创建项目</button> : null}
        <div className="nav-label"><span>组织</span></div>
        <NavItem id="team" label="团队与权限" icon={Users} active={active('/team')} onClick={()=>go('/team')}/>
        <a className="nav-item nav-link" data-view="docs" href="/docs" onClick={()=>setMobileOpen(false)}><CircleHelp size={18}/><span>使用文档</span></a>
        {canManage&&<button className="nav-item" onClick={onAdmin}><Shield size={18}/><span>运行配置</span></button>}
      </nav>
      <div className="sidebar-footer"><Settings size={17}/><div><strong>{session.user.display_name}</strong><span>{session.user.email}</span></div><IconButton label="退出登录" onClick={onLogout}><LogOut size={16}/></IconButton></div>
    </aside>
    {mobileOpen&&<button className="sidebar-scrim" aria-label="关闭导航" onClick={()=>setMobileOpen(false)}/>}
    <main className="main"><header className="topbar"><div className="project-select-wrap"><span>当前项目</span><select value={project?.id||''} onChange={(event)=>{const next=projects.find(p=>p.id===event.target.value);if(next)onProject(next)}}><option value="">{projects.length ? '选择项目' : '还没有项目'}</option>{projects.map(p=><option key={p.id} value={p.id}>{p.brand_name} · {p.product_name}</option>)}</select></div></header>{children}</main>
  </div>
}

function NavItem({id,label,icon:Icon,active,onClick}:{id:string;label:string;icon:typeof LayoutDashboard;active:boolean;onClick:()=>void}) { return <button data-view={id} className={`nav-item ${active?'active':''}`} onClick={onClick}><Icon size={18}/><span>{label}</span></button> }
