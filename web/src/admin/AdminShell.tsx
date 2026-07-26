import { useState } from 'react';
import { Building2, Gauge, LayoutDashboard, LogOut, Menu, RefreshCw, ShieldCheck, Users, X } from 'lucide-react';
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { post } from '../api';
import { Banner, IconButton, Loading } from '../components/ui';
import { adminPath } from './routes';
import { useAdmin } from './context';

const routeTitles:Record<string,string>={
  [adminPath('dashboard')]:'系统概览',
  [adminPath('tenants')]:'租户管理',
  [adminPath('users')]:'用户目录'
};

export function AdminShell() {
  const {session,data,loading,refreshing,error,clearError,refresh}=useAdmin();
  const location=useLocation();const navigate=useNavigate();
  const [mobileOpen,setMobileOpen]=useState(false);
  const logout=async()=>{await post('/api/bff/session/logout');navigate('/login',{replace:true})};
  return <div className="admin-shell">
    <header className="admin-mobile-header"><IconButton label="打开后台导航" onClick={()=>setMobileOpen(true)}><Menu size={20}/></IconButton><div className="brand-mark">CC</div><strong>系统后台</strong><Link className="icon-button" aria-label="返回工作台" title="返回工作台" to="/"><LayoutDashboard size={18}/></Link></header>
    <aside className={`admin-sidebar ${mobileOpen?'admin-sidebar-open':''}`}>
      <div className="admin-brand"><div className="brand-mark">CC</div><div><strong>ContentCloud</strong><span>系统后台</span></div><IconButton label="关闭后台导航" onClick={()=>setMobileOpen(false)}><X size={18}/></IconButton></div>
      <div className="admin-environment"><span></span><div><strong>平台运行中</strong><small>{data?.counts.active_tenants||0} 个活跃租户</small></div></div>
      <nav>
        <AdminNav to={adminPath('dashboard')} icon={Gauge} label="概览" onClick={()=>setMobileOpen(false)}/>
        <AdminNav to={adminPath('tenants')} icon={Building2} label="租户" count={data?.counts.tenants} onClick={()=>setMobileOpen(false)}/>
        <AdminNav to={adminPath('users')} icon={Users} label="用户" count={data?.counts.users} onClick={()=>setMobileOpen(false)}/>
      </nav>
      <div className="admin-sidebar-footer"><ShieldCheck size={17}/><div><strong>{session.user.display_name}</strong><span>平台管理员</span></div><IconButton label="退出登录" onClick={logout}><LogOut size={16}/></IconButton></div>
    </aside>
    {mobileOpen&&<button className="sidebar-scrim" aria-label="关闭后台导航" onClick={()=>setMobileOpen(false)}/>}
    <main className="admin-main">
      <header className="admin-topbar"><div><strong>{routeTitles[location.pathname]||'系统后台'}</strong><span>{data?`更新于 ${formatDateTime(data.generated_at)}`:'正在读取平台数据'}</span></div><div className="admin-topbar-actions"><IconButton label="刷新数据" disabled={refreshing} onClick={()=>refresh(true)}><RefreshCw className={refreshing?'is-spinning':''} size={17}/></IconButton><Link className="button button-secondary" to="/"><LayoutDashboard size={15}/>工作台</Link></div></header>
      <div className="admin-page">{error&&<Banner kind="error" onClose={clearError}>{error}</Banner>}{loading?<div className="admin-loading"><Loading/></div>:!data?<div className="fatal"><Banner kind="error">系统后台暂不可用</Banner><button className="button button-primary" onClick={()=>refresh()}>重试</button></div>:<Outlet/>}</div>
    </main>
  </div>;
}

function AdminNav({to,icon:Icon,label,count,onClick}:{to:string;icon:typeof Gauge;label:string;count?:number;onClick:()=>void}) {return <NavLink to={to} className={({isActive})=>isActive?'active':''} onClick={onClick}><Icon size={18}/><strong>{label}</strong>{count!==undefined&&<small>{count}</small>}</NavLink>}
const formatDateTime=(value:string)=>new Intl.DateTimeFormat('zh-CN',{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'}).format(new Date(value));
