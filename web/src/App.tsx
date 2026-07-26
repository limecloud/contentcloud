import { useCallback, useEffect, useMemo, useState } from 'react';
import { LockKeyhole } from 'lucide-react';
import { api, post } from './api';
import type { Dashboard, Project, Session, Tenant } from './types';
import { Layout, type View } from './components/Layout';
import { CreateProjectModal } from './components/CreateProjectModal';
import { Banner, Button, Field, Loading } from './components/ui';
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
import { DeviceAuthView, PublicReviewView } from './views/PublicViews';

export function App() {
  const reviewMatch=window.location.pathname.match(/^\/review\/([^/]+)$/);
  if(reviewMatch)return <PublicReviewView token={decodeURIComponent(reviewMatch[1])}/>;
  if(window.location.pathname==='/device-auth')return <DeviceAuthView/>;
  const [session,setSession]=useState<Session>();const [tenants,setTenants]=useState<Tenant[]>([]);const [dashboard,setDashboard]=useState<Dashboard>();const [selectedID,setSelectedID]=useState<string>();const [view,setView]=useState<View>('dashboard');const [createOpen,setCreateOpen]=useState(false);const [loading,setLoading]=useState(true);const [authRequired,setAuthRequired]=useState(false);const [error,setError]=useState('');
  const applyLoaded=(nextSession:Session,nextDashboard:Dashboard,nextTenants:Tenant[])=>{setSession(nextSession);setDashboard(nextDashboard);setTenants(nextTenants);setSelectedID(prev=>nextDashboard.projects.some(project=>project.id===prev)?prev:nextDashboard.projects[0]?.id);setAuthRequired(false)};
  const load=useCallback(async()=>{try{const [nextSession,nextDashboard,nextTenants]=await Promise.all([api<Session>('/api/bff/session'),api<Dashboard>('/api/bff/dashboard'),api<Tenant[]>('/api/bff/tenants')]);applyLoaded(nextSession,nextDashboard,nextTenants)}catch(e){const status=(e as {status?:number}).status;if(status===401){try{await post('/api/v1/dev/bootstrap');const [nextSession,nextDashboard,nextTenants]=await Promise.all([api<Session>('/api/bff/session'),api<Dashboard>('/api/bff/dashboard'),api<Tenant[]>('/api/bff/tenants')]);applyLoaded(nextSession,nextDashboard,nextTenants)}catch{setAuthRequired(true)}}else{setError(e instanceof Error?e.message:'加载失败')}}finally{setLoading(false)}},[]);
  useEffect(()=>{load()},[load]);
  const project=useMemo(()=>dashboard?.projects.find(p=>p.id===selectedID),[dashboard,selectedID]);
  const selectProject=(p:Project)=>{setSelectedID(p.id);setView('overview')};
  const switchTenant=async(tenantID:string)=>{try{await post('/api/bff/session/switch',{tenant_id:tenantID});setSelectedID(undefined);setView('dashboard');await load()}catch(e){setError(e instanceof Error?e.message:'租户切换失败')}};
  const logout=async()=>{try{await post('/api/bff/session/logout');setSession(undefined);setDashboard(undefined);setAuthRequired(true)}catch(e){setError(e instanceof Error?e.message:'退出失败')}};
  if(loading)return <div className="splash"><div className="brand-mark">CC</div><Loading/></div>;
  if(authRequired||!session)return <Login onSuccess={load}/>;
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
  switch(view){case'overview':return <OverviewView project={project} role={session.role} onChanged={refresh}/>;case'sources':return <SourcesView project={project}/>;case'assets':return <AssetRightsView project={project}/>;case'knowledge':return <KnowledgeView project={project} onChanged={refresh}/>;case'strategy':return <StrategyView project={project}/>;case'briefs':return <BriefsView project={project}/>;case'scripts':return <ScriptsView project={project}/>;case'results':return <ResultsView project={project}/>;case'lineage':return <LineageView project={project}/>;case'audit':return <AuditView project={project}/>;default:return null}
}

function Login({onSuccess}:{onSuccess:()=>Promise<void>}) {const [mode,setMode]=useState<'login'|'register'>('login');const [form,setForm]=useState({email:'',password:'',display_name:'',tenant_name:''});const [error,setError]=useState('');const [busy,setBusy]=useState(false);const submit=async()=>{setBusy(true);setError('');try{await post(`/api/v1/auth/${mode}`,form);await onSuccess()}catch(e){setError(e instanceof Error?e.message:'登录失败')}finally{setBusy(false)}};return <main className="auth-page"><section className="auth-panel"><div className="auth-brand"><div className="brand-mark">CC</div><strong>ContentCloud</strong></div><div className="auth-icon"><LockKeyhole size={22}/></div><h1>{mode==='login'?'登录工作台':'创建团队'}</h1><div className="auth-form">{mode==='register'&&<><Field label="姓名"><input value={form.display_name} onChange={e=>setForm({...form,display_name:e.target.value})}/></Field><Field label="团队名称"><input value={form.tenant_name} onChange={e=>setForm({...form,tenant_name:e.target.value})}/></Field></>}<Field label="邮箱"><input type="email" value={form.email} onChange={e=>setForm({...form,email:e.target.value})}/></Field><Field label="密码"><input type="password" value={form.password} onChange={e=>setForm({...form,password:e.target.value})}/></Field>{error&&<p className="form-error">{error}</p>}<Button disabled={busy||!form.email||!form.password} onClick={submit}>{busy?'提交中…':mode==='login'?'登录':'创建团队'}</Button><button className="auth-switch" onClick={()=>setMode(mode==='login'?'register':'login')}>{mode==='login'?'创建新团队':'返回登录'}</button></div></section></main>}
