import { consolePath } from '../../consoleRoutes';

const projectIDPattern=/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/;
const staticPaths=new Set(['/admin','/admin/dashboard','/admin/tenants']);
const studioPaths=new Set([consolePath.studio,consolePath.team,'/studio/connect','/studio/tasks','/studio/tasks/new','/studio/assets','/studio/knowledge','/studio/deliveries']);
const defaultReturnPath=consolePath.studio;

export function safeReturnPath(value:string|null|undefined):string {
  if(!value||!value.startsWith('/')||value.startsWith('//')||value.includes('\\')||hasControlCharacter(value))return defaultReturnPath;
  let target:URL;
  try{target=new URL(value,'https://contentcloud.invalid')}catch{return defaultReturnPath}
  if(target.origin!=='https://contentcloud.invalid'||target.hash)return defaultReturnPath;
  const studioPath=safeStudioPath(target);
  if(studioPath)return studioPath;
  if(staticPaths.has(target.pathname)&&!target.search)return target.pathname;
  return defaultReturnPath;
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
  const allowedParams=target.pathname==='/studio/tasks/new'?['experience','project','asset_ref','material_ref']:target.pathname==='/studio/assets'?['task_id']:target.pathname==='/studio/knowledge'?['project']:target.pathname==='/studio/connect'?['session','project']:[];
  if(allowedParams.length===0||target.searchParams.size===0)return undefined;
  const entries:string[]=[];
  for(const key of allowedParams){
    const values=target.searchParams.getAll(key);
    if(values.length>1)return undefined;
    if(values.length===1){if(!projectIDPattern.test(values[0]))return undefined;entries.push(`${key}=${encodeURIComponent(values[0])}`)}
  }
  if(entries.length!==target.searchParams.size)return undefined;
  if(target.pathname==='/studio/connect'&&entries.length!==1)return undefined;
  return `${target.pathname}?${entries.join('&')}`;
}
