import { useState } from 'react';
import { Activity, AlertTriangle, Boxes, CircleDollarSign, FolderKanban, Gauge, GitBranch, LayoutDashboard, LogOut, Menu, RefreshCw, Settings2, ShieldCheck, Users, Workflow, X, type LucideIcon } from 'lucide-react';
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { post } from '../api';
import { Banner, IconButton, Loading } from '../components/ui';
import { BrandLockup, BrandMark } from '../components/Brand';
import { consolePath } from '../consoleRoutes';
import { adminPath } from './routes';
import { useAdmin } from './context';

const routeTitles:Record<string,string>={
  [adminPath('dashboard')]:'运营总览',
  [adminPath('products')]:'创作产品',
  [adminPath('releases')]:'发布记录',
  [adminPath('customers')]:'客户开通',
  [adminPath('capabilities')]:'能力目录',
  [adminPath('skills')]:'技能包',
  [adminPath('executors')]:'执行端',
  [adminPath('jobs')]:'任务记录',
  [adminPath('alerts')]:'异常处理',
  [adminPath('tenants')]:'客户管理',
  [adminPath('audit')]:'操作审计',
  [adminPath('costs')]:'任务用量'
};

export function AdminShell() {
  const {session,data,loading,refreshing,error,clearError,refresh}=useAdmin();
  const location=useLocation();const navigate=useNavigate();
  const [mobileOpen,setMobileOpen]=useState(false);
  const logout=async()=>{await post('/api/bff/session/logout');navigate('/login',{replace:true})};
  return <div className="admin-shell">
    <header className="admin-mobile-header"><IconButton label="打开后台导航" onClick={()=>setMobileOpen(true)}><Menu size={20}/></IconButton><BrandMark/><strong>平台运营后台</strong><Link className="icon-button" aria-label="返回客户工作台" title="返回客户工作台" to={consolePath.studio}><LayoutDashboard size={18}/></Link></header>
    <aside className={`admin-sidebar ${mobileOpen?'admin-sidebar-open':''}`}>
      <div className="admin-brand"><BrandLockup subtitle="平台运营后台"/><IconButton label="关闭后台导航" onClick={()=>setMobileOpen(false)}><X size={18}/></IconButton></div>
      <div className="admin-environment"><span></span><div><strong>运营后台已连接</strong><small>真实运营数据已加载</small></div></div>
      <nav>
        <AdminNav to={adminPath('dashboard')} icon={Gauge} label="运营总览" onClick={()=>setMobileOpen(false)}/>
        <div className="admin-nav-label">创作运营</div>
        <AdminNav to={adminPath('products')} icon={FolderKanban} label="创作产品" onClick={()=>setMobileOpen(false)}/>
        <AdminNav to={adminPath('releases')} icon={GitBranch} label="发布记录" onClick={()=>setMobileOpen(false)}/>
        <AdminNav to={adminPath('customers')} icon={Users} label="客户开通" onClick={()=>setMobileOpen(false)}/>
        <div className="admin-nav-label">能力运营</div>
        <AdminNav to={adminPath('capabilities')} icon={Settings2} label="能力目录" onClick={()=>setMobileOpen(false)}/>
        <AdminNav to={adminPath('skills')} icon={Boxes} label="技能包" onClick={()=>setMobileOpen(false)}/>
        <AdminNav to={adminPath('executors')} icon={Workflow} label="执行端" onClick={()=>setMobileOpen(false)}/>
        <div className="admin-nav-label">运行保障</div>
        <AdminNav to={adminPath('jobs')} icon={Activity} label="任务记录" onClick={()=>setMobileOpen(false)}/>
        <AdminNav to={adminPath('alerts')} icon={AlertTriangle} label="异常处理" onClick={()=>setMobileOpen(false)}/>
        <div className="admin-nav-label">平台管理</div>
        <AdminNav to={adminPath('tenants')} icon={Users} label="客户管理" onClick={()=>setMobileOpen(false)}/>
        <AdminNav to={adminPath('audit')} icon={ShieldCheck} label="操作审计" onClick={()=>setMobileOpen(false)}/>
        <AdminNav to={adminPath('costs')} icon={CircleDollarSign} label="任务用量" onClick={()=>setMobileOpen(false)}/>
      </nav>
      <div className="admin-sidebar-footer"><ShieldCheck size={17}/><div><strong>{session.user.display_name}</strong><span>{session.is_platform_admin?'平台管理员':'租户配置管理员'}</span></div><IconButton label="退出登录" onClick={logout}><LogOut size={16}/></IconButton></div>
    </aside>
    {mobileOpen&&<button className="sidebar-scrim" aria-label="关闭后台导航" onClick={()=>setMobileOpen(false)}/>}
    <main className="admin-main">
      <header className="admin-topbar"><div><strong>{adminRouteTitle(location.pathname)}</strong><span>{data?`数据更新于 ${formatDateTime(data.generated_at)}`:'正在读取运营数据'}</span></div><div className="admin-topbar-actions"><IconButton label="刷新运营数据" disabled={refreshing} onClick={()=>refresh(true)}><RefreshCw className={refreshing?'is-spinning':''} size={17}/></IconButton><Link className="button button-secondary" to={consolePath.studio}><LayoutDashboard size={15}/>客户工作台</Link></div></header>
      <div className="admin-page"><AdminFeedback error={error} onClose={clearError}/>{loading?<div className="admin-loading"><Loading/></div>:!data?<div className="fatal"><Banner kind="error">系统后台暂不可用</Banner><button className="button button-primary" onClick={()=>refresh()}>重试</button></div>:<Outlet/>}</div>
    </main>
  </div>;
}

export function AdminFeedback({error,onClose}:{error:string;onClose:()=>void}) {
  return error?<Banner kind="error" onClose={onClose}>{error}</Banner>:null;
}

function AdminNav({to,icon:Icon,label,count,onClick}:{to:string;icon:LucideIcon;label:string;count?:number;onClick:()=>void}) {return <NavLink to={to} className={({isActive})=>isActive?'active':''} onClick={onClick}><Icon size={18}/><strong>{label}</strong>{count!==undefined&&<small>{count}</small>}</NavLink>}
function adminRouteTitle(pathname:string) { return routeTitles[pathname]||(pathname.startsWith(`${adminPath('products')}/`)?'创作产品':pathname.startsWith(`${adminPath('releases')}/`)?'发布结果':pathname.startsWith(`${adminPath('customers')}/`)?'客户开通':pathname.startsWith(`${adminPath('capabilities')}/`)?'能力详情':'平台运营后台'); }
const formatDateTime=(value:string)=>new Intl.DateTimeFormat('zh-CN',{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'}).format(new Date(value));
