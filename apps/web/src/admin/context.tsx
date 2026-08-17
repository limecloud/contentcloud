import { createContext, useCallback, useContext, useEffect, useMemo, useState, type PropsWithChildren } from 'react';
import { api, patch } from '../api';
import type { AdminWorkOSView, ContentType, OperationsExecutorDirectory, OperationsSkillDirectory, PlatformOverview, PlatformTenant, Session, Tenant } from '../types';
import { normalizeAdminWorkOSView, normalizeOperationsExecutorDirectory, normalizeOperationsSkillDirectory } from './operationsData';

interface AdminContextValue {
  session:Session;
  data?:PlatformOverview;
  workOS?:AdminWorkOSView;
  executorDirectory?:OperationsExecutorDirectory;
  skillDirectory?:OperationsSkillDirectory;
  executorDirectoryError:string;
  skillDirectoryError:string;
  loading:boolean;
  refreshing:boolean;
  error:string;
  clearError:()=>void;
  refresh:(silent?:boolean)=>Promise<void>;
  setTenantStatus:(tenantID:string,status:'active'|'suspended')=>Promise<Tenant>;
  setTenantContentCapability:(tenantID:string,contentType:Exclude<ContentType,'video_script'>,enabled:boolean)=>Promise<PlatformTenant>;
}

const AdminContext=createContext<AdminContextValue|undefined>(undefined);

type OptionalLoad<T>={ok:true;value:T}|{ok:false;error:string};

async function loadOptional<T>(request:Promise<T>):Promise<OptionalLoad<T>> {
  try{return {ok:true,value:await request}}
  catch(value){return {ok:false,error:value instanceof Error?value.message:'目录数据加载失败'}}
}

export async function loadAdminSnapshot(isPlatformAdmin:boolean) {
  const [workOSResponse,executorResult,skillResult]=await Promise.all([
    api<AdminWorkOSView>('/api/bff/admin/work-os'),
    loadOptional(api<OperationsExecutorDirectory>('/api/bff/operations/executors')),
    isPlatformAdmin?loadOptional(api<OperationsSkillDirectory>('/api/bff/operations/skills')):Promise.resolve(undefined)
  ]);
  const workOS=normalizeAdminWorkOSView(workOSResponse);
  const executorDirectory=executorResult.ok
    ?normalizeOperationsExecutorDirectory(executorResult.value)
    :{executors:[],generated_at:workOS.generated_at,online_window_seconds:45};
  const skillDirectory=isPlatformAdmin
    ?skillResult?.ok
      ?normalizeOperationsSkillDirectory(skillResult.value)
      :{configured:false,skills:[],generated_at:workOS.generated_at}
    :undefined;
  const data:PlatformOverview={counts:{tenants:1,active_tenants:workOS.environments.filter(item=>item.status==='active').length,users:0,projects:0,online_devices:executorDirectory.executors.filter(item=>item.presence_status==='online').length,active_runs:workOS.usage.running_count},tenants:[],users:[],generated_at:executorDirectory.generated_at||workOS.generated_at};
  return {
    data,
    workOS,
    executorDirectory,
    skillDirectory,
    executorDirectoryError:executorResult.ok?'':executorResult.error,
    skillDirectoryError:skillResult&&!skillResult.ok?skillResult.error:''
  };
}

export function AdminProvider({session,children}:PropsWithChildren<{session:Session}>) {
  const [data,setData]=useState<PlatformOverview>();
  const [workOS,setWorkOS]=useState<AdminWorkOSView>();
  const [executorDirectory,setExecutorDirectory]=useState<OperationsExecutorDirectory>();
  const [skillDirectory,setSkillDirectory]=useState<OperationsSkillDirectory>();
  const [executorDirectoryError,setExecutorDirectoryError]=useState('');
  const [skillDirectoryError,setSkillDirectoryError]=useState('');
  const [loading,setLoading]=useState(true);
  const [refreshing,setRefreshing]=useState(false);
  const [error,setError]=useState('');
  const refresh=useCallback(async(silent=false)=>{
    silent?setRefreshing(true):setLoading(true);setError('');setExecutorDirectoryError('');setSkillDirectoryError('');
    try{
      const snapshot=await loadAdminSnapshot(session.is_platform_admin);
      setWorkOS(snapshot.workOS);
      setExecutorDirectory(snapshot.executorDirectory);
      setSkillDirectory(snapshot.skillDirectory);
      setExecutorDirectoryError(snapshot.executorDirectoryError);
      setSkillDirectoryError(snapshot.skillDirectoryError);
      setData(snapshot.data)
    }
    catch(value){setError(value instanceof Error?value.message:'后台数据加载失败')}
    finally{setLoading(false);setRefreshing(false)}
  },[session]);
  useEffect(()=>{refresh()},[refresh]);
  const setTenantStatus=useCallback(async(tenantID:string,status:'active'|'suspended')=>{
    setError('');
    try{const tenant=await patch<Tenant>(`/api/v1/admin/tenants/${tenantID}`,{status});await refresh(true);return tenant}
    catch(value){setError(value instanceof Error?value.message:'客户状态更新失败');throw value}
  },[refresh]);
  const setTenantContentCapability=useCallback(async(tenantID:string,contentType:Exclude<ContentType,'video_script'>,enabled:boolean)=>{
    setError('');
    try{const tenant=await api<PlatformTenant>(`/api/v1/admin/tenants/${tenantID}/content-capabilities/${contentType}`,{method:'PUT',body:JSON.stringify({enabled})});await refresh(true);return tenant}
    catch(value){setError(value instanceof Error?value.message:'内容能力更新失败');throw value}
  },[refresh]);
  const value=useMemo<AdminContextValue>(()=>({session,data,workOS,executorDirectory,skillDirectory,executorDirectoryError,skillDirectoryError,loading,refreshing,error,clearError:()=>setError(''),refresh,setTenantStatus,setTenantContentCapability}),[session,data,workOS,executorDirectory,skillDirectory,executorDirectoryError,skillDirectoryError,loading,refreshing,error,refresh,setTenantStatus,setTenantContentCapability]);
  return <AdminContext.Provider value={value}>{children}</AdminContext.Provider>;
}

export function useAdmin():AdminContextValue {
  const value=useContext(AdminContext);
  if(!value)throw new Error('useAdmin must be used inside AdminProvider');
  return value;
}
