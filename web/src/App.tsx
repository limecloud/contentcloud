import { useCallback, useEffect, useMemo, useState } from 'react';
import { api, post } from './api';
import type { Dashboard, Project, Session, Tenant } from './types';
import { Layout, type View } from './components/Layout';
import { CreateProjectModal } from './components/CreateProjectModal';
import { Banner, Button, Loading } from './components/ui';
import { DashboardView } from './views/DashboardView';
import { OverviewView } from './views/OverviewView';
import { KnowledgeView } from './views/KnowledgeView';
import { BriefsView } from './views/BriefsView';
import { ScriptsView } from './views/ScriptsView';
import { AuditView } from './views/AuditView';
import { ResultsView, SourcesView, StrategyView } from './views/AssetViews';
import { AssetRightsView } from './views/AssetRightsView';
import { LineageView } from './views/LineageView';
import { TeamView } from './views/TeamView';
import { SubmissionsView } from './views/SubmissionsView';
import { DeviceAuthView, PublicReviewView } from './views/PublicViews';
import { LoginView } from './views/auth/LoginView';
import { RegisterView } from './views/auth/RegisterView';

export function App() {
  const reviewMatch=window.location.pathname.match(/^\/review\/([^/]+)$/);
  if(reviewMatch)return <PublicReviewView token={decodeURIComponent(reviewMatch[1])}/>;
  if(window.location.pathname==='/device-auth')return <DeviceAuthView/>;
  const [session,setSession]=useState<Session>();const [tenants,setTenants]=useState<Tenant[]>([]);const [dashboard,setDashboard]=useState<Dashboard>();const [selectedID,setSelectedID]=useState<string>();const [view,setView]=useState<View>('dashboard');const [createOpen,setCreateOpen]=useState(false);const [loading,setLoading]=useState(true);const [authRequired,setAuthRequired]=useState(false);const [error,setError]=useState('');
  const [path,setPath]=useState(window.location.pathname);
  const navigate=useCallback((next:string)=>{window.history.pushState({},'',next);setPath(next)},[]);
  useEffect(()=>{const onPop=()=>setPath(window.location.pathname);window.addEventListener('popstate',onPop);return()=>window.removeEventListener('popstate',onPop)},[]);
  const isAuthRoute=path==='/login'||path==='/register';
  const applyLoaded=(nextSession:Session,nextDashboard:Dashboard,nextTenants:Tenant[])=>{setSession(nextSession);setDashboard(nextDashboard);setTenants(nextTenants);setSelectedID(prev=>nextDashboard.projects.some(project=>project.id===prev)?prev:nextDashboard.projects[0]?.id);setAuthRequired(false)};
  const load=useCallback(async()=>{try{const [nextSession,nextDashboard,nextTenants]=await Promise.all([api<Session>('/api/bff/session'),api<Dashboard>('/api/bff/dashboard'),api<Tenant[]>('/api/bff/tenants')]);applyLoaded(nextSession,nextDashboard,nextTenants)}catch(e){const status=(e as {status?:number}).status;if(status===401){try{await post('/api/v1/dev/bootstrap');const [nextSession,nextDashboard,nextTenants]=await Promise.all([api<Session>('/api/bff/session'),api<Dashboard>('/api/bff/dashboard'),api<Tenant[]>('/api/bff/tenants')]);applyLoaded(nextSession,nextDashboard,nextTenants)}catch{setAuthRequired(true)}}else{setError(e instanceof Error?e.message:'加载失败')}}finally{setLoading(false)}},[]);
  const [reloads,setReloads]=useState(0);
  // 停留在 /login 或 /register 时不拉取会话：dev bootstrap 会静默建号，绕过用户正在填的表单
  useEffect(()=>{if(isAuthRoute){setLoading(false);return}load()},[load,isAuthRoute,reloads]);
  // 登录成功后只切路由并递增 reloads，由上面的 effect 单点触发加载，避免重复请求
  const authSuccess=useCallback(async()=>{window.history.replaceState({},'','/');setLoading(true);setAuthRequired(false);setPath('/');setReloads(n=>n+1)},[]);
  const project=useMemo(()=>dashboard?.projects.find(p=>p.id===selectedID),[dashboard,selectedID]);
  const selectProject=(p:Project)=>{setSelectedID(p.id);setView('overview')};
  const switchTenant=async(tenantID:string)=>{try{await post('/api/bff/session/switch',{tenant_id:tenantID});setSelectedID(undefined);setView('dashboard');await load()}catch(e){setError(e instanceof Error?e.message:'租户切换失败')}};
  const logout=async()=>{try{await post('/api/bff/session/logout');setSession(undefined);setDashboard(undefined);setAuthRequired(true)}catch(e){setError(e instanceof Error?e.message:'退出失败')}};
  if(path==='/register')return <RegisterView onSuccess={authSuccess} onNavigate={navigate} initialInviteToken={new URLSearchParams(window.location.search).get('invite')||undefined}/>;
  if(path==='/login')return <LoginView onSuccess={authSuccess} onNavigate={navigate}/>;
  if(loading)return <div className="splash"><div className="brand-mark">CC</div><Loading/></div>;
  if(authRequired||!session)return <LoginView onSuccess={authSuccess} onNavigate={navigate}/>;
  if(!dashboard)return <div className="fatal"><Banner kind="error">{error||'工作台暂不可用'}</Banner><Button onClick={load}>重试</Button></div>;
  return <Layout session={session} tenants={tenants} projects={dashboard.projects} project={project} view={view} onView={setView} onTenant={switchTenant} onProject={selectProject} onCreateProject={()=>setCreateOpen(true)} onLogout={logout}>
    {error&&<div className="global-banner"><Banner kind="error" onClose={()=>setError('')}>{error}</Banner></div>}
    <ViewContent view={view} session={session} dashboard={dashboard} project={project} onProject={selectProject} onCreate={()=>setCreateOpen(true)} refresh={load}/>
    {createOpen&&<CreateProjectModal role={session.role} onClose={()=>setCreateOpen(false)} onCreated={(p)=>{setCreateOpen(false);load().then(()=>selectProject(p))}}/>}
  </Layout>
}

function ViewContent({view,session,dashboard,project,onProject,onCreate,refresh}:{view:View;session:Session;dashboard:Dashboard;project?:Project;onProject:(p:Project)=>void;onCreate:()=>void;refresh:()=>Promise<void>}) {
  if(view==='team')return <TeamView session={session} onChanged={refresh}/>;
  if(view==='dashboard'||!project)return <DashboardView data={dashboard} canManage={session.role==='tenant_admin'||session.role==='project_manager'} onProject={onProject} onCreate={onCreate}/>;
  switch(view){case'overview':return <OverviewView project={project} role={session.role} onChanged={refresh}/>;case'sources':return <SourcesView project={project}/>;case'assets':return <AssetRightsView project={project}/>;case'knowledge':return <KnowledgeView project={project} onChanged={refresh}/>;case'strategy':return <StrategyView project={project}/>;case'briefs':return <BriefsView project={project}/>;case'scripts':return <ScriptsView project={project}/>;case'submissions':return <SubmissionsView project={project} role={session.role}/>;case'results':return <ResultsView project={project}/>;case'lineage':return <LineageView project={project}/>;case'audit':return <AuditView project={project}/>;default:return null}
}
