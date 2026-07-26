import { Navigate, useNavigate, useOutletContext, useParams } from 'react-router-dom';
import type { Project } from '../types';
import { DashboardView } from '../views/DashboardView';
import { TeamView } from '../views/TeamView';
import { OverviewView } from '../views/OverviewView';
import { ResultsView, SourcesView, StrategyView } from '../views/AssetViews';
import { AssetRightsView } from '../views/AssetRightsView';
import { KnowledgeView } from '../views/KnowledgeView';
import { BriefsView } from '../views/BriefsView';
import { ScriptsView } from '../views/ScriptsView';
import { SubmissionsView } from '../views/SubmissionsView';
import { LineageView } from '../views/LineageView';
import { AuditView } from '../views/AuditView';
import { useWorkspace } from './context';
import type { WorkspaceOutletContext } from './WorkspaceShell';

export function WorkspaceDashboardPage() {
  const {session,dashboard}=useWorkspace();const {openCreateProject}=useOutletContext<WorkspaceOutletContext>();const navigate=useNavigate();
  const selectProject=(project:Project)=>navigate(`/workspace/projects/${project.id}/overview`);
  return <DashboardView data={dashboard} canManage={session.role==='tenant_admin'||session.role==='project_manager'} onProject={selectProject} onCreate={openCreateProject}/>;
}

export function WorkspaceTeamPage() {const {session,refresh}=useWorkspace();return <TeamView session={session} onChanged={refresh}/>}

export type ProjectView='overview'|'sources'|'assets'|'knowledge'|'strategy'|'briefs'|'scripts'|'submissions'|'results'|'lineage'|'audit';

export function WorkspaceProjectPage({view}:{view:ProjectView}) {
  const {projectID}=useParams();const {session,dashboard,refresh}=useWorkspace();
  const project=dashboard.projects.find(item=>item.id===projectID);
  if(!project)return <Navigate to="/workspace/dashboard" replace/>;
  switch(view){case'overview':return <OverviewView project={project} role={session.role} onChanged={refresh}/>;case'sources':return <SourcesView project={project}/>;case'assets':return <AssetRightsView project={project}/>;case'knowledge':return <KnowledgeView project={project} onChanged={refresh}/>;case'strategy':return <StrategyView project={project}/>;case'briefs':return <BriefsView project={project}/>;case'scripts':return <ScriptsView project={project}/>;case'submissions':return <SubmissionsView project={project} role={session.role}/>;case'results':return <ResultsView project={project}/>;case'lineage':return <LineageView project={project}/>;case'audit':return <AuditView project={project}/>}
}
