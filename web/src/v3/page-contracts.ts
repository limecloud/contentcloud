import rawRegistry from '../../../contracts/project-pages-1.0.json';

export type ProjectView = keyof typeof rawRegistry.views;
export const projectViewIDs = Object.freeze([...rawRegistry.order]) as readonly ProjectView[];
export type ProjectionSectionID = 'onboarding'|'methodology'|'knowledge'|'intelligence'|'strategy'|'planning'|'creative'|'review'|'delivery'|'learning'|'automation';

export interface ProjectPageContract {
  id:ProjectView;
  label:string;
  eyebrow:string;
  title:string;
  description:string;
  section?:ProjectionSectionID;
  submissionTypes:readonly string[];
  snapshotTypes:readonly string[];
  focusKinds:readonly string[];
}

export interface ProjectPageFocus {
  kind:string;
  id:string;
  digest?:string;
}

export interface ProjectPageFocusError {
  code:'PROJECT_FOCUS_INVALID'|'PROJECT_FOCUS_DIGEST_INVALID'|'PROJECT_FOCUS_DIGEST_REQUIRED';
  message:string;
}

export interface ProjectNavigationTarget {
  view:string;
  focus?:ProjectPageFocus;
}

export const projectPageContracts=projectViewIDs.reduce((contracts,id)=>{
  const raw=rawRegistry.views[id];
  contracts[id]={
    id,
    label:raw.label,
    eyebrow:raw.eyebrow,
    title:raw.title,
    description:raw.description,
    section:(raw.section||undefined) as ProjectionSectionID|undefined,
    submissionTypes:raw.submission_types,
    snapshotTypes:raw.snapshot_types,
    focusKinds:raw.focus_kinds.map(focus=>focus.kind)
  };
  return contracts;
},{} as Record<ProjectView,ProjectPageContract>);

export function projectRoute(view:ProjectView):string {
  return rawRegistry.views[view].route;
}

export function projectNavigationSuffix(value:unknown):string|undefined {
  if(!isProjectNavigationTarget(value))return undefined;
  const target=value;
  if(!isProjectView(target.view))return undefined;
  const route=projectRoute(target.view);
  if(!target.focus)return route;
  if(validateProjectFocus(target.view,target.focus))return undefined;
  const candidate=rawRegistry.views[target.view].focus_kinds.find(value=>value.kind===target.focus?.kind);
  if(!candidate)return undefined;
  const params=new URLSearchParams();
  if('query_key' in candidate&&typeof candidate.query_key==='string'&&candidate.query_key)params.set(candidate.query_key,target.focus.id);
  else{params.set('focus_kind',target.focus.kind);params.set('focus_id',target.focus.id)}
  if(target.focus.digest)params.set('expected_digest',target.focus.digest);
  return `${route}?${params.toString()}`;
}

export function projectNavigationFromSearch(view:ProjectView,search:string):ProjectNavigationTarget|undefined {
  const params=new URLSearchParams(search);
  if([...params.keys()].some(key=>!projectFocusQueryKeys(view).has(key)))return undefined;
  const parsed=projectFocusFromSearch(view,search);
  if(parsed.error)return undefined;
  if(params.size>0&&!parsed.focus)return undefined;
  return parsed.focus?{view,focus:parsed.focus}:{view};
}

export function projectViewFromRoute(route:string):ProjectView|undefined {
  return projectViewIDs.find(view=>projectRoute(view)===route);
}

export function projectFocusFromSearch(view:ProjectView,search:string):{focus?:ProjectPageFocus;error?:ProjectPageFocusError} {
  const params=new URLSearchParams(search);
  const raw=rawRegistry.views[view];
  const legacy=raw.focus_kinds.flatMap(candidate=>{
    const key='query_key' in candidate?candidate.query_key:undefined;
    return key&&params.has(key)?[{candidate,id:params.get(key)||'',count:params.getAll(key).length}]:[];
  });
  const hasGeneric=params.has('focus_kind')||params.has('focus_id')||params.has('expected_digest');
  const repeatedGeneric=['focus_kind','focus_id','expected_digest'].some(key=>params.getAll(key).length>1);
  if(repeatedGeneric||legacy.length>1||(legacy.length===1&&hasGeneric)||legacy.some(value=>value.count!==1))return focusError('PROJECT_FOCUS_INVALID','页面焦点参数存在冲突');

  let candidate:typeof raw.focus_kinds[number]|undefined;
  let focus:ProjectPageFocus|undefined;
  if(legacy.length===1){
    candidate=legacy[0].candidate;
    focus={kind:candidate.kind,id:legacy[0].id};
  }else if(hasGeneric){
    const kind=params.get('focus_kind')||'';
    candidate=raw.focus_kinds.find(value=>value.kind===kind);
    focus={kind,id:params.get('focus_id')||'',digest:params.get('expected_digest')||undefined};
  }else return {};

  const validation=validateProjectFocus(view,focus);
  if(validation)return {error:validation};
  return {focus};
}

const focusIDPattern=/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/;
const focusDigestPattern=/^sha256:[a-f0-9]{64}$/;
function focusError(code:ProjectPageFocusError['code'],message:string):{error:ProjectPageFocusError}{return {error:{code,message}}}

function validateProjectFocus(view:ProjectView,focus:ProjectPageFocus):ProjectPageFocusError|undefined {
  const candidate=rawRegistry.views[view].focus_kinds.find(value=>value.kind===focus.kind);
  if(!candidate||!focusIDPattern.test(focus.id))return {code:'PROJECT_FOCUS_INVALID',message:'页面焦点与当前视图不匹配，或焦点标识（ID）无效'};
  if(focus.digest&&!focusDigestPattern.test(focus.digest))return {code:'PROJECT_FOCUS_DIGEST_INVALID',message:'页面焦点摘要（digest）必须是完整的 sha256 摘要'};
  if('digest_required' in candidate&&candidate.digest_required&&!focus.digest)return {code:'PROJECT_FOCUS_DIGEST_REQUIRED',message:'该页面焦点需要不可变的内容版本摘要（revision digest）'};
  return undefined;
}

function projectFocusQueryKeys(view:ProjectView):Set<string> {
  const keys=new Set(['focus_kind','focus_id','expected_digest']);
  for(const candidate of rawRegistry.views[view].focus_kinds){
    if('query_key' in candidate&&typeof candidate.query_key==='string')keys.add(candidate.query_key);
  }
  return keys;
}

export function isProjectView(value:string):value is ProjectView {
  return (projectViewIDs as readonly string[]).includes(value);
}

function isProjectNavigationTarget(value:unknown):value is ProjectNavigationTarget {
  if(!value||typeof value!=='object')return false;
  const target=value as Record<string,unknown>;
  if(typeof target.view!=='string')return false;
  if(target.focus===undefined)return true;
  if(!target.focus||typeof target.focus!=='object')return false;
  const focus=target.focus as Record<string,unknown>;
  return typeof focus.kind==='string'&&typeof focus.id==='string'&&(focus.digest===undefined||typeof focus.digest==='string');
}
