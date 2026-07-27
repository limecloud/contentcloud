import { useCallback, useEffect, useMemo, useState } from 'react';
import { Navigate, Outlet, useLocation } from 'react-router-dom';
import { api, post } from './api';
import type { Dashboard, Session, Tenant } from './types';
import { Banner, Button, Loading } from './components/ui';
import { WorkspaceContext, type WorkspaceContextValue } from './workspace/context';
import { loginPath } from './views/auth/returnPath';

export function App() {
  const location=useLocation();
  const [session,setSession]=useState<Session>();
  const [tenants,setTenants]=useState<Tenant[]>([]);
  const [dashboard,setDashboard]=useState<Dashboard>();
  const [loading,setLoading]=useState(true);
  const [authRequired,setAuthRequired]=useState(false);
  const [error,setError]=useState('');

  const load=useCallback(async()=>{
    setError('');
    const loadWorkspace=async()=>{const [nextSession,nextDashboard,nextTenants]=await Promise.all([api<Session>('/api/bff/session'),api<Dashboard>('/api/bff/dashboard'),api<Tenant[]>('/api/bff/tenants')]);setSession(nextSession);setDashboard(nextDashboard);setTenants(nextTenants);setAuthRequired(false)};
    try{await loadWorkspace()}
    catch(value){
      if((value as {status?:number}).status===401){try{await post('/api/v1/dev/bootstrap');await loadWorkspace()}catch{setAuthRequired(true)}}
      else setError(value instanceof Error?value.message:'工作台加载失败');
    }finally{setLoading(false)}
  },[]);
  useEffect(()=>{load()},[load]);

  const switchTenant=useCallback(async(tenantID:string)=>{setError('');try{await post('/api/bff/session/switch',{tenant_id:tenantID});await load();return true}catch(value){setError(value instanceof Error?value.message:'租户切换失败');return false}},[load]);
  const logout=useCallback(async()=>{await post('/api/bff/session/logout');setSession(undefined);setDashboard(undefined)},[]);
  const value=useMemo<WorkspaceContextValue|undefined>(()=>session&&dashboard?{session,tenants,dashboard,error,clearError:()=>setError(''),refresh:load,switchTenant,logout}:undefined,[session,tenants,dashboard,error,load,switchTenant,logout]);

  if(loading)return <div className="splash"><div className="brand-mark">CC</div><Loading/></div>;
  if(error&&!session)return <div className="fatal"><Banner kind="error">{error}</Banner><Button onClick={load}>重试</Button></div>;
  if(authRequired||!session)return <Navigate to={loginPath(location.pathname+location.search)} replace/>;
  if(!value)return <div className="fatal"><Banner kind="error">{error||'工作台暂不可用'}</Banner><Button onClick={load}>重试</Button></div>;
  return <WorkspaceContext.Provider value={value}><Outlet/></WorkspaceContext.Provider>;
}
