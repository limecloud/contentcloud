import {
  AlertCircle,
  ArrowLeft,
  Archive,
  CheckCircle2,
  Download,
  File,
  FileSpreadsheet,
  FileText,
  Image as ImageIcon,
  Library,
  LoaderCircle,
  Music2,
  PencilLine,
  Plus,
  Sparkles,
  Video,
} from 'lucide-react';
import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { Link, useParams, useSearchParams } from 'react-router-dom';
import { Button } from '../components/ui';
import { studioApi } from './studioApi';
import type {
  StudioAssetItem,
  StudioAssetMedia,
  StudioCreativeResultDetail,
  StudioTaskSummary,
  WorkspaceMaterialItem,
} from './studioTypes';

type UnknownRecord=Record<string,unknown>;

const completedStatuses=['delivered','cancelled','canceled'];
const resultTypeLabels:Record<StudioAssetItem['result_type'],string>={persona:'人物原型',script:'剧本',storyboard:'分镜',image:'图片',video:'视频'};
const materialTypeLabels:Record<WorkspaceMaterialItem['material_kind'],string>={document:'文档',image:'图片',video:'视频',audio:'音频',table:'表格',other:'其他文件'};

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
  return <AssetDetailFrame
    backTo="/studio/assets"
    eyebrow={`我的资产 · ${materialTypeLabels[item.material_kind]}`}
    title={item.title}
    state={materialStateLabel(item.processing_state)}
    icon={<MaterialIcon kind={item.material_kind}/>}>
    <MaterialViewer item={item}/>
    <AssetFacts title="文件信息" rows={[
      ['文件名',item.file_name],['所属项目',item.project_name],['文件类型',item.mime_type],['文件大小',formatBytes(item.byte_size)],['来源',materialOriginLabel(item.origin)],['权利提示',item.rights_summary],['更新时间',formatDate(item.updated_at)],
    ]}/>
    <AssetUsePanel
      itemProjectID={item.project_id}
      tasks={tasks}
      initialTaskID={searchParams.get('task_id')||''}
      canReuse
      createHref={`/studio/tasks/new?material_ref=${encodeURIComponent(item.material_ref)}&project=${encodeURIComponent(item.project_id)}`}
      downloadHref={materialHref(item)}
      onAttach={taskID=>studioApi.attachMaterials(taskID,[item.material_ref])}/>
  </AssetDetailFrame>;
}

export function StudioCreativeResultDetailPage(){
  const {taskID='',resultID=''}=useParams();
  const [searchParams]=useSearchParams();
  const [detail,setDetail]=useState<StudioCreativeResultDetail>();
  const [tasks,setTasks]=useState<StudioTaskSummary[]>([]);
  const [loading,setLoading]=useState(true);
  const [error,setError]=useState('');
  const load=useCallback(async()=>{
    setLoading(true);setError('');
    try{
      const [nextDetail,nextTasks]=await Promise.all([studioApi.creativeResult(resultID,taskID),studioApi.tasks()]);
      setDetail(nextDetail);setTasks(nextTasks);
    }catch(value){setError(value instanceof Error?value.message:'创作结果加载失败')}
    finally{setLoading(false)}
  },[resultID,taskID]);
  useEffect(()=>{void load()},[load]);
  if(loading)return <AssetPageLoading/>;
  if(error||!detail)return <AssetPageError message={error||'没有找到这项创作结果'} onRetry={load}/>;
  const {item}=detail;
  return <AssetDetailFrame
    backTo="/studio/assets"
    eyebrow={`创作结果 · ${resultTypeLabels[item.result_type]}`}
    title={item.title}
    state={resultStateLabel(item)}
    icon={<ResultIcon type={item.result_type}/>}
    sourceTask={<Link to={`/studio/tasks/${encodeURIComponent(item.task_id)}`}><PencilLine size={15}/>在来源任务中修改</Link>}>
    <CreativeResultViewer detail={detail}/>
    <AssetFacts title="版本信息" rows={[
      ['所属项目',item.project_name],['来源任务',item.task_title],['当前版本',item.version],['确认状态',resultStateLabel(item)],['生成时间',formatDate(item.created_at)],
    ]}/>
    <AssetUsePanel
      itemProjectID={item.project_id}
      tasks={tasks}
      initialTaskID={searchParams.get('task_id')||''}
      canReuse={item.reusable}
      blockedReason={item.blocked_reason}
      createHref={`/studio/tasks/new?asset_ref=${encodeURIComponent(item.ref)}&project=${encodeURIComponent(item.project_id)}`}
      downloads={item.downloads.map(file=>({href:file.href,label:file.file_name}))}
      onAttach={targetTaskID=>studioApi.attachAssets(targetTaskID,[item.ref])}/>
  </AssetDetailFrame>;
}

function AssetDetailFrame({backTo,eyebrow,title,state,icon,sourceTask,children}:{backTo:string;eyebrow:string;title:string;state:string;icon:ReactNode;sourceTask?:ReactNode;children:ReactNode}){
  const parts=Array.isArray(children)?children:[children];
  return <div className="studio-view studio-asset-detail-page">
    <Link className="studio-back" to={backTo}><ArrowLeft size={15}/>返回资产</Link>
    <header className="studio-asset-page-header"><span className="studio-asset-page-icon">{icon}</span><div><small>{eyebrow}</small><h1>{title}</h1><div><b>{state}</b><span><CheckCircle2 size={13}/>固定版本，只读</span></div></div>{sourceTask&&<div className="studio-asset-source-action">{sourceTask}</div>}</header>
    <div className="studio-asset-page-layout"><main>{parts[0]}</main><aside>{parts.slice(1)}</aside></div>
  </div>;
}

function MaterialViewer({item}:{item:WorkspaceMaterialItem}){
  const href=materialHref(item);
  if(item.material_kind==='image')return <section className="studio-asset-viewer is-image"><ViewerHeading label="图片查看器"/><div className="studio-image-canvas"><img src={href} alt={item.title}/></div></section>;
  if(item.material_kind==='video')return <section className="studio-asset-viewer is-video"><ViewerHeading label="视频播放器"/><div className="studio-video-canvas"><video src={href} controls preload="metadata">当前浏览器无法播放这个视频。</video></div></section>;
  if(item.material_kind==='audio')return <section className="studio-asset-viewer is-audio"><ViewerHeading label="音频播放器"/><div className="studio-audio-canvas"><Music2 size={36}/><strong>{item.file_name}</strong><audio src={href} controls preload="metadata">当前浏览器无法播放这个音频。</audio></div></section>;
  if(item.material_kind==='table')return <TableMaterialViewer item={item}/>;
  if(item.material_kind==='document')return <DocumentMaterialViewer item={item}/>;
  return <UnsupportedFileViewer item={item} label="文件预览"/>;
}

function DocumentMaterialViewer({item}:{item:WorkspaceMaterialItem}){
  const href=materialHref(item);
  if(item.mime_type==='application/pdf')return <section className="studio-asset-viewer is-document"><ViewerHeading label="PDF 阅读器"/><iframe src={href} title={item.title}/></section>;
  if(item.mime_type==='text/markdown'||item.mime_type==='text/plain')return <TextFileViewer item={item} markdown={item.mime_type==='text/markdown'}/>;
  return <UnsupportedFileViewer item={item} label={item.mime_type.includes('wordprocessing')?'Word 文档':item.mime_type.includes('presentation')?'演示文稿':'文档预览'}/>;
}

function TextFileViewer({item,markdown}:{item:WorkspaceMaterialItem;markdown:boolean}){
  const {text,error,loading}=useTextFile(materialHref(item));
  return <section className="studio-asset-viewer is-document"><ViewerHeading label={markdown?'Markdown 阅读器':'文本阅读器'}/>{loading?<InlineLoading label="正在读取正文…"/>:error?<InlineError message={error}/>:markdown?<article className="studio-asset-markdown"><ReactMarkdown remarkPlugins={[remarkGfm]} skipHtml>{text}</ReactMarkdown></article>:<pre className="studio-asset-plain-text">{text}</pre>}</section>;
}

function TableMaterialViewer({item}:{item:WorkspaceMaterialItem}){
  if(item.mime_type!=='text/csv')return <UnsupportedFileViewer item={item} label="Excel 表格"/>;
  const {text,error,loading}=useTextFile(materialHref(item));
  const rows=useMemo(()=>text?parseCSV(text):[],[text]);
  return <section className="studio-asset-viewer is-table"><ViewerHeading label="表格查看器"/>{loading?<InlineLoading label="正在读取表格…"/>:error?<InlineError message={error}/>:rows.length===0?<InlineError message="表格中没有可显示的数据"/>:<div className="studio-table-canvas"><table><thead><tr>{rows[0].map((cell,index)=><th key={`${cell}-${index}`}>{cell||`第 ${index+1} 列`}</th>)}</tr></thead><tbody>{rows.slice(1,201).map((row,rowIndex)=><tr key={rowIndex}>{rows[0].map((_,cellIndex)=><td key={cellIndex}>{row[cellIndex]||''}</td>)}</tr>)}</tbody></table>{rows.length>201&&<small>当前显示前 200 行，下载文件可查看全部内容。</small>}</div>}</section>;
}

function UnsupportedFileViewer({item,label}:{item:WorkspaceMaterialItem;label:string}){
  return <section className="studio-asset-viewer is-file"><ViewerHeading label={label}/><div className="studio-file-canvas"><File size={38}/><strong>{item.file_name}</strong><p>浏览器暂不支持直接渲染这种文件，原文件保持不变。</p><a href={materialHref(item)} download><Download size={15}/>下载原文件</a></div></section>;
}

function CreativeResultViewer({detail}:{detail:StudioCreativeResultDetail}){
  switch(detail.item.result_type){
    case'image':return <ResultImageViewer item={detail.item}/>;
    case'video':return <ResultVideoViewer item={detail.item}/>;
    case'persona':return <PersonaViewer content={detail.content} summary={detail.item.summary}/>;
    case'script':return <ScriptViewer content={detail.content} summary={detail.item.summary}/>;
    case'storyboard':return <StoryboardViewer content={detail.content} media={detail.media} summary={detail.item.summary}/>;
  }
}

function ResultImageViewer({item}:{item:StudioAssetItem}){
  const file=item.downloads.find(value=>value.media_type.startsWith('image/'));
  return <section className="studio-asset-viewer is-image"><ViewerHeading label="图片查看器"/>{file?<div className="studio-image-canvas"><img src={file.href} alt={item.title}/></div>:<ViewerSummary icon={<ImageIcon size={32}/>} title="图片文件尚未登记" detail={item.summary}/>}</section>;
}

function ResultVideoViewer({item}:{item:StudioAssetItem}){
  const file=item.downloads.find(value=>value.media_type.startsWith('video/'));
  return <section className="studio-asset-viewer is-video"><ViewerHeading label="视频播放器"/>{file?<div className="studio-video-canvas"><video src={file.href} controls preload="metadata">当前浏览器无法播放这个视频。</video></div>:<ViewerSummary icon={<Video size={32}/>} title="视频文件尚未登记" detail={item.summary}/>}</section>;
}

function PersonaViewer({content,summary}:{content:unknown;summary:string}){
  const facts=list(record(content).facts).map(record);
  return <section className="studio-asset-viewer is-persona"><ViewerHeading label="人物原型工作面"/>{facts.length===0?<ViewerSummary icon={<Library size={32}/>} title="人物原型已固定" detail={summary}/>:<div className="studio-persona-sheet"><header><span>人物定位</span><strong>{facts.length} 个已确认要点</strong></header><div>{facts.map((fact,index)=><article key={`${stringValue(fact.title)}-${index}`}><small>{personLayerLabel(stringValue(fact.layer))}</small><h2>{stringValue(fact.title,`人物要点 ${index+1}`)}</h2><p>{stringValue(fact.statement,'暂未提供说明')}</p><span>{stringValue(fact.object_type,'人物事实')}</span></article>)}</div></div>}</section>;
}

function ScriptViewer({content,summary}:{content:unknown;summary:string}){
  const root=primaryContent(content);
  const scenes=findArray(root,['scenes','segments','shots']).map(record);
  const title=stringValue(root.title,'已确认剧本');
  return <section className="studio-asset-viewer is-script"><ViewerHeading label="剧本文稿"/>{scenes.length===0?<StructuredContentFallback content={root} title={title} summary={summary}/>:<article className="studio-script-sheet"><header><small>确认稿</small><h2>{title}</h2><p>{stringValue(root.summary,summary)}</p></header><ol>{scenes.map((scene,index)=><li key={stringValue(scene.id,String(index))}><span>{String(index+1).padStart(2,'0')}</span><div><small>{scriptSceneMeta(scene)}</small><h3>{stringValue(scene.visual,stringValue(scene.scene,`场景 ${index+1}`))}</h3><p>{stringValue(scene.voiceover,stringValue(scene.narration,'无旁白'))}</p>{stringValue(scene.on_screen_text)&&<blockquote>{stringValue(scene.on_screen_text)}</blockquote>}</div></li>)}</ol></article>}</section>;
}

function StoryboardViewer({content,media,summary}:{content:unknown;media:StudioAssetMedia[];summary:string}){
  const root=primaryContent(content);
  const shots=findArray(root,['shots','scenes']).map(record);
  const mediaByAsset=new Map(media.map(value=>[value.asset_ref,value.file]));
  return <section className="studio-asset-viewer is-storyboard"><ViewerHeading label="分镜工作台"/>{shots.length===0?<ViewerSummary icon={<Archive size={32}/>} title="分镜版本已固定" detail={summary}/>:<div className="studio-storyboard-grid">{shots.map((shot,index)=>{const assetID=stringValue(shot.first_frame_artifact_id);const file=mediaByAsset.get(assetID);return <article key={stringValue(shot.shot_id,String(index))}><div className="studio-storyboard-frame">{file?.media_type.startsWith('image/')?<img src={file.href} alt={`${stringValue(shot.shot_id,`镜头 ${index+1}`)}首帧`} loading="lazy"/>:<div><ImageIcon size={26}/><span>首帧待登记</span></div>}</div><header><strong>{stringValue(shot.shot_id,`镜头 ${String(index+1).padStart(2,'0')}`)}</strong><span>{shotDuration(shot)}</span></header><h2>{stringValue(shot.scene,stringValue(shot.subject,'未命名画面'))}</h2><p>{stringValue(shot.action,stringValue(shot.image_prompt_zh,'未提供画面动作'))}</p><small>{stringValue(shot.camera,stringValue(shot.composition,'机位待定'))}</small></article>})}</div>}</section>;
}

function StructuredContentFallback({content,title,summary}:{content:UnknownRecord;title:string;summary:string}){
  const hasContent=Object.keys(content).length>0;
  return <div className="studio-structured-fallback"><FileText size={30}/><h2>{title}</h2><p>{summary}</p>{hasContent&&<pre>{JSON.stringify(content,null,2)}</pre>}</div>;
}

function AssetFacts({title,rows}:{title:string;rows:Array<[string,string]>}){return <section className="studio-asset-facts"><h2>{title}</h2><dl>{rows.map(([label,value])=><div key={label}><dt>{label}</dt><dd>{value||'未记录'}</dd></div>)}</dl></section>}

function AssetUsePanel({itemProjectID,tasks,initialTaskID,canReuse,blockedReason,createHref,downloadHref,downloads=[],onAttach}:{itemProjectID:string;tasks:StudioTaskSummary[];initialTaskID:string;canReuse:boolean;blockedReason?:string;createHref:string;downloadHref?:string;downloads?:Array<{href:string;label:string}>;onAttach:(taskID:string)=>Promise<unknown>}){
  const reusableTasks=tasks.filter(task=>task.project.id===itemProjectID&&!completedStatuses.includes(task.status));
  const [taskID,setTaskID]=useState(reusableTasks.some(task=>task.id===initialTaskID)?initialTaskID:'');
  const [busy,setBusy]=useState(false);
  const [notice,setNotice]=useState('');
  const [error,setError]=useState('');
  const attach=async()=>{if(!taskID||!canReuse)return;setBusy(true);setError('');try{await onAttach(taskID);setNotice('已加入所选创作任务')}catch(value){setError(value instanceof Error?value.message:'加入任务失败')}finally{setBusy(false)}};
  return <section className="studio-asset-use-panel"><h2>继续使用</h2>{downloadHref&&<a href={downloadHref} download><Download size={15}/>下载原文件</a>}{downloads.map(file=><a href={file.href} download key={file.href}><Download size={15}/>{file.label}</a>)}{canReuse?<>{reusableTasks.length>0&&<label><span>加入已有创作</span><select value={taskID} onChange={event=>setTaskID(event.target.value)}><option value="">选择进行中的任务</option>{reusableTasks.map(task=><option key={task.id} value={task.id}>{task.title}</option>)}</select></label>}{reusableTasks.length>0&&<Button disabled={!taskID||busy} onClick={()=>void attach()}><Plus size={15}/>{busy?'正在加入…':'加入所选任务'}</Button>}<Link className="studio-primary-link" to={createHref}><Sparkles size={15}/>用它开始新创作</Link></>:<div className="studio-asset-readonly-note"><AlertCircle size={16}/><span><strong>当前版本暂不可复用</strong>{blockedReason||'完成确认后即可用于新的创作。'}</span></div>}{notice&&<p className="is-success">{notice}</p>}{error&&<p className="is-error">{error}</p>}</section>;
}

function ViewerHeading({label}:{label:string}){return <header className="studio-viewer-heading"><strong>{label}</strong><span>只读</span></header>}
function ViewerSummary({icon,title,detail}:{icon:ReactNode;title:string;detail:string}){return <div className="studio-viewer-summary">{icon}<strong>{title}</strong><p>{detail}</p></div>}
function AssetPageLoading(){return <div className="studio-asset-page-state"><LoaderCircle className="is-spinning" size={20}/>正在打开资产…</div>}
function AssetPageError({message,onRetry}:{message:string;onRetry:()=>Promise<void>}){return <div className="studio-view studio-asset-detail-page"><Link className="studio-back" to="/studio/assets"><ArrowLeft size={15}/>返回资产</Link><div className="studio-asset-page-state is-error"><AlertCircle size={22}/><strong>资产无法打开</strong><p>{message}</p><Button variant="secondary" onClick={()=>void onRetry()}>重新加载</Button></div></div>}
function InlineLoading({label}:{label:string}){return <div className="studio-inline-viewer-state"><LoaderCircle className="is-spinning" size={17}/>{label}</div>}
function InlineError({message}:{message:string}){return <div className="studio-inline-viewer-state is-error"><AlertCircle size={17}/>{message}</div>}

function useTextFile(href:string){
  const [text,setText]=useState('');const [loading,setLoading]=useState(true);const [error,setError]=useState('');
  useEffect(()=>{let active=true;setLoading(true);setError('');fetch(href,{credentials:'same-origin'}).then(response=>{if(!response.ok)throw new Error(`文件读取失败 (${response.status})`);return response.text()}).then(value=>{if(active)setText(value)}).catch(value=>{if(active)setError(value instanceof Error?value.message:'文件读取失败')}).finally(()=>{if(active)setLoading(false)});return()=>{active=false}},[href]);
  return {text,loading,error};
}

export function parseCSV(value:string):string[][]{
  const rows:string[][]=[];let row:string[]=[];let field='';let quoted=false;
  for(let index=0;index<value.length;index+=1){
    const char=value[index];
    if(quoted){if(char==='"'&&value[index+1]==='"'){field+='"';index+=1}else if(char==='"')quoted=false;else field+=char;continue}
    if(char==='"'){quoted=true;continue}
    if(char===','){row.push(field);field='';continue}
    if(char==='\n'){row.push(field);rows.push(row);row=[];field='';continue}
    if(char!=='\r')field+=char;
  }
  if(field||row.length){row.push(field);rows.push(row)}
  return rows;
}

function primaryContent(value:unknown):UnknownRecord{const root=record(value);const objects=list(root.objects);return objects.length?record(objects[0]):root}
function findArray(root:UnknownRecord,keys:string[]):unknown[]{for(const key of keys){const values=list(root[key]);if(values.length)return values}return[]}
function record(value:unknown):UnknownRecord{return typeof value==='object'&&value!==null&&!Array.isArray(value)?value as UnknownRecord:{}}
function list(value:unknown):unknown[]{return Array.isArray(value)?value:[]}
function stringValue(value:unknown,fallback=''){if(typeof value==='string'&&value.trim())return value.trim();if(typeof value==='number')return String(value);return fallback}
function scriptSceneMeta(scene:UnknownRecord){const duration=stringValue(scene.duration_seconds);const role=stringValue(scene.role);return [role,duration?`${duration} 秒`:''].filter(Boolean).join(' · ')||'场景'}
function shotDuration(shot:UnknownRecord){const start=Number(shot.start_ms||0);const end=Number(shot.end_ms||0);return end>start?`${((end-start)/1000).toFixed(1)} 秒`:'时长待定'}
function personLayerLabel(value:string){return {identity:'身份',market:'受众',expression:'表达',product:'产品',compliance:'边界'}[value]||'人物要点'}
function materialHref(item:WorkspaceMaterialItem){return `/api/studio/materials/${encodeURIComponent(item.material_ref.replace(/^material:/,''))}/download`}
function materialOriginLabel(value:WorkspaceMaterialItem['origin']){return {uploaded:'上传',imported:'导入',linked:'外部链接'}[value]}
function materialStateLabel(value:WorkspaceMaterialItem['processing_state']){return {uploading:'上传中',processing:'处理中',ready:'可预览',failed:'处理失败'}[value]}
function resultStateLabel(item:StudioAssetItem){return {draft:'草稿',pending_confirmation:'待确认',changes_requested:'需修改',confirmed:'已确认',delivered:'已交付',superseded:'已被替代',blocked:'已阻止'}[item.status]||item.status}
function formatDate(value:string){const date=new Date(value);return Number.isNaN(date.getTime())?'未知时间':new Intl.DateTimeFormat('zh-CN',{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'}).format(date)}
function formatBytes(value:number){if(value<1024)return`${value} B`;if(value<1024*1024)return`${(value/1024).toFixed(1)} KB`;return`${(value/(1024*1024)).toFixed(1)} MB`}
function MaterialIcon({kind}:{kind:WorkspaceMaterialItem['material_kind']}){if(kind==='image')return <ImageIcon size={22}/>;if(kind==='video')return <Video size={22}/>;if(kind==='audio')return <Music2 size={22}/>;if(kind==='table')return <FileSpreadsheet size={22}/>;if(kind==='document')return <FileText size={22}/>;return <File size={22}/>}
function ResultIcon({type}:{type:StudioAssetItem['result_type']}){if(type==='image')return <ImageIcon size={22}/>;if(type==='video')return <Video size={22}/>;if(type==='persona')return <Library size={22}/>;if(type==='storyboard')return <Archive size={22}/>;return <FileText size={22}/>}
