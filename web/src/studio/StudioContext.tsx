import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { Navigate, Outlet, useLocation } from 'react-router-dom';
import { BrandMark } from '../components/Brand';
import { Banner, Button, Loading } from '../components/ui';
import { bootstrapDevelopmentSession } from '../devBootstrap';
import { loginPath } from '../views/auth/returnPath';
import { studioApi } from './studioApi';
import type { StudioBootstrap } from './studioTypes';

interface StudioContextValue {
  bootstrap:StudioBootstrap;
  refresh:()=>Promise<void>;
  switchTenant:(tenantID:string)=>Promise<boolean>;
  logout:()=>Promise<void>;
}

const StudioContext=createContext<StudioContextValue|undefined>(undefined);

export function CustomerStudioApp(){
  const location=useLocation();
  const [bootstrap,setBootstrap]=useState<StudioBootstrap>();
  const [loading,setLoading]=useState(true);
  const [authRequired,setAuthRequired]=useState(false);
  const [error,setError]=useState('');

  const load=useCallback(async()=>{
    setError('');
    try{
      setBootstrap(await studioApi.bootstrap());
      setAuthRequired(false);
    }catch(value){
      if((value as {status?:number}).status===401){
        try{
          if(!await bootstrapDevelopmentSession()){setAuthRequired(true);return}
          setBootstrap(await studioApi.bootstrap());
          setAuthRequired(false);
        }catch{setAuthRequired(true)}
      }else setError(value instanceof Error?value.message:'创作台加载失败');
    }finally{setLoading(false)}
  },[]);

  useEffect(()=>{void load()},[load]);
  const switchTenant=useCallback(async(tenantID:string)=>{setError('');try{await studioApi.switchTenant(tenantID);await load();return true}catch(value){setError(value instanceof Error?value.message:'团队切换失败');return false}},[load]);
  const logout=useCallback(async()=>{await studioApi.logout();setBootstrap(undefined)},[]);
  const value=useMemo<StudioContextValue|undefined>(()=>bootstrap?{bootstrap,refresh:load,switchTenant,logout}:undefined,[bootstrap,load,switchTenant,logout]);

  if(loading)return <div className="splash"><BrandMark/><Loading/></div>;
  if(authRequired||!bootstrap&&!error)return <Navigate to={loginPath(location.pathname+location.search)} replace/>;
  if(!value)return <div className="fatal"><Banner kind="error">{error||'创作台暂不可用'}</Banner><Button onClick={()=>void load()}>重试</Button></div>;
  return <StudioContext.Provider value={value}><Outlet/></StudioContext.Provider>;
}

export function useStudio():StudioContextValue {
  const value=useContext(StudioContext);
  if(!value)throw new Error('useStudio must be used inside the customer studio route');
  return value;
}
