import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { Navigate, Outlet, useLocation } from 'react-router-dom';
import { BrandMark } from '../components/Brand';
import { Banner, Button, Loading } from '../components/ui';
import { bootstrapDevelopmentSession } from '../devBootstrap';
import { loginPath } from '../views/auth/returnPath';
import { studioApi } from './studioApi';
import { ALL_EXPERIENCES } from './studioScope';
import type { StudioBootstrap } from './studioTypes';

interface StudioContextValue {
  bootstrap:StudioBootstrap;
  refresh:()=>Promise<void>;
  switchTenant:(tenantID:string)=>Promise<boolean>;
  selectedExperienceID:string;
  selectExperience:(experienceID:string)=>void;
  logout:()=>Promise<void>;
}

const StudioContext=createContext<StudioContextValue|undefined>(undefined);
export { ALL_EXPERIENCES } from './studioScope';

function storedExperienceID():string {
  if(typeof window==='undefined')return '';
  return window.localStorage.getItem('contentcloud.selected-experience')||'';
}

export function CustomerStudioApp(){
  const location=useLocation();
  const [bootstrap,setBootstrap]=useState<StudioBootstrap>();
  const [loading,setLoading]=useState(true);
  const [authRequired,setAuthRequired]=useState(false);
  const [error,setError]=useState('');
  const [selectedExperienceID,setSelectedExperienceID]=useState(storedExperienceID);

  const load=useCallback(async()=>{
    setError('');
    try{
      const nextBootstrap=await studioApi.bootstrap();
      setBootstrap(nextBootstrap);
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
  useEffect(()=>{
    if(!bootstrap)return;
    if(selectedExperienceID===ALL_EXPERIENCES&&bootstrap.experiences.length>0)return;
    if(selectedExperienceID&&bootstrap.experiences.some(item=>item.id===selectedExperienceID))return;
    const nextID=bootstrap.experiences[0]?.id||'';
    setSelectedExperienceID(nextID);
    if(typeof window!=='undefined'){
      if(nextID)window.localStorage.setItem('contentcloud.selected-experience',nextID);
      else window.localStorage.removeItem('contentcloud.selected-experience');
    }
  },[bootstrap,selectedExperienceID]);
  const switchTenant=useCallback(async(tenantID:string)=>{setError('');try{await studioApi.switchTenant(tenantID);await load();return true}catch(value){setError(value instanceof Error?value.message:'团队切换失败');return false}},[load]);
  const selectExperience=useCallback((experienceID:string)=>{
    setSelectedExperienceID(experienceID);
    if(typeof window!=='undefined'){
      if(experienceID)window.localStorage.setItem('contentcloud.selected-experience',experienceID);
      else window.localStorage.removeItem('contentcloud.selected-experience');
    }
  },[]);
  const logout=useCallback(async()=>{await studioApi.logout();setBootstrap(undefined)},[]);
  const value=useMemo<StudioContextValue|undefined>(()=>bootstrap?{bootstrap,refresh:load,switchTenant,selectedExperienceID,selectExperience,logout}:undefined,[bootstrap,load,switchTenant,selectedExperienceID,selectExperience,logout]);

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
