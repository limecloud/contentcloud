import { useMemo, useState } from 'react';
import { Outlet, useLocation, useNavigate } from 'react-router-dom';
import type { Project } from '../types';
import { Layout, type View } from '../components/Layout';
import { CreateProjectModal } from '../components/CreateProjectModal';
import { Banner } from '../components/ui';
import { useWorkspace } from './context';

const projectViews=new Set<View>(['overview','sources','assets','knowledge','strategy','briefs','scripts','submissions','results','lineage','audit']);

export function WorkspaceShell() {
  const {session,tenants,dashboard,error,clearError,refresh,switchTenant,logout}=useWorkspace();
  const location=useLocation();const navigate=useNavigate();const [createOpen,setCreateOpen]=useState(false);
  const match=location.pathname.match(/^\/workspace\/projects\/([^/]+)\/([^/]+)$/);
  const project=useMemo(()=>dashboard.projects.find(item=>item.id===match?.[1]),[dashboard.projects,match?.[1]]);
  const routeView=(match?.[2]&&projectViews.has(match[2] as View)?match[2]:location.pathname.endsWith('/team')?'team':'dashboard') as View;
  const selectProject=(value:Project)=>navigate(`/workspace/projects/${value.id}/overview`);
  const selectView=(view:View)=>navigate(view==='dashboard'||view==='team'?`/workspace/${view}`:project?`/workspace/projects/${project.id}/${view}`:'/workspace/dashboard');
  const signOut=async()=>{await logout();navigate('/login',{replace:true})};
  return <Layout session={session} tenants={tenants} projects={dashboard.projects} project={project} view={routeView} onView={selectView} onTenant={async id=>{if(await switchTenant(id))navigate('/workspace/dashboard')}} onProject={selectProject} onCreateProject={()=>setCreateOpen(true)} onAdmin={()=>navigate('/admin/dashboard')} onLogout={signOut}>
    {error&&<div className="global-banner"><Banner kind="error" onClose={clearError}>{error}</Banner></div>}
    <Outlet context={{openCreateProject:()=>setCreateOpen(true)}}/>
    {createOpen&&<CreateProjectModal role={session.role} onClose={()=>setCreateOpen(false)} onCreated={value=>{setCreateOpen(false);refresh().then(()=>selectProject(value))}}/>}
  </Layout>;
}

export interface WorkspaceOutletContext {openCreateProject:()=>void}
