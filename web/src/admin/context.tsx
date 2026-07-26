import { createContext, useCallback, useContext, useEffect, useMemo, useState, type PropsWithChildren } from 'react';
import { api, patch } from '../api';
import type { PlatformOverview, Session, Tenant } from '../types';

interface AdminContextValue {
  session:Session;
  data?:PlatformOverview;
  loading:boolean;
  refreshing:boolean;
  error:string;
  clearError:()=>void;
  refresh:(silent?:boolean)=>Promise<void>;
  setTenantStatus:(tenantID:string,status:'active'|'suspended')=>Promise<Tenant>;
}

const AdminContext=createContext<AdminContextValue|undefined>(undefined);

export function AdminProvider({session,children}:PropsWithChildren<{session:Session}>) {
  const [data,setData]=useState<PlatformOverview>();
  const [loading,setLoading]=useState(true);
  const [refreshing,setRefreshing]=useState(false);
  const [error,setError]=useState('');
  const refresh=useCallback(async(silent=false)=>{
    silent?setRefreshing(true):setLoading(true);setError('');
    try{setData(await api<PlatformOverview>('/api/v1/admin/dashboard'))}
    catch(value){setError(value instanceof Error?value.message:'后台数据加载失败')}
    finally{setLoading(false);setRefreshing(false)}
  },[]);
  useEffect(()=>{refresh()},[refresh]);
  const setTenantStatus=useCallback(async(tenantID:string,status:'active'|'suspended')=>{
    setError('');
    try{const tenant=await patch<Tenant>(`/api/v1/admin/tenants/${tenantID}`,{status});await refresh(true);return tenant}
    catch(value){setError(value instanceof Error?value.message:'租户状态更新失败');throw value}
  },[refresh]);
  const value=useMemo<AdminContextValue>(()=>({session,data,loading,refreshing,error,clearError:()=>setError(''),refresh,setTenantStatus}),[session,data,loading,refreshing,error,refresh,setTenantStatus]);
  return <AdminContext.Provider value={value}>{children}</AdminContext.Provider>;
}

export function useAdmin():AdminContextValue {
  const value=useContext(AdminContext);
  if(!value)throw new Error('useAdmin must be used inside AdminProvider');
  return value;
}
