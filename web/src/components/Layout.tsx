import { Activity, BookOpenCheck, ChevronDown, ClipboardList, FileArchive, FileText, GitBranch, LayoutDashboard, LogOut, Menu, ScrollText, Settings, ShieldCheck, Sparkles, Users, X } from 'lucide-react';
import { useState, type ReactNode } from 'react';
import type { Project, Session, Tenant } from '../types';
import { IconButton, Status } from './ui';

export type View = 'dashboard'|'team'|'overview'|'sources'|'assets'|'knowledge'|'strategy'|'briefs'|'scripts'|'results'|'lineage'|'audit';
const globalItems: {id:View;label:string;icon:typeof LayoutDashboard;roles?:string[]}[] = [{id:'dashboard',label:'工作台',icon:LayoutDashboard},{id:'team',label:'团队',icon:Users}];
const projectItems: {id:View;label:string;icon:typeof LayoutDashboard}[] = [
  {id:'overview',label:'项目总览',icon:LayoutDashboard},
  {id:'sources',label:'资料',icon:FileArchive},
  {id:'assets',label:'素材权利',icon:ShieldCheck},
  {id:'knowledge',label:'可信知识',icon:BookOpenCheck},
  {id:'strategy',label:'内容策略',icon:Sparkles},
  {id:'briefs',label:'Brief',icon:ClipboardList},
  {id:'scripts',label:'剧本',icon:FileText},
  {id:'results',label:'结果',icon:Activity},
  {id:'lineage',label:'追踪与影响',icon:GitBranch},
  {id:'audit',label:'审计',icon:ScrollText}
];

export function Layout({session,tenants,projects,project,view,onView,onTenant,onProject,onCreateProject,onLogout,children}: {session:Session;tenants:Tenant[];projects:Project[];project?:Project;view:View;onView:(view:View)=>void;onTenant:(tenantID:string)=>void;onProject:(project:Project)=>void;onCreateProject:()=>void;onLogout:()=>void;children:ReactNode}) {
  const [mobileOpen,setMobileOpen]=useState(false);
  const canManage=session.role==='tenant_admin'||session.role==='project_manager';
  const navigate=(id:View)=>{onView(id);setMobileOpen(false)};
  return <div className="app-shell">
    <header className="mobile-header"><div className="brand-mark">CC</div><strong>ContentCloud</strong><IconButton label="打开导航" onClick={()=>setMobileOpen(true)}><Menu size={20}/></IconButton></header>
    <aside className={`sidebar ${mobileOpen?'sidebar-open':''}`}>
      <div className="brand"><div className="brand-mark">CC</div><div><strong>ContentCloud</strong><span>内容运营控制面</span></div><IconButton label="关闭导航" className="mobile-close" onClick={()=>setMobileOpen(false)}><X size={18}/></IconButton></div>
      <div className="tenant-switcher"><div className="tenant-avatar">{session.tenant.name.slice(0,1)}</div><label><span className="visually-hidden">切换租户</span><select value={session.tenant.id} onChange={event=>onTenant(event.target.value)}>{tenants.map(tenant=><option key={tenant.id} value={tenant.id}>{tenant.name}</option>)}</select><small>{session.role}</small></label><ChevronDown size={16}/></div>
      <nav>
        {globalItems.filter(item=>!item.roles||item.roles.includes(session.role)).map(item=><NavItem key={item.id} {...item} active={view===item.id} onClick={()=>navigate(item.id)}/>)}
        <div className="nav-label"><span>当前项目</span>{project&&<Status value={project.status}/>}</div>
        {project ? projectItems.map(item=><NavItem key={item.id} {...item} active={view===item.id} onClick={()=>navigate(item.id)}/>) : canManage ? <button className="nav-create" onClick={onCreateProject}>创建项目</button> : null}
      </nav>
      <div className="sidebar-footer"><Settings size={17}/><div><strong>{session.user.display_name}</strong><span>{session.user.email}</span></div><IconButton label="退出登录" onClick={onLogout}><LogOut size={16}/></IconButton></div>
    </aside>
    {mobileOpen&&<button className="sidebar-scrim" aria-label="关闭导航" onClick={()=>setMobileOpen(false)}/>}
    <main className="main"><header className="topbar"><div className="project-select-wrap"><span>项目</span><select value={project?.id||''} onChange={(event)=>{const next=projects.find(p=>p.id===event.target.value);if(next)onProject(next)}}><option value="">全部项目</option>{projects.map(p=><option key={p.id} value={p.id}>{p.brand_name} · {p.product_name}</option>)}</select></div>{canManage&&<button className="new-project" onClick={onCreateProject}>新建项目</button>}</header>{children}</main>
  </div>
}

function NavItem({id,label,icon:Icon,active,onClick}:{id:View;label:string;icon:typeof LayoutDashboard;active:boolean;onClick:()=>void}) { return <button data-view={id} className={`nav-item ${active?'active':''}`} onClick={onClick}><Icon size={18}/><span>{label}</span></button> }
