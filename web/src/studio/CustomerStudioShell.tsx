import { Archive, ChevronDown, ClipboardCheck, LayoutDashboard, ListTodo, LogOut, Menu, MonitorUp, PackageCheck, Sparkles, X } from 'lucide-react';
import { useState } from 'react';
import { Link, Navigate, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { BrandLockup, BrandMark } from '../components/Brand';
import { IconButton } from '../components/ui';
import { useStudio } from './StudioContext';
import './studio.css';

const navItems=[
  {to:'/studio',end:true,label:'今天',icon:Sparkles},
  {to:'/studio/connect',end:true,label:'连接创作电脑',icon:MonitorUp},
  {to:'/studio/tasks',end:false,label:'创作任务',icon:ListTodo},
  {to:'/studio/assets',end:false,label:'资产',icon:Archive},
  {to:'/studio/deliveries',end:false,label:'交付',icon:PackageCheck},
] as const;

export function CustomerStudioShell(){
  const {bootstrap,switchTenant,logout}=useStudio();
  const {session,tenants}=bootstrap;
  const navigate=useNavigate();
  const location=useLocation();
  const [mobileOpen,setMobileOpen]=useState(false);
  const [accountOpen,setAccountOpen]=useState(false);
  const canOperate=session.can_view_operations&&Boolean(session.operations_path);
  const activeProjects=bootstrap.projects.filter(project=>project.status!=='archived');
  const needsConnection=session.can_create&&activeProjects.length>0&&activeProjects.every(project=>!project.execution_client_connected);
  const signOut=async()=>{await logout();navigate('/login',{replace:true})};
  if(location.pathname==='/studio'&&needsConnection)return <Navigate to="/studio/connect" replace/>;
  return <div className="studio-shell">
    <header className="studio-mobile-header"><BrandMark/><strong>创作台</strong><IconButton label={mobileOpen?'关闭导航':'打开导航'} onClick={()=>setMobileOpen(value=>!value)}>{mobileOpen?<X size={20}/>:<Menu size={20}/>}</IconButton></header>
    <aside className={`studio-sidebar ${mobileOpen?'is-open':''}`}>
      <div className="studio-brand"><BrandLockup subtitle="客户创作台"/></div>
      <nav aria-label="客户创作台导航">{navItems.map(({to,end,label,icon:Icon})=><NavLink key={to} to={to} end={end} onClick={()=>setMobileOpen(false)} className={({isActive})=>isActive?'is-active':''}><Icon size={18}/><span>{label}</span></NavLink>)}</nav>
      {canOperate&&<Link className="studio-operations-link" to={session.operations_path||'/workspace'}><LayoutDashboard size={16}/><span>运营与管理</span></Link>}
      <div className="studio-sidebar-footer"><span>{session.user.display_name.slice(0,1).toUpperCase()}</span><div><strong>{session.user.display_name}</strong><small>{session.tenant.name}</small></div><IconButton label="退出登录" onClick={signOut}><LogOut size={16}/></IconButton></div>
    </aside>
    {mobileOpen&&<button className="studio-scrim" type="button" aria-label="关闭导航" onClick={()=>setMobileOpen(false)}/>}
    <main className="studio-main">
      <header className="studio-topbar"><label><span>当前团队</span><select value={session.tenant.id} onChange={async event=>{if(await switchTenant(event.target.value))navigate('/studio')}}>{tenants.map(tenant=><option key={tenant.id} value={tenant.id}>{tenant.name}</option>)}</select><ChevronDown size={14}/></label><div><ClipboardCheck size={16}/><span>所有确认都会固定当前版本</span></div><button type="button" onClick={()=>setAccountOpen(value=>!value)} aria-expanded={accountOpen}><span>{session.user.display_name.slice(0,1).toUpperCase()}</span><strong>{session.user.display_name}</strong><ChevronDown size={14}/></button>{accountOpen&&<div className="studio-account-menu">{session.can_manage_team&&<Link to="/team">团队成员</Link>}{canOperate&&<Link to={session.operations_path||'/workspace'}>运营与管理</Link>}<button type="button" onClick={signOut}>退出登录</button></div>}</header>
      <div className="studio-page"><Outlet/></div>
    </main>
  </div>;
}
