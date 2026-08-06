import { useState } from 'react';
import { Activity, Gauge, GitBranch, LayoutDashboard, LogOut, Menu, PackageCheck, RefreshCw, Settings2, ShieldCheck, SlidersHorizontal, Workflow, X } from 'lucide-react';
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { post } from '../api';
import { Banner, IconButton, Loading } from '../components/ui';
import { BrandLockup, BrandMark } from '../components/Brand';
import { consolePath } from '../consoleRoutes';
import { adminPath } from './routes';
import { useAdmin } from './context';

const routeTitles:Record<string,string>={
  [adminPath('dashboard')]:'运行基础设施',
  [adminPath('tenants')]:'租户管理',
  [adminPath('users')]:'用户目录',
  [adminPath('environments')]:'执行环境',
  [adminPath('sops')]:'流程规范',
  [adminPath('gates')]:'检查与审批',
  [adminPath('capabilities')]:'本地能力',
  [adminPath('runtime')]:'Runtime 运行',
  [adminPath('audit')]:'权限与审计',
  [adminPath('usage')]:'用量与成本'
};

export function AdminShell() {
  const {session,data,loading,refreshing,error,clearError,refresh}=useAdmin();
  const location=useLocation();const navigate=useNavigate();
  const [mobileOpen,setMobileOpen]=useState(false);
  const logout=async()=>{await post('/api/bff/session/logout');navigate('/login',{replace:true})};
  return <div className="admin-shell">
    <header className="admin-mobile-header"><IconButton label="打开后台导航" onClick={()=>setMobileOpen(true)}><Menu size={20}/></IconButton><BrandMark/><strong>运行基础设施</strong><Link className="icon-button" aria-label="返回工作台" title="返回工作台" to={consolePath.dashboard}><LayoutDashboard size={18}/></Link></header>
    <aside className={`admin-sidebar ${mobileOpen?'admin-sidebar-open':''}`}>
      <div className="admin-brand"><BrandLockup subtitle="运行基础设施"/><IconButton label="关闭后台导航" onClick={()=>setMobileOpen(false)}><X size={18}/></IconButton></div>
      <div className="admin-environment"><span></span><div><strong>运行基础设施在线</strong><small>{data?.counts.active_tenants||0} 个活跃租户 · 配置可用</small></div></div>
      <nav>
        <AdminNav to={adminPath('dashboard')} icon={Gauge} label="概览" onClick={()=>setMobileOpen(false)}/>
        <div className="admin-nav-label">运行配置</div>
        <AdminNav to={adminPath('environments')} icon={Workflow} label="执行环境" onClick={()=>setMobileOpen(false)}/>
        <AdminNav to={adminPath('sops')} icon={GitBranch} label="流程规范" onClick={()=>setMobileOpen(false)}/>
        <AdminNav to={adminPath('gates')} icon={SlidersHorizontal} label="检查与审批" onClick={()=>setMobileOpen(false)}/>
        <AdminNav to={adminPath('capabilities')} icon={Settings2} label="本地能力" onClick={()=>setMobileOpen(false)}/>
        <AdminNav to={adminPath('runtime')} icon={Activity} label="Runtime 运行" onClick={()=>setMobileOpen(false)}/>
        <div className="admin-nav-label">治理</div>
        <AdminNav to={adminPath('audit')} icon={ShieldCheck} label="权限与审计" onClick={()=>setMobileOpen(false)}/>
        <AdminNav to={adminPath('usage')} icon={PackageCheck} label="用量与成本" onClick={()=>setMobileOpen(false)}/>
      </nav>
      <div className="admin-sidebar-footer"><ShieldCheck size={17}/><div><strong>{session.user.display_name}</strong><span>{session.is_platform_admin?'平台管理员':'租户配置管理员'}</span></div><IconButton label="退出登录" onClick={logout}><LogOut size={16}/></IconButton></div>
    </aside>
    {mobileOpen&&<button className="sidebar-scrim" aria-label="关闭后台导航" onClick={()=>setMobileOpen(false)}/>}
    <main className="admin-main">
      <header className="admin-topbar"><div><strong>{routeTitles[location.pathname]||'运行基础设施'}</strong><span>{data?`更新于 ${formatDateTime(data.generated_at)}`:'正在读取运行配置'}</span></div><div className="admin-topbar-actions"><IconButton label="刷新数据" disabled={refreshing} onClick={()=>refresh(true)}><RefreshCw className={refreshing?'is-spinning':''} size={17}/></IconButton><Link className="button button-secondary" to={consolePath.dashboard}><LayoutDashboard size={15}/>工作台</Link></div></header>
      <div className="admin-page"><AdminFeedback error={error} onClose={clearError}/>{loading?<div className="admin-loading"><Loading/></div>:!data?<div className="fatal"><Banner kind="error">系统后台暂不可用</Banner><button className="button button-primary" onClick={()=>refresh()}>重试</button></div>:<Outlet/>}</div>
    </main>
  </div>;
}

export function AdminFeedback({error,onClose}:{error:string;onClose:()=>void}) {
  return error?<Banner kind="error" onClose={onClose}>{error}</Banner>:null;
}

function AdminNav({to,icon:Icon,label,count,onClick}:{to:string;icon:typeof Gauge;label:string;count?:number;onClick:()=>void}) {return <NavLink to={to} className={({isActive})=>isActive?'active':''} onClick={onClick}><Icon size={18}/><strong>{label}</strong>{count!==undefined&&<small>{count}</small>}</NavLink>}
const formatDateTime=(value:string)=>new Intl.DateTimeFormat('zh-CN',{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'}).format(new Date(value));
