import { AlertCircle, ArrowLeft, CheckCircle2, Download, LoaderCircle, PencilLine, Plus, Sparkles } from 'lucide-react';
import { useCallback, useEffect, useState, type ReactNode } from 'react';
import { Link, useParams, useSearchParams } from 'react-router-dom';
import { Button } from '../../components/ui';
import { studioApi } from '../studioApi';
import type { StudioTaskSummary, WorkspaceMaterialItem } from '../studioTypes';
import {
  completedTaskStatuses,
  formatAssetBytes,
  formatAssetDate,
  materialHref,
  materialOriginLabel,
  materialStateLabel,
  materialTypeLabels,
  resultStateLabel,
  resultTypeLabels,
} from './assetFormat';
import { CreativeResultViewer, MaterialIcon, MaterialViewer, ResultIcon } from './AssetViewers';

export function StudioMaterialDetailPage(){
  const {materialID=''}=useParams();
  const [searchParams]=useSearchParams();
  const [item,setItem]=useState<WorkspaceMaterialItem>();
  const [tasks,setTasks]=useState<StudioTaskSummary[]>([]);
  const [loading,setLoading]=useState(true);
  const [error,setError]=useState('');
  const load=useCallback(async()=>{
    setLoading(true);setError('');
    try{
      const [surface,nextTasks]=await Promise.all([studioApi.assets(),studioApi.tasks()]);
      const found=surface.workspace.materials.find(material=>material.material_ref===`material:${materialID}`);
      if(!found)throw new Error('没有找到这项资产，可能已被移除或不属于当前团队');
      setItem(found);setTasks(nextTasks);
    }catch(value){setError(value instanceof Error?value.message:'资产加载失败')}
    finally{setLoading(false)}
  },[materialID]);
  useEffect(()=>{void load()},[load]);
  if(loading)return <AssetPageLoading/>;
  if(error||!item)return <AssetPageError message={error||'没有找到这项资产'} onRetry={load}/>;
  return <AssetDetailFrame backTo="/studio/assets" eyebrow={`我的资产 · ${materialTypeLabels[item.material_kind]}`} title={item.title} state={materialStateLabel(item.processing_state)} icon={<MaterialIcon kind={item.material_kind}/> }>
    <MaterialViewer item={item}/>
    <AssetFacts title="文件信息" rows={[
      ['文件名',item.file_name],['所属项目',item.project_name],['文件类型',item.mime_type],['文件大小',formatAssetBytes(item.byte_size)],['来源',materialOriginLabel(item.origin)],['权利提示',item.rights_summary],['更新时间',formatAssetDate(item.updated_at)],
    ]}/>
    <AssetUsePanel itemProjectID={item.project_id} tasks={tasks} initialTaskID={searchParams.get('task_id')||''} canReuse createHref={`/studio/tasks/new?material_ref=${encodeURIComponent(item.material_ref)}&project=${encodeURIComponent(item.project_id)}`} downloadHref={materialHref(item)} onAttach={taskID=>studioApi.attachMaterials(taskID,[item.material_ref])}/>
  </AssetDetailFrame>;
}

export function StudioCreativeResultDetailPage(){
  const {taskID='',resultID=''}=useParams();
  const [searchParams]=useSearchParams();
  const [detail,setDetail]=useState<Awaited<ReturnType<typeof studioApi.creativeResult>>>();
  const [tasks,setTasks]=useState<StudioTaskSummary[]>([]);
  const [loading,setLoading]=useState(true);
  const [error,setError]=useState('');
  const load=useCallback(async()=>{
    setLoading(true);setError('');
    try{const [nextDetail,nextTasks]=await Promise.all([studioApi.creativeResult(resultID,taskID),studioApi.tasks()]);setDetail(nextDetail);setTasks(nextTasks)}
    catch(value){setError(value instanceof Error?value.message:'创作结果加载失败')}
    finally{setLoading(false)}
  },[resultID,taskID]);
  useEffect(()=>{void load()},[load]);
  if(loading)return <AssetPageLoading/>;
  if(error||!detail)return <AssetPageError message={error||'没有找到这项创作结果'} onRetry={load}/>;
  const {item}=detail;
  return <AssetDetailFrame backTo="/studio/assets" eyebrow={`创作结果 · ${resultTypeLabels[item.result_type]}`} title={item.title} state={resultStateLabel(item)} icon={<ResultIcon type={item.result_type}/>} sourceTask={<Link to={`/studio/tasks/${encodeURIComponent(item.task_id)}`}><PencilLine size={15}/>在来源任务中修改</Link>}>
    <CreativeResultViewer detail={detail}/>
    <AssetFacts title="版本信息" rows={[
      ['所属项目',item.project_name],['来源任务',item.task_title],['当前版本',item.version],['确认状态',resultStateLabel(item)],['生成时间',formatAssetDate(item.created_at)],
    ]}/>
    <AssetUsePanel itemProjectID={item.project_id} tasks={tasks} initialTaskID={searchParams.get('task_id')||''} canReuse={item.reusable} blockedReason={item.blocked_reason} createHref={`/studio/tasks/new?asset_ref=${encodeURIComponent(item.ref)}&project=${encodeURIComponent(item.project_id)}`} downloads={item.downloads.map(file=>({href:file.href,label:file.file_name}))} onAttach={targetTaskID=>studioApi.attachAssets(targetTaskID,[item.ref])}/>
  </AssetDetailFrame>;
}

function AssetDetailFrame({backTo,eyebrow,title,state,icon,sourceTask,children}:{backTo:string;eyebrow:string;title:string;state:string;icon:ReactNode;sourceTask?:ReactNode;children:ReactNode}){
  const parts=Array.isArray(children)?children:[children];
  return <div className="studio-view studio-asset-detail-page"><Link className="studio-back" to={backTo}><ArrowLeft size={15}/>返回资产</Link><header className="studio-asset-page-header"><span className="studio-asset-page-icon">{icon}</span><div><small>{eyebrow}</small><h1>{title}</h1><div><b>{state}</b><span><CheckCircle2 size={13}/>固定版本，只读</span></div></div>{sourceTask&&<div className="studio-asset-source-action">{sourceTask}</div>}</header><div className="studio-asset-page-layout"><main>{parts[0]}</main><aside>{parts.slice(1)}</aside></div></div>;
}

function AssetFacts({title,rows}:{title:string;rows:Array<[string,string]>}){return <section className="studio-asset-facts"><h2>{title}</h2><dl>{rows.map(([label,value])=><div key={label}><dt>{label}</dt><dd>{value||'未记录'}</dd></div>)}</dl></section>}

function AssetUsePanel({itemProjectID,tasks,initialTaskID,canReuse,blockedReason,createHref,downloadHref,downloads=[],onAttach}:{itemProjectID:string;tasks:StudioTaskSummary[];initialTaskID:string;canReuse:boolean;blockedReason?:string;createHref:string;downloadHref?:string;downloads?:Array<{href:string;label:string}>;onAttach:(taskID:string)=>Promise<unknown>}){
  const reusableTasks=tasks.filter(task=>task.project.id===itemProjectID&&!completedTaskStatuses.includes(task.status));
  const [taskID,setTaskID]=useState(reusableTasks.some(task=>task.id===initialTaskID)?initialTaskID:'');
  const [busy,setBusy]=useState(false);const [notice,setNotice]=useState('');const [error,setError]=useState('');
  const attach=async()=>{if(!taskID||!canReuse)return;setBusy(true);setError('');try{await onAttach(taskID);setNotice('已加入所选创作任务')}catch(value){setError(value instanceof Error?value.message:'加入任务失败')}finally{setBusy(false)}};
  return <section className="studio-asset-use-panel"><h2>继续使用</h2>{downloadHref&&<a href={downloadHref} download><Download size={15}/>下载原文件</a>}{downloads.map(file=><a href={file.href} download key={file.href}><Download size={15}/>{file.label}</a>)}{canReuse?<>{reusableTasks.length>0&&<label><span>加入已有创作</span><select value={taskID} onChange={event=>setTaskID(event.target.value)}><option value="">选择进行中的任务</option>{reusableTasks.map(task=><option key={task.id} value={task.id}>{task.title}</option>)}</select></label>}{reusableTasks.length>0&&<Button disabled={!taskID||busy} onClick={()=>void attach()}><Plus size={15}/>{busy?'正在加入…':'加入所选任务'}</Button>}<Link className="studio-primary-link" to={createHref}><Sparkles size={15}/>用它开始新创作</Link></>:<div className="studio-asset-readonly-note"><AlertCircle size={16}/><span><strong>当前版本暂不可复用</strong>{blockedReason||'完成确认后即可用于新的创作。'}</span></div>}{notice&&<p className="is-success">{notice}</p>}{error&&<p className="is-error">{error}</p>}</section>;
}

function AssetPageLoading(){return <div className="studio-asset-page-state"><LoaderCircle className="is-spinning" size={20}/>正在打开资产…</div>}
function AssetPageError({message,onRetry}:{message:string;onRetry:()=>Promise<void>}){return <div className="studio-view studio-asset-detail-page"><Link className="studio-back" to="/studio/assets"><ArrowLeft size={15}/>返回资产</Link><div className="studio-asset-page-state is-error"><AlertCircle size={22}/><strong>资产无法打开</strong><p>{message}</p><Button variant="secondary" onClick={()=>void onRetry()}>重新加载</Button></div></div>}
