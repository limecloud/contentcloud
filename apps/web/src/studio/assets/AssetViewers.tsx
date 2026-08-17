import {
  AlertCircle,
  Archive,
  Download,
  File,
  FileSpreadsheet,
  FileText,
  Image as ImageIcon,
  Library,
  LoaderCircle,
  Music2,
  Video,
} from 'lucide-react';
import { useEffect, useMemo, useState, type ReactNode } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import type { StudioAssetItem, StudioAssetMedia, StudioCreativeResultDetail, WorkspaceMaterialItem } from '../studioTypes';
import { materialHref, parseCSV } from './assetFormat';

type UnknownRecord=Record<string,unknown>;

export function MaterialViewer({item}:{item:WorkspaceMaterialItem}){
  const href=materialHref(item);
  if(item.material_kind==='image')return <section className="studio-asset-viewer is-image"><ViewerHeading label="图片查看器"/><div className="studio-image-canvas"><img src={href} alt={item.title}/></div></section>;
  if(item.material_kind==='video')return <section className="studio-asset-viewer is-video"><ViewerHeading label="视频播放器"/><div className="studio-video-canvas"><video src={href} controls preload="metadata">当前浏览器无法播放这个视频。</video></div></section>;
  if(item.material_kind==='audio')return <section className="studio-asset-viewer is-audio"><ViewerHeading label="音频播放器"/><div className="studio-audio-canvas"><Music2 size={36}/><strong>{item.file_name}</strong><audio src={href} controls preload="metadata">当前浏览器无法播放这个音频。</audio></div></section>;
  if(item.material_kind==='table')return <TableMaterialViewer item={item}/>;
  if(item.material_kind==='document')return <DocumentMaterialViewer item={item}/>;
  return <UnsupportedFileViewer item={item} label="文件预览"/>;
}

export function CreativeResultViewer({detail}:{detail:StudioCreativeResultDetail}){
  switch(detail.item.result_type){
    case'image':return <ResultImageViewer item={detail.item}/>;
    case'video':return <ResultVideoViewer item={detail.item}/>;
    case'persona':return <PersonaViewer content={detail.content} summary={detail.item.summary}/>;
    case'script':return <ScriptViewer content={detail.content} summary={detail.item.summary}/>;
    case'storyboard':return <StoryboardViewer content={detail.content} media={detail.media} summary={detail.item.summary}/>;
  }
}

export function MaterialIcon({kind}:{kind:WorkspaceMaterialItem['material_kind']}){if(kind==='image')return <ImageIcon size={22}/>;if(kind==='video')return <Video size={22}/>;if(kind==='audio')return <Music2 size={22}/>;if(kind==='table')return <FileSpreadsheet size={22}/>;if(kind==='document')return <FileText size={22}/>;return <File size={22}/>}
export function ResultIcon({type}:{type:StudioAssetItem['result_type']}){if(type==='image')return <ImageIcon size={22}/>;if(type==='video')return <Video size={22}/>;if(type==='persona')return <Library size={22}/>;if(type==='storyboard')return <Archive size={22}/>;return <FileText size={22}/>}

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

function StructuredContentFallback({content,title,summary}:{content:UnknownRecord;title:string;summary:string}){const hasContent=Object.keys(content).length>0;return <div className="studio-structured-fallback"><FileText size={30}/><h2>{title}</h2><p>{summary}</p>{hasContent&&<pre>{JSON.stringify(content,null,2)}</pre>}</div>}
function ViewerHeading({label}:{label:string}){return <header className="studio-viewer-heading"><strong>{label}</strong><span>只读</span></header>}
function ViewerSummary({icon,title,detail}:{icon:ReactNode;title:string;detail:string}){return <div className="studio-viewer-summary">{icon}<strong>{title}</strong><p>{detail}</p></div>}
function InlineLoading({label}:{label:string}){return <div className="studio-inline-viewer-state"><LoaderCircle className="is-spinning" size={17}/>{label}</div>}
function InlineError({message}:{message:string}){return <div className="studio-inline-viewer-state is-error"><AlertCircle size={17}/>{message}</div>}

function useTextFile(href:string){
  const [text,setText]=useState('');const [loading,setLoading]=useState(true);const [error,setError]=useState('');
  useEffect(()=>{let active=true;setLoading(true);setError('');fetch(href,{credentials:'same-origin'}).then(response=>{if(!response.ok)throw new Error(`文件读取失败 (${response.status})`);return response.text()}).then(value=>{if(active)setText(value)}).catch(value=>{if(active)setError(value instanceof Error?value.message:'文件读取失败')}).finally(()=>{if(active)setLoading(false)});return()=>{active=false}},[href]);
  return {text,loading,error};
}

function primaryContent(value:unknown):UnknownRecord{const root=record(value);const objects=list(root.objects);return objects.length?record(objects[0]):root}
function findArray(root:UnknownRecord,keys:string[]):unknown[]{for(const key of keys){const values=list(root[key]);if(values.length)return values}return[]}
function record(value:unknown):UnknownRecord{return typeof value==='object'&&value!==null&&!Array.isArray(value)?value as UnknownRecord:{}}
function list(value:unknown):unknown[]{return Array.isArray(value)?value:[]}
function stringValue(value:unknown,fallback=''){if(typeof value==='string'&&value.trim())return value.trim();if(typeof value==='number')return String(value);return fallback}
function scriptSceneMeta(scene:UnknownRecord){const duration=stringValue(scene.duration_seconds);const role=stringValue(scene.role);return [role,duration?`${duration} 秒`:''].filter(Boolean).join(' · ')||'场景'}
function shotDuration(shot:UnknownRecord){const start=Number(shot.start_ms||0);const end=Number(shot.end_ms||0);return end>start?`${((end-start)/1000).toFixed(1)} 秒`:'时长待定'}
function personLayerLabel(value:string){return {identity:'身份',market:'受众',expression:'表达',product:'产品',compliance:'边界'}[value]||'人物要点'}
