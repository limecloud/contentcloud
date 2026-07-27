import { projectNavigationSuffix, projectRoute, type ProjectNavigationTarget, type ProjectView } from './v3/page-contracts';

export const consolePath = {
  dashboard: '/',
  team: '/team',
  project: (projectID:string,view:ProjectView='setup')=>`/projects/${encodeURIComponent(projectID)}/${projectRoute(view)}`,
  projectNavigation: (projectID:string,target:ProjectNavigationTarget|undefined)=>{
    const suffix=projectNavigationSuffix(target);
    return suffix===undefined?undefined:`/projects/${encodeURIComponent(projectID)}/${suffix}`;
  }
} as const;
