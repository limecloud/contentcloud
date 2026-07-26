import { createContext, useContext } from 'react';
import type { Dashboard, Session, Tenant } from '../types';

export interface WorkspaceContextValue {
  session:Session;
  tenants:Tenant[];
  dashboard:Dashboard;
  error:string;
  clearError:()=>void;
  refresh:()=>Promise<void>;
  switchTenant:(tenantID:string)=>Promise<boolean>;
  logout:()=>Promise<void>;
}

export const WorkspaceContext=createContext<WorkspaceContextValue|undefined>(undefined);

export function useWorkspace():WorkspaceContextValue {
  const value=useContext(WorkspaceContext);
  if(!value)throw new Error('useWorkspace must be used inside the workspace route');
  return value;
}
