import { BookOpenCheck, ChevronDown, Clapperboard, ClipboardList, LayoutDashboard, LogOut, Menu, MessageSquareMore, PackageCheck, PlugZap, Radar, Settings, Shield, SlidersHorizontal, Target, TrendingUp, Users, Workflow, X } from 'lucide-react';
import { useState, type ReactNode } from 'react';
import type { Project, Session, Tenant } from '../types';
import type { ProjectView } from '../v3/page-contracts';
import { IconButton, Status } from './ui';

export type View = 'dashboard'|'team'|ProjectView;
const globalItems: {id:View;label:string;icon:typeof LayoutDashboard;roles?:string[]}[] = [{id:'dashboard',label:'工作台',icon:LayoutDashboard},{id:'team',label:'团队',icon:Users}];
const projectItems: {id:ProjectView;label:string;icon:typeof LayoutDashboard;group:'start'|'work'|'automation'}[] = [
  {id:'setup',label:'接入与初始化',icon:PlugZap,group:'start'},
  {id:'overview',label:'项目总览',icon:LayoutDashboard,group:'start'},
  {id:'context',label:'方法论与上下文',icon:SlidersHorizontal,group:'work'},
  {id:'knowledge',label:'可信知识',icon:BookOpenCheck,group:'work'},
  {id:'intelligence',label:'市场情报',icon:Radar,group:'work'},
  {id:'strategy',label:'营销策略',icon:Target,group:'work'},
  {id:'planning',label:'内容策划',icon:ClipboardList,group:'work'},
  {id:'creative',label:'创意与剧本',icon:Clapperboard,group:'work'},
  {id:'review',label:'审核协作',icon:MessageSquareMore,group:'work'},
  {id:'delivery',label:'交付制作',icon:PackageCheck,group:'work'},
  {id:'learning',label:'结果学习',icon:TrendingUp,group:'work'},
  {id:'automation',label:'Automation 与运行',icon:Workflow,group:'automation'}
];

export function Layout({session,tenants,projects,project,view,onView,onTenant,onProject,onCreateProject,onAdmin,onLogout,children}: {session:Session;tenants:Tenant[];projects:Project[];project?:Project;view:View;onView:(view:View)=>void;onTenant:(tenantID:string)=>void;onProject:(project:Project)=>void;onCreateProject:()=>void;onAdmin:()=>void;onLogout:()=>void;children:ReactNode}) {
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
        {session.is_platform_admin&&<button className="nav-item" onClick={onAdmin}><Shield size={18}/><span>系统后台</span></button>}
        <div className="nav-label"><span>当前项目</span>{project&&<Status value={project.status}/>}</div>
        {project ? projectItems.map((item,index)=><div className="nav-entry" key={item.id}>{(index===0||projectItems[index-1].group!==item.group)&&<div className="nav-group-label">{item.group==='start'?'项目':item.group==='work'?'业务流程':'自动化'}</div>}<NavItem {...item} active={view===item.id} onClick={()=>navigate(item.id)}/></div>) : canManage ? <button className="nav-create" onClick={onCreateProject}>创建项目</button> : null}
      </nav>
      <div className="sidebar-footer"><Settings size={17}/><div><strong>{session.user.display_name}</strong><span>{session.user.email}</span></div><IconButton label="退出登录" onClick={onLogout}><LogOut size={16}/></IconButton></div>
    </aside>
    {mobileOpen&&<button className="sidebar-scrim" aria-label="关闭导航" onClick={()=>setMobileOpen(false)}/>}
    <main className="main"><header className="topbar"><div className="project-select-wrap"><span>项目</span><select value={project?.id||''} onChange={(event)=>{const next=projects.find(p=>p.id===event.target.value);if(next)onProject(next)}}><option value="">全部项目</option>{projects.map(p=><option key={p.id} value={p.id}>{p.brand_name} · {p.product_name}</option>)}</select></div>{canManage&&<button className="new-project" onClick={onCreateProject}>新建项目</button>}</header>{children}</main>
  </div>
}

function NavItem({id,label,icon:Icon,active,onClick}:{id:View;label:string;icon:typeof LayoutDashboard;active:boolean;onClick:()=>void}) { return <button data-view={id} className={`nav-item ${active?'active':''}`} onClick={onClick}><Icon size={18}/><span>{label}</span></button> }
