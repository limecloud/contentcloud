import { useNavigate, useOutletContext } from 'react-router-dom';
import type { Project } from '../types';
import { DashboardView } from '../views/DashboardView';
import { TeamView } from '../views/TeamView';
import { consolePath } from '../consoleRoutes';
import { V3ProjectPage } from '../v3/ProjectPage';
import type { ProjectView } from '../v3/page-contracts';
import { useWorkspace } from './context';
import type { ConsoleOutletContext } from './WorkspaceShell';

export function ConsoleDashboardPage() {
  const {session,dashboard}=useWorkspace();const {openCreateProject}=useOutletContext<ConsoleOutletContext>();const navigate=useNavigate();
  const selectProject=(project:Project)=>navigate(consolePath.project(project.id));
  return <DashboardView data={dashboard} canManage={session.role==='tenant_admin'||session.role==='project_manager'} onProject={selectProject} onCreate={openCreateProject}/>;
}

export function ConsoleTeamPage() {const {session,refresh}=useWorkspace();return <TeamView session={session} onChanged={refresh}/>}

export function ConsoleProjectPage({view}:{view:ProjectView}) {
  return <V3ProjectPage view={view}/>;
}
