import { createContext, useCallback, useContext, useEffect, useMemo, useState, type PropsWithChildren } from 'react';
import { api, patch } from '../api';
import type { AdminWorkOSView, ContentType, PlatformOverview, PlatformTenant, Session, Tenant } from '../types';

interface AdminContextValue {
  session:Session;
  data?:PlatformOverview;
  loading:boolean;
  refreshing:boolean;
  error:string;
  clearError:()=>void;
  refresh:(silent?:boolean)=>Promise<void>;
  setTenantStatus:(tenantID:string,status:'active'|'suspended')=>Promise<Tenant>;
  setTenantContentCapability:(tenantID:string,contentType:Exclude<ContentType,'video_script'>,enabled:boolean)=>Promise<PlatformTenant>;
}

const AdminContext=createContext<AdminContextValue|undefined>(undefined);

export function AdminProvider({session,children}:PropsWithChildren<{session:Session}>) {
  const [data,setData]=useState<PlatformOverview>();
  const [loading,setLoading]=useState(true);
  const [refreshing,setRefreshing]=useState(false);
  const [error,setError]=useState('');
  const refresh=useCallback(async(silent=false)=>{
    silent?setRefreshing(true):setLoading(true);setError('');
    try{
      const workOS=await api<AdminWorkOSView>('/api/bff/admin/work-os');
      setData({counts:{tenants:1,active_tenants:workOS.environments.filter(item=>item.status==='active').length,users:0,projects:0,online_devices:0,active_runs:workOS.usage.running_count},tenants:[],users:[],generated_at:workOS.generated_at})
    }
    catch(value){setError(value instanceof Error?value.message:'后台数据加载失败')}
    finally{setLoading(false);setRefreshing(false)}
  },[session]);
  useEffect(()=>{refresh()},[refresh]);
  const setTenantStatus=useCallback(async(tenantID:string,status:'active'|'suspended')=>{
    setError('');
    try{const tenant=await patch<Tenant>(`/api/v1/admin/tenants/${tenantID}`,{status});await refresh(true);return tenant}
    catch(value){setError(value instanceof Error?value.message:'租户状态更新失败');throw value}
  },[refresh]);
  const setTenantContentCapability=useCallback(async(tenantID:string,contentType:Exclude<ContentType,'video_script'>,enabled:boolean)=>{
    setError('');
    try{const tenant=await api<PlatformTenant>(`/api/v1/admin/tenants/${tenantID}/content-capabilities/${contentType}`,{method:'PUT',body:JSON.stringify({enabled})});await refresh(true);return tenant}
    catch(value){setError(value instanceof Error?value.message:'内容能力更新失败');throw value}
  },[refresh]);
  const value=useMemo<AdminContextValue>(()=>({session,data,loading,refreshing,error,clearError:()=>setError(''),refresh,setTenantStatus,setTenantContentCapability}),[session,data,loading,refreshing,error,refresh,setTenantStatus,setTenantContentCapability]);
  return <AdminContext.Provider value={value}>{children}</AdminContext.Provider>;
}

export function useAdmin():AdminContextValue {
  const value=useContext(AdminContext);
  if(!value)throw new Error('useAdmin must be used inside AdminProvider');
  return value;
}
