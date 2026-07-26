export const consolePath = {
  dashboard: '/',
  team: '/team',
  project: (projectID:string,view:string='overview')=>`/projects/${projectID}/${view}`
} as const;

export function canonicalConsolePath(pathname:string):string {
  if(pathname==='/workspace'||pathname==='/workspace/'||pathname==='/workspace/dashboard')return consolePath.dashboard;
  if(pathname==='/workspace/team')return consolePath.team;
  if(pathname.startsWith('/workspace/projects/'))return pathname.slice('/workspace'.length);
  return consolePath.dashboard;
}
