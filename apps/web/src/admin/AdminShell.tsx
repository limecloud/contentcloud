import { useState } from 'react';
import { Activity, AlertTriangle, Boxes, CircleDollarSign, FolderKanban, Gauge, GitBranch, LayoutDashboard, LogOut, Menu, RefreshCw, Settings2, ShieldCheck, Users, Workflow, X, type LucideIcon, PlugZap } from 'lucide-react';
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { post } from '../api';
import { Banner, IconButton, Loading } from '../components/ui';
import { BrandLockup, BrandMark } from '../components/Brand';
import { consolePath } from '../consoleRoutes';
import { adminPath } from './routes';
import { useAdmin } from './context';

const routeTitles:Record<string,string>={
  [adminPath('dashboard')]:'运营总览',
  [adminPath('products')]:'创作流程',
  [adminPath('releases')]:'发布版本',
  [adminPath('customers')]:'客户设置',
  [adminPath('capabilities')]:'功能清单',
  [adminPath('skills')]:'自动化工具',
  [adminPath('executors')]:'连接的电脑',
  [adminPath('providers')]:'视频服务',
  [adminPath('jobs')]:'任务进度',
  [adminPath('alerts')]:'需要处理',
  [adminPath('tenants')]:'客户列表',
  [adminPath('audit')]:'变更记录',
  [adminPath('costs')]:'任务统计'
};

export function AdminShell() {
  const {session,data,loading,refreshing,error,clearError,refresh}=useAdmin();
  const location=useLocation();const navigate=useNavigate();
  const [mobileOpen,setMobileOpen]=useState(false);
  const logout=async()=>{await post('/api/bff/session/logout');navigate('/login',{replace:true})};
  return <div className="admin-shell">
    <header className="admin-mobile-header"><IconButton label="打开后台导航" onClick={()=>setMobileOpen(true)}><Menu size={20}/></IconButton><BrandMark/><strong>后台管理</strong><Link className="icon-button" aria-label="返回我的创作" title="返回我的创作" to={consolePath.studio}><LayoutDashboard size={18}/></Link></header>
    <aside className={`admin-sidebar ${mobileOpen?'admin-sidebar-open':''}`}>
      <div className="admin-brand"><BrandLockup subtitle="后台管理"/><IconButton label="关闭后台导航" onClick={()=>setMobileOpen(false)}><X size={18}/></IconButton></div>
      <div className="admin-environment"><span></span><div><strong>后台已连接</strong><small>最新数据已加载</small></div></div>
      <nav>
        <AdminNav to={adminPath('dashboard')} icon={Gauge} label="运营总览" onClick={()=>setMobileOpen(false)}/>
        <div className="admin-nav-label">内容管理</div>
        <AdminNav to={adminPath('products')} icon={FolderKanban} label="创作流程" onClick={()=>setMobileOpen(false)}/>
        <AdminNav to={adminPath('releases')} icon={GitBranch} label="发布版本" onClick={()=>setMobileOpen(false)}/>
        <AdminNav to={adminPath('customers')} icon={Users} label="客户设置" onClick={()=>setMobileOpen(false)}/>
        <div className="admin-nav-label">功能设置</div>
        <AdminNav to={adminPath('capabilities')} icon={Settings2} label="功能清单" onClick={()=>setMobileOpen(false)}/>
        <AdminNav to={adminPath('skills')} icon={Boxes} label="自动化工具" onClick={()=>setMobileOpen(false)}/>
        <AdminNav to={adminPath('executors')} icon={Workflow} label="连接的电脑" onClick={()=>setMobileOpen(false)}/>
        <AdminNav to={adminPath('providers')} icon={PlugZap} label="视频服务" onClick={()=>setMobileOpen(false)}/>
        <div className="admin-nav-label">任务跟进</div>
        <AdminNav to={adminPath('jobs')} icon={Activity} label="任务进度" onClick={()=>setMobileOpen(false)}/>
        <AdminNav to={adminPath('alerts')} icon={AlertTriangle} label="需要处理" onClick={()=>setMobileOpen(false)}/>
        <div className="admin-nav-label">账号与记录</div>
        <AdminNav to={adminPath('tenants')} icon={Users} label="客户列表" onClick={()=>setMobileOpen(false)}/>
        <AdminNav to={adminPath('audit')} icon={ShieldCheck} label="变更记录" onClick={()=>setMobileOpen(false)}/>
        <AdminNav to={adminPath('costs')} icon={CircleDollarSign} label="任务统计" onClick={()=>setMobileOpen(false)}/>
      </nav>
      <div className="admin-sidebar-footer"><ShieldCheck size={17}/><div><strong>{session.user.display_name}</strong><span>{session.is_platform_admin?'平台管理员':'客户设置管理员'}</span></div><IconButton label="退出登录" onClick={logout}><LogOut size={16}/></IconButton></div>
    </aside>
    {mobileOpen&&<button className="sidebar-scrim" aria-label="关闭后台导航" onClick={()=>setMobileOpen(false)}/>}
    <main className="admin-main">
      <header className="admin-topbar"><div><strong>{adminRouteTitle(location.pathname)}</strong><span>{data?`数据更新于 ${formatDateTime(data.generated_at)}`:'正在读取最新数据'}</span></div><div className="admin-topbar-actions"><IconButton label="刷新数据" disabled={refreshing} onClick={()=>refresh(true)}><RefreshCw className={refreshing?'is-spinning':''} size={17}/></IconButton><Link className="button button-secondary" to={consolePath.studio}><LayoutDashboard size={15}/>我的创作</Link></div></header>
      <div className="admin-page"><AdminFeedback error={error} onClose={clearError}/>{loading?<div className="admin-loading"><Loading/></div>:!data?<div className="fatal"><Banner kind="error">系统后台暂不可用</Banner><button className="button button-primary" onClick={()=>refresh()}>重试</button></div>:<Outlet/>}</div>
    </main>
  </div>;
}

export function AdminFeedback({error,onClose}:{error:string;onClose:()=>void}) {
  return error?<Banner kind="error" onClose={onClose}>{error}</Banner>:null;
}

function AdminNav({to,icon:Icon,label,count,onClick}:{to:string;icon:LucideIcon;label:string;count?:number;onClick:()=>void}) {return <NavLink to={to} className={({isActive})=>isActive?'active':''} onClick={onClick}><Icon size={18}/><strong>{label}</strong>{count!==undefined&&<small>{count}</small>}</NavLink>}
function adminRouteTitle(pathname:string) { return routeTitles[pathname]||(pathname.startsWith(`${adminPath('products')}/`)?'创作流程':pathname.startsWith(`${adminPath('releases')}/`)?'发布结果':pathname.startsWith(`${adminPath('customers')}/`)?'客户设置':pathname.startsWith(`${adminPath('capabilities')}/`)?'功能详情':'后台管理'); }
const formatDateTime=(value:string)=>new Intl.DateTimeFormat('zh-CN',{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'}).format(new Date(value));
