import { useMemo, useState } from 'react';
import { Navigate, Outlet, useLocation, useNavigate } from 'react-router-dom';
import type { Project } from '../types';
import { Layout } from '../components/Layout';
import { CreateProjectModal } from '../components/CreateProjectModal';
import { Banner } from '../components/ui';
import { consolePath } from '../consoleRoutes';
import { useWorkspace } from './context';

export function ConsoleShell() {
  const {session,tenants,dashboard,error,clearError,refresh,switchTenant,logout}=useWorkspace();
  const location=useLocation();const navigate=useNavigate();const [createOpen,setCreateOpen]=useState(false);
  const match=location.pathname.match(/^\/projects\/([^/]+)/);
  const projectID=match?.[1] ? decodeURIComponent(match[1]) : undefined;
  // Workspace routes still need a working Project context. Keep the URL
  // authoritative when present, otherwise use the first available Project.
  const project=useMemo(()=>projectID ? dashboard.projects.find(item=>item.id===projectID) : dashboard.projects[0],[dashboard.projects,projectID]);
  const setupPath=project ? consolePath.project(project.id,'setup') : undefined;
  const needsConnection=Boolean(project&&project.status!=='archived'&&project.connected_devices===0);
  const isConsoleSurface=location.pathname.startsWith('/workspace')||Boolean(projectID);
  if(needsConnection&&isConsoleSurface&&setupPath&&location.pathname!==setupPath)return <Navigate to={setupPath} replace/>;
  const selectProject=(value:Project)=>navigate(`/projects/${encodeURIComponent(value.id)}/tasks`);
  const signOut=async()=>{await logout();navigate('/login',{replace:true})};
  return <Layout session={session} tenants={tenants} projects={dashboard.projects} project={project} onTenant={async id=>{if(await switchTenant(id))navigate(consolePath.dashboard)}} onProject={selectProject} onCreateProject={()=>setCreateOpen(true)} onAdmin={()=>navigate('/admin/dashboard')} onLogout={signOut}>
    {error&&<div className="global-banner"><Banner kind="error" onClose={clearError}>{error}</Banner></div>}
    <Outlet context={{openCreateProject:()=>setCreateOpen(true)}}/>
    {createOpen&&<CreateProjectModal role={session.role} onClose={()=>setCreateOpen(false)} onCreated={value=>{setCreateOpen(false);refresh().then(()=>selectProject(value))}}/>}
  </Layout>;
}

export interface ConsoleOutletContext {openCreateProject:()=>void}
