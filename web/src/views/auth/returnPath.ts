import { consolePath } from '../../consoleRoutes';
import { projectNavigationFromSearch, projectViewFromRoute } from '../../v3/page-contracts';

const projectIDPattern=/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/;
const staticPaths=new Set([consolePath.dashboard,consolePath.team,'/admin','/admin/dashboard','/admin/tenants','/admin/users']);
const studioPaths=new Set([consolePath.studio,'/studio/connect','/studio/tasks','/studio/tasks/new','/studio/assets','/studio/deliveries']);
const defaultReturnPath=consolePath.studio;

export function safeReturnPath(value:string|null|undefined):string {
  if(!value||!value.startsWith('/')||value.startsWith('//')||value.includes('\\')||hasControlCharacter(value))return defaultReturnPath;
  let target:URL;
  try{target=new URL(value,'https://contentcloud.invalid')}catch{return defaultReturnPath}
  if(target.origin!=='https://contentcloud.invalid'||target.hash)return defaultReturnPath;
  const studioPath=safeStudioPath(target);
  if(studioPath)return studioPath;
  if(staticPaths.has(target.pathname)&&!target.search)return target.pathname;

  const match=target.pathname.match(/^\/projects\/([^/]+)\/([^/]+)$/);
  if(!match)return defaultReturnPath;
  let projectID:string;
  let route:string;
  try{projectID=decodeURIComponent(match[1]);route=decodeURIComponent(match[2])}catch{return defaultReturnPath}
  if(!projectIDPattern.test(projectID))return defaultReturnPath;
  const view=projectViewFromRoute(route);
  if(!view)return defaultReturnPath;
  const navigation=projectNavigationFromSearch(view,target.search);
  if(!navigation)return defaultReturnPath;
  return consolePath.projectNavigation(projectID,navigation)||defaultReturnPath;
}

export function loginPath(returnPath:string):string {
  return `/login?next=${encodeURIComponent(safeReturnPath(returnPath))}`;
}

function hasControlCharacter(value:string):boolean {
  for(let index=0;index<value.length;index+=1){
    const code=value.charCodeAt(index);
    if(code<0x20||code===0x7f)return true;
  }
  return false;
}

function safeStudioPath(target:URL):string|undefined {
  const taskMatch=target.pathname.match(/^\/studio\/tasks\/([^/]+)$/);
  if(taskMatch&&!target.search){
    let taskID:string;
    try{taskID=decodeURIComponent(taskMatch[1])}catch{return undefined}
    return projectIDPattern.test(taskID)?`/studio/tasks/${encodeURIComponent(taskID)}`:undefined;
  }
  if(!studioPaths.has(target.pathname))return undefined;
  if(!target.search)return target.pathname;
  const allowedParam=target.pathname==='/studio/tasks/new'?'experience':target.pathname==='/studio/assets'?'task_id':target.pathname==='/studio/connect'?(target.searchParams.has('session')?'session':'project'):'';
  if(!allowedParam||target.searchParams.size!==1||!target.searchParams.has(allowedParam))return undefined;
  const value=target.searchParams.get(allowedParam)||'';
  if(!projectIDPattern.test(value))return undefined;
  return `${target.pathname}?${allowedParam}=${encodeURIComponent(value)}`;
}
