import { consolePath } from '../../consoleRoutes';
import { projectNavigationFromSearch, projectViewFromRoute } from '../../v3/page-contracts';

const projectIDPattern=/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/;
const staticPaths=new Set([consolePath.dashboard,consolePath.team,'/admin','/admin/dashboard','/admin/tenants','/admin/users']);

export function safeReturnPath(value:string|null|undefined):string {
  if(!value||!value.startsWith('/')||value.startsWith('//')||value.includes('\\')||hasControlCharacter(value))return consolePath.dashboard;
  let target:URL;
  try{target=new URL(value,'https://contentcloud.invalid')}catch{return consolePath.dashboard}
  if(target.origin!=='https://contentcloud.invalid'||target.hash)return consolePath.dashboard;
  if(staticPaths.has(target.pathname)&&!target.search)return target.pathname;

  const match=target.pathname.match(/^\/projects\/([^/]+)\/([^/]+)$/);
  if(!match)return consolePath.dashboard;
  let projectID:string;
  let route:string;
  try{projectID=decodeURIComponent(match[1]);route=decodeURIComponent(match[2])}catch{return consolePath.dashboard}
  if(!projectIDPattern.test(projectID))return consolePath.dashboard;
  const view=projectViewFromRoute(route);
  if(!view)return consolePath.dashboard;
  const navigation=projectNavigationFromSearch(view,target.search);
  if(!navigation)return consolePath.dashboard;
  return consolePath.projectNavigation(projectID,navigation)||consolePath.dashboard;
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
