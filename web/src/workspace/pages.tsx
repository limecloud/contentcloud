import { Navigate } from 'react-router-dom';
import { TeamView } from '../views/TeamView';
import type { ProjectView } from '../v3/page-contracts';
import { V3ProjectPage } from '../v3/ProjectPage';
import { useWorkspace } from './context';
import { WorkOSHomePage, WorkOSInboxPage, WorkOSKnowledgePage, WorkOSNewTaskPage, WorkOSSOPPage, WorkOSTaskDetailPage, WorkOSTaskListPage } from './workOS';

export function ConsoleDashboardPage() {
  return <WorkOSHomePage/>;
}

export function WorkspaceKnowledgeRedirectPage() {
  const {dashboard} = useWorkspace();
  const project = dashboard.projects[0];
  return <Navigate to={project ? `/projects/${encodeURIComponent(project.id)}/knowledge` : '/workspace'} replace/>;
}

export function ConsoleTeamPage() {const {session,refresh}=useWorkspace();return <TeamView session={session} onChanged={refresh}/>}

export function ConsoleProjectPage({view}:{view:ProjectView}) {
  return <V3ProjectPage view={view}/>;
}

export {WorkOSInboxPage,WorkOSKnowledgePage,WorkOSNewTaskPage,WorkOSSOPPage,WorkOSTaskDetailPage,WorkOSTaskListPage};
