import { useCallback, useEffect, useState } from 'react';
import { Navigate, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { api, postWithoutBody } from '../api';
import type { Session } from '../types';
import { Banner, Button, Loading } from '../components/ui';
import { BrandMark } from '../components/Brand';
import { AdminProvider } from './context';
import { loginPath } from '../views/auth/returnPath';
import { consolePath } from '../consoleRoutes';

export function AdminRoute() {
  const navigate=useNavigate();const location=useLocation();
  const [session,setSession]=useState<Session>();
  const [loading,setLoading]=useState(true);
  const [authRequired,setAuthRequired]=useState(false);
  const [error,setError]=useState('');

  const loadSession=useCallback(async()=>{
    setLoading(true);setError('');
    try{setSession(await api<Session>('/api/bff/session'));setAuthRequired(false)}
    catch(value){
      if((value as {status?:number}).status===401){
        try{await postWithoutBody('/api/v1/dev/bootstrap');setSession(await api<Session>('/api/bff/session'));setAuthRequired(false)}
        catch{setAuthRequired(true)}
      }else setError(value instanceof Error?value.message:'管理员会话加载失败');
    }finally{setLoading(false)}
  },[]);

  useEffect(()=>{loadSession()},[loadSession]);
  if(loading)return <div className="splash"><BrandMark/><Loading/></div>;
  if(error)return <div className="fatal"><Banner kind="error">{error}</Banner><Button onClick={loadSession}>重试</Button></div>;
  if(authRequired||!session)return <Navigate to={loginPath(location.pathname+location.search)} replace/>;
  if(!session.is_platform_admin&&!['tenant_admin','project_manager'].includes(session.role))return <div className="fatal"><Banner kind="error">当前账号没有后台配置权限</Banner><Button onClick={()=>navigate(consolePath.dashboard)}>返回工作台</Button></div>;
  return <AdminProvider session={session}><Outlet/></AdminProvider>;
}
